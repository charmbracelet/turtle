//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !windows

package uname

import "runtime"

// The constraint above excludes android too: it satisfies linux, so it
// collects through the uname(2) files instead.

// hostFields gathers what the runtime knows about the host, as the platform
// exposes nothing further through the standard library.
func hostFields() (fields, error) {
	return fields{
		sysname:  runtime.GOOS,
		nodename: hostname(),
		machine:  goarchMachine(runtime.GOARCH),
		osName:   runtime.GOOS,
	}, nil
}
