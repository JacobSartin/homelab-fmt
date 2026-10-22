package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSummaryInBothModes(t *testing.T) {
	for _, check := range []bool{true, false} {
		t.Run(map[bool]string{true: "check", false: "write"}[check], func(t *testing.T) {
			dir := t.TempDir()
			if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
				t.Fatalf("git init: %v: %s", err, out)
			}
			input := "kind: ConfigMap\napiVersion: v1\nmetadata:\n  name: x\n"
			for name, text := range map[string]string{"changed.yaml": input, "invalid.yaml": "a: [\n"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(dir)
			var stdout, stderr bytes.Buffer
			var args []string
			verb := "Formatted"
			if check {
				args = []string{"--check"}
				verb = "Checked"
			}
			if code := run(args, &stdout, &stderr); code != exitRuntime {
				t.Fatalf("exit = %d, stderr = %s", code, &stderr)
			}
			want := verb + " 2 files; 1 changed; 1 mappings reordered; 0 blank lines added, 0 removed; 0 warnings; 1 errors.\n"
			if !strings.HasSuffix(stdout.String(), want) {
				t.Errorf("stdout = %q, want suffix %q", stdout.String(), want)
			}
			data, err := os.ReadFile(filepath.Join(dir, "changed.yaml"))
			if err != nil || (string(data) == input) != check {
				t.Errorf("write behavior: %q, %v", data, err)
			}
		})
	}
}

func TestFailedOutcomeDoesNotCountUnwrittenEdits(t *testing.T) {
	result := processFile("missing.yaml", filepath.Join(t.TempDir(), "missing.yaml"), false)
	if got := summarize([]fileOutcome{result}); got.errors != 1 || got.changed != 0 {
		t.Errorf("summary = %+v", got)
	}
}
