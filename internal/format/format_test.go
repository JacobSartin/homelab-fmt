package format

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/JacobSartin/homelab-fmt/internal/rules"
	"github.com/JacobSartin/homelab-fmt/internal/templates"
	"github.com/JacobSartin/homelab-fmt/internal/yamltext"
)

// Each testdata/<name>.in.yaml must format to testdata/<name>.out.yaml, and
// formatting the output again must not change it.
func TestGolden(t *testing.T) {
	inputs, err := filepath.Glob("testdata/*.in.yaml")
	if err != nil || len(inputs) == 0 {
		t.Fatal("no fixtures", err)
	}
	for _, input := range inputs {
		name := strings.TrimSuffix(filepath.Base(input), ".in.yaml")
		t.Run(name, func(t *testing.T) {
			in := read(t, input)
			want := read(t, strings.TrimSuffix(input, ".in.yaml")+".out.yaml")
			got := format(t, name+".yaml", in)
			if got != want {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
			if again := format(t, name+".yaml", got); again != got {
				t.Errorf("not idempotent:\n%s", again)
			}
		})
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func format(t *testing.T, path, text string) string {
	t.Helper()
	result, err := Format(path, text, templates.For)
	if err != nil {
		t.Fatal(err)
	}
	return result.Output
}

func with(set ...rules.Rule) RulesFor {
	return func(*yaml.Node) []rules.Rule { return set }
}

func TestRules(t *testing.T) {
	sorted := rules.Rule{Path: "", SortRest: true}
	for _, tc := range []struct {
		name, in, want string
		rules          []rules.Rule
	}{
		{
			name:  "listed keys first, rest kept",
			rules: []rules.Rule{{Path: "", Order: []string{"a"}}},
			in:    "z: 1\nb: 2\na: 3\n",
			want:  "a: 3\n\nz: 1\nb: 2\n",
		},
		{
			name:  "unlisted keys are separated from listed ones, also when sorted",
			rules: []rules.Rule{{Path: "m", Order: []string{"a", "b"}, SortRest: true}},
			in:    "m:\n  y: 1\n  b: 2\n  x: 3\n  a: 4\nn: 5\n",
			want:  "m:\n  a: 4\n  b: 2\n\n  x: 3\n  y: 1\nn: 5\n",
		},
		{
			name:  "compact wins over the separator",
			rules: []rules.Rule{{Path: "", Order: []string{"a"}, Spacing: rules.Compact}},
			in:    "z: 1\n\na: 2\n",
			want:  "a: 2\nz: 1\n",
		},
		{
			name:  "comments move with their key, blank lines stay",
			rules: []rules.Rule{{Path: "m", SortRest: true}},
			in:    "m:\n  b: 1\n\n  # about a\n  a: 2\n  c: 3\n",
			want:  "m:\n  # about a\n  a: 2\n\n  b: 1\n  c: 3\n",
		},
		{
			name:  "the document's opening comment stays on top",
			rules: []rules.Rule{sorted},
			in:    "# file comment\nb: 1\na: 2\n",
			want:  "# file comment\na: 2\nb: 1\n",
		},
		{
			name:  "sequence item mappings keep their dash",
			rules: []rules.Rule{{Path: "list.*", SortRest: true}},
			in:    "list:\n  - b: 1\n    a:\n      x: 1\n  - d: 1\n    c: 2\n",
			want:  "list:\n  - a:\n      x: 1\n    b: 1\n  - c: 2\n    d: 1\n",
		},
		{
			name: "spacing and blank-after rules",
			rules: []rules.Rule{
				{Path: "m", Spacing: rules.Compact, BlankAfter: []string{"b"}},
				{Path: "n", Spacing: rules.Separate},
			},
			in:   "m:\n  a: 1\n\n  b: 2\n  c: 3\nn:\n  x: 1\n  y: 2\n",
			want: "m:\n  a: 1\n  b: 2\n\n  c: 3\nn:\n  x: 1\n\n  y: 2\n",
		},
		{
			name:  "separate list items, with comments above the dash",
			rules: []rules.Rule{{Path: "list", Items: true, Spacing: rules.Separate}},
			in:    "list:\n  - a: 1\n    b: 2\n  # about c\n  - c: 3\n  -\n    d: 4\n  - # e\n    e: 5\nm: 1\n",
			want:  "list:\n  - a: 1\n    b: 2\n\n  # about c\n  - c: 3\n\n  -\n    d: 4\n\n  - # e\n    e: 5\nm: 1\n",
		},
		{
			name:  "compact list items, also at the key's indentation",
			rules: []rules.Rule{{Path: "list", Items: true, Spacing: rules.Compact}},
			in:    "list:\n- a\n\n- b\n\nm: 1\n",
			want:  "list:\n- a\n- b\n\nm: 1\n",
		},
		{
			name: "list rules leave mappings at the same path alone",
			rules: []rules.Rule{
				{Path: "*", Items: true, Spacing: rules.Separate},
				{Path: "*", Spacing: rules.Compact},
			},
			in:   "list:\n  - a\n  - b\nm:\n  x: 1\n\n  y: 2\n",
			want: "list:\n  - a\n\n  - b\nm:\n  x: 1\n  y: 2\n",
		},
		{
			name:  "flow lists are left alone",
			rules: []rules.Rule{{Path: "f", Items: true, Spacing: rules.Separate}},
			in:    "f: [a, b]\n",
			want:  "f: [a, b]\n",
		},
		{
			name:  "flow mappings are left alone",
			rules: []rules.Rule{{Path: "f", SortRest: true, Spacing: rules.Separate}},
			in:    "f: { b: 1, a: 2 }\n",
			want:  "f: { b: 1, a: 2 }\n",
		},
		{
			name:  "line endings are kept",
			rules: []rules.Rule{sorted},
			in:    "b: 1\r\na: 2\r\n",
			want:  "a: 2\r\nb: 1\r\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Format("x.yaml", tc.in, with(tc.rules...))
			if err != nil {
				t.Fatal(err)
			}
			if result.Output != tc.want {
				t.Errorf("got:\n%q\nwant:\n%q", result.Output, tc.want)
			}
		})
	}
}

func TestAliasBeforeAnchorIsLeftUnsorted(t *testing.T) {
	in := "service:\n  port: &port 80\ncontrollers:\n  port: *port\n"
	result, err := Format("x.yaml", in, with(rules.Rule{Path: "", Order: []string{"controllers"}}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != in {
		t.Errorf("changed:\n%s", result.Output)
	}
	want := []string{"(root): left unsorted because an alias would come before its anchor"}
	if !slices.Equal(result.Warnings, want) {
		t.Errorf("warnings %q", result.Warnings)
	}
}

func TestSOPSFilesAreNeverRewritten(t *testing.T) {
	in := "kind: Secret\napiVersion: v1\nsops:\n    mac: x\n"
	for _, path := range []string{"secret.sops.yaml", "secret.yaml"} {
		if got := format(t, path, in); got != in {
			t.Errorf("%s changed:\n%s", path, got)
		}
	}
}

func TestInvalidYAMLIsAnError(t *testing.T) {
	if _, err := Format("x.yaml", "a: [\n", templates.For); err == nil {
		t.Error("expected an error")
	}
}

// Every template must already follow its own rules, and must be restored
// after every mapping in it is reversed.
func TestTemplatesFormatToThemselves(t *testing.T) {
	for _, tmpl := range templates.Default.Templates() {
		t.Run(tmpl.Name, func(t *testing.T) {
			if got := format(t, tmpl.Name, tmpl.Text); got != tmpl.Text {
				t.Errorf("template is not in its own order:\n%s", got)
			}
			shuffled := reverseMappings(t, tmpl.Text)
			if shuffled == tmpl.Text {
				t.Fatal("reversing changed nothing")
			}
			if got := format(t, tmpl.Name, shuffled); got != tmpl.Text {
				t.Errorf("shuffled template formats to:\n%s", got)
			}
		})
	}
}

// reverseMappings reverses the entries of every block mapping in text.
func reverseMappings(t *testing.T, text string) string {
	t.Helper()
	done := map[string]bool{}
	for {
		docs, err := parse(text)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(text, "\n")
		changed := false
		for d, doc := range docs {
			changed = walk(doc.Content[0], nil, func(node *yaml.Node, path []string) bool {
				id := strconv.Itoa(d) + ":" + strings.Join(path, ".")
				if done[id] {
					return false
				}
				done[id] = true
				m, ok := yamltext.Layout(lines, node, node == doc.Content[0])
				keys := m.Keys()
				// Wildcard examples have no order of their own.
				if !ok || slices.ContainsFunc(keys, func(k string) bool { return strings.HasPrefix(k, "*") }) {
					return false
				}
				slices.Reverse(keys)
				text = strings.Join(m.Replace(lines, m.Render(lines, keys, m.Gaps(lines))), "\n")
				return true
			})
			if changed {
				break
			}
		}
		if !changed {
			return text
		}
	}
}

func TestExplain(t *testing.T) {
	in := "apiVersion: helm.toolkit.fluxcd.io/v2\nkind: HelmRelease\nmetadata:\n  name: x\n" +
		"---\nfoo: bar\n"
	got, err := Explain("x.yaml", in, templates.Default)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"x.yaml, document 1: template helm-release.yaml extends kubernetes.yaml\n",
		"  metadata: order name, generateName, namespace, labels, annotations (helm-release.yaml:",
		"x.yaml, document 2: no template matches; left as is\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestChangedReportsEdits(t *testing.T) {
	set := with(rules.Rule{Path: "", Order: []string{"a"}})
	for in, want := range map[string]bool{"a: 1\n\nb: 2\n": false, "b: 2\na: 1\n": true} {
		result, err := Format("x.yaml", in, set)
		if err != nil {
			t.Fatal(err)
		}
		if result.Changed != want {
			t.Errorf("%q: Changed = %v", in, result.Changed)
		}
	}
}
