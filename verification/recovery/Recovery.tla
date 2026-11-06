---------------------------- MODULE Recovery ----------------------------
(***************************************************************************)
(* Cross-session recovery drafts for one file path.                        *)
(*                                                                         *)
(* Code map (internal/documents/recovery.go):                              *)
(*   BeginWrite/CommitWrite   RecoveryStore.Write: lock, header check,     *)
(*                            temp file, root.Rename(temp, name)           *)
(*   BeginDiscard/Commit...   RecoveryStore.Discard -> remove(approved)    *)
(*   BeginRemove/Commit...    RecoveryStore.Remove  -> remove(nil)         *)
(*   List                     RecoveryStore.List (records shown to user)   *)
(*   Load                     RecoveryStore.Load (r == record) + claim     *)
(*   Crash                    process death: flock released, new session  *)
(*                            identity (NewRecoveryStore -> rand.Text())   *)
(* The Begin/Commit split exists so that a missing lock (BrokenLock) lets  *)
(* other sessions run between the header check and the rename/remove.     *)
(* Load is lock-free in the code; it is one atomic read here because a     *)
(* draft only changes by rename and Load reads one open file.              *)
(***************************************************************************)
EXTENDS Naturals
CONSTANTS MaxSteps, BrokenLock, BrokenClaim, BrokenDiscard

Sessions == {"s1", "s2"}
NoDraft == [owner |-> <<"none", 0>>, ver |-> 0]

VARIABLES disk,       \* the draft file for the path, or NoDraft
          lock,       \* "free" or the session holding .lock
          gen,        \* per-process generation; <<s, gen[s]>> is the session id
          pc,         \* "idle" or the operation between check and commit
          approval,   \* record the user approved for discard
          claim,      \* record the user recovered with Load (Document.recoveryClaim)
          seen,       \* record last shown by List
          ver,        \* next draft content number
          lost,       \* ghost: an unreviewed draft of another session was replaced or deleted
          steps

vars == <<disk, lock, gen, pc, approval, claim, seen, ver, lost, steps>>

Me(s) == <<s, gen[s]>>
Draft(r) == r # NoDraft
Foreign(s, r) == Draft(r) /\ r.owner # Me(s)
\* Replacing or deleting r is safe for s only if r is s's own draft, the user
\* recovered exactly r (claim), or approved discarding exactly r.
Unreviewed(s, r, approved) == Foreign(s, r) /\ claim[s] # r /\ approved # r

Init == /\ disk = NoDraft /\ lock = "free" /\ gen = [s \in Sessions |-> 0]
        /\ pc = [s \in Sessions |-> "idle"] /\ approval = [s \in Sessions |-> NoDraft]
        /\ claim = [s \in Sessions |-> NoDraft] /\ seen = [s \in Sessions |-> NoDraft]
        /\ ver = 1 /\ lost = FALSE /\ steps = 0

Step == steps < MaxSteps /\ steps' = steps + 1
CanLock(s) == BrokenLock \/ lock = "free"
Take(s) == lock' = IF BrokenLock THEN lock ELSE s
Release(s) == lock' = IF BrokenLock THEN lock ELSE "free"

\* RecoveryStore.Write header check: no draft, our own, or the claimed one.
WriteAllowed(s) == \/ ~Draft(disk)
                   \/ disk.owner = Me(s)
                   \/ IF BrokenClaim THEN Draft(claim[s]) ELSE claim[s] = disk

BeginWrite(s) == /\ Step /\ pc[s] = "idle" /\ CanLock(s) /\ WriteAllowed(s)
                 /\ Take(s) /\ pc' = [pc EXCEPT ![s] = "write"]
                 /\ UNCHANGED <<disk, gen, approval, claim, seen, ver, lost>>

