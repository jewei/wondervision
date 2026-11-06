# M3 design: command palette, Quick Open, workspace search

Produced with the `architect` workflow: three independent candidates (data
structures first, smallest caller API first, concurrency and failure first),
screened against the design red flags, then synthesized. This page is the
contract the implementation follows; deviations are recorded at the end.

## Problem

M3 adds three keyboard overlays (spec §5): a command palette
(`Cmd/Ctrl+Shift+P`), Quick Open (`Cmd/Ctrl+P`) and workspace content search
(`Cmd/Ctrl+Shift+F`). Constraints from the code:

- `App` state belongs to the UI thread; file I/O runs on **one serial worker**
  (`internal/app/app.go: work/enqueue`). A long scan or search on it would make
  saves wait, so M3 work must not use it.
- Stale results are guarded today by generation IDs (`openID`, `folderID`),
  exhaustively tested by `scripts/verify_request_order.py`.
- Shortcuts are registered ad hoc in `view`, and the same actions are listed
  again in the File menu and toolbar: a hand-synced list the palette would make
  three-way.
- Guardrails: `app` is the only MyGo layer; no `time.Sleep/Tick`; app code does
  no direct `os` file calls.
- The editor binds `Ctrl+F/G/H` on Linux/Windows and `Cmd+F/G` plus Emacs
  `Ctrl+P/N/B/F` on macOS; none of the three M3 shortcuts collide.

## Usage (caller's view)

```go
// Quick Open, in the overlay's view (UI thread, every frame; memoized):
found := a.files.Find(query, a.prefs.Recent, 200)   // never blocks; starts a scan if needed
// found.Paths best first ('/'-separated, relative); found.Scanning shows "Scanning…"

// Content search, on Enter in the overlay:
a.startTextSearch()   // cancels the previous run; results land in that run's own value

// A future MCP binary, no UI:
ix := search.NewIndex(ctx, root, nil); ix.Wait(ctx); paths := ix.Find(q, nil, 50).Paths
sum, err := search.Grep(ctx, root, search.Query{Text: "TODO"}, nil, func(b []search.Hit) { ... })

// Commands: the one table drives shortcuts, File menu, toolbar and palette.
a.bindShortcuts(c)          // the only OnShortcut calls for commands
a.menuItem(m, cmdSave)      // File menu entries by ID
a.toolbarButton(c, cmdSave) // toolbar buttons by ID
```

## Shape

**`internal/search`** (ordinary Go; imports only the standard library):

- `Index` — one per workspace root. `NewIndex(ctx, root, changed)` does no I/O.
  `Find(query, recent, limit) Found` ranks the cached paths, starts a scan when
  the cache isn't fresh, and never blocks. `Invalidate()` marks it stale and
  keeps serving the old paths until a rescan replaces them atomically; a cold
  index publishes batches progressively. `Wait(ctx)` blocks until fresh (for
  non-UI callers). Cancelling `ctx` stops every scan. The path list is an
  append-only slice published by header, so readers never see a mutation.
- `Rank(query, candidates, recent, limit)` and `Score(query, text)` — pure; the
  one fuzzy scorer, also used by the palette. Exact basename > basename prefix >
  word start > substring > subsequence; recent files get a boost and lead an
  empty query.
- `Grep(ctx, root, Query, unsaved map[string]Text, emit) (Summary, error)` —
  one walker and 4 readers; `emit` is called on the caller's goroutine with
  batches in path order; cut at `Query.Limit` (500) with `Truncated`; returns
  only after every goroutine it started has exited.
- `Text` (`LineCount`, `Line`) — satisfied by `editor.Snapshot`, so search
  reads an immutable snapshot of an unsaved buffer and never imports MyGo.
- One unexported `walk` owns the ignore list (`.git node_modules .venv vendor
  dist build .next target __pycache__`) and the policy: regular files only,
  directory symlinks never followed, `.env` and other dotfiles included.
  Files over `documents.MaxFileSize`, with NUL bytes or invalid UTF-8 are
  skipped.

**`internal/app`**:

- `commands.go` — `[commandCount]command` indexed by `commandID` constants:
  title, category, keywords, shortcut, `available(a) (bool, reason)`,
  `run(a, c)`. `bindShortcuts`, the File menu, toolbar buttons and the palette
  all take an ID. A test rejects empty slots and duplicate shortcuts.
