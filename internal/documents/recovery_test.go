package documents

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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
	store := RecoveryStore{Dir: filepath.Join(t.TempDir(), "recovery")}
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
	store := RecoveryStore{Dir: filepath.Join(t.TempDir(), "no-snapshots")}
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
	store := RecoveryStore{Dir: filepath.Join(t.TempDir(), "recovery")}
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
