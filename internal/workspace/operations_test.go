package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCreateMoveDeleteWithinWorkspace(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("exclusive rename adapter is currently Linux only")
	}
	root := t.TempDir()
	w, _ := Open(context.Background(), root)
	if _, err := w.Create(context.Background(), "notes", true); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Create(context.Background(), "notes/todo.md", false); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Create(context.Background(), "notes/todo.md", false); !errors.Is(err, os.ErrExist) {
		t.Fatal("create accepted a collision")
	}
	os.WriteFile(filepath.Join(root, "notes", "todo.md"), []byte("draft"), 0600)
	if _, err := w.Move(context.Background(), "notes/todo.md", "todo.md"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "todo.md"))
	if string(raw) != "draft" {
		t.Fatal("move changed content")
	}
	if _, err := w.Move(context.Background(), "notes", "archive"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Delete(context.Background(), "archive"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Delete(context.Background(), "todo.md"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("delete left entries")
	}
}

func TestFileActionsRejectEscapeCollisionAndSelfMove(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	w, _ := Open(context.Background(), root)
	os.Mkdir(filepath.Join(root, "dir"), 0700)
	os.WriteFile(filepath.Join(root, "source"), []byte("source"), 0600)
	os.WriteFile(filepath.Join(root, "target"), []byte("target"), 0600)
	for _, path := range []string{"../outside", "dir/../outside", "/absolute", ".", "dir/../../outside", "dir\\..\\outside"} {
		if _, err := w.Create(context.Background(), path, false); err == nil {
			t.Errorf("create accepted traversal: %s", path)
		}
		if _, err := w.Delete(context.Background(), path); err == nil {
			t.Errorf("delete accepted traversal: %s", path)
		}
	}
	if _, err := w.Move(context.Background(), "source", "target"); err == nil {
		t.Fatal("move overwrote a collision")
	}
	if _, err := w.Move(context.Background(), "dir", "dir/child"); err == nil {
		t.Fatal("folder moved into itself")
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	if _, err := w.Create(context.Background(), "escape/file", false); err == nil {
		t.Fatal("create followed a symlink outside")
	}
	if _, err := w.Delete(context.Background(), "escape"); err == nil {
		t.Fatal("destructive action accepted a symlink")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "target"))
	if string(raw) != "target" {
		t.Fatal("collision target changed")
	}
}

func TestDeletionDoesNotFollowNestedSymlinksAndCancels(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	w, _ := Open(context.Background(), root)
	os.Mkdir(filepath.Join(root, "folder"), 0700)
	os.WriteFile(filepath.Join(outside, "keep"), []byte("outside"), 0600)
	if err := os.Symlink(outside, filepath.Join(root, "folder", "link")); err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := w.Delete(ctx, "folder"); err == nil {
		t.Fatal("cancelled deletion ran")
	}
	if _, err := w.Delete(context.Background(), "folder"); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join(outside, "keep")); err != nil || string(raw) != "outside" {
		t.Fatal("deletion followed an outside symlink")
	}
}
