package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JacobSartin/homelab-fmt/internal/format"
	"github.com/JacobSartin/homelab-fmt/internal/templates"
)

func TestSOPSStatus(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		err          error
		encrypted    bool
	}{
		{name: "encrypted", output: `{"encrypted":true}`, encrypted: true},
		{name: "plaintext", output: `{"encrypted":false}`},
		{name: "missing command", err: &exec.Error{Name: "sops", Err: exec.ErrNotFound}, encrypted: true},
		{name: "failed check", output: `{"encrypted":false}`, err: errors.New("exit status 1"), encrypted: true},
		{name: "malformed JSON", output: "invalid", encrypted: true},
		{name: "missing encrypted field", output: `{}`, encrypted: true},
		{name: "null encrypted field", output: `{"encrypted":null}`, encrypted: true},
		{name: "wrong encrypted type", output: `{"encrypted":"false"}`, encrypted: true},
		{name: "trailing garbage", output: `{"encrypted":false} x`, encrypted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{".sops.yaml", filepath.Join("nested dir", "-secret.yaml")} {
				input := "kind: ConfigMap\napiVersion: v1\n"
				if filepath.Base(path) != ".sops.yaml" {
					input += "sops:\n  mac: placeholder\n"
				}
				full, err := filepath.Abs(path)
				if err != nil {
					t.Fatal(err)
				}
				called := false
				options := sopsOptions(path, input, func(got string) ([]byte, error) {
					called = true
					if got != full {
						t.Errorf("check path = %q, want absolute path %q", got, full)
					}
					return []byte(tc.output), tc.err
				})
				if !called {
					t.Fatal("expected status check")
				}
				result, err := format.Format(path, input, templates.For, options)
				if err != nil {
					t.Fatal(err)
				}
				if result.Changed == tc.encrypted {
					t.Errorf("%s: Changed = %v, encrypted = %v", path, result.Changed, tc.encrypted)
				}
				if tc.encrypted && result.Output != input {
					t.Errorf("%s: encrypted content changed", path)
				}
				explanation, err := format.Explain(path, input, templates.Default, options)
				if err != nil {
					t.Fatal(err)
				}
				if got := strings.Contains(explanation, "SOPS file"); got != tc.encrypted {
					t.Errorf("%s: encrypted = %v, explanation = %q", path, tc.encrypted, explanation)
				}
				if !tc.encrypted && !strings.Contains(explanation, "template kubernetes.yaml") {
					t.Errorf("%s: missing template explanation: %s", path, explanation)
				}
			}
		})
	}
}

func TestOrdinaryYAMLDoesNotRunSOPS(t *testing.T) {
	options := sopsOptions("config.yaml", "kind: ConfigMap\n", func(string) ([]byte, error) {
		t.Fatal("ordinary YAML should not need sops")
		return nil, nil
	})
	if options.SOPSStatus != nil {
		t.Error("ordinary YAML should retain the default detection")
	}
}
