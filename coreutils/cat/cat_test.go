package cat

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

type spyReader struct {
	read bool
}

func (r *spyReader) Read([]byte) (int, error) {
	r.read = true
	return 0, io.EOF
}

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

func (readerFS) Chown(string, int, int) error {
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

// testFS holds the fixtures used by the tests:
//
//	a.txt:    "alpha\n"
//	b.txt:    "beta\n"
//	empty.txt: ""
//	bin.txt:  arbitrary bytes
//	"-u":     "dash-u\n" (a file named like an option)
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":         {Data: []byte("alpha\n")},
		"b.txt":         {Data: []byte("beta\n")},
		"empty.txt":     {Data: []byte("")},
		"bin.txt":       {Data: []byte{0x00, 0x01, 0xff, '\n'}},
		"-u":            {Data: []byte("dash-u\n")},
		"sub/inner.txt": {Data: []byte("inner\n")},
	}
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
	fsys     fsx.FS
	dir      string
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
		Stdin:      test.stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         test.fsys,
		WorkingDir: test.dir,
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

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			name:  "no operands reads the standard input",
			stdin: strings.NewReader("hello\n"),
			want:  "hello\n",
		},
		{
			name:  "empty standard input",
			stdin: strings.NewReader(""),
			want:  "",
		},
		{
			name:  "standard input without a trailing newline",
			stdin: strings.NewReader("no trailing newline"),
			want:  "no trailing newline",
		},
		{
			name: "single file",
			fsys: testFS(),
			args: []string{"a.txt"},
			want: "alpha\n",
		},
		{
			name: "multiple files are concatenated in order",
			fsys: testFS(),
			args: []string{"a.txt", "b.txt"},
			want: "alpha\nbeta\n",
		},
		{
			name: "the same file twice",
			fsys: testFS(),
			args: []string{"a.txt", "a.txt"},
			want: "alpha\nalpha\n",
		},
		{
			name: "empty file",
			fsys: testFS(),
			args: []string{"empty.txt"},
			want: "",
		},
		{
			name: "arbitrary bytes are written untouched",
			fsys: testFS(),
			args: []string{"bin.txt"},
			want: string([]byte{0x00, 0x01, 0xff, '\n'}),
		},
		{
			name: "pathnames are resolved from the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt"},
			want: "inner\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunStandardInputOperand(t *testing.T) {
	tests := []testCase{
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("stdin\n"),
			args:  []string{"-"},
			want:  "stdin\n",
		},
		{
			name:  "dash combined with a file",
			stdin: strings.NewReader("stdin\n"),
			fsys:  testFS(),
			args:  []string{"-", "a.txt"},
			want:  "stdin\nalpha\n",
		},
		{
			name:  "dash between files reads the standard input at that point",
			stdin: strings.NewReader("stdin\n"),
			fsys:  testFS(),
			args:  []string{"a.txt", "-", "b.txt"},
			want:  "alpha\nstdin\nbeta\n",
		},
		{
			name:  "repeated standard input operands",
			stdin: strings.NewReader("stdin\n"),
			args:  []string{"-", "-"},
			want:  "stdin\n",
		},
		{
			name:  "double dash with no operands reads the standard input",
			stdin: strings.NewReader("stdin\n"),
			args:  []string{"--"},
			want:  "stdin\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOptions(t *testing.T) {
	tests := []testCase{
		{
			name: "u is accepted",
			fsys: testFS(),
			args: []string{"-u", "a.txt"},
			want: "alpha\n",
		},
		{
			name:  "u with the standard input",
			stdin: strings.NewReader("stdin\n"),
			args:  []string{"-u"},
			want:  "stdin\n",
		},
		{
			name: "repeated u",
			fsys: testFS(),
			args: []string{"-u", "-u", "a.txt"},
			want: "alpha\n",
		},
		{
			name: "grouped u",
			fsys: testFS(),
			args: []string{"-uu", "a.txt"},
			want: "alpha\n",
		},
		{
			name: "a lone double dash is accepted",
			fsys: testFS(),
			args: []string{"--", "a.txt"},
			want: "alpha\n",
		},
		{
			name: "a file named like an option after a double dash",
			fsys: testFS(),
			args: []string{"--", "-u"},
			want: "dash-u\n",
		},
		{
			name: "options are not parsed after the first operand",
			fsys: testFS(),
			args: []string{"a.txt", "-u"},
			want: "alpha\ndash-u\n",
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
			name:     "missing file",
			fsys:     testFS(),
			args:     []string{"missing.txt"},
			wantErr:  "cat: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing file doesn't stop the other files",
			fsys:     testFS(),
			args:     []string{"missing.txt", "a.txt"},
			want:     "alpha\n",
			wantErr:  "cat: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "every missing file reports an error",
			fsys:     testFS(),
			args:     []string{"missing.txt", "other.txt"},
			wantErr:  "cat: missing.txt: file does not exist\ncat: other.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "directory operand",
			fsys:     fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			args:     []string{"dir"},
			wantErr:  "cat: dir: invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "invalid option",
			args:     []string{"-x"},
			wantErr:  "cat: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "invalid option in a group",
			args:     []string{"-ux"},
			wantErr:  "cat: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "unknown long option",
			args:     []string{"--number"},
			wantErr:  "cat: unrecognized option '--number'\n",
			wantExit: 2,
		},
		{
			name:     "invalid option before operands",
			args:     []string{"-x", "a.txt"},
			wantErr:  "cat: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "zero-length file name",
			fsys:     testFS(),
			args:     []string{""},
			wantErr:  "cat: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "file operand without a file system",
			args:     []string{"a.txt"},
			wantErr:  "cat: a.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			fsys:     testFS(),
			wantErr:  "cat: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "dash without a standard input",
			fsys:     testFS(),
			args:     []string{"-"},
			wantErr:  "cat: -: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "standard input read error",
			stdin:    errorReader{err: errors.New("read failed")},
			fsys:     testFS(),
			wantErr:  "cat: standard input: read failed\n",
			wantExit: 1,
		},
		{
			name:     "dash read error is labeled with the operand",
			stdin:    errorReader{err: errors.New("read failed")},
			args:     []string{"-"},
			wantErr:  "cat: -: read failed\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDoesNotReadStdinWithFiles(t *testing.T) {
	stdin := &spyReader{}

	runCase(t, testCase{
		name:  "file operands don't read the standard input",
		stdin: stdin,
		fsys:  testFS(),
		args:  []string{"a.txt"},
		want:  "alpha\n",
	})
	if stdin.read {
		t.Error("Run() read stdin, want it untouched")
	}
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("hello\n"),
		Stdout: errorWriter{err: writeErr},
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context())

	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
	if got := stderr.String(); got != "cat: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "cat: write error: write failed\n")
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		runCancelled(t, ctx, strings.NewReader("hello\n"), nil)
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runCancelled(t, ctx, &cancelReader{cancel: cancel}, nil)
	})

	t.Run("cancelled while reading a file", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runCancelled(t, ctx, strings.NewReader(""), readerFS{&cancelReader{cancel: cancel}})
	})
}

// runCancelled runs the utility with a context that is either already done or
// becomes done while reading, and checks that the context error is returned
// without any output.
func runCancelled(t *testing.T, ctx context.Context, stdin io.Reader, fsys fsx.FS) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsys,
	}
	var args []string
	if fsys != nil {
		args = []string{"file"}
	}
	err := New(rt).Run(ctx, args...)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
}
