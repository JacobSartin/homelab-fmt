package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/JacobSartin/homelab-fmt/internal/files"
	"github.com/JacobSartin/homelab-fmt/internal/format"
)

func explain(paths []string, templatePath string, stdout, stderr io.Writer) int {
	if len(paths) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRuntime
	}
	set, templateDir, diagnostics := loadTemplates(root, templatePath)
	warnTemplates(stderr, diagnostics)
	targets, err := files.Select(root, paths)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRuntime
	}
	targets = withoutTemplates(root, templateDir, targets)
	errored := false
	for _, file := range targets {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			fmt.Fprintln(stderr, err)
			errored = true
			continue
		}
		text, err := format.Explain(file, string(data), set, sopsOptions(filepath.Join(root, file), string(data), fileStatus))
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
