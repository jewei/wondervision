package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/jewei/wondervision/internal/documents"
)

func (a *App) startWatch(d *documents.Document) {
	a.stopWatch(d)
	if a.win == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.watchCancel[d] = cancel
	path := d.Path
	go func() {
		watch, err := mygo.WatchFiles(ctx, path, mygo.FileWatchOptions{Debounce: 100 * time.Millisecond})
		if err != nil {
			if ctx.Err() == nil {
				a.post(func() {
					if !a.closed && ctx.Err() == nil {
						a.notify(fmt.Errorf("file watch unavailable; use Reload: %w", err))
					}
				})
			}
			return
		}
		defer watch.Close()
		a.post(func() {
			a.watchHint(d, path, ctx)
		})
		for {
			_, err := watch.Next(ctx)
			if err != nil {
				if ctx.Err() == nil {
					a.post(func() {
						if !a.closed && a.hasDocument(d) {
							a.notify(errors.New("file watch ended; use Reload to check the disk"))
						}
					})
				}
				return
			}
			a.post(func() {
				if !a.closed && ctx.Err() == nil && a.hasDocument(d) && d.Path == path {
					a.checkDisk(d)
				}
			})
		}
	}()
}

func (a *App) stopWatch(d *documents.Document) {
	if cancel := a.watchCancel[d]; cancel != nil {
		cancel()
		delete(a.watchCancel, d)
	}
}

func (a *App) checkDisk(d *documents.Document) {
	if !a.hasDocument(d) {
		return
	}
	if d.Saving() || a.checking[d] {
		a.checkAgain[d] = true
		return
	}
	a.checking[d] = true
	path, area, version, saved := d.Path, d.Area, d.Buffer.Version(), d.SavedFingerprint()
	if !a.enqueue(func() func() {
		disk, err := documents.ReadDisk(a.ctx, path, area)
		return func() {
			if !a.hasDocument(d) || d.Path != path {
				return
			}
			a.checking[d] = false
			defer func() {
				if a.checkAgain[d] {
					delete(a.checkAgain, d)
					a.checkDisk(d)
				}
			}()
			if d.Saving() {
				a.checkAgain[d] = true
				return
			}
			if saved != d.SavedFingerprint() {
				a.checkAgain[d] = true
				return
			}
			if err != nil {
				if disk.Known && disk.Fingerprint != d.SavedFingerprint() {
					a.external[d] = disk
				}
				a.notify(fmt.Errorf("read disk version: %w", err))
				return
			}
			if disk.Fingerprint == d.SavedFingerprint() {
				delete(a.external, d)
				return
			}
			if !d.Dirty() && d.Buffer.Version() == version && disk.Document != nil {
				if d.AcceptExternal(disk.Document, version) {
					delete(a.external, d)
					a.releasePreview()
				}
				return
			}
			a.external[d] = disk
		}
	}) {
		a.checking[d] = false
	}
}

func (a *App) requestReload(d *documents.Document) {
	if d == nil || d.Saving() {
		return
	}
	if d.Dirty() {
		a.reloadDoc = d
		a.askReload = true
		return
	}
	a.reloadDocument(d)
}

func (a *App) reloadDocument(d *documents.Document) {
	path, area, version := d.Path, d.Area, d.Buffer.Version()
	a.enqueue(func() func() {
		disk, err := documents.ReadDisk(a.ctx, path, area)
		return func() {
			if !a.hasDocument(d) || d.Path != path {
				return
			}
			if err != nil {
				a.notify(err)
				return
			}
			if d.Saving() || d.Buffer.Version() != version {
				a.notify(errors.New("the buffer changed while reloading; review the current edits before retrying"))
				return
			}
			if disk.Document == nil {
				a.external[d] = disk
				a.notify(errors.New("file no longer exists; use Save As or explicitly recreate it"))
				return
			}
			if d.ReloadDiscarding(disk.Document, version) {
				a.discardRecovery(d)
				delete(a.external, d)
				a.releasePreview()
			}
		}
	})
}

func (a *App) compareDisk(d *documents.Document) {
	if d == nil {
		return
	}
	path, area := d.Path, d.Area
	a.enqueue(func() func() {
		disk, err := documents.ReadDisk(a.ctx, path, area)
		return func() {
			if !a.hasDocument(d) || d.Path != path {
				return
			}
			if err != nil {
				a.notify(err)
				return
			}
			if disk.Document == nil {
				a.notify(errors.New("there is no disk version to display"))
				return
			}
			a.external[d] = disk
			a.compareDoc = d
			a.compareEditor = disk.Document.Editor
			a.compareEditor.ReadOnly = true
			a.compareOpen = true
		}
	})
}

type overwriteRequest struct {
	doc      *documents.Document
	path     string
	version  uint64
	expected documents.Fingerprint
}

func (a *App) requestOverwrite(d *documents.Document) {
	if d == nil || d.Saving() {
		return
	}
	disk, known := a.external[d]
	if !known {
		a.checkDisk(d)
		return
	}
	a.pendingOverwrite = &overwriteRequest{d, d.Path, d.Buffer.Version(), disk.Fingerprint}
	a.askOverwrite = true
}

func (a *App) confirmOverwrite() {
	r := a.pendingOverwrite
	a.pendingOverwrite = nil
	if r == nil || !a.hasDocument(r.doc) {
		return
	}
	if r.doc.Path != r.path || r.doc.Buffer.Version() != r.version {
		a.notify(errors.New("the buffer changed while confirming; review it before overwriting"))
		return
	}
	plan, err := r.doc.PrepareOverwrite(r.expected)
	if err != nil {
		a.notify(err)
		return
	}
	a.writeSave(r.doc, plan, nil)
}

func (a *App) showDiskComparison(c *ui.Context) {
	ui.Modal(c, &a.compareOpen, func() {
		ui.Column(c).Width(760).Height(480).Gap(8).Children(func() {
			ui.Text(c, "Disk version · "+a.compareDoc.Path).Bold()
			editor.View(c, a.compareEditor).Label("Disk version").Grow(1).FillWidth()
			ui.Button(c, "Close comparison").OnClick(func() { a.compareOpen = false })
		})
	})
	if !a.compareOpen {
		a.compareDoc = nil
		a.compareEditor = nil
	}
}

func (a *App) watchHint(d *documents.Document, path string, ctx context.Context) {
	if !a.closed && ctx.Err() == nil && a.hasDocument(d) && d.Path == path {
		a.checkDisk(d)
	}
}
