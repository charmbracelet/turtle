package ls

// statInfo is the information a file system may report about a file beyond
// what io/fs models. It's zero when the file system reports none of it.
type statInfo struct {
	ino    uint64
	dev    uint64
	nlink  uint64
	blocks uint64 // 512-byte blocks
	uid    uint64
	gid    uint64
}
