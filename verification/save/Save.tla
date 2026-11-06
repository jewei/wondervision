----------------------------- MODULE Save -----------------------------
EXTENDS Naturals
CONSTANTS MaxSteps, BrokenClean, BrokenConflict, BrokenApproval
Values == 0..2
VARIABLES current, saved, disk, accepted, source, expected, approval, before, checked,
          phase, success, lastSavedSource, unsafeWrite, racedWrite, steps
vars == <<current,saved,disk,accepted,source,expected,approval,before,checked,
          phase,success,lastSavedSource,unsafeWrite,racedWrite,steps>>
Init == /\ current=0 /\ saved=0 /\ disk=0 /\ accepted=0
        /\ source=0 /\ expected=0 /\ approval=0 /\ before=0 /\ checked=0
        /\ phase="idle" /\ success=FALSE /\ lastSavedSource=0
        /\ unsafeWrite=FALSE /\ racedWrite=FALSE /\ steps=0
Edit == /\ steps<MaxSteps
        /\ \E value \in Values : /\ current'=value /\ value#current
        /\ steps'=steps+1
        /\ UNCHANGED <<saved,disk,accepted,source,expected,approval,before,checked,
                       phase,success,lastSavedSource,unsafeWrite,racedWrite>>
OutsideWrite == /\ steps<MaxSteps
                /\ \E value \in Values : /\ disk'=value /\ value#disk
                /\ steps'=steps+1
                /\ UNCHANGED <<current,saved,accepted,source,expected,approval,before,checked,
                               phase,success,lastSavedSource,unsafeWrite,racedWrite>>
BeginSave == /\ steps<MaxSteps /\ phase="idle"
             /\ source'=current /\ expected'=accepted /\ approval'=accepted /\ phase'="reading"
             /\ success'=FALSE /\ steps'=steps+1
             /\ UNCHANGED <<current,saved,disk,accepted,before,checked,lastSavedSource,unsafeWrite,racedWrite>>
PromptOverwrite == /\ steps<MaxSteps /\ phase="idle"
                   /\ source'=current /\ expected'=disk /\ approval'=disk /\ phase'="confirming"
                   /\ success'=FALSE /\ steps'=steps+1
                   /\ UNCHANGED <<current,saved,disk,accepted,before,checked,lastSavedSource,unsafeWrite,racedWrite>>
ConfirmOverwrite == /\ steps<MaxSteps /\ phase="confirming" /\ current=source
                    /\ expected'=IF BrokenApproval THEN disk ELSE expected
                    /\ phase'="reading" /\ steps'=steps+1
                    /\ UNCHANGED <<current,saved,disk,accepted,source,approval,before,checked,success,lastSavedSource,unsafeWrite,racedWrite>>
CancelPrompt == /\ steps<MaxSteps /\ phase="confirming"
                /\ phase'="idle" /\ steps'=steps+1
                /\ UNCHANGED <<current,saved,disk,accepted,source,expected,approval,before,checked,success,lastSavedSource,unsafeWrite,racedWrite>>
InspectDisk == /\ steps<MaxSteps /\ phase="reading"
               /\ before'=disk
               /\ phase'=IF disk=expected \/ BrokenConflict THEN "writing" ELSE "completed"
               /\ steps'=steps+1
               /\ UNCHANGED <<current,saved,disk,accepted,source,expected,approval,checked,success,lastSavedSource,unsafeWrite,racedWrite>>
Validate == /\ steps<MaxSteps /\ phase="writing"
            /\ checked'=disk
            /\ phase'=IF disk=before THEN "validated" ELSE "completed"
            /\ steps'=steps+1
            /\ UNCHANGED <<current,saved,disk,accepted,source,expected,approval,before,success,lastSavedSource,unsafeWrite,racedWrite>>
Replace == /\ steps<MaxSteps /\ phase="validated"
           /\ disk'=source /\ success'=TRUE /\ phase'="completed"
           /\ unsafeWrite'=(unsafeWrite \/ (before#expected) \/ (checked#before))
           /\ racedWrite'=(racedWrite \/ (disk#checked))
           /\ steps'=steps+1
           /\ UNCHANGED <<current,saved,accepted,source,expected,approval,before,checked,lastSavedSource>>
WriteFailure == /\ steps<MaxSteps /\ phase \in {"reading","writing","validated"}
                /\ success'=FALSE /\ phase'="completed" /\ steps'=steps+1
                /\ UNCHANGED <<current,saved,disk,accepted,source,expected,approval,before,checked,lastSavedSource,unsafeWrite,racedWrite>>
Acknowledge == /\ steps<MaxSteps /\ phase="completed"
               /\ saved'=IF success THEN (IF BrokenClean THEN current ELSE source) ELSE saved
               /\ accepted'=IF success THEN source ELSE accepted
               /\ lastSavedSource'=IF success THEN source ELSE lastSavedSource
               /\ phase'="idle" /\ steps'=steps+1
               /\ UNCHANGED <<current,disk,source,expected,approval,before,checked,success,unsafeWrite,racedWrite>>
Done == /\ steps=MaxSteps /\ UNCHANGED vars
Next == Edit \/ OutsideWrite \/ BeginSave \/ PromptOverwrite \/ ConfirmOverwrite \/ CancelPrompt \/ InspectDisk \/ Validate \/ Replace \/ WriteFailure \/ Acknowledge \/ Done
NoFalseClean == (current#lastSavedSource) => (current#saved)
SavedBaselineIsSnapshot == saved=lastSavedSource
KnownConflictsNeverWritten == ~unsafeWrite
NoRacedOverwrite == ~racedWrite
ExpectedIsReviewed == expected=approval
=============================================================================
