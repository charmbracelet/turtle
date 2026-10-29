//go:build darwin || dragonfly || freebsd || ios || netbsd || openbsd

package fsx

import "syscall"

// osSync flushes the data buffered by the whole file system, like sync(2).
func osSync() error {
	return syscall.Sync()
}
