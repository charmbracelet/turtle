//go:build windows

package mv

import "syscall"

// exdev is the error a rename across volumes fails with: ERROR_NOT_SAME_DEVICE,
// which the syscall package doesn't export.
var exdev error = syscall.Errno(433)
