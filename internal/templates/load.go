package templates

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Load reads *.yaml files and resolves extends chains. Invalid templates and
// their descendants are skipped; the returned set is usable even with errors.
func Load(fsys fs.FS) (*Set, []error) {
	return LoadSources(Source{FS: fsys})
}

// Source supplies templates from an FS root. Name identifies their directory
// in diagnostics and explanations; an empty Name keeps bare filenames.
type Source struct {
	FS   fs.FS
	Name string
}

// LoadSources combines sources in order, replacing files with the same name.
// Replacement happens before validation, so a bad override remains skipped.
func LoadSources(sources ...Source) (*Set, []error) {
	set := &Set{}
	var diagnostics []error
	type file struct {
		source, text string
		err          error
	}
	files := map[string]file{}
	for _, source := range sources {
		entries, err := fs.ReadDir(source.FS, ".")
		if err != nil {
			diagnostics = append(diagnostics, diagnostic(filepath.ToSlash(filepath.Join(source.Name, ".")), err))
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !(strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
				continue
			}
			data, err := fs.ReadFile(source.FS, name)
			files[name] = file{source: filepath.ToSlash(filepath.Join(source.Name, name)), text: strings.ReplaceAll(string(data), "\r\n", "\n"), err: err}
		}
	}
	invalid := map[string]bool{}
	reject := func(name string, err error) {
		invalid[name] = true
		diagnostics = append(diagnostics, diagnostic(files[name].source, err))
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		file := files[name]
		if file.err != nil {
			reject(name, file.err)
			continue
		}
		t, err := parse(name, file.source, file.text)
		if err != nil {
			reject(name, err)
			continue
		}
		set.templates = append(set.templates, t)
	}
	var resolved []*Template
	for _, t := range set.templates {
		if err := set.resolve(t, nil, invalid); err != nil {
			reject(t.Name, err)
			continue
		}
		resolved = append(resolved, t)
	}
	set.templates = resolved
	for i, t := range resolved {
		for _, other := range resolved[:i] {
			if sameSelector(t, other) {
				for _, conflict := range []*Template{other, t} {
					if !invalid[conflict.Name] {
						reject(conflict.Name, fmt.Errorf("line %d: templates %s and %s select the same documents", conflict.line, other.Name, t.Name))
					}
				}
			}
		}
	}
	// A structurally valid template can still extend a duplicate that was skipped.
	set.templates = nil
	candidates := &Set{templates: resolved}
	for _, t := range resolved {
		if invalid[t.Name] {
			continue
		}
		// Resolve against the original candidates while filtering the final set.
		if err := candidates.resolve(t, nil, invalid); err != nil {
			reject(t.Name, err)
			continue
		}
		set.templates = append(set.templates, t)
	}
	return set, diagnostics
}

var errorLine = regexp.MustCompile(`(?:yaml: )?line ([0-9]+):\s*`)

func diagnostic(name string, err error) error {
	line, reason := "1", err.Error()
	if m := errorLine.FindStringSubmatch(reason); m != nil {
		line = m[1]
		reason = strings.Replace(reason, m[0], "", 1)
	}
	reason = strings.Join(strings.Fields(reason), " ")
	return fmt.Errorf("template %s:%s: %s; skipped; matching documents fall back to the next most specific valid template or remain unchanged", name, line, reason)
}
