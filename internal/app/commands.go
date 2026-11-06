package app

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/jewei/wondervision/internal/documents"
)

// commandID names one command. The commands table is the single source of
// truth for titles, shortcuts and availability: shortcuts, the File menu,
// toolbar buttons and the command palette all refer to commands by ID.
type commandID int

const (
	cmdOpenFile commandID = iota
	cmdOpenFolder
	cmdQuickOpen
	cmdSearchWorkspace
	cmdCommandPalette
	cmdSave
	cmdSaveAs
	cmdReloadFromDisk
	cmdCompareDisk
	cmdCloseTab
	cmdNextTab
	cmdNewFile
	cmdNewFolder
	cmdRename
	cmdMove
	cmdDelete
	cmdToggleSidebar
	cmdRefreshExplorer
	cmdEditView
	cmdPreviewView
	cmdSplitView
	cmdToggleWordWrap
	cmdGoToLine
	cmdReveal
	cmdSettings
	commandCount
)

type shortcut struct {
	mods ui.Modifiers
	key  ui.Key
}

type command struct {
	title    string
	category string
	keywords []string
	key      shortcut // zero: no shortcut
	hidden   bool     // not listed in the palette (the palette's own command)
	// available reports whether the command can run now, and why not.
	available func(a *App) (bool, string)
	run       func(a *App, c *ui.Context)
}

// commands is indexed by commandID; a test rejects empty slots.
var commands [commandCount]command

