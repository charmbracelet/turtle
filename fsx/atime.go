package fsx

import (
	"io/fs"
	"time"
)

// Atime returns the last access time of fi, if available.
//
// The access time is not part of fs.FileInfo, so it's read from its Sys value
// when the underlying system provides it.
func Atime(fi fs.FileInfo) (time.Time, bool) {
	if fi == nil {
		return time.Time{}, false
	}
	if mf, ok := fi.Sys().(*MemFile); ok {
		if mf.atime.IsZero() {
			return mf.ModTime, true
		}
		return mf.atime, true
	}
	return sysAtime(fi.Sys())
}
