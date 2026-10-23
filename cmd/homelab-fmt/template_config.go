package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/JacobSartin/homelab-fmt/internal/templates"
)

func loadTemplates(root, selected string) (*templates.Set, string, []error) {
	if selected == "" {
		selected = os.Getenv("HOMELAB_FMT_TEMPLATES")
	}
	if selected == "" {
		base := root
		cmd := exec.Command("git", "rev-parse", "--show-toplevel")
		cmd.Dir = root
		if output, err := cmd.Output(); err == nil {
			base = strings.TrimSpace(string(output))
		}
		selected = filepath.Join(base, ".homelab-fmt", "templates")
		if _, err := os.Stat(selected); errors.Is(err, os.ErrNotExist) {
			return templates.Default, selected, templates.DefaultErrors
		}
	}
	if !filepath.IsAbs(selected) {
		selected = filepath.Join(root, selected)
	}
	set, diagnostics := templates.WithDirectory(selected)
	return set, selected, diagnostics
}

// Templates are configuration, including invalid files already reported as
// warnings. Do not run the formatter over its own selected template directory.
func withoutTemplates(root, dir string, paths []string) []string {
	return slices.DeleteFunc(paths, func(path string) bool {
		rel, err := filepath.Rel(dir, filepath.Join(root, path))
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	})
}