func init() {
	always := func(*App) (bool, string) { return true, "" }
	needDoc := func(a *App) (bool, string) {
		if a.active() == nil {
			return false, "Open a file first"
		}
		return true, ""
	}
	needIdleDoc := func(a *App) (bool, string) {
		d := a.active()
		switch {
		case d == nil:
			return false, "Open a file first"
		case d.Saving() || a.fileBusy:
			return false, "Wait for the current operation to finish"
		}
		return true, ""
	}
	needMarkdown := func(a *App) (bool, string) {
		if d := a.active(); d == nil || !d.Markdown {
			return false, "Open a Markdown file first"
		}
		return true, ""
	}
	canChoose := func(a *App) (bool, string) {
		if a.restoring || a.dialogOpen || a.fileBusy {
			return false, "Wait for the current operation to finish"
		}
		return true, ""
	}
	needWorkspace := func(a *App) (bool, string) {
		if a.workspace == nil {
			return false, "Open a folder first"
		}
		return true, ""
	}
	fileActions := func(a *App) (bool, string) {
		if ok, why := needWorkspace(a); !ok {
			return ok, why
		}
		if a.fileBusy || a.anySaving() || a.folderLoading {
			return false, "Wait for the current operation to finish"
		}
		return true, ""
	}
	selection := func(a *App) (bool, string) {
		if ok, why := fileActions(a); !ok {
			return ok, why
		}
		if _, selected := a.actionSource(); !selected {
			return false, "Select a file or folder first"
		}
		return true, ""
	}
	mode := func(m documents.Mode) func(a *App, c *ui.Context) {
		return func(a *App, c *ui.Context) { a.setMode(a.active(), m) }
	}
	cmd := ui.Cmd
	commands = [commandCount]command{
		cmdOpenFile:        {title: "Open File", category: "File", key: shortcut{cmd, ui.KeyO}, available: canChoose, run: func(a *App, c *ui.Context) { a.choose(false) }},
		cmdOpenFolder:      {title: "Open Folder", category: "File", keywords: []string{"workspace", "project"}, key: shortcut{cmd | ui.Shift, ui.KeyO}, available: canChoose, run: func(a *App, c *ui.Context) { a.choose(true) }},
		cmdQuickOpen:       {title: "Quick Open", category: "Go", keywords: []string{"find file", "go to file"}, key: shortcut{cmd, ui.KeyP}, available: always, run: func(a *App, c *ui.Context) { a.showOverlay(quickOpenOverlay) }},
		cmdSearchWorkspace: {title: "Search in Workspace", category: "Go", keywords: []string{"find in files", "grep", "text"}, key: shortcut{cmd | ui.Shift, ui.KeyF}, available: needWorkspace, run: func(a *App, c *ui.Context) { a.showOverlay(searchOverlay) }},
		cmdCommandPalette:  {title: "Command Palette", category: "Go", key: shortcut{cmd | ui.Shift, ui.KeyP}, hidden: true, available: always, run: func(a *App, c *ui.Context) { a.showOverlay(paletteOverlay) }},
		cmdSave:            {title: "Save", category: "File", key: shortcut{cmd, ui.KeyS}, available: needIdleDoc, run: func(a *App, c *ui.Context) { a.saveDocument(a.active(), nil) }},
		cmdSaveAs:          {title: "Save As", category: "File", key: shortcut{cmd | ui.Shift, ui.KeyS}, available: needIdleDoc, run: func(a *App, c *ui.Context) { a.saveAs(a.active(), nil) }},
		cmdReloadFromDisk:  {title: "Reload from Disk", category: "File", keywords: []string{"revert"}, available: needIdleDoc, run: func(a *App, c *ui.Context) { a.requestReload(a.active()) }},
		cmdCompareDisk:     {title: "Compare Disk Version", category: "File", keywords: []string{"diff"}, available: needDoc, run: func(a *App, c *ui.Context) { a.compareDisk(a.active()) }},
		cmdCloseTab:        {title: "Close Tab", category: "File", key: shortcut{cmd, ui.KeyW}, available: needIdleDoc, run: func(a *App, c *ui.Context) { a.requestClose(a.active()) }},
		cmdNextTab: {title: "Next Tab", category: "View", key: shortcut{cmd, ui.KeyTab}, available: func(a *App) (bool, string) {
			if len(a.docs) < 2 {
				return false, "Open another file first"
			}
			return true, ""
		}, run: func(a *App, c *ui.Context) { a.switchTab((a.tab + 1) % len(a.docs)) }},
		cmdNewFile:       {title: "New File", category: "File", key: shortcut{cmd, ui.KeyN}, available: fileActions, run: func(a *App, c *ui.Context) { a.requestFileAction(createFile) }},
		cmdNewFolder:     {title: "New Folder", category: "File", available: fileActions, run: func(a *App, c *ui.Context) { a.requestFileAction(createFolder) }},
		cmdRename:        {title: "Rename", category: "File", key: shortcut{0, ui.KeyF2}, available: selection, run: func(a *App, c *ui.Context) { a.requestFileAction(renameFile) }},
		cmdMove:          {title: "Move", category: "File", available: selection, run: func(a *App, c *ui.Context) { a.requestFileAction(moveFile) }},
		cmdDelete:        {title: "Delete Permanently", category: "File", keywords: []string{"delete", "remove"}, available: selection, run: func(a *App, c *ui.Context) { a.requestFileAction(deleteFile) }},
		cmdToggleSidebar: {title: "Toggle Sidebar", category: "View", keywords: []string{"explorer", "hide", "show"}, key: shortcut{cmd, ui.KeyB}, available: always, run: func(a *App, c *ui.Context) { a.prefs.SidebarHidden = !a.prefs.SidebarHidden; a.remember() }},
		cmdRefreshExplorer: {title: "Refresh Explorer", category: "View", keywords: []string{"reload", "rescan"}, available: func(a *App) (bool, string) {
			if ok, why := needWorkspace(a); !ok {
				return ok, why
			}
			if a.folderLoading {
				return false, "Wait for the folder to finish opening"
			}
			return true, ""
		}, run: func(a *App, c *ui.Context) { a.openFolder(a.workspace.Root) }},
		cmdEditView:       {title: "Edit View", category: "View", keywords: []string{"markdown", "source"}, available: needMarkdown, run: mode(documents.Edit)},
		cmdPreviewView:    {title: "Preview View", category: "View", keywords: []string{"markdown", "render"}, available: needMarkdown, run: mode(documents.Preview)},
		cmdSplitView:      {title: "Split View", category: "View", keywords: []string{"markdown", "side by side"}, available: needMarkdown, run: mode(documents.Split)},
		cmdToggleWordWrap: {title: "Toggle Word Wrap", category: "View", keywords: []string{"soft wrap", "lines"}, available: needDoc, run: func(a *App, c *ui.Context) { a.active().Editor.Run(editor.ToggleSoftWrap) }},
		cmdGoToLine:       {title: "Go to Line", category: "Go", keywords: []string{"jump", "line number"}, available: needDoc, run: func(a *App, c *ui.Context) { a.active().Editor.Run(editor.GoToLine) }},
		cmdReveal: {title: "Reveal in File Manager", category: "File", keywords: []string{"show in folder", "finder", "explorer"}, available: func(a *App) (bool, string) {
			if a.active() == nil && a.workspace == nil {
				return false, "Open a file or folder first"
			}
			return true, ""
		}, run: func(a *App, c *ui.Context) { c.OpenURL(a.revealURL()) }},
		cmdSettings: {title: "Settings", category: "App", keywords: []string{"preferences"}, available: func(*App) (bool, string) {
			return false, "No settings yet"
		}, run: func(*App, *ui.Context) {}},
	}
}

