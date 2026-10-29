//go:build !linux && !darwin && !dragonfly && !freebsd && !ios && !netbsd && !openbsd

package fsx

// osSync is a no-op on systems without a global sync, like Windows.
func osSync() error {
	return nil
}
