package main

import (
	"os"

	"github.com/JacobSartin/homelab-fmt/internal/format"
)

// Both modes produce the same result; write mode also persists successful edits.
func processFile(file, full string, check bool, rulesFor format.RulesFor) (format.Result, error) {
	return processFileWithWriter(file, full, check, replaceFile, rulesFor)
}

func processFileWithWriter(file, full string, check bool, write func(string, []byte) error, rulesFor format.RulesFor) (format.Result, error) {
	data, err := os.ReadFile(full)
	if err != nil {
		return format.Result{}, err
	}
	result, err := format.Format(file, string(data), rulesFor, sopsOptions(full, string(data), fileStatus))
	if err == nil && result.Changed && !check {
		err = write(full, []byte(result.Output))
	}
	return result, err
}
