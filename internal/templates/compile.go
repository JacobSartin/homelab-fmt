package templates

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/JacobSartin/homelab-fmt/internal/rules"
	"github.com/JacobSartin/homelab-fmt/internal/yamltext"
)

var directivePrefix = regexp.MustCompile(`^#\s*homelab-fmt:\s*(.*)$`)

// adds a rule for each mapping under node. flags are the directives on the
// key holding node, and line is that key's line.
func (t *Template) compile(lines []string, node *yaml.Node, path, flags []string, line int) error {
	mode, sorted := sortOf(flags)
	switch node.Kind {
	case yaml.MappingNode:
	case yaml.SequenceNode:
		if sorted {
			return fmt.Errorf("line %d: sort applies to mappings", line)
		}
		if spacing := spacingOf(flags); spacing != rules.Keep {
			t.own = append(t.own, rules.Rule{
				Path: rules.PathString(path), Items: true, Spacing: spacing,
				Source: fmt.Sprintf("%s:%d", t.Name, line),
			})
		}
		if len(node.Content) == 0 {
			return nil
		}
		item := node.Content[0]
		return t.compile(lines, item, append(slices.Clip(path), "*"), nil, item.Line)
	default:
		if len(flags) > 0 {
			return fmt.Errorf("line %d: directives apply to mappings and lists", line)
		}
		return nil
	}

	rule := rules.Rule{
		Path: rules.PathString(path), SortRest: sorted, SortMode: mode, Spacing: spacingOf(flags),
		Source: fmt.Sprintf("%s:%d", t.Name, line),
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if key := node.Content[i].Value; !isWildcard(key) {
			rule.Order = append(rule.Order, key)
		}
	}
	if m, ok := yamltext.Layout(lines, node, len(path) == 0); ok {
		gaps := m.Gaps(lines)
		for i := 1; i < len(m.Entries); i++ {
			if prev := m.Entries[i-1].Key; !isWildcard(prev) && slices.ContainsFunc(gaps[i], yamltext.IsBlank) {
				rule.BlankAfter = append(rule.BlankAfter, prev)
			}
		}
	}
	if rule.Reorders() || rule.Spacing != rules.Keep || len(rule.BlankAfter) > 0 {
		t.own = append(t.own, rule)
	}

	// Named keys first, so their rules win over the wildcard's.
	var wildcard []int
	for i := 0; i+1 < len(node.Content); i += 2 {
		if isWildcard(node.Content[i].Value) {
			wildcard = append(wildcard, i)
			continue
		}
		if err := t.compilePair(lines, node, i, path); err != nil {
			return err
		}
	}
	for _, i := range wildcard {
		if err := t.compilePair(lines, node, i, path); err != nil {
			return err
		}
	}
	return nil
}

func (t *Template) compilePair(lines []string, node *yaml.Node, i int, path []string) error {
	key, value := node.Content[i], node.Content[i+1]
	comments := []string{key.LineComment, value.LineComment, value.HeadComment}
	commentLines := []int{key.Line, value.Line, value.Line}
	// The first key's head comment is the template header.
	if !(len(path) == 0 && i == 0) {
		comments = append(comments, key.HeadComment)
		commentLines = append(commentLines, key.Line)
	}
	var flags []string
	source := key.Line
	for i, comment := range comments {
		for _, d := range directives(comment) {
			line := directiveLine(lines, d, commentLines[i])
			if len(flags) == 0 {
				source = line
			}
			switch d {
			case "sort", "sort alphabetical", "sort size-asc", "sort size-desc", "compact", "separate":
				if strings.HasPrefix(d, "sort") {
					before, hadSort := sortOf(flags)
					after, _ := sortOf([]string{d})
					if hadSort && before != after {
						return fmt.Errorf("line %d: conflicting sort directives", line)
					}
				}
				flags = append(flags, d)
			default:
				return fmt.Errorf("line %d: unknown directive %q", line, d)
			}
		}
	}
	segment := key.Value
	if isWildcard(segment) {
		segment = "*"
	}
	return t.compile(lines, value, append(slices.Clip(path), segment), flags, source)
}

// Find the actual comment line, including head comments separated by blanks.
func directiveLine(lines []string, directive string, before int) int {
	for i := min(before, len(lines)); i > 0; i-- {
		for at, char := range lines[i-1] {
			if char == '#' && slices.Contains(directives(lines[i-1][at:]), directive) {
				return i
			}
		}
	}
	return max(1, before)
}

func sortOf(flags []string) (rules.SortMode, bool) {
	for _, flag := range flags {
		switch flag {
		case "sort", "sort size-asc":
			return rules.SizeAscending, true
		case "sort size-desc":
			return rules.SizeDescending, true
		case "sort alphabetical":
			return rules.Alphabetical, true
		}
	}
	return rules.Alphabetical, false
}

func spacingOf(flags []string) rules.Spacing {
	switch {
	case slices.Contains(flags, "compact"):
		return rules.Compact
	case slices.Contains(flags, "separate"):
		return rules.Separate
	}
	return rules.Keep
}

// "*" and labelled variants such as "*nfs" stand for any key. Variants let one
// mapping hold several examples that must stay valid against a schema.
func isWildcard(key string) bool { return strings.HasPrefix(key, "*") }

// wildcard variants compile to the same path; join their rules into one.
func joinVariants(own []rules.Rule) []rules.Rule {
	var joined []rules.Rule
	for _, r := range own {
		i := slices.IndexFunc(joined, func(j rules.Rule) bool { return j.Path == r.Path && j.Items == r.Items })
		if i < 0 {
			joined = append(joined, r)
			continue
		}
		j := &joined[i]
		j.Order = interleave(j.Order, r.Order)
		if !j.SortRest {
			j.SortMode = r.SortMode
		}
		j.SortRest = j.SortRest || r.SortRest
		if j.Spacing == rules.Keep {
			j.Spacing = r.Spacing
		}
		for _, key := range r.BlankAfter {
			if !slices.Contains(j.BlankAfter, key) {
				j.BlankAfter = append(j.BlankAfter, key)
			}
		}
		j.Source += ", " + r.Source
	}
	return joined
}

// adds b's new keys to a, each right before the next key of b that a already has.
func interleave(a, b []string) []string {
	out := slices.Clone(a)
	for i, key := range b {
		if slices.Contains(out, key) {
			continue
		}
		at := len(out)
		for _, next := range b[i+1:] {
			if j := slices.Index(out, next); j >= 0 {
				at = j
				break
			}
		}
		out = slices.Insert(out, at, key)
	}
	return out
}

// homelab-fmt directives in the comments, one entry per comma-separated item.
func directives(comments ...string) []string {
	var found []string
	for _, comment := range comments {
		for _, line := range strings.Split(comment, "\n") {
			m := directivePrefix.FindStringSubmatch(strings.TrimSpace(line))
			if m == nil {
				continue
			}
			for _, d := range strings.Split(m[1], ",") {
				if d = strings.Join(strings.Fields(d), " "); d != "" {
					found = append(found, d)
				}
			}
		}
	}
	return found
}
