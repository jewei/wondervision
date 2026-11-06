# Browse a workspace

Choose a folder and inspect its dotfiles and nested files without a startup crawl.

## Sub-features

- EX1. Open Folder through its native dialog.
- EX2. Show `.env` and ordinary Markdown/configuration files.
- EX3. Expand `docs` on demand.

## How to get to it (user POV)

Click `Open Folder` or press Command/Control+Shift+O. Expand `docs` in Explorer.

## Driving it with the native harness

Preconditions: the standalone stage has completed in the common run.

- Run `python3 scripts/verify_native.py run --out build/verification/explorer`.
- The runner presses `CmdOrCtrl+Shift+O`, chooses its workspace, requires `.env`,
  repeats Open Folder through the toolbar, expands the accessible `docs` tree item, and waits for `next.md`.
- Read the dialog trace, expansion action, and resulting tree in `actions.jsonl`.
  `preview.png` shows the opened workspace.

## Gotchas

This run drives both shortcut and toolbar. Unit tests prove the root listing is shallow;
`restored.json` proves a restart does not expand descendants. Directory symlinks
are not expandable. Explicit Refresh reloads root metadata and resets expansion.
