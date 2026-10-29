//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package coreutils

import (
	"syscall"
	"unsafe"
)

// isTerminal reports whether the given file descriptor is open and refers to
// a terminal, which holds when the terminal driver answers for its
// attributes.
func isTerminal(fd int) bool {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL, uintptr(fd), ioctlReadTermios,
		uintptr(unsafe.Pointer(&termios)), 0, 0, 0,
	)
	return errno == 0
}
