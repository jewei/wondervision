package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/jewei/wondervision/internal/documents"
	"github.com/jewei/wondervision/internal/workspace"
)

type fileAction uint8

const (
	createFile fileAction = iota
	createFolder
	renameFile
	moveFile
	deleteFile
)

type fileRequest struct {
	action    fileAction
	source    string
	workspace *workspace.Workspace
	epoch     uint64
	versions  map[*documents.Document]uint64
}

func (a *App) actionSource() (string, bool) {
	if a.workspace == nil {
		return "", false
	}
	if a.row >= 0 && a.row < a.outline.Rows() {
		return a.outline.Item(a.row).Path, true
	}
	if d := a.active(); d != nil && workspace.Contains(a.workspace.Root, d.Path) {
		return d.Path, true
	}
	return "", false
}

func (a *App) requestFileAction(action fileAction) {
	if a.workspace == nil {
		a.notify(errors.New("open a folder before using workspace file actions"))
		return
	}
	if a.fileBusy || a.anySaving() || a.folderLoading {
		a.notify(errors.New("wait for the current file operation or save to finish"))
		return
	}
	source, selected := a.actionSource()
	if action >= renameFile && !selected {
		a.notify(errors.New("select a workspace file or folder first"))
		return
	}
	if a.openCancel != nil {
		a.openCancel()
	}
	a.openID++
	a.fileLoading = false
	r := &fileRequest{action: action, source: source, workspace: a.workspace, epoch: a.epoch, versions: make(map[*documents.Document]uint64)}
	for _, d := range a.docs {
		if source != "" && workspace.Contains(source, d.Path) {
			r.versions[d] = d.Buffer.Version()
		}
	}
	a.filePrompt = r
	switch action {
	case createFile:
		a.fileName = "new.md"
	case createFolder:
		a.fileName = "New folder"
	case renameFile:
		a.fileName = filepath.Base(source)
	case moveFile:
		a.fileName, _ = filepath.Rel(a.workspace.Root, source)
	case deleteFile:
		a.deletePrompt = true
		return
	}
	a.filePromptOpen = true
}

func (a *App) submitFilePrompt() {
	r := a.filePrompt
	if r == nil {
		return
	}
	destination := a.fileName
	if r.action == renameFile {
		if strings.ContainsAny(destination, "/\\") {
			a.notify(errors.New("use Move to change folders"))
			return
		}
		relative, _ := filepath.Rel(r.workspace.Root, r.source)
		destination = filepath.Join(filepath.Dir(relative), destination)
	}
	a.filePromptOpen = false
	a.filePrompt = nil
	a.executeFileAction(r, destination)
}

