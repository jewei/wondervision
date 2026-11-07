# Testing WonderVision on a Mac

CI builds the app for macOS and runs the Go test suite on macOS 15. Those
tests are headless: nothing has opened the real window on a Mac yet. The
scripted GUI verifiers (`scripts/verify_*.py`) drive X11 and AT-SPI, so they
only run on Linux. This page is the hand-run equivalent, about 20 minutes.

## 1. Get the app

Either one works.

**Build it from source** (needs Go 1.27.3 or later; `go version` to check):

```bash
git clone https://github.com/jewei/wondervision && cd wondervision
```

```bash
go test ./...
```

```bash
go tool mygo build -platform darwin/universal -o build/dist
```

```bash
open build/dist
```

**Or download the disk image CI built:** in GitHub, Actions → release → Run
workflow (on `work`), wait for it, and download the `macos-dmg` artifact.

Either way the app is only ad-hoc signed (there's no Developer ID), so a
downloaded copy is blocked the first time with "WonderVision.app Not Opened …
Apple could not verify …". Click **Done** (not Move to Trash). Since macOS 15,
right-click → Open no longer gets past this. Drag WonderVision to
Applications, then either remove the download's quarantine flag:

```bash
xattr -dr com.apple.quarantine /Applications/WonderVision.app
```

or open System Settings → Privacy & Security, find "WonderVision.app was
blocked" under Security, click **Open Anyway**, enter your password, and
confirm **Open Anyway**. A copy you built yourself is not quarantined and
opens directly.

## 2. Make a test folder

```bash
mkdir -p ~/wv-test/docs && cd ~/wv-test
```

```bash
printf '# Report\n\nLocal files, native views.\n\n- [x] Done\n- [ ] Open\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\n```go\nfunc main() {}\n```\n\n[Next](docs/next.md)\n' > report.md
```

```bash
printf '# TODO\n\n- [ ] first\n' > todo.md; printf 'API_KEY=secret\n' > .env; printf 'name: test\n' > config.yaml; printf '# Next\n' > docs/next.md
```

## 3. Checklist

Note anything that fails, looks wrong, or feels slow, with a screenshot if
you can. ⌘ is Command, ⇧ Shift, ⌃ Control.

**Window and files**

1. Open the app. It opens a window with a toolbar and an empty sidebar, in
   your system's light or dark appearance. Switch appearance in System
   Settings: the window follows.
2. Open Folder (⇧⌘O) → `~/wv-test`. The sidebar lists `docs`, `.env`,
   `config.yaml`, `report.md` and `todo.md` (the dotfile too).
3. Click `report.md`: it opens in Preview with a heading, a checklist, a
   table and a highlighted code block. Click Edit, then Split: the same text
   in both panes. Type in the editor: the preview follows.
4. In Preview, click the "Next" link: `docs/next.md` opens.

**Editing and saving**

5. Open `.env`, type a character. The tab shows a dot and the status bar
   says Unsaved. In Terminal, `cat ~/wv-test/.env`: still unchanged. Press ⌘S:
   the file now has your change, and `ls -l` shows the same permissions.
6. Edit `todo.md` without saving, then close the tab (⌘W): you are asked
   about unsaved edits. Cancel.
7. With `todo.md` still unsaved, change it in Terminal
   (`echo extra >> ~/wv-test/todo.md`): the app warns of a conflict rather
   than overwriting your edit.
8. Quit with ⌘Q while something is unsaved: you are asked first.

**Keyboard**

9. ⇧⌘P opens the command palette; type `toggle side`, Return: the sidebar
   hides. ⌘B shows it again. Escape closes the palette and typing goes back
   into the editor.
10. ⌘P (Quick Open), type `nxt`, Return: `docs/next.md` opens.
11. ⇧⌘F, type `native views`, Return, then Return on the result: `report.md`
    opens at line 3.
12. In the editor, ⌘F finds text; ⇧⌘H (or ⌥⌘F) shows Replace, where Return
    replaces one match.
13. ⌃Tab moves to the next tab. (Not ⌘Tab: that is the app switcher.)

**File actions** (new on macOS in this build)

14. Select `config.yaml` in the sidebar, Rename (F2, or fn+F2 on a laptop, or
    the palette) to `config2.yaml`. Then Move it into `docs`. Both work; an
    open tab follows the file.
15. Rename a file to a name that already exists: it refuses and nothing is
    overwritten.
16. Delete a file: it asks first, then deletes it permanently (not to the
    Trash).

**Crash recovery**

17. Edit `todo.md` and `.env` without saving, wait 20 seconds, then force quit
    (⌥⌘Esc → WonderVision → Force Quit). Reopen: it offers to recover
    `todo.md` only, never `.env`.

**Idle**

18. Leave the app open and untouched for a minute with `report.md` in
    Preview. In Activity Monitor, WonderVision's CPU should sit at 0.0–0.1%
    and its Memory under 150 MB.

## 4. What to send back

- Which steps failed or looked wrong, with screenshots.
- Your Mac: chip (Apple silicon or Intel) and macOS version.
- The Activity Monitor numbers from step 18.
- `~/Library/Application Support/wondervision/preferences.json`, which holds
  only paths and view settings, to confirm nothing else is stored there.

Recovery drafts live in `~/Library/Caches/wondervision/recovery`; you can
delete `~/wv-test` and both folders afterwards.
