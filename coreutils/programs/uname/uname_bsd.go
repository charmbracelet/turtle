//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package uname

import "syscall"

// hostFields gathers the system information of the host from the sysctl
// interface of the kernel, leaving empty whatever it cannot report.
func hostFields() (fields, error) {
	sysname := sysctl("kern.ostype")
	machine := sysctl("hw.machine")
	return fields{
		sysname:  sysname,
		nodename: sysctl("kern.hostname"),
		release:  sysctl("kern.osrelease"),
		version:  sysctl("kern.version"),
		machine:  machine,
		// The hardware platform repeats the machine value, as the GNU
		// utility reports on Linux, and the processor type stays unknown.
		hardwarePlatform: machine,
		osName:           sysname,
	}, nil
}

// sysctl returns the string value of a sysctl variable, or empty when the
// kernel doesn't report it.
func sysctl(name string) string {
	value, err := syscall.Sysctl(name)
	if err != nil {
		return ""
	}
	return value
}
