//go:build darwin || freebsd || ios || netbsd

package stat

import (
	"syscall"
	"time"
)

// sysBtime returns the birth time of a file, when the system reports it.
func sysBtime(sys any) (time.Time, bool) {
	if st, ok := sys.(*syscall.Stat_t); ok {
		if st.Birthtimespec.Sec == 0 && st.Birthtimespec.Nsec == 0 {
			return time.Time{}, false
		}
		return time.Unix(int64(st.Birthtimespec.Sec), int64(st.Birthtimespec.Nsec)), true
	}
	return time.Time{}, false
}
