package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// replaceFile finishes writing a same-directory temporary file before replacing
// the original. Failure before the rename leaves the original contents intact.
func replaceFile(path string, data []byte) error {
	return replaceFileWithRename(path, data, os.Rename)
}

func replaceFileWithRename(path string, data []byte, rename func(string, string) error) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("cannot replace non-regular file %s", path)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".homelab-fmt-*")
	if err != nil {
		return err
	}
	replaced := false
	defer func() {
		tmp.Close()
		if !replaced {
			// A restored read-only mode must not prevent cleanup on Windows.
			os.Chmod(tmp.Name(), 0o600)
			os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := rename(tmp.Name(), path); err != nil {
		return err
	}
	replaced = true
	return nil
}
