# Changelog

## 0.1.0

First release, for Linux (amd64 and arm64). macOS and Windows builds compile
but have not been run; see the README's platform table.

- Native single-window workspace: Open Folder or Open File, a lazy Explorer
  that shows dotfiles, tabs, and an editor with syntax highlighting, find and
  replace, word wrap and Go to Line.
- Markdown Edit, Preview and Split on the same live buffer: GitHub Flavored
  Markdown, local images and links only; remote images and scripts are never
  loaded.
- Safe editing: edits stay in memory until Save; saves keep line endings, a
  UTF-8 BOM, the final newline and the file mode; external changes reload
  clean files and never overwrite dirty ones; conflicts need an explicit
  choice.
- Crash recovery for non-sensitive drafts, never for `.env`, keys,
  certificates or credential files.
- New, Rename, Move and Delete inside the workspace, refusing symlink and
  `../` escapes.
- Keyboard: command palette (Ctrl+Shift+P), Quick Open (Ctrl+P) and workspace
  search (Ctrl+Shift+F), all cancellable.
- No network access, no telemetry, no logs, and nothing written inside the
  workspace except the files you save.

Known limitations:

- UTF-8 text only; files over 20 MiB are refused, and Markdown over 2 MiB
  opens in Edit.
- Replace All in the find bar is the button only: MyGo's Ctrl+Enter shortcut
  for it does not fire.
- Preview memory grows with document length (about 30 MB for a 50 KiB
  report); see PERFORMANCE.md.
