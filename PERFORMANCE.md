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

## M4: the spec §9 contract

Measured on 2026-11-07 with `scripts/measure_performance.py` from source commit
`3cda8b90943a078d8c56480d315382e0fa00c240`. Environment: a Linux x86_64
container (4 logical CPUs), Xvfb with openbox, MyGo's software renderer (no
GPU), Go 1.27.3, MyGo v0.7.4. This is not the spec's reference desktop: the
numbers catch regressions; they don't certify a machine a person uses.

Workload (spec §9): 1,000 text/Markdown files across 50 folders, a 50 KiB
Markdown report open in Preview, `.env` and a 100 KiB Go file open, no input,
settled for 10 seconds. The production binary (no automation linked) restored
the workspace from preferences; the other two tabs were opened with real X11
keystrokes into Quick Open. See `docs/verification/performance-m4-workload.png`;
raw values are in `docs/verification/performance-m4.json`.

| Metric | Observed | Target | Result |
| --- | --- | --- | --- |
| Warm launch to usable window (median of 5) | 0.44 s (cold: 0.62 s) | ≤ 1.5 s | met |
| Warm launch to restored report in Preview (median) | 2.14 s | — | recorded |
| Idle CPU, foreground, 600 s | 0.02% of one core (0.12 s CPU) | < 0.5% | met |
| Idle CPU, minimized, 600 s | 0.015% of one core (0.09 s CPU) | < 0.5% | met |
| Resident memory, foreground (max; end) | 135.6 MiB; 106.4 MiB | ≤ 150 MiB | met |
| Resident memory, minimized (start → end) | 106.4 → 106.2 MiB | ≤ 150 MiB | met |
| Internet sockets, both phases | 0 | zero network | met |
| Threads / file descriptors, both phases | 15 → 15 / 20 → 20 | no growth | met |
| Quick Open appears (median, minus one 46 ms accessibility-tree poll) | 73 ms (119 ms as measured) | ≤ 100 ms | met |
| Quick Open cached query, 1,000 paths (median, max) | 28, 30 ms | ≤ 100 ms | met |
| Quick Open, 10,000 paths: first scan | 0.10 s | progressive | recorded |
| Quick Open, 10,000 paths: slowest UI answer during the scan | 4 ms | responsive | recorded |
| Typing in the 100 KiB Go file, per key (median, p95, max of 60) | 17, 18, 19 ms | no stall > 50 ms | met |

How to read it:

- Launch is timed from exec until AT-SPI exposes the enabled Open File button;
  "restored" waits for the active document's status line.
- CPU is the process's utime + stime from `/proc` (100 Hz accounting) over each
  phase; memory is VmRSS sampled every minute. During the foreground phase RSS
  fell from 135.6 to 106.4 MiB as Go returned memory to the OS; the minimized
  phase stayed flat.
- Quick Open timings run on the e2e binary through the automation socket, so
  they include a key injection and polling of the accessibility tree; even the
  "minus one poll" figure is an upper bound.
- Typing was measured separately (`--only typing`, raw values in
  `docs/verification/performance-typing.json`): each sample injects one
  character into the 100 KiB file and waits until the UI is idle again, so it
  is input to settled frame plus an automation round trip (a bare idle call
  takes 0.2 ms). The steady ~17 ms is one frame at 60 Hz.
- Memory is dominated by rendering the 50 KiB report in Preview. Separate
  15-second samples of that one document varied from 100 to 160 MiB on both the
  pre-M3 and current binaries, so a single run can exceed the 150 MiB target.
  M3 itself adds little: using Quick Open grew RSS by 1–7 MiB.

Not measured here: the 24-hour leak test, macOS and Windows (compiled in CI,
never run), and a real desktop with a GPU.

## Where Preview memory goes

Investigated on 2026-11-07 because single runs of the M4 workload ranged from
100 to 160 MiB against the 150 MiB target. Same container and workload, the
production binary, RSS 15 seconds after launch, `GODEBUG=gctrace=1` for the Go
heap and `/proc/<pid>/smaps` for the mappings:

| Launch | Mode | RSS | Go heap (live) | Notable mappings |
| --- | --- | --- | --- | --- |
| First on a fresh X display | Preview | 138 MiB | 30 MB | `libLLVM` 40 MiB, `libgallium` 4 MiB |
| Every later launch (5 runs) | Preview | 88 MiB | 30 MB | none of those |
| Every launch (5 runs) | Edit | 53 MiB | 4 MB | none of those |

- The run-to-run variance is Mesa's software OpenGL stack. On the first launch
  on a fresh Xvfb display, GDK initializes GLX (it logs "screen 0 does not
  appear to be DRI3 capable") and Mesa's llvmpipe stays mapped: 44 MiB that
  later launches don't pay. MyGo itself draws without OpenGL here because
  there is no GPU device. A desktop with a GPU loads its own driver instead,
  so only a measurement on such a machine gives the number that matters.
- WonderVision's own cost is the Go heap of the Preview: 30 MB live for the
  50 KiB report against 4 MB in Edit. A heap profile of `markdown.View`
  rendering that report alone holds 39 MB, 83% of it UI elements and their
  per-element state: MyGo's Markdown view builds every block of the document,
  visible or not, and has no virtualized mode. Preview memory therefore grows
  with document length, about 0.75 MB per KiB of Markdown.
- Steady state is within target (88 MiB). Chunked, virtualized preview of long
  documents is a candidate for v0.2; it would have to keep cross-block
  selection, heading anchors and footnotes working.
