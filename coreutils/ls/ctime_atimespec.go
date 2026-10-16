//go:build darwin || freebsd || ios || netbsd

package ls

import (
	"syscall"
	"time"
)

// sysCtime returns the time the status of a file last changed, when the
// system reports it.
func sysCtime(sys any) (time.Time, bool) {
	if st, ok := sys.(*syscall.Stat_t); ok {
		return time.Unix(int64(st.Ctimespec.Sec), int64(st.Ctimespec.Nsec)), true
	}
	return time.Time{}, false
}
