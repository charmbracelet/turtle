//go:build !(darwin || freebsd || ios || netbsd)

package fileinfo

import (
	"io/fs"
	"time"
)

// Btime returns the birth time of a file, which the systems here don't
// report.
func Btime(fs.FileInfo) (time.Time, bool) {
	return time.Time{}, false
}
