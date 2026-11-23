package fsx

import "io/fs"

// Owner returns the numeric user and group IDs of the file described by fi.
//
// Ownership is not part of fs.FileInfo, so it's read from its Sys value when
// the underlying system reports it. On systems that don't model file
// ownership, ok is false.
func Owner(fi fs.FileInfo) (uid, gid int, ok bool) {
	if fi == nil {
		return 0, 0, false
	}
	if mf, isMem := fi.Sys().(*MemoryFile); isMem {
		return mf.Uid, mf.Gid, true
	}
	return sysOwner(fi.Sys())
}
