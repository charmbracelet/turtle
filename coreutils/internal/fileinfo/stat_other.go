//go:build !unix || aix || hurd

package fileinfo

import "io/fs"

// statOf returns the information a file system reports about a file beyond
// what io/fs models, which the systems here report none of.
func statOf(fs.FileInfo) (Info, bool) {
	return Info{}, false
}
