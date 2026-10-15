// Package format applies key-order and blank-line rules to YAML text.
//
// Edits move whole lines: comments travel with the key below them, blank
// lines stay where they were, and scalars, quoting and indentation are never
// re-printed. Layout is left to oxfmt.
package format

import (
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/JacobSartin/homelab-fmt/internal/rules"
	"github.com/JacobSartin/homelab-fmt/internal/yamltext"
)

type Result struct {
	Output   string
	Changed  bool
	Warnings []string
}

// picks the rules for one document.
type RulesFor func(doc *yaml.Node) []rules.Rule

var sopsKey = regexp.MustCompile(`(?m)^sops:\s*$`)

// SOPS files are never rewritten: their MAC covers the document structure.
func IsSOPS(path, text string) bool {
	return strings.Contains(filepath.Base(path), ".sops.") || sopsKey.MatchString(text)
}

// applies rulesFor's rules to every document. Fails rather than return output
// that parses to different data.
func Format(path, text string, rulesFor RulesFor) (Result, error) {
	if IsSOPS(path, text) {
		return Result{Output: text}, nil
	}
	crlf := strings.Contains(text, "\r\n")
	src := strings.ReplaceAll(text, "\r\n", "\n")
	before, err := decode(src)
	if err != nil {
		return Result{}, err
	}
	result := Result{}
	blocked := map[string]bool{}
	// Each pass fixes the first mapping that breaks a rule and re-parses, so
	// line numbers are always current.
	for pass := 0; ; pass++ {
		if pass > 10000 {
			return Result{}, errors.New("formatting did not settle")
		}
		docs, err := parse(src)
		if err != nil {
			return Result{}, err
		}
		next, ok := nextEdit(src, docs, rulesFor, blocked)
		if !ok {
			break
		}
		if _, err := decode(next.text); err != nil {
			if !next.moves {
				return Result{}, err
			}
			// Reordering moved an alias above its anchor.
			blocked[next.mapping] = true
			result.Warnings = append(result.Warnings,
				next.path+": left unsorted because an alias would come before its anchor")
			continue
		}
		src = next.text
		result.Changed = true
	}
	after, err := decode(src)
	if err != nil || !reflect.DeepEqual(before, after) {
		return Result{}, errors.New("formatting would change the parsed data")
	}
	if crlf {
		src = strings.ReplaceAll(src, "\n", "\r\n")
	}
	result.Output = src
	return result, nil
}

func parse(text string) ([]*yaml.Node, error) {
	var docs []*yaml.Node
	dec := yaml.NewDecoder(strings.NewReader(text))
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); errors.Is(err, io.EOF) {
			return docs, nil
		} else if err != nil {
			return nil, err
		}
		docs = append(docs, &doc)
	}
}

