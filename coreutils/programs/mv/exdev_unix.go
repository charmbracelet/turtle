//go:build unix

package mv

import "syscall"

// exdev is the error a rename across file systems fails with.
var exdev error = syscall.EXDEV
