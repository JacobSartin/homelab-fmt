package templates

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

// reads one template's header directives and compiles its rules; extends
// chains are resolved later, once every template is parsed.
func parse(name, source, text string) (*Template, error) {
	dec := yaml.NewDecoder(strings.NewReader(text))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		return nil, fmt.Errorf("line %d: a template holds one document", extra.Line)
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: a template is a mapping", doc.Line)
	}
	// Decoding the node validates duplicate keys and invalid aliases as well.
	if err := doc.Decode(new(any)); err != nil {
		return nil, err
	}
	root := doc.Content[0]
	lines := strings.Split(text, "\n")
	t := &Template{Name: name, Source: source, Text: text, line: root.Line}
	for i, line := range lines[:root.Line-1] {
		for _, d := range directives(line) {
			verb, arg, _ := strings.Cut(d, " ")
			switch verb {
			case "extends", "schema":
				if arg == "" {
					return nil, fmt.Errorf("line %d: %s requires an argument", i+1, verb)
				}
				if verb == "extends" {
					t.Extends, t.extendsLine = arg, i+1
				} else {
					t.schema = glob(arg)
				}
			default:
				return nil, fmt.Errorf("line %d: unknown directive %q", i+1, d)
			}
		}
	}
	if t.schema == nil {
		apiVersion, kind := selector(&doc)
		if apiVersion == "" || kind == "" {
			return nil, fmt.Errorf(`line %d: select documents with "schema" or with apiVersion and kind`, root.Line)
		}
		t.group, t.kind = group(apiVersion), kind
	}
	if err := t.compile(lines, root, nil, nil, root.Line); err != nil {
		return nil, err
	}
	t.own = joinVariants(t.own)
	return t, nil
}
