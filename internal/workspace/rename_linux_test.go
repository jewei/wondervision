//go:build linux && (amd64 || arm64)

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExclusiveRenameRejectsAConcurrentDestination(t *testing.T) {
	path := t.TempDir()
	root, _ := os.OpenRoot(path)
	defer root.Close()
	os.WriteFile(filepath.Join(path, "source"), []byte("source"), 0600)
	os.WriteFile(filepath.Join(path, "created-after-check"), []byte("outside writer"), 0600)
	if err := renameExclusive(root, "source", root, "created-after-check"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("exclusive rename accepted collision: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(path, "created-after-check"))
	if string(raw) != "outside writer" {
		t.Fatal("concurrent destination was replaced")
	}
}
