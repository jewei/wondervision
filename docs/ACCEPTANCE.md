# Acceptance: spec §13 traced to evidence

Each item of the spec's definition of done (`spec.md` §13) with the checks that
cover it. "Go" tests run in `go test -race ./...`; "GUI" verifiers drive the
real e2e binary through AT-SPI and X11 under Xvfb (`scripts/verify_*.py`);
"guardrail" tests in `internal/guardrails` fail the build on a forbidden code
shape; "TLC" models live in `verification/`. All of these run in CI
(`.github/workflows/verify.yml`) on a pull request that changes code, except
the performance and soak jobs, which run on demand. A docs-only change does
not start a run. macOS tests and the other-OS builds run on a push to `work`,
on a manual run, and on a pull request that changes platform-specific code.

Status: **met** = covered by a check that runs and passes; **partial** = covered,
with a stated limit; **open** = not yet done.

## Functional

| # | Item | Evidence | Status |
| --- | --- | --- | --- |
| F1 | Open a folder with `report.md`, `todo.md`, `.env`, `config.yaml`, `AGENTS.md`; dotfiles shown | GUI `verify_native.py` (Open Folder dialog, Explorer rows incl. `.env`); Go `TestListIsShallowAndShowsDotfiles`, `TestOpenViewAndLazyExplorer` | met |
| F2 | `report.md` shows GFM and local images; no network fetch | GUI `verify_native.py` (Preview of headings, lists, task items, table, code fence, local image); Go `TestLocalImagesAndMarkdownLinks` (refuses `https:`, `file:`, `../` and `data:` images); guardrail `TestNativeFilesystemAndIdleBoundaries` (no `net`, `net/http` or `os/exec` import); measured 0 Internet sockets (PERFORMANCE.md, M4) | met |
| F3 | Edit/Preview/Split show current unsaved edits | GUI `verify_native.py` (types into the editor, switches modes, reads the preview) | met |
| F4 | Editing `.env` doesn't touch disk before Save; Save keeps newline, permissions and values | Go `TestOpenTextAndEditOnlyInMemory`, `TestSavePreservesTextFormatAndPermissions`; GUI `verify_native.py` (workspace hashes unchanged after typing), `verify_editing.py` (BOM, CRLF, final newline, mode 0640 after Save) | met |
| F5 | Ctrl+P opens a nested file; Ctrl+Shift+P runs Toggle Sidebar; Escape restores focus | Go `TestQuickOpenFindsNested…`, `TestPaletteRunsToggleSidebar…`, `TestEscapeReturnsFocusToTheEditor`, `TestEscapeClosesTheOverlayWithAQuery`; GUI `verify_m3.py` (real X11 keys) | met |
| F6 | Ctrl+Shift+F opens the right file and line; cancel stops promptly | Go `TestContentSearchOpensTheMatchingLine…`, `TestGrepCancellationReturnsPromptlyWithoutLeaks`; GUI `verify_m3.py` (lands on line 3, column 14) | met |
| F7 | A large folder is usable before full enumeration | Go `TestListIsShallowAndShowsDotfiles` (one level per listing), `TestOpenViewAndLazyExplorer`; Quick Open scans lazily on first use; measured 10,000 paths: first scan 0.10 s, slowest UI answer during it 4 ms (PERFORMANCE.md) | met |
| F8 | Rename/move of an open file keeps its tab and dirty buffer | Go `TestFileActionsRetainDirtyBufferAndUpdatePaths`; GUI `verify_editing.py` | met |
| F9 | Dirty documents prompt on close/quit; recovery covers `todo.md`, never `.env` | GUI `verify_native.py` (Alt+F4 prompt, Cancel keeps the app), `verify_editing.py` (crash and recover `todo.md`); Go `TestSensitiveRecoveryNeverWrites`, `TestWriteRecoveryRefusesSensitiveWithoutCallerCheck`, `TestRecoveryStartsOnlyAfterEditsAndStopsAfterSave`; guardrails `TestRecoveryWritesCheckSensitiveFirst`, `TestRecoveryPlanCannotBeForged`; TLC recovery model (`verify-recovery-protocol` skill) | met |
| F10 | An agent's rewrite reloads a clean file and never overwrites dirty edits | Go `TestExternalChangeReloadsCleanAndProtectsDirty`, `TestReloadAndOverwriteRejectNewerChanges`; GUI `verify_editing.py` | met |
| F11 | Binary or invalid UTF-8 is refused; no corruption on save | Go `TestInvalidAndLargeFiles`; content search skips the same files (`TestSearchSizeLimitMatchesDocuments` guardrail keeps the limits equal) | met |
| F12 | No vault, metadata directory or index inside the workspace | GUI `verify_native.py` (SHA-256 of every workspace file, new files included, before and after the session; preferences and recovery live under the XDG directories) | met |

