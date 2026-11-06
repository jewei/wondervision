package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/jewei/wondervision/internal/documents"
)

type recoveryEntry struct {
	version uint64
	dirty   bool
	timer   *time.Timer
	cancel  context.CancelFunc
	hasCopy bool
}

func (a *App) initializeRecovery() {
	dir, err := os.UserCacheDir()
	if err != nil {
		a.notify(err)
		return
	}
	a.recoveryStore = documents.NewRecoveryStore(filepath.Join(dir, "wondervision", "recovery"))
}

func (a *App) loadRecoveries() {
	if a.recoveryStore == nil {
		return
	}
	store := *a.recoveryStore
	a.enqueue(func() func() {
		records, err := store.List(a.ctx)
		return func() { a.recoveries = records; a.notify(err) }
	})
}

func (a *App) observeEdits() {
	if a.recoveryStore == nil || a.allowQuit {
		return
	}
	for _, d := range a.docs {
		if a.hasUnreviewedRecovery(d.Path) {
			continue
		}
		dirty, version := d.Dirty(), d.Buffer.Version()
		e := a.recoveryEntries[d]
		if e != nil && e.version == version && e.dirty == dirty {
			continue
		}
		if e != nil {
			if e.timer != nil {
				e.timer.Stop()
			}
			if e.cancel != nil {
				e.cancel()
			}
		} else {
			e = &recoveryEntry{}
			a.recoveryEntries[d] = e
		}
		e.version, e.dirty = version, dirty
		if !dirty || documents.Sensitive(d.Path) {
			if e.hasCopy {
				e.hasCopy = false
				a.removeRecoveryPath(d.Path)
			}
			continue
		}
		ctx, cancel := context.WithCancel(a.ctx)
		e.cancel = cancel
		e.timer = time.AfterFunc(a.recoveryDelay, func() {
			if ctx.Err() != nil || a.post == nil {
				return
			}
			a.post(func() {
				if a.closed || ctx.Err() != nil || !a.hasDocument(d) || d.Buffer.Version() != version || !d.Dirty() {
					return
				}
				a.writeRecovery(d, ctx)
			})
		})
	}
}

func (a *App) writeRecovery(d *documents.Document, ctx context.Context) {
	if a.recoveryStore == nil || a.allowQuit || a.hasUnreviewedRecovery(d.Path) {
		return
	}
	plan, err := d.RecoverySnapshot()
	if err != nil {
		return
	}
	store := *a.recoveryStore
	a.pendingWrites++
	if !a.enqueue(func() func() {
		err := store.Write(ctx, plan)
		return func() {
			a.pendingWrites--
			if err != nil && !errors.Is(err, context.Canceled) {
				a.notify(fmt.Errorf("recovery snapshot: %w", err))
				if errors.Is(err, documents.ErrUnreviewedRecovery) {
					a.loadRecoveries()
				}
			}
			a.settleQuit()
		}
	}) {
		a.pendingWrites--
		return
	}
	if e := a.recoveryEntries[d]; e != nil {
		e.hasCopy = true
	}
}

func (a *App) removeRecoveryPath(path string) {
	if a.recoveryStore == nil {
		return
	}
	store := *a.recoveryStore
	a.pendingWrites++
	if !a.enqueue(func() func() {
		err := store.Remove(a.ctx, path)
		return func() {
			a.pendingWrites--
			if err != nil {
				a.notify(fmt.Errorf("remove recovery copy: %w", err))
				a.allowQuit = false
				a.quitWhenSaved = false
			}
			a.settleQuit()
		}
	}) {
		a.pendingWrites--
	}
}

func (a *App) discardRecovery(d *documents.Document) {
	if e := a.recoveryEntries[d]; e != nil {
		if e.timer != nil {
			e.timer.Stop()
		}
		if e.cancel != nil {
			e.cancel()
		}
		delete(a.recoveryEntries, d)
	}
	a.removeRecoveryPath(d.Path)
	if record, approved := d.RecoveryClaim(d.Path); approved {
		a.discardRecoveryRecord(record)
	}
}

