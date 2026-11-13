package fsx

import (
	"io/fs"
	"os"
)

// RootFS returns an FS backed by root. Operations are confined to root as
// described by os.Root. The caller retains ownership of root and must close it.
func RootFS(root *os.Root) FS {
	return &rootFS{Root: root}
}

type rootFS struct {
	*os.Root
}

func (f *rootFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return f.Root.Open(name)
}

func (f *rootFS) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	return f.Root.OpenFile(name, flag, perm)
}

func (f *rootFS) ReadLink(name string) (string, error) {
	return f.Root.Readlink(name)
}

var (
	_ FS        = (*rootFS)(nil)
	_ ChownFS   = (*rootFS)(nil)
	_ fs.StatFS = (*rootFS)(nil)
)
