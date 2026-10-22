package main

import (
	"os"

	"github.com/JacobSartin/homelab-fmt/internal/format"
	"github.com/JacobSartin/homelab-fmt/internal/templates"
)

type fileOutcome struct {
	result format.Result
	err    error
}

// Both modes produce the same result; write mode also persists successful edits.
func processFile(file, full string, check bool) fileOutcome {
	data, err := os.ReadFile(full)
	if err != nil {
		return fileOutcome{err: err}
	}
	result, err := format.Format(file, string(data), templates.For)
	if err == nil && result.Changed && !check {
		err = os.WriteFile(full, []byte(result.Output), 0o644)
	}
	return fileOutcome{result: result, err: err}
}

type runSummary struct {
	changed, warnings, errors int
	stats                     format.Statistics
}

func summarize(outcomes []fileOutcome) runSummary {
	var summary runSummary
	for _, outcome := range outcomes {
		summary.warnings += len(outcome.result.Warnings)
		if outcome.err != nil {
			summary.errors++
			continue
		}
		if outcome.result.Changed {
			summary.changed++
		}
		summary.stats.MappingsReordered += outcome.result.Stats.MappingsReordered
		summary.stats.BlankLinesAdded += outcome.result.Stats.BlankLinesAdded
		summary.stats.BlankLinesRemoved += outcome.result.Stats.BlankLinesRemoved
	}
	return summary
}
