// Command homelab-fmt applies key-order and blank-line rules to the YAML in
// the homelab GitOps repositories.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/JacobSartin/homelab-fmt/internal/files"
	"github.com/JacobSartin/homelab-fmt/internal/format"
)

// set by the release build.
var version = "dev"

const usage = `Usage: homelab-fmt [--check] [--templates DIR] [path...]
       homelab-fmt explain [--templates DIR] path...

Orders keys and blank lines in app-template values and Kubernetes manifests.
Without paths, formats every YAML file Git does not ignore. Paths that are not
YAML files are skipped; directories are searched recursively. Outside Git,
pass an explicit path such as ".". Layout is left to oxfmt.

explain prints the template each document matches and the rule for each mapping.

Exit status: 0 done, 1 --check found files that need formatting, 2 usage
error, 3 runtime error (listing, reading or writing files, invalid YAML, or
an edit that would change the data). A runtime error wins over 1.

`

// Each code names one outcome, so scripts can tell them apart.
const (
	exitOK          = 0
	exitUnformatted = 1 // --check found files that need formatting
	exitUsage       = 2 // bad flags or arguments
	exitRuntime     = 3 // a file could not be listed, read, formatted or written
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	explainMode := len(args) > 0 && args[0] == "explain"
	if explainMode {
		args = args[1:]
	}
	flags := flag.NewFlagSet("homelab-fmt", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprint(stderr, usage)
		flags.PrintDefaults()
	}
	var checkMode, showVersion bool
	var templatePath string
	flags.BoolVar(&checkMode, "check", false, "report files that need formatting instead of writing them")
	flags.BoolVar(&showVersion, "version", false, "print the version")
	flags.StringVar(&templatePath, "templates", "", "template directory (overrides HOMELAB_FMT_TEMPLATES and repository defaults)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	if showVersion {
		fmt.Fprintln(stdout, version)
		return exitOK
	}
	paths := flags.Args()
	if !explainMode && len(paths) > 0 && paths[0] == "explain" {
		explainMode, paths = true, paths[1:]
	}
	if explainMode {
		return explain(paths, templatePath, stdout, stderr)
	}

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRuntime
	}
	set, templateDir, diagnostics := loadTemplates(root, templatePath)
	warnTemplates(stderr, diagnostics)
	var targets []string
	if len(paths) == 0 {
		targets, err = files.Discover(root)
	} else {
		targets, err = files.Select(root, paths)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRuntime
	}

	targets = withoutTemplates(root, templateDir, targets)

	changed, warnings, failed := 0, len(diagnostics), 0
	var stats format.Statistics
	for _, file := range targets {
		result, err := processFile(file, filepath.Join(root, file), checkMode, set.For)
		if err != nil {
			failed++
			fmt.Fprintf(stderr, "%s: %v\n", file, err)
		}
		warnings += len(result.Warnings)
		for _, warning := range result.Warnings {
			fmt.Fprintf(stderr, "%s: %s\n", file, warning)
		}
		if err != nil || !result.Changed {
			continue
		}
		changed++
		stats.Add(result.Stats)
		verb := "formatted"
		if checkMode {
			verb = "needs formatting"
		}
		fmt.Fprintf(stdout, "%s: %s\n", verb, file)
	}
	if len(paths) == 0 {
		verb := "Formatted"
		if checkMode {
			verb = "Checked"
		}
		fmt.Fprintf(stdout, "%s %d files; %d changed; %d mappings reordered; %d blank lines added, %d removed; %d warnings; %d errors.\n",
			verb, len(targets), changed, stats.MappingsReordered,
			stats.BlankLinesAdded, stats.BlankLinesRemoved, warnings, failed)
	}
	switch {
	case failed > 0:
		return exitRuntime
	case checkMode && changed > 0:
		return exitUnformatted
	}
	return exitOK
}

func warnTemplates(stderr io.Writer, diagnostics []error) {
	for _, err := range diagnostics {
		fmt.Fprintf(stderr, "warning: %v\n", err)
	}
}
