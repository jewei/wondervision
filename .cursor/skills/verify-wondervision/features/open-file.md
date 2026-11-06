# Open a standalone file

Open a real UTF-8 file without choosing a workspace.

## Sub-features

- OF1. Choose an ordinary file through the native Open File dialog.
- OF2. Keep folder operations scoped while a standalone tab is open.

## How to get to it (user POV)

Click `Open File` or press Command/Control+O. Select `standalone.txt`.

## Driving it with the native harness

Preconditions: an authorized X11 desktop, empty verification profile.

- Run `python3 scripts/verify_native.py run --out build/verification/open-file`.
- The runner clicks the `Open File` button, chooses its standalone fixture in
  GTK, and requires a `standalone.txt` tab with `No folder open` still visible.
- Inspect `standalone.png`, `standalone.json`, and the preceding dialog action
  in `actions.jsonl`. The runner repeats the open through Command/Control+O and requires one tab for the same file.

## Gotchas

Both toolbar and keyboard dialog entry points are driven. Binary/invalid UTF-8 refusal
and size limits are covered by `go test ./internal/documents`, not this fixture.
