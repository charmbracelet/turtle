//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !windows

package term

// IsTerminal reports whether the given file descriptor is open and refers to
// a terminal. Terminals can't be told apart on this platform, so it's always
// false.
func IsTerminal(fd int) bool {
	return false
}
