package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestCommandTableIsCompleteAndShortcutsAreUnique(t *testing.T) {
	titles := make(map[string]commandID)
	keys := make(map[shortcut]commandID)
	for i, cmd := range commands {
		id := commandID(i)
		if cmd.title == "" || cmd.category == "" || cmd.available == nil || cmd.run == nil {
			t.Fatalf("command %d is incomplete: %+v", id, cmd)
		}
		if other, ok := titles[cmd.title]; ok {
			t.Errorf("commands %d and %d share the title %q", other, id, cmd.title)
		}
		titles[cmd.title] = id
		if cmd.key.key == ui.KeyUnknown {
			continue
		}
		if other, ok := keys[cmd.key]; ok {
			t.Errorf("commands %q and %q share a shortcut", commands[other].title, cmd.title)
		}
		keys[cmd.key] = id
		if _, ok := keyNames[cmd.key.key]; !ok {
			t.Errorf("shortcut of %q has no label in keyNames", cmd.title)
		}
	}
	// Spec §5: the initial palette commands, by title.
	for _, title := range []string{"Open File", "Open Folder", "Quick Open", "Search in Workspace", "New File", "New Folder", "Rename", "Move", "Delete Permanently", "Save", "Close Tab", "Toggle Sidebar", "Refresh Explorer", "Edit View", "Preview View", "Split View", "Toggle Word Wrap", "Go to Line", "Reveal in File Manager", "Settings"} {
		if _, ok := titles[title]; !ok {
			t.Errorf("missing command %q", title)
		}
	}
}

func TestPaletteRanksTitlesThenKeywords(t *testing.T) {
	a := newApp()
	t.Cleanup(a.cancel)
	entries := a.paletteEntries("toggle side")
	if len(entries) == 0 || entries[0].id != cmdToggleSidebar {
		t.Fatalf("toggle side -> %v", entries)
	}
	entries = a.paletteEntries("grep")
	if len(entries) == 0 || entries[0].id != cmdSearchWorkspace {
		t.Fatalf("keyword grep -> %v", entries)
	}
	for _, e := range a.paletteEntries("") {
		if e.id == cmdCommandPalette {
			t.Fatal("the palette lists itself")
		}
	}
}

func TestPaletteRunsToggleSidebarWithKeyboardAndEscapeCloses(t *testing.T) {
	a := newApp()
	t.Cleanup(a.cancel)
	tt := ui.NewTester(a.view, 1040, 720)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyP)
	tt.Frame() // the virtualized list lays out its rows on the next frame
	if a.overlay.kind != paletteOverlay || !tt.HasText("Quick Open") || !tt.HasText("Ctrl+P") {
		t.Fatalf("Ctrl+Shift+P did not open the palette: kind=%d", a.overlay.kind)
	}
	tt.Type("toggle side")
	tt.Key(0, ui.KeyEnter)
	if !a.prefs.SidebarHidden || a.overlay.kind != noOverlay {
		t.Fatalf("Enter did not run Toggle Sidebar and close: hidden=%v kind=%d", a.prefs.SidebarHidden, a.overlay.kind)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyP)
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.overlay.kind != noOverlay {
		t.Fatal("Escape did not close the palette")
	}
}

func TestQuickOpenFindsNestedFileByPartialNameAndRemembersIt(t *testing.T) {
	a := newApp()
	t.Cleanup(a.cancel)
	root := fixture(t)
	a.openFolder(root)
	drain(t, a)
	tt := ui.NewTester(a.view, 1040, 720)
	tt.Key(ui.Cmd, ui.KeyP)
	if a.overlay.kind != quickOpenOverlay {
		t.Fatal("Ctrl+P did not open Quick Open")
	}
	if err := a.files.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	tt.Type("nxt")
	tt.Frame()
	if !tt.HasText("next.md") {
		t.Fatalf("nested file not listed: %v", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	drain(t, a)
	want := filepath.Join(root, "docs", "next.md")
	if d := a.active(); d == nil || d.Path != want {
		t.Fatalf("opened %v, want %s", a.active(), want)
	}
	if len(a.prefs.Recent) == 0 || a.prefs.Recent[0] != want {
		t.Fatalf("opened file not recorded as recent: %v", a.prefs.Recent)
	}
}

// pump runs posted closures until cond holds.
func pump(t *testing.T, posted chan func(), cond func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for !cond() {
		select {
		case f := <-posted:
			f()
		case <-deadline:
			t.Fatal("background work did not finish")
		}
	}
}

func TestContentSearchOpensTheMatchingLineAndSearchesUnsavedEdits(t *testing.T) {
	a := newApp()
	t.Cleanup(a.cancel)
	posted := make(chan func(), 100)
	a.post = func(f func()) { posted <- f }
	root := fixture(t)
	a.openFolder(root)
	drain(t, a)
	a.openFile(filepath.Join(root, "todo.md"), root, "")
	drain(t, a)
	a.active().Buffer.Set("# TODO\nunsaved needle here\n")

	tt := ui.NewTester(a.view, 1040, 720)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyF)
	if a.overlay.kind != searchOverlay {
		t.Fatal("Ctrl+Shift+F did not open search")
	}
	tt.Type("needle")
	tt.Key(0, ui.KeyEnter)
	run := a.overlay.search
	if run == nil {
		t.Fatal("Enter did not start a search")
	}
	pump(t, posted, func() bool { return !run.running })
	if len(run.hits) != 1 || run.hits[0].Path != "todo.md" || run.hits[0].Line != 2 {
		t.Fatalf("unsaved buffer not searched: %+v", run.hits)
	}

	tt.Key(0, ui.KeyEscape) // a modal overlay holds the keyboard until closed
	tt.Frame()
	a.openFile(filepath.Join(root, "report.md"), root, "")
	drain(t, a)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyF)
	tt.Type("Review")
	tt.Key(0, ui.KeyEnter)
	run = a.overlay.search
	pump(t, posted, func() bool { return !run.running })
	tt.Frame()
	if len(run.hits) != 1 || !tt.HasText("report.md:3") {
		t.Fatalf("hits=%+v texts=%v", run.hits, tt.Texts())
	}
	tt.Key(0, ui.KeyEnter) // same query: open the selected hit
	drain(t, a)
	d := a.active()
	if d == nil || !strings.HasSuffix(d.Path, "report.md") || d.Editor.CursorPos().Line != 2 || d.Editor.CursorPos().Col != 6 {
		t.Fatalf("hit not opened at line 3, byte column 6: %v %+v", d.Path, d.Editor.CursorPos())
	}
}

