# Work from the keyboard

Run any command from a palette, open a workspace file by a few letters of its
name, and find text across the workspace, all without the mouse (spec §5, M3).

## Sub-features

- KB1. Command/Control+Shift+P opens the command palette; typing ranks commands
  by title, keyword and category; Enter runs the selected one; unavailable
  commands are listed with the reason.
- KB2. Escape closes any overlay and the keyboard returns to the element that
  had it (the editor).
- KB3. Command/Control+P opens Quick Open: fuzzy file names first, recent files
  boosted, a background rescan on each open; with no folder it lists recent
  files.
- KB4. Command/Control+Shift+F opens workspace search: Enter searches (case
  insensitive unless Match case), results stream in path order and stop at 500;
  Enter on a result opens the file at that line and column.
- KB5. Shortcuts, the File menu, toolbar buttons and the palette come from one
  command table, so they always agree.

## How to get to it (user POV)

Press the shortcuts above anywhere in the window, or choose Quick Open and
Search in Workspace from the File menu or the palette.

## Driving it with the native harness

Preconditions: a fresh output directory under `build/verification/`, the e2e
binary built from the current source, a virtual display with a window manager.

- Run `python3 scripts/verify_m3.py --out build/verification/m3`. It opens the
  seeded workspace through the native folder dialog, then:
  - focuses the editor of `report.md`, opens the palette with
    `CmdOrCtrl+Shift+P`, presses Escape, and types a real X11 key, which must
    mark the file Unsaved (focus came back), then undoes it;
  - types `toggle side` in the palette and presses Enter: `Show Sidebar` must
    appear; `CmdOrCtrl+B` must bring back `Hide Sidebar`;
  - types `nxt` in Quick Open: the `next.md` row must appear and Enter must
    open the `next.md` tab;
  - searches `native views`: the row `report.md line 3` must appear and Enter
    must put the cursor at `Ln 3, Col 14`.
- Inspect `palette.png`, `sidebar-hidden.png`, `quick-open.png`, `search.png`,
  `opened-at-line.png`, `result.json` and `cleanup.json`.

## Gotchas

- A modal overlay holds the keyboard: window shortcuts don't fire while one is
  open. Close it with Escape before pressing another overlay's shortcut.
- The overlay query fields must not use AutoFocus; it breaks focus return on
  Escape (see `queryField` in `internal/app/overlay.go`).
- The overlay rows share labels with Explorer items and tabs (`next.md`); match
  rows by role, as `row()` in the driver does.
- Unit tests in `internal/app/m3_test.go` cover the same journeys headlessly
  with `ui.Tester`; this driver proves them in the real native window.
