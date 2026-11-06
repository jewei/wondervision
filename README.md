# WonderVision

A native, single-window Go text workspace using MyGo. This repository implements
the first two milestones of [the specification](spec.md).

## Build and run

Use **Go 1.27.3** and the pinned **MyGo v0.7.4**. On Linux the native application
requires GTK 3 and a desktop display; WebKitGTK is not needed.

```sh
go test ./...
go build -tags mygo_noinspector -trimpath -o build/wondervision ./cmd/wondervision
./build/wondervision
```

The native MyGo template was initialized with the v0.7.4 CLI. Its entry point is
under `cmd/wondervision`; `mygo.json` also supports `go tool mygo build` for packaging.
No frontend, local server, or runtime network client is used.

## Milestones

- M0: native shell, system light/dark theme, dismissible errors, idle-frame test.
- M1: native Open File/Open Folder, lazy virtualized Explorer, retained tabs,
  highlighted text/code editor, live Markdown Preview/Edit/Split with local
  images and links, and lazy restoration of the workspace and active file.

Edits stay in memory. This build has no Save/Save As, tab closing, file mutations,
external-change detection, recovery, Quick Open, search, or command palette.
Those belong to M2/M3. Quit confirms before discarding edits. Files over 20 MiB
are refused; Markdown over 2 MiB starts in Edit and requires an explicit preview
action. Only UTF-8 text is supported. The Linux desktop was exercised; macOS and
Windows have not been built or run in this pass.

Preferences contain paths and UI choices in the OS config directory under
`wondervision/preferences.json`. No document contents or metadata are written
inside the user's workspace.

## Verification

```sh
go test -race ./...
go vet ./...
go tool mygo vet
go build -tags e2e -o build/wondervision-e2e ./cmd/wondervision
python3 scripts/verify_native.py run --out build/verification/native-run
python3 scripts/verify_request_order.py --mutation-check
```

The native driver needs an authorized X11 desktop, Python 3, libX11, libXtst,
libatspi, `xwininfo`, and `xprop`. The `e2e` tag adds a private test-only Unix
socket. It is excluded from production. Only one driver should use a desktop at
a time. All test workspaces and preferences are isolated.

Project-local pstack verification skills live in `.cursor/skills/`. See
[the workflow](docs/WORKFLOW.md), [verification evidence](docs/verification/M1.md),
and [observed performance](PERFORMANCE.md). Bounded request-order tests cover
143 schedules and reject two deliberately broken guard variants; they are not an
unbounded formal proof.

![WonderVision Split view](docs/screenshots/split.png)
