//go:build unix && !aix && !hurd

package ls

import (
	"io/fs"
	"syscall"
)

// statOf returns the information a file system reports about a file beyond
// what io/fs models.
func statOf(fi fs.FileInfo) (statInfo, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return statInfo{}, false
	}
	return statInfo{
		ino:    uint64(st.Ino),
		dev:    uint64(st.Dev),
		nlink:  uint64(st.Nlink),
		blocks: uint64(st.Blocks),
		uid:    uint64(st.Uid),
		gid:    uint64(st.Gid),
	}, true
}
