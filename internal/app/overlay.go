package app

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/jewei/wondervision/internal/documents"
	"github.com/jewei/wondervision/internal/preferences"
	"github.com/jewei/wondervision/internal/search"
)

type overlayKind int

const (
	noOverlay overlayKind = iota
	paletteOverlay
	quickOpenOverlay
	searchOverlay
)

const quickOpenLimit = 200

// overlayState is the one overlay that can be open. All of it belongs to the
// UI thread.
type overlayState struct {
	kind          overlayKind
	open          bool // bound to ui.DialogBase; Escape and backdrop clicks clear it
	query         string
	shown         string // the query the list was last built for
	selected      int
	list          ui.ListState
	caseSensitive bool
	search        *textSearch // the current content search run, if any
}

// textSearch is one content search run. Its apply closures write only into
// this value, and the view reads only a.overlay.search, so a batch from a
// superseded run lands where nothing shows it. No generation IDs needed.
type textSearch struct {
	query   search.Query
	root    string
	hits    []search.Hit
	sum     search.Summary
	running bool
	err     error
	cancel  context.CancelFunc
}

// showOverlay opens an overlay of kind, replacing any open one.
func (a *App) showOverlay(kind overlayKind) {
	a.closeOverlay()
	a.overlay = overlayState{kind: kind, open: true, selected: 0}
	a.overlay.list.Selected = &a.overlay.selected
	if kind == quickOpenOverlay && a.files != nil {
		// Stale-while-revalidate: show the cached paths now, rescan behind them.
		a.files.Invalidate()
	}
}

// closeOverlay is the only place an overlay ends; it stops a running search.
func (a *App) closeOverlay() {
	if s := a.overlay.search; s != nil && s.cancel != nil {
		s.cancel()
	}
	a.overlay = overlayState{}
}

// redraw asks for a frame from any goroutine.
func (a *App) redraw() {
	if a.post != nil {
		a.post(func() {})
	}
}

// show moves a just-opened document to its target and records it as recent.
func (a *App) show(d *documents.Document, at target) {
	a.setAnchor(d, at.anchor)
	if at.line > 0 {
		if d.Markdown && d.Mode == documents.Preview {
			a.setMode(d, documents.Split) // keep the preview, show the line
		}
		d.Editor.GoTo(editor.Pos{Line: at.line - 1, Col: at.col})
	}
	a.rememberRecent(d.Path)
}

func (a *App) rememberRecent(path string) {
	if len(a.prefs.Recent) > 0 && a.prefs.Recent[0] == path {
		return
	}
	a.prefs.Recent = preferences.AddRecent(a.prefs.Recent, path)
	a.remember()
}

// overlays builds the open overlay, if any.
func (a *App) overlays(c *ui.Context) {
	o := &a.overlay
	if o.kind == noOverlay {
		return
	}
	if !o.open { // Escape or a backdrop click closed it
		a.closeOverlay()
		return
	}
	t := c.Theme()
	ui.DialogBase(c, &o.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(600).Height(420).Padding(12).Radius(10).Background(t.Background).
			Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, 0.3))
		ui.Column(c).Fill().Gap(8).Children(func() {
			switch o.kind {
			case paletteOverlay:
				a.palette(c, panel)
			case quickOpenOverlay:
				a.quickOpen(c, panel)
			case searchOverlay:
				a.searchView(c, panel)
			}
		})
	})
}

// listKeys lets Up and Down move the result list's selection while the query
// field keeps the focus; Enter arrives as the field's Submitted (see choose).
func (a *App) listKeys(panel ui.Element, n int) {
	o := &a.overlay
	panel.OnShortcut(0, ui.KeyDown, func() {
		if o.selected < n-1 {
			o.selected++
			o.list.ScrollIntoView(o.selected)
		}
	})
	panel.OnShortcut(0, ui.KeyUp, func() {
		if o.selected > 0 {
			o.selected--
			o.list.ScrollIntoView(o.selected)
		}
	})
}

// submit runs choose for the selected row when Enter was pressed in field.
func (a *App) submit(field ui.Element, n int, choose func(int)) {
	if o := &a.overlay; field.Submitted() && o.selected >= 0 && o.selected < n {
		choose(o.selected)
	}
}

