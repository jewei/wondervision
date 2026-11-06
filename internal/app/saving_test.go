package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestManualSaveAndCloseWithNewerEdits(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "todo.md")
	a := newApp()
	defer a.cancel()
	a.openFile(path, "", "")
	drain(t, a)
	d := a.active()
	tt := ui.NewTester(a.view, 1040, 720)
	d.Buffer.Set("snapshot\n")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyS)
	raw, _ := os.ReadFile(path)
	if string(raw) == "snapshot\n" {
		t.Fatal("save blocked the UI thread")
	}
	d.Buffer.Set("newer edit\n")
	drain(t, a)
	tt.Frame()
	raw, _ = os.ReadFile(path)
	if string(raw) != "snapshot\n" || !d.Dirty() {
		t.Fatal("save did not preserve newer in-memory edits")
	}
	tt.Key(ui.Cmd, ui.KeyW)
	if !a.askClose {
		t.Fatal("dirty tab did not ask before closing")
	}
	tt.Key(0, ui.KeyEscape)
	if len(a.docs) != 1 {
		t.Fatal("cancel closed the dirty tab")
	}
	tt.Key(ui.Cmd, ui.KeyW)
	tt.Key(0, ui.KeyEnter)
	drain(t, a)
	tt.Frame()
	if len(a.docs) != 0 {
		t.Fatal("saved tab was not closed")
	}
	raw, _ = os.ReadFile(path)
	if string(raw) != "newer edit\n" {
		t.Fatal("save and close lost edits")
	}
}

func TestSaveAsConfirmationAndOpenTabCollision(t *testing.T) {
	root := fixture(t)
	source := filepath.Join(root, "todo.md")
	target := filepath.Join(root, "copy.txt")
	os.WriteFile(target, []byte("existing"), 0600)
	a := newApp()
	defer a.cancel()
	a.openFile(source, "", "")
	drain(t, a)
	d := a.active()
	d.Buffer.Set("draft")
	a.prepareSaveAs(d, target, nil)
	drain(t, a)
	if !a.askReplace || d.Saving() {
		t.Fatal("existing Save As destination did not require confirmation")
	}
	raw, _ := os.ReadFile(target)
	if string(raw) != "existing" {
		t.Fatal("unconfirmed Save As changed the target")
	}
	a.confirmReplaceSave()
	drain(t, a)
	raw, _ = os.ReadFile(target)
	if string(raw) != "draft" || d.Path != target || d.Markdown || d.Dirty() {
		t.Fatal("Save As did not update file or tab state")
	}
	a.openFile(source, "", "")
	drain(t, a)
	a.prepareSaveAs(a.active(), target, nil)
	drain(t, a)
	if a.err != "destination is already open in another tab" {
		t.Fatal("Save As accepted another editable session's path")
	}
}

func TestDiscardApprovalDoesNotCoverFutureEdits(t *testing.T) {
	a := newApp()
	defer a.cancel()
	root := fixture(t)
	a.openFile(filepath.Join(root, "todo.md"), root, "")
	drain(t, a)
	d := a.active()
	d.Buffer.Set("approved draft")
	a.prefsPath = filepath.Join(t.TempDir(), "preferences.json")
	a.remember()
	a.approveDiscardQuit()
	finished := false
	a.finishQuit = func() { finished = true }
	if !a.deferQuit() {
		t.Fatal("quit should await preferences")
	}
	d.Buffer.Set("unapproved later edit")
	drain(t, a)
	if finished || !a.askQuit || a.allowQuit {
		t.Fatal("discard approval silently discarded a later edit")
	}
}
