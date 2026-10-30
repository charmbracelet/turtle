package wc

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
	reads  int
}

func (r *cancelReader) Read(p []byte) (int, error) {
	if r.reads > 0 {
		return 0, io.EOF
	}
	r.reads++
	r.cancel()
	return copy(p, "a b\nc"), nil
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

// testFS holds the fixtures used by the tests:
//
//	file.txt:    "hello world\nfoo bar\n"  (2 lines, 4 words, 20 bytes, 11 longest)
//	one.txt:     "a b\nc"                  (1 line, 3 words, 5 bytes, 3 longest)
//	two.txt:     "one\ntwo\n"              (2 lines, 2 words, 8 bytes, 3 longest)
//	empty.txt:   ""                        (0 lines, 0 words, 0 bytes, 0 longest)
//	spaces.txt:  "   \t\n"                 (1 line, 0 words, 5 bytes, 8 longest)
//	utf8.txt:    "héllo wörld\n"           (1 line, 2 words, 14 bytes, 12 chars, 9 longest)
//	invalid.txt: "\xffa\xfe é\n"           (1 line, 2 words, 7 bytes, 6 chars, 2 longest)
//	"-l":        "x\n"                     (1 line, 1 word, 2 bytes, 1 longest)
//	sub/inner.txt: "inner\n"               (1 line, 1 word, 6 bytes, 5 longest)
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"file.txt":      {Data: []byte("hello world\nfoo bar\n")},
		"one.txt":       {Data: []byte("a b\nc")},
		"two.txt":       {Data: []byte("one\ntwo\n")},
		"empty.txt":     {Data: []byte("")},
		"spaces.txt":    {Data: []byte("   \t\n")},
		"utf8.txt":      {Data: []byte("héllo wörld\n")},
		"invalid.txt":   {Data: []byte{0xff, 'a', 0xfe, ' ', 0xc3, 0xa9, '\n'}},
		"-l":            {Data: []byte("x\n")},
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
			stdin: strings.NewReader("a b\nc"),
			want:  "1 3 5\n",
		},
		{
			name:  "empty standard input",
			stdin: strings.NewReader(""),
			want:  "0 0 0\n",
		},
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("a b\nc"),
			args:  []string{"-"},
			want:  "1 3 5 -\n",
		},
		{
			name:  "dash combined with a file",
			stdin: strings.NewReader("a b\nc"),
			fsys:  testFS(),
			args:  []string{"-", "one.txt"},
			want:  "1 3 5 -\n1 3 5 one.txt\n2 6 10 total\n",
		},
		{
			name:  "double dash with no operands reads the standard input",
			stdin: strings.NewReader("a b\nc"),
			args:  []string{"--"},
			want:  "1 3 5\n",
		},
		{
			name:  "repeated standard input operands",
			stdin: strings.NewReader("a b\nc"),
			args:  []string{"-", "-"},
			want:  "1 3 5 -\n0 0 0 -\n1 3 5 total\n",
		},
		{
			name: "single file",
			fsys: testFS(),
			args: []string{"file.txt"},
			want: "2 4 20 file.txt\n",
		},
		{
			name: "empty file",
			fsys: testFS(),
			args: []string{"empty.txt"},
			want: "0 0 0 empty.txt\n",
		},
		{
			name: "multiple files write a total",
			fsys: testFS(),
			args: []string{"file.txt", "one.txt"},
			want: "2 4 20 file.txt\n1 3 5 one.txt\n3 7 25 total\n",
		},
		{
			name: "the same file twice",
			fsys: testFS(),
			args: []string{"file.txt", "file.txt"},
			want: "2 4 20 file.txt\n2 4 20 file.txt\n4 8 40 total\n",
		},
		{
			name: "the pathname is echoed as given",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt"},
			want: "1 1 6 inner.txt\n",
		},
		{
			name: "a file named like an option after a double dash",
			fsys: testFS(),
			args: []string{"--", "-l"},
			want: "1 1 2 -l\n",
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
			name: "c writes only bytes",
			fsys: testFS(),
			args: []string{"-c", "file.txt"},
			want: "20 file.txt\n",
		},
		{
			name: "l writes only newlines",
			fsys: testFS(),
			args: []string{"-l", "file.txt"},
			want: "2 file.txt\n",
		},
		{
			name: "w writes only words",
			fsys: testFS(),
			args: []string{"-w", "file.txt"},
			want: "4 file.txt\n",
		},
		{
			name: "m writes only characters",
			fsys: testFS(),
			args: []string{"-m", "utf8.txt"},
			want: "12 utf8.txt\n",
		},
		{
			name: "grouped options",
			fsys: testFS(),
			args: []string{"-lw", "file.txt"},
			want: "2 4 file.txt\n",
		},
		{
			name: "grouped options in any order",
			fsys: testFS(),
			args: []string{"-wl", "file.txt"},
			want: "2 4 file.txt\n",
		},
		{
			name: "repeated options",
			fsys: testFS(),
			args: []string{"-l", "-l", "file.txt"},
			want: "2 file.txt\n",
		},
		{
			name: "l and c",
			fsys: testFS(),
			args: []string{"-lc", "file.txt"},
			want: "2 20 file.txt\n",
		},
		{
			name: "l, w and c",
			fsys: testFS(),
			args: []string{"-lwc", "file.txt"},
			want: "2 4 20 file.txt\n",
		},
		{
			name: "full output with characters",
			fsys: testFS(),
			args: []string{"-mlw", "utf8.txt"},
			want: "1 2 12 utf8.txt\n",
		},
		{
			name: "c and m together write both counts",
			fsys: testFS(),
			args: []string{"-cm", "utf8.txt"},
			want: "12 14 utf8.txt\n",
		},
		{
			name: "c and m together in any order",
			fsys: testFS(),
			args: []string{"-mc", "utf8.txt"},
			want: "12 14 utf8.txt\n",
		},
		{
			name: "options are not parsed after the first operand",
			fsys: testFS(),
			args: []string{"file.txt", "-l"},
			want: "2 4 20 file.txt\n1 1 2 -l\n3 5 22 total\n",
		},
		{
			name: "options apply to every file and the total",
			fsys: testFS(),
			args: []string{"-l", "file.txt", "one.txt"},
			want: "2 file.txt\n1 one.txt\n3 total\n",
		},
		{
			name: "a lone double dash is accepted",
			fsys: testFS(),
			args: []string{"--", "file.txt"},
			want: "2 4 20 file.txt\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunWords(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		want  string
	}{
		{
			name:  "leading and trailing white space",
			stdin: "  leading and trailing  ",
			want:  "0 3 24\n",
		},
		{
			name:  "every ASCII white space character separates words",
			stdin: "a\tb\vc\fd\re\nf",
			want:  "1 6 11\n",
		},
		{
			name:  "only white space",
			stdin: "   ",
			want:  "0 0 3\n",
		},
		{
			name:  "no trailing newline",
			stdin: "no-trailing-newline",
			want:  "0 1 19\n",
		},
		{
			name:  "consecutive newlines",
			stdin: "a b\nc\n\n",
			want:  "3 3 7\n",
		},
		{
			name:  "invalid UTF-8 bytes are counted as words",
			stdin: string([]byte{0xff, 0xfe}),
			want:  "0 1 2\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, testCase{stdin: strings.NewReader(test.stdin), want: test.want})
		})
	}
}

