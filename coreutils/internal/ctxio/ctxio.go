// Package ctxio provides I/O helpers that stop operations once a context is
// done.
package ctxio

import (
	"context"
	"io"
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
