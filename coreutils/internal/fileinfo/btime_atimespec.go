//go:build darwin || freebsd || ios || netbsd

package fileinfo

import (
	"io/fs"
	"syscall"
	"time"
)

// Btime returns the birth time of a file, when the system reports it.
func Btime(fi fs.FileInfo) (time.Time, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	if st.Birthtimespec.Sec == 0 && st.Birthtimespec.Nsec == 0 {
		return time.Time{}, false
	}
	return time.Unix(int64(st.Birthtimespec.Sec), int64(st.Birthtimespec.Nsec)), true
}
