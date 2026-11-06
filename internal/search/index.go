package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// publishEvery is how many paths a first (cold) scan collects between
// progressive publishes.
const publishEvery = 512

// Index is the in-memory list of one workspace's file paths, filled lazily by
// a background scan, for Quick Open. It is safe for concurrent use. Readers
// get a published snapshot whose elements are never modified afterwards.
type Index struct {
	ctx     context.Context
	root    string
	changed func()

	mu       sync.Mutex
	paths    []string // published snapshot, relative and '/'-separated
	complete bool     // a scan has finished at least once
	fresh    bool     // paths reflect the tree as of the last Invalidate
	scanning bool
	gen      uint64 // bumped by each scan start and Invalidate; stale scans can't publish
	cancel   context.CancelFunc
	done     chan struct{} // closed when the current scan ends
	err      error         // the last scan's error, other than cancellation
	version  uint64        // bumped on every publish
	memo     memo
}

type memo struct {
	query, recent string
	limit         int
	version       uint64
	paths         []string
	valid         bool
}

// Found is what Find knows now.
type Found struct {
	Paths    []string // best first, relative to the root, '/'-separated
	Scanning bool     // a scan is running; more or fresher paths may follow
	Err      error    // the last scan's error, other than cancellation
}

// NewIndex returns an Index for root. It does no I/O. ctx bounds every scan
// the Index starts: cancelling it stops the scan, and later calls keep
// returning the cache. changed may be nil; otherwise it is called from the
// scan goroutine after each publish and must not block.
func NewIndex(ctx context.Context, root string, changed func()) *Index {
	if changed == nil {
		changed = func() {}
	}
	return &Index{ctx: ctx, root: root, changed: changed}
}

// Find ranks the cached paths for query (see Rank) and returns up to limit of
// them. If the cache isn't fresh and no scan is running, it starts one. It
// never blocks on I/O, so a UI may call it on every frame; repeated calls with
// the same arguments and no new paths reuse the last result. recent are
// absolute OS paths, most recent first; those inside the root are boosted.
func (ix *Index) Find(query string, recent []string, limit int) Found {
	rel := ix.relative(recent)
	key := strings.Join(rel, "\x00")
	ix.mu.Lock()
	// A failed scan is not retried until the next Invalidate, so a missing
	// root can't make every frame start a scan.
	if !ix.fresh && !ix.scanning && ix.err == nil && ix.ctx.Err() == nil {
		ix.startLocked()
	}
	found := Found{Scanning: ix.scanning, Err: ix.err}
	m := ix.memo
	if m.valid && m.query == query && m.recent == key && m.limit == limit && m.version == ix.version {
		ix.mu.Unlock()
		found.Paths = m.paths
		return found
	}
	paths, version := ix.paths, ix.version
	ix.mu.Unlock()

	found.Paths = Rank(query, paths, rel, limit)
	ix.mu.Lock()
	if ix.version == version {
		ix.memo = memo{query: query, recent: key, limit: limit, version: version, paths: found.Paths, valid: true}
	}
	ix.mu.Unlock()
	return found
}

// Invalidate marks the cache stale, forgets the last scan error and stops a
// running scan. The current
// paths keep being returned until the next scan finishes and replaces them,
// unless no scan has ever finished, in which case the next scan publishes
// progressively. It does no I/O; the rescan starts at the next Find or Wait.
func (ix *Index) Invalidate() {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.fresh, ix.err = false, nil
	if ix.scanning {
		ix.cancel()
		ix.scanning = false
		ix.gen++
	}
}

// Wait starts a scan if needed and blocks until the cache is fresh, the scan
// fails, or ctx or the Index's context is done.
func (ix *Index) Wait(ctx context.Context) error {
	for {
		ix.mu.Lock()
		if ix.fresh {
			ix.mu.Unlock()
			return nil
		}
		if err := ix.ctx.Err(); err != nil {
			ix.mu.Unlock()
			return err
		}
		if !ix.scanning {
			if ix.err != nil {
				err := ix.err
				ix.mu.Unlock()
				return err
			}
			ix.startLocked()
		}
		done := ix.done
		ix.mu.Unlock()
		select {
		case <-done:
			ix.mu.Lock()
			err, fresh := ix.err, ix.fresh
			ix.mu.Unlock()
			if err != nil && !fresh {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-ix.ctx.Done():
			return ix.ctx.Err()
		}
	}
}

// relative converts absolute recent paths inside the root to the Index's
// '/'-separated relative form, dropping the others.
func (ix *Index) relative(recent []string) []string {
	var rel []string
	for _, p := range recent {
		r, err := filepath.Rel(ix.root, p)
		if err == nil && filepath.IsLocal(r) {
			rel = append(rel, filepath.ToSlash(r))
		}
	}
	return rel
}

// startLocked starts a scan. ix.mu must be held.
func (ix *Index) startLocked() {
	ix.gen++
	gen, cold := ix.gen, !ix.complete
	ctx, cancel := context.WithCancel(ix.ctx)
	done := make(chan struct{})
	ix.cancel, ix.done, ix.scanning, ix.err = cancel, done, true, nil
	if cold {
		ix.paths = nil
		ix.version++
	}
	go func() {
		defer close(done)
		defer cancel()
		var found []string
		err := func() error {
			r, err := os.OpenRoot(ix.root)
			if err != nil {
				return err
			}
			defer r.Close()
			return walk(ctx, r, func(rel string) error {
				found = append(found, rel)
				if cold && len(found)%publishEvery == 0 {
					ix.publish(gen, found[:len(found):len(found)], false, nil)
				}
				return nil
			})
		}()
		if ctx.Err() != nil {
			// Cancelled by Invalidate or the Index's context: a newer scan,
			// or none, owns the state now.
			ix.mu.Lock()
			if ix.gen == gen {
				ix.scanning = false
			}
			ix.mu.Unlock()
			return
		}
		ix.publish(gen, found[:len(found):len(found)], true, err)
	}()
}

// publish makes paths the current snapshot if the scan gen is still current.
func (ix *Index) publish(gen uint64, paths []string, final bool, err error) {
	ix.mu.Lock()
	if ix.gen != gen {
		ix.mu.Unlock()
		return
	}
	if err == nil || !ix.complete {
		ix.paths = paths
		ix.version++
	}
	if final {
		ix.scanning = false
		ix.err = err
		if err == nil {
			ix.fresh, ix.complete = true, true
		}
	}
	ix.mu.Unlock()
	ix.changed()
}
