---
name: verify-save-protocol
description: Recheck WonderVision's finite save/edit/conflict protocol and real-code counterexamples after changing save plans, disk checks, acknowledgements, or overwrite approvals.
---

# Verify the save protocol

A successful save must mark only its captured snapshot as saved, and must block
a disk conflict found by its checks unless that exact version was approved.
Breaking this promise can lose later edits or overwrite unreviewed disk content.

## Run

Use Go 1.27.3, Python 3, and Java 11 or later:

```sh
python3 scripts/verify_save_model.py
```

The helper downloads a development-only TLC jar if absent, validates its SHA-256,
checks the finite model, requires three model mutants to break named invariants,
replays the corresponding faults in scratch copies of actual Go code, and reruns
unmodified code. It also requires a counterexample for the documented final
filesystem replacement window. It never mutates the working tree. Outputs are in
`build/verification/save-model/`.

Pinned asset: TLA+ GitHub release `v1.8.0`, `tla2tools.jar`, SHA-256
`7a0a1e90628ebff2e3e8733375f8b684fa59504d2e02bddc4cb51a7679bb7e04`.
The verified asset reports `TLC2 Version 2026.11.06.100558 (rev: 3ec1cc4)`.
The helper's checksum, rather than a moving release URL alone, identifies it.
There is no application runtime dependency on Java or TLC.

Read every model result with pstack's **`/statechart`** skill. Use
`verification/save/Save.view.mjs` for readable state names. Follow that skill's
current invocation instructions; its commands are intentionally not copied here.
Reports and shortest counterexample paths from the recorded run are under
`docs/verification/save-statechart/`. Above its 500-state chart budget, a passing
run has a report without a whole-graph chart; failing runs retain bug-path charts.

## Coverage record

Code sync commit: `31bc6b0170df557053e92fc393549bf2bdb265f6`.

| Model action / invariant | Production transition / empirical replay |
| --- | --- |
| `Edit` | MyGo buffer edits; `TestSaveKeepsNewerEditsDirty` |
| `BeginSave` | `Document.plan`, `saveState.begin`: one immutable snapshot, source revision and expected disk fingerprint |
| `PromptOverwrite`, `ConfirmOverwrite`, `ExpectedIsReviewed` | `App.requestOverwrite`, `confirmOverwrite`; `TestOverwriteApprovalDoesNotFollowNewWatcherVersion` |
| `InspectDisk` | `SavePlan.write` initial `fingerprintAt` and expected-fingerprint check; `TestSaveRejectsKnownConflict` |
| `Validate` | `SavePlan.write` final fingerprint/mode recheck after temporary file sync/close |
| `Replace`, `NoRacedOverwrite` | rooted `Rename` of an existing file, a separate step from validation; `TestFinalReplaceWindowIsNotCompareAndSwap` |
| `Acknowledge`, `NoFalseClean`, `SavedBaselineIsSnapshot` | `Document.CompleteSave`, `saveState.complete`; newer edits remain dirty |
| `KnownConflictsNeverWritten` | no replacement when the initial fingerprint differs from the accepted/approved fingerprint or the final check differs from the first |

The model is `verification/save/Save.tla`, configured by `Save.cfg`. It enumerates
three abstract content revisions and every permitted sequence up to ten actions,
with one pending save and one document. The recorded baseline reached **10,180
distinct states**, 40,212 generated states, depth 11, with no invariant violation
or deadlock. This is a bounded design check, not verification of Go source for all
inputs or an unbounded proof.

The UI thread issues requests and handles each acknowledgement indivisibly.
Edits and atomic outside writes may happen during save I/O. Initial inspection,
final validation, replacement and acknowledgement are separate steps. Fingerprints
are treated as coherent content identifiers without collisions; reads interrupted
inside a hash, torn in-place writes, filesystem/toolkit correctness, durability
after power loss, permissions, Save As, watchers, recovery, multi-process draft
locking, close/quit, duplicate receipts and queue saturation are outside the model.
Several have real tests, but do not inherit this model's guarantee.

## Interpret results

The baseline must pass. Broken-clean advances the baseline to newer buffer text;
broken-conflict removes the first disk check; broken-approval retrieves a mutable
watcher fingerprint at confirmation. All must fail a named behavioral assertion
in real Go code, not just compilation. A changed source marker stops the helper
and requires reviewing the replay rather than guessing a new replacement.

`NoRacedOverwrite` intentionally fails on unmodified code: start saving, read
matching disk bytes, pass the final check, let another process write, then rename
our snapshot over it. The named Go test replays the same ordering through a
private seam immediately before replacement. This is the specification's stated
non-CAS filesystem limitation. Do not remove that action or claim all outside
writes are protected. See `docs/verification/save-statechart/final-window-limit/`.

Any other baseline failure is a finding or a modeling error. Inspect its shortest
path and replay it against actual code. Do not weaken a property to get a pass.
Mutation output that fails for a tool or compile error is inconclusive.

## Maintain

After a covered transition changes, compare its real atomic steps, source
snapshot, approval lifetime and receipt lifetime with the model and replay.
Update the finite model when intended behavior changes; rerun model/mutations and
the relevant Go tests. Move the code sync commit only after that comparison.
Use pstack's `/maintain-formal-verification-skill` after changes that did not
recheck coverage. Follow `verify-wondervision` for empirical native editing flows;
run `python3 scripts/verify_recovery.py` for the separate recovery-lock regressions.
