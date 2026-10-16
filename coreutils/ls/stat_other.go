//go:build !unix || aix || hurd

package ls

import "io/fs"

// statOf returns the information a file system reports about a file beyond
// what io/fs models, which the systems here report none of.
func statOf(fs.FileInfo) (statInfo, bool) {
	return statInfo{}, false
}
