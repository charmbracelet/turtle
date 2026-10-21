package expand

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

// testFS holds the fixtures used by the tests:
//
//	tabs.txt:      "\tfoo\nbar\tbaz\n"
//	utf8.txt:      "á\tb\n日本\t語\n"
//	invalid.txt:   "a\xff\tb\n" (an invalid UTF-8 byte)
//	nonl.txt:      "a\tb" (no trailing newline)
//	empty.txt:     ""
//	"-t":          "z\tw\n" (a file named like an option)
//	sub/inner.txt: "x\ty\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"tabs.txt":      {Data: []byte("\tfoo\nbar\tbaz\n")},
		"utf8.txt":      {Data: []byte("á\tb\n日本\t語\n")},
		"invalid.txt":   {Data: []byte{'a', 0xff, '\t', 'b', '\n'}},
		"nonl.txt":      {Data: []byte("a\tb")},
		"empty.txt":     {Data: []byte("")},
		"-t":            {Data: []byte("z\tw\n")},
		"sub/inner.txt": {Data: []byte("x\ty\n")},
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
			name:  "no operands read the standard input",
			stdin: strings.NewReader("\tfoo\n"),
			want:  "        foo\n",
		},
		{
			name:  "an empty standard input",
			stdin: strings.NewReader(""),
			want:  "",
		},
		{
			name:  "an empty line stays empty",
			stdin: strings.NewReader("\n"),
			want:  "\n",
		},
		{
			name:  "a tab jumps to the next stop",
			stdin: strings.NewReader("a\tb\n"),
			want:  "a       b\n",
		},
		{
			name:  "a tab on a stop jumps to the next one",
			stdin: strings.NewReader("12345678\tb\n"),
			want:  "12345678        b\n",
		},
		{
			name:  "consecutive tabs",
			stdin: strings.NewReader("\t\tx\n"),
			want:  "                x\n",
		},
		{
			name:  "a line without tabs is copied",
			stdin: strings.NewReader("plain text\n"),
			want:  "plain text\n",
		},
		{
			name:  "a carriage return is an ordinary character",
			stdin: strings.NewReader("ab\rc\td\n"),
			want:  "ab\rc    d\n",
		},
		{
			name:  "a missing final newline is not added",
			stdin: strings.NewReader("a\tb"),
			want:  "a       b",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunTabStops(t *testing.T) {
	tests := []testCase{
		{
			name:  "a single number sets the distance between stops",
			stdin: strings.NewReader("a\tb\tc\n"),
			args:  []string{"-t", "4"},
			want:  "a   b   c\n",
		},
		{
			name:  "a distance of one",
			stdin: strings.NewReader("\ta\tb\n"),
			args:  []string{"-t", "1"},
			want:  " a b\n",
		},
		{
			name:  "an attached distance",
			stdin: strings.NewReader("a\tb\n"),
			args:  []string{"-t4"},
			want:  "a   b\n",
		},
		{
			name:  "a list of stops",
			stdin: strings.NewReader("\ta\tb\n"),
			args:  []string{"-t", "4,8"},
			want:  "    a   b\n",
		},
		{
			name:  "a blank separated list of stops",
			stdin: strings.NewReader("\ta\tb\n"),
			args:  []string{"-t", "4 8"},
			want:  "    a   b\n",
		},
		{
			name:  "tabs past the last stop become single spaces",
			stdin: strings.NewReader("\ta\tb\tc\td\n"),
			args:  []string{"-t", "4,8"},
			want:  "    a   b c d\n",
		},
		{
			name:  "the last list wins",
			stdin: strings.NewReader("\ta\n"),
			args:  []string{"-t", "4", "-t", "8"},
			want:  "        a\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunBackspace(t *testing.T) {
	tests := []testCase{
		{
			name:  "a backspace decrements the column",
			stdin: strings.NewReader("ab\b\tc\n"),
			want:  "ab\b       c\n",
		},
		{
			name:  "the column is not decremented below zero",
			stdin: strings.NewReader("\b\ta\n"),
			want:  "\b        a\n",
		},
		{
			name:  "a tab is recomputed after a backspace",
			stdin: strings.NewReader("12345678\b\ta\n"),
			want:  "12345678\b a\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunUTF8(t *testing.T) {
	tests := []testCase{
		{
			name:  "a multibyte character counts as one column",
			stdin: strings.NewReader("á\tb\n"),
			want:  "á       b\n",
		},
		{
			name: "an invalid byte counts as one column",
			fsys: testFS(),
			args: []string{"invalid.txt"},
			want: "a\xff      b\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFiles(t *testing.T) {
	tests := []testCase{
		{
			name: "a file operand",
			fsys: testFS(),
			args: []string{"tabs.txt"},
			want: "        foo\nbar     baz\n",
		},
		{
			name: "multiple files are concatenated in order",
			fsys: testFS(),
			args: []string{"tabs.txt", "utf8.txt"},
			want: "        foo\nbar     baz\ná       b\n日本      語\n",
		},
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("s\tt\n"),
			fsys:  testFS(),
			args:  []string{"-"},
			want:  "s       t\n",
		},
		{
			name:  "dash between files reads the standard input at that point",
			stdin: strings.NewReader("s\tt\n"),
			fsys:  testFS(),
			args:  []string{"nonl.txt", "-"},
			want:  "a       bs       t\n",
		},
		{
			name: "pathnames are resolved from the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt"},
			want: "x       y\n",
		},
		{
			name: "a missing final newline is not added",
			fsys: testFS(),
			args: []string{"nonl.txt"},
			want: "a       b",
		},
		{
			name: "an empty file produces no output",
			fsys: testFS(),
			args: []string{"empty.txt"},
			want: "",
		},
		{
			name: "a file named like an option after a double dash",
			fsys: testFS(),
			args: []string{"--", "-t"},
			want: "z       w\n",
		},
		{
			name:     "options are not parsed after the first operand",
			fsys:     testFS(),
			args:     []string{"tabs.txt", "-x"},
			want:     "        foo\nbar     baz\n",
			wantErr:  "expand: -x: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing file doesn't stop the other files",
			fsys:     testFS(),
			args:     []string{"missing.txt", "tabs.txt"},
			want:     "        foo\nbar     baz\n",
			wantErr:  "expand: missing.txt: file does not exist\n",
			wantExit: 1,
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
			name:     "invalid option",
			args:     []string{"-x"},
			wantErr:  "expand: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "the gnu initial option is not supported",
			args:     []string{"-i"},
			wantErr:  "expand: invalid option -- 'i'\n",
			wantExit: 2,
		},
		{
			name:     "unknown long option",
			args:     []string{"--foo"},
			wantErr:  "expand: unrecognized option '--foo'\n",
			wantExit: 2,
		},
		{
			name:     "missing tab list argument",
			args:     []string{"-t"},
			wantErr:  "expand: option requires an argument -- 't'\n",
			wantExit: 2,
		},
		{
			name:     "zero distance",
			args:     []string{"-t", "0"},
			wantErr:  "expand: invalid tab stop: '0'\n",
			wantExit: 2,
		},
		{
			name:     "a negative stop",
			args:     []string{"-t", "-4"},
			wantErr:  "expand: invalid tab stop: '-4'\n",
			wantExit: 2,
		},
		{
			name:     "a non numeric stop",
			args:     []string{"-t", "x"},
			wantErr:  "expand: invalid tab stop: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "not ascending stops",
			args:     []string{"-t", "8,4"},
			wantErr:  "expand: tab sizes must be ascending\n",
			wantExit: 2,
		},
		{
			name:     "repeated stops",
			args:     []string{"-t", "4,4"},
			wantErr:  "expand: tab sizes must be ascending\n",
			wantExit: 2,
		},
		{
			name:     "an empty tab list",
			args:     []string{"-t", ""},
			wantErr:  "expand: empty tab list\n",
			wantExit: 2,
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
			name:  "tabs",
			stdin: strings.NewReader("a\tb\n"),
			args:  []string{"--tabs", "4"},
			want:  "a   b\n",
		},
		{
			name:  "tabs with an attached value",
			stdin: strings.NewReader("a\tb\n"),
			args:  []string{"--tabs=4"},
			want:  "a   b\n",
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
			wantErr:  "expand: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "directory operand",
			fsys:     fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			args:     []string{"dir"},
			wantErr:  "expand: dir: invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "zero-length file name",
			fsys:     testFS(),
			args:     []string{""},
			wantErr:  "expand: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "file operand without a file system",
			args:     []string{"tabs.txt"},
			wantErr:  "expand: tabs.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			wantErr:  "expand: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "standard input read error",
			stdin:    errorReader{err: errors.New("read failed")},
			wantErr:  "expand: standard input: read failed\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
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
	if got := stderr.String(); got != "expand: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "expand: write error: write failed\n")
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		runCancelled(t, ctx, strings.NewReader("hello\n"))
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runCancelled(t, ctx, &cancelReader{cancel: cancel})
	})
}

// runCancelled runs the utility with a context that is either already done or
// becomes done while reading, and checks that the context error is returned
// without any output.
func runCancelled(t *testing.T, ctx context.Context, stdin io.Reader) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
	err := New(rt).Run(ctx)

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
