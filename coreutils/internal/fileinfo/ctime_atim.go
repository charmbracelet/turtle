//go:build darwin || freebsd || ios || netbsd

package fileinfo

import (
	"io/fs"
	"syscall"
	"time"
)

// Ctime returns the time the status of a file last changed, when the system
// reports it.
func Ctime(fi fs.FileInfo) (time.Time, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(int64(st.Ctimespec.Sec), int64(st.Ctimespec.Nsec)), true
}
