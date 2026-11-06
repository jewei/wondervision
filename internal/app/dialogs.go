package app

import (
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *App) editingDialogs(c *ui.Context) {
	switch ui.AlertDialog(c, &a.askQuit, "Discard unsaved edits?", "Save your changes before closing WonderVision.", "Discard and Quit", "Keep Editing", "Save and Quit") {
	case 0:
		a.allowQuit = true
		if !a.deferQuit() {
			if a.finishQuit != nil {
				a.finishQuit()
			} else {
				mygo.App.Quit()
			}
		}
	case 2:
		a.saveAndQuit()
	}
	if d := a.closeDoc; d != nil {
		switch ui.AlertDialog(c, &a.askClose, "Save changes to "+filepath.Base(d.Path)+"?", "Your changes have not been saved.", "Discard", "Cancel", "Save") {
		case 0:
			a.closeDoc = nil
			a.removeDocument(d)
		case 1, ui.AlertDismissed:
			a.closeDoc = nil
		case 2:
			a.closeDoc = nil
			a.saveDocument(d, func() { a.removeDocument(d) })
		}
	}
	if r := a.replaceSave; r != nil {
		switch ui.AlertDialog(c, &a.askReplace, "Replace existing file?", r.path, "Cancel", "Replace") {
		case 0, ui.AlertDismissed:
			a.replaceSave = nil
		case 1:
			a.confirmReplaceSave()
		}
	}
	if d := a.reloadDoc; d != nil {
		switch ui.AlertDialog(c, &a.askReload, "Discard edits and reload?", d.Path, "Cancel", "Reload from Disk") {
		case 0, ui.AlertDismissed:
			a.reloadDoc = nil
		case 1:
			a.reloadDoc = nil
			a.reloadDocument(d)
		}
	}
	if d := a.overwriteDoc; d != nil {
		switch ui.AlertDialog(c, &a.askOverwrite, "Overwrite disk changes?", "Replace the last observed disk version with this buffer. A newer disk change will still block the save.", "Cancel", "Overwrite Disk") {
		case 0, ui.AlertDismissed:
			a.overwriteDoc = nil
		case 1:
			a.overwriteDoc = nil
			a.overwriteDocument(d)
		}
	}
	a.showDiskComparison(c)
}
