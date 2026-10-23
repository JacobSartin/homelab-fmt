package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/JacobSartin/homelab-fmt/internal/files"
	"github.com/JacobSartin/homelab-fmt/internal/format"
	"github.com/JacobSartin/homelab-fmt/internal/templates"
)

func explain(paths []string, stdout, stderr io.Writer) int {
	if len(paths) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	warnTemplates(stderr)
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRuntime
	}
	targets, err := files.Select(root, paths)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRuntime
	}
	errored := false
	for _, file := range targets {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			fmt.Fprintln(stderr, err)
			errored = true
			continue
		}
		text, err := format.Explain(file, string(data), templates.Default, sopsOptions(filepath.Join(root, file), string(data), fileStatus))
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", file, err)
			errored = true
			continue
		}
		fmt.Fprint(stdout, text)
	}
	if errored {
		return exitRuntime
	}
	return exitOK
}