func TestRunChars(t *testing.T) {
	tests := []testCase{
		{
			name: "multibyte characters",
			fsys: testFS(),
			args: []string{"-ml", "utf8.txt"},
			want: "1 12 utf8.txt\n",
		},
		{
			name: "invalid bytes count as one character each",
			fsys: testFS(),
			args: []string{"-m", "invalid.txt"},
			want: "6 invalid.txt\n",
		},
		{
			name: "invalid bytes are counted in bytes mode",
			fsys: testFS(),
			args: []string{"invalid.txt"},
			want: "1 2 7 invalid.txt\n",
		},
		{
			name: "words are also counted in characters mode",
			fsys: testFS(),
			args: []string{"-mw", "invalid.txt"},
			want: "2 6 invalid.txt\n",
		},
		{
			name:  "characters from the standard input",
			stdin: strings.NewReader("héllo wörld\n"),
			args:  []string{"-m"},
			want:  "12\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLongOptions(t *testing.T) {
	tests := []testCase{
		{
			name: "lines counts the lines",
			fsys: testFS(),
			args: []string{"--lines", "file.txt"},
			want: "2 file.txt\n",
		},
		{
			name: "words counts the words",
			fsys: testFS(),
			args: []string{"--words", "file.txt"},
			want: "4 file.txt\n",
		},
		{
			name: "bytes counts the bytes",
			fsys: testFS(),
			args: []string{"--bytes", "file.txt"},
			want: "20 file.txt\n",
		},
		{
			name: "chars counts the characters",
			fsys: testFS(),
			args: []string{"--chars", "utf8.txt"},
			want: "12 utf8.txt\n",
		},
		{
			name: "max-line-length counts the longest line",
			fsys: testFS(),
			args: []string{"--max-line-length", "file.txt"},
			want: "11 file.txt\n",
		},
		{
			name:     "a long option with a value it doesn't take is an error",
			fsys:     testFS(),
			args:     []string{"--lines=2", "file.txt"},
			wantErr:  "wc: option '--lines=2' doesn't allow an argument\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunMaxLineLength(t *testing.T) {
	tests := []testCase{
		{
			name: "the longest line",
			fsys: testFS(),
			args: []string{"-L", "file.txt"},
			want: "11 file.txt\n",
		},
		{
			name:  "empty input",
			stdin: strings.NewReader(""),
			args:  []string{"-L"},
			want:  "0\n",
		},
		{
			name:  "a trailing line without a newline is counted",
			stdin: strings.NewReader("a\nlonger"),
			args:  []string{"-L"},
			want:  "6\n",
		},
		{
			name:  "a tab advances to the next 8th column",
			stdin: strings.NewReader("a\tb\n"),
			args:  []string{"-L"},
			want:  "9\n",
		},
		{
			name:  "a tab at the end of a line rounds up to a multiple of 8",
			stdin: strings.NewReader("a\t\n"),
			args:  []string{"-L"},
			want:  "8\n",
		},
		{
			name:  "a carriage return ends a line",
			stdin: strings.NewReader("ab\rcd\n"),
			args:  []string{"-L"},
			want:  "2\n",
		},
		{
			name:  "a form feed ends a line",
			stdin: strings.NewReader("ab\fcd\n"),
			args:  []string{"-L"},
			want:  "2\n",
		},
		{
			name:  "a vertical tab counts no columns",
			stdin: strings.NewReader("a\vb\n"),
			args:  []string{"-L"},
			want:  "2\n",
		},
		{
			name:  "control bytes count no columns",
			stdin: strings.NewReader("\x01ab\x7f\n"),
			args:  []string{"-L"},
			want:  "2\n",
		},
		{
			name: "multibyte characters count no columns",
			fsys: testFS(),
			args: []string{"-L", "utf8.txt"},
			want: "9 utf8.txt\n",
		},
		{
			name: "characters mode keeps the byte columns",
			fsys: testFS(),
			args: []string{"-mL", "utf8.txt"},
			want: "12 9 utf8.txt\n",
		},
		{
			name: "invalid bytes count no columns",
			fsys: testFS(),
			args: []string{"-L", "invalid.txt"},
			want: "2 invalid.txt\n",
		},
		{
			name: "the longest line is the fourth of the five columns",
			fsys: testFS(),
			args: []string{"-lwmL", "utf8.txt"},
			want: "1 2 12 9 utf8.txt\n",
		},
		{
			name: "all counts in the manual order",
			fsys: testFS(),
			args: []string{"-lcwmL", "utf8.txt"},
			want: "1 2 12 14 9 utf8.txt\n",
		},
		{
			name: "the total is the maximum, not the sum",
			fsys: testFS(),
			args: []string{"-L", "file.txt", "one.txt"},
			want: "11 file.txt\n3 one.txt\n11 total\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunTotal(t *testing.T) {
	tests := []testCase{
		{
			name: "auto is the default",
			fsys: testFS(),
			args: []string{"--total=auto", "file.txt", "one.txt"},
			want: "2 4 20 file.txt\n1 3 5 one.txt\n3 7 25 total\n",
		},
		{
			name: "auto with a single file writes no total",
			fsys: testFS(),
			args: []string{"--total=auto", "file.txt"},
			want: "2 4 20 file.txt\n",
		},
		{
			name: "always writes a total for a single file",
			fsys: testFS(),
			args: []string{"--total=always", "file.txt"},
			want: "2 4 20 file.txt\n2 4 20 total\n",
		},
		{
			name:  "always writes a total for the standard input",
			stdin: strings.NewReader("a b\nc"),
			args:  []string{"--total=always"},
			want:  "1 3 5\n1 3 5 total\n",
		},
		{
			name: "only writes just the cumulative counts",
			fsys: testFS(),
			args: []string{"--total=only", "file.txt", "one.txt"},
			want: "3 7 25\n",
		},
		{
			name: "only with a single file writes its counts",
			fsys: testFS(),
			args: []string{"--total=only", "file.txt"},
			want: "2 4 20\n",
		},
		{
			name:  "only with the standard input",
			stdin: strings.NewReader("a b\nc"),
			args:  []string{"--total=only"},
			want:  "1 3 5\n",
		},
		{
			name: "never writes no total",
			fsys: testFS(),
			args: []string{"--total=never", "file.txt", "one.txt"},
			want: "2 4 20 file.txt\n1 3 5 one.txt\n",
		},
		{
			name: "the word may be held by the next argument",
			fsys: testFS(),
			args: []string{"--total", "never", "file.txt", "one.txt"},
			want: "2 4 20 file.txt\n1 3 5 one.txt\n",
		},
		{
			name: "the total keeps the selected counts",
			fsys: testFS(),
			args: []string{"-L", "--total=always", "file.txt"},
			want: "11 file.txt\n11 total\n",
		},
		{
			name:     "an invalid word is an error",
			args:     []string{"--total=bogus"},
			wantErr:  "wc: invalid argument 'bogus' for '--total'\n",
			wantExit: 2,
		},
		{
			name:     "a missing word is an error",
			args:     []string{"--total"},
			wantErr:  "wc: option '--total' requires an argument\n",
			wantExit: 2,
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
			wantErr:  "wc: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing file doesn't stop the other files",
			fsys:     testFS(),
			args:     []string{"missing.txt", "file.txt"},
			want:     "2 4 20 file.txt\n2 4 20 total\n",
			wantErr:  "wc: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "every missing file still writes a total",
			fsys:    testFS(),
			args:    []string{"missing.txt", "other.txt"},
			want:    "0 0 0 total\n",
			wantErr: "wc: missing.txt: file does not exist\nwc: other.txt: file does not exist\n",
			// The total is written even when no file could be read.
			wantExit: 1,
		},
		{
			name:     "invalid option",
			args:     []string{"-x"},
			wantErr:  "wc: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "invalid option in a group",
			args:     []string{"-lx"},
			wantErr:  "wc: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "unknown long option",
			args:     []string{"--bogus"},
			wantErr:  "wc: unrecognized option '--bogus'\n",
			wantExit: 2,
		},
		{
			name:     "invalid option before operands",
			args:     []string{"-x", "file.txt"},
			wantErr:  "wc: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "zero-length file name",
			fsys:     testFS(),
			args:     []string{""},
			wantErr:  "wc: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "file operand without a file system",
			args:     []string{"file.txt"},
			wantErr:  "wc: file.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			fsys:     testFS(),
			wantErr:  "wc: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "standard input read error",
			stdin:    errorReader{err: errors.New("read failed")},
			fsys:     testFS(),
			wantErr:  "wc: standard input: read failed\n",
			wantExit: 1,
		},
		{
			name:     "dash read error is labeled with the operand",
			stdin:    errorReader{err: errors.New("read failed")},
			args:     []string{"-"},
			wantErr:  "wc: -: read failed\n",
			wantExit: 1,
		},
		{
			name:     "file read error",
			stdin:    strings.NewReader(""),
			args:     []string{"dir"},
			fsys:     fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			wantErr:  "wc: dir: invalid argument\n",
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
		args:  []string{"file.txt"},
		want:  "2 4 20 file.txt\n",
	})
	if stdin.read {
		t.Error("Run() read stdin, want it untouched")
	}
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("a b\nc"),
		Stdout: errorWriter{err: writeErr},
		Stderr: new(bytes.Buffer),
	}
	err := New(rt).Run(t.Context())

	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		runCancelled(t, ctx, strings.NewReader("a b\nc"), nil)
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

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "wc (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
