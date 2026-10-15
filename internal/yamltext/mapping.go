// Package yamltext treats YAML block mappings as ranges of whole lines, so
// entries can be moved and spaced without re-printing anything.
package yamltext

import (
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// one pair as lines: comments directly above the key, the key, and
// everything indented under it.
type Entry struct {
	Key        string
	Start, End int // inclusive line indexes
}

// The lines between entries (blank lines, detached comments) are gaps; they
// stay in place when entries move.
type Mapping struct {
	Entries []Entry
	Indent  int
	Prefix  string // text before the first key on its line, like "- " for a list item
}

var itemPrefix = regexp.MustCompile(`^[ -]*$`)

func IsBlank(line string) bool { return strings.TrimSpace(line) == "" }

func indentOf(line string) int {
	if IsBlank(line) {
		return -1
	}
	return len(line) - len(strings.TrimLeft(line, " "))
}

func isComment(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "#") }

// line ranges of a block mapping's entries; false when the layout cannot be
// rearranged line by line. top marks a document root, whose opening comment
// stays on top.
func Layout(lines []string, node *yaml.Node, top bool) (Mapping, bool) {
	if node.Kind != yaml.MappingNode || node.Style&yaml.FlowStyle != 0 || len(node.Content) < 2 {
		return Mapping{}, false
	}
	indent := node.Content[0].Column - 1
	first := lines[node.Content[0].Line-1]
	m := Mapping{Indent: indent, Prefix: first[:min(indent, len(first))]}
	if strings.TrimSpace(m.Prefix) == "" {
		m.Prefix = ""
	} else if !itemPrefix.MatchString(m.Prefix) {
		return Mapping{}, false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		line := key.Line - 1
		if key.Kind != yaml.ScalarNode || key.Column-1 != indent ||
			(i > 0 && indentOf(lines[line]) != indent) {
			return Mapping{}, false
		}
		start := line
		// The document's opening comment (such as a schema modeline) stays on
		// top, and a sequence dash keeps whatever is above it.
		if !(i == 0 && (top || m.Prefix != "")) {
			for start > 0 && isComment(lines[start-1]) && indentOf(lines[start-1]) == indent {
				start--
			}
		}
		end := line
		for j := line + 1; j < len(lines); j++ {
			ind := indentOf(lines[j])
			if ind < 0 {
				continue
			}
			// A sequence may sit at its key's indentation ("key:\n- a").
			indentless := ind == indent && value.Kind == yaml.SequenceNode &&
				value.Column-1 == indent && strings.HasPrefix(lines[j][ind:], "-") &&
				!strings.HasPrefix(lines[j], "---")
			if ind <= indent && !indentless {
				break
			}
			end = j
		}
		if len(m.Entries) > 0 && start <= m.Entries[len(m.Entries)-1].End {
			return Mapping{}, false
		}
		m.Entries = append(m.Entries, Entry{Key: key.Value, Start: start, End: end})
	}
	return m, true
}

func (m Mapping) Keys() []string {
	keys := make([]string, len(m.Entries))
	for i, e := range m.Entries {
		keys[i] = e.Key
	}
	return keys
}

// lines before each entry; the first entry has none.
func (m Mapping) Gaps(lines []string) [][]string {
	gaps := make([][]string, len(m.Entries))
	for i := 1; i < len(m.Entries); i++ {
		gaps[i] = slices.Clone(lines[m.Entries[i-1].End+1 : m.Entries[i].Start])
	}
	return gaps
}

// the mapping's lines with entries in order and gaps[i] before position i.
func (m Mapping) Render(lines []string, order []string, gaps [][]string) []string {
	var out []string
	for i, key := range order {
		e := m.Entries[slices.IndexFunc(m.Entries, func(e Entry) bool { return e.Key == key })]
		block := slices.Clone(lines[e.Start : e.End+1])
		if m.Prefix != "" && e.Start == m.Entries[0].Start {
			block[0] = strings.Repeat(" ", m.Indent) + block[0][m.Indent:]
		}
		out = append(append(out, gaps[i]...), block...)
	}
	if m.Prefix != "" {
		out[0] = m.Prefix + out[0][m.Indent:]
	}
	return out
}

// lines with the mapping's range replaced by with.
func (m Mapping) Replace(lines []string, with []string) []string {
	first, last := m.Entries[0].Start, m.Entries[len(m.Entries)-1].End
	return slices.Concat(lines[:first], with, lines[last+1:])
}

// line ranges of a block sequence's items, each with the comments directly
// above its dash; false when the layout cannot be spaced line by line.
func Items(lines []string, node *yaml.Node) (Mapping, bool) {
	if node.Kind != yaml.SequenceNode || node.Style&yaml.FlowStyle != 0 || len(node.Content) < 2 {
		return Mapping{}, false
	}
	indent := node.Column - 1
	isDash := func(line string) bool {
		return indentOf(line) == indent && strings.HasPrefix(line[indent:], "-") && !strings.HasPrefix(line, "---")
	}
	m := Mapping{Indent: indent}
	for i, item := range node.Content {
		// The dash may sit above the item's first line ("- # comment").
		line := item.Line - 1
		for line > 0 && !isDash(lines[line]) {
			line--
		}
		if !isDash(lines[line]) || (i > 0 && line <= m.Entries[i-1].End) {
			return Mapping{}, false
		}
		start := line
		if i > 0 {
			for start > m.Entries[i-1].End+1 && isComment(lines[start-1]) && indentOf(lines[start-1]) == indent {
				start--
			}
		}
		end := line
		for j := line + 1; j < len(lines); j++ {
			ind := indentOf(lines[j])
			if ind < 0 {
				continue
			}
			if ind <= indent {
				break
			}
			end = j
		}
		m.Entries = append(m.Entries, Entry{Start: start, End: end})
	}
	return m, true
}

// the mapping's lines with entries in place and gaps[i] before entry i.
func (m Mapping) Space(lines []string, gaps [][]string) []string {
	var out []string
	for i, e := range m.Entries {
		out = append(append(out, gaps[i]...), lines[e.Start:e.End+1]...)
	}
	return out
}
