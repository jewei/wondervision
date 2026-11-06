# Save

TLC reached 15,055 states, more than the chart budget of 500, so the chart draws only the bug paths: 5 states and 5 arrows.

## KnownConflictsNeverWritten breaks
The shortest path has 5 steps:
0. Ready for edits or saving
1. another process writes the file: Ready for edits or saving
2. you save a captured snapshot: Checking expected disk content
3. the first disk check finishes: Writing the captured snapshot to a temporary file
4. the final disk check finishes: Final disk check passed
5. the snapshot replaces the file: Known disk conflict was overwritten

## Rules that hold
NoFalseClean, SavedBaselineIsSnapshot, ExpectedIsReviewed, NoDeadlock
