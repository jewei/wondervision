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
	var f *os.File
	for attempt := 0; ; attempt++ {
		if info, err := root.Lstat(".lock"); err == nil && !info.Mode().IsRegular() {
			return nil, errors.New("recovery lock must be a regular file")
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		var err error
		f, err = root.OpenFile(".lock", os.O_CREATE|os.O_RDWR, 0600)
		// On macOS, opening with O_CREATE while another session creates the
		// same file can fail with ENOENT; by then the file exists, so try
		// again at once (TestRecoveryLockCreationRace).
		if errors.Is(err, os.ErrNotExist) && attempt < 3 {
			continue
		}
		if err != nil {
			return nil, err
		}
		break
	}
	if err := lockRecoveryFile(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
