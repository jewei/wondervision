# Observed performance

Reference environment: Debian 13, Linux x86_64, Intel Xeon Platinum 8573C
(3 visible logical CPUs), remote X11 desktop. MyGo reported no GPU and used
its software renderer. Go 1.27.3, MyGo v0.7.4. Measurements on 2026-11-06.

## M0

Production build: `go build -tags mygo_noinspector -trimpath -o build/wondervision ./cmd/wondervision`.
After launch settled, sampled `/proc/PID/stat` and `/proc/PID/status` before and
after 30.001 seconds with the native shell foreground and untouched:

| Observation | Result |
| --- | --- |
| Process CPU time delta | 0.00 s (100 Hz process accounting) |
| Average CPU, one logical core | 0.00% at that resolution |
| Resident memory, start → end | 30,224 → 30,224 KiB |
| OS threads, start → end | 12 → 12 |

The headless UI test also advances one minute in both system themes and checks
that the settled shell requests no additional frames. No application timer,
animation, watcher, or scan exists in M0.

This short shell sample is not the specification's reference workload or its
10-minute foreground/minimized and 24-hour release gates. Those remain M4 work.
No general cross-platform performance claim is made.
