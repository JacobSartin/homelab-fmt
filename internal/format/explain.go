package format

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/JacobSartin/homelab-fmt/internal/rules"
	"github.com/JacobSartin/homelab-fmt/internal/templates"
)

// lists the template each document matches and the rule for each of its
// mappings and lists.
func Explain(path, text string, set *templates.Set, options ...Options) (string, error) {
	if isSOPS(path, text, options) {
		return path + ": SOPS file, never formatted\n", nil
	}
	docs, err := parse(strings.ReplaceAll(text, "\r\n", "\n"))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for d, doc := range docs {
		fmt.Fprintf(&b, "%s, document %d: ", path, d+1)
		t := set.Match(doc)
		if t == nil || len(doc.Content) == 0 {
			b.WriteString("no template matches; left as is\n")
			continue
		}
		fmt.Fprintf(&b, "template %s\n", strings.Join(t.Chain, " extends "))
		walk(doc.Content[0], nil, func(node *yaml.Node, keys []string) bool {
			if rule, ok := rules.First(t.Rules, keys, node.Kind == yaml.SequenceNode); ok {
				fmt.Fprintf(&b, "  %s: %s\n", pathName(keys), rule.Summarize())
			}
			return false
		})
	}
	return b.String(), nil
}
