# Save, resolve conflicts, and recover drafts

Run `python3 scripts/verify_editing.py --out build/verification/editing-run` with
the built `e2e` binary and authorized X11 display. The output directory must be
new. No real credentials, user workspace or existing application is used.

The driver creates BOM/CRLF `.env` fixture data with mode 0640, opens a folder
through GTK, and edits using native accessibility actions. It checks that typing
leaves disk unchanged, Ctrl+S preserves format/mode, and Cmd/Ctrl+Shift+S opens the
real Save As chooser, creates its chosen copy, retargets the tab and refreshes
Explorer. `manual-save-dirty.png` and `filesystem-check.json` record the proof.

It writes the open `.env` from outside the application: clean text reloads, dirty
text remains intact, Compare opens a read-only disk view, and an overwrite prompt
cannot approve a later unreviewed disk version. A fresh approval succeeds.
Inspect `external-conflict.png`, `disk-comparison.png`, `conflict-resolved.png`
and the corresponding action/tree records.

The File popover drives New Folder, New File, Rename, Move and Delete Permanently.
The Path field accepts a relative workspace destination. Rename/move must retain
the dirty buffer while the file remains empty on disk. Close cancellation and
delete cancellation retain it. Confirmed deletion removes it and closes the tab.
`file-operations.png` shows the retained buffer under the new path. Rooted path,
symlink, collision, cancellation and exclusive-rename behavior also have Go tests.

The driver edits `todo.md` and `.env`, waits for the 15-second edit-triggered
recovery, verifies private recovery for only TODO, and kills only its own process.
Restart must offer **Recover drafts**. Recover restores unsaved text for review
without saving the workspace file; explicit Save writes it and removes the copy.
Finally, **Save** in the quit prompt must settle all writes and cleanup before exit. See
`recovery-review.png`, `recovered-draft.png`, `result.json` and `cleanup.json`.

`python3 scripts/verify_recovery.py` separately checks old-session ownership,
concurrent-session exclusion, recovered Reload/discard/rename lifetimes and quit
on cleanup failure. Removing the write lock in a disposable overlay must reproduce
the actual lost-draft race. The original code must pass again. These are empirical
checks, not a formal recovery proof. On Linux/BSD/macOS the nonblocking advisory
lock is released by the OS on exit; unsupported targets fail closed. Contention
reports an error and needs another edit to retry, rather than periodic polling.

Stable native names include File (menuButton), Save, Save As, Editor, Path,
Overwrite, Overwrite Disk, Disk version, Close comparison, Recover drafts,
Recover, Discard recovery, the quit prompt's Save, and Close followed by the tab basename.
GTK Save As uses the **Save** accept button; Open dialogs use **Open**. A settled
frame alone does not mean I/O completed; wait for the intended tab, bytes or view.

Cleanup is shared with the native verification skill: the runner's `finally`
terminates only its own last process, removes its private scratch tree and keeps
evidence. Its `instance.json` is accepted by `verify_native.py cleanup` if the
driver was interrupted. Never kill application processes by name.
