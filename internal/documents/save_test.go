package documents

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func TestSavePreservesTextFormatAndPermissions(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n"} {
		for _, bom := range []bool{false, true} {
			for _, final := range []bool{false, true} {
				t.Run(ending+map[bool]string{true: "BOM", false: "UTF8"}[bom]+map[bool]string{true: "final", false: "no-final"}[final], func(t *testing.T) {
					original := []byte("KEY=old")
					if final {
						original = append(original, []byte(ending)...)
					}
					if bom {
						original = append([]byte{0xef, 0xbb, 0xbf}, original...)
					}
					path := filepath.Join(t.TempDir(), ".env")
					if err := os.WriteFile(path, original, 0640); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(path, 0640); err != nil {
						t.Fatal(err)
					}
					d, err := Open(context.Background(), path, "", Edit)
					if err != nil {
						t.Fatal(err)
					}
					d.Buffer.Edit(editor.Edit{Start: 4, End: 7, Text: "new"})
					before, _ := os.ReadFile(path)
					if !bytes.Equal(before, original) {
						t.Fatal("typing wrote to disk")
					}
					plan, err := d.PrepareSave()
					if err != nil {
						t.Fatal(err)
					}
					result, err := plan.Write(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					if !d.CompleteSave(result, nil) || d.Dirty() {
						t.Fatal("saved document was not marked clean")
					}
					expected := bytes.Replace(original, []byte("old"), []byte("new"), 1)
					actual, _ := os.ReadFile(path)
					if !bytes.Equal(actual, expected) {
						t.Fatalf("format changed: got %q want %q", actual, expected)
					}
					info, _ := os.Stat(path)
					if info.Mode().Perm() != 0640 {
						t.Fatalf("mode changed: %v", info.Mode())
					}
				})
			}
		}
	}
}

func TestSaveKeepsNewerEditsDirty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.md")
	os.WriteFile(path, []byte("old\n"), 0600)
	d, _ := Open(context.Background(), path, "", Edit)
	d.Buffer.Set("snapshot\n")
	plan, _ := d.PrepareSave()
	if _, err := d.PrepareSave(); err == nil {
		t.Fatal("overlapping save accepted")
	}
	result, err := plan.Write(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d.Buffer.Set("newer edit\n")
	if !d.CompleteSave(result, nil) || !d.Dirty() {
		t.Fatal("save completion marked newer edits clean")
	}
	disk, _ := os.ReadFile(path)
	if string(disk) != "snapshot\n" || d.Buffer.String() != "newer edit\n" {
		t.Fatal("save mixed disk snapshot and current buffer")
	}
	if d.CompleteSave(result, nil) {
		t.Fatal("duplicate receipt accepted")
	}
	d.Buffer.Set("snapshot\n")
	if d.Dirty() {
		t.Fatal("returning to saved content remains dirty")
	}
}

func TestSaveRejectsKnownConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("original"), 0600)
	d, _ := Open(context.Background(), path, "", Edit)
	d.Buffer.Set("mine")
	plan, _ := d.PrepareSave()
	os.WriteFile(path, []byte("external"), 0600)
	result, err := plan.Write(context.Background())
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("known external change was not blocked: %v", err)
	}
	d.CompleteSave(result, err)
	disk, _ := os.ReadFile(path)
	if string(disk) != "external" || !d.Dirty() {
		t.Fatal("known conflict overwrote external content or cleared edits")
	}
	root, _ := os.OpenRoot(filepath.Dir(path))
	observed, _, _ := fingerprintAt(context.Background(), root, filepath.Base(path))
	root.Close()
	plan, _ = d.PrepareOverwrite(observed)
	result, err = plan.Write(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d.CompleteSave(result, err)
	disk, _ = os.ReadFile(path)
	if string(disk) != "mine" || d.Dirty() {
		t.Fatal("explicit overwrite did not save")
	}
}

func TestSaveFailureKeepsOriginal(t *testing.T) {
	for _, scenario := range []string{"read-only-file", "read-only-directory", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "note.txt")
			os.WriteFile(path, []byte("original"), 0600)
			d, _ := Open(context.Background(), path, "", Edit)
			d.Buffer.Set("edited")
			plan, _ := d.PrepareSave()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "read-only-file":
				os.Chmod(path, 0400)
				defer os.Chmod(path, 0600)
			case "read-only-directory":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses directory mode")
				}
				os.Chmod(dir, 0500)
				defer os.Chmod(dir, 0700)
			case "cancelled":
				cancel()
			}
			result, err := plan.Write(ctx)
			if err == nil {
				t.Fatal("failed write was accepted")
			}
			d.CompleteSave(result, err)
			raw, _ := os.ReadFile(path)
			if string(raw) != "original" || !d.Dirty() {
				t.Fatal("failed save changed original or discarded edits")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 1 {
				t.Fatal("failed save left a temporary file")
			}
		})
	}
}

func TestSaveAsNeverOverwritesAndRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "note.txt")
	target := filepath.Join(root, "copy.txt")
	os.WriteFile(source, []byte("source"), 0600)
	os.WriteFile(target, []byte("existing"), 0600)
	d, _ := Open(context.Background(), source, "", Edit)
	d.Buffer.Set("edited")
	plan, _ := d.PrepareSaveAs(target)
	result, err := plan.Write(context.Background())
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatal(err)
	}
	d.CompleteSave(result, err)
	raw, _ := os.ReadFile(target)
	if string(raw) != "existing" {
		t.Fatal("Save As overwrote destination")
	}
	os.Remove(target)
	plan, _ = d.PrepareSaveAs(target)
	result, err = plan.Write(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d.CompleteSave(result, err)
	if d.Path != target || d.Dirty() {
		t.Fatal("Save As did not update document identity")
	}
	raw, _ = os.ReadFile(source)
	if string(raw) != "source" {
		t.Fatal("Save As changed original")
	}
	alias := filepath.Join(root, "alias.txt")
	if err := os.Symlink(source, alias); err != nil {
		t.Skip(err)
	}
	plan, _ = d.PrepareSaveAs(alias)
	result, err = plan.Write(context.Background())
	if err == nil {
		t.Fatal("final symlink destination accepted")
	}
	d.CompleteSave(result, err)
}
