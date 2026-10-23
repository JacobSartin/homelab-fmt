package templates

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func source(name string, files map[string]string) Source {
	fsys := fstest.MapFS{}
	for name, text := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(text)}
	}
	return Source{FS: fsys, Name: name}
}

type unreadableOverride struct{ fs.FS }

func (f unreadableOverride) Open(name string) (fs.File, error) {
	if name == "base.yaml" {
		return nil, fs.ErrPermission
	}
	return f.FS.Open(name)
}

func TestUnreadableOverrideDoesNotRestoreBundledTemplate(t *testing.T) {
	base := source("bundled", map[string]string{
		"base.yaml":  "apiVersion: v1\nkind: A\n",
		"child.yaml": "# homelab-fmt: extends base.yaml\napiVersion: v1\nkind: B\n",
	})
	user := source("user", map[string]string{"base.yaml": "apiVersion: v1\nkind: A\n"})
	user.FS = unreadableOverride{user.FS}
	set, diagnostics := LoadSources(base, user)
	if len(set.Templates()) != 0 || len(diagnostics) != 2 {
		t.Fatalf("templates = %v, diagnostics = %v", set.Templates(), diagnostics)
	}
	if !strings.Contains(diagnostics[0].Error(), "user/base.yaml:1:") || !strings.Contains(diagnostics[1].Error(), "bundled/child.yaml:1:") {
		t.Errorf("source diagnostics = %v", diagnostics)
	}
}

func TestDuplicateSelectorsAcrossSourcesExcludeDescendants(t *testing.T) {
	set, diagnostics := LoadSources(source("bundled", map[string]string{
		"base.yaml":  "apiVersion: v1\nkind: A\n",
		"child.yaml": "# homelab-fmt: extends base.yaml\napiVersion: v1\nkind: B\n",
	}), source("user", map[string]string{
		"duplicate.yaml": "apiVersion: v1\nkind: A\n", "valid.yaml": "apiVersion: v1\nkind: C\n",
	}))
	if len(set.Templates()) != 1 || set.Templates()[0].Source != "user/valid.yaml" || len(diagnostics) != 3 {
		t.Fatalf("templates = %v, diagnostics = %v", set.Templates(), diagnostics)
	}
	for _, path := range []string{"bundled/base.yaml:1:", "user/duplicate.yaml:1:", "bundled/child.yaml:1:"} {
		if !strings.Contains(diagnostics[0].Error()+diagnostics[1].Error()+diagnostics[2].Error(), path) {
			t.Errorf("missing source %q in diagnostics %v", path, diagnostics)
		}
	}
}

func TestThreeSourcesUseLastOverride(t *testing.T) {
	var sources []Source
	for _, name := range []string{"first", "second", "third"} {
		sources = append(sources, source(name, map[string]string{"base.yaml": "apiVersion: v1\nkind: A\nmetadata:\n  " + name + ": x\n  name: y\n"}))
	}
	set, diagnostics := LoadSources(sources...)
	if len(set.Templates()) != 1 || len(diagnostics) != 0 {
		t.Fatalf("templates = %v, diagnostics = %v", set.Templates(), diagnostics)
	}
	tmpl := set.Templates()[0]
	if tmpl.Source != "third/base.yaml" || !slices.Equal(ruleAt(t, tmpl, "metadata").Order, []string{"third", "name"}) {
		t.Errorf("last override = %+v", tmpl)
	}
}

func TestSourcesOverrideBeforeLayering(t *testing.T) {
	set, diagnostics := LoadSources(source("bundled", map[string]string{
		"base.yaml":  "apiVersion: \"*\"\nkind: \"*\"\nmetadata:\n  name: x\n  namespace: y\n",
		"child.yaml": "# homelab-fmt: extends base.yaml\napiVersion: apps/v1\nkind: Deployment\n",
	}), source("user", map[string]string{
		"base.yaml":  "apiVersion: \"*\"\nkind: \"*\"\nmetadata:\n  namespace: y\n  name: x\n",
		"custom.yml": "# homelab-fmt: extends base.yaml\napiVersion: v1\nkind: ConfigMap\n",
	}))
	if len(diagnostics) != 0 || len(set.Templates()) != 3 {
		t.Fatalf("templates = %v, diagnostics = %v", set.Templates(), diagnostics)
	}
	child := set.Match(parseDoc(t, "apiVersion: apps/v1\nkind: Deployment\n"))
	if !slices.Equal(child.Chain, []string{"bundled/child.yaml", "user/base.yaml"}) {
		t.Errorf("chain = %q", child.Chain)
	}
	metadata := ruleAt(t, child, "metadata")
	if !slices.Equal(metadata.Order, []string{"namespace", "name"}) || metadata.Source != "user/base.yaml:3" {
		t.Errorf("override metadata = %+v", metadata)
	}
	custom := set.Match(parseDoc(t, "apiVersion: v1\nkind: ConfigMap\n"))
	if custom.Name != "custom.yml" || custom.Source != "user/custom.yml" {
		t.Errorf("custom template = %+v", custom)
	}
	if len(set.For(parseDoc(t, "apiVersion: v1\nkind: ConfigMap\n"))) == 0 {
		t.Error("combined set did not return formatting rules")
	}
}

func TestBrokenOverrideDoesNotRestoreBundledTemplate(t *testing.T) {
	set, diagnostics := LoadSources(source("bundled", map[string]string{
		"base.yaml":  "apiVersion: \"*\"\nkind: \"*\"\n",
		"child.yaml": "# homelab-fmt: extends base.yaml\napiVersion: v1\nkind: ConfigMap\n",
		"valid.yaml": "apiVersion: v1\nkind: Secret\n",
	}), source("user", map[string]string{"base.yaml": "invalid: [\n"}))
	if len(set.Templates()) != 1 || set.Templates()[0].Name != "valid.yaml" || len(diagnostics) != 2 {
		t.Fatalf("templates = %v, diagnostics = %v", set.Templates(), diagnostics)
	}
	if !strings.Contains(diagnostics[0].Error(), "user/base.yaml:") || !strings.Contains(diagnostics[1].Error(), "bundled/child.yaml:1:") {
		t.Errorf("source diagnostics = %v", diagnostics)
	}
	if set.Match(parseDoc(t, "apiVersion: v1\nkind: ConfigMap\n")) != nil {
		t.Error("invalid override or descendant was restored")
	}
}

func TestMissingTemplateDirectoryKeepsBundledTemplates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	set, diagnostics := WithDirectory(path)
	if len(set.Templates()) != len(Default.Templates()) || len(diagnostics) != 1 {
		t.Fatalf("templates = %v, diagnostics = %v", set.Templates(), diagnostics)
	}
	if !strings.Contains(diagnostics[0].Error(), filepath.ToSlash(path)+":1:") {
		t.Errorf("missing source directory in diagnostic: %v", diagnostics[0])
	}
}
