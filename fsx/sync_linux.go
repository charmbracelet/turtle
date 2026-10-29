//go:build linux

package fsx

import "syscall"

// osSync flushes the data buffered by the whole file system, like sync(2).
// The syscall reports no error, as it's a best-effort flush.
func osSync() error {
	syscall.Sync()
	return nil
}
