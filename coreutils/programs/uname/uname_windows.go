//go:build windows

package uname

import (
	"runtime"
	"strconv"
	"syscall"
)

// hostFields gathers the system information of the host from the Windows
// version API and the runtime. The version API reports the values Windows
// keeps for processes without an application manifest, which on Windows 8
// and later means 6.2.9200 whatever the actual release is.
func hostFields() (fields, error) {
	f := fields{
		sysname:  "Windows_NT",
		nodename: hostname(),
		machine:  goarchMachine(runtime.GOARCH),
		osName:   "Windows_NT",
	}
	if ver, err := syscall.GetVersion(); err == nil {
		major, minor, build := ver&0xff, ver>>8&0xff, ver>>16
		f.release = strconv.FormatUint(uint64(major), 10) + "." +
			strconv.FormatUint(uint64(minor), 10)
		f.version = "build " + strconv.FormatUint(uint64(build), 10)
	}
	return f, nil
}
