package documents

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui/editor"
	"github.com/jewei/wondervision/internal/workspace"
)

func TestOpenTextAndEditOnlyInMemory(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []byte
		lang string
		md   bool
	}{
		{"report.md", []byte("# Report\n"), "Markdown", true},
		{".env", []byte("\xef\xbb\xbfKEY=value\r\n"), "Shell", false},
		{"config.yaml", []byte("enabled: true"), "YAML", false},
		{"unknown.custom", []byte("ordinary UTF-8: 世界"), "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.name)
			if err := os.WriteFile(path, tc.raw, 0600); err != nil {
				t.Fatal(err)
			}
			d, err := Open(context.Background(), path, "", Preview)
			if err != nil {
				t.Fatal(err)
			}
			if d.Markdown != tc.md || (d.Markdown && d.Mode != Preview) || (!d.Markdown && d.Mode != Edit) {
				t.Fatal("incorrect document mode")
			}
			if tc.lang != "" && (d.Buffer.Language() == nil || d.Buffer.Language().Name != tc.lang) {
				t.Fatalf("language: %+v", d.Buffer.Language())
			}
			if tc.name == ".env" && (!d.BOM || d.Buffer.LineEnding() != "\r\n" || d.Buffer.String() != "KEY=value\n") {
				t.Fatal("BOM or newline detection failed")
			}
			d.Buffer.Edit(editor.Edit{Start: 0, Text: "edited "})
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tc.raw) {
				t.Fatal("typing changed the disk file")
			}
		})
	}
}

func TestInvalidAndLargeFiles(t *testing.T) {
	root := t.TempDir()
	for i, raw := range [][]byte{{0, 1, 2}, {0xff, 0xfe, 'a'}, {0xc0, 0xaf}} {
		path := filepath.Join(root, string(rune('a'+i)))
		os.WriteFile(path, raw, 0600)
		if _, err := Open(context.Background(), path, "", Preview); err == nil {
			t.Fatal("non-UTF-8 text accepted")
		}
	}
	path := filepath.Join(root, "large.md")
	os.WriteFile(path, bytes.Repeat([]byte("a"), LargeMarkdown+1), 0600)
	d, err := Open(context.Background(), path, "", Preview)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Large || d.Mode != Edit {
		t.Fatal("large Markdown should require explicit preview")
	}
	f, err := os.Create(filepath.Join(root, "huge.txt"))
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(MaxFileSize + 1)
	f.Close()
	if _, err := Open(context.Background(), f.Name(), "", Preview); err == nil {
		t.Fatal("oversized file accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(ctx, path, "", Preview); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := Open(context.Background(), root, "", Preview); err == nil {
		t.Fatal("directory accepted as text")
	}
}

func TestFileIdentityAndOutsideScope(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	os.WriteFile(path, []byte("text"), 0600)
	d, err := Open(context.Background(), path, "", Preview)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias.md")
	if err := os.Link(path, alias); err != nil {
		t.Skip(err)
	}
	other, err := Open(context.Background(), alias, "", Preview)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Same(other) {
		t.Fatal("hard links should share a document")
	}
	if _, err := Open(context.Background(), path, t.TempDir(), Preview); err == nil {
		t.Fatal("outside workspace file accepted")
	}
	if canonical, _ := workspace.Canonical(root); d.Area != canonical {
		t.Fatal("manually opened file gained sibling folder scope")
	}
}
