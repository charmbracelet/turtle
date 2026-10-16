//go:build windows

package fsx

import (
	"syscall"
	"time"
)

func sysAtime(sys any) (time.Time, bool) {
	if st, ok := sys.(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, st.LastAccessTime.Nanoseconds()), true
	}
	return time.Time{}, false
}
