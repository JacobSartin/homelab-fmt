package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JacobSartin/homelab-fmt/internal/templates"
)

func TestExitCodes(t *testing.T) {
	t.Setenv("HOMELAB_FMT_TEMPLATES", "")
	dir := t.TempDir()
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ordered.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n")
	write("unordered.yaml", "kind: ConfigMap\napiVersion: v1\nmetadata:\n  name: x\n")
	write("invalid.yaml", "a: [\n")
	t.Chdir(dir)

	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{"formatted file", []string{"--check", "ordered.yaml"}, exitOK},
		{"file needs formatting", []string{"--check", "unordered.yaml"}, exitUnformatted},
		{"runtime error wins over needs formatting", []string{"--check", "unordered.yaml", "invalid.yaml"}, exitRuntime},
		{"unknown flag", []string{"--nope"}, exitUsage},
		{"explain without paths", []string{"explain"}, exitUsage},
		{"explain invalid YAML", []string{"explain", "invalid.yaml"}, exitRuntime},
		{"missing path", []string{"missing.yaml"}, exitRuntime},
		{"explain missing path", []string{"explain", "missing.yaml"}, exitRuntime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(tc.args, io.Discard, io.Discard); got != tc.want {
				t.Errorf("exit %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDirectoryArguments(t *testing.T) {
	t.Setenv("HOMELAB_FMT_TEMPLATES", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("kind: ConfigMap\napiVersion: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	var output bytes.Buffer
	if got := run([]string{"--check", "."}, &output, io.Discard); got != exitUnformatted {
		t.Fatalf("directory check exit %d, want %d", got, exitUnformatted)
	}
	output.Reset()
	if got := run([]string{"explain", "."}, &output, io.Discard); got != exitOK || !bytes.Contains(output.Bytes(), []byte("a.yaml")) {
		t.Errorf("directory explain exit %d, output %q", got, output.String())
	}
}

func TestTemplateWarningsContinueBothCommands(t *testing.T) {
	t.Setenv("HOMELAB_FMT_TEMPLATES", "")
	t.Chdir(t.TempDir())
	if err := os.WriteFile("valid.yaml", []byte("apiVersion: v1\nkind: ConfigMap\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	saved := templates.DefaultErrors
	t.Cleanup(func() { templates.DefaultErrors = saved })
	templates.DefaultErrors = []error{errors.New("template broken.yaml:3: invalid directive; skipped; matching documents fall back")}
	for _, args := range [][]string{{"--check", "valid.yaml"}, {"explain", "valid.yaml"}} {
		var stderr bytes.Buffer
		if got := run(args, io.Discard, &stderr); got != exitOK {
			t.Errorf("%v: exit = %d", args, got)
		}
		if got := stderr.String(); !strings.HasPrefix(got, "warning: template broken.yaml:3:") || !strings.Contains(got, "fall back") {
			t.Errorf("%v: warnings = %q", args, got)
		}
	}
}
