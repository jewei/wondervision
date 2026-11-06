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
