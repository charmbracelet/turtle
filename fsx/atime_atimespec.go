//go:build darwin || freebsd || ios || netbsd

package fsx

import (
	"syscall"
	"time"
)

func sysAtime(sys any) (time.Time, bool) {
	if st, ok := sys.(*syscall.Stat_t); ok {
		return time.Unix(int64(st.Atimespec.Sec), int64(st.Atimespec.Nsec)), true
	}
	return time.Time{}, false
}
