//go:build windows

package term

import "syscall"

// IsTerminal reports whether the given file descriptor is open and refers to
// a console, which only console handles answer a mode query for.
func IsTerminal(fd int) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(fd), &mode) == nil
}
