package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestListIsShallowAndShowsDotfiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "deep"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env", "AGENTS.md", "report.md", "docs/deep/hidden.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("text"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	w, err := Open(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.List(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 || !entries[0].Directory || entries[0].Name != "docs" {
		t.Fatalf("unexpected shallow listing: %+v", entries)
	}
	if entries[1].Name != ".env" {
		t.Fatal("dotfile was hidden")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := w.List(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := w.List(context.Background(), t.TempDir()); err == nil {
		t.Fatal("outside directory accepted")
	}
}

func TestLocalURLsStayWithinAuthorizedArea(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	doc := filepath.Join(root, "report.md")
	for _, path := range []string{doc, filepath.Join(root, "image with space.png"), filepath.Join(outside, "private.md")} {
		if err := os.WriteFile(path, []byte("content"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path, fragment, err := ResolveLocal(root, doc, "image%20with%20space.png#sample")
	if err != nil || filepath.Base(path) != "image with space.png" || fragment != "sample" {
		t.Fatalf("resolve: %s %s %v", path, fragment, err)
	}
	for _, raw := range []string{"https://example.com/image.png", "//example.com/image.png", "file:///etc/passwd", "javascript:alert(1)", "/etc/passwd", "%2e%2e/private.md", "..%5cprivate.md", "image%20with%20space.png?x=1"} {
		if _, _, err := ResolveLocal(root, doc, raw); err == nil {
			t.Errorf("unsafe URL accepted: %s", raw)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	if _, _, err := ResolveLocal(root, doc, "escape/private.md"); err == nil {
		t.Fatal("symlink escape accepted")
	}
	w, _ := Open(context.Background(), root)
	entries, err := w.List(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == "escape" && (!e.Symlink || e.Directory) {
			t.Fatal("directory symlink is expandable")
		}
	}
}

func TestRootedOpenRejectsSymlinkSwapAfterResolution(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	root, _ = Canonical(root) // Contains compares resolved paths
	path := filepath.Join(root, "image.png")
	os.WriteFile(path, []byte("inside"), 0600)
	target := filepath.Join(outside, "private.png")
	os.WriteFile(target, []byte("outside"), 0600)
	checked, err := Canonical(path)
	if err != nil {
		t.Fatal(err)
	}
	if !Contains(root, checked) {
		t.Fatal("fixture is outside root")
	}
	os.Remove(path)
	if err := os.Symlink(target, path); err != nil {
		t.Skip(err)
	}
	if f, err := OpenWithin(root, checked); err == nil {
		f.Close()
		t.Fatal("rooted open followed a symlink swapped in after validation")
	}
}
