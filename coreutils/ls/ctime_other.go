//go:build !(aix || dragonfly || illumos || linux || openbsd || solaris || darwin || freebsd || ios || netbsd)

package ls

import "time"

// sysCtime returns the time the status of a file last changed, which the
// systems here don't report.
func sysCtime(any) (time.Time, bool) {
	return time.Time{}, false
}
