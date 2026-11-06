# Save

TLC reached 10,180 states, more than the chart budget of 500, so the chart draws only the bug paths: 6 states and 5 arrows.

## NoRacedOverwrite breaks
The shortest path has 5 steps:
0. Ready for edits or saving
1. you save a captured snapshot: Checking expected disk content
2. the first disk check finishes: Writing the captured snapshot to a temporary file
3. the final disk check finishes: Final disk check passed
4. another process writes the file: Outside writer changed disk after final check
5. the snapshot replaces the file: Newer outside write was replaced

## Rules that hold
NoFalseClean, SavedBaselineIsSnapshot, KnownConflictsNeverWritten, ExpectedIsReviewed, NoDeadlock
