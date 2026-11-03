//go:build !darwin && !freebsd && !ios && !netbsd

package ls

import "time"

// sysBtime returns the birth time of a file, which the systems here don't
// report.
func sysBtime(any) (time.Time, bool) {
	return time.Time{}, false
}
