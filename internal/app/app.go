package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/jewei/wondervision/internal/documents"
	"github.com/jewei/wondervision/internal/preferences"
	"github.com/jewei/wondervision/internal/workspace"
)

type directory struct {
	entries []workspace.Entry
	loading bool
	loaded  bool
	err     error
}

// All model fields belong to the UI thread. A single event-driven worker
// performs file I/O and returns a closure to Window.Update. No idle polling.
type App struct {
	win           *mygo.Window
	ctx           context.Context
	cancel        context.CancelFunc
	jobs          chan func() func()
	post          func(func())
	closed        bool
	err           string
	prefs         preferences.State
	prefsPath     string
	prefsSaving   bool
	restoring     bool
	dialogOpen    bool
	workspace     *workspace.Workspace
	dirs          map[string]*directory
	outline       ui.OutlineState[workspace.Entry]
	row           int
	wsCtx         context.Context
	wsCancel      context.CancelFunc
	epoch         uint64
	folderID      uint64
	folderCancel  context.CancelFunc
	folderLoading bool
	openID        uint64
	openCancel    context.CancelFunc
	fileLoading   bool
	openScoped    bool
	docs          []*documents.Document
	tab           int
	preview       *previewState
	scroll        map[*documents.Document]*ui.ScrollState
	splitPercent  float32
	askQuit       bool
	allowQuit     bool
	quitWhenSaved bool
	finishQuit    func()
}

func newApp() *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{ctx: ctx, cancel: cancel, jobs: make(chan func() func(), 32),
		prefs: preferences.State{MarkdownMode: int(documents.Preview), SidebarWidth: 220},
		dirs:  make(map[string]*directory), row: -1, scroll: make(map[*documents.Document]*ui.ScrollState), splitPercent: 50}
}

func Run() error {
	a := newApp()
	defer a.cancel()
	mygo.App.SetName("WonderVision")
	mygo.App.WhenReady(func() {
		a.finishQuit = mygo.App.Quit
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title: "WonderVision", Width: 1040, Height: 720, MinWidth: 640, MinHeight: 400, Content: ui.View(a.view),
		})
		a.post = a.win.Update
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if a.deferQuit() {
				e.PreventDefault()
				a.win.Invalidate()
			}
		})
		mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
			if a.deferQuit() {
				e.PreventDefault()
				a.win.Invalidate()
			}
		})
		a.win.OnClosed(func() { a.closed = true; a.cancel() })
		go a.work()
		a.restore()
	})
	return mygo.App.Run()
}

func (a *App) work() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case job := <-a.jobs:
			if a.ctx.Err() != nil {
				return
			}
			apply := job()
			if a.ctx.Err() == nil && apply != nil {
				a.post(func() {
					if !a.closed {
						apply()
					}
				})
			}
		}
	}
}

func (a *App) enqueue(job func() func()) bool {
	select {
	case a.jobs <- job:
		return true
	default:
		a.notify(errors.New("too many pending file operations; try again when loading finishes"))
		return false
	}
}

func (a *App) notify(err error) {
	if err != nil && !errors.Is(err, context.Canceled) {
		a.err = err.Error()
	}
}

func (a *App) restore() {
	path, err := preferences.Path()
	if err != nil {
		a.notify(err)
		return
	}
	a.prefsPath = path
	a.restoring = true
	a.enqueue(func() func() {
		s, err := preferences.Load(path)
		return func() {
			a.restoring = false
			if err != nil {
				a.notify(fmt.Errorf("read preferences: %w", err))
				return
			}
			a.prefs = s
			if s.Workspace != "" {
				a.openFolder(s.Workspace)
			}
			if s.ActiveFile != "" {
				a.openFile(s.ActiveFile, "", "")
			}
		}
	})
}

func (a *App) remember() {
	if a.prefsPath == "" || a.prefsSaving {
		return
	}
	s, path := a.prefs, a.prefsPath
	a.prefsSaving = true
	if !a.enqueue(func() func() {
		err := preferences.Save(path, s)
		return func() {
			a.prefsSaving = false
			if err != nil {
				a.notify(fmt.Errorf("save preferences: %w", err))
			}
			if a.prefs != s {
				a.remember()
			} else if a.quitWhenSaved {
				a.quitWhenSaved = false
				if err == nil && a.finishQuit != nil {
					a.finishQuit()
				}
			}
		}
	}) {
		a.prefsSaving = false
	}
}

func (a *App) choose(directory bool) {
	if a.restoring || a.dialogOpen {
		return
	}
	a.dialogOpen = true
	win := a.win
	defaultPath, _ := os.UserHomeDir()
	if a.prefs.ActiveFile != "" {
		defaultPath = filepath.Dir(a.prefs.ActiveFile)
	}
	if a.workspace != nil {
		defaultPath = a.workspace.Root
	}
	go func() {
		title := "Open File"
		if directory {
			title = "Open Folder"
		}
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: title, Directory: directory, ShowHiddenFiles: true, DefaultPath: defaultPath})
		win.Update(func() {
			a.dialogOpen = false
			if a.closed {
				return
			}
			if err != nil {
				a.notify(err)
				return
			}
			if len(paths) == 0 {
				return
			}
			if directory {
				a.openFolder(paths[0])
			} else {
				a.openFile(paths[0], "", "")
			}
		})
	}()
}

