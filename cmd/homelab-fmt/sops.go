package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"

	"github.com/JacobSartin/homelab-fmt/internal/format"
)

func fileStatus(path string) ([]byte, error) {
	return exec.Command("sops", "filestatus", path).Output()
}

// Only heuristic-positive files need a subprocess. A failed or incomplete
// check leaves the conservative heuristics in place, without requiring sops.
func sopsOptions(path, text string, check func(string) ([]byte, error)) format.Options {
	if !format.IsSOPS(path, text) {
		return format.Options{}
	}
	// Use an absolute path so nested files and names beginning with '-' reach
	// sops correctly, without being interpreted as command flags.
	full, err := filepath.Abs(path)
	if err != nil {
		return format.Options{}
	}
	output, err := check(full)
	if err != nil {
		return format.Options{}
	}
	var status struct {
		Encrypted *bool `json:"encrypted"`
	}
	if err := json.Unmarshal(output, &status); err != nil {
		return format.Options{}
	}
	return format.Options{SOPSStatus: status.Encrypted}
}