- One overlay at a time: `overlay` holds a kind enum, the query, list
  selection, and for content search the current run. Built on `ui.DialogBase`,
  which closes on Escape or a backdrop click and returns focus to the element
  that had it.
- Content search results are **owned by the run that produced them**: each run
  is a `*textSearch`; the apply closures write into that value, and the view
  reads only `a.overlay.search`. Starting a new run cancels the old context and
  replaces the pointer, so a late batch from a superseded run lands somewhere
  the view can no longer see. No generation IDs.
- `openFile` gains a line target so a search hit opens at its line.
- Preferences gain `Recent []string` (at most 20 absolute paths of files the
  user opened; paths only, never contents).

## Misuse resistance

- **Impossible by construction:**
  - a command whose shortcut, menu item, toolbar button and palette entry
    disagree, since every surface takes a `commandID`;
  - two overlays open at once (one field);
  - showing another content-search run's results;
  - search mutating a buffer (`Text` has no mutators);
  - search leaking goroutines (`Grep` returns after joining them).
- **Caught by guardrail tests:**
  - `internal/search` importing MyGo or app packages;
  - `OnShortcut` for commands outside `commands.go`;
  - `search.Grep` or `Index` work queued on the serial worker;
  - a second ignore list.
- **Caught by tests:**
  - ranking order;
  - `Grep` ordering, truncation, unsaved overlay, binary skipping and
    cancellation, plus a goroutine-leak loop;
  - `Index` invalidation and stale-while-revalidate;
  - shortcut uniqueness.
- **Left to review and the GUI verifiers:** overlay focus behaviour and the
  end-to-end journeys in spec §13.

## Synthesis decision

- **Base:** the smallest-API candidate's `search` package (`Index` and `Grep`
  expose no channels, goroutines or lifecycles; one package serves the app and
  a future MCP binary).
- **From the concurrency candidate:** run-owned results instead of ID checks,
  the failure table (supersession, dismissal, workspace switch, window close)
  and the rule that M3 never uses the serial worker.
- **From the data-structures candidate:** the enum-indexed command array with a
  completeness test, the single overlay enum, and the append-only published path
  slice.
- **Rejected:**
  - the concurrency candidate's generic `Stream`/`slot`/`pump` machinery: more
    surface than the problem needs once `Grep` joins its own goroutines;
  - its proposed TLA+ model of that handoff, which doesn't exist in this design;
  - the data candidate's incremental ranking memo: plain ranking of 10,000
    paths takes a few milliseconds, and a benchmark guards the 100 ms target.

## Tradeoffs accepted

- **Freshness without a recursive watch:** we accept a background rescan each
  time Quick Open opens (stale-while-revalidate: old results show at once) in
  exchange for no recursive directory watch, which avoids OS watch limits and
  keeps idle cost at zero. Folder actions and Refresh also invalidate.
- **Search on Enter:** we accept content search running on Enter rather than
  on every keystroke, in exchange for no debounce timers and no wasted scans.
- **Ranking on the UI thread:** we accept ranking running there, in exchange
  for results that are never stale relative to the query.
- **Unavailable commands:** we accept that they stay listed with a reason
  ("Open a folder first") instead of disappearing, so people learn what exists.

## Alternatives considered

- A generic streaming abstraction shared by scan and search (concurrency
  candidate): a wider surface and one more protocol to verify, for no
  capability the simpler `Index` + blocking `Grep` lacks.
- An incremental ranking memo over column-oriented path storage (data
  candidate): faster at extreme scale, but more state to keep coherent;
  deferred until a benchmark shows the need.
- Keeping shortcuts in `view` and building the palette as a fourth list:
  rejected; it's the hand-synced-list red flag.

## Open questions and risks

- Should Settings exist in M3, or show "No settings yet" (chosen for now)?
- Should Save As, Reload from Disk and Compare Disk Version be in the palette?
  (Yes: they're in the table, so they appear everywhere.)
- `Grep` reports byte columns. The editor's `Pos.Col` unit needs checking
  before open-at-column; open-at-line is safe either way.

## Next implementation step

Build `internal/search` with its tests, then `commands.go` with the palette,
then Quick Open, then content search, each slice runnable and verified.
