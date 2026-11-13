//go:build windows

package chown

import (
	"errors"
	"syscall"
)

// unsupportedOwnership reports whether an error says the file system cannot
// carry ownership at all, like the one os.Chown returns on Windows. Changing
// the ownership is then a no-op.
func unsupportedOwnership(err error) bool {
	return errors.Is(err, syscall.EWINDOWS)
}
