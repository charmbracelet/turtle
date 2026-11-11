// Package ctxio provides I/O helpers that stop operations once a context is
// done.
package ctxio

import (
	"context"
	"io"
	"io/fs"
)

// ReadCloser stops read operations with the context error once Ctx is done,
// so long inputs don't delay returning.
type ReadCloser struct {
	Ctx context.Context
	RC  io.ReadCloser
}

// NewReader returns a ReadCloser for a reader the caller doesn't own, so
// closing it does nothing.
func NewReader(ctx context.Context, r io.Reader) ReadCloser {
	return ReadCloser{Ctx: ctx, RC: io.NopCloser(r)}
}

// Read implements the io.Reader interface.
func (c ReadCloser) Read(p []byte) (int, error) {
	if err := c.Ctx.Err(); err != nil {
		return 0, err
	}
	return c.RC.Read(p)
}

// Close implements the io.Closer interface.
func (c ReadCloser) Close() error {
	return c.RC.Close()
}

// WriteCloser stops write operations with the context error once Ctx is
// done, so long outputs don't delay returning.
type WriteCloser struct {
	Ctx context.Context
	WC  io.WriteCloser
}

// Write implements the io.Writer interface.
func (c WriteCloser) Write(p []byte) (int, error) {
	if err := c.Ctx.Err(); err != nil {
		return 0, err
	}
	return c.WC.Write(p)
}

// Close implements the io.Closer interface.
func (c WriteCloser) Close() error {
	return c.WC.Close()
}

// Stat returns the status of the wrapped file, when it is one, so callers
// can keep telling what they write to through the wrapper.
func (c WriteCloser) Stat() (fs.FileInfo, error) {
	s, ok := c.WC.(interface{ Stat() (fs.FileInfo, error) })
	if !ok {
		return nil, fs.ErrInvalid
	}
	return s.Stat()
}
