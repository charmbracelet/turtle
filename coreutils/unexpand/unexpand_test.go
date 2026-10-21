package unexpand

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
//	spaces.txt:    "        foo\n" (8 leading blanks)
//	mid.txt:       "a       b\n" (7 blanks after a character)
//	utf8.txt:      "á       b\n" (a multibyte character before the blanks)
//	invalid.txt:   "a\xff       b\n" (an invalid UTF-8 byte)
//	nonl.txt:      "        x" (no trailing newline)
//	empty.txt:     ""
//	"-a":          "        z\n" (a file named like an option)
//	sub/inner.txt: "    y\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"spaces.txt":    {Data: []byte("        foo\n")},
		"mid.txt":       {Data: []byte("a       b\n")},
		"utf8.txt":      {Data: []byte("á       b\n")},
		"invalid.txt":   {Data: []byte{'a', 0xff, ' ', ' ', ' ', ' ', ' ', ' ', ' ', 'b', '\n'}},
		"nonl.txt":      {Data: []byte("        x")},
		"empty.txt":     {Data: []byte("")},
		"-a":            {Data: []byte("        z\n")},
		"sub/inner.txt": {Data: []byte("    y\n")},
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
			stdin: strings.NewReader("        foo\n"),
			want:  "\tfoo\n",
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
			name:  "fewer than a tab stop of leading blanks",
			stdin: strings.NewReader("       foo\n"),
			want:  "       foo\n",
		},
		{
			name:  "leading blanks become tabs and spaces",
			stdin: strings.NewReader("         foo\n"),
			want:  "\t foo\n",
		},
		{
			name:  "two tab stops of leading blanks",
			stdin: strings.NewReader("                foo\n"),
			want:  "\t\tfoo\n",
		},
		{
			name:  "leading blanks and tabs mix",
			stdin: strings.NewReader("  \tfoo\n"),
			want:  "\tfoo\n",
		},
		{
			name:  "interior blanks are not converted",
			stdin: strings.NewReader("a       b\n"),
			want:  "a       b\n",
		},
		{
			name:  "a line starting with a non blank is untouched",
			stdin: strings.NewReader("x       y\n"),
			want:  "x       y\n",
		},
		{
			name:  "a missing final newline is not added",
			stdin: strings.NewReader("        x"),
			want:  "\tx",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunAll(t *testing.T) {
	tests := []testCase{
		{
			name:  "interior blanks preceding a stop are converted",
			stdin: strings.NewReader("a       b\n"),
			args:  []string{"-a"},
			want:  "a\tb\n",
		},
		{
			name:  "interior blanks not reaching a stop stay",
			stdin: strings.NewReader("a  b\n"),
			args:  []string{"-a"},
			want:  "a  b\n",
		},
		{
			name:  "a run of blanks and tabs",
			stdin: strings.NewReader("a \tb\n"),
			args:  []string{"-a"},
			want:  "a\tb\n",
		},
		{
			name:  "only the runs reaching a stop are converted",
			stdin: strings.NewReader("a       b     c\n"),
			args:  []string{"-a"},
			want:  "a\tb     c\n",
		},
		{
			name:  "trailing blanks",
			stdin: strings.NewReader("a       \n"),
			args:  []string{"-a"},
			want:  "a\t\n",
		},
		{
			name:  "leading blanks are still converted",
			stdin: strings.NewReader("        foo\n"),
			args:  []string{"-a"},
			want:  "\tfoo\n",
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
			stdin: strings.NewReader("     foo\n"),
			args:  []string{"-t", "4"},
			want:  "\t foo\n",
		},
		{
			name:  "a single number of eight is the default distance",
			stdin: strings.NewReader("        foo\n"),
			args:  []string{"-t", "8"},
			want:  "\tfoo\n",
		},
		{
			name:  "a list of stops",
			stdin: strings.NewReader("    x\n"),
			args:  []string{"-t", "4,8"},
			want:  "\tx\n",
		},
		{
			name:  "a list of stops with a remainder",
			stdin: strings.NewReader("     x\n"),
			args:  []string{"-t", "4,8"},
			want:  "\t x\n",
		},
		{
			name:  "multiple list stops",
			stdin: strings.NewReader("         x\n"),
			args:  []string{"-t", "4,8"},
			want:  "\t\t x\n",
		},
		{
			name:  "no conversions past the last stop",
			stdin: strings.NewReader("          x\n"),
			args:  []string{"-t", "4,8"},
			want:  "\t\t  x\n",
		},
		{
			name:  "a tab past the last stop is copied",
			stdin: strings.NewReader("        \t  x\n"),
			args:  []string{"-t", "4,8"},
			want:  "\t\t\t  x\n",
		},
		{
			name:  "a single blank just before a stop is not converted",
			stdin: strings.NewReader("a b\n"),
			args:  []string{"-t", "2"},
			want:  "a b\n",
		},
		{
			name:  "two blanks before a stop are converted",
			stdin: strings.NewReader("a  b\n"),
			args:  []string{"-t", "2"},
			want:  "a\t b\n",
		},
		{
			name:  "a lone leading blank on a stop is converted",
			stdin: strings.NewReader(" x\n"),
			args:  []string{"-t", "1"},
			want:  "\tx\n",
		},
		{
			name:  "t implies a",
			stdin: strings.NewReader("a       b\n"),
			args:  []string{"-t", "8"},
			want:  "a\tb\n",
		},
		{
			name:  "grouped a and t",
			stdin: strings.NewReader("     foo\n"),
			args:  []string{"-at", "4"},
			want:  "\t foo\n",
		},
		{
			name:  "the last list wins",
			stdin: strings.NewReader("     foo\n"),
			args:  []string{"-t", "8", "-t", "4"},
			want:  "\t foo\n",
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
			name:  "backspaces are copied and decrement the column",
			stdin: strings.NewReader("  \b        x\n"),
			args:  []string{"-a"},
			want:  "  \b\t x\n",
		},
		{
			name:  "the column is not decremented below zero",
			stdin: strings.NewReader("\b        x\n"),
			args:  []string{"-a"},
			want:  "\b\tx\n",
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
			name: "a multibyte character counts as one column",
			fsys: testFS(),
			args: []string{"-a", "utf8.txt"},
			want: "á\tb\n",
		},
		{
			name: "an invalid byte counts as one column",
			fsys: testFS(),
			args: []string{"-a", "invalid.txt"},
			want: "a\xff\t b\n",
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
			args: []string{"spaces.txt"},
			want: "\tfoo\n",
		},
		{
			name: "multiple files are concatenated in order",
			fsys: testFS(),
			args: []string{"spaces.txt", "mid.txt"},
			want: "\tfoo\na       b\n",
		},
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("        s\n"),
			fsys:  testFS(),
			args:  []string{"-"},
			want:  "\ts\n",
		},
		{
			name:  "dash between files reads the standard input at that point",
			stdin: strings.NewReader("        s\n"),
			fsys:  testFS(),
			args:  []string{"spaces.txt", "-", "mid.txt"},
			want:  "\tfoo\n\ts\na       b\n",
		},
		{
			name: "pathnames are resolved from the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt"},
			want: "    y\n",
		},
		{
			name: "a missing final newline is not added",
			fsys: testFS(),
			args: []string{"nonl.txt"},
			want: "\tx",
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
			args: []string{"--", "-a"},
			want: "\tz\n",
		},
		{
			name:     "options are not parsed after the first operand",
			fsys:     testFS(),
			args:     []string{"spaces.txt", "-x"},
			want:     "\tfoo\n",
			wantErr:  "unexpand: -x: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing file doesn't stop the other files",
			fsys:     testFS(),
			args:     []string{"missing.txt", "spaces.txt"},
			want:     "\tfoo\n",
			wantErr:  "unexpand: missing.txt: file does not exist\n",
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
			wantErr:  "unexpand: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "unknown long option",
			args:     []string{"--foo"},
			wantErr:  "unexpand: unrecognized option '--foo'\n",
			wantExit: 2,
		},
		{
			name:     "missing tab list argument",
			args:     []string{"-t"},
			wantErr:  "unexpand: option requires an argument -- 't'\n",
			wantExit: 2,
		},
		{
			name:     "zero distance",
			args:     []string{"-t", "0"},
			wantErr:  "unexpand: invalid tab stop: '0'\n",
			wantExit: 2,
		},
		{
			name:     "a non numeric stop",
			args:     []string{"-t", "x"},
			wantErr:  "unexpand: invalid tab stop: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "not ascending stops",
			args:     []string{"-t", "8,4"},
			wantErr:  "unexpand: tab sizes must be ascending\n",
			wantExit: 2,
		},
		{
			name:     "an empty tab list",
			args:     []string{"-t", ""},
			wantErr:  "unexpand: empty tab list\n",
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
			name:  "all",
			stdin: strings.NewReader("a       b\n"),
			args:  []string{"--all"},
			want:  "a\tb\n",
		},
		{
			name:  "tabs",
			stdin: strings.NewReader("     foo\n"),
			args:  []string{"--tabs", "4"},
			want:  "\t foo\n",
		},
		{
			name:  "tabs with an attached value",
			stdin: strings.NewReader("     foo\n"),
			args:  []string{"--tabs=4"},
			want:  "\t foo\n",
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
			wantErr:  "unexpand: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "directory operand",
			fsys:     fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			args:     []string{"dir"},
			wantErr:  "unexpand: dir: invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "zero-length file name",
			fsys:     testFS(),
			args:     []string{""},
			wantErr:  "unexpand: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "file operand without a file system",
			args:     []string{"spaces.txt"},
			wantErr:  "unexpand: spaces.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			wantErr:  "unexpand: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "standard input read error",
			stdin:    errorReader{err: errors.New("read failed")},
			wantErr:  "unexpand: standard input: read failed\n",
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
	if got := stderr.String(); got != "unexpand: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "unexpand: write error: write failed\n")
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