// queryField builds the focused query field and resets the selection when
// the query changes.
func (a *App) queryField(c *ui.Context, label, placeholder string) ui.Element {
	o := &a.overlay
	f := ui.SearchField(c, &o.query).Label(label).Placeholder(placeholder).AutoFocus()
	if o.query != o.shown {
		o.shown, o.selected = o.query, 0
		o.list.ScrollIntoView(0)
	}
	return f
}

// paletteEntry is a command ranked for the current query.
type paletteEntry struct {
	id    commandID
	score int
}

func (a *App) paletteEntries(query string) []paletteEntry {
	var out []paletteEntry
	for id, cmd := range commands {
		if cmd.hidden {
			continue
		}
		s := search.Score(query, cmd.title) * 4
		for _, k := range append([]string{cmd.category}, cmd.keywords...) {
			s = max(s, search.Score(query, k))
		}
		if s > 0 {
			out = append(out, paletteEntry{commandID(id), s})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	return out
}

func (a *App) palette(c *ui.Context, panel ui.Element) {
	t := c.Theme()
	entries := a.paletteEntries(a.overlay.query)
	run := func(i int) {
		id := entries[i].id
		if ok, _ := commands[id].available(a); !ok {
			return
		}
		a.closeOverlay()
		a.invoke(c, id)
	}
	a.listKeys(panel, len(entries))
	a.submit(a.queryField(c, "Command palette", "Type a command"), len(entries), run)
	ui.List(c, &a.overlay.list, len(entries), func(i int) {
		cmd := commands[entries[i].id]
		ok, why := cmd.available(a)
		row := ui.Row(c).Padding(6, 8).Gap(12).Children(func() {
			title := ui.Text(c, cmd.title).Grow(1)
			if !ok {
				title.TextColor(t.TextMuted)
				ui.Text(c, why).FontSize(12).TextColor(t.TextMuted)
			}
			ui.Text(c, cmd.key.label()).FontSize(12).TextColor(t.TextMuted)
		})
		row.Label(cmd.title)
		if row.Clicked() {
			run(i)
		}
	}).Grow(1)
	if len(entries) == 0 {
		ui.Text(c, "No matching commands").Padding(8).TextColor(t.TextMuted)
	}
}

func (a *App) quickOpen(c *ui.Context, panel ui.Element) {
	t := c.Theme()
	var paths []string
	var root string
	scanning := false
	if a.files != nil && a.workspace != nil {
		root = a.workspace.Root
		found := a.files.Find(a.overlay.query, a.prefs.Recent, quickOpenLimit)
		paths, scanning = found.Paths, found.Scanning
		if found.Err != nil {
			defer ui.Text(c, "Can't list files: "+found.Err.Error()).Padding(8).TextColor(t.TextMuted)
		}
	} else {
		// No workspace: rank the recent manually opened files.
		paths = search.Rank(a.overlay.query, a.prefs.Recent, a.prefs.Recent, quickOpenLimit)
	}
	open := func(i int) {
		p := paths[i]
		area := ""
		if root != "" {
			p, area = filepath.Join(root, filepath.FromSlash(p)), root
		}
		a.closeOverlay()
		a.openFileTo(p, area, target{})
	}
	a.listKeys(panel, len(paths))
	a.submit(a.queryField(c, "Quick Open", "Type a file name"), len(paths), open)
	ui.List(c, &a.overlay.list, len(paths), func(i int) {
		p := paths[i]
		name, dir := path.Base(filepath.ToSlash(p)), path.Dir(filepath.ToSlash(p))
		row := ui.Row(c).Padding(6, 8).Gap(10).Children(func() {
			ui.Text(c, name).SingleLine()
			if dir != "." {
				ui.Text(c, dir).FontSize(12).SingleLine().TextColor(t.TextMuted).Grow(1)
			}
		})
		row.Label(name)
		if row.Clicked() {
			open(i)
		}
	}).Grow(1)
	switch {
	case scanning:
		ui.Text(c, "Scanning…").FontSize(12).Padding(4, 8).TextColor(t.TextMuted)
	case len(paths) == 0 && root == "":
		ui.Text(c, "No recent files. Open a folder to find files in it.").Padding(8).TextColor(t.TextMuted)
	case len(paths) == 0:
		ui.Text(c, "No matching files").Padding(8).TextColor(t.TextMuted)
	}
}

// unsavedTexts snapshots the open documents with unsaved edits, so content
// search reads what the user sees. Snapshots are immutable; never pass the
// live Buffer.
func (a *App) unsavedTexts() map[string]search.Text {
	texts := make(map[string]search.Text)
	for _, d := range a.docs {
		if d.Dirty() {
			texts[d.Path] = d.Buffer.Snapshot()
		}
	}
	return texts
}

// startTextSearch cancels the current content search and starts a new one
// for the overlay's query. Grep runs on its own goroutine, never on the
// serial file worker, so saves don't wait behind it.
func (a *App) startTextSearch() {
	o := &a.overlay
	if old := o.search; old != nil && old.cancel != nil {
		old.cancel()
	}
	o.search = nil
	if a.workspace == nil || o.query == "" {
		return
	}
	ctx, cancel := context.WithCancel(a.wsCtx)
	run := &textSearch{query: search.Query{Text: o.query, CaseSensitive: o.caseSensitive}, root: a.workspace.Root, running: true, cancel: cancel}
	o.search, o.selected = run, 0
	unsaved, post := a.unsavedTexts(), a.post
	go func() {
		defer cancel()
		sum, err := search.Grep(ctx, run.root, run.query, unsaved, func(hits []search.Hit) {
			post(func() { run.hits = append(run.hits, hits...) })
		})
		post(func() {
			run.running, run.sum = false, sum
			if err != nil && !errors.Is(err, context.Canceled) {
				run.err = err
			}
		})
	}()
}

func (a *App) searchView(c *ui.Context, panel ui.Element) {
	t := c.Theme()
	o := &a.overlay
	run := o.search
	var hits []search.Hit
	if run != nil {
		hits = run.hits
	}
	open := func(i int) {
		h, root := hits[i], run.root
		a.closeOverlay()
		a.openFileTo(filepath.Join(root, filepath.FromSlash(h.Path)), root, target{line: h.Line, col: h.Column})
	}
	// Enter searches when the query or case setting changed since the last
	// run, and otherwise opens the selected hit.
	a.listKeys(panel, len(hits))
	ui.Row(c).Gap(8).Children(func() {
		field := a.queryField(c, "Search in workspace", "Search text, then press Enter").Grow(1)
		a.submit(field, max(len(hits), 1), func(i int) {
			if run == nil || run.query.Text != o.query || run.query.CaseSensitive != o.caseSensitive || len(hits) == 0 {
				a.startTextSearch()
				return
			}
			open(i)
		})
		if ui.Checkbox(c, &o.caseSensitive, "Match case").Changed() && run != nil {
			a.startTextSearch()
		}
	})
	ui.List(c, &o.list, len(hits), func(i int) {
		h := hits[i]
		row := ui.Column(c).Padding(6, 8).Gap(2).Children(func() {
			ui.Textf(c, "%s:%d", h.Path, h.Line).FontSize(12).SingleLine().TextColor(t.TextMuted)
			ui.Text(c, h.Excerpt).SingleLine()
		})
		row.Label(fmt.Sprintf("%s line %d", h.Path, h.Line))
		if row.Clicked() {
			open(i)
		}
	}).Grow(1)
	status := "Press Enter to search. Escape closes."
	switch {
	case run == nil:
	case run.err != nil:
		status = "Search failed: " + run.err.Error()
	case run.running:
		status = fmt.Sprintf("Searching… %d matches", len(hits))
	case run.sum.Truncated:
		status = fmt.Sprintf("Showing the first %d matches; refine the search for more", len(hits))
	case len(hits) == 0:
		status = fmt.Sprintf("No matches in %d files", run.sum.Files)
	default:
		status = fmt.Sprintf("%d matches in %d files", len(hits), run.sum.Files)
	}
	ui.Text(c, status).FontSize(12).Padding(4, 8).TextColor(t.TextMuted)
}
