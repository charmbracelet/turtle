package testutil

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/fsx"
)

// fakeNew returns a program that copies its standard input to its standard
// output, stopping with the context error when the context is done.
func fakeNew(rt *coreutils.Runtime) coreutils.Program {
	return fakeProg{rt}
}

type fakeProg struct {
	rt *coreutils.Runtime
}

func (p fakeProg) Run(ctx context.Context, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "--fail" {
		return &coreutils.ExitError{Code: 1, Err: errors.New("nope")}
	}
	_, err := io.Copy(p.rt.Stdout, p.rt.Stdin)
	return err
}

func TestRun(t *testing.T) {
	tests := []Case{
		{
			Name:  "the standard input is copied",
			Stdin: strings.NewReader("hello\n"),
			Want:  "hello\n",
		},
		{
			Name:     "an error exit",
			Stdin:    strings.NewReader("x"),
			Args:     []string{"--fail"},
			WantExit: 1,
		},
	}

	Run(t, fakeNew, tests)
}

func TestOnce(t *testing.T) {
	mod := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	result := Once(t, fakeNew, Case{
		Name:  "result and file checks",
		Stdin: strings.NewReader("hi\n"),
		FS: fsx.MemFS{
			"in.txt": {Data: []byte("data"), Mode: 0o644, ModTime: mod},
		},
		Want:      "hi\n",
		WantFiles: map[string]string{"in.txt": "data"},
		WantMode:  map[string]fs.FileMode{"in.txt": 0o644},
		WantTime:  map[string]time.Time{"in.txt": mod},
		Present:   []string{"in.txt"},
		Absent:    []string{"missing.txt"},
	})

	if result.Exit != 0 || result.Err != nil {
		t.Errorf("Once() = %+v, want exit 0 and no error", result)
	}
	if result.Stdout != "hi\n" {
		t.Errorf("Once() stdout = %q, want %q", result.Stdout, "hi\n")
	}
}

func TestExitCode(t *testing.T) {
	if code := ExitCode(t, nil); code != 0 {
		t.Errorf("ExitCode(nil) = %d, want 0", code)
	}
	if code := ExitCode(t, &coreutils.ExitError{Code: 2}); code != 2 {
		t.Errorf("ExitCode() = %d, want 2", code)
	}
}

func TestCancelled(t *testing.T) {
	t.Run("before running", func(t *testing.T) {
		Cancelled(t, fakeNew, nil, nil)
	})

	t.Run("while reading", func(t *testing.T) {
		Cancelled(t, fakeNew, &CancelReader{}, nil)
	})
}

func TestErrorReader(t *testing.T) {
	err := errors.New("read failed")
	if _, got := (ErrorReader{Err: err}).Read(nil); got != err {
		t.Errorf("Read() error = %v, want %v", got, err)
	}
	if _, got := (ErrorReader{}).Read(nil); got.Error() != "boom" {
		t.Errorf("Read() error = %v, want boom", got)
	}
}

func TestErrorWriter(t *testing.T) {
	err := errors.New("write failed")
	if _, got := (ErrorWriter{Err: err}).Write(nil); got != err {
		t.Errorf("Write() error = %v, want %v", got, err)
	}
	if _, got := (ErrorWriter{}).Write(nil); got.Error() != "boom" {
		t.Errorf("Write() error = %v, want boom", got)
	}
}

func TestSpyReader(t *testing.T) {
	r := &SpyReader{}
	for range 2 {
		if _, err := r.Read(nil); err != io.EOF {
			t.Errorf("Read() error = %v, want io.EOF", err)
		}
	}
	if r.Reads != 2 {
		t.Errorf("Reads = %d, want 2", r.Reads)
	}
}

func TestCancelReader(t *testing.T) {
	t.Run("without data", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		r := &CancelReader{Cancel: cancel}
		if _, err := r.Read(nil); err != context.Canceled {
			t.Errorf("Read() error = %v, want context.Canceled", err)
		}
		if ctx.Err() == nil {
			t.Error("context was not cancelled")
		}
	})

	t.Run("with data", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		r := &CancelReader{Cancel: cancel, Data: []byte("hi")}
		p := make([]byte, 8)
		n, err := r.Read(p)
		if n != 2 || string(p[:n]) != "hi" || err != nil {
			t.Errorf("Read() = %d, %q, %v, want 2, %q, nil", n, p[:n], err, "hi")
		}
		if ctx.Err() == nil {
			t.Error("context was not cancelled")
		}
		if _, err := r.Read(p); err != io.EOF {
			t.Errorf("Read() error = %v, want io.EOF", err)
		}
	})
}

func TestEnviron(t *testing.T) {
	got := Environ(map[string]string{"HOME": "/home/u", "PATH": "/bin"})
	sort.Strings(got)
	want := []string{"HOME=/home/u", "PATH=/bin"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Environ() = %q, want %q", got, want)
	}
}

func TestReaderFS(t *testing.T) {
	fsys := ReaderFS(strings.NewReader("contents"))
	got, err := fs.ReadFile(fsys, "whatever.txt")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "contents" {
		t.Errorf("ReadFile() = %q, want %q", got, "contents")
	}
}

func TestFailWriteFS(t *testing.T) {
	err := errors.New("disk full")
	fsys := FailWriteFS{FS: fsx.MemFS{}, Err: err}

	f, openErr := fsys.OpenFile("f", os.O_CREATE|os.O_WRONLY, 0o644)
	if openErr != nil {
		t.Fatalf("OpenFile() error = %v", openErr)
	}
	if _, writeErr := f.Write([]byte("data")); writeErr != err {
		t.Errorf("Write() error = %v, want %v", writeErr, err)
	}

	in, openErr := fsys.OpenFile("f", os.O_RDONLY, 0o644)
	if openErr != nil {
		t.Fatalf("OpenFile() error = %v", openErr)
	}
	if closeErr := in.Close(); closeErr != nil {
		t.Errorf("Close() error = %v", closeErr)
	}
}
