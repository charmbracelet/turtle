//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package fsx

import "time"

func sysAtime(any) (time.Time, bool) {
	return time.Time{}, false
}
