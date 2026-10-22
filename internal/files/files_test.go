package files

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func setup(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{
		"apps/a/values.yaml", "apps/a/ks.yml", "apps/a/README.md", "ignored/values.yaml",
		"nested/values.yaml",
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("a: 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, filepath.Join(root, "nested")} {
		if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
			t.Fatal(string(out))
		}
	}
	return root
}

func TestDiscoverSkipsIgnoredFilesAndNestedRepositories(t *testing.T) {
	root := setup(t)
	if out, err := exec.Command("git", "-C", root, "add", "apps/a/values.yaml").CombinedOutput(); err != nil {
		t.Fatalf("git add: %s: %v", out, err)
	}
	got, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"apps/a/ks.yml", "apps/a/values.yaml"}; !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
	selected, err := Select(root, []string{"apps", ".", "apps/a/values.yaml"})
	if err != nil || !slices.Equal(selected, got) {
		t.Errorf("directory selection = %q, %v; want %q", selected, err, got)
	}
}

func TestSelectKeepsExistingYAMLInsideRoot(t *testing.T) {
	root := setup(t)
	got, err := Select(root, []string{
		filepath.Join(root, "apps/a/values.yaml"),
		"apps/a/values.yaml",
		"apps/a/README.md",
		"ignored/values.yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"apps/a/values.yaml", "ignored/values.yaml"}; !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestOutsideGitRequiresExplicitPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.yaml", "sub/b.yml", "sub/readme.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("a: 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"Git available", "Git unavailable"} {
		t.Run(name, func(t *testing.T) {
			if name == "Git unavailable" {
				t.Setenv("PATH", t.TempDir())
			}
			if _, err := Discover(root); err == nil || !strings.Contains(err.Error(), "explicit path") {
				t.Fatalf("Discover error = %v; want explicit-path instruction", err)
			}
			got, err := Select(root, []string{"sub", ".", "a.yaml"})
			if want := []string{"a.yaml", "sub/b.yml"}; err != nil || !slices.Equal(got, want) {
				t.Errorf("Select = %q, %v; want %q", got, err, want)
			}
		})
	}
}

func TestSelectionErrors(t *testing.T) {
	root := setup(t)
	for _, path := range []string{"missing.yaml", "../outside.yaml"} {
		if _, err := Select(root, []string{path}); err == nil {
			t.Errorf("Select(%q) succeeded", path)
		}
	}
	index := filepath.Join(root, "invalid-index")
	if err := os.WriteFile(index, []byte("invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_INDEX_FILE", index)
	if _, err := Discover(root); err == nil {
		t.Error("Discover ignored a Git failure")
	}
	if _, err := Select(root, []string{"."}); err == nil {
		t.Error("Select ignored a Git failure")
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := Select(root, []string{"."}); err == nil {
		t.Error("Select walked a Git repository without honoring exclusions")
	}
}

func TestSelectDoesNotFollowSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "a.yaml"), []byte("a: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	got, err := Select(root, []string{".", "linked"})
	if err != nil || len(got) != 0 {
		t.Errorf("Select followed a directory symlink: %q, %v", got, err)
	}
}
