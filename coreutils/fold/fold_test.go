package fold

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
//	long.txt:      90 "a" characters
//	tabs.txt:      "ab\tcd\n"
//	cr.txt:        "abc\rdef\n"
//	bs.txt:        "abc\b\b\bdefgh\n"
//	utf8.txt:      "áéç\n"
//	invalid.txt:   "a\xffbc\n" (an invalid UTF-8 byte)
//	nonl.txt:      "abcdef" (no trailing newline)
//	empty.txt:     ""
//	"-s":          "aa bb\n" (a file named like an option)
//	sub/inner.txt: "abcdefghi\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"long.txt":      {Data: []byte(strings.Repeat("a", 90) + "\n")},
		"tabs.txt":      {Data: []byte("ab\tcd\n")},
		"cr.txt":        {Data: []byte("abc\rdef\n")},
		"bs.txt":        {Data: []byte("abc\b\b\bdefgh\n")},
		"utf8.txt":      {Data: []byte("áéç\n")},
		"invalid.txt":   {Data: []byte{'a', 0xff, 'b', 'c', '\n'}},
		"nonl.txt":      {Data: []byte("abcdef")},
		"empty.txt":     {Data: []byte("")},
		"-s":            {Data: []byte("aa bb\n")},
		"sub/inner.txt": {Data: []byte("abcdefghi\n")},
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
			stdin: strings.NewReader("hello\n"),
			want:  "hello\n",
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
			name:  "short lines are unchanged",
			stdin: strings.NewReader("ab\ncd\n"),
			want:  "ab\ncd\n",
		},
		{
			name:  "the default width is eighty",
			stdin: strings.NewReader(strings.Repeat("a", 90) + "\n"),
			want:  strings.Repeat("a", 80) + "\n" + strings.Repeat("a", 10) + "\n",
		},
		{
			name:  "a line of the exact width is not broken",
			stdin: strings.NewReader(strings.Repeat("a", 80) + "\n"),
			want:  strings.Repeat("a", 80) + "\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunWidth(t *testing.T) {
	tests := []testCase{
		{
			name:  "lines are broken at the width",
			stdin: strings.NewReader("abcdefgh\n"),
			args:  []string{"-w", "5"},
			want:  "abcde\nfgh\n",
		},
		{
			name:  "a line of the exact width is not broken",
			stdin: strings.NewReader("abcde\n"),
			args:  []string{"-w", "5"},
			want:  "abcde\n",
		},
		{
			name:  "a width of one breaks after every character",
			stdin: strings.NewReader("abc\n"),
			args:  []string{"-w", "1"},
			want:  "a\nb\nc\n",
		},
		{
			name:  "the last width wins",
			stdin: strings.NewReader("abcdef\n"),
			args:  []string{"-w", "5", "-w", "3"},
			want:  "abc\ndef\n",
		},
		{
			name:  "an attached width",
			stdin: strings.NewReader("abcdefgh\n"),
			args:  []string{"-w5"},
			want:  "abcde\nfgh\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunBytes(t *testing.T) {
	tests := []testCase{
		{
			name: "bytes count as columns",
			fsys: testFS(),
			args: []string{"-b", "-w", "2", "utf8.txt"},
			want: "á\né\nç\n",
		},
		{
			name:  "a wide character is never split",
			stdin: strings.NewReader("á\n"),
			args:  []string{"-b", "-w", "1"},
			want:  "á\n",
		},
		{
			name: "tabs count one byte",
			fsys: testFS(),
			args: []string{"-b", "-w", "3", "tabs.txt"},
			want: "ab\t\ncd\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunColumns(t *testing.T) {
	tests := []testCase{
		{
			name: "tabs advance to the next multiple of eight",
			fsys: testFS(),
			args: []string{"-w", "4", "tabs.txt"},
			want: "ab\n\t\ncd\n",
		},
		{
			name: "backspaces decrement the column",
			fsys: testFS(),
			args: []string{"-w", "5", "bs.txt"},
			want: "abc\b\b\bdefgh\n",
		},
		{
			name: "a carriage return resets the column",
			fsys: testFS(),
			args: []string{"-w", "5", "cr.txt"},
			want: "abc\rdef\n",
		},
		{
			name:  "the column is not decremented below zero",
			stdin: strings.NewReader("\b\babc\n"),
			args:  []string{"-w", "5"},
			want:  "\b\babc\n",
		},
		{
			name: "a multibyte character counts as one column",
			fsys: testFS(),
			args: []string{"-w", "2", "utf8.txt"},
			want: "áé\nç\n",
		},
		{
			name: "an invalid byte counts as one column",
			fsys: testFS(),
			args: []string{"-w", "3", "invalid.txt"},
			want: "a\xffb\nc\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSpaces(t *testing.T) {
	tests := []testCase{
		{
			name:  "the line breaks after the last blank",
			stdin: strings.NewReader("aaa bbbb\n"),
			args:  []string{"-w", "4", "-s"},
			want:  "aaa \nbbbb\n",
		},
		{
			name:  "the break happens after the last blank of the segment",
			stdin: strings.NewReader("aa bb cc\n"),
			args:  []string{"-w", "4", "-s"},
			want:  "aa \nbb \ncc\n",
		},
		{
			name:  "a segment without blanks breaks at the width",
			stdin: strings.NewReader("aaaaa\n"),
			args:  []string{"-w", "3", "-s"},
			want:  "aaa\naa\n",
		},
		{
			name:  "tabs are blanks for the break",
			stdin: strings.NewReader("aa\tbb\n"),
			args:  []string{"-w", "8", "-s"},
			want:  "aa\t\nbb\n",
		},
		{
			name:  "a blank at the start of the segment",
			stdin: strings.NewReader(" abc\n"),
			args:  []string{"-w", "2", "-s"},
			want:  " \nab\nc\n",
		},
		{
			name:  "blanks are counted in bytes with b",
			stdin: strings.NewReader("á b\n"),
			args:  []string{"-b", "-w", "3", "-s"},
			want:  "á \nb\n",
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
			args: []string{"long.txt"},
			want: strings.Repeat("a", 80) + "\n" + strings.Repeat("a", 10) + "\n",
		},
		{
			name: "multiple files are concatenated in order",
			fsys: testFS(),
			args: []string{"-w", "3", "tabs.txt", "cr.txt"},
			want: "ab\n\t\ncd\nabc\rdef\n",
		},
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("abcdefgh\n"),
			fsys:  testFS(),
			args:  []string{"-w", "4", "-"},
			want:  "abcd\nefgh\n",
		},
		{
			name:  "dash between files reads the standard input at that point",
			stdin: strings.NewReader("abcdefgh\n"),
			fsys:  testFS(),
			args:  []string{"-w", "4", "tabs.txt", "-"},
			want:  "ab\n\t\ncd\nabcd\nefgh\n",
		},
		{
			name: "pathnames are resolved from the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"-w", "4", "inner.txt"},
			want: "abcd\nefgh\ni\n",
		},
		{
			name: "a missing final newline is not added",
			fsys: testFS(),
			args: []string{"-w", "3", "nonl.txt"},
			want: "abc\ndef",
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
			args: []string{"--", "-s"},
			want: "aa bb\n",
		},
		{
			name:     "options are not parsed after the first operand",
			fsys:     testFS(),
			args:     []string{"tabs.txt", "-x"},
			want:     "ab\tcd\n",
			wantErr:  "fold: -x: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing file doesn't stop the other files",
			fsys:     testFS(),
			args:     []string{"missing.txt", "tabs.txt"},
			want:     "ab\tcd\n",
			wantErr:  "fold: missing.txt: file does not exist\n",
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
			wantErr:  "fold: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "unknown long option",
			args:     []string{"--foo"},
			wantErr:  "fold: unrecognized option '--foo'\n",
			wantExit: 2,
		},
		{
			name:     "missing width argument",
			args:     []string{"-w"},
			wantErr:  "fold: option requires an argument -- 'w'\n",
			wantExit: 2,
		},
		{
			name:     "a zero width",
			args:     []string{"-w", "0"},
			wantErr:  "fold: invalid width: '0'\n",
			wantExit: 2,
		},
		{
			name:     "a negative width",
			args:     []string{"-w", "-1"},
			wantErr:  "fold: invalid width: '-1'\n",
			wantExit: 2,
		},
		{
			name:     "a non numeric width",
			args:     []string{"-w", "x"},
			wantErr:  "fold: invalid width: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "a decimal width",
			args:     []string{"-w", "4.5"},
			wantErr:  "fold: invalid width: '4.5'\n",
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
			name:  "width",
			stdin: strings.NewReader("abcdefgh\n"),
			args:  []string{"--width", "5"},
			want:  "abcde\nfgh\n",
		},
		{
			name:  "width with an attached value",
			stdin: strings.NewReader("abcdefgh\n"),
			args:  []string{"--width=5"},
			want:  "abcde\nfgh\n",
		},
		{
			name:  "bytes",
			stdin: strings.NewReader("áé\n"),
			args:  []string{"--bytes", "--width", "2"},
			want:  "á\né\n",
		},
		{
			name:  "spaces",
			stdin: strings.NewReader("aaa bbbb\n"),
			args:  []string{"--spaces", "--width", "4"},
			want:  "aaa \nbbbb\n",
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
			wantErr:  "fold: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "directory operand",
			fsys:     fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			args:     []string{"dir"},
			wantErr:  "fold: dir: invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "zero-length file name",
			fsys:     testFS(),
			args:     []string{""},
			wantErr:  "fold: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "file operand without a file system",
			args:     []string{"tabs.txt"},
			wantErr:  "fold: tabs.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			wantErr:  "fold: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "standard input read error",
			stdin:    errorReader{err: errors.New("read failed")},
			wantErr:  "fold: standard input: read failed\n",
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
	if got := stderr.String(); got != "fold: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "fold: write error: write failed\n")
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
		want := "fold (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
