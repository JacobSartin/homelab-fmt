package templates

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

// Load reads *.yaml files and resolves extends chains. Invalid templates and
// their descendants are skipped; the returned set is usable even with errors.
func Load(fsys fs.FS) (*Set, []error) {
	set := &Set{}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return set, []error{diagnostic("templates", err)}
	}
	var diagnostics []error
	invalid := map[string]bool{}
	reject := func(name string, err error) {
		invalid[name] = true
		diagnostics = append(diagnostics, diagnostic(name, err))
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			reject(name, err)
			continue
		}
		t, err := parse(name, strings.ReplaceAll(string(data), "\r\n", "\n"))
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
