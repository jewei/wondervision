# Save

TLC reached 18,210 states, more than the chart budget of 500, so the chart draws only the bug paths: 3 states and 3 arrows.

## ExpectedIsReviewed breaks
The shortest path has 3 steps:
0. Ready for edits or saving
1. you review the observed disk version: Reviewing overwrite
2. another process writes the file: Reviewing overwrite
3. you approve that version: Overwrite approval changed without review

## Rules that hold
NoFalseClean, SavedBaselineIsSnapshot, KnownConflictsNeverWritten, NoDeadlock
