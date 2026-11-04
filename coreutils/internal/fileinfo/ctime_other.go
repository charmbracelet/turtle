//go:build !(aix || dragonfly || illumos || linux || openbsd || solaris || darwin || freebsd || ios || netbsd)

package fileinfo

import (
	"io/fs"
	"time"
)

// Ctime returns the time the status of a file last changed, which the
// systems here don't report.
func Ctime(fs.FileInfo) (time.Time, bool) {
	return time.Time{}, false
}
