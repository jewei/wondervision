//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package documents

import "os"

// Fail closed until this platform has a tested process-lock adapter.
func lockRecoveryFile(*os.File) error { return os.ErrUnsupported }
