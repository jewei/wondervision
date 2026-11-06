---
name: verify-request-order
description: Recheck WonderVision's bounded asynchronous file/folder request ordering and stale-result guards after coordinator or cancellation changes.
---

# Verify request ordering

An old file or folder result must not replace the user's newer choice. Breaking
this promise can open the wrong workspace or replace the active editing session.
Repeated opens of the same identity must preserve its existing editable buffer.

## Run

Use Go 1.27.3 and Python 3. No GUI, network, model-checker download, or extra
runtime dependency is needed once Go modules are installed.

```sh
python3 scripts/verify_request_order.py --mutation-check
```

The executable helper runs the bounded tests on production callbacks, copies the
code into disposable scratch directories, removes each stale-result guard in
turn, requires a behavioral failure, then reruns the unmodified code. It never
mutates the working tree. Proof survives under `build/verification/request-order/`.

## Coverage and limits

Last confirmed code commit: `b8d8baf338dc137afd343c83f070f82da0b188af`.

| Production code | Check |
| --- | --- |
| `internal/app/app.go` `openFile`, `activate`; document identity merging | `TestFileCompletionSchedules` |
| `internal/app/app.go` `openFolder`; cancelled callback rejection | `TestFolderCompletionSchedules` |
| Folder/file overlap and authorization changes | `TestWorkspaceChangeCancelsLateScopedOpenAndUpdatesExistingArea` |
| Final preferences snapshot at quit | `TestQuitWaitsForLatestPreferences` |

The exhaustive checks enumerate every issue/delivery schedule with user requests
issued in order and their already-computed UI callbacks delivered in any order.
There are 1, 3, 15, and 105 schedules for one through four file requests, and 1,
3, and 15 schedules for one through three folder requests. All 143 exercise the
real coordinator methods and result closures. Fixtures include two distinct
files and an alias. Previously accepted buffers receive an unsaved edit so
identity merging must retain the actual buffer.

Assumptions match MyGo's UI contract: request issuance and each Window.Update
callback are indivisible on one UI thread; each callback is delivered once. I/O
can complete before cancellation while its UI callback is still pending. The
check explicitly includes that ordering.

There is no formal verifier configured for this Go project. This is exhaustive
bounded testing of actual code, not a theorem, TLA+ model, or unbounded proof.
It omits unbounded request counts, operating-system scheduling inside a read,
framework correctness, I/O errors, queue saturation, and full mixed-operation
schedule exploration. Mixed folder/file and shutdown cases have named examples,
not exhaustive coverage. Path safety has separate rooted-open tests.

## Read failures

The helper records the baseline, both mutant failures, and the restored pass.
A normal-code failure is a finding; do not remove a schedule or weaken the
expected session state to make it pass. Tell the counterexample in request order.
For example, request report, request TODO, deliver report's old result; report
must not become the active buffer.

A mutant that compiles but is accepted means the harness no longer guards the
property. A compile/tool error is not an acceptable mutant rejection. The helper
requires the specific behavioral assertion. A changed guard shape stops the
helper and requires a coverage review rather than a guessed text replacement.

## Keep the check aligned

After a change to the covered methods, read each production transition against
its test oracle and confirm the same UI-thread atomic steps and lifetime rules.
Extend the schedules when the domain changes. Run the helper and the full test
suite. Move the sync commit forward only after that review and successful
mutation checks. Use pstack's `/maintain-formal-verification-skill` to audit
coverage after changes that did not recheck it.
