//go:build windows

package coreutils

// osProcessAlive is nil on Windows, where watching processes isn't supported
// and the utilities which need it warn about that.
var osProcessAlive func(pid int) bool
