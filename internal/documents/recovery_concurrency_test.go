package documents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Pause both writes at the third context boundary. With locking this is lock
// admission; without the lock call it is the header write after ownership read.
// The latter makes the original check-then-replace race deterministic.
type recoveryBarrierContext struct {
	context.Context
	calls   atomic.Int32
	ready   chan<- struct{}
	release <-chan struct{}
}

func (c *recoveryBarrierContext) Err() error {
	if c.calls.Add(1) == 3 {
		c.ready <- struct{}{}
		<-c.release
	}
	return nil
}
func TestConcurrentSessionRecoveryWrites(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "todo.md")
	os.WriteFile(path, []byte("base"), 0600)
	d, _ := Open(ctx, path, root, Edit)
	d.Buffer.Set("draft A")
	p1, _ := d.RecoverySnapshot()
	d.Buffer.Set("draft B")
	p2, _ := d.RecoverySnapshot()
	dir := filepath.Join(t.TempDir(), "recovery")
	os.Mkdir(dir, 0700)
	a, b := NewRecoveryStore(dir), NewRecoveryStore(dir)
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	var e1, e2 error
	go func() {
		defer wg.Done()
		e1 = a.Write(&recoveryBarrierContext{Context: ctx, ready: ready, release: release}, p1)
	}()
	go func() {
		defer wg.Done()
		e2 = b.Write(&recoveryBarrierContext{Context: ctx, ready: ready, release: release}, p2)
	}()
	for range 2 {
		select {
		case <-ready:
		case <-time.After(2 * time.Second):
			close(release)
			wg.Wait()
			t.Fatal("writers did not reach the lock boundary")
		}
	}
	close(release)
	wg.Wait()
	for _, err := range []error{e1, e2} {
		if err != nil && !errors.Is(err, ErrRecoveryBusy) && !errors.Is(err, ErrUnreviewedRecovery) {
			t.Fatal(err)
		}
	}
	if e1 == nil && e2 == nil {
		t.Fatal("both sessions wrote successfully to one path; one unreviewed draft was overwritten")
	}
}

// A crash between creating and renaming a temp file leaves the temp behind.
// List removes such files only while it holds the recovery lock: a live writer
// keeps the lock until its temp is gone, so a temp seen without the lock may
// belong to a write in progress. This is CleanTemps in
// verification/recovery/Recovery.tla.
func TestListRemovesOnlyOrphanedRecoveryTemps(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "todo.md")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := Open(ctx, path, root, Edit)
	if err != nil {
		t.Fatal(err)
	}
	d.Buffer.Set("unsaved draft")
	store := NewRecoveryStore(filepath.Join(t.TempDir(), "recovery"))
	plan, _ := d.RecoverySnapshot()
	if err = store.Write(ctx, plan); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(store.Dir, recoveryTempPrefix+"crashed")
	if err = os.WriteFile(orphan, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}

	dir, err := os.OpenRoot(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	held, err := recoveryLock(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	records, err := NewRecoveryStore(store.Dir).List(ctx)
	if err != nil || len(records) != 1 {
		t.Fatalf("list while busy: %v %v", records, err)
	}
	if _, err = os.Stat(orphan); err != nil {
		t.Fatalf("List removed a temp file while another session held the lock: %v", err)
	}
	held.Close()

	records, err = NewRecoveryStore(store.Dir).List(ctx)
	if err != nil || len(records) != 1 {
		t.Fatalf("list: %v %v", records, err)
	}
	if _, err = os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("List kept an orphaned temp file: %v", err)
	}
}
