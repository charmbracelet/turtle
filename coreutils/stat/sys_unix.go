//go:build unix

package stat

import "syscall"

// sysBlksize returns the optimal I/O transfer size the system reports.
func sysBlksize(sys any) (int64, bool) {
	if st, ok := sys.(*syscall.Stat_t); ok {
		return int64(st.Blksize), true
	}
	return 0, false
}

// sysRdev returns the device type a special file stands for, zero for
// every other file.
func sysRdev(sys any) uint64 {
	if st, ok := sys.(*syscall.Stat_t); ok {
		return uint64(st.Rdev)
	}
	return 0
}
