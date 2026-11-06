# Read and edit Markdown

Use native GFM preview and a shared editable buffer with local images and links.

## Sub-features

- MD1. Preview headings, task lists, a table, highlighted fenced code, and a local image.
- MD2. Switch Edit, Preview, and Split while showing unsaved text.
- MD3. Follow a relative Markdown link and return to the retained tab.

## How to get to it (user POV)

Choose `report.md` in Explorer. Use the `Edit`, `Preview`, and `Split` buttons.
Click `Next document`, then the `report.md` tab.

## Driving it with the native harness

Preconditions: the seeded folder is open and the report is unmodified on disk.

- Run `python3 scripts/verify_native.py run --out build/verification/markdown`.
- The runner opens `report.md`, waits for the heading and fully loaded `Local
  image`, captures `preview.png`, switches to Edit and captures `edit.png`.
- In Split it inserts `# Unsaved heading` through the accessible editor, requires
  the heading in preview, and captures `split.png`.
- It follows the relative link and returns to the report. File hashes must match
  the initial workspace in `filesystem-check.json`.

## Gotchas

Saving belongs to M2. This build retains edits in memory and confirms at quit.
Remote images and unsafe schemes are refused; unit tests cover those boundaries.
The runner's small report does not prove large-file latency or long-term memory.
