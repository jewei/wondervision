package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jewei/wondervision/internal/documents"
)

func TestUnreviewedRecoverySurvivesFreshEditingAndSave(t *testing.T) {
	a := newApp()
	defer a.cancel()
	root := fixture(t)
	path := filepath.Join(root, "todo.md")
	a.openFile(path, root, "")
	drain(t, a)
	d := a.active()
	d.Buffer.Set("old crash draft")
	old := documents.NewRecoveryStore(filepath.Join(t.TempDir(), "recovery"))
	plan, _ := d.RecoverySnapshot()
	if err := old.Write(a.ctx, plan); err != nil {
		t.Fatal(err)
	}
	a.recoveryStore = documents.NewRecoveryStore(old.Dir)
	a.loadRecoveries()
	drain(t, a)
	d.Buffer.Set("fresh session edit")
	a.observeEdits()
	if e := a.recoveryEntries[d]; e != nil && e.timer != nil {
		t.Fatal("unreviewed draft started an overwrite timer")
	}
	a.writeRecovery(d, a.ctx)
	drain(t, a)
	a.saveDocument(d, nil)
	drain(t, a)
	records, err := old.List(a.ctx)
	if err != nil || len(records) != 1 {
		t.Fatal("fresh save removed the older unreviewed draft")
	}
	copy, err := old.Load(a.ctx, records[0])
	if err != nil || copy.Buffer.String() != "old crash draft" {
		t.Fatal("old recovery content changed")
	}
	a.recoverDraft(records[0])
	drain(t, a)
	if d.Buffer.String() != "old crash draft" || !d.Dirty() || a.hasUnreviewedRecovery(path) {
		t.Fatal("explicit recover did not restore the draft")
	}
	// Saving immediately after Recover, before another recovery timer, also removes the old copy.
	a.saveDocument(d, nil)
	drain(t, a)
	if !d.Dirty() {
		t.Fatal("recovered draft silently overwrote the fresh session's saved file")
	}
	a.checkDisk(d)
	drain(t, a)
	a.requestOverwrite(d)
	a.confirmOverwrite()
	drain(t, a)
	records, err = old.List(a.ctx)
	if err != nil || len(records) != 0 {
		t.Fatal("saved recovered draft was retained")
	}
}

func TestRecoveryStartsOnlyAfterEditsAndStopsAfterSave(t *testing.T) {
	a := newApp()
	defer a.cancel()
	root := fixture(t)
	a.openFile(filepath.Join(root, "todo.md"), root, "")
	drain(t, a)
	d := a.active()
	a.recoveryStore = documents.NewRecoveryStore(filepath.Join(t.TempDir(), "recovery"))
	a.recoveryDelay = time.Millisecond
	callbacks := make(chan func(), 4)
	a.post = func(f func()) { callbacks <- f }
	a.observeEdits()
	if e := a.recoveryEntries[d]; e != nil && e.timer != nil {
		t.Fatal("untouched document started a recovery timer")
	}
	d.Buffer.Set("draft")
	a.observeEdits()
	select {
	case apply := <-callbacks:
		apply()
	case <-time.After(time.Second):
		t.Fatal("edit-triggered recovery never fired")
	}
	drain(t, a)
	records, err := a.recoveryStore.List(a.ctx)
	if err != nil || len(records) != 1 {
		t.Fatal("dirty draft was not stored")
	}
	a.saveDocument(d, nil)
	drain(t, a)
	a.observeEdits()
	drain(t, a)
	records, err = a.recoveryStore.List(a.ctx)
	if err != nil || len(records) != 0 {
		t.Fatal("saved draft recovery was retained")
	}
	if e := a.recoveryEntries[d]; e != nil && e.timer != nil {
		t.Fatal("clean document kept an idle recovery timer")
	}
}

func TestDiscardedDraftCleanupCompletesBeforeQuit(t *testing.T) {
	a := newApp()
	defer a.cancel()
	root := fixture(t)
	a.openFile(filepath.Join(root, "todo.md"), root, "")
	drain(t, a)
	d := a.active()
	d.Buffer.Set("draft")
	a.recoveryStore = documents.NewRecoveryStore(filepath.Join(t.TempDir(), "recovery"))
	a.writeRecovery(d, a.ctx)
	drain(t, a)
	finished := false
	a.finishQuit = func() { finished = true }
	a.approveDiscardQuit()
	if !a.deferQuit() {
		t.Fatal("quit did not wait for recovery cleanup")
	}
	if finished {
		t.Fatal("quit ran before cleanup")
	}
	drain(t, a)
	records, _ := a.recoveryStore.List(a.ctx)
	if !finished || len(records) != 0 || a.pendingWrites != 0 {
		t.Fatal("quit left recovery behind or failed to settle")
	}
	if _, err := os.Stat(d.Path); err != nil {
		t.Fatal("discard removed the original file")
	}
}
