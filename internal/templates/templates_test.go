package templates

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"go.yaml.in/yaml/v3"

	"github.com/JacobSartin/homelab-fmt/internal/rules"
)

func load(files map[string]string) (*Set, error) {
	fsys := fstest.MapFS{}
	for name, text := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(text)}
	}
	return Load(fsys)
}

func mustLoad(t *testing.T, files map[string]string) *Set {
	t.Helper()
	set, err := load(files)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func parseDoc(t *testing.T, text string) *yaml.Node {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(text), &node); err != nil {
		t.Fatal(err)
	}
	return &node
}

func ruleAt(t *testing.T, tmpl *Template, path string) rules.Rule {
	t.Helper()
	i := slices.IndexFunc(tmpl.Rules, func(r rules.Rule) bool { return r.Path == path })
	if i < 0 {
		t.Fatalf("no rule for %q in %v", path, tmpl.Rules)
	}
	return tmpl.Rules[i]
}

func TestCompile(t *testing.T) {
	set := mustLoad(t, map[string]string{"t.yaml": `apiVersion: v1
kind: ConfigMap
metadata:
  name: x
  labels: {} # homelab-fmt: sort
data:
  "*": # homelab-fmt: compact
    b: 1

    a: 2
list:
  - z: 1
    y: 2
`})
	tmpl := set.Templates()[0]
	root := []string{"apiVersion", "kind", "metadata", "data", "list"}
	if got := ruleAt(t, tmpl, "").Order; !slices.Equal(got, root) {
		t.Errorf("root order %q", got)
	}
	if !ruleAt(t, tmpl, "metadata.labels").SortRest {
		t.Error("labels not sorted")
	}
	wild := ruleAt(t, tmpl, "data.*")
	if wild.Spacing != rules.Compact || !slices.Equal(wild.BlankAfter, []string{"b"}) {
		t.Errorf("data.* rule %+v", wild)
	}
	if got := ruleAt(t, tmpl, "list.*").Order; !slices.Equal(got, []string{"z", "y"}) {
		t.Errorf("sequence item order %q", got)
	}
	if got := ruleAt(t, tmpl, "metadata").Source; got != "t.yaml:3" {
		t.Errorf("source %q", got)
	}
}

func TestNamedKeysWinOverWildcards(t *testing.T) {
	set := mustLoad(t, map[string]string{"t.yaml": `apiVersion: v1
kind: List
items:
  "*":
    a: 1
    b: 2
  special:
    b: 1
    a: 2
`})
	got, _ := rules.First(set.Templates()[0].Rules, []string{"items", "special"})
	if !slices.Equal(got.Order, []string{"b", "a"}) {
		t.Errorf("got %+v", got)
	}
}

func TestExtendsMergesPathByPath(t *testing.T) {
	set := mustLoad(t, map[string]string{
		"base.yaml": "apiVersion: \"*\"\nkind: \"*\"\nmetadata:\n  name: x\n  namespace: y\n" +
			"  labels: {} # homelab-fmt: sort\n",
		"child.yaml": "# homelab-fmt: extends base.yaml\napiVersion: apps/v1\nkind: Deployment\n" +
			"metadata:\n  namespace: y\nspec:\n  replicas: 1\n",
	})
	child := set.Templates()[1]
	if !slices.Equal(child.Chain, []string{"child.yaml", "base.yaml"}) {
		t.Errorf("chain %q", child.Chain)
	}
	if got := ruleAt(t, child, "metadata").Order; !slices.Equal(got, []string{"namespace", "name", "labels"}) {
		t.Errorf("merged metadata order %q", got)
	}
	if !ruleAt(t, child, "metadata.labels").SortRest {
		t.Error("parent rule missing")
	}
	root := []string{"apiVersion", "kind", "metadata", "spec"}
	if got := ruleAt(t, child, "").Order; !slices.Equal(got, root) {
		t.Errorf("merged root order %q", got)
	}
}

func TestMatchPicksTheMostSpecificTemplate(t *testing.T) {
	set := mustLoad(t, map[string]string{
		"any.yaml":    "apiVersion: \"*\"\nkind: \"*\"\n",
		"apps.yaml":   "apiVersion: apps/v1\nkind: \"*\"\n",
		"deploy.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
		"core.yaml":   "apiVersion: v1\nkind: ConfigMap\n",
		"values.yaml": "# homelab-fmt: schema */chart/values.schema.json\nkey: 1\n",
	})
	for text, want := range map[string]string{
		"apiVersion: apps/v2\nkind: Deployment\n":                                                "deploy.yaml",
		"apiVersion: apps/v1\nkind: StatefulSet\n":                                               "apps.yaml",
		"apiVersion: v1\nkind: ConfigMap\n":                                                      "core.yaml",
		"apiVersion: v1\nkind: Secret\n":                                                         "any.yaml",
		"# yaml-language-server: $schema=https://example.com/chart/values.schema.json\nkey: 1\n": "values.yaml",
		"key: 1\n": "",
	} {
		name := ""
		if got := set.Match(parseDoc(t, text)); got != nil {
			name = got.Name
		}
		if name != want {
			t.Errorf("%q matched %q, want %q", text, name, want)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	for want, files := range map[string]map[string]string{
		"unknown directive":            {"t.yaml": "apiVersion: v1\nkind: A\nx: {} # homelab-fmt: alphabetize\n"},
		"directives apply to mappings": {"t.yaml": "apiVersion: v1\nkind: A\nx: 1 # homelab-fmt: sort\n"},
		"apiVersion and kind":          {"t.yaml": "a: 1\n"},
		"one document":                 {"t.yaml": "apiVersion: v1\nkind: A\n---\nb: 1\n"},
		"unknown template":             {"t.yaml": "# homelab-fmt: extends nope.yaml\napiVersion: v1\nkind: A\n"},
		"extends cycle": {
			"a.yaml": "# homelab-fmt: extends b.yaml\napiVersion: v1\nkind: A\n",
			"b.yaml": "# homelab-fmt: extends a.yaml\napiVersion: v1\nkind: B\n",
		},
		"select the same documents": {
			"a.yaml": "apiVersion: v1\nkind: A\n",
			"b.yaml": "apiVersion: v1\nkind: A\n",
		},
	} {
		if _, err := load(files); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want error containing %q, got %v", want, err)
		}
	}
}

// Every bundled template must load and select documents like itself.
func TestBundledTemplates(t *testing.T) {
	for _, tmpl := range Default.Templates() {
		if got := Default.Match(parseDoc(t, tmpl.Text)); got != tmpl {
			t.Errorf("%s matches template %v", tmpl.Name, got)
		}
	}
}

func TestWildcardVariantsMergeTheirOrders(t *testing.T) {
	set := mustLoad(t, map[string]string{"t.yaml": `apiVersion: v1
kind: List
volumes:
  "*claim":
    type: pvc
    claim: x
    mounts: []
  "*nfs":
    type: nfs
    server: x
    path: x
    mounts: []
`})
	want := []string{"type", "claim", "server", "path", "mounts"}
	if got := ruleAt(t, set.Templates()[0], "volumes.*").Order; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