func decode(text string) ([]any, error) {
	var values []any
	dec := yaml.NewDecoder(strings.NewReader(text))
	for {
		var value any
		if err := dec.Decode(&value); errors.Is(err, io.EOF) {
			return values, nil
		} else if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
}

// calls fn for every mapping and list under node, parents first; stops when
// fn returns true.
func walk(node *yaml.Node, path []string, fn func(node *yaml.Node, path []string) bool) bool {
	switch node.Kind {
	case yaml.MappingNode:
		if fn(node, path) {
			return true
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if walk(node.Content[i+1], append(slices.Clip(path), node.Content[i].Value), fn) {
				return true
			}
		}
	case yaml.SequenceNode:
		if fn(node, path) {
			return true
		}
		for i, item := range node.Content {
			if walk(item, append(slices.Clip(path), strconv.Itoa(i)), fn) {
				return true
			}
		}
	}
	return false
}

func pathName(path []string) string {
	if len(path) == 0 {
		return "(root)"
	}
	return strings.Join(path, ".")
}

type edit struct {
	text    string
	path    string
	mapping string // document index and key path
	moves   bool
}

func nextEdit(text string, docs []*yaml.Node, rulesFor RulesFor, blocked map[string]bool) (edit, bool) {
	lines := strings.Split(text, "\n")
	for d, doc := range docs {
		if len(doc.Content) == 0 {
			continue
		}
		set := rulesFor(doc)
		var found edit
		ok := walk(doc.Content[0], nil, func(node *yaml.Node, path []string) bool {
			items := node.Kind == yaml.SequenceNode
			rule, ok := rules.First(set, path, items)
			if !ok {
				return false
			}
			if items {
				found, ok = fixItems(lines, node, path, rule)
				return ok
			}
			id := strconv.Itoa(d) + ":" + strings.Join(path, ".")
			found, ok = fix(lines, node, path, rule, node == doc.Content[0], blocked[id])
			found.mapping = id
			return ok
		})
		if ok {
			return found, true
		}
	}
	return edit{}, false
}

// the edit that makes one mapping follow its rule, if it needs one.
func fix(lines []string, node *yaml.Node, path []string, rule rules.Rule, top, blocked bool) (edit, bool) {
	m, ok := yamltext.Layout(lines, node, top)
	if !ok {
		return edit{}, false
	}
	keys := m.Keys()
	gaps := m.Gaps(lines)
	if rule.Reorders() && !blocked {
		if sorted := rule.Sorted(keys); !slices.Equal(sorted, keys) {
			text := strings.Join(m.Replace(lines, m.Render(lines, sorted, gaps)), "\n")
			return edit{text: text, path: pathName(path), moves: true}, true
		}
	}
	spaced := slices.Clone(gaps)
	listed := func(key string) bool { return slices.Contains(rule.Order, key) }
	// Keys the rule does not list follow the listed ones after a blank line.
	boundary := slices.IndexFunc(keys, func(key string) bool { return !listed(key) })
	if boundary <= 0 || slices.ContainsFunc(keys[boundary:], listed) {
		boundary = -1
	}
	for i := 1; i < len(keys); i++ {
		blank := slices.ContainsFunc(spaced[i], yamltext.IsBlank)
		// Blank lines drawn in the template win over compact, and compact
		// wins over the separator before unlisted keys.
		switch {
		case slices.Contains(rule.BlankAfter, keys[i-1]) || rule.Spacing == rules.Separate:
			if !blank {
				spaced[i] = append([]string{""}, spaced[i]...)
			}
		case rule.Spacing == rules.Compact:
			spaced[i] = slices.DeleteFunc(slices.Clone(spaced[i]), yamltext.IsBlank)
		case i == boundary && !blank:
			spaced[i] = append([]string{""}, spaced[i]...)
		}
	}
	if slices.EqualFunc(gaps, spaced, slices.Equal) {
		return edit{}, false
	}
	text := strings.Join(m.Replace(lines, m.Render(lines, keys, spaced)), "\n")
	return edit{text: text, path: pathName(path)}, true
}

// the edit that spaces a list's items by its rule, if it needs one.
func fixItems(lines []string, node *yaml.Node, path []string, rule rules.Rule) (edit, bool) {
	m, ok := yamltext.Items(lines, node)
	if !ok {
		return edit{}, false
	}
	gaps := m.Gaps(lines)
	spaced := slices.Clone(gaps)
	for i := 1; i < len(spaced); i++ {
		switch blank := slices.ContainsFunc(spaced[i], yamltext.IsBlank); {
		case rule.Spacing == rules.Separate && !blank:
			spaced[i] = append([]string{""}, spaced[i]...)
		case rule.Spacing == rules.Compact:
			spaced[i] = slices.DeleteFunc(slices.Clone(spaced[i]), yamltext.IsBlank)
		}
	}
	if slices.EqualFunc(gaps, spaced, slices.Equal) {
		return edit{}, false
	}
	text := strings.Join(m.Replace(lines, m.Space(lines, spaced)), "\n")
	return edit{text: text, path: pathName(path)}, true
}
