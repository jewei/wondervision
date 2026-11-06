// Package workspace provides cancellable, nonrecursive filesystem reads.
package workspace

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Entry struct {
	Path      string
	Name      string
	Directory bool
	Symlink   bool
}

type Workspace struct{ Root string }

func Canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func Contains(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(rel)
}

// OpenWithin lets the OS enforce containment while resolving the path, so a
// concurrent symlink swap cannot redirect a checked read outside its area.
func OpenWithin(area, path string) (*os.File, error) {
	rel, err := filepath.Rel(area, path)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, errors.New("file is outside the authorized folder")
	}
	root, err := os.OpenRoot(area)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(rel)
}

func Open(ctx context.Context, path string) (*Workspace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := Canonical(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("choose a directory to open a workspace")
	}
	return &Workspace{Root: root}, ctx.Err()
}

// List reads only this directory. Directory symlinks are never expanded.
func (w *Workspace) List(ctx context.Context, path string) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	real, err := Canonical(path)
	if err != nil {
		return nil, err
	}
	if !Contains(w.Root, real) {
		return nil, errors.New("directory is outside the workspace")
	}
	f, err := OpenWithin(w.Root, real)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries := make([]Entry, 0)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch, err := f.ReadDir(128)
		for _, d := range batch {
			entries = append(entries, Entry{Path: filepath.Join(path, d.Name()), Name: d.Name(), Directory: d.IsDir(), Symlink: d.Type()&os.ModeSymlink != 0})
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return nil, err
			}
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Directory != entries[j].Directory {
			return entries[i].Directory
		}
		a, b := strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name)
		if a == b {
			return entries[i].Name < entries[j].Name
		}
		return a < b
	})
	return entries, nil
}

// ResolveLocal accepts relative local URLs and checks canonical containment,
// including symlinks. A manually opened file uses its parent as its area.
func ResolveLocal(area, document, raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}
	if u.Scheme != "" || u.Host != "" || u.RawQuery != "" || filepath.IsAbs(u.Path) || strings.ContainsAny(u.Path, "\\\x00") {
		return "", "", errors.New("only relative local file links are allowed")
	}
	path := document
	if u.Path != "" {
		path = filepath.Join(filepath.Dir(document), filepath.FromSlash(u.Path))
	}
	path, err = Canonical(path)
	if err != nil {
		return "", "", err
	}
	if !Contains(area, path) {
		return "", "", errors.New("linked file is outside the authorized folder")
	}
	return path, u.Fragment, nil
}
