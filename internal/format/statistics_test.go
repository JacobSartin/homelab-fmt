package format

import (
	"testing"

	"github.com/JacobSartin/homelab-fmt/internal/rules"
)

func TestEditStatistics(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		rules       []rules.Rule
		want        Statistics
	}{
		{"reorder and separate", "b: 1\na: 2\n", []rules.Rule{{Order: []string{"a"}}}, Statistics{1, 1, 0}},
		{"compact", "a: 1\n\n\nb: 2\n", []rules.Rule{{Spacing: rules.Compact}}, Statistics{0, 0, 2}},
		{"list items", "list:\n  - a\n  - b\n", []rules.Rule{{Path: "list", Items: true, Spacing: rules.Separate}}, Statistics{0, 1, 0}},
		{"scalar blank stays", "a: |\n  one\n\n  two\nb: 1\n", []rules.Rule{{Spacing: rules.Compact}}, Statistics{}},
		{"blocked reorder", "z: &value 1\na: *value\n", []rules.Rule{{SortRest: true}}, Statistics{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Format("x.yaml", tc.input, with(tc.rules...))
			if err != nil {
				t.Fatal(err)
			}
			if result.Stats != tc.want {
				t.Errorf("stats = %+v, want %+v", result.Stats, tc.want)
			}
			again, err := Format("x.yaml", result.Output, with(tc.rules...))
			if err != nil || again.Stats != (Statistics{}) || again.Changed {
				t.Errorf("second pass = %+v, %v", again, err)
			}
		})
	}
}
