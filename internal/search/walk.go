// Package search finds workspace files by name (Index, Rank) and text by
// content (Grep). It is ordinary Go: no UI imports and no app state, so a
// future non-UI tool can reuse it.
package search

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

// MaxFileSize is the largest file Grep reads, the same limit as
// documents.MaxFileSize (internal/guardrails asserts they are equal).
const MaxFileSize = 20 << 20

// ignoredDirs are tool-generated directories skipped by every walk. This is
// the only ignore list; internal/guardrails rejects a second one.
var ignoredDirs = map[string]bool{
	".git": true, "node_modules": true, ".venv": true, "vendor": true, "dist": true,
	"build": true, ".next": true, "target": true, "__pycache__": true,
}

// walk visits the regular files under r in a fixed order (each directory's
// files by name, then its subdirectories by name), with paths relative
// to r and '/'-separated. It never descends into ignored directories or into a
// directory reached through a symlink; a symlink to a regular file inside r is
// visited (os.Root rejects absolute symlink targets, so only relative ones).
// Dotfiles such as .env are included. Entries that disappear or can't be read
// during the walk are skipped. It stops when ctx is done or visit
// returns an error, and returns that error.
func walk(ctx context.Context, r *os.Root, visit func(rel string) error) error {
	stack := []string{"."}
	for len(stack) > 0 {
		dir := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := readDir(r, dir)
		if err != nil {
			continue
		}
		var subdirs []string
		for _, e := range entries {
			rel := path.Join(dir, e.Name())
			switch {
			case e.IsDir():
				if !ignoredDirs[e.Name()] {
					subdirs = append(subdirs, rel)
				}
			case e.Type().IsRegular():
				if err := visit(rel); err != nil {
					return err
				}
			case e.Type()&fs.ModeSymlink != 0:
				// Follow file symlinks only; os.Root keeps the target inside r.
				if info, err := r.Stat(filepath.FromSlash(rel)); err == nil && info.Mode().IsRegular() {
					if err := visit(rel); err != nil {
						return err
					}
				}
			}
		}
		// Push in reverse so the stack pops subdirectories in lexical order.
		for i := len(subdirs) - 1; i >= 0; i-- {
			stack = append(stack, subdirs[i])
		}
	}
	return ctx.Err()
}

func readDir(r *os.Root, dir string) ([]fs.DirEntry, error) {
	f, err := r.Open(filepath.FromSlash(dir))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, err
}
