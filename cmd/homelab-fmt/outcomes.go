package main

import (
	"os"

	"github.com/JacobSartin/homelab-fmt/internal/format"
	"github.com/JacobSartin/homelab-fmt/internal/templates"
)

// Both modes produce the same result; write mode also persists successful edits.
func processFile(file, full string, check bool) (format.Result, error) {
	return processFileWithWriter(file, full, check, replaceFile)
}

func processFileWithWriter(file, full string, check bool, write func(string, []byte) error) (format.Result, error) {
	data, err := os.ReadFile(full)
	if err != nil {
		return format.Result{}, err
	}
	result, err := format.Format(file, string(data), templates.For, sopsOptions(full, string(data), fileStatus))
	if err == nil && result.Changed && !check {
		err = write(full, []byte(result.Output))
	}
	return result, err
}
