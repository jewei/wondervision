# Save

TLC reached 12,004 states, more than the chart budget of 500, so the chart draws only the bug paths: 6 states and 6 arrows.

## NoFalseClean breaks
The shortest path has 6 steps:
0. Ready for edits or saving
1. you save a captured snapshot: Checking expected disk content
2. you edit the buffer: Checking expected disk content
3. the first disk check finishes: Writing the captured snapshot to a temporary file
4. the final disk check finishes: Final disk check passed
5. the snapshot replaces the file: Snapshot replaced disk; receipt pending
6. the save receipt updates the baseline: Newer buffer was incorrectly marked saved

## SavedBaselineIsSnapshot breaks
The shortest path has 6 steps:
0. Ready for edits or saving
1. you save a captured snapshot: Checking expected disk content
2. you edit the buffer: Checking expected disk content
3. the first disk check finishes: Writing the captured snapshot to a temporary file
4. the final disk check finishes: Final disk check passed
5. the snapshot replaces the file: Snapshot replaced disk; receipt pending
6. the save receipt updates the baseline: Newer buffer was incorrectly marked saved

## Rules that hold
KnownConflictsNeverWritten, ExpectedIsReviewed, NoDeadlock
