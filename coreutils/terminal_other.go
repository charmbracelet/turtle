//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !windows

package coreutils

// isTerminal reports whether the given file descriptor is open and refers to
// a terminal. Terminals can't be told apart on this platform, so it's always
// false.
func isTerminal(fd int) bool {
	return false
}
