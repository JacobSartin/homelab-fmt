// Package templates turns example documents into formatting rules.
//
// A template is laid out the way formatted documents should look:
//
//   - Keys are ordered as in the template; keys it does not list keep their
//     order after the listed ones.
//   - A "*" key stands for any key. In a list, the first item describes every item.
//   - A blank line between two keys is added after the first key when missing.
//   - "# homelab-fmt: sort | compact | separate" on a key sets how the mapping
//     under it treats unlisted keys and blank lines; compact and separate on a
//     list's key space its items.
//   - Header directives select documents ("schema <glob>", or apiVersion and
//     kind) and layer templates ("extends <template>").
package templates

import (
	"embed"
	"io/fs"
	"regexp"

	"go.yaml.in/yaml/v3"

	"github.com/JacobSartin/homelab-fmt/internal/rules"
)

type Template struct {
	Name    string
	Text    string
	Extends string
	Chain   []string     // this template, then the templates it extends
	Rules   []rules.Rule // merged with the parents' rules

	schema            *regexp.Regexp
	group, kind       string
	own               []rules.Rule
	line, extendsLine int
}

type Set struct {
	templates []*Template // sorted by name
}

func (s *Set) Templates() []*Template { return s.templates }

//go:embed bundled/*.yaml
var bundled embed.FS

// The valid templates shipped in the binary and diagnostics for skipped ones.
var Default, DefaultErrors = func() (*Set, []error) {
	sub, err := fs.Sub(bundled, "bundled")
	if err != nil {
		return &Set{}, []error{err}
	}
	return Load(sub)
}()

// rules of the bundled template that matches doc; none when nothing matches.
func For(doc *yaml.Node) []rules.Rule {
	if t := Default.Match(doc); t != nil {
		return t.Rules
	}
	return nil
}
