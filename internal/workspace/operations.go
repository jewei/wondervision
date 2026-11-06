package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Change struct {
	OldPath, NewPath string
	Directory        bool
}

func relativePath(raw string) (string, error) {
	portable := strings.ReplaceAll(raw, "\\", "/")
	if portable == "" || strings.ContainsRune(portable, 0) || strings.HasPrefix(portable, "/") {
		return "", errors.New("use a path relative to the workspace")
	}
	for _, part := range strings.Split(portable, "/") {
		if part == ".." || part == "." {
			return "", errors.New("traversal paths are not allowed")
		}
	}
	path := filepath.FromSlash(portable)
	if !filepath.IsLocal(path) || path == "." {
		return "", errors.New("path is outside the workspace")
	}
	return filepath.Clean(path), nil
}

func (w *Workspace) operationParent(raw string) (*os.Root, string, string, error) {
	rel, err := relativePath(raw)
	if err != nil {
		return nil, "", "", err
	}
	root, err := os.OpenRoot(w.Root)
	if err != nil {
		return nil, "", "", err
	}
	defer root.Close()
	parts := strings.Split(rel, string(filepath.Separator))
	for i := 1; i < len(parts); i++ {
		info, err := root.Lstat(filepath.Join(parts[:i]...))
		if err != nil {
			return nil, "", "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, "", "", errors.New("symlink directories are not allowed in workspace file actions")
		}
	}
	expected, err := root.Stat(filepath.Dir(rel))
	if err != nil {
		return nil, "", "", err
	}
	parent, err := root.OpenRoot(filepath.Dir(rel))
	if err != nil {
		return nil, "", "", err
	}
	opened, err := parent.Open(".")
	if err != nil {
		parent.Close()
		return nil, "", "", err
	}
	actual, err := opened.Stat()
	opened.Close()
	if err != nil || !os.SameFile(expected, actual) {
		parent.Close()
		return nil, "", "", errors.New("parent directory changed while preparing the operation")
	}
	return parent, filepath.Base(rel), rel, nil
}

func (w *Workspace) Create(ctx context.Context, path string, directory bool) (Change, error) {
	if err := ctx.Err(); err != nil {
		return Change{}, err
	}
	parent, name, rel, err := w.operationParent(path)
	if err != nil {
		return Change{}, err
	}
	defer parent.Close()
	if directory {
		err = parent.Mkdir(name, 0700)
	} else {
		var f *os.File
		f, err = parent.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			err = f.Sync()
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		return Change{}, err
	}
	return Change{NewPath: filepath.Join(w.Root, rel), Directory: directory}, nil
}

func (w *Workspace) Move(ctx context.Context, source, destination string) (Change, error) {
	if err := ctx.Err(); err != nil {
		return Change{}, err
	}
	oldParent, oldName, oldRel, err := w.operationParent(source)
	if err != nil {
		return Change{}, err
	}
	defer oldParent.Close()
	info, err := oldParent.Lstat(oldName)
	if err != nil {
		return Change{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Change{}, errors.New("symlink entries cannot be moved by workspace actions")
	}
	newParent, newName, newRel, err := w.operationParent(destination)
	if err != nil {
		return Change{}, err
	}
	defer newParent.Close()
	if oldRel == newRel || (info.IsDir() && strings.HasPrefix(newRel, oldRel+string(filepath.Separator))) {
		return Change{}, errors.New("a folder cannot be moved into itself")
	}
	if _, err := newParent.Lstat(newName); err == nil {
		return Change{}, os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return Change{}, err
	}
	if err = ctx.Err(); err != nil {
		return Change{}, err
	}
	if err = renameExclusive(oldParent, oldName, newParent, newName); err != nil {
		return Change{}, fmt.Errorf("move without replacing a destination: %w", err)
	}
	return Change{OldPath: filepath.Join(w.Root, oldRel), NewPath: filepath.Join(w.Root, newRel), Directory: info.IsDir()}, nil
}

func (w *Workspace) Delete(ctx context.Context, path string) (Change, error) {
	parent, name, rel, err := w.operationParent(path)
	if err != nil {
		return Change{}, err
	}
	defer parent.Close()
	info, err := parent.Lstat(name)
	if err != nil {
		return Change{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Change{}, errors.New("symlink entries cannot be deleted by workspace actions")
	}
	if err = removeTree(ctx, parent, name); err != nil {
		return Change{}, fmt.Errorf("deletion stopped; some entries may already be removed: %w", err)
	}
	return Change{OldPath: filepath.Join(w.Root, rel), Directory: info.IsDir()}, nil
}

func removeTree(ctx context.Context, parent *os.Root, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := parent.Lstat(name)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return parent.Remove(name)
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return err
	}
	f, err := child.Open(".")
	if err != nil {
		child.Close()
		return err
	}
	defer f.Close()
	defer child.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return errors.New("folder changed while preparing deletion")
	}
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		entries, readErr := f.ReadDir(64)
		for _, entry := range entries {
			if err = removeTree(ctx, child, entry.Name()); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	f.Close()
	child.Close()
	return parent.Remove(name)
}
