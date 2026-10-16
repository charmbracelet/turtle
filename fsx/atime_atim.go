//go:build aix || dragonfly || illumos || linux || openbsd || solaris

package fsx

import (
	"syscall"
	"time"
)

func sysAtime(sys any) (time.Time, bool) {
	if st, ok := sys.(*syscall.Stat_t); ok {
		return time.Unix(int64(st.Atim.Sec), int64(st.Atim.Nsec)), true
	}
	return time.Time{}, false
}
