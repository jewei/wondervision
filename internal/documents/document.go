// Package documents owns editable buffers and their accepted disk versions.
package documents

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
	"github.com/jewei/wondervision/internal/workspace"
)

const MaxFileSize = 20 << 20
const LargeMarkdown = 2 << 20

type Mode int

const (
	Edit Mode = iota
	Preview
	Split
)

type Document struct {
	Path            string
	Area            string
	Info            os.FileInfo
	Buffer          *editor.Buffer
	Editor          *editor.State
	Markdown        bool
	Mode            Mode
	BOM             bool
	Large           bool
	Anchor          string
	state           saveState
	saved           Fingerprint
	revisionVersion uint64
	revisionValue   Revision
	revisionKnown   bool
}

func Open(ctx context.Context, path, area string, markdownMode Mode) (*Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := workspace.Canonical(path)
	if err != nil {
		return nil, err
	}
	if area != "" && !workspace.Contains(area, path) {
		return nil, errors.New("file is outside the authorized folder")
	}
	before, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("only regular text files can be opened")
	}
	if area == "" {
		area = filepath.Dir(path)
	}
	f, err := workspace.OpenWithin(area, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("only regular text files can be opened")
	}
	if info.Size() > MaxFileSize {
		return nil, errors.New("file exceeds 20 MiB; large-file loading is not supported in this milestone")
	}
	var data bytes.Buffer
	chunk := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := f.Read(chunk)
		if data.Len()+n > MaxFileSize {
			return nil, errors.New("file grew beyond the 20 MiB loading limit")
		}
		data.Write(chunk[:n])
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	raw := data.Bytes()
	fingerprint := Fingerprint{Hash: sha256.Sum256(raw), Exists: true}
	bom := bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	if bom {
		raw = raw[3:]
	}
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return nil, errors.New("unsupported file: expected UTF-8 text without binary NUL bytes")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if area == "" {
		area = filepath.Dir(path)
	}
	b := editor.NewBuffer(string(raw))
	b.SetLanguage(language(path))
	ext := strings.ToLower(filepath.Ext(path))
	md := ext == ".md" || ext == ".markdown"
	d := &Document{Path: path, Area: area, Info: info, Buffer: b, Editor: &editor.State{Buffer: b}, Markdown: md, BOM: bom, Large: md && len(raw) > LargeMarkdown}
	d.saved = fingerprint
	d.state.saved = snapshotRevision(b.Snapshot())
	if md && !d.Large {
		d.Mode = markdownMode
	}
	return d, nil
}

func language(path string) *editor.Language {
	name := strings.ToLower(filepath.Base(path))
	if name == ".env" || strings.HasPrefix(name, ".env.") || name == ".gitignore" || name == ".editorconfig" {
		return editor.LanguageNamed("Shell")
	}
	return editor.LanguageFor(path)
}

func (d *Document) Same(other *Document) bool {
	return d.Path == other.Path || (d.Info != nil && other.Info != nil && os.SameFile(d.Info, other.Info))
}