func (a *App) executeFileAction(r *fileRequest, destination string) {
	if a.fileBusy || a.anySaving() || a.workspace != r.workspace || a.epoch != r.epoch {
		a.notify(errors.New("workspace changed; choose the file action again"))
		return
	}
	if r.action == deleteFile {
		affected := 0
		for _, d := range a.docs {
			if workspace.Contains(r.source, d.Path) {
				affected++
				if version, known := r.versions[d]; !known || version != d.Buffer.Version() {
					a.notify(errors.New("an open buffer changed; review it before deleting"))
					return
				}
			}
		}
		if affected != len(r.versions) {
			a.notify(errors.New("open tabs changed; review them before deleting"))
			return
		}
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.fileCancel = cancel
	a.fileBusy = true
	source, _ := filepath.Rel(r.workspace.Root, r.source)
	if !a.enqueue(func() func() {
		var change workspace.Change
		var err error
		switch r.action {
		case createFile:
			change, err = r.workspace.Create(ctx, destination, false)
		case createFolder:
			change, err = r.workspace.Create(ctx, destination, true)
		case renameFile, moveFile:
			change, err = r.workspace.Move(ctx, source, destination)
		case deleteFile:
			change, err = r.workspace.Delete(ctx, source)
		}
		return func() {
			a.fileBusy = false
			a.fileCancel = nil
			cancel()
			defer a.settleQuit()
			if err != nil {
				if errors.Is(err, context.Canceled) && r.action == deleteFile {
					a.notify(errors.New("deletion cancelled; some entries may already have been removed"))
				} else {
					a.notify(fmt.Errorf("file action: %w", err))
				}
				a.refreshExplorer()
				return
			}
			if r.action == deleteFile {
				for d, version := range r.versions {
					if !a.hasDocument(d) {
						continue
					}
					if d.Buffer.Version() != version {
						a.notify(errors.New("file deleted; later in-memory edits were retained"))
						a.checkDisk(d)
					} else {
						a.removeDocument(d)
					}
				}
			} else if change.OldPath != "" {
				for _, d := range a.docs {
					if workspace.Contains(change.OldPath, d.Path) {
						relative, _ := filepath.Rel(change.OldPath, d.Path)
						path := change.NewPath
						if relative != "." {
							path = filepath.Join(path, relative)
						}
						oldPath := d.Path
						a.stopWatch(d)
						d.Relocate(path, r.workspace.Root, d.Info)
						delete(a.external, d)
						delete(a.checking, d)
						delete(a.checkAgain, d)
						a.recoverySaved(d, oldPath)
						if d.Dirty() && !documents.Sensitive(d.Path) {
							a.writeRecovery(d, a.ctx)
						}
						a.startWatch(d)
						if a.preview != nil && a.preview.doc == d {
							a.releasePreview()
						}
					}
				}
			}
			a.prefs.ActiveFile = a.activePath()
			a.remember()
			a.refreshExplorer()
			if r.action == createFile {
				a.openFile(change.NewPath, r.workspace.Root, "")
			}
		}
	}) {
		a.fileBusy = false
		cancel()
	}
}

func (a *App) refreshExplorer() {
	if a.workspace == nil {
		return
	}
	w, epoch, ctx := a.workspace, a.epoch, a.wsCtx
	a.enqueue(func() func() {
		entries, err := w.List(ctx, w.Root)
		return func() {
			if a.workspace != w || a.epoch != epoch || ctx.Err() != nil {
				return
			}
			if err != nil {
				a.notify(err)
				return
			}
			a.dirs = map[string]*directory{w.Root: {entries: entries, loaded: true}}
			a.outline = ui.OutlineState[workspace.Entry]{}
			a.row = -1
		}
	})
}

func (a *App) fileDialogs(c *ui.Context) {
	if r := a.filePrompt; r != nil {
		title := "New File"
		switch r.action {
		case createFolder:
			title = "New Folder"
		case renameFile:
			title = "Rename"
		case moveFile:
			title = "Move"
		}
		ui.Modal(c, &a.filePromptOpen, func() {
			ui.Column(c).Width(480).Gap(12).Children(func() {
				ui.Text(c, title).Bold()
				if r.source != "" {
					ui.Text(c, r.source).TextColor(c.Theme().TextMuted)
				}
				ui.TextInput(c, &a.fileName).Label("Path").AutoFocus().OnSubmit(a.submitFilePrompt)
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					ui.Button(c, "Cancel").OnClick(func() { a.filePromptOpen = false })
					ui.PrimaryButton(c, title).OnClick(a.submitFilePrompt)
				})
			})
		})
		if r.action != deleteFile && !a.filePromptOpen {
			a.filePrompt = nil
		}
		if r.action == deleteFile {
			message := r.source + "\nThis permanently deletes the selected path and its contents. Unsaved buffers under this path will be discarded."
			switch ui.AlertDialog(c, &a.deletePrompt, "Delete permanently?", message, "Delete permanently", "Cancel") {
			case 0:
				a.filePrompt = nil
				a.executeFileAction(r, "")
			case 1, ui.AlertDismissed:
				a.filePrompt = nil
			}
		}
	}
}
