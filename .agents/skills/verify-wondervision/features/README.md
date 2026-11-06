# WonderVision verification map

The native desktop is the primary user surface. Start every run with the isolated
workspace and empty profile created by `scripts/verify_native.py`. No account,
server, or production data is needed. Run one driver per desktop.

The runner saves each action and the resulting accessibility state. A UI claim
also needs a screenshot, and an editing claim needs the before/after file hashes.
A failed or skipped entry point stays unverified; another path cannot replace it.

- [Open standalone files](open-file.md).
- [Browse a folder](explorer.md).
- [Read and edit Markdown](markdown.md).
- [Keep a session](session.md).
- [Save, resolve conflicts, and recover drafts](editing.md), with the separate
  `scripts/verify_editing.py` native runner.
- [Work from the keyboard](keyboard.md): command palette, Quick Open and
  workspace search, with the separate `scripts/verify_m3.py` native runner.

Every recipe runs with `python3 scripts/verify_native.py run --out` followed by a
fresh directory under `build/verification/`. The common run covers the map in
sequence. Toolbar and shortcut paths are named separately below.
