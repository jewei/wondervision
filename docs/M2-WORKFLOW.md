# M2 editing correctness

The done predicate is a native application that saves only on explicit commands,
preserves supported text formats and permissions, rejects known disk conflicts,
protects newer edits during I/O and on close, detects external changes without
polling, recovers permitted drafts privately, and performs workspace file actions
without path escape or accidental replacement.

The scope is M2 only. Quick Open, content search, palettes, and release packaging
remain later milestones. Each slice must compile and pass its relevant tests
before the next slice begins. Commits use Conventional Commits and are pushed to
`origin/work`.

- [x] Read the existing verification skills and inspect the pinned APIs.
- [x] Add an immutable save plan, content fingerprints, atomic replacement, and
  dirty-state transitions. Exercise format preservation, failures, conflicts,
  and editing while saving against actual files.
- [x] Wire manual Save/Save As and close/quit decisions into the native UI.
- [x] Add scoped file watches and explicit reload/compare/overwrite resolution.
- [x] Add edit-triggered recovery with no snapshots for sensitive filenames.
- [x] Add rooted create/rename/move/delete operations and their native controls.
- [x] Add architecture guardrails and a TLA+ save-order model with a real-code
  regression replay and deliberately broken variants.
- [x] Run native user journeys, race/vet/build checks, review evidence and risks,
  update reusable verification skills, and push the verified slices.

The UI thread owns document/session state. An immutable buffer snapshot crosses
the I/O boundary; a matching save token returns. A successful write advances the
saved baseline to that snapshot, so later edits remain dirty. Every write checks
the last accepted disk fingerprint and rechecks just before replacing the file.
Atomic rename is not a filesystem compare-and-swap; the final outside-writer race
is an explicit limitation rather than a claimed proof.

Three layers have different jobs. Types and API boundaries constrain who can
write and acknowledge a save. Real filesystem/UI tests cover observable behavior
and OS failures. The finite TLA+ model checks save/edit/conflict ordering under
stated bounds and atomic steps. It does not prove the filesystem or toolkit.
The decision trail remains `docs/decisions.tsv`.
