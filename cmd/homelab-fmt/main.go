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
)

// set by the release build.
var version = "dev"

const usage = `Usage: homelab-fmt [--check] [path...]
       homelab-fmt explain path...

Orders keys and blank lines in app-template values and Kubernetes manifests.
Without paths, formats every YAML file Git does not ignore. Paths that are not
YAML files are skipped. Layout is left to oxfmt.

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
	if len(args) > 0 && args[0] == "explain" {
		return explain(args[1:], stdout, stderr)
	}
	flags := flag.NewFlagSet("homelab-fmt", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprint(stderr, usage)
		flags.PrintDefaults()
	}
	var checkMode, showVersion bool
	flags.BoolVar(&checkMode, "check", false, "report files that need formatting instead of writing them")
	flags.BoolVar(&showVersion, "version", false, "print the version")
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

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRuntime
	}
	paths := flags.Args()
	targets := files.Select(root, paths)
	if len(paths) == 0 {
		if targets, err = files.Discover(root); err != nil {
			fmt.Fprintf(stderr, "listing files with git: %v\n", err)
			return exitRuntime
		}
	}

	var outcomes []fileOutcome
	for _, file := range targets {
		outcome := processFile(file, filepath.Join(root, file), checkMode)
		outcomes = append(outcomes, outcome)
		if outcome.err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", file, outcome.err)
		}
		for _, warning := range outcome.result.Warnings {
			fmt.Fprintf(stderr, "%s: %s\n", file, warning)
		}
		if outcome.err == nil && outcome.result.Changed {
			verb := "formatted"
			if checkMode {
				verb = "needs formatting"
			}
			fmt.Fprintf(stdout, "%s: %s\n", verb, file)
		}
	}
	summary := summarize(outcomes)
	if len(paths) == 0 {
		verb := "Formatted"
		if checkMode {
			verb = "Checked"
		}
		fmt.Fprintf(stdout, "%s %d files; %d changed; %d mappings reordered; %d blank lines added, %d removed; %d warnings; %d errors.\n",
			verb, len(outcomes), summary.changed, summary.stats.MappingsReordered,
			summary.stats.BlankLinesAdded, summary.stats.BlankLinesRemoved, summary.warnings, summary.errors)
	}
	switch {
	case summary.errors > 0:
		return exitRuntime
	case checkMode && summary.changed > 0:
		return exitUnformatted
	}
	return exitOK
}
