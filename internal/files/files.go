// Package files finds the YAML files to format.
package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var yamlFile = regexp.MustCompile(`\.ya?ml$`)

// Discover finds tracked and untracked, non-ignored YAML files under root.
// Git leaves out nested repositories and worktrees. Outside Git, callers must
// explicitly select a directory instead of implicitly walking the current one.
func Discover(root string) ([]string, error) {
	found, available, err := gitFiles(root, ".")
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, fmt.Errorf("Git discovery unavailable; pass an explicit path such as \".\" to search a directory")
	}
	return found, nil
}

// Select finds YAML files among paths inside root. Explicit files can be ignored
// by Git; directories respect Git exclusions when Git is available.
func Select(root string, paths []string) ([]string, error) {
	var found []string
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("path %q is outside the current directory", path)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			listed, available, err := gitFiles(root, rel)
			if err != nil {
				return nil, err
			}
			if !available {
				err = filepath.WalkDir(path, func(name string, entry fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if entry.IsDir() && entry.Name() == ".git" {
						return filepath.SkipDir
					}
					if entry.Type().IsRegular() && yamlFile.MatchString(name) {
						rel, err := filepath.Rel(root, name)
						if err != nil {
							return err
						}
						listed = append(listed, filepath.ToSlash(rel))
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
			}
			found = append(found, listed...)
		} else if info.Mode().IsRegular() && yamlFile.MatchString(rel) {
			found = append(found, filepath.ToSlash(rel))
		}
	}
	slices.Sort(found)
	return slices.Compact(found), nil
}

func gitFiles(root, path string) ([]string, bool, error) {
	cmd := exec.Command("git", "--literal-pathspecs", "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", filepath.ToSlash(path))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		for dir := root; ; dir = filepath.Dir(dir) {
			_, statErr := os.Lstat(filepath.Join(dir, ".git"))
			if statErr == nil {
				return nil, false, fmt.Errorf("Git is required to honor repository exclusions: %w", err)
			}
			if !os.IsNotExist(statErr) {
				return nil, false, statErr
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
		return nil, false, nil
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if strings.Contains(string(exit.Stderr), "not a git repository") {
				return nil, false, nil
			}
			return nil, false, fmt.Errorf("listing files with Git: %w: %s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, false, fmt.Errorf("listing files with Git: %w", err)
	}
	var found []string
	for _, path := range strings.Split(string(out), "\x00") {
		if !yamlFile.MatchString(path) {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, path))
		if os.IsNotExist(err) {
			continue // A tracked file may have been deleted from the working tree.
		}
		if err != nil {
			return nil, true, err
		}
		if info.Mode().IsRegular() {
			found = append(found, filepath.ToSlash(path))
		}
	}
	slices.Sort(found)
	return slices.Compact(found), true, nil
}
