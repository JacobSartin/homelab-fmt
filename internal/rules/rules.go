// Package rules describes key-order and blank-line rules for YAML mappings.
package rules

import (
	"fmt"
	"slices"
	"strings"
)

// blank lines between the entries of a mapping.
type Spacing int

const (
	Keep     Spacing = iota // leave spacing as-is
	Compact                 // remove blank lines
	Separate                // require a blank line
)

// how to order and space the mappings, or with Items the list items, found at Path.
type Rule struct {
	Path       string   // PathString-encoded keys; "*" matches any key or list index, "" is the root
	Items      bool     // spaces the items of a list; only Spacing applies
	Order      []string // keys moved to the front, in this order
	SortRest   bool     // sort unlisted keys instead of keeping their order
	Spacing    Spacing
	BlankAfter []string // keys followed by a blank line
	Source     string   // template file and line, shown by explain
}

// reports whether the rule can move keys.
func (r Rule) Reorders() bool { return len(r.Order) > 0 || r.SortRest }

// describes the rule for explain output.
func (r Rule) Summarize() string {
	var parts []string
	if len(r.Order) > 0 {
		parts = append(parts, "order "+strings.Join(r.Order, ", "))
	}
	if r.SortRest {
		parts = append(parts, "sort other keys")
	}
	switch r.Spacing {
	case Compact:
		parts = append(parts, "compact")
	case Separate:
		if r.Items {
			parts = append(parts, "separate items")
		} else {
			parts = append(parts, "separate entries")
		}
	}
	if len(r.BlankAfter) > 0 {
		parts = append(parts, "blank line after "+strings.Join(r.BlankAfter, ", "))
	}
	return fmt.Sprintf("%s (%s)", strings.Join(parts, "; "), r.Source)
}

// PathString joins keys with dots, escaping tildes as ~0 and dots as ~1.
// An empty key becomes ~2, distinct from the empty path used for the root.
// A "*" segment retains its meaning as a wildcard in rule patterns.
func PathString(path []string) string {
	parts := make([]string, len(path))
	for i, key := range path {
		parts[i] = pathKey(key)
	}
	return strings.Join(parts, ".")
}

func pathKey(key string) string {
	if key == "" {
		return "~2"
	}
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), ".", "~1")
}

// reports whether a PathString-encoded pattern matches the key path.
func Match(pattern string, path []string) bool {
	var parts []string
	if pattern != "" {
		parts = strings.Split(pattern, ".")
	}
	if len(parts) != len(path) {
		return false
	}
	for i, part := range parts {
		if part != "*" && part != pathKey(path[i]) {
			return false
		}
	}
	return true
}

// returns the first rule that matches the key path, for a list when items is
// set and for a mapping otherwise.
func First(set []Rule, path []string, items bool) (Rule, bool) {
	i := slices.IndexFunc(set, func(r Rule) bool { return r.Items == items && Match(r.Path, path) })
	if i < 0 {
		return Rule{}, false
	}
	return set[i], true
}

// returns keys in the order the rule requires.
func (r Rule) Sorted(keys []string) []string {
	rank := func(key string) int {
		if i := slices.Index(r.Order, key); i >= 0 {
			return i
		}
		return len(r.Order)
	}
	sorted := slices.Clone(keys)
	slices.SortStableFunc(sorted, func(a, b string) int {
		if d := rank(a) - rank(b); d != 0 || !r.SortRest || rank(a) < len(r.Order) {
			return d
		}
		return NaturalCompare(a, b)
	})
	return sorted
}

// orders strings by code point, comparing ASCII digit runs numerically.
func NaturalCompare(a, b string) int {
	x, y := chunks(a), chunks(b)
	for i := 0; i < min(len(x), len(y)); i++ {
		if x[i] == y[i] {
			continue
		}
		if isDigits(x[i]) && isDigits(y[i]) {
			m, n := strings.TrimLeft(x[i], "0"), strings.TrimLeft(y[i], "0")
			if len(m) != len(n) {
				return len(m) - len(n)
			}
			if d := strings.Compare(m, n); d != 0 {
				return d
			}
		}
		return strings.Compare(x[i], y[i])
	}
	return len(x) - len(y)
}

// splits a string into runs of digits and non-digits.
func chunks(s string) []string {
	var out []string
	for len(s) > 0 {
		digit := isDigits(s)
		i := strings.IndexFunc(s, func(r rune) bool { return (r >= '0' && r <= '9') != digit })
		if i < 0 {
			i = len(s)
		}
		out = append(out, s[:i])
		s = s[i:]
	}
	return out
}

// checks if the first byte is an ASCII digit.
func isDigits(s string) bool { return s != "" && s[0] >= '0' && s[0] <= '9' }
