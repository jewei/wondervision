package app

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/jewei/wondervision/internal/documents"
	"github.com/jewei/wondervision/internal/preferences"
)

// Execute queued I/O and completions separately, without a window or sleeps.
func drain(t *testing.T, a *App) {
	t.Helper()
	for range 100 {
		select {
		case job := <-a.jobs:
			if apply := job(); apply != nil {
				apply()
			}
		default:
			return
		}
	}
	t.Fatal("file operations did not settle")
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"report.md": "# Report\n\n- [ ] Review\n\n| Key | Value |\n| --- | --- |\n| A | B |\n\n```go\npackage main\n```\n\n[Next](docs/next.md)\n",
		"todo.md":   "# TODO\n", ".env": "KEY=original\n", "config.yaml": "enabled: true\n", "AGENTS.md": "Ordinary document\n", "docs/next.md": "# Next page\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestExplorerRefreshKeepsRenderedStateAliveUntilNextFrame(t *testing.T) {
	for _, operation := range []string{"reopen", "refresh"} {
		t.Run(operation, func(t *testing.T) {
			a := newApp()
			t.Cleanup(a.cancel)
			root := fixture(t)
			a.openFolder(root)
			drain(t, a)
			tt := ui.NewTester(a.view, 1040, 720)
			previous := a.outline
			rows, first := previous.Rows(), previous.Item(0).Name
			label := previous.List.Label
			if operation == "reopen" {
				a.openFolder(root)
			} else {
				a.refreshExplorer()
			}
			drain(t, a)
			// Native accessibility may read the last rendered frame before the
			// next UI frame. Its retained list state and label closure must work.
			if previous.Rows() != rows || label(0) != first {
				t.Fatal("refresh invalidated the still-rendered Explorer state")
			}
			tt.Frame()
			if a.outline.Rows() != rows || !tt.HasText(".env") {
				t.Fatal("refreshed Explorer did not render")
			}
		})
	}
}

func TestOpenViewAndLazyExplorer(t *testing.T) {
	root := fixture(t)
	a := newApp()
	t.Cleanup(a.cancel)
	a.openFolder(root)
	drain(t, a)
	tt := ui.NewTester(a.view, 1040, 720)
	if len(a.dirs) != 1 || len(a.jobs) != 0 || tt.HasText("next.md") || !tt.HasText(".env") {
		t.Fatal("startup eagerly loaded descendants or hid dotfiles")
	}
	if err := tt.Click("report.md"); err != nil {
		t.Fatal(err)
	}
	drain(t, a)
	tt.Frame()
	d := a.active()
	if d == nil || !d.Markdown || d.Mode != documents.Preview || !tt.HasText("Report") {
		t.Fatalf("report did not open in preview: %q", tt.Texts())
	}
	if err := tt.Click("Split"); err != nil {
		t.Fatal(err)
	}
	if d.Mode != documents.Split {
		t.Fatal("split action failed")
	}
	d.Buffer.Edit(editor.Edit{Start: 0, End: len("# Report"), Text: "# Updated report"})
	tt.Frame()
	if !tt.HasText("Updated report") {
		t.Fatalf("preview missed unsaved buffer: %q", tt.Texts())
	}
	raw, _ := os.ReadFile(d.Path)
	if string(raw[:8]) != "# Report" {
		t.Fatal("preview or editor wrote to disk")
	}
	if err := tt.Click("Edit"); err != nil {
		t.Fatal(err)
	}
	if a.preview != nil {
		t.Fatal("hidden preview retained assets")
	}
	for _, entry := range a.dirs[root].entries {
		if entry.Name == "docs" {
			a.outline.Open.Add(entry)
		}
	}
	tt.Frame()
	if len(a.jobs) != 1 || tt.HasText("next.md") {
		t.Fatal("expansion should asynchronously read exactly one directory")
	}
	drain(t, a)
	tt.Frame()
	if !tt.HasText("next.md") || len(a.dirs) != 2 {
		t.Fatal("expanded children missing")
	}
	a.openFile(filepath.Join(root, ".env"), root, "")
	drain(t, a)
	tt.Frame()
	if a.active().Markdown || a.active().Mode != documents.Edit || a.preview != nil {
		t.Fatal("plain file incurred a preview")
	}
	a.active().Editor.Select(2, 2)
	tt.Key(ui.Cmd, ui.KeyTab)
	if a.active() != d || d.Buffer.String()[:16] != "# Updated report" {
		t.Fatal("tab switching lost the editing session")
	}
	tt.Key(ui.Cmd, ui.KeyTab)
	if a.active().Editor.Cursor() != 2 {
		t.Fatal("tab switching lost cursor state")
	}
	tt.Advance(time.Minute)
	if tt.FramePending() {
		t.Fatal("settled document view requests continuous frames")
	}
}

