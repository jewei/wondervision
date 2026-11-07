//go:build !darwin && (!linux || (!amd64 && !arm64))

package workspace

import (
	"errors"
	"os"
)

func renameExclusive(*os.Root, string, *os.Root, string) error { return errors.ErrUnsupported }
