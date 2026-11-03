//go:build !windows

package coreutils

import (
	"errors"
	"os"
	"syscall"
)

// osProcessAlive reports whether the process with the given ID exists, as
// kill(pid, 0) does.
func osProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
