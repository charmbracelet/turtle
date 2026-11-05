//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package coreutils

// osExecUsage reports that the platform carries no resource usage counts
// for executed children.
func osExecUsage() (ExecUsage, bool) {
	return ExecUsage{}, false
}
