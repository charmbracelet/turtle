package ctxio

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type closer struct {
	r      io.Reader
	closed bool
}

func (c *closer) Read(p []byte) (int, error) { return c.r.Read(p) }

func (c *closer) Close() error {
	c.closed = true
	return nil
}

func TestRead(t *testing.T) {
	tests := []struct {
		name  string
		ctx   context.Context
		read  int
		want  string
		errIs error
	}{
		{
			name: "reads while the context is alive",
			ctx:  t.Context(),
			read: 5,
			want: "hello",
		},
		{
			name:  "stops with the context error once done",
			ctx:   func() context.Context { ctx, cancel := context.WithCancel(t.Context()); cancel(); return ctx }(),
			read:  5,
			errIs: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ReadCloser{Ctx: tt.ctx, RC: &closer{r: strings.NewReader("hello")}}
			p := make([]byte, tt.read)
			n, err := c.Read(p)
			if !errors.Is(err, tt.errIs) {
				t.Fatalf("Read error = %v, want %v", err, tt.errIs)
			}
			if got := string(p[:n]); got != tt.want {
				t.Errorf("Read = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClose(t *testing.T) {
	rc := &closer{r: strings.NewReader("hello")}
	c := ReadCloser{Ctx: t.Context(), RC: rc}
	if err := c.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if !rc.closed {
		t.Error("Close didn't close the wrapped reader")
	}
}

func TestNewReader(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	c := NewReader(ctx, strings.NewReader("hello"))
	cancel()
	if _, err := c.Read(make([]byte, 5)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read error = %v, want %v", err, context.Canceled)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
}
