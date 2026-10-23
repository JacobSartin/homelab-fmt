package templates

import (
	"fmt"
	"slices"
	"strings"

	"github.com/JacobSartin/homelab-fmt/internal/rules"
)

// fills t.Chain and t.Rules from t and the templates it extends.
func (s *Set) resolve(t *Template, seen []string, invalid map[string]bool) error {
	if slices.Contains(seen, t.Name) {
		return fmt.Errorf("line %d: extends cycle: %s", t.extendsLine, strings.Join(append(seen, t.Name), " -> "))
	}
	t.Chain = []string{t.Source}
	t.Rules = slices.Clone(t.own)
	if t.Extends != "" {
		if invalid[t.Extends] {
			return fmt.Errorf("line %d: extends invalid template %s", t.extendsLine, t.Extends)
		}
		i := slices.IndexFunc(s.templates, func(p *Template) bool { return p.Name == t.Extends })
		if i < 0 {
			return fmt.Errorf("line %d: extends unknown template %s", t.extendsLine, t.Extends)
		}
		parent := s.templates[i]
		if err := s.resolve(parent, append(seen, t.Name), invalid); err != nil {
			return fmt.Errorf("line %d: extends %s: %w", t.extendsLine, parent.Name, err)
		}
		t.Chain = append(t.Chain, parent.Chain...)
		t.Rules = merge(t.own, parent.Rules)
	}
	// Literal paths win over wildcard paths for the same mapping.
	slices.SortStableFunc(t.Rules, func(a, b rules.Rule) int {
		return strings.Count(a.Path, "*") - strings.Count(b.Path, "*")
	})
	return nil
}

// for the same path, the child's keys come first, then the parent's keys
// the child does not list.
func merge(child, parent []rules.Rule) []rules.Rule {
	merged := slices.Clone(child)
	for _, p := range parent {
		i := slices.IndexFunc(merged, func(c rules.Rule) bool { return c.Path == p.Path && c.Items == p.Items })
		if i < 0 {
			merged = append(merged, p)
			continue
		}
		c := &merged[i]
		for _, key := range p.Order {
			if !slices.Contains(c.Order, key) {
				c.Order = append(c.Order, key)
			}
		}
		if !c.SortRest {
			c.SortMode = p.SortMode
		}
		c.SortRest = c.SortRest || p.SortRest
		if c.Spacing == rules.Keep {
			c.Spacing = p.Spacing
		}
		for _, key := range p.BlankAfter {
			if !slices.Contains(c.BlankAfter, key) {
				c.BlankAfter = append(c.BlankAfter, key)
			}
		}
		c.Source += ", " + p.Source
	}
	return merged
}
