package main

import (
	"os"

	"github.com/JacobSartin/homelab-fmt/internal/format"
	"github.com/JacobSartin/homelab-fmt/internal/templates"
)

// Both modes produce the same result; write mode also persists successful edits.
func processFile(file, full string, check bool) (format.Result, error) {
	data, err := os.ReadFile(full)
	if err != nil {
		return format.Result{}, err
	}
	result, err := format.Format(file, string(data), templates.For)
	if err == nil && result.Changed && !check {
		err = os.WriteFile(full, []byte(result.Output), 0o644)
	}
	return result, err
}