func (a *App) recoverySaved(d *documents.Document, oldPath string) {
	if e := a.recoveryEntries[d]; e != nil {
		if e.timer != nil {
			e.timer.Stop()
		}
		if e.cancel != nil {
			e.cancel()
		}
		delete(a.recoveryEntries, d)
	}
	if oldPath != d.Path || !d.Dirty() || documents.Sensitive(d.Path) {
		a.removeRecoveryPath(oldPath)
		if record, approved := d.RecoveryClaim(oldPath); approved {
			a.discardRecoveryRecord(record)
		}
	}
}

func (a *App) recoverDraft(record documents.RecoveryRecord) {
	if a.fileBusy {
		return
	}
	if a.openCancel != nil {
		a.openCancel()
	}
	a.openID++
	request := a.openID
	if a.recoveryStore == nil {
		return
	}
	store := *a.recoveryStore
	a.enqueue(func() func() {
		recovered, err := store.Load(a.ctx, record)
		return func() {
			if request != a.openID {
				return
			}
			if err != nil {
				a.notify(err)
				return
			}
			for i, d := range a.docs {
				if d.Same(recovered) {
					if !d.RestoreDraft(recovered) {
						a.notify(errors.New("a dirty buffer is already open; save or discard it before recovering this copy"))
						return
					}
					a.activate(i)
					a.releasePreview()
					a.checkDisk(d)
					a.forgetRecoveryRecord(record)
					return
				}
			}
			a.docs = append(a.docs, recovered)
			a.scroll[recovered] = &ui.ScrollState{}
			a.activate(len(a.docs) - 1)
			a.startWatch(recovered)
			a.checkDisk(recovered)
			a.forgetRecoveryRecord(record)
		}
	})
}

func (a *App) forgetRecoveryRecord(record documents.RecoveryRecord) {
	for _, d := range a.docs {
		if d.Path == record.Path {
			if e := a.recoveryEntries[d]; e != nil {
				if e.timer != nil {
					e.timer.Stop()
				}
				if e.cancel != nil {
					e.cancel()
				}
				delete(a.recoveryEntries, d)
			}
		}
	}
	for i, r := range a.recoveries {
		if r.Path == record.Path {
			a.recoveries = append(a.recoveries[:i], a.recoveries[i+1:]...)
			break
		}
	}
	if len(a.recoveries) == 0 {
		a.recoveryPanel = false
	}
}

func (a *App) discardRecoveryRecord(record documents.RecoveryRecord) {
	if a.recoveryStore == nil {
		return
	}
	store := *a.recoveryStore
	a.pendingWrites++
	if !a.enqueue(func() func() {
		err := store.Discard(a.ctx, record)
		return func() {
			a.pendingWrites--
			if err == nil {
				a.forgetRecoveryRecord(record)
			} else {
				a.notify(err)
			}
			a.settleQuit()
		}
	}) {
		a.pendingWrites--
	}
}

func (a *App) showRecoveryPanel(c *ui.Context) {
	ui.Modal(c, &a.recoveryPanel, func() {
		ui.Column(c).Width(680).MaxHeight(500).Gap(12).Children(func() {
			ui.Text(c, "Recover unsaved drafts").Bold()
			ui.Scroll(c).Height(min(360, float32(len(a.recoveries))*64)).FillWidth().Children(func() {
				for _, record := range a.recoveries {
					ui.Row(c.Key(record.Path)).Padding(8).Gap(8).Children(func() {
						ui.Text(c, record.Path).Grow(1)
						ui.Button(c, "Recover").OnClick(func() { a.recoverDraft(record) })
						ui.Button(c, "Discard recovery").OnClick(func() { a.discardRecoveryRecord(record) })
					})
				}
			})
			ui.Button(c, "Later").OnClick(func() { a.recoveryPanel = false })
		})
	})
}

func (a *App) hasUnreviewedRecovery(path string) bool {
	for _, record := range a.recoveries {
		if record.Path == path {
			return true
		}
	}
	return false
}
