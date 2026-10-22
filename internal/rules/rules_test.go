package rules

import (
	"cmp"
	"slices"
	"testing"
)

func TestMatch(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		path    []string
		want    bool
	}{
		{"", nil, true},
		{"", []string{"a"}, false},
		{"a.*.c", []string{"a", "b", "c"}, true},
		{"a.*.c", []string{"a", "c"}, false},
	} {
		if got := Match(tc.pattern, tc.path); got != tc.want {
			t.Errorf("Match(%q, %q) = %v", tc.pattern, tc.path, got)
		}
	}
}

func TestNaturalCompare(t *testing.T) {
	got := []string{"a10", "a2", "B", "a1"}
	slices.SortFunc(got, NaturalCompare)
	if want := []string{"B", "a1", "a2", "a10"}; !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestNaturalCompareUnicodeAndLargeNumbers(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"١", "2", 1},
		{"a١", "a2", 1},
		{"١2", "١10", -1},
		{"a１２", "a١", 1},
		{"a100000000000000000000", "a99999999999999999999", 1},
		{"a18446744073709551616", "a18446744073709551617", -1},
		{"a002", "a10", -1},
		{"a001", "a1", -1},
		{"a000", "a0", 1},
	} {
		t.Run(tc.a+"/"+tc.b, func(t *testing.T) {
			if got := cmp.Compare(NaturalCompare(tc.a, tc.b), 0); got != tc.want {
				t.Errorf("NaturalCompare(%q, %q) = %d; want sign %d", tc.a, tc.b, got, tc.want)
			}
			if got := cmp.Compare(NaturalCompare(tc.b, tc.a), 0); got != -tc.want {
				t.Errorf("reverse comparison = %d; want sign %d", got, -tc.want)
			}
		})
	}
}

func TestSorted(t *testing.T) {
	keys := []string{"z", "b", "a", "c"}
	kept := Rule{Order: []string{"c"}}.Sorted(keys)
	if want := []string{"c", "z", "b", "a"}; !slices.Equal(kept, want) {
		t.Errorf("kept: got %q", kept)
	}
	sorted := Rule{Order: []string{"c"}, SortRest: true}.Sorted(keys)
	if want := []string{"c", "a", "b", "z"}; !slices.Equal(sorted, want) {
		t.Errorf("sorted: got %q", sorted)
	}
}
