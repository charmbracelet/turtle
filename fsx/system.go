package fsx

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// SystemFS returns an FS backed by the operating system's file system.
//
// Names are resolved inside dir, which works like a working directory, except
// that absolute paths are used as given. An empty dir means the current
// working directory of the process.
func SystemFS(dir string) FS {
	return &systemFS{dir: dir}
}

type systemFS struct {
	dir string
}

// Open implements fs.FS.
func (f *systemFS) Open(name string) (fs.File, error) {
	return os.Open(f.resolve(name))
}

// OpenFile implements WriteFS.
func (f *systemFS) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	return os.OpenFile(f.resolve(name), flag, perm)
}

// Chtimes implements ChtimesFS.
func (f *systemFS) Chtimes(name string, atime, mtime time.Time) error {
	return os.Chtimes(f.resolve(name), atime, mtime)
}

// Chmod implements ChmodFS.
func (f *systemFS) Chmod(name string, mode fs.FileMode) error {
	return os.Chmod(f.resolve(name), mode)
}

// Chown implements ChownFS.
func (f *systemFS) Chown(name string, uid, gid int) error {
	return os.Chown(f.resolve(name), uid, gid)
}

// Mkdir implements MkdirFS.
func (f *systemFS) Mkdir(name string, perm fs.FileMode) error {
	return os.Mkdir(f.resolve(name), perm)
}

// Remove implements RemoveFS.
func (f *systemFS) Remove(name string) error {
	return os.Remove(f.resolve(name))
}

// Rename implements RenameFS.
func (f *systemFS) Rename(oldName, newName string) error {
	return os.Rename(f.resolve(oldName), f.resolve(newName))
}

// Link implements LinkFS.
func (f *systemFS) Link(oldName, newName string) error {
	return os.Link(f.resolve(oldName), f.resolve(newName))
}

// Symlink implements SymlinkFS. The target is stored as given, so a relative
// one keeps pointing relative to the link.
func (f *systemFS) Symlink(oldName, newName string) error {
	return os.Symlink(oldName, f.resolve(newName))
}

// ReadLink implements fs.ReadLinkFS.
func (f *systemFS) ReadLink(name string) (string, error) {
	return os.Readlink(f.resolve(name))
}

// Lstat implements fs.ReadLinkFS. It doesn't follow a final symbolic link,
// unlike Stat.
func (f *systemFS) Lstat(name string) (fs.FileInfo, error) {
	return os.Lstat(f.resolve(name))
}

// Sync implements SyncFS. It's a no-op on systems without a global sync,
// like Windows.
func (f *systemFS) Sync() error {
	return osSync()
}

// resolve converts name to an OS path and joins it to the base directory.
func (f *systemFS) resolve(name string) string {
	name = filepath.FromSlash(name)
	if f.dir == "" || filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(f.dir, name)
}

var (
	_ FS       = (*systemFS)(nil)
	_ SyncFS   = (*systemFS)(nil)
	_ SeekFile = (*os.File)(nil)
)
