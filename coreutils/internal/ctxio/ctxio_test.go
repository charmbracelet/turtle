package ctxio

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"

	"charm.land/turtle/fsx"
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

type writeCloser struct {
	buf    strings.Builder
	closed bool
}

func (w *writeCloser) Write(p []byte) (int, error) { return w.buf.Write(p) }

func (w *writeCloser) Close() error {
	w.closed = true
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

func TestWrite(t *testing.T) {
	tests := []struct {
		name  string
		ctx   context.Context
		want  string
		errIs error
	}{
		{
			name: "writes while the context is alive",
			ctx:  t.Context(),
			want: "hello",
		},
		{
			name:  "stops with the context error once done",
			ctx:   func() context.Context { ctx, cancel := context.WithCancel(t.Context()); cancel(); return ctx }(),
			errIs: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wc := &writeCloser{}
			c := WriteCloser{Ctx: tt.ctx, WC: wc}
			_, err := c.Write([]byte("hello"))
			if !errors.Is(err, tt.errIs) {
				t.Fatalf("Write error = %v, want %v", err, tt.errIs)
			}
			if got := wc.buf.String(); got != tt.want {
				t.Errorf("Write wrote %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteClose(t *testing.T) {
	wc := &writeCloser{}
	c := WriteCloser{Ctx: t.Context(), WC: wc}
	if err := c.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if !wc.closed {
		t.Error("Close didn't close the wrapped writer")
	}
}

func TestStat(t *testing.T) {
	fsys := fsx.MemFS{"file.txt": {Data: []byte("hello")}}
	f, err := fsys.OpenFile("file.txt", os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile error = %v", err)
	}
	c := WriteCloser{Ctx: t.Context(), WC: f.(io.WriteCloser)}
	fi, err := c.Stat()
	if err != nil {
		t.Fatalf("Stat error = %v", err)
	}
	if fi.Name() != "file.txt" {
		t.Errorf("Stat name = %q, want %q", fi.Name(), "file.txt")
	}

	c = WriteCloser{Ctx: t.Context(), WC: &writeCloser{}}
	if _, err := c.Stat(); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("Stat error = %v, want %v", err, fs.ErrInvalid)
	}
}
