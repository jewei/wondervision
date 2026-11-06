# Folio — Lean Markdown & Text Workspace

**Status:** Implementation specification (MVP v0.1)  
**Updated:** 2026-11-06  
**Working name:** Folio (rename freely)  
**Audience:** Coding agent implementing the application  
**Framework:** [MyGo](https://github.com/egoist/mygo), native Go UI

> **Mission:** Build the text-file utility I can leave open 24/7. It should launch quickly, stay quiet at idle, and let me browse, search, review, and safely edit Markdown, notes, `.env`, and configuration files without opening a full IDE.

## 0. Agent instructions — read first

1. **Optimize for low idle CPU and stable RAM, not feature count.** A feature that needs perpetual polling, indexing, a browser engine, a language server, or an always-on subprocess does not belong in v0.1.
2. Build a **single-window native MyGo application in Go**. Do not use a JS frontend, Electron, WebView, Monaco, CodeMirror, React, or a separate local server.
3. Work with **real files and directories**. Do not migrate or copy a workspace into an app-controlled database. No proprietary file format.
4. Prioritize the complete user journeys in §2 and acceptance tests in §13 before polish or optional features.
5. Use existing MyGo primitives where appropriate. **Verify method signatures against the version pinned in `go.mod`**; framework APIs can evolve. Code fragments in this document are conceptual unless explicitly labeled executable.
6. Keep dependencies minimal. Prefer the standard library and MyGo over adding search, indexing, database, watcher, markdown, or editor packages.
7. All filesystem operations, recursive scanning, and content search must be cancellable where appropriate and must not block the UI thread.
8. Do not add MCP, AI chat, agents, Git UI, rich-text/WYSIWYG editing, LSP, terminal, or code execution to the MVP.
9. Before declaring complete, build and run tests; report actual performance measurements instead of claiming numerical goals were met without measurement.

## 1. Product definition

### Intended daily use

- Open a project or notes folder; quickly inspect its text files.
- Edit `.env`, `.env.local`, `.gitignore`, `.toml`, `.yaml`, `.json`, `.ini`, shell scripts, and other small configuration files.
- Read AI-generated Markdown reports before approving work **outside this app**. This is a review surface, not an approval or agent-execution system.
- Maintain `todo.md`, scratch notes, checklists, and ordinary project notes.
- Read and edit agent-oriented files such as `AGENTS.md`, `CLAUDE.md`, `MEMORY.md`, and files in user-chosen memory folders; treat them as normal files without app-specific semantics.
- Keep the app open for days without noticeable idle CPU use or steadily growing memory.

### Product character

**A fast document/file utility with a Markdown reader**, not a VS Code/Cursor replacement, IDE, knowledge graph, cloud notes platform, or AI application. Optimize for a few frequently opened files and a lot of time spent idle.

### Core defaults

- Offline and local-first; no account, network activity, analytics, or telemetry.
- One native window with a collapsible sidebar.
- Files stay in place. No automatic workspace copies or renaming.
- **Manual Save is the default**. Never silently write changes to config/secret files.
- Opened Markdown files default to **Preview** (when selected for reading) or the user's last Markdown mode; editing remains one action away. Plain text/code files open in **Edit**.
- Dotfiles such as `.env`, `.gitignore`, and `.editorconfig` are discoverable and visible; large tool/build directories are ignored in search by default.
- Search and previews run only when requested/visible; do not precompute everything.

## 2. User journeys (must work end-to-end)

**A. Review an agent report**
1. Open the project directory.
2. Press `Cmd/Ctrl+P`, type part of `report.md`, press Enter.
3. Read correctly rendered Markdown with headings, lists, checkboxes, tables, fenced code, and local images.
4. Switch to Edit or Split view if needed; follow a local Markdown link to another workspace file.
5. Return to the previous file without losing scroll position.

**B. Make a two-line config change**
1. Press `Cmd/Ctrl+P`, type `.env` or `config.yaml`, press Enter.
2. See syntax-highlighted editable text; find a key with `Cmd/Ctrl+F`.
3. Edit it; a visible dirty marker appears.
4. Press `Cmd/Ctrl+S`; preserve the file's encoding/line endings and permissions where supported.
5. If another process changed the file, do not overwrite it silently; prompt for conflict resolution.

**C. Maintain lightweight TODOs**
1. Open or create `todo.md` directly from a file action or command palette.
2. Write normal GitHub-flavored Markdown task lists (`- [ ]`, `- [x]`).
3. View them in Markdown preview, then return to source to edit them.
4. Save and find the file again using Quick Open.

**D. Open a quick file without a workspace**
1. Use Open File and select a text file directly.
2. Read/edit it in a tab; the app does not require creating or choosing a vault.
3. The selected file is an explicitly authorized outside-workspace document; folder-scoped operations do not implicitly expand to its siblings.

**E. Stay open all day**
1. Open a moderate workspace and a few tabs.
2. Leave Folio idle or minimized for several hours.
3. No periodic repainting, crawling, repeated preview parsing, or needless disk reads.
4. External edits to an open file can still be detected and surfaced without silently discarding unsaved work.

## 3. MVP features (P0)

| Feature | Required behavior |
|---|---|
| Native window | Single window, system light/dark, minimal toolbar/status bar |
| Workspace | Open Folder; last workspace remembered; also Open File without workspace |
| Explorer | Folder tree/outline, lazy directory expansion, keyboard navigation, refresh |
| File actions | New File/Folder, Rename, Move, Delete with appropriate confirmation and errors |
| File tabs | Open, switch, close; dirty indicator; reopen last active file lazily |
| Editor | Plain text + code highlighting, undo/redo, find/replace, line number, wrap toggle |
| Markdown | Render GFM; Preview/Edit/Split; local relative images and file links |
| Command palette | Keyboard-first fuzzy action launcher (`Cmd/Ctrl+Shift+P`) |
| Quick Open | Filename/path fuzzy search (`Cmd/Ctrl+P`), cross-folder, keyboard-first |
| Workspace search | On-demand text search (`Cmd/Ctrl+Shift+F`), file/line/snippet results |
| File correctness | Preserve UTF-8 BOM if present, newline style, final newline, and existing file permissions where feasible |
| Safe saving | Manual Save, dirty tracking, conflict detection, explicit discard, crash recovery of non-sensitive drafts |
| External changes | Detect changes to open files; reload unchanged buffers; conflict prompt for dirty buffers |
| Preferences | Minimal persistent state (workspace, sidebar, recent files, view preference) |
| Efficiency | Low idle wakeups, bounded caches, virtualized UI, cancellable file work |

**Supported in v0.1:** `.md`, `.markdown`, `.txt`, `.env`, `.env.*`, `.gitignore`, `.editorconfig`, `.json`, `.jsonc`, `.yaml`, `.yml`, `.toml`, `.ini`, `.conf`, `.properties`, `.go`, `.py`, `.js`, `.ts`, `.sh`, `.bash`, `.css`, `.html`, `.xml`, `.sql`, `.rs`, plus unrecognized UTF-8 text files. Unknown files open as plain text after validation. MIME labels are informational; extensions are not a security boundary.

**No special TODO database:** A TODO is ordinary Markdown. Preview checkboxes are read-only in v0.1; edit the text to toggle a task.

## 4. Explicit non-goals and scope cuts

**Do not build these for v0.1:**

- Integrated terminal, shell execution, debugger, tasks/build runner, LSP, code intelligence, code formatters, extensions, plugins, Git UI, source control status polling.
- Embedded AI, chat, vector search, embeddings, agent orchestration, automated approval, cloud sync, collaboration, accounts, telemetry.
- Graph view, backlinks, tags database, rich-text editor, daily-note generator, template system, embedded browser.
- Background content indexing, filesystem-wide recursive watcher, always-on MCP server, login item, system-tray daemon.
- Automatic updating, remote-image fetching, any network connection initiated by the app at runtime.
- Binary editing, huge-file streaming editor, arbitrary encodings, archive browsing, remote filesystem integration.

If a requested feature conflicts with performance or simplicity, **prefer omission with a documented follow-up**.

## 5. UI and interaction specification

### Layout

```text
┌──────────────────────────────────────────────────────────────────┐
│ Folio | [workspace name]                   [Search] [Palette]     │
├───────────────────┬──────────────────────────────────────────────┤
│ EXPLORER          │ [report.md] [todo.md •] [config.yaml]        │
│ project/          ├──────────────────────────────────────────────┤
│  ▸ .agent/        │     [Edit] [Preview] [Split]                  │
│  ▾ docs/          │                                              │
│    report.md      │  Markdown renderer / text editor            │
│  .env             │  (split: editor left, preview right)         │
│  todo.md          │                                              │
├───────────────────┴──────────────────────────────────────────────┤
│ Markdown | UTF-8 | LF | Saved   •   line/column                   │
└──────────────────────────────────────────────────────────────────┘
```

- Sidebar is collapsible and resizable. Single document area. No secondary panels until needed for search.
- Tabs show basename and dirty dot; tooltip shows full path. Do not animate tabs or background decorations.
- Status bar is terse: file type, newline format, save state, cursor position.
- Keep per-document cursor and scroll positions while tabs remain open.
- If reopened after restarting, restore the last workspace and active tab **lazily**, not every prior buffer.
- Markdown-only controls appear only for Markdown files. Non-Markdown files never incur Markdown rendering.
- Use system theme; optional manual light/dark override. No theme/plugin framework.

### Keyboard shortcuts

| Shortcut | Action |
|---|---|
| `Cmd/Ctrl+P` | Quick Open file (fuzzy filename/path) |
| `Cmd/Ctrl+Shift+P` | Command palette (fuzzy actions) |
| `Cmd/Ctrl+Shift+F` | Search content in workspace |
| `Cmd/Ctrl+F` | Find within current editor (use MyGo editor built-in) |
| `Cmd/Ctrl+O` | Open File |
| `Cmd/Ctrl+Shift+O` | Open Folder (or use menu if platform conflict) |
| `Cmd/Ctrl+N` | New file (prompt for filename/location) |
| `Cmd/Ctrl+S` | Save current file |
| `Cmd/Ctrl+W` | Close current tab (prompt if dirty) |
| `Cmd/Ctrl+Tab` | Next tab |
| `Cmd/Ctrl+B` | Toggle sidebar |
| `Esc` | Close overlay; restore previous focus |

Check shortcut conflicts against actual MyGo editor bindings; application commands must win while the corresponding overlay is active. Maintain native macOS Command / Linux and Windows Control conventions.

### Command palette

- One small, **ephemeral** overlay containing a text input and virtualized/ranked action list; no separate window.
- Open by `Cmd/Ctrl+Shift+P`, focus the search box immediately, arrow keys move selection, Enter executes, Escape closes, return focus to previous editor/explorer.
- Scope only registered actions. Fuzzy match title, alternate keywords, and category; prefix and exact-word matches rank higher. Do not require an external fuzzy-search dependency.
- Initial commands: `Open File`, `Open Folder`, `Quick Open`, `Search in Workspace`, `New File`, `New Folder`, `Rename`, `Move`, `Delete`, `Save`, `Close Tab`, `Toggle Sidebar`, `Refresh Explorer`, `Edit View`, `Preview View`, `Split View`, `Toggle Word Wrap`, `Go to Line`, `Reveal in File Manager`, `Settings`.
- Show shortcuts alongside commands. Disabled/unavailable actions should be hidden or explained.
- No command search history or telemetry required.

### Quick Open

- `Cmd/Ctrl+P` opens a similar overlay, but results are workspace file paths (not commands).
- Fuzzy match filenames, then paths; strongly prioritize basename exact/prefix matches and recently opened files. Show relative directory as secondary text.
- Do **not** crawl the whole workspace during app startup. On first Quick Open, start a cancellable asynchronous scan and show partial results progressively. Cache relative file paths in memory, invalidate on folder actions and watched changes. Rescan on explicit Refresh or workspace change.
- Ignore tool-generated directories by default: `.git`, `node_modules`, `.venv`, `vendor`, `dist`, `build`, `.next`, `target`, `__pycache__`. Do not ignore `.env` or note/memory Markdown files.
- If a workspace is huge, surface partial results and a scanning indicator; never freeze the UI. Do not follow directory symlinks during traversal.
- When there is no workspace, show recent manually opened files; Open File remains available.

### Workspace content search

- A **separate on-demand feature** from Quick Open. Only run when invoked; stop when dismissed or superseded.
- Scan allowed text files, return line number and excerpt, click to open at position.
- Default to simple case-insensitive substring search; optional case-sensitive switch. Regex is not needed in v0.1.
- Respect ignore directories and configurable file-size limit. Never read binary files into search results.
- Include current unsaved buffer for open files instead of stale disk contents.
- Stream results incrementally through a bounded channel; cap displayed results (e.g., 500) and show truncation.
- No persistent search index, no SQLite, no external search subprocess.

## 6. MyGo technology choices (verified references)

Use **native UI only** with the pinned MyGo version. Verified native components/APIs include:

| Purpose | MyGo mechanism | Docs |
|---|---|---|
| Window and app lifecycle | `mygo.App`, `mygo.NewWindow`, `ui.View` | [Native UI](https://mygo.egoist.dev/docs/ui) |
| Efficient explorer | `ui.Outline`, `ui.OutlineState` | [Outline](https://mygo.egoist.dev/docs/ui/outline) |
| Editor | `editor.NewBuffer`, `editor.State`, `editor.View`, `editor.LanguageFor` | [Code editor](https://mygo.egoist.dev/docs/ui/code-editor) |
| Markdown | `markdown.View`, `markdown.Options`, `markdown.FileImages` | [Markdown](https://mygo.egoist.dev/docs/ui/markdown) |
| Keyboard and focus | `c.Shortcut`, `ui.Handle`, focus APIs | [Input](https://mygo.egoist.dev/docs/ui/input) |
| File observation | `mygo.WatchFiles` (scoped, not full-tree by default) | [Desktop APIs](https://mygo.egoist.dev/docs/native) |
| Performance tools | MyGo UI inspector / Tester benchmarks | [Native UI performance](https://mygo.egoist.dev/docs/ui/performance) |

Prefer **`ui.Outline` over `ui.Tree`** for the explorer because `ui.Tree` builds every visible item while `ui.Outline` builds only rows currently in view. Opened directories should load their children lazily; outline virtualization does **not** by itself make disk traversal lazy.

The editor buffer uses a rope and keeps versions/undo data; it handles syntax highlighting and efficient visible-line rendering. `editor.Buffer` preserves line-ending style on write, but **BOM behavior, atomic-save behavior, and state after concurrent edits must be verified or handled by Folio**.

The Markdown renderer supports CommonMark, GitHub tables, checklists, fenced-code highlighting, local images, and incremental updates. **Raw HTML is not an HTML-executing browser preview**, and mathematical notation is not guaranteed to be fully typeset; do not promise browser-equivalent rendering.

The native window should settle without requesting new frames when untouched. Any continuously running animation or timer added by Folio is a regression.

### Initialize

```bash
# See the upstream docs for supported Go versions and platform requirements.
go run github.com/egoist/mygo/cmd/mygo@latest init -template native folio
cd folio
go tool mygo dev
```

Once initialized: **pin and record a tested MyGo release** and run reproducible builds. At specification time the upstream docs require Go 1.27+, and current getting-started material states Go 1.27.3+. Verify with the chosen release.

**Do not copy unverified snippets from earlier discussions into production.** Use the pinned package documentation and compiler as authority.

## 7. Proposed repository structure — intentionally small

```text
folio/
├── cmd/folio/main.go            # App entry point / native window
├── internal/app/
│   ├── app.go                   # State, wiring, lifecycle
│   ├── view.go                  # Window layout, tabs, explorer, preview
│   ├── palette.go               # Command palette + Quick Open UI
│   └── actions.go               # Command dispatch / shortcuts
├── internal/documents/
│   ├── manager.go               # Open buffers, tabs, dirty state
│   ├── save.go                  # Save/conflict handling
│   └── recovery.go              # Recovery snapshots for safe files
├── internal/workspace/
│   ├── workspace.go             # Root, directory listing, path checks
│   └── watch.go                 # Event-driven scoped watchers
├── internal/search/
│   └── search.go                # Cancellable filesystem search
├── internal/preferences/
│   └── preferences.go           # Small JSON settings file
├── go.mod
├── mygo.json                    # If emitted by native template
└── spec.md
```

Add files/packages only when they clarify a real dependency boundary. Place Go tests beside the code they test (`*_test.go`); do not create a second mirrored `tests/` tree unnecessarily.

### Package boundaries

- `app`: **only** layer creating MyGo widgets or managing their focus. Uses the other services.
- `documents`: desktop editing session; owns `editor.Buffer` and per-tab `editor.State`; communicates with filesystem through small service functions.
- `workspace`: ordinary Go path/dir/list/move/rename behavior; no MyGo UI imports.
- `search`: ordinary Go on-demand file search; no UI imports; receives buffers for unsaved-file overlays via a narrow interface.
- `preferences`: standard library JSON and OS application-config directory; no workspace content stored.
- Future MCP could reuse `workspace`/`search` without importing `app` or spawning the GUI.

Avoid interface-heavy dependency injection. Prefer explicit constructors and simple concrete structs until tests require an interface.

## 8. State, data flow, and safety

### Document state (conceptual)

```go
// Conceptual model, not a compile-verified API contract.
type Document struct {
    Path            string
    Buffer          *editor.Buffer
    Editor          *editor.State
    Kind            DocumentKind // markdown, code, plain
    ViewMode        ViewMode     // edit, preview, split
    SavedFingerprint Fingerprint // last disk version we accepted
    ExternalChange  bool
    RecoveryAllowed bool
    // Cursor/scroll position managed by Editor state.
}
```

- Exactly one editable buffer per open document. The Markdown preview reads **the current buffer**, never an independent editable copy.
- For a file opened via two paths or symlinks, canonicalize identity sufficiently to avoid accidentally creating two conflicting editable sessions; don't silently follow directory symlinks in search.
- The saved fingerprint should include a content hash for conflict verification, optionally using size/mtime as a cheap first check. Detect changes from external agents writing or atomically replacing files.
- Each background operation includes a request/version identifier; discard results for a previous workspace, query, or closed tab.
- Go UI mutations occur through MyGo's supported main-window update mechanism. Editor snapshots may be read from worker goroutines according to MyGo's documented editor API.

### Saving policy

**Default = manual save**, especially for `.env` and configs. Users must not accidentally trigger an agent, dev server, or other watcher by merely typing.

1. On edit, mark the tab dirty; keep changes in memory and undo history.
2. On `Save`, snapshot the buffer and compare the disk file with the last accepted disk fingerprint.
3. If disk changed, **do not replace it automatically**. Offer **Reload**, **Compare/View disk version**, **Save As**, and an explicit **Overwrite** option. Provide a basic text comparison or file copy display if a full diff view is not ready.
4. On no conflict, save through a same-directory temporary file with restrictive permissions; preserve existing mode and line endings/BOM where applicable; flush and replace using OS-appropriate operations. Handle Windows replace/lock failures explicitly. Do not truncate the original before a successful write.
5. Update saved fingerprint and mark clean only if the currently edited buffer still matches the snapshot saved. If the user typed during I/O, keep the later changes dirty.
6. A watcher event caused by our own save must not become a false conflict; reconcile by expected fingerprint.
7. Failed save must leave the original intact whenever possible, keep dirty state, and show actionable error text.

Atomic rename is not a universal compare-and-swap: an outside writer can race the final replace. Recheck immediately before replacement and minimize the window, but **do not claim perfect conflict prevention across all filesystems**. Never silently discard an external change that is already known.

### Recovery versus secrets

- Recover ordinary **non-sensitive** dirty drafts after crashes. Create debounced recovery snapshots **only following edits**; do not wake periodically during idle. Ensure recovery files are private (`0600` where applicable) and remove them once the document is safely saved or discarded.
- Default **no on-disk recovery snapshot** for sensitive-looking files (`.env`, `.env.*`, private key/certificate/credential/secret names). Show a clear but restrained indication that recovery is disabled for these files; offer explicit Save/Save As, and confirm unsaved edits on close/quit.
- Do not log file contents, copied selections, search snippets, or secret values. Keep logs disabled by default unless troubleshooting is explicitly requested.
- Preferences may store file paths and UI state, **never note contents or secrets**.
- Don't automatically upload, analyze, execute, or share any opened file.

### External changes

- Watch **open documents**, and where practical directories currently expanded in the explorer. Do **not** recursively watch the entire workspace by default.
- Debounce bursts; verify meaningful changes with fingerprint before processing. If document is clean, reload from disk (preserving view position if possible); if dirty, show a non-modal conflict indicator and offer resolution.
- If OS watch setup fails or reaches its limit, keep editing functional and allow explicit Refresh/reload; never start a CPU-intensive polling loop.
- Closing tabs and changing workspaces must cancel stale watches and background operations.

### File operations / path rules

- Create, rename, move, delete only within the selected workspace unless a single outside file was explicitly opened. Outside-file tabs allow normal Save/Save As but no implied folder-wide destructive operations.
- Prevent accidental overwrite and confirm destructive operations. Use the OS Trash where the platform provides a supported safe API; otherwise clearly label permanent deletion and confirm.
- Reject traversal paths, symlink escape during workspace-wide destructive operations, folder-into-itself moves, and unsafe destination collisions.
- Rename/move updates open document paths, tab labels, recent paths, and cached search entries; prompt if dirty data would be compromised.
- File permissions and I/O failures are surfaced, not hidden. Do not try to change filesystem permissions automatically to gain access.

### Markdown safety

- Use native MyGo Markdown, not an embedded browser.
- Relative image URLs resolve from the document's directory; ensure resolved image paths remain within the explicitly opened file's authorized area/workspace. Never load remote images automatically.
- Resolve relative `.md` links inside the workspace and open in tabs. For external URLs use native browser handling with safe schemes; do not execute local scripts or commands.
- Markdown preview is display-only; no live HTML/JS or executable code fences.
- No Mermaid, diagram generation, syntax server, or math engine in MVP.

### Large files and encoding

- Small text files should feel instant. Use the editor's efficient rope and visible-line rendering.
- For Markdown files over **2 MiB**, default to Edit mode and disable automatic split/preview until explicitly requested; warn that rendering may be expensive.
- For files over **20 MiB**, show a file-size warning before loading. Opening may be supported if measurements are acceptable, but no special huge-file infrastructure is required for v0.1.
- Detect binary-like content (e.g., NUL bytes) and unsupported encodings; show a clear unsupported-file message, never write back corrupted text.
- Fully support UTF-8 and UTF-8 with BOM. Preserve LF vs CRLF when saving and preserve whether the file ended with a newline.

## 9. Performance contract (engineering targets, not guarantees)

**Measure on a recorded reference machine and OS**, with a production build and representative local SSD workspace. Targets below are **provisional release gates**; verify and adjust for platform differences based on evidence, never substitute MyGo marketing benchmarks for application measurements.

**Reference workload:** ~1,000 text/Markdown files across 50 folders; one 50 KiB Markdown report, one `.env`, one 100 KiB code/config file open; five tabs at most; no active user input; indexing/initial scans settled.

| Metric | Initial target | Measurement |
|---|---|---|
| Idle CPU | **<0.5% of one logical core on average** over 10 minutes after settling | OS per-process CPU, foreground and minimized |
| Idle network | **Zero requests** | Firewall/network inspection |
| Warm launch to usable window | **<=1.5 s** on reference SSD machine | Repeated median after process exit |
| Resident memory with reference workload | **<=150 MiB desired**, with OS-specific deviations documented | OS RSS/private footprint; include all Folio processes |
| Memory after 24h idle | No monotonic leak; **<=20 MiB sustained increase** from stabilized baseline after comparable GC/state | 24h process memory samples + heap profiles |
| Quick Open appearance | **<=100 ms** to focused overlay; results may continue loading | Instrument input-to-frame latency |
| Quick Open cached query | **<=100 ms** typical for 1,000 cached paths | Benchmark fuzzy matcher + UI |
| Typing in 100 KiB text | No perceptible stalls; investigate >50 ms UI stalls | Input-to-frame trace |
| Explorer scrolling | Only render visible rows | UI instrumentation |
| Idle wakeups | No application-owned periodic timers/animations | CPU trace / goroutine and timer inspection |

The measurements are acceptance goals, not a claim that the current source meets them. Re-run on at least one additional platform before claiming general cross-platform performance. A temporary high CPU cost during an **explicit** search or file-opening operation is acceptable if cancellable and bounded.

### Implementation constraints to meet targets

- Never continuously redraw a window. Let MyGo's event-driven native UI sleep when settled.
- Never run full recursive workspace scanning at startup. Directory listing is lazy; Quick Open scanning is on demand.
- No persistent full-text search index, background AI client, sync timer, hidden HTTP service, or frequent save tick.
- Markdown preview only exists for the **active Markdown tab** when Preview/Split is visible. Do not render previews for background tabs.
- Cache only lightweight path metadata, bounded visible-row state, and necessary open buffers. Free preview assets and search result buffers when no longer used.
- For search, use bounded concurrency (e.g., one directory walker plus a small fixed number of file readers), cancellation via `context.Context`, and bounded result channels. Do not spawn one goroutine per file.
- Show partial results or a spinner rather than blocking UI for disk work. Avoid unnecessary `Buffer.String()` on every redraw if no buffer version changed.
- Keep menu and overlay transitions unanimated or minimal. No refresh/polling loop simply to update an idle status bar.
- If idle CPU exceeds the budget, profile timers, animations, repaint triggers, watchers, and goroutines **before** adding new features.

## 10. Architecture/data flow

```text
Native MyGo window (single event-driven UI)
  ├── Explorer (lazy ui.Outline)
  ├── Tabs + active editor (editor.State/Buffer)
  ├── Markdown preview (active tab only)
  └── Overlays (Command Palette / Quick Open / Search)
           │ command dispatch, snapshots, cancelable requests
           ▼
Application coordinators
  ├── DocumentManager (tabs, dirty/recovery/conflicts)
  ├── WorkspaceService (directory listing + file actions)
  ├── SearchService (on-demand filesystem walking)
  └── Preferences (small JSON file)
           │
           ▼
Local filesystem (original workspace files)
```

**No database, webview, IPC daemon, or companion process.** The search layer and workspace layer remain independent from the UI so an optional future MCP binary can use them without loading or starting a window.

## 11. Planned later, not MVP

**v0.2 (if daily use demonstrates need):** pinned/recent files, optional quick-capture note, better in-file navigation, Markdown backlinks, read-only comparison of external edits using MyGo's diff viewer, user-configured ignored directories.

**v0.3 (optional MCP):** separate opt-in `folio-mcp` process based on the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk), first exposing `list_files`, `read_file`, and `search_files` over stdio. No MCP process runs with the GUI by default. Restrict to explicit workspace roots; **exclude `.env`, credentials, private keys, and other sensitive patterns from MCP access by default**, even though the human UI can read them. Later writes require explicit per-action approval and diff review. Do not expose unsaved editor buffers automatically.

**MCP is not needed for agents to maintain their memory.** Existing agents can write standard Markdown files, and Folio can read/review those files immediately as ordinary workspace documents.

## 12. Build sequence and deliverables

Implement vertical slices; keep the application runnable after each milestone.

### M0 — Boot
- [ ] Initialize MyGo native project; pin tested Go/MyGo versions.
- [ ] Single window with inert sidebar, content area, status bar.
- [ ] Basic error notification and theme behavior.
- [ ] Verify app idles without periodic repainting.

### M1 — Open and view
- [ ] Open Folder and Open File.
- [ ] Lazy workspace explorer using `ui.Outline`.
- [ ] Open tabs and text/code editor with syntax highlighting.
- [ ] Markdown Preview/Edit/Split, with relative local images.
- [ ] Restore last workspace and active file lazily.

### M2 — Correctness of editing
- [ ] Dirty indicators, Save/Save As, unsaved-close prompts.
- [ ] UTF-8/BOM/newline/permission handling.
- [ ] External-change detection and conflict handling.
- [ ] Crash recovery of permitted non-sensitive drafts; safe exclusions for `.env`.
- [ ] Create, rename, move, delete, with path safety.

### M3 — Keyboard productivity
- [ ] Command palette (`Cmd/Ctrl+Shift+P`), keyboard-first.
- [ ] Quick Open (`Cmd/Ctrl+P`), cancellable scan and fuzzy results.
- [ ] Workspace content search (`Cmd/Ctrl+Shift+F`), streamed results.
- [ ] Ensure editor shortcuts and command shortcuts don't collide.

### M4 — Hardening and release
- [ ] Windows/macOS/Linux build sanity as available.
- [ ] Unit tests for document save and path safety, search cancellation, palette ranking.
- [ ] Integration exercises for external agent writes and `.env` edits.
- [ ] 10-minute idle CPU/memory test and 24-hour leak/idle test on reference machine.
- [ ] Package/document how to run, known platform differences, and unsupported formats.

Deliver: working source repository, passing tests/build instructions, screenshots of Edit/Preview/Split and palettes, and a short `PERFORMANCE.md` listing observed measurements and machine specs. Do not silently declare work complete when tests or measurement are skipped.

## 13. Acceptance tests / definition of done

### Functional
- [ ] Open a folder with `report.md`, `todo.md`, `.env`, `config.yaml`, `AGENTS.md`; files display correctly, including dotfiles.
- [ ] Opening `report.md` shows readable GFM headings, lists, checkboxes, tables, code fences, and local images; no network fetch.
- [ ] Switching Edit/Preview/Split displays current unsaved Markdown edits without manually reopening the document.
- [ ] Editing `.env` does **not** modify disk before explicit Save; after Save, it preserves newline/permissions and the values written.
- [ ] `Cmd/Ctrl+P` finds a nested file by partial name and opens it; `Cmd/Ctrl+Shift+P` runs `Toggle Sidebar`; Escape restores prior focus.
- [ ] `Cmd/Ctrl+Shift+F` finds a phrase in files and opens the correct file and line; cancelling stops work promptly.
- [ ] Opening a large folder does not force full recursive enumeration before the UI becomes usable.
- [ ] Renaming/moving an open file updates its tab/path without losing its dirty buffer.
- [ ] A dirty document prompts on close/quit; crash recovery works for `todo.md` but does not write a recovery snapshot for `.env`.
- [ ] If an agent rewrites an open clean report, UI offers/does reload updated content; if a file is dirty, it does not overwrite in-memory edits automatically.
- [ ] Unknown binary or invalid-UTF-8 data is refused safely; no corruption on save.
- [ ] App does not create a vault, hidden metadata directory, or index inside the user's workspace without consent.

### Reliability/security
- [ ] Test LF, CRLF, BOM, final newline, permissions, failed writes, destination collisions, and disk permission errors.
- [ ] Test symlink and `../` path escapes for destructive workspace commands.
- [ ] Test external write occurring before save; known conflict blocks unconfirmed overwrite.
- [ ] Test a save while the user continues typing; newer edits remain marked dirty.
- [ ] Test repeated workspace switching/cancelled scans for goroutine/watch leaks.
- [ ] Verify no secret content appears in app logs, preferences, recovery snapshots, or external requests.
- [ ] Verify preview links never execute arbitrary scripts and remote images are not loaded.

### Performance
- [ ] Verify CPU/memory/launch/Quick Open metrics in §9 with production build; record methodology and any misses.
- [ ] Let the app sit foreground and minimized for 10 minutes; inspect CPU and wakeups.
- [ ] Run a 24-hour idle soak test (with a few opened files); check memory trend, goroutines, watches, and open file descriptors/handles.
- [ ] Test a workspace with 10,000 nested file paths; Quick Open may scan progressively but the UI must remain responsive.
- [ ] Test repeated search/cancel and file edits while content search runs; no deadlocks or stale UI results.

## 14. Open implementation decisions (resolve simply; do not block the MVP)

1. **OS first:** implement/test on the coding agent's available desktop OS, but keep platform-specific code isolated and list known untested targets.
2. **File deletion:** use OS Trash if MyGo or a small platform adapter provides a safe documented API; otherwise explicit permanent-delete confirmation. Do not add a large dependency solely for Trash support.
3. **Recovery cadence:** debounced after edits (e.g., 10–20 seconds of activity/quiescence); choose based on measured I/O and never tick while untouched.
4. **Large Markdown:** active preview may be explicitly enabled for big files, but it must not run in the background or block typing.
5. **Warnings:** keep dialogs rare; make errors precise and visible, especially for writes and conflicts.

## 15. Reference material (official sources)

- MyGo project: https://github.com/egoist/mygo
- MyGo docs: https://mygo.egoist.dev/docs
- Native UI: https://mygo.egoist.dev/docs/ui
- Editor: https://mygo.egoist.dev/docs/ui/code-editor
- Markdown: https://mygo.egoist.dev/docs/ui/markdown
- Virtualized outline: https://mygo.egoist.dev/docs/ui/outline
- Native input and shortcuts: https://mygo.egoist.dev/docs/ui/input
- File watching: https://mygo.egoist.dev/docs/native
- Architecture and idle behavior: https://mygo.egoist.dev/docs/architecture
- Native UI performance: https://mygo.egoist.dev/docs/ui/performance
- MCP Go SDK (future only): https://github.com/modelcontextprotocol/go-sdk

---

**Final rule:** When deciding between a clever feature and an app that stays fast and quiet for days, choose the fast and quiet app. A dependable Markdown reader, config editor, file browser, Quick Open, and command palette are a complete MVP.
