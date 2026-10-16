//go:build aix || dragonfly || illumos || linux || openbsd || solaris

package ls

import (
	"syscall"
	"time"
)

// sysCtime returns the time the status of a file last changed, when the
// system reports it.
func sysCtime(sys any) (time.Time, bool) {
	if st, ok := sys.(*syscall.Stat_t); ok {
		return time.Unix(int64(st.Ctim.Sec), int64(st.Ctim.Nsec)), true
	}
	return time.Time{}, false
}
