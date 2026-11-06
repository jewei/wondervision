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
	busy := d == nil || (d != nil && d.Saving())
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
}
