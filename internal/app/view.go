package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/jewei/wondervision/internal/documents"
	"github.com/jewei/wondervision/internal/workspace"
)

func (a *App) view(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyN, func() { a.requestFileAction(createFile) })
	c.OnShortcut(0, ui.KeyF2, func() { a.requestFileAction(renameFile) })
	c.OnShortcut(ui.Cmd, ui.KeyS, func() { a.saveDocument(a.active(), nil) })
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyS, func() { a.saveAs(a.active(), nil) })
	c.OnShortcut(ui.Cmd, ui.KeyW, func() { a.requestClose(a.active()) })
	c.OnShortcut(ui.Cmd, ui.KeyO, func() { a.choose(false) })
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyO, func() { a.choose(true) })
	c.OnShortcut(ui.Cmd, ui.KeyB, func() { a.prefs.SidebarHidden = !a.prefs.SidebarHidden; a.remember() })
	c.OnShortcut(ui.Cmd, ui.KeyTab, func() {
		if len(a.docs) > 0 {
			a.switchTab((a.tab + 1) % len(a.docs))
		}
	})
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Padding(8, 16).Gap(12).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, "WonderVision").Bold()
			ui.PopoverMenuButton(c, "File", a.fileMenu)
			ui.Text(c, a.title()).Grow(1).SingleLine().TextColor(t.TextMuted)
			if len(a.recoveries) > 0 {
				ui.Button(c, "Recover drafts").OnClick(func() { a.recoveryPanel = true })
			}
			ui.Button(c, "Save").Disabled(a.active() == nil || (a.active() != nil && (a.active().Saving() || a.fileBusy))).OnClick(func() { a.saveDocument(a.active(), nil) })
			ui.Button(c, "Open File").Disabled(a.restoring || a.dialogOpen || a.fileBusy).OnClick(func() { a.choose(false) })
			ui.Button(c, "Open Folder").Disabled(a.restoring || a.dialogOpen || a.fileBusy).OnClick(func() { a.choose(true) })
			label := "Hide Sidebar"
			if a.prefs.SidebarHidden {
				label = "Show Sidebar"
			}
			ui.Button(c, label).OnClick(func() { a.prefs.SidebarHidden = !a.prefs.SidebarHidden; a.remember() })
		})
		if a.err != "" {
			ui.Row(c).Padding(8, 16).Gap(12).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
				ui.Text(c, a.err).Grow(1)
				ui.Button(c, "Dismiss").OnClick(func() { a.err = "" })
			})
		}
		if a.prefs.SidebarHidden {
			ui.Column(c).Grow(1).Children(func() { a.documentView(c) })
		} else {
			if ui.Split(c, &a.prefs.SidebarWidth, func() { a.explorer(c) }, func() { a.documentView(c) }).Grow(1).Changed() {
				a.remember()
			}
		}
		ui.Row(c).Padding(6, 16).Gap(12).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
			status := "Ready"
			if a.restoring {
				status = "Restoring…"
			} else if a.folderLoading {
				status = "Opening folder…"
			} else if a.fileLoading {
				status = "Opening file…"
			}
			if d := a.active(); d != nil {
				kind := "Plain text"
				if l := d.Buffer.Language(); l != nil {
					kind = l.Name
				}
				encoding := "UTF-8"
				if d.BOM {
					encoding = "UTF-8 BOM"
				}
				state := "Saved"
				if d.Dirty() {
					state = "Unsaved"
				}
				if d.Saving() {
					state = "Saving…"
				}
				if documents.Sensitive(d.Path) {
					state += " · Recovery off"
				}
				newline := "LF"
				if d.Buffer.LineEnding() == "\r\n" {
					newline = "CRLF"
				}
				status = fmt.Sprintf("%s | %s | %s | %s", kind, encoding, newline, state)
				ui.Text(c, status).FontSize(12).TextColor(t.TextMuted).Grow(1)
				p := d.Editor.CursorPos()
				ui.Textf(c, "Ln %d, Col %d", p.Line+1, p.Col+1).FontSize(12).TextColor(t.TextMuted)
			} else {
				ui.Text(c, status).FontSize(12).TextColor(t.TextMuted)
			}
		})
	})
	a.editingDialogs(c)
	a.fileDialogs(c)
	a.observeEdits()
}

