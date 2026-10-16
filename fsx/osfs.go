package fsx

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// OsFS returns an FS backed by the operating system's file system.
//
// Names are resolved inside dir, which works like a working directory, except
// that absolute paths are used as given. An empty dir means the current
// working directory of the process.
func OsFS(dir string) FS {
	return &osFS{dir: dir}
}

type osFS struct {
	dir string
}

// Open implements fs.FS.
func (f *osFS) Open(name string) (fs.File, error) {
	return os.Open(f.resolve(name))
}

// OpenFile implements WriteFS.
func (f *osFS) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	return os.OpenFile(f.resolve(name), flag, perm)
}

// Chtimes implements ChtimesFS.
func (f *osFS) Chtimes(name string, atime, mtime time.Time) error {
	return os.Chtimes(f.resolve(name), atime, mtime)
}

// Chmod implements ChmodFS.
func (f *osFS) Chmod(name string, mode fs.FileMode) error {
	return os.Chmod(f.resolve(name), mode)
}

// Mkdir implements MkdirFS.
func (f *osFS) Mkdir(name string, perm fs.FileMode) error {
	return os.Mkdir(f.resolve(name), perm)
}

// Remove implements RemoveFS.
func (f *osFS) Remove(name string) error {
	return os.Remove(f.resolve(name))
}

// Rename implements RenameFS.
func (f *osFS) Rename(oldName, newName string) error {
	return os.Rename(f.resolve(oldName), f.resolve(newName))
}

// Link implements LinkFS.
func (f *osFS) Link(oldName, newName string) error {
	return os.Link(f.resolve(oldName), f.resolve(newName))
}

// Symlink implements SymlinkFS. The target is stored as given, so a relative
// one keeps pointing relative to the link.
func (f *osFS) Symlink(oldName, newName string) error {
	return os.Symlink(oldName, f.resolve(newName))
}

// ReadLink implements fs.ReadLinkFS.
func (f *osFS) ReadLink(name string) (string, error) {
	return os.Readlink(f.resolve(name))
}

// Lstat implements fs.ReadLinkFS. It doesn't follow a final symbolic link,
// unlike Stat.
func (f *osFS) Lstat(name string) (fs.FileInfo, error) {
	return os.Lstat(f.resolve(name))
}

// resolve converts name to an OS path and joins it to the base directory.
func (f *osFS) resolve(name string) string {
	name = filepath.FromSlash(name)
	if f.dir == "" || filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(f.dir, name)
}

var (
	_ FS       = (*osFS)(nil)
	_ SeekFile = (*os.File)(nil)
)
