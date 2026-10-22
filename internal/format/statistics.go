package format

import "github.com/JacobSartin/homelab-fmt/internal/yamltext"

// Statistics counts accepted edits. A mapping is counted once, even if later
// passes adjust its spacing. Blank lines inside scalar values are not edits.
type Statistics struct {
	MappingsReordered int
	BlankLinesAdded   int
	BlankLinesRemoved int
}

// Add accumulates accepted edit counts in the same statistics representation.
func (s *Statistics) Add(other Statistics) {
	s.MappingsReordered += other.MappingsReordered
	s.BlankLinesAdded += other.BlankLinesAdded
	s.BlankLinesRemoved += other.BlankLinesRemoved
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
