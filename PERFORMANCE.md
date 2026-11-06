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

## M1

One foreground sample of the production binary from source commit
`7fcdb295d42e315bfd1de76fe6c45a1985ec81d6`, on the same machine and software
renderer. The workspace held 1,008 files across 50 generated folders plus a docs
folder. Open tabs were a 50 KiB report (active native Preview with a local SVG),
`.env`, and approximately 100 KiB of Go text. These were opened through the
production window's native file chooser; no automation plugin was linked into
that binary. The screen was captured before sampling to verify the workload.

| Observation | Result |
| --- | --- |
| Sample duration and count | 30.001 s, one sample |
| Process CPU time delta | 0.00 s (100 Hz process accounting) |
| Average CPU, one logical core | 0.00% at that resolution |
| Resident memory, start → end | 57,116 → 57,116 KiB (about 55.8 MiB) |
| OS threads, start → end | 15 → 15 |
| Open file descriptors, start → end | 22 → 22 |

The native view had settled before the interval. The measurement reads process
CPU accounting, RSS, thread count, and file descriptor count only; it does not
time a file operation or count throughput. The observed workload is in
`docs/verification/production-workload.png`, with raw values in
`docs/verification/production-idle.json`. No speedup or cross-platform comparison
is inferred. A later link-scope snapshot fix does not change the idle path; this
record identifies the exact measured source rather than substituting a new SHA.

Not measured in this pass: 10-minute foreground/minimized CPU, 24-hour memory
trend, launch latency, input-to-frame latency, or network traffic. Quick Open and
workspace search do not exist yet. These short observations do not certify the
full performance contract or exclude a long-term leak.
