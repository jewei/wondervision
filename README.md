# Folio

A native, single-window Go text workspace using MyGo. This repository implements
the first two milestones of [the specification](spec.md).

## Build and run

Use **Go 1.27.3** and the pinned **MyGo v0.7.4**. On Linux the native application
requires GTK 3 and a desktop display; WebKitGTK is not needed.

```sh
go test ./...
go build -tags mygo_noinspector -trimpath -o build/folio ./cmd/folio
./build/folio
```

The native MyGo template was initialized with the v0.7.4 CLI. Its entry point is
under `cmd/folio`; `mygo.json` also supports `go tool mygo build` for packaging.
No frontend, local server, or runtime network client is used.

## Milestones

- M0: native shell, system light/dark theme, dismissible errors, idle-frame test.
- M1: in progress.

Save, file mutations, conflict handling, recovery, watchers, Quick Open, search,
and the command palette belong to later milestones.