func (a *App) openFolder(path string) {
	if a.openScoped && a.openCancel != nil {
		a.openCancel()
		a.openID++
		a.fileLoading = false
	}
	if a.folderCancel != nil {
		a.folderCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.folderCancel = cancel
	a.folderID++
	id := a.folderID
	a.folderLoading = true
	if !a.enqueue(func() func() {
		w, err := workspace.Open(ctx, path)
		var entries []workspace.Entry
		if err == nil {
			entries, err = w.List(ctx, w.Root)
		}
		return func() {
			if id != a.folderID || ctx.Err() != nil {
				return
			}
			a.folderLoading = false
			if err != nil {
				a.notify(fmt.Errorf("open folder: %w", err))
				return
			}
			if a.wsCancel != nil {
				a.wsCancel()
			}
			if a.openScoped && a.openCancel != nil {
				a.openCancel()
				a.openID++
				a.fileLoading = false
			}
			a.wsCtx, a.wsCancel = context.WithCancel(a.ctx)
			a.epoch++
			a.workspace = w
			for _, d := range a.docs {
				if workspace.Contains(w.Root, d.Path) && d.Area != w.Root {
					d.Area = w.Root
					if a.preview != nil && a.preview.doc == d {
						a.releasePreview()
					}
				}
			}
			a.dirs = map[string]*directory{w.Root: {entries: entries, loaded: true}}
			a.outline = ui.OutlineState[workspace.Entry]{}
			a.row = -1
			a.prefs.Workspace = w.Root
			a.remember()
		}
	}) {
		a.folderLoading = false
		cancel()
	}
}

func (a *App) loadDirectory(path string) {
	d := a.dirs[path]
	if d == nil {
		d = &directory{}
		a.dirs[path] = d
	}
	if d.loaded || d.loading || d.err != nil {
		return
	}
	d.loading = true
	w, ctx, epoch := a.workspace, a.wsCtx, a.epoch
	if !a.enqueue(func() func() {
		entries, err := w.List(ctx, path)
		return func() {
			if epoch != a.epoch || ctx.Err() != nil {
				return
			}
			d.entries, d.err, d.loading, d.loaded = entries, err, false, err == nil
			if err != nil {
				a.notify(fmt.Errorf("read folder: %w", err))
			}
		}
	}) {
		d.loading = false
	}
}

func (a *App) active() *documents.Document {
	if a.tab < 0 || a.tab >= len(a.docs) {
		return nil
	}
	return a.docs[a.tab]
}

func (a *App) activate(index int) {
	a.tab = index
	if a.preview != nil && a.preview.doc != a.active() {
		a.releasePreview()
	}
	if d := a.active(); d != nil {
		a.prefs.ActiveFile = d.Path
		a.remember()
	}
}

func (a *App) switchTab(index int) {
	if a.openCancel != nil {
		a.openCancel()
	}
	a.openID++
	a.fileLoading = false
	a.activate(index)
}

func (a *App) openFile(path, area, anchor string) {
	a.openScoped = area != ""
	if a.openCancel != nil {
		a.openCancel()
	}
	a.openID++
	for i, d := range a.docs {
		if d.Path == path {
			a.fileLoading = false
			a.activate(i)
			a.setAnchor(d, anchor)
			return
		}
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.openCancel = cancel
	id, mode := a.openID, documents.Mode(a.prefs.MarkdownMode)
	a.fileLoading = true
	if !a.enqueue(func() func() {
		d, err := documents.Open(ctx, path, area, mode)
		return func() {
			if id != a.openID || ctx.Err() != nil {
				return
			}
			a.fileLoading = false
			if err != nil {
				a.notify(fmt.Errorf("open file: %w", err))
				return
			}
			for i, existing := range a.docs {
				if existing.Same(d) {
					a.activate(i)
					a.setAnchor(existing, anchor)
					return
				}
			}
			if a.workspace != nil && workspace.Contains(a.workspace.Root, d.Path) {
				d.Area = a.workspace.Root
			}
			a.docs = append(a.docs, d)
			a.scroll[d] = &ui.ScrollState{}
			a.activate(len(a.docs) - 1)
			a.setAnchor(d, anchor)
		}
	}) {
		a.fileLoading = false
		cancel()
	}
}

func (a *App) needsQuitConfirmation() bool {
	if a.allowQuit {
		return false
	}
	for _, d := range a.docs {
		if d.Buffer.Modified() {
			a.askQuit = true
			return true
		}
	}
	return false
}

func (a *App) deferQuit() bool {
	if a.needsQuitConfirmation() {
		return true
	}
	if a.prefsSaving {
		a.quitWhenSaved = true
		return true
	}
	return false
}

func (a *App) title() string {
	if a.workspace == nil {
		return "No folder open"
	}
	return filepath.Base(a.workspace.Root)
}
