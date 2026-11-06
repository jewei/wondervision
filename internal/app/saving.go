package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/jewei/wondervision/internal/documents"
	"github.com/jewei/wondervision/internal/workspace"
)

type replaceSaveRequest struct {
	doc      *documents.Document
	path     string
	expected documents.Fingerprint
	after    func()
}

func (a *App) hasDocument(d *documents.Document) bool {
	for _, open := range a.docs {
		if open == d {
			return true
		}
	}
	return false
}
func (a *App) anySaving() bool {
	for _, d := range a.docs {
		if d.Saving() {
			return true
		}
	}
	return false
}

func (a *App) saveDocument(d *documents.Document, after func()) {
	if d == nil || d.Saving() {
		return
	}
	plan, err := d.PrepareSave()
	if err != nil {
		a.notify(err)
		return
	}
	a.writeSave(d, plan, after)
}

func (a *App) writeSave(d *documents.Document, plan documents.SavePlan, after func()) {
	oldPath := d.Path
	if !a.enqueue(func() func() {
		result, err := plan.Write(a.ctx)
		return func() {
			defer a.settleQuit()
			if !a.hasDocument(d) {
				return
			}
			if !d.CompleteSave(result, err) {
				return
			}
			if err != nil {
				a.notify(fmt.Errorf("save: %w", err))
				a.checkDisk(d)
				a.quitWhenSaved = false
				a.allowQuit = false
				return
			}
			area := d.Area
			if !workspace.Contains(area, d.Path) {
				area = filepath.Dir(d.Path)
			}
			if a.workspace != nil && workspace.Contains(a.workspace.Root, d.Path) {
				area = a.workspace.Root
			}
			d.Relocate(d.Path, area, result.Info)
			if oldPath != d.Path {
				a.releasePreview()
			}
			delete(a.external, d)
			if oldPath != d.Path {
				a.stopWatch(d)
				delete(a.checking, d)
				delete(a.checkAgain, d)
				a.startWatch(d)
			}
			if a.checkAgain[d] {
				delete(a.checkAgain, d)
				a.checkDisk(d)
			}
			a.prefs.ActiveFile = a.activePath()
			a.remember()
			a.notify(result.Warning)
			if d.Dirty() {
				a.notify(errors.New("saved the snapshot; newer edits are still unsaved"))
				return
			}
			if after != nil {
				after()
			}
		}
	}) {
		d.CancelSave(plan)
	}
}

func (a *App) activePath() string {
	if d := a.active(); d != nil {
		return d.Path
	}
	return ""
}

func (a *App) saveAs(d *documents.Document, after func()) {
	if d == nil || d.Saving() || a.dialogOpen {
		return
	}
	a.dialogOpen = true
	win := a.win
	path := d.Path
	go func() {
		target, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: win, Title: "Save As", DefaultPath: path, ShowHiddenFiles: true})
		win.Update(func() {
			a.dialogOpen = false
			if a.closed || !a.hasDocument(d) {
				return
			}
			if err != nil {
				a.notify(err)
				return
			}
			if target == "" {
				return
			}
			a.prepareSaveAs(d, target, after)
		})
	}()
}

func (a *App) prepareSaveAs(d *documents.Document, target string, after func()) {
	a.enqueue(func() func() {
		path, err := documents.Destination(target)
		var fp documents.Fingerprint
		if err == nil {
			fp, err = documents.InspectDestination(a.ctx, path)
		}
		return func() {
			if !a.hasDocument(d) || d.Saving() {
				return
			}
			if err != nil {
				a.notify(err)
				return
			}
			for _, other := range a.docs {
				same := other.Path == path || (runtime.GOOS == "windows" && strings.EqualFold(other.Path, path))
				if same && other != d {
					a.notify(errors.New("destination is already open in another tab"))
					return
				}
			}
			if path == d.Path {
				a.saveDocument(d, after)
				return
			}
			if fp.Exists {
				a.replaceSave = &replaceSaveRequest{d, path, fp, after}
				a.askReplace = true
				return
			}
			plan, err := d.PrepareSaveAs(path)
			if err != nil {
				a.notify(err)
				return
			}
			a.writeSave(d, plan, after)
		}
	})
}

func (a *App) confirmReplaceSave() {
	r := a.replaceSave
	a.replaceSave = nil
	if r == nil || !a.hasDocument(r.doc) {
		return
	}
	plan, err := r.doc.PrepareSaveAsReplace(r.path, r.expected)
	if err != nil {
		a.notify(err)
		return
	}
	a.writeSave(r.doc, plan, r.after)
}

func (a *App) requestClose(d *documents.Document) {
	if d == nil {
		return
	}
	if d.Saving() {
		a.notify(errors.New("wait for the current save to finish before closing"))
		return
	}
	if d.Dirty() {
		a.closeDoc = d
		a.askClose = true
		return
	}
	a.removeDocument(d)
}

func (a *App) removeDocument(d *documents.Document) {
	for i, open := range a.docs {
		if open == d {
			a.stopWatch(d)
			delete(a.external, d)
			delete(a.checking, d)
			delete(a.checkAgain, d)
			if a.preview != nil && a.preview.doc == d {
				a.releasePreview()
			}
			a.docs = append(a.docs[:i], a.docs[i+1:]...)
			delete(a.scroll, d)
			if i < a.tab {
				a.tab--
			}
			if a.tab >= len(a.docs) {
				a.tab = max(0, len(a.docs)-1)
			}
			a.switchTab(a.tab)
			a.prefs.ActiveFile = a.activePath()
			a.remember()
			return
		}
	}
}

func (a *App) saveAndQuit() {
	for _, d := range a.docs {
		if d.Saving() {
			a.notify(errors.New("wait for the current save to finish"))
			return
		}
		if d.Dirty() {
			a.saveDocument(d, a.saveAndQuit)
			return
		}
	}
	a.allowQuit = true
	if !a.deferQuit() && a.finishQuit != nil {
		a.finishQuit()
	}
}
