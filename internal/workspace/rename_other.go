//go:build !linux || (!amd64 && !arm64)

package workspace

import "os"

func renameExclusive(*os.Root, string, *os.Root, string) error { return os.ErrUnsupported }
