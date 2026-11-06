# Retain the last session

Remember the workspace, active file, sidebar, and Markdown mode without restoring
all old buffers or writing their drafts to disk.

## Sub-features

- SE1. Keep per-tab buffers when switching.
- SE2. Toggle the sidebar with Command/Control+B.
- SE3. Confirm before discarding unsaved edits.
- SE4. Restore only root metadata and the last active file on restart.

## How to get to it (user POV)

Switch tabs, toggle the sidebar, close the window, choose Cancel or Discard in the
quit prompt, then launch WonderVision again.

## Driving it with the native harness

Preconditions: the common run has unsaved report and `.env` edits.

- Run `python3 scripts/verify_native.py run --out build/verification/session`.
- The runner toggles the sidebar, verifies the tree disappears and returns,
  requests a native window close, and selects `Cancel`.
- It closes again, selects `Discard`, and starts the same binary with
  the same private profile. `restored.json` must show exactly one report tab,
  the original on-disk heading, and no expanded `next.md` Explorer row.
- Inspect `before-quit.png`, `restored.png`, `filesystem-check.json`, and
  `cleanup.json` alongside the recorded actions.

## Gotchas

Crash recovery and external-change detection belong to M2 and are not tested here.
A rapid-quit preference flush has its own behavioral test. The runner's final
cleanup terminates only its last clean verification process and preserves proof.
