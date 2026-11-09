//go:build !windows

package chown

// unsupportedOwnership reports whether an error says the file system cannot
// carry ownership at all. Every other platform supports it.
func unsupportedOwnership(error) bool {
	return false
}
