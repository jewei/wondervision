# M0 and M1 verification workflow

The goal is a native, minimal, event-driven WonderVision that opens real files,
loads directories lazily, shows editable text and GFM, and restores only the root
listing and last active buffer. M2 and M3 features stay out of this pass.

This workflow adopts the requested pstack guidance during M1. M0 was already
compiled, tested, measured, and committed. The reusable control skill was not
present before M1 implementation; the M0 screenshot and commit are the available
pre-change baseline.

## Checklist

- [x] Read the Principles section of pstack poteto-mode and the applicable leaves.
- [x] Phase A. Frame the outcome and establish scope.
- [x] Phase B. Design the workflow.
- [x] Phase C. Verify the native open/view slice and keep the smallest fixes.
- [x] Build the reusable desktop harness and feature map; run it against an empty
  isolated configuration through real Open File and Open Folder dialogs.
- [x] Exhaustively explore production request/completion callbacks up to four
  file requests and three folder requests. Show that removing each stale-result
  guard causes a counterexample. This is bounded testing, not an unbounded proof.
- [x] Phase D. Keep the decision trail in `docs/decisions.tsv` as each gate lands.
- [x] Phase E. Run the full build, tests, UI harness, and bounded checks. Review
  evidence, commit with Conventional Commit messages, and push to `origin/work`.

## Scope and rigor

Two application milestones, one Go module, four internal packages, two reusable
verification skills. The high-risk boundaries in this slice are stale asynchronous
results, file identity, invalid file data, and local preview authorization. They
get behavioral tests and bounded request-order exploration. Layout changes get a
real native-window exercise and screenshots. There is no irreversible deployment.

One UI thread owns the model, one bounded I/O queue owns disk work, and each
cancelled request loses permission to apply its result. The test-only MyGo
native automation plugin drives accessible controls and is excluded from the
production build. Native file dialogs are driven through the X11 desktop in the
Linux harness. No browser, server, or production automation endpoint is added.

The done predicate is a successful production build plus passing behavioral
checks and a real native run that opens a folder and standalone file, expands a
directory, switches tabs and Markdown modes, displays a local image, shows an
unsaved edit in Split, leaves the original bytes unchanged, and restores lazily.
A short idle observation is reported separately from M4's unrun release gates.

## Evidence and decisions

`docs/verification/` holds durable evidence summaries; `docs/screenshots/` holds
native captures. Full rerun artifacts go to ignored `build/verification/` and
survive harness cleanup. `docs/decisions.tsv` records decisions and gates.

The Go toolchain has no verifier configured for this project. The request-order
check exercises real production callbacks under every schedule in its stated
bound instead of proving a separate hand-written model. It does not prove all
possible executions, filesystem atomicity, or framework behavior.

Applied pstack sources:

- https://github.com/cursor/plugins/blob/main/pstack/skills/figure-it-out/SKILL.md
- https://github.com/cursor/plugins/blob/main/pstack/skills/create-verification-skill/SKILL.md
- https://github.com/cursor/plugins/blob/main/pstack/skills/create-formal-verification-skill/SKILL.md