func (a *App) explorer(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Surface).Children(func() {
		ui.Row(c).Padding(12).Children(func() {
			ui.Text(c, "EXPLORER").FontSize(11).TextColor(t.TextMuted).Grow(1)
			ui.Button(c, "Refresh").Disabled(a.workspace == nil || a.folderLoading).OnClick(func() {
				if a.workspace != nil {
					a.openFolder(a.workspace.Root)
				}
			})
		})
		if a.workspace == nil {
			ui.Text(c, "No folder open").Padding(12).TextColor(t.TextMuted)
			return
		}
		root := a.dirs[a.workspace.Root]
		if len(root.entries) == 0 {
			ui.Text(c, "Empty folder").Padding(12).TextColor(t.TextMuted)
		}
		a.outline.List.Selected = &a.row
		a.outline.List.Label = func(i int) string { return a.outline.Item(i).Name }
		tree := ui.Outline(c.Key("explorer"), &a.outline, root.entries, func(e workspace.Entry) []workspace.Entry {
			if !e.Directory || e.Symlink {
				return nil
			}
			if d := a.dirs[e.Path]; d != nil && d.loaded {
				return d.entries
			}
			return []workspace.Entry{} // disclosure before children have been read
		}, func(e workspace.Entry) {
			name := e.Name
			if e.Symlink {
				name += " ↗"
			}
			if d := a.dirs[e.Path]; d != nil && d.loading {
				name += " …"
			}
			ui.Text(c, name).SingleLine().Tooltip(e.Path)
		}).Grow(1).FillWidth()
		if (tree.Changed() || tree.Submitted()) && a.row >= 0 && a.row < a.outline.Rows() {
			e := a.outline.Item(a.row)
			if !e.Directory {
				a.openFile(e.Path, a.workspace.Root, "")
			}
		}
		for e := range a.outline.Open.All() {
			if e.Directory && !e.Symlink {
				a.loadDirectory(e.Path)
			}
		}
	})
}

func (a *App) documentView(c *ui.Context) {
	ui.Column(c).Fill().Clip().Children(func() {
		if len(a.docs) == 0 {
			ui.Column(c).Grow(1).Center().Children(func() { ui.Text(c, "Open a file or folder to begin.").TextColor(c.Theme().TextMuted) })
			return
		}
		old := a.tab
		ui.ScrollHorizontal(c.Key("tab-scroll")).FillWidth().Height(40).Children(func() {
			tabs := ui.TabsBase(c, &a.tab, len(a.docs))
			tabs.List.Gap(2).Children(func() {
				for i, d := range a.docs {
					tab := tabs.Tab(i).Padding(6, 10).Tooltip(d.Path).Label(baseName(d.Path))
					if i == a.tab {
						tab.Background(c.Theme().Surface)
					}
					tab.Children(func() {
						name := baseName(d.Path)
						if d.Dirty() {
							name += " •"
						}
						ui.Text(c, name)
						ui.Button(c, "×").Label("Close "+baseName(d.Path)).Padding(1, 4).Disabled(d.Saving()).OnClick(func() { a.requestClose(d) })
					})
				}
			})
		})
		if a.tab != old {
			a.switchTab(a.tab)
		}
		d := a.active()
		if a.fileBusy {
			ui.Row(c).Padding(8, 12).Gap(8).Children(func() {
				ui.Text(c, "File operation in progress…").Grow(1)
				ui.Button(c, "Cancel operation").OnClick(func() {
					if a.fileCancel != nil {
						a.fileCancel()
					}
				})
			})
		}
		if disk, changed := a.external[d]; changed {
			ui.Row(c).Padding(8, 12).Gap(8).Background(c.Theme().Surface).Children(func() {
				label := "File changed on disk"
				if !disk.Fingerprint.Exists {
					label = "File deleted on disk"
				}
				ui.Text(c, label).Grow(1)
				ui.Button(c, "Reload").Disabled(d.Saving()).OnClick(func() { a.requestReload(d) })
				ui.Button(c, "Compare").OnClick(func() { a.compareDisk(d) })
				ui.Button(c, "Save As").Disabled(d.Saving()).OnClick(func() { a.saveAs(d, nil) })
				ui.Button(c, "Overwrite").Disabled(d.Saving()).OnClick(func() { a.requestOverwrite(d) })
			})
		}
		if d.Markdown {
			ui.Row(c).Padding(6, 12).Gap(6).Children(func() {
				for _, m := range []struct {
					label string
					mode  documents.Mode
				}{{"Edit", documents.Edit}, {"Preview", documents.Preview}, {"Split", documents.Split}} {
					ui.Button(c, m.label).Disabled(d.Mode == m.mode).OnClick(func() { a.setMode(d, m.mode) })
				}
				if d.Large {
					ui.Text(c, "Large Markdown: preview may be expensive.").FontSize(12).TextColor(c.Theme().TextMuted)
				}
			})
		}
		switch d.Mode {
		case documents.Preview:
			a.markdownView(c, d)
		case documents.Split:
			ui.SplitPercent(c, &a.splitPercent, func() { a.editorView(c, d) }, func() { a.markdownView(c, d) }).Grow(1)
		default:
			a.releasePreview()
			a.editorView(c, d)
		}
	})
}

func (a *App) editorView(c *ui.Context, d *documents.Document) {
	d.Editor.ReadOnly = a.fileBusy
	editor.View(c.Key(d.Path), d.Editor).Label("Editor").AutoFocus().Grow(1).FillWidth()
}

func (a *App) setMode(d *documents.Document, mode documents.Mode) {
	d.Mode = mode
	a.prefs.MarkdownMode = int(mode)
	if mode == documents.Edit {
		a.releasePreview()
	}
	a.remember()
}
