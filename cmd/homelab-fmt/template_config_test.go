package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateDirectoryPrecedence(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, dir := range []string{filepath.Join(".homelab-fmt", "templates"), "env-templates", "flag-templates"} {
		writeConfigFile(t, filepath.Join(dir, "kubernetes.yaml"), "kind: \"*\"\napiVersion: \"*\"\n")
	}
	writeConfigFile(t, "workload.yaml", "apiVersion: v1\nkind: ConfigMap\n")
	for _, tc := range []struct {
		name, env, want string
		args            []string
	}{
		{"default outside Git", "", ".homelab-fmt/templates", []string{"explain", "workload.yaml"}},
		{"environment overrides default", "env-templates", "env-templates", []string{"explain", "workload.yaml"}},
		{"flag overrides environment", "env-templates", "flag-templates", []string{"explain", "--templates", "flag-templates", "workload.yaml"}},
		{"flag before subcommand", "env-templates", "flag-templates", []string{"--templates", "flag-templates", "explain", "workload.yaml"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOMELAB_FMT_TEMPLATES", tc.env)
			var stdout, stderr bytes.Buffer
			if got := run(tc.args, &stdout, &stderr); got != exitOK || stderr.Len() != 0 {
				t.Fatalf("exit = %d, stderr = %s", got, &stderr)
			}
			want := filepath.ToSlash(filepath.Join(root, tc.want, "kubernetes.yaml"))
			if !strings.Contains(stdout.String(), "template "+want) || !strings.Contains(stdout.String(), "order kind, apiVersion") {
				t.Errorf("explanation = %s, want source %s", &stdout, want)
			}
		})
	}
}

func TestDefaultTemplatesAtGitRootAndConfigurationSkipped(t *testing.T) {
	root := t.TempDir()
	if output, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	t.Setenv("HOMELAB_FMT_TEMPLATES", "")
	config := filepath.Join(root, ".homelab-fmt", "templates")
	writeConfigFile(t, filepath.Join(config, "kubernetes.yaml"), "kind: \"*\"\napiVersion: \"*\"\n")
	writeConfigFile(t, filepath.Join(config, "broken.yaml"), "invalid: [\n")
	writeConfigFile(t, filepath.Join(root, "apps", "workload.yaml"), "apiVersion: v1\nkind: ConfigMap\n")
	t.Chdir(filepath.Join(root, "apps"))
	var stdout, stderr bytes.Buffer
	if got := run([]string{"explain", "workload.yaml"}, &stdout, &stderr); got != exitOK {
		t.Fatalf("exit = %d, stderr = %s", got, &stderr)
	}
	if !strings.Contains(stdout.String(), filepath.ToSlash(filepath.Join(config, "kubernetes.yaml"))) || strings.Count(stderr.String(), "warning:") != 1 {
		t.Fatalf("stdout = %s, stderr = %s", &stdout, &stderr)
	}
	t.Chdir(root)
	stdout.Reset()
	stderr.Reset()
	if got := run([]string{"--check"}, &stdout, &stderr); got != exitUnformatted {
		t.Fatalf("exit = %d, stderr = %s", got, &stderr)
	}
	if !strings.Contains(stdout.String(), "needs formatting: apps/workload.yaml") || strings.Contains(stdout.String(), ".homelab-fmt/templates/") {
		t.Errorf("configuration was included as formatting input: %s", &stdout)
	}
	if !strings.Contains(stdout.String(), "; 1 warnings; 0 errors.") {
		t.Errorf("summary did not count the template warning: %s", &stdout)
	}
	if got := run([]string{"apps/workload.yaml"}, &stdout, &stderr); got != exitOK {
		t.Fatalf("write exit = %d, stderr = %s", got, &stderr)
	}
	data, err := os.ReadFile(filepath.Join(root, "apps", "workload.yaml"))
	if err != nil || string(data) != "kind: ConfigMap\napiVersion: v1\n" {
		t.Errorf("formatted data = %q, error = %v", data, err)
	}
}

func TestMissingTemplateDirectoryWarnsOnlyWhenExplicit(t *testing.T) {
	t.Chdir(t.TempDir())
	writeConfigFile(t, "workload.yaml", "apiVersion: v1\nkind: ConfigMap\n")
	for _, tc := range []struct {
		env      string
		args     []string
		warnings int
	}{
		{"", []string{"explain", "workload.yaml"}, 0},
		{"missing", []string{"explain", "workload.yaml"}, 1},
		{"", []string{"explain", "--templates", "missing", "workload.yaml"}, 1},
	} {
		t.Setenv("HOMELAB_FMT_TEMPLATES", tc.env)
		var stdout, stderr bytes.Buffer
		if got := run(tc.args, &stdout, &stderr); got != exitOK || strings.Count(stderr.String(), "warning:") != tc.warnings {
			t.Errorf("%v: exit = %d, stderr = %s", tc.args, got, &stderr)
		}
		if !strings.Contains(stdout.String(), "template bundled/kubernetes.yaml") {
			t.Errorf("missing bundled fallback: %s", &stdout)
		}
	}
}
