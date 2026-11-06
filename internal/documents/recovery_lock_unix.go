//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package documents

import (
	"errors"
	"os"
	"syscall"
)

func lockRecoveryFile(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrRecoveryBusy
	}
	return err
}
