//go:build darwin

package workspace

import (
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

// renameExclusive renames without replacing an existing destination:
// renameatx_np with RENAME_EXCL, macOS's renameat2(RENAME_NOREPLACE).
func renameExclusive(oldRoot *os.Root, oldName string, newRoot *os.Root, newName string) error {
	oldDir, err := oldRoot.Open(".")
	if err != nil {
		return err
	}
	defer oldDir.Close()
	newDir, err := newRoot.Open(".")
	if err != nil {
		return err
	}
	defer newDir.Close()
	err = unix.RenameatxNp(int(oldDir.Fd()), oldName, int(newDir.Fd()), newName, unix.RENAME_EXCL)
	runtime.KeepAlive(oldDir)
	runtime.KeepAlive(newDir)
	return err
}