## Reliability and security

| # | Item | Evidence | Status |
| --- | --- | --- | --- |
| R1 | LF, CRLF, BOM, final newline, permissions, failed writes, collisions, permission errors | Go `TestSavePreservesTextFormatAndPermissions`, `TestSaveFailureKeepsOriginal` (read-only file and directory; the directory case skips as root, so it runs in CI, not in a root container), `TestSaveAsNeverOverwrites…`, `TestExclusiveRenameRejects…`, `TestFileActionsRejectEscapeCollisionAndSelfMove`; GUI `verify_editing.py` | met |
| R2 | Symlink and `../` escapes for destructive commands | Go `TestFileActionsRejectEscapeCollisionAndSelfMove`, `TestRootedOpenRejectsSymlinkSwap`, `TestSaveRejectsParentSymlinkEscape`, `TestDeletionDoesNotFollowNestedSymlinks` | met |
| R3 | External write before save; a known conflict blocks an unconfirmed overwrite | Go `TestSaveRejectsKnownConflict`, `TestReloadAndOverwriteRejectNewerChanges`; TLC save model (`verify-save-protocol` skill) | met |
| R4 | Save while typing leaves newer edits dirty | Go `TestSaveKeepsNewerEditsDirty`; TLC save model (`NoFalseClean`) | met |
| R5 | Repeated workspace switching and cancelled scans don't leak goroutines or watches | Go `TestRepeatedWorkspaceSwitchingWithCancelledScansDoesNotLeak` (30 switches, each with a Quick Open scan and a content search cancelled mid-run; goroutines return to baseline; proven to fail on a leaked goroutine per search); `TestGrepCancellationReturnsPromptlyWithoutLeaks`. File watches belong to open documents and are not countable headless; the soak runs track file descriptors, which every watch holds | partial: watches checked through file descriptors only |
| R6 | No secret content in logs, preferences, recovery snapshots or external requests | Logs: guardrail `TestNoLoggingOutsideStartupErrors` (no `log` package anywhere; no printing outside the startup error in `main.go`). Preferences: GUI `verify_native.py` (allowed keys only; recent files are paths). Recovery: F9 checks. Requests: no network imports (guardrail) and 0 Internet sockets measured | met |
| R7 | Preview links never run scripts; remote images aren't loaded | Go `TestLocalImagesAndMarkdownLinks` (refuses `javascript:` links and remote images); the preview is native MyGo widgets, not a web view, and the Linux package does not depend on WebKitGTK (CI step "Linux packages depend on GTK only") | met |

## Performance

| # | Item | Evidence | Status |
| --- | --- | --- | --- |
| P1 | §9 CPU, memory, launch and Quick Open metrics on a production build, with methodology and misses | PERFORMANCE.md, M4 section, and `docs/verification/performance-m4.json`; the CI soak settled at 155.5 MiB, over the 150 MiB target; typing in the 100 KiB file: max 19 ms per key (`docs/verification/performance-typing.json`). All targets met in the recorded run; memory with the 50 KiB report in Preview varies 100–160 MiB between runs, so one run can exceed 150 MiB | met, with a memory risk noted |
| P2 | 10 minutes foreground and minimized: CPU and wakeups | PERFORMANCE.md M4: 0.02% and 0.015% of one core over 600 s each; Go `TestShellSettlesAndFollowsTheme`; guardrail `TestNativeFilesystemAndIdleBoundaries` (no tickers or polling sleeps) | met |
| P3 | 24-hour idle soak: memory, goroutines, watches, file descriptors | `scripts/soak.py`. CI soak, 5.5 h: leak checks PASS (RSS flat at 155.5 MiB, +0.002 MiB/h; threads 14 → 14; fds 20 → 20; 0 sockets), but above the 150 MiB target on that runner (PERFORMANCE.md, `docs/verification/soak-ci.json`). A 24-hour run needs a desktop that stays up: `python3 scripts/soak.py run --out build/verification/soak-24h --hours 24` | partial: 5.5 h on CI passed; 24 h open |
| P4 | 10,000 nested paths: Quick Open may scan progressively, UI stays responsive | PERFORMANCE.md: first scan 0.10 s, slowest UI answer during the scan 4 ms | met |
| P5 | Repeated search/cancel and edits during content search: no deadlocks, no stale results | Go `TestSearchCancelAndEditLoopShowsOnlyCurrentResults` (60 edit-and-search rounds, some superseded at once, some finished with results still queued; the final results equal a fresh search; proven to fail when runs share results or drop unsaved buffers), `TestSupersededSearchResultsNeverShow` | met |

## Not covered anywhere

- macOS and Windows are compiled and vetted in CI but never run.
- No measurement on a real desktop with a GPU.
