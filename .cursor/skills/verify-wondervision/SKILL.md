---
name: verify-wondervision
description: Verify WonderVision's native Linux desktop open/view workflow after changes to dialogs, explorer, tabs, Markdown, preferences, or application lifecycle.
---

# Verify WonderVision

## Launch

Use Go 1.27.3 and the pinned MyGo v0.7.4. The primary surface is a native Go
window, with GTK file dialogs on Linux. There is no web UI or local HTTP server.

```sh
go test ./...
go build -tags e2e -o build/wondervision-e2e ./cmd/wondervision
python3 scripts/verify_native.py run --out build/verification/native-run
```

The output directory must not exist. The runner creates its own workspace,
standalone file, private preferences directory, and private Unix socket. Readiness
requires one native window titled `WonderVision`, the `Open File` control, and a
matching live executable. The runner starts from empty preferences, drives the
features, restarts once, and tears down its own process and scratch files.

Requirements are Linux X11, Python 3, libX11, libXtst, libatspi, `xwininfo`, and `xprop`. Set `DISPLAY`
and `XAUTHORITY` to the authorized desktop before running. Managed desktops can
require their designated input authority for synthetic keyboard events. A
`WV_INPUT_XAUTHORITY` selector can specify that authority separately. Use the
configured authority file as a selector; never read or print its contents.

Only the `e2e` build imports MyGo's automation plugin. Production is built with
`go build -tags mygo_noinspector -trimpath -o build/wondervision ./cmd/wondervision`
and contains no automation listener. Test automation uses a Unix socket, no port.
Instances have separate profiles and data, but the desktop keyboard is shared.
Run only one native driver at a time and never drive a user's existing instance.

## Doctor

For a running verification instance, use this read-only identity check.

```sh
python3 scripts/verify_native.py doctor --out build/verification/native-run
```

It checks the PID, executable, private socket owner/mode, native window identity,
and usable controls. The full runner performs it before driving. After automatic
cleanup the live check should fail; inspect the saved `doctor.json` and
`cleanup.json` instead. A dead or mismatched instance is not worth driving.

## Drive

Read [the feature map](features/README.md). The runner drives all mapped features
in order. Its native controller finds fresh accessible names and roles, then
clicks their current bounds. Native GTK dialogs receive desktop focus/keyboard input and AT-SPI text/activate actions scoped to the recorded process.
There are no internal model setters or file-open test endpoints.

Stable names include `Open File`, `Open Folder`, `docs`, `report.md`, `Edit`,
`Preview`, `Split`, `Editor`, `Next document`, `Keep Editing`, and
`Discard and Quit`. File dialog readiness also requires a viewable native window.

## Evidence

The selected output directory retains `actions.jsonl`, accessibility snapshots,
PNG captures, `doctor.json`, `filesystem-check.json`, `result.json`, `app.log`,
and `cleanup.json`. A pass requires `result.json` to say `VERIFIED` and both
cleanup flags to be true. Read each failed run's error and captured state.
An existing screenshot alone does not prove the action worked.

The driver compares every seeded workspace file's hash before and after typing,
checks preferences contain only allowed path/UI keys, and verifies that restart
opens only the last active file. Fixtures contain no actual credentials. A
failure at a native dialog is a failed entry point; do not substitute a seeded
preferences file and claim the dialog passed.

## Cleanup

The runner cleans up in `finally` and retains evidence. If the interpreter is
interrupted externally, run:

```sh
python3 scripts/verify_native.py cleanup --out build/verification/native-run
```

Cleanup validates the recorded executable and process start identity before
signalling the PID it created. It never kills by process name. It removes only
the recorded verification scratch directory and leaves the evidence directory.
Confirm that `preview.png`, `edit.png`, `split.png`, and the JSON proof survive.

## Helpers

`scripts/verify_native.py` is executable and provides `run`, `doctor`, and
`cleanup` as shown above. It uses the standard Python library and native X11
libraries. Keep the feature map aligned with the runner. Use pstack's
`/maintain-verification-skill` when changing the control surface or journeys.
