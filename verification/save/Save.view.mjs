export const title = "WonderVision save protocol";
export function state(v) {
  if (v.racedWrite) return "Newer outside write was replaced";
  if (v.saved !== v.lastSavedSource) return "Newer buffer was incorrectly marked saved";
  if (v.expected !== v.approval) return "Overwrite approval changed without review";
  if (v.unsafeWrite) return "Known disk conflict was overwritten";
  if (v.phase === "validated" && v.disk !== v.checked) return "Outside writer changed disk after final check";
  return {idle: "Ready for edits or saving", confirming: "Reviewing overwrite", reading: "Checking expected disk content", writing: "Writing the captured snapshot to a temporary file", validated: "Final disk check passed", completed: v.success ? "Snapshot replaced disk; receipt pending" : "Save stopped without replacing disk"}[v.phase];
}
export const step = action => ({Edit: "you edit the buffer", OutsideWrite: "another process writes the file", BeginSave: "you save a captured snapshot", PromptOverwrite: "you review the observed disk version", ConfirmOverwrite: "you approve that version", CancelPrompt: "you cancel", InspectDisk: "the first disk check finishes", Validate: "the final disk check finishes", Replace: "the snapshot replaces the file", WriteFailure: "writing fails", Acknowledge: "the save receipt updates the baseline", Done: "the bounded run ends"}[action] ?? action);
