//go:build !windows

package preview

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestFindReadmeRejectsOutsideAndSpecialFiles(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(secret, filepath.Join(outside, "README.md")); err != nil {
		t.Fatal(err)
	}
	if got := findReadme(outside); got != "" {
		t.Errorf("symlink leaving the project accepted: %s", got)
	}

	fifo := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(fifo, "README.md"), 0o644); err != nil {
		t.Skip("mkfifo unavailable:", err)
	}
	if got := findReadme(fifo); got != "" {
		t.Errorf("FIFO accepted as README: %s", got)
	}

	inside := t.TempDir()
	if err := os.WriteFile(filepath.Join(inside, "docs.md"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("docs.md", filepath.Join(inside, "README.md")); err != nil {
		t.Fatal(err)
	}
	if got := findReadme(inside); got == "" {
		t.Error("symlink inside the project rejected")
	}
}