func TestSupersededSearchResultsNeverShow(t *testing.T) {
	a := newApp()
	t.Cleanup(a.cancel)
	posted := make(chan func(), 100)
	a.post = func(f func()) { posted <- f }
	root := fixture(t)
	a.openFolder(root)
	drain(t, a)
	a.showOverlay(searchOverlay)
	a.overlay.query = "Report"
	a.startTextSearch()
	first := a.overlay.search
	// Let the first run finish but keep its results queued, as when the UI
	// thread is busy: one batch for report.md, then its end.
	deadline := time.After(5 * time.Second)
	for len(posted) < 2 {
		select {
		case <-deadline:
			t.Fatal("first search did not finish")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	a.overlay.query = "TODO"
	a.startTextSearch() // supersedes the first run while its results are in flight
	second := a.overlay.search
	pump(t, posted, func() bool { return !second.running && len(posted) == 0 })
	if a.overlay.search != second {
		t.Fatal("the current run was replaced by a stale one")
	}
	if len(second.hits) == 0 {
		t.Fatal("the current run found nothing")
	}
	for _, h := range second.hits {
		if h.Path != "todo.md" {
			t.Fatalf("a superseded run's hit is visible: %+v", h)
		}
	}
	if len(first.hits) == 0 {
		t.Fatal("the first run's late batch went nowhere; the test did not exercise supersession")
	}
	a.closeOverlay()
	if a.overlay.search != nil {
		t.Fatal("closing the overlay kept the search run")
	}
}

func TestWorkspaceSwitchClosesSearchAndReplacesTheIndex(t *testing.T) {
	a := newApp()
	t.Cleanup(a.cancel)
	a.openFolder(fixture(t))
	drain(t, a)
	old := a.files
	a.showOverlay(searchOverlay)
	a.openFolder(fixture(t))
	drain(t, a)
	if a.overlay.kind != noOverlay || a.files == old || a.files == nil {
		t.Fatalf("switch kept stale search state: kind=%d sameIndex=%v", a.overlay.kind, a.files == old)
	}
}

func TestArrowKeysMoveTheSelectionWhileTyping(t *testing.T) {
	a := newApp()
	t.Cleanup(a.cancel)
	tt := ui.NewTester(a.view, 1040, 720)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyP)
	tt.Type("view")
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyUp)
	if a.overlay.selected != 1 {
		t.Fatalf("selection = %d after Down, Down, Up; want 1", a.overlay.selected)
	}
	tt.Type("x") // a new query resets the selection
	tt.Frame()
	if a.overlay.selected != 0 {
		t.Fatalf("selection = %d after typing; want 0", a.overlay.selected)
	}
}

func TestFileActionsInvalidateQuickOpen(t *testing.T) {
	a := newApp()
	t.Cleanup(a.cancel)
	root := fixture(t)
	a.openFolder(root)
	drain(t, a)
	a.files.Find("", nil, 10)
	if err := a.files.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "created.md"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.refreshExplorer() // what create, rename, move, delete and Save As call
	drain(t, a)
	a.files.Find("created", nil, 10)
	if err := a.files.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := a.files.Find("created", nil, 10).Paths; len(got) != 1 || got[0] != "docs/created.md" {
		t.Fatalf("Quick Open missed a file created by a file action: %v", got)
	}
}
