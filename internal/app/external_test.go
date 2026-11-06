package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExternalChangeReloadsCleanAndProtectsDirty(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "todo.md")
	a := newApp()
	defer a.cancel()
	a.openFile(path, root, "")
	drain(t, a)
	d := a.active()
	os.WriteFile(path, []byte("clean external\n"), 0600)
	a.checkDisk(d)
	drain(t, a)
	if d.Buffer.String() != "clean external\n" || d.Dirty() {
		t.Fatal("clean buffer did not reload external content")
	}
	d.Buffer.Set("my dirty draft\n")
	os.WriteFile(path, []byte("new disk\n"), 0600)
	a.checkDisk(d)
	drain(t, a)
	if d.Buffer.String() != "my dirty draft\n" || !d.Dirty() {
		t.Fatal("external edit discarded dirty buffer")
	}
	if _, known := a.external[d]; !known {
		t.Fatal("dirty external change has no conflict indicator")
	}
	a.requestOverwrite(d)
	a.confirmOverwrite()
	drain(t, a)
	raw, _ := os.ReadFile(path)
	if string(raw) != "my dirty draft\n" || d.Dirty() {
		t.Fatal("explicit overwrite failed")
	}
	if _, known := a.external[d]; known {
		t.Fatal("our own save became a false conflict")
	}
}

func TestReloadAndOverwriteRejectNewerChanges(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "todo.md")
	a := newApp()
	defer a.cancel()
	a.openFile(path, root, "")
	drain(t, a)
	d := a.active()
	d.Buffer.Set("draft")
	os.WriteFile(path, []byte("disk version"), 0600)
	a.checkDisk(d)
	drain(t, a)
	a.reloadDocument(d)
	apply := (<-a.jobs)()
	d.Buffer.Set("later edit")
	apply()
	if d.Buffer.String() != "later edit" {
		t.Fatal("reload discarded edits made during read")
	}
	os.WriteFile(path, []byte("newer outside writer"), 0600)
	a.requestOverwrite(d)
	a.confirmOverwrite()
	drain(t, a)
	raw, _ := os.ReadFile(path)
	if string(raw) != "newer outside writer" || !d.Dirty() {
		t.Fatal("stale overwrite approval clobbered a newer disk version")
	}
}

func TestOwnSaveObservationAndCloseDiscardStaleReads(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "todo.md")
	a := newApp()
	defer a.cancel()
	a.openFile(path, root, "")
	drain(t, a)
	d := a.active()
	d.Buffer.Set("saved")
	a.saveDocument(d, nil)
	a.checkDisk(d)
	drain(t, a)
	if d.Dirty() || len(a.external) != 0 || a.checkAgain[d] {
		t.Fatal("own save was reported as external")
	}
	a.checkDisk(d)
	apply := (<-a.jobs)()
	a.removeDocument(d)
	apply()
	if len(a.checking) != 0 || len(a.external) != 0 || len(a.docs) != 0 {
		t.Fatal("closed tab retained a stale read or conflict")
	}
}

func TestDeletedFileRequiresExplicitRecreation(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "todo.md")
	a := newApp()
	defer a.cancel()
	a.openFile(path, root, "")
	drain(t, a)
	d := a.active()
	os.Remove(path)
	a.checkDisk(d)
	drain(t, a)
	disk, known := a.external[d]
	if !known || disk.Fingerprint.Exists {
		t.Fatal("missing disk file was not surfaced")
	}
	a.requestOverwrite(d)
	a.confirmOverwrite()
	drain(t, a)
	if _, err := os.Stat(path); err != nil {
		t.Fatal("explicit recreation did not restore the file")
	}
}

func TestOverwriteApprovalDoesNotFollowNewWatcherVersion(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "todo.md")
	a := newApp()
	defer a.cancel()
	a.openFile(path, root, "")
	drain(t, a)
	d := a.active()
	d.Buffer.Set("my draft")
	os.WriteFile(path, []byte("first external"), 0600)
	a.checkDisk(d)
	drain(t, a)
	a.requestOverwrite(d)
	os.WriteFile(path, []byte("unreviewed second external"), 0600)
	a.checkDisk(d)
	drain(t, a)
	a.confirmOverwrite()
	drain(t, a)
	raw, _ := os.ReadFile(path)
	if string(raw) != "unreviewed second external" || !d.Dirty() {
		t.Fatal("overwrite approval followed a newer unreviewed watcher version")
	}
}

func TestWatchEnrollmentReconcilesTheReadToWatchGap(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "todo.md")
	a := newApp()
	defer a.cancel()
	a.openFile(path, root, "")
	drain(t, a)
	d := a.active()
	os.WriteFile(path, []byte("changed before watch enrollment"), 0600)
	a.watchHint(d, path, a.ctx)
	drain(t, a)
	if d.Buffer.String() != "changed before watch enrollment" {
		t.Fatal("watch registration missed its initial reconciliation")
	}
	a.removeDocument(d)
	drain(t, a)
	a.watchHint(d, path, a.ctx)
	if len(a.jobs) != 0 {
		t.Fatal("closed document accepted a delayed watch hint")
	}
}
