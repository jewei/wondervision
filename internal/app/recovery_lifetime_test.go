package app

import (
	"github.com/jewei/wondervision/internal/documents"
	"os"
	"path/filepath"
	"testing"
)

func recoveredFixture(t *testing.T) (*App, *documents.Document, *documents.RecoveryStore, *documents.Document) {
	a := newApp()
	t.Cleanup(a.cancel)
	root := fixture(t)
	path := filepath.Join(root, "todo.md")
	original, err := documents.Open(a.ctx, path, root, documents.Edit)
	if err != nil {
		t.Fatal(err)
	}
	original.Buffer.Set("previous session draft")
	old := documents.NewRecoveryStore(filepath.Join(t.TempDir(), "recovery"))
	plan, _ := original.RecoverySnapshot()
	if err = old.Write(a.ctx, plan); err != nil {
		t.Fatal(err)
	}
	records, err := old.List(a.ctx)
	if err != nil || len(records) != 1 {
		t.Fatal(err)
	}
	a.recoveryStore = documents.NewRecoveryStore(old.Dir)
	a.recoverDraft(records[0])
	drain(t, a)
	return a, a.active(), old, original
}
func TestReloadRemovesRecoveredClaim(t *testing.T) {
	a, d, old, _ := recoveredFixture(t)
	a.observeEdits()
	a.reloadDocument(d)
	drain(t, a)
	a.observeEdits()
	drain(t, a)
	if d.Dirty() {
		t.Fatal("reload did not complete")
	}
	records, err := old.List(a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("confirmed reload discarded buffer but retained %d recovery records", len(records))
	}
}
func TestClaimDiscardFailureBlocksQuit(t *testing.T) {
	a, _, old, original := recoveredFixture(t)
	original.Buffer.Set("newer draft by original owner")
	plan, _ := original.RecoverySnapshot()
	if err := old.Write(a.ctx, plan); err != nil {
		t.Fatal(err)
	}
	finished := false
	a.finishQuit = func() { finished = true }
	a.approveDiscardQuit()
	if !a.deferQuit() {
		t.Fatal("expected pending cleanup")
	}
	drain(t, a)
	if finished {
		t.Fatalf("quit completed despite rejected claim cleanup: %s", a.err)
	}
}

func TestDiscardQuitBlocksWhenCleanupCannotBeQueued(t *testing.T) {
	a, _, old, _ := recoveredFixture(t)
	for len(a.jobs) < cap(a.jobs) {
		a.jobs <- func() func() { return nil }
	}
	finished := false
	a.finishQuit = func() { finished = true }
	a.approveDiscardQuit()
	if !a.deferQuit() {
		t.Fatal("quit bypassed rejected recovery cleanup")
	}
	drain(t, a)
	if finished || a.allowQuit || !a.askQuit {
		t.Fatal("queue saturation lost the cleanup failure")
	}
	records, err := old.List(a.ctx)
	if err != nil || len(records) != 1 {
		t.Fatal("failed cleanup did not preserve the copy")
	}
}
func TestRecoveredRenameCleansOldClaim(t *testing.T) {
	for _, name := range []string{"renamed.md", ".env"} {
		t.Run(name, func(t *testing.T) {
			a, d, old, _ := recoveredFixture(t)
			oldPath := d.Path
			newPath := filepath.Join(filepath.Dir(oldPath), name)
			if err := os.Rename(oldPath, newPath); err != nil {
				t.Fatal(err)
			}
			d.Relocate(newPath, d.Area, d.Info)
			a.recoverySaved(d, oldPath)
			if d.Dirty() {
				a.writeRecovery(d, a.ctx)
			}
			drain(t, a)
			records, err := old.List(a.ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range records {
				if r.Path == oldPath {
					t.Fatal("old claim retained")
				}
			}
			if name == ".env" && len(records) != 0 {
				t.Fatal("sensitive rename retained a recovery")
			}
		})
	}
}
