package documents

import (
	"context"
	"errors"
	"os"
)

var ErrRecoveryBusy = errors.New("another WonderVision session is updating recovery; this draft was not changed")

// A single advisory lock serializes app-owned recovery check/write/remove across
// processes. It never waits or polls; the OS releases it when the holder exits.
// Retain the empty lock file so every session locks the same inode.
func recoveryLock(ctx context.Context, root *os.Root) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if info, err := root.Lstat(".lock"); err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("recovery lock must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := root.OpenFile(".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockRecoveryFile(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
