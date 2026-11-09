//go:build !unix

package stat

// sysBlksize returns the optimal I/O transfer size, which the systems here
// don't report.
func sysBlksize(any) (int64, bool) {
	return 0, false
}

// sysRdev returns the device type a special file stands for, which the
// systems here don't report.
func sysRdev(any) uint64 {
	return 0
}
