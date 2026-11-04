// Package fileinfo reports the information a file system may hold about a
// file beyond what io/fs models, read from the Sys value of its FileInfo:
// identifiers, link counts, ownership and the time of the last status
// change. A file system that doesn't report a value makes its functions
// report zero or false.
package fileinfo

import "io/fs"

// Info is the information a file system may report about a file beyond what
// io/fs models. It's zero when the file system reports none of it.
type Info struct {
	Ino    uint64
	Dev    uint64
	Nlink  uint64
	Blocks uint64 // 512-byte blocks
	Uid    uint64
	Gid    uint64
}

// Stat returns the information a file system reports about a file beyond
// what io/fs models.
func Stat(fi fs.FileInfo) (Info, bool) {
	return statOf(fi)
}
