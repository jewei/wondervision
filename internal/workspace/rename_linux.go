//go:build linux && (amd64 || arm64)

package workspace

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

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
	old, err := syscall.BytePtrFromString(oldName)
	if err != nil {
		return err
	}
	next, err := syscall.BytePtrFromString(newName)
	if err != nil {
		return err
	}
	_, _, errno := syscall.Syscall6(renameAt2, oldDir.Fd(), uintptr(unsafe.Pointer(old)), newDir.Fd(), uintptr(unsafe.Pointer(next)), 1, 0)
	runtime.KeepAlive(oldDir)
	runtime.KeepAlive(newDir)
	runtime.KeepAlive(old)
	runtime.KeepAlive(next)
	if errno != 0 {
		return errno
	}
	return nil
}
