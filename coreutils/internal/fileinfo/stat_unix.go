//go:build unix && !aix && !hurd

package fileinfo

import (
	"io/fs"
	"syscall"
)

// statOf returns the information a file system reports about a file beyond
// what io/fs models.
func statOf(fi fs.FileInfo) (Info, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return Info{}, false
	}
	return Info{
		Ino:    uint64(st.Ino),
		Dev:    uint64(st.Dev),
		Nlink:  uint64(st.Nlink),
		Blocks: uint64(st.Blocks),
		Uid:    uint64(st.Uid),
		Gid:    uint64(st.Gid),
	}, true
}