// invoke runs a command if it is available now.
func (a *App) invoke(c *ui.Context, id commandID) {
	cmd := commands[id]
	if ok, _ := cmd.available(a); ok {
		cmd.run(a, c)
	}
}

// bindShortcuts registers every command shortcut. It is the only place
// commands get keyboard shortcuts (internal/guardrails enforces this).
func (a *App) bindShortcuts(c *ui.Context) {
	for id, cmd := range commands {
		if cmd.key.key != ui.KeyUnknown {
			c.OnShortcut(cmd.key.mods, cmd.key.key, func() { a.invoke(c, commandID(id)) })
		}
	}
}

// toolbarButton builds a button for a command, with label instead of its
// title when label isn't empty.
func (a *App) toolbarButton(c *ui.Context, id commandID, label string) ui.Element {
	cmd := commands[id]
	if label == "" {
		label = cmd.title
	}
	ok, _ := cmd.available(a)
	return ui.Button(c, label).Disabled(!ok).OnClick(func() { a.invoke(c, id) })
}

// menuItem adds a command to a menu.
func (a *App) menuItem(c *ui.Context, m *ui.Menu, id commandID) {
	cmd := commands[id]
	item := m.Item(cmd.title)
	if cmd.key.key != ui.KeyUnknown {
		item = item.Shortcut(cmd.key.mods, cmd.key.key)
	}
	ok, _ := cmd.available(a)
	if item.Disabled(!ok).Chosen() {
		a.invoke(c, id)
	}
}

// fileMenuSections lists the File menu by command, with separators between
// sections.
var fileMenuSections = [][]commandID{
	{cmdOpenFile, cmdOpenFolder, cmdQuickOpen, cmdSearchWorkspace},
	{cmdSave, cmdSaveAs, cmdReloadFromDisk, cmdCompareDisk, cmdCloseTab},
	{cmdNewFile, cmdNewFolder, cmdRename, cmdMove, cmdDelete},
}

// revealURL is a file:// URL for the folder holding the active file, or the
// workspace root.
func (a *App) revealURL() string {
	dir := ""
	if d := a.active(); d != nil {
		dir = filepath.Dir(d.Path)
	} else if a.workspace != nil {
		dir = a.workspace.Root
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(dir)}).String()
}

var keyNames = map[ui.Key]string{
	ui.KeyA: "A", ui.KeyB: "B", ui.KeyF: "F", ui.KeyN: "N", ui.KeyO: "O", ui.KeyP: "P",
	ui.KeyS: "S", ui.KeyW: "W", ui.KeyF2: "F2", ui.KeyTab: "Tab",
}

// label formats a shortcut the platform's way: ⇧⌘P on macOS, Ctrl+Shift+P
// elsewhere. It returns "" for no shortcut.
func (s shortcut) label() string {
	if s.key == ui.KeyUnknown {
		return ""
	}
	name := keyNames[s.key]
	if runtime.GOOS == "darwin" {
		var b strings.Builder
		for _, m := range []struct {
			mod  ui.Modifiers
			mark string
		}{{ui.Ctrl, "⌃"}, {ui.Alt, "⌥"}, {ui.Shift, "⇧"}, {ui.Super, "⌘"}} {
			if s.mods&m.mod != 0 {
				b.WriteString(m.mark)
			}
		}
		return b.String() + name
	}
	var parts []string
	for _, m := range []struct {
		mod  ui.Modifiers
		name string
	}{{ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}, {ui.Super, "Super"}} {
		if s.mods&m.mod != 0 {
			parts = append(parts, m.name)
		}
	}
	return strings.Join(append(parts, name), "+")
}