func TestStaleResultsAndDuplicateFiles(t *testing.T) {
	first, second := fixture(t), fixture(t)
	a := newApp()
	t.Cleanup(a.cancel)
	a.openFolder(first)
	old := (<-a.jobs)()
	a.openFolder(second)
	drain(t, a)
	old()
	if a.workspace.Root != second {
		t.Fatal("stale workspace replaced current one")
	}
	a.openFile(filepath.Join(first, "report.md"), "", "")
	old = (<-a.jobs)()
	a.openFile(filepath.Join(second, "todo.md"), "", "")
	drain(t, a)
	old()
	if len(a.docs) != 1 || filepath.Base(a.active().Path) != "todo.md" {
		t.Fatal("stale file result created a tab")
	}
	d := a.active()
	d.Buffer.Edit(editor.Edit{Start: 0, Text: "unsaved "})
	alias := filepath.Join(second, "alias.md")
	if err := os.Symlink(d.Path, alias); err != nil {
		t.Skip(err)
	}
	a.openFile(alias, second, "")
	drain(t, a)
	if len(a.docs) != 1 || a.active() != d || !d.Buffer.Modified() {
		t.Fatal("alias created a second buffer or discarded edits")
	}
	if !a.needsQuitConfirmation() || !a.askQuit {
		t.Fatal("unsaved edits are not protected at quit")
	}
}

func TestRestoreLoadsOnlyRootAndActiveFile(t *testing.T) {
	root := fixture(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := preferences.Path()
	if err != nil {
		t.Fatal(err)
	}
	s := preferences.State{Workspace: root, ActiveFile: filepath.Join(root, "docs", "next.md"), MarkdownMode: int(documents.Split), SidebarWidth: 230}
	if err := preferences.Save(path, s); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	t.Cleanup(a.cancel)
	a.restore()
	if a.workspace != nil || len(a.docs) != 0 {
		t.Fatal("restore blocked window creation")
	}
	drain(t, a)
	if a.restoring || len(a.dirs) != 1 || len(a.docs) != 1 || a.active().Path != s.ActiveFile || a.active().Mode != documents.Split {
		t.Fatal("restore should load only root metadata and active buffer")
	}
	b := newApp()
	t.Cleanup(b.cancel)
	s.Workspace = ""
	if err := preferences.Save(path, s); err != nil {
		t.Fatal(err)
	}
	b.restore()
	drain(t, b)
	if b.workspace != nil || len(b.docs) != 1 {
		t.Fatal("standalone file restore requires a workspace")
	}
}

func TestLocalImagesAndMarkdownLinks(t *testing.T) {
	root := fixture(t)
	file, err := os.Create(filepath.Join(root, "sample.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	file.Close()
	source := filepath.Join(root, "report.md")
	if _, err := loadLocalImage(context.Background(), root, source, "sample.png"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"https://example.com/image.png", "file:///etc/passwd", "../escape.png", "data:image/png;base64,a"} {
		if _, err := loadLocalImage(context.Background(), root, source, raw); err == nil {
			t.Fatalf("unsafe image loaded: %s", raw)
		}
	}
	a := newApp()
	t.Cleanup(a.cancel)
	a.openFile(source, root, "")
	drain(t, a)
	d := a.active()
	a.followLink(d, "docs/next.md#next-page")
	drain(t, a)
	if filepath.Base(a.active().Path) != "next.md" {
		t.Fatal("local Markdown link did not open")
	}
	a.activate(0)
	a.followLink(d, "javascript:alert(1)")
	drain(t, a)
	if len(a.docs) != 2 {
		t.Fatal("unsafe link opened a document")
	}
}

func TestWorkspaceChangeCancelsLateScopedOpenAndUpdatesExistingArea(t *testing.T) {
	first, second := fixture(t), fixture(t)
	nested := filepath.Join(first, "docs", "next.md")
	a := newApp()
	defer a.cancel()
	a.openFile(nested, "", "")
	drain(t, a)
	d := a.active()
	if d.Area != filepath.Dir(nested) {
		t.Fatal("standalone area was not restricted")
	}
	a.openFolder(first)
	drain(t, a)
	if d.Area != first {
		t.Fatal("existing document did not gain the explicitly opened workspace area")
	}
	a.openFolder(second)
	folderResult := (<-a.jobs)()
	a.openFile(filepath.Join(first, "todo.md"), first, "")
	fileResult := (<-a.jobs)()
	folderResult()
	fileResult()
	if a.workspace.Root != second || len(a.docs) != 1 || a.active() != d || a.fileLoading {
		t.Fatal("old workspace file completion changed the new workspace session")
	}
}

func TestQuitWaitsForLatestPreferences(t *testing.T) {
	a := newApp()
	defer a.cancel()
	a.prefsPath = filepath.Join(t.TempDir(), "preferences.json")
	a.prefs.ActiveFile = "first.md"
	a.remember()
	a.prefs.ActiveFile = "last.md"
	a.remember()
	finished := false
	a.finishQuit = func() { finished = true }
	if !a.deferQuit() {
		t.Fatal("quit did not defer the pending preferences write")
	}
	drain(t, a)
	saved, err := preferences.Load(a.prefsPath)
	if err != nil || saved.ActiveFile != "last.md" || !finished {
		t.Fatalf("quit lost last preferences: %+v %v finished=%v", saved, err, finished)
	}
}
