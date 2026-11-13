package stat

import (
	"io/fs"
	"time"

	"charm.land/turtle/coreutils/internal/fileinfo"
	"charm.land/turtle/coreutils/internal/filemode"
	"charm.land/turtle/fsx"
)

// fileStat holds the information a file is written with, gathered from what
// the file system reports. Fields it doesn't report are left unknown, which
// the directives degrade from: a device number or a file serial number of
// zero, a link count of one, and the modification time standing in for the
// times that aren't reported.
type fileStat struct {
	name    string // the operand as it was given
	fi      fs.FileInfo
	target  string // the target of an unfollowed symbolic link
	linkErr error  // why the target of a symbolic link couldn't be read
	isLink  bool   // whether the file itself is a link that wasn't followed
	follows bool   // whether a link operand was dereferenced

	uid, gid int
	hasOwner bool

	dev     uint64
	ino     uint64
	nlink   uint64
	blocks  uint64
	rdev    uint64
	blksize int64
	hasSys  bool // whether the file system reports the fields above

	atime, mtime, ctime time.Time
	btime               time.Time
	hasBtime            bool
}

// newFileStat gathers the information of the file at path, named by an
// operand, from what the file system reports.
func (p *Prog) newFileStat(name, path string, fi fs.FileInfo, opts options) fileStat {
	st := fileStat{
		name:    name,
		fi:      fi,
		isLink:  fi.Mode()&fs.ModeSymlink != 0,
		follows: opts.deref,
		mtime:   fi.ModTime(),
	}
	st.atime, _ = fsx.Atime(fi)
	st.ctime, _ = fileinfo.Ctime(fi)
	st.btime, st.hasBtime = fileinfo.Btime(fi)
	if st.atime.IsZero() && !st.mtime.IsZero() {
		// A file system that doesn't report the access time is written
		// with its modification time, the way ls writes it.
		st.atime = st.mtime
	}
	if st.ctime.IsZero() {
		// A file system that doesn't report the status change time is
		// written with its modification time, the way ls writes it.
		st.ctime = st.mtime
	}
	if info, ok := fileinfo.Stat(fi); ok {
		st.dev, st.ino, st.nlink = info.Dev, info.Ino, info.Nlink
		st.blocks, st.uid, st.gid = info.Blocks, int(info.Uid), int(info.Gid)
		st.hasSys = true
	}
	st.blksize, _ = sysBlksize(fi.Sys())
	st.rdev = sysRdev(fi.Sys())
	if !st.hasSys {
		// A file system that doesn't report blocks is measured by the size
		// of the file, rounded up to whole 512-byte blocks. A system that
		// reports them, even as zero for a sparse file, is written as it
		// reports them.
		size := max(fi.Size(), 0)
		st.blocks = uint64((size + 511) / 512)
	}
	if st.nlink == 0 {
		st.nlink = 1
	}
	if st.blksize <= 0 {
		st.blksize = 512
	}
	if uid, gid, ok := fsx.Owner(fi); ok {
		st.uid, st.gid, st.hasOwner = uid, gid, true
	}
	if st.isLink && !st.follows {
		st.target, st.linkErr = p.FS.ReadLink(path)
	}
	return st
}

// rawMode returns the mode of a file as the raw st_mode of the systems
// holds it: the type bits of the POSIX interface, followed by the special
// and permission bits.
func rawMode(m fs.FileMode) uint64 {
	v := uint64(filemode.Octal(m))
	switch {
	case m&fs.ModeDir != 0:
		v |= 0o040000
	case m&fs.ModeSymlink != 0:
		v |= 0o120000
	case m&fs.ModeDevice != 0 && m&fs.ModeCharDevice != 0:
		v |= 0o020000
	case m&fs.ModeDevice != 0:
		v |= 0o060000
	case m&fs.ModeNamedPipe != 0:
		v |= 0o010000
	case m&fs.ModeSocket != 0:
		v |= 0o140000
	default:
		v |= 0o100000
	}
	return v
}

// devicePart takes the whole device number of a conversion, or the major
// or minor one the H and L modifiers ask for.
func devicePart(dev uint64, mod byte) uint64 {
	switch mod {
	case 'H':
		return major(dev)
	case 'L':
		return minor(dev)
	}
	return dev
}

// fileType returns the words GNU writes for the type of a file.
func fileType(m fs.FileMode) string {
	switch {
	case m.IsDir():
		return "directory"
	case m&fs.ModeSymlink != 0:
		return "symbolic link"
	case m&fs.ModeDevice != 0 && m&fs.ModeCharDevice != 0:
		return "character special file"
	case m&fs.ModeDevice != 0:
		return "block special file"
	case m&fs.ModeNamedPipe != 0:
		return "fifo"
	case m&fs.ModeSocket != 0:
		return "socket"
	case m.IsRegular():
		return "regular file"
	default:
		return "unknown"
	}
}

// isDevice reports whether a file is a character or block special file.
func isDevice(m fs.FileMode) bool {
	return m&fs.ModeDevice != 0
}

// major splits a device number the way the POSIX systems do, where the high
// bits name the device and the low ones the unit within it.
func major(dev uint64) uint64 {
	return (dev>>8)&0xfff | dev>>32
}

func minor(dev uint64) uint64 {
	return dev&0xff | (dev>>12)&^0xff
}
