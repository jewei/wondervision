package documents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPreviousSessionDraftRequiresReview(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "todo.md")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := Open(ctx, path, root, Edit)
	if err != nil {
		t.Fatal(err)
	}
	d.Buffer.Set("older crash draft")
	old := NewRecoveryStore(filepath.Join(t.TempDir(), "recovery"))
	plan, _ := d.RecoverySnapshot()
	if err = old.Write(ctx, plan); err != nil {
		t.Fatal(err)
	}
	records, err := old.List(ctx)
	if err != nil || len(records) != 1 {
		t.Fatalf("list: %v %v", records, err)
	}
	fresh := NewRecoveryStore(old.Dir)
	d.Buffer.Set("fresh session edits")
	plan, _ = d.RecoverySnapshot()
	if err = fresh.Write(ctx, plan); !errors.Is(err, ErrUnreviewedRecovery) {
		t.Fatalf("unreviewed draft replaced: %v", err)
	}
	if err = fresh.Remove(ctx, path); err != nil {
		t.Fatal(err)
	}
	recovered, err := fresh.Load(ctx, records[0])
	if err != nil || recovered.Buffer.String() != "older crash draft" {
		t.Fatalf("old draft lost: %v", err)
	}
	// Recovering is explicit ownership of this exact copy, even before its next timer fires.
	if claim, ok := recovered.RecoveryClaim(path); !ok {
		t.Fatal("recovered copy has no cleanup claim")
	} else if err = fresh.Discard(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if remaining, err := fresh.List(ctx); err != nil || len(remaining) != 0 {
		t.Fatal("explicit discard did not remove recovered copy")
	}
	if err = old.Write(ctx, plan); err != nil {
		t.Fatal(err)
	}
	records, _ = fresh.List(ctx)
	recovered, err = fresh.Load(ctx, records[0])
	if err != nil {
		t.Fatal(err)
	}
	recovered.Buffer.Set("continued recovered draft")
	plan, _ = recovered.RecoverySnapshot()
	if err = fresh.Write(ctx, plan); err != nil {
		t.Fatalf("reviewed recovery could not continue: %v", err)
	}
	if err = old.Discard(ctx, records[0]); !errors.Is(err, ErrUnreviewedRecovery) {
		t.Fatalf("stale discard approval deleted a newer copy: %v", err)
	}
}

// A recovered claim covers exactly the copy the user reviewed. If the other
// session writes a newer draft afterwards, the stale claim must not replace it.
// This is the BrokenClaim counterexample of verification/recovery/Recovery.tla.
func TestStaleRecoveryClaimCannotReplaceNewerDraft(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "todo.md")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	other, err := Open(ctx, path, root, Edit)
	if err != nil {
		t.Fatal(err)
	}
	old := NewRecoveryStore(filepath.Join(t.TempDir(), "recovery"))
	fresh := NewRecoveryStore(old.Dir)
	other.Buffer.Set("first draft")
	plan, _ := other.RecoverySnapshot()
	if err = old.Write(ctx, plan); err != nil {
		t.Fatal(err)
	}
	records, err := fresh.List(ctx)
	if err != nil || len(records) != 1 {
		t.Fatalf("list: %v %v", records, err)
	}
	recovered, err := fresh.Load(ctx, records[0])
	if err != nil {
		t.Fatal(err)
	}
	other.Buffer.Set("newer unreviewed draft")
	plan, _ = other.RecoverySnapshot()
	if err = old.Write(ctx, plan); err != nil {
		t.Fatal(err)
	}
	recovered.Buffer.Set("edits on the reviewed copy")
	plan, _ = recovered.RecoverySnapshot()
	if err = fresh.Write(ctx, plan); !errors.Is(err, ErrUnreviewedRecovery) {
		t.Fatalf("stale claim replaced a newer unreviewed draft: %v", err)
	}
	records, err = fresh.List(ctx)
	if err != nil || len(records) != 1 {
		t.Fatalf("list after rejected write: %v %v", records, err)
	}
	if kept, err := fresh.Load(ctx, records[0]); err != nil || kept.Buffer.String() != "newer unreviewed draft" {
		t.Fatalf("newer draft was not preserved: %v", err)
	}
}

func TestRecoveryPrivateRoundTripAndRemoval(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "todo.md")
	os.WriteFile(path, []byte("original\r\n"), 0600)
	d, _ := Open(context.Background(), path, root, Edit)
	d.Buffer.Set("draft\n")
	plan, err := d.RecoverySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	store := *NewRecoveryStore(filepath.Join(t.TempDir(), "recovery"))
	if err = store.Write(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(store.Dir, recoveryName(path)))
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("recovery copy is not private")
	}
	records, err := store.List(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("list recovery: %+v %v", records, err)
	}
	recovered, err := store.Load(context.Background(), records[0])
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Buffer.String() != "draft\n" || recovered.Buffer.LineEnding() != "\r\n" || !recovered.Dirty() {
		t.Fatal("recovery lost text, newline format, or dirty state")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "original\r\n" {
		t.Fatal("recovery wrote the workspace file")
	}
	if err = store.Remove(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	records, err = store.List(context.Background())
	if err != nil || len(records) != 0 {
		t.Fatal("saved/discarded draft was not removed")
	}
}

func TestSensitiveRecoveryNeverWrites(t *testing.T) {
	for _, name := range []string{".env", ".env.local", "private.key", "server.pem", "credentials.json", "secret-notes.md", "id_ed25519", ".ssh/config"} {
		if !Sensitive(filepath.Join(t.TempDir(), name)) {
			t.Errorf("sensitive name missed: %s", name)
		}
	}
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	os.WriteFile(path, []byte("VALUE=original"), 0600)
	d, _ := Open(context.Background(), path, root, Edit)
	d.Buffer.Set("VALUE=private draft")
	if _, err := d.RecoverySnapshot(); err == nil {
		t.Fatal("sensitive document generated a recovery plan")
	}
	store := *NewRecoveryStore(filepath.Join(t.TempDir(), "no-snapshots"))
	if err := store.Write(context.Background(), RecoveryPlan{record: RecoveryRecord{Path: path}}); err == nil {
		t.Fatal("store accepted a forged sensitive plan")
	}
	if _, err := os.Stat(store.Dir); !os.IsNotExist(err) {
		t.Fatal("sensitive recovery created disk state")
	}
}

func TestRecoveryRetainsExternalConflictAndRejectsDamage(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "todo.md")
	os.WriteFile(path, []byte("original"), 0600)
	d, _ := Open(context.Background(), path, root, Edit)
	d.Buffer.Set("draft")
	plan, _ := d.RecoverySnapshot()
	store := *NewRecoveryStore(filepath.Join(t.TempDir(), "recovery"))
	store.Write(context.Background(), plan)
	records, _ := store.List(context.Background())
	os.WriteFile(path, []byte("external after crash"), 0600)
	recovered, err := store.Load(context.Background(), records[0])
	if err != nil {
		t.Fatal(err)
	}
	save, _ := recovered.PrepareSave()
	result, err := save.Write(context.Background())
	recovered.CompleteSave(result, err)
	if err != ErrConflict {
		t.Fatal("recovered draft overwrote changes made after its baseline")
	}
	f, _ := os.OpenFile(filepath.Join(store.Dir, recoveryName(path)), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("damage")
	f.Close()
	if _, err = store.Load(context.Background(), records[0]); err == nil {
		t.Fatal("damaged recovery copy accepted")
	}
}
