package app

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jewei/wondervision/internal/search"
)

// goroutinesSettle waits for the goroutine count to fall back to base.
func goroutinesSettle(t *testing.T, base int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > base && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > base {
		buf := make([]byte, 1<<16)
		t.Fatalf("goroutines leaked: %d before, %d after\n%s", base, n, buf[:runtime.Stack(buf, true)])
	}
}

// Spec §13: repeated workspace switching and cancelled scans don't leak.
// Each cycle opens a folder, starts a Quick Open scan and a content search,
// then switches away before either finishes.
func TestRepeatedWorkspaceSwitchingWithCancelledScansDoesNotLeak(t *testing.T) {
	a := newApp()
	t.Cleanup(a.cancel)
	posted := make(chan func(), 1000)
	a.post = func(f func()) { posted <- f }
	roots := []string{fixture(t), fixture(t), fixture(t)}
	runtime.GC()
	base := runtime.NumGoroutine()
	for i := range 30 {
		a.openFolder(roots[i%len(roots)])
		drain(t, a)
		a.files.Find("next", nil, 10) // starts the scan
		a.showOverlay(searchOverlay)
		a.overlay.query = "Report"
		a.startTextSearch()
		if i%2 == 0 {
			a.closeOverlay() // cancel the search by dismissing
		}
	}
	a.openFolder(roots[0])
	drain(t, a)
	a.closeOverlay()
	// Let cancelled work deliver its last posts, then apply them.
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > base && time.Now().Before(deadline) {
		select {
		case f := <-posted:
			f()
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	for len(posted) > 0 {
		(<-posted)()
	}
	goroutinesSettle(t, base)
	if a.overlay.search != nil || a.overlay.kind != noOverlay {
		t.Fatal("a cancelled search survived the workspace switch")
	}
	if a.workspace.Root != roots[0] {
		t.Fatalf("workspace = %s, want %s", a.workspace.Root, roots[0])
	}
}

// Spec §13: repeated search and cancel while the user edits; no deadlocks
// and no stale results. The search that is left running must show exactly
// what a fresh search of the current buffer and disk finds.
func TestSearchCancelAndEditLoopShowsOnlyCurrentResults(t *testing.T) {
	a := newApp()
	t.Cleanup(a.cancel)
	posted := make(chan func(), 1000)
	a.post = func(f func()) { posted <- f }
	root := fixture(t)
	a.openFolder(root)
	drain(t, a)
	a.openFile(filepath.Join(root, "todo.md"), root, "")
	drain(t, a)
	d := a.active()
	runtime.GC()
	base := runtime.NumGoroutine()

	a.showOverlay(searchOverlay)
	for i := range 60 {
		// Edit while searches are in flight; the snapshot taken at each start
		// is what that run searches.
		d.Buffer.Set(fmt.Sprintf("# TODO\nmarker-%d\nmarker-%d again\n", i, i))
		a.overlay.query = fmt.Sprintf("marker-%d", i)
		a.startTextSearch()
		switch i % 3 {
		case 0:
			a.startTextSearch() // superseded at once by an identical search
		case 1:
			// Let this run finish but leave its results queued, as when the UI
			// thread is busy; they are applied only after newer runs start.
			before := len(posted)
			wait := time.After(5 * time.Second)
			for len(posted) < before+2 { // one batch, then the end
				select {
				case <-wait:
					t.Fatal("a search never posted its results")
				default:
					time.Sleep(time.Millisecond)
				}
			}
		}
	}
	final := a.overlay.search
	deadline := time.After(10 * time.Second)
	for final.running {
		select {
		case f := <-posted:
			f()
		case <-deadline:
			t.Fatal("the last search never finished (deadlock?)")
		}
	}
	for len(posted) > 0 {
		(<-posted)()
	}

	var want []search.Hit
	_, err := search.Grep(context.Background(), root, final.query, a.unsavedTexts(), func(b []search.Hit) { want = append(want, b...) })
	if err != nil {
		t.Fatal(err)
	}
	got := a.overlay.search.hits
	if a.overlay.search != final || len(got) != len(want) || len(got) != 2 {
		t.Fatalf("shown %d hits, a fresh search finds %d (want the 2 lines of marker-59)", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("hit %d is stale: shown %+v, current %+v", i, got[i], want[i])
		}
	}
	a.closeOverlay()
	goroutinesSettle(t, base)
}
