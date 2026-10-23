package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFilePreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(path, []byte("original\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(path, []byte("replacement\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "replacement\n" {
		t.Fatalf("contents = %q, error = %v", data, err)
	}
	after, err := os.Stat(path)
	if err != nil || after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("permissions changed: before = %v, after = %v, error = %v", before, after, err)
	}
	assertOnlyOriginal(t, dir, filepath.Base(path))
}

func TestFailedReplacementPreservesOriginalAndCleansTemporaryFile(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o400} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "manifest.yaml")
			if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(path, 0o600) })
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("replacement failed")
			called := false
			err = replaceFileWithRename(path, []byte("replacement\n"), func(from, to string) error {
				called = true
				if filepath.Dir(from) != dir || to != path {
					t.Errorf("replacement paths = %q -> %q", from, to)
				}
				data, err := os.ReadFile(from)
				if err != nil || string(data) != "replacement\n" {
					t.Errorf("temporary contents = %q, error = %v", data, err)
				}
				return failure
			})
			if !called || !errors.Is(err, failure) {
				t.Fatalf("rename called = %v, error = %v", called, err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "original\n" {
				t.Fatalf("original contents = %q, error = %v", data, err)
			}
			after, err := os.Stat(path)
			if err != nil || after.Mode().Perm() != before.Mode().Perm() {
				t.Fatalf("permissions changed: before = %v, after = %v, error = %v", before, after, err)
			}
			assertOnlyOriginal(t, dir, filepath.Base(path))
		})
	}
}

func TestReplaceFileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target, link := filepath.Join(dir, "target.yaml"), filepath.Join(t.TempDir(), "link.yaml")
	if err := os.WriteFile(target, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := replaceFile(link, []byte("replacement\n")); err == nil {
		t.Fatal("expected refusal to replace a symlink")
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink was replaced: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "original\n" {
		t.Fatalf("target contents = %q, error = %v", data, err)
	}
	assertOnlyOriginal(t, filepath.Dir(link), filepath.Base(link))
}

func assertOnlyOriginal(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != name {
		t.Errorf("unexpected temporary files remain: %v", entries)
	}
}
