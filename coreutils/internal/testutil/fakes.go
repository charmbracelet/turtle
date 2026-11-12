package testutil

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"time"

	"charm.land/snail/fsx"
)

// boom is the error the fakes fail with when no other one is given.
var boom = errors.New("boom")

// ErrorReader fails every read with Err, or with a "boom" error when it's nil.
type ErrorReader struct {
	Err error
}

// Read implements io.Reader.
func (r ErrorReader) Read([]byte) (int, error) {
	if r.Err == nil {
		return 0, boom
	}
	return 0, r.Err
}

// ErrorWriter fails every write with Err, or with a "boom" error when it's nil.
type ErrorWriter struct {
	Err error
}

// Write implements io.Writer.
func (w ErrorWriter) Write([]byte) (int, error) {
	if w.Err == nil {
		return 0, boom
	}
	return 0, w.Err
}

// SpyReader counts the reads done on it, ending each one with io.EOF.
type SpyReader struct {
	Reads int
}

// Read implements io.Reader.
func (r *SpyReader) Read([]byte) (int, error) {
	r.Reads++
	return 0, io.EOF
}

// CancelReader cancels its context on the first read, simulating a context
// that is done while input is being read.
//
// When Data is set, the first read returns it after cancelling and the
// following ones end with io.EOF. Otherwise every read returns the context
// error.
type CancelReader struct {
	Cancel context.CancelFunc
	Data   []byte

	reads int
}

// Read implements io.Reader.
func (r *CancelReader) Read(p []byte) (int, error) {
	if len(r.Data) == 0 {
		r.Cancel()
		return 0, context.Canceled
	}
	if r.reads > 0 {
		return 0, io.EOF
	}
	r.reads++
	r.Cancel()
	return copy(p, r.Data), nil
}

// CancelWriter cancels its context on the first write, simulating a context
// that is done while output is being written. The writes still succeed.
type CancelWriter struct {
	Cancel context.CancelFunc
}

// Write implements io.Writer.
func (w *CancelWriter) Write(b []byte) (int, error) {
	w.Cancel()
	return len(b), nil
}

// Environ builds a "KEY=VALUE" environment from a map.
func Environ(values map[string]string) []string {
	entries := make([]string, 0, len(values))
	for name, value := range values {
		entries = append(entries, name+"="+value)
	}
	return entries
}

// ReaderFS returns a file system whose only file reads from r, whatever its
// name, so an input can be made non-seekable, slow or failing.
func ReaderFS(r io.Reader) fsx.FS {
	return readerFS{r}
}

// readerFS is an fs.FS whose only file reads from a reader.
type readerFS struct {
	r io.Reader
}

func (f readerFS) Open(string) (fs.File, error) {
	return readerFile{f.r}, nil
}

func (readerFS) OpenFile(string, int, fs.FileMode) (fsx.File, error) {
	return nil, fs.ErrInvalid
}

func (readerFS) Chtimes(string, time.Time, time.Time) error {
	return fs.ErrInvalid
}

func (readerFS) Chmod(string, fs.FileMode) error {
	return fs.ErrInvalid
}

func (readerFS) Mkdir(string, fs.FileMode) error {
	return fs.ErrInvalid
}

func (readerFS) Remove(string) error {
	return fs.ErrInvalid
}

func (readerFS) Rename(string, string) error {
	return fs.ErrInvalid
}

func (readerFS) Link(string, string) error {
	return fs.ErrInvalid
}

func (readerFS) Symlink(string, string) error {
	return fs.ErrInvalid
}

func (readerFS) ReadLink(string) (string, error) {
	return "", fs.ErrInvalid
}

func (readerFS) Lstat(string) (fs.FileInfo, error) {
	return nil, fs.ErrInvalid
}

type readerFile struct {
	io.Reader
}

func (readerFile) Close() error               { return nil }
func (readerFile) Stat() (fs.FileInfo, error) { return nil, fs.ErrInvalid }

// FailWriteFS wraps a file system so that writes to the files it opens fail
// with Err, or with a "boom" error when it's nil. Reads still work.
type FailWriteFS struct {
	fsx.FS
	Err error
}

// OpenFile implements fsx.WriteFS.
func (m FailWriteFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	f, err := m.FS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return FailWriteFile{File: f, Err: m.Err}, nil
}

// FailWriteFile is the file FailWriteFS opens, failing every write with Err.
type FailWriteFile struct {
	fsx.File
	Err error
}

// Write implements io.Writer.
func (f FailWriteFile) Write([]byte) (int, error) {
	if f.Err == nil {
		return 0, boom
	}
	return 0, f.Err
}
