// Package fsx is a superset of the `io/fs` package from the standard library.
// The one from the stdlib is read-only, while this one target adding write-like
// functionality like writing to files, chmod, etc.
package fsx

import (
	"errors"
	"io"
	"io/fs"
	"time"
)

// File is a fs.File that also allows writing to it.
type File interface {
	fs.File
	io.Writer
}

// SeekFile is a file whose read/write offset can be changed, like os.File.
//
// It isn't embedded into File nor into FS because not every file is
// seekable, like pipes: callers must check for it, with a type assertion,
// and fall back to reading sequentially otherwise.
type SeekFile interface {
	fs.File
	io.Seeker
}

// WriteFS can open files for writing and create new ones.
type WriteFS interface {
	// OpenFile opens the named file, like os.OpenFile. Implementations only
	// need to honor O_CREATE, O_EXCL, O_TRUNC, O_APPEND and the
	// O_RDONLY/O_WRONLY/O_RDWR mode bits.
	OpenFile(name string, flag int, perm fs.FileMode) (File, error)
}

// ChtimesFS can change the access and modification times of a file.
type ChtimesFS interface {
	Chtimes(name string, atime, mtime time.Time) error
}

// ChmodFS can change the file mode bits of a file.
type ChmodFS interface {
	// Chmod changes the mode bits of the named file, like os.Chmod. Only the
	// ModePerm, ModeSetuid, ModeSetgid and ModeSticky bits are considered;
	// the file type bits are kept.
	Chmod(name string, mode fs.FileMode) error
}

// MkdirFS can create directories.
type MkdirFS interface {
	// Mkdir creates a new directory with the given permissions, like
	// os.Mkdir. In the real file system, perm is subject to the process file
	// mode creation mask.
	Mkdir(name string, perm fs.FileMode) error
}

// RemoveFS can remove files and empty directories.
type RemoveFS interface {
	// Remove removes the named file or empty directory, like os.Remove.
	Remove(name string) error
}

// ErrNotEmpty is returned when removing a directory that still holds
// entries.
var ErrNotEmpty = errors.New("directory not empty")

// RenameFS can move files between names.
type RenameFS interface {
	// Rename renames (moves) oldPath to newPath, like os.Rename. An existing
	// newPath is replaced: a directory only by an empty directory, or by a
	// file, and a file by a file.
	Rename(oldPath, newPath string) error
}

// LinkFS can create hard links.
type LinkFS interface {
	// Link creates newName as a hard link to the oldName file, like os.Link.
	// Whether a symbolic link oldName is followed depends on the platform, as
	// with the link(2) function.
	Link(oldName, newName string) error
}

// SymlinkFS can create and inspect symbolic links.
type SymlinkFS interface {
	// Symlink creates newName as a symbolic link to oldName, like os.Symlink.
	// oldName is stored as given, without being resolved, and need not name
	// an existing file.
	Symlink(oldName, newName string) error

	fs.ReadLinkFS
}

// FS is the main interface of this package, which embeds `io/fs.FS` from the
// stdlib and all the extra interfaces that we added to this package.
type FS interface {
	fs.FS
	WriteFS
	ChtimesFS
	ChmodFS
	MkdirFS
	RemoveFS
	RenameFS
	LinkFS
	SymlinkFS
}
