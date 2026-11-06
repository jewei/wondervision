package app

import "github.com/egoist/mygo/ui"

func (a *App) fileMenu(m *ui.Menu) {
	if m.Item("Open File").Shortcut(ui.Cmd, ui.KeyO).Chosen() {
		a.choose(false)
	}
	if m.Item("Open Folder").Shortcut(ui.Cmd|ui.Shift, ui.KeyO).Chosen() {
		a.choose(true)
	}
	m.Separator()
	d := a.active()
	busy := a.fileBusy || d == nil || (d != nil && d.Saving())
	if m.Item("Save").Shortcut(ui.Cmd, ui.KeyS).Disabled(busy).Chosen() {
		a.saveDocument(d, nil)
	}
	if m.Item("Save As").Shortcut(ui.Cmd|ui.Shift, ui.KeyS).Disabled(busy).Chosen() {
		a.saveAs(d, nil)
	}
	if m.Item("Reload from Disk").Disabled(busy).Chosen() {
		a.requestReload(d)
	}
	if m.Item("Compare Disk Version").Disabled(d == nil).Chosen() {
		a.compareDisk(d)
	}
	if m.Item("Close Tab").Shortcut(ui.Cmd, ui.KeyW).Disabled(busy).Chosen() {
		a.requestClose(d)
	}
	m.Separator()
	unavailable := a.workspace == nil || a.fileBusy || a.anySaving() || a.folderLoading
	if m.Item("New File").Shortcut(ui.Cmd, ui.KeyN).Disabled(unavailable).Chosen() {
		a.requestFileAction(createFile)
	}
	if m.Item("New Folder").Disabled(unavailable).Chosen() {
		a.requestFileAction(createFolder)
	}
	_, selected := a.actionSource()
	if m.Item("Rename").Shortcut(0, ui.KeyF2).Disabled(unavailable || !selected).Chosen() {
		a.requestFileAction(renameFile)
	}
	if m.Item("Move").Disabled(unavailable || !selected).Chosen() {
		a.requestFileAction(moveFile)
	}
	if m.Item("Delete Permanently").Disabled(unavailable || !selected).Chosen() {
		a.requestFileAction(deleteFile)
	}
}
