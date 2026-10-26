package cksum

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

// cancelReader cancels its context on the first read, simulating a context
// that is done while input is being read.
type cancelReader struct {
	cancel context.CancelFunc
}

func (r *cancelReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
}

func exitCode(t *testing.T, err error) int {
	t.Helper()

	if err == nil {
		return 0
	}
	var exitErr *coreutils.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
	}
	return exitErr.Code
}

type testCase struct {
	name     string
	stdin    io.Reader
	fsys     fsx.MemFS
	args     []string
	want     string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  test.stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
	if test.fsys != nil {
		rt.FS = test.fsys
	}
	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.want {
		t.Errorf("Run() stdout = %q, want %q", got, test.want)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
}

// files holds the fixtures used by the tests, with the checksums the
// algorithm of the specification gives for them.
var files = fsx.MemFS{
	"abc.txt":   {Data: []byte("abc")},
	"empty.txt": {Data: []byte("")},
	"hello.txt": {Data: []byte("hello")},
	"sub/1.txt": {Data: []byte("1\n")},
}

func TestRun(t *testing.T) {
	tests := []testCase{
		{
			name:  "the standard input without a name",
			stdin: strings.NewReader("abc"),
			want:  "1219131554 3\n",
		},
		{
			name:  "an empty input",
			stdin: strings.NewReader(""),
			want:  "4294967295 0\n",
		},
		{
			name:  "a binary input",
			stdin: strings.NewReader("\x00\xff\x80abc"),
			want:  "1630418514 6\n",
		},
		{
			name:  "an input across chunks",
			stdin: strings.NewReader(strings.Repeat("a", 40000)),
			want:  "330876385 40000\n",
		},
		{
			name:  "an explicit standard input keeps its name",
			stdin: strings.NewReader("abc"),
			args:  []string{"-"},
			want:  "1219131554 3 -\n",
		},
		{
			name: "a file",
			fsys: files,
			args: []string{"abc.txt"},
			want: "1219131554 3 abc.txt\n",
		},
		{
			name: "several files",
			fsys: files,
			args: []string{"abc.txt", "hello.txt"},
			want: "1219131554 3 abc.txt\n3287646509 5 hello.txt\n",
		},
		{
			name: "a file in a subdirectory",
			fsys: files,
			args: []string{"sub/1.txt"},
			want: "4219530715 2 sub/1.txt\n",
		},
		{
			name: "a file named like an option",
			fsys: files,
			args: []string{"--", "abc.txt"},
			want: "1219131554 3 abc.txt\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "an invalid option",
			fsys:     files,
			args:     []string{"-x", "abc.txt"},
			wantErr:  "cksum: invalid option -- 'x'\n",
			wantExit: 1,
		},
		{
			name:     "a missing file",
			fsys:     files,
			args:     []string{"missing.txt"},
			wantErr:  "cksum: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "the files after a missing one are still written",
			fsys:     files,
			args:     []string{"missing.txt", "abc.txt"},
			want:     "1219131554 3 abc.txt\n",
			wantErr:  "cksum: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "no file system",
			args:     []string{"abc.txt"},
			wantErr:  "cksum: abc.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "a read error",
			stdin:    errorReader{err: errors.New("boom")},
			args:     []string{"-"},
			wantErr:  "cksum: -: boom\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunIOErrors(t *testing.T) {
	t.Run("a write error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader("abc"),
			Stdout: errorWriter{err: errors.New("boom")},
			Stderr: new(bytes.Buffer),
		}
		err := New(rt).Run(t.Context())
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
	})

	t.Run("a done context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		rt := &coreutils.Runtime{
			Stdin:  &cancelReader{cancel: cancel},
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
		}
		err := New(rt).Run(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})
}

// failFS opens files whose reads always fail, recording when they are
// closed.
type failFS struct {
	fsx.MemFS
	closed bool
}

type failFile struct {
	fs *failFS
}

func (f *failFile) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}

func (f *failFile) Stat() (fs.FileInfo, error) {
	return nil, errors.New("no file information")
}

func (f *failFile) Close() error {
	f.fs.closed = true
	return nil
}

func (f *failFS) Open(string) (fs.File, error) {
	return &failFile{fs: f}, nil
}

func TestChecksumClosesOnError(t *testing.T) {
	t.Run("a read error", func(t *testing.T) {
		fsys := &failFS{}
		rt := &coreutils.Runtime{
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
			FS:     fsys,
		}
		p := &Prog{Runtime: rt}
		_, _, err := p.checksum(t.Context(), "file")
		if err == nil {
			t.Fatal("checksum() error = nil, want an error")
		}
		if !fsys.closed {
			t.Error("checksum() didn't close the file")
		}
	})

	t.Run("a done context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		fsys := &failFS{}
		rt := &coreutils.Runtime{
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
			FS:     fsys,
		}
		p := &Prog{Runtime: rt}
		if _, _, err := p.checksum(ctx, "file"); !errors.Is(err, context.Canceled) {
			t.Errorf("checksum() error = %v, want context.Canceled", err)
		}
		if !fsys.closed {
			t.Error("checksum() didn't close the file")
		}
	})
}

func TestCrc(t *testing.T) {
	tests := []struct {
		name string
		data string
		want uint32
	}{
		{name: "an empty input", data: "", want: 4294967295},
		{name: "abc", data: "abc", want: 1219131554},
		{name: "hello", data: "hello", want: 3287646509},
		{name: "the quick brown fox", data: "The quick brown fox jumps over the lazy dog", want: 2074844392},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var c crc
			c.write([]byte(test.data))
			c.writeSize(int64(len(test.data)))
			if got := ^c.value; got != test.want {
				t.Errorf("checksum of %q = %d, want %d", test.data, got, test.want)
			}
		})
	}
}
