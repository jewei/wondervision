package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileActionsRetainDirtyBufferAndUpdatePaths(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("exclusive rename adapter is Linux and macOS only")
	}
	root := fixture(t)
	a := newApp()
	defer a.cancel()
	a.openFolder(root)
	drain(t, a)
	a.openFile(filepath.Join(root, "todo.md"), root, "")
	drain(t, a)
	d := a.active()
	d.Buffer.Set("unsaved draft")
	buffer := d.Buffer
	a.requestFileAction(renameFile)
	a.fileName = "renamed.md"
	a.submitFilePrompt()
	drain(t, a)
	if d.Path != filepath.Join(root, "renamed.md") || d.Buffer != buffer || !d.Dirty() || a.prefs.ActiveFile != d.Path {
		t.Fatal("rename lost the editing session or retained stale paths")
	}
	raw, _ := os.ReadFile(d.Path)
	if string(raw) == "unsaved draft" {
		t.Fatal("rename silently saved the dirty buffer")
	}
	a.requestFileAction(moveFile)
	a.fileName = "docs/moved.md"
	a.submitFilePrompt()
	drain(t, a)
	if d.Path != filepath.Join(root, "docs", "moved.md") || d.Buffer != buffer || d.Buffer.String() != "unsaved draft" {
		t.Fatal("move lost dirty text")
	}
	a.requestFileAction(deleteFile)
	r := a.filePrompt
	a.filePrompt = nil
	a.executeFileAction(r, "")
	drain(t, a)
	if len(a.docs) != 0 {
		t.Fatal("confirmed delete did not close the affected tab")
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "moved.md")); !os.IsNotExist(err) {
		t.Fatal("delete did not remove the file")
	}
}

func TestCreateAndDeleteApprovalLifetime(t *testing.T) {
	root := fixture(t)
	a := newApp()
	defer a.cancel()
	a.openFolder(root)
	drain(t, a)
	a.requestFileAction(createFolder)
	a.fileName = "notes"
	a.submitFilePrompt()
	drain(t, a)
	a.requestFileAction(createFile)
	a.fileName = "notes/new.md"
	a.submitFilePrompt()
	drain(t, a)
	d := a.active()
	if d == nil || d.Path != filepath.Join(root, "notes", "new.md") {
		t.Fatal("new file was not created and opened")
	}
	d.Buffer.Set("draft")
	a.requestFileAction(deleteFile)
	r := a.filePrompt
	d.Buffer.Set("later unapproved draft")
	a.executeFileAction(r, "")
	drain(t, a)
	if len(a.docs) != 1 || d.Buffer.String() != "later unapproved draft" {
		t.Fatal("delete discarded edits made after confirmation opened")
	}
	if _, err := os.Stat(d.Path); err != nil {
		t.Fatal("unapproved delete removed the file")
	}
}