CommitWrite(s) == /\ Step /\ pc[s] = "write"
                  /\ lost' = (lost \/ Unreviewed(s, disk, NoDraft))
                  /\ disk' = [owner |-> Me(s), ver |-> ver] /\ ver' = ver + 1
                  /\ Release(s) /\ pc' = [pc EXCEPT ![s] = "idle"]
                  /\ UNCHANGED <<gen, approval, claim, seen>>

\* remove(approved): own drafts, or exactly the approved record.
DiscardAllowed(s, r) == /\ Draft(disk)
                        /\ \/ disk.owner = Me(s)
                           \/ IF BrokenDiscard THEN Draft(r) ELSE disk = r

BeginDiscard(s) == /\ Step /\ pc[s] = "idle" /\ CanLock(s)
                   /\ \E r \in {seen[s], claim[s]} :
                        /\ Draft(r) /\ DiscardAllowed(s, r)
                        /\ approval' = [approval EXCEPT ![s] = r]
                   /\ Take(s) /\ pc' = [pc EXCEPT ![s] = "remove"]
                   /\ UNCHANGED <<disk, gen, claim, seen, ver, lost>>

\* remove(nil): only this session's own draft.
BeginRemove(s) == /\ Step /\ pc[s] = "idle" /\ CanLock(s)
                  /\ Draft(disk) /\ disk.owner = Me(s)
                  /\ approval' = [approval EXCEPT ![s] = NoDraft]
                  /\ Take(s) /\ pc' = [pc EXCEPT ![s] = "remove"]
                  /\ UNCHANGED <<disk, gen, claim, seen, ver, lost>>

CommitRemove(s) == /\ Step /\ pc[s] = "remove"
                   /\ lost' = (lost \/ Unreviewed(s, disk, approval[s]))
                   /\ disk' = NoDraft
                   /\ Release(s) /\ pc' = [pc EXCEPT ![s] = "idle"]
                   /\ UNCHANGED <<gen, approval, claim, seen, ver>>

List(s) == /\ Step /\ pc[s] = "idle" /\ Draft(disk) /\ seen[s] # disk
           /\ seen' = [seen EXCEPT ![s] = disk]
           /\ UNCHANGED <<disk, lock, gen, pc, approval, claim, ver, lost>>

\* Load succeeds only if the copy still equals the listed record.
Load(s) == /\ Step /\ pc[s] = "idle" /\ Foreign(s, disk) /\ seen[s] = disk
           /\ claim[s] # disk
           /\ claim' = [claim EXCEPT ![s] = disk]
           /\ UNCHANGED <<disk, lock, gen, pc, approval, seen, ver, lost>>

\* A crash mid-write leaves only a temp file; the draft is untouched.
Crash(s) == /\ Step
            /\ gen' = [gen EXCEPT ![s] = gen[s] + 1]
            /\ pc' = [pc EXCEPT ![s] = "idle"]
            /\ claim' = [claim EXCEPT ![s] = NoDraft]
            /\ seen' = [seen EXCEPT ![s] = NoDraft]
            /\ approval' = [approval EXCEPT ![s] = NoDraft]
            /\ lock' = IF lock = s THEN "free" ELSE lock
            /\ UNCHANGED <<disk, ver, lost>>

\* The step bound ends a behavior; it is not a deadlock of the protocol.
Done == steps = MaxSteps /\ UNCHANGED vars

SessionStep(s) == \/ BeginWrite(s) \/ CommitWrite(s) \/ BeginDiscard(s) \/ BeginRemove(s)
                  \/ CommitRemove(s) \/ List(s) \/ Load(s) \/ Crash(s)

Next == (\E s \in Sessions : SessionStep(s)) \/ Done

Spec == Init /\ [][Next]_vars

\* A draft another session wrote is never replaced or deleted unless this
\* session's user recovered or approved discarding exactly that draft.
NoUnreviewedLoss == ~lost

\* Only the lock holder is ever between check and commit.
LockedCommit == \A s \in Sessions : pc[s] # "idle" => (BrokenLock \/ lock = s)
=============================================================================
