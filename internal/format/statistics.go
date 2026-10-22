package format

import "github.com/JacobSartin/homelab-fmt/internal/yamltext"

// Statistics counts accepted edits. A mapping is counted once, even if later
// passes adjust its spacing. Blank lines inside scalar values are not edits.
type Statistics struct {
	MappingsReordered int
	BlankLinesAdded   int
	BlankLinesRemoved int
}

func spacingStats(before, after [][]string) Statistics {
	var stats Statistics
	for i := range before {
		count := func(lines []string) int {
			n := 0
			for _, line := range lines {
				if yamltext.IsBlank(line) {
					n++
				}
			}
			return n
		}
		delta := count(after[i]) - count(before[i])
		stats.BlankLinesAdded += max(delta, 0)
		stats.BlankLinesRemoved += max(-delta, 0)
	}
	return stats
}
