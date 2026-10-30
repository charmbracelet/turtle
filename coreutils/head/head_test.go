package head

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

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

// testFS holds the fixtures used by the tests:
//
//	a.txt:    "alpha\n"
//	b.txt:    "beta\n"
//	empty.txt: ""
//	nonl.txt: "one\ntwo\nthree" (no trailing newline)
//	lines.txt: "l1\nl2\nl3\nl4\nl5\n"
//	many.txt: 12 lines, "01\n" to "12\n"
//	bytes.txt: "0123456789"
//	big.txt: 4100 bytes, "0123456789" repeated
//	nlines.txt: 2000 lines, "0000\n" to "1999\n"
//	nul.txt: "a\x00b\x00c\x00"
//	nulpart.txt: "x\x00y" (a final item without the NUL)
//	"-n":     "dash-n\n" (a file named like an option)
//	"-5":     "dash-5\n" (a file named like the obsolete form)
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":         {Data: []byte("alpha\n")},
		"b.txt":         {Data: []byte("beta\n")},
		"empty.txt":     {Data: []byte("")},
		"nonl.txt":      {Data: []byte("one\ntwo\nthree")},
		"lines.txt":     {Data: []byte("l1\nl2\nl3\nl4\nl5\n")},
		"many.txt":      {Data: []byte("01\n02\n03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n")},
		"bytes.txt":     {Data: []byte("0123456789")},
		"big.txt":       {Data: bytes.Repeat([]byte("0123456789"), 410)},
		"nlines.txt":    {Data: linesFile(2000)},
		"nul.txt":       {Data: []byte("a\x00b\x00c\x00")},
		"nulpart.txt":   {Data: []byte("x\x00y")},
		"-n":            {Data: []byte("dash-n\n")},
		"-5":            {Data: []byte("dash-5\n")},
		"sub/inner.txt": {Data: []byte("inner\n")},
	}
}

// linesFile builds the contents of a file with n numbered lines.
func linesFile(n int) []byte {
	var data []byte
	for i := range n {
		data = fmt.Appendf(data, "%04d\n", i)
	}
	return data
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
			name: "fewer than ten lines copies the whole file",
			fsys: testFS(),
			args: []string{"lines.txt"},
			want: "l1\nl2\nl3\nl4\nl5\n",
		},
		{
			name: "defaults to the first ten lines",
			fsys: testFS(),
			args: []string{"many.txt"},
			want: "01\n02\n03\n04\n05\n06\n07\n08\n09\n10\n",
		},
		{
			name: "file without a trailing newline",
			fsys: testFS(),
			args: []string{"nonl.txt"},
			want: "one\ntwo\nthree",
		},
		{
			name: "empty file",
			fsys: testFS(),
			args: []string{"empty.txt"},
			want: "",
		},
		{
			name: "pathnames are resolved from the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt"},
			want: "inner\n",
		},
		{
			name: "double dash allows a file named like an option",
			fsys: testFS(),
			args: []string{"--", "-n"},
			want: "dash-n\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLines(t *testing.T) {
	tests := []testCase{
		{
			name: "the first lines of a file",
			fsys: testFS(),
			args: []string{"-n", "2", "lines.txt"},
			want: "l1\nl2\n",
		},
		{
			name: "number joined to the option",
			fsys: testFS(),
			args: []string{"-n2", "lines.txt"},
			want: "l1\nl2\n",
		},
		{
			name: "zero lines",
			fsys: testFS(),
			args: []string{"-n", "0", "lines.txt"},
			want: "",
		},
		{
			name: "more lines than the file copies it in its entirety",
			fsys: testFS(),
			args: []string{"-n", "100", "lines.txt"},
			want: "l1\nl2\nl3\nl4\nl5\n",
		},
		{
			name: "a final line without a newline counts as a line",
			fsys: testFS(),
			args: []string{"-n", "2", "nonl.txt"},
			want: "one\ntwo\n",
		},
		{
			name: "counting the final incomplete line",
			fsys: testFS(),
			args: []string{"-n", "3", "nonl.txt"},
			want: "one\ntwo\nthree",
		},
		{
			name:  "lines of the standard input",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-n", "2"},
			want:  "a\nb\n",
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
			name: "the first bytes of a file",
			fsys: testFS(),
			args: []string{"-c", "3", "bytes.txt"},
			want: "012",
		},
		{
			name: "number joined to the option",
			fsys: testFS(),
			args: []string{"-c3", "bytes.txt"},
			want: "012",
		},
		{
			name: "zero bytes",
			fsys: testFS(),
			args: []string{"-c", "0", "bytes.txt"},
			want: "",
		},
		{
			name: "more bytes than the file copies it in its entirety",
			fsys: testFS(),
			args: []string{"-c", "20", "bytes.txt"},
			want: "0123456789",
		},
		{
			name: "bytes can cut through a line",
			fsys: testFS(),
			args: []string{"-c", "4", "lines.txt"},
			want: "l1\nl",
		},
		{
			name: "bytes ending at a newline",
			fsys: testFS(),
			args: []string{"-c", "3", "lines.txt"},
			want: "l1\n",
		},
		{
			name:  "bytes of the standard input",
			stdin: strings.NewReader("hello world"),
			args:  []string{"-c", "5"},
			want:  "hello",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunMultipleFiles(t *testing.T) {
	tests := []testCase{
		{
			name: "each file is preceded by a header",
			fsys: testFS(),
			args: []string{"a.txt", "b.txt"},
			want: "==> a.txt <==\nalpha\n\n==> b.txt <==\nbeta\n",
		},
		{
			name: "headers with a line count",
			fsys: testFS(),
			args: []string{"-n", "1", "lines.txt", "many.txt"},
			want: "==> lines.txt <==\nl1\n\n==> many.txt <==\n01\n",
		},
		{
			name: "the same file twice",
			fsys: testFS(),
			args: []string{"a.txt", "a.txt"},
			want: "==> a.txt <==\nalpha\n\n==> a.txt <==\nalpha\n",
		},
		{
			name: "empty file",
			fsys: testFS(),
			args: []string{"a.txt", "empty.txt"},
			want: "==> a.txt <==\nalpha\n\n==> empty.txt <==\n",
		},
		{
			name:     "a failing file doesn't stop the others",
			fsys:     testFS(),
			args:     []string{"missing.txt", "a.txt"},
			want:     "==> a.txt <==\nalpha\n",
			wantErr:  "head: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a failing file between valid ones",
			fsys:     testFS(),
			args:     []string{"a.txt", "missing.txt", "b.txt"},
			want:     "==> a.txt <==\nalpha\n\n==> b.txt <==\nbeta\n",
			wantErr:  "head: missing.txt: file does not exist\n",
			wantExit: 1,
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
			want:  "==> - <==\nstdin\n\n==> a.txt <==\nalpha\n",
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

func TestRunLastOptionWins(t *testing.T) {
	tests := []testCase{
		{
			name: "bytes after lines",
			fsys: testFS(),
			args: []string{"-n", "2", "-c", "4", "lines.txt"},
			want: "l1\nl",
		},
		{
			name: "lines after bytes",
			fsys: testFS(),
			args: []string{"-c", "4", "-n", "2", "lines.txt"},
			want: "l1\nl2\n",
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
			name: "lines takes its value in the next argument",
			fsys: testFS(),
			args: []string{"--lines", "2", "lines.txt"},
			want: "l1\nl2\n",
		},
		{
			name: "lines takes its value glued to it",
			fsys: testFS(),
			args: []string{"--lines=2", "lines.txt"},
			want: "l1\nl2\n",
		},
		{
			name: "bytes takes its value glued to it",
			fsys: testFS(),
			args: []string{"--bytes=4", "bytes.txt"},
			want: "0123",
		},
		{
			name:     "lines requires an argument",
			fsys:     testFS(),
			args:     []string{"--lines"},
			wantErr:  "head: option '--lines' requires an argument\nTry 'head --help' for more information.\n",
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
			wantErr:  "head: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "zero-length file name",
			fsys:     testFS(),
			args:     []string{""},
			wantErr:  "head: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "read error",
			stdin:    errorReader{err: errors.New("boom")},
			args:     []string{"-"},
			wantErr:  "head: -: boom\n",
			wantExit: 1,
		},
		{
			name:     "no file system available",
			args:     []string{"a.txt"},
			wantErr:  "head: a.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "no standard input available",
			wantErr:  "head: standard input: no standard input available\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunInvalidUsage(t *testing.T) {
	tests := []testCase{
		{
			name:     "invalid option",
			args:     []string{"-x", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: invalid option -- 'x'\nTry 'head --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "unrecognized long option",
			args:     []string{"--number", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: unrecognized option '--number'\nTry 'head --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "invalid number",
			args:     []string{"-n", "x", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: invalid number of lines: 'x'\nTry 'head --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "missing option argument",
			args:     []string{"-n"},
			fsys:     testFS(),
			wantErr:  "head: option requires an argument -- 'n'\nTry 'head --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "non-decimal number",
			args:     []string{"-c", "1.5", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: invalid number of bytes: '1.5'\nTry 'head --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "empty number",
			args:     []string{"-n", "", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: invalid number of lines: ''\nTry 'head --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "invalid suffix",
			args:     []string{"-c", "1J", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: invalid number of bytes: '1J'\nTry 'head --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "suffix too large",
			args:     []string{"-c", "1Z", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: invalid number of bytes: '1Z'\nTry 'head --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "obsolete number after another option",
			args:     []string{"-n", "2", "-5", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: invalid trailing option -- 5\nTry 'head --help' for more information.\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSuffixes(t *testing.T) {
	tests := []testCase{
		{
			name: "a kibi prefix multiplies the byte count",
			fsys: testFS(),
			args: []string{"-c", "1K", "big.txt"},
			want: strings.Repeat("0123456789", 103)[:1024],
		},
		{
			name: "a kilo prefix multiplies the byte count by 1000",
			fsys: testFS(),
			args: []string{"-c", "1KB", "big.txt"},
			want: strings.Repeat("0123456789", 101)[:1000],
		},
		{
			name: "a binary prefix multiplies the byte count",
			fsys: testFS(),
			args: []string{"-c", "10KiB", "big.txt"},
			want: string(testFS()["big.txt"].Data),
		},
		{
			name: "a block suffix counts 512 bytes",
			fsys: testFS(),
			args: []string{"-c", "3b", "big.txt"},
			want: strings.Repeat("0123456789", 154)[:1536],
		},
		{
			name: "suffixes are case insensitive",
			fsys: testFS(),
			args: []string{"-c", "1kb", "big.txt"},
			want: strings.Repeat("0123456789", 101)[:1000],
		},
		{
			name: "a mebi prefix multiplies the byte count",
			fsys: testFS(),
			args: []string{"-c", "2M", "big.txt"},
			want: string(testFS()["big.txt"].Data),
		},
		{
			name: "a suffix multiplies the line count",
			fsys: testFS(),
			args: []string{"-n", "1000c", "nlines.txt"},
			want: string(linesFile(1000)),
		},
		{
			name: "a kibi suffix multiplies the line count",
			fsys: testFS(),
			args: []string{"-n", "1k", "nlines.txt"},
			want: string(linesFile(1024)),
		},
		{
			name: "a suffix on a long option",
			fsys: testFS(),
			args: []string{"--bytes=1K", "big.txt"},
			want: strings.Repeat("0123456789", 103)[:1024],
		},
		{
			name:  "a suffix on the standard input",
			stdin: strings.NewReader(strings.Repeat("x", 2000)),
			args:  []string{"-c", "1K"},
			want:  strings.Repeat("x", 1024),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunNegativeCounts(t *testing.T) {
	tests := []testCase{
		{
			name: "all but the last lines",
			fsys: testFS(),
			args: []string{"-n", "-2", "lines.txt"},
			want: "l1\nl2\nl3\n",
		},
		{
			name: "more lines to elide than the file has",
			fsys: testFS(),
			args: []string{"-n", "-10", "lines.txt"},
			want: "",
		},
		{
			name: "all but the last bytes",
			fsys: testFS(),
			args: []string{"-c", "-4", "lines.txt"},
			want: "l1\nl2\nl3\nl4",
		},
		{
			name: "more bytes to elide than the file has",
			fsys: testFS(),
			args: []string{"-c", "-20", "lines.txt"},
			want: "",
		},
		{
			name: "zero lines to elide copies everything",
			fsys: testFS(),
			args: []string{"-n", "-0", "lines.txt"},
			want: "l1\nl2\nl3\nl4\nl5\n",
		},
		{
			name: "a negative count on a final incomplete line",
			fsys: testFS(),
			args: []string{"-n", "-1", "nonl.txt"},
			want: "one\ntwo\n",
		},
		{
			name:  "a negative count on the standard input",
			stdin: strings.NewReader("a\nb\nc\nd\n"),
			args:  []string{"-n", "-1"},
			want:  "a\nb\nc\n",
		},
		{
			name: "a leading plus is accepted and ignored on lines",
			fsys: testFS(),
			args: []string{"-n", "+2", "lines.txt"},
			want: "l1\nl2\n",
		},
		{
			name: "a leading plus is accepted and ignored on bytes",
			fsys: testFS(),
			args: []string{"-c", "+3", "bytes.txt"},
			want: "012",
		},
		{
			name: "a leading plus joined to the option",
			fsys: testFS(),
			args: []string{"-n+2", "lines.txt"},
			want: "l1\nl2\n",
		},
		{
			name: "a negative count on a long option",
			fsys: testFS(),
			args: []string{"--lines=-2", "lines.txt"},
			want: "l1\nl2\nl3\n",
		},
		{
			name: "a negative count with a suffix",
			fsys: testFS(),
			args: []string{"-c", "-1K", "big.txt"},
			want: strings.Repeat("0123456789", 308)[:3076],
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunObsoleteSyntax(t *testing.T) {
	tests := []testCase{
		{
			name: "a lone number counts lines",
			fsys: testFS(),
			args: []string{"-5", "many.txt"},
			want: "01\n02\n03\n04\n05\n",
		},
		{
			name:  "a lone number on the standard input",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-2"},
			want:  "a\nb\n",
		},
		{
			name: "c counts bytes",
			fsys: testFS(),
			args: []string{"-3c", "bytes.txt"},
			want: "012",
		},
		{
			name: "b counts 512-byte blocks",
			fsys: testFS(),
			args: []string{"-2b", "big.txt"},
			want: strings.Repeat("0123456789", 103)[:1024],
		},
		{
			name: "k counts 1024 bytes",
			fsys: testFS(),
			args: []string{"-1k", "big.txt"},
			want: strings.Repeat("0123456789", 103)[:1024],
		},
		{
			name: "m counts 1024 kibibytes",
			fsys: testFS(),
			args: []string{"-2m", "big.txt"},
			want: string(testFS()["big.txt"].Data),
		},
		{
			name: "l counts lines",
			fsys: testFS(),
			args: []string{"-2l", "lines.txt"},
			want: "l1\nl2\n",
		},
		{
			name: "the last multiplier letter wins",
			fsys: testFS(),
			args: []string{"-2bk", "big.txt"},
			want: strings.Repeat("0123456789", 205)[:2048],
		},
		{
			name: "c resets the multiplier",
			fsys: testFS(),
			args: []string{"-2bc", "bytes.txt"},
			want: "01",
		},
		{
			name: "q never prints headers",
			fsys: testFS(),
			args: []string{"-5q", "a.txt", "b.txt"},
			want: "alpha\nbeta\n",
		},
		{
			name: "v always prints headers",
			fsys: testFS(),
			args: []string{"-5v", "a.txt"},
			want: "==> a.txt <==\nalpha\n",
		},
		{
			name: "z terminates the items with a NUL byte",
			fsys: testFS(),
			args: []string{"-2z", "nul.txt"},
			want: "a\x00b\x00",
		},
		{
			name: "the obsolete form is combined with normal options",
			fsys: testFS(),
			args: []string{"-5", "-c", "3", "bytes.txt"},
			want: "012",
		},
		{
			name:     "an invalid trailing option",
			fsys:     testFS(),
			args:     []string{"-5x", "a.txt"},
			wantErr:  "head: invalid trailing option -- x\nTry 'head --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid trailing option after digits and flags",
			fsys:     testFS(),
			args:     []string{"-5qzK", "a.txt"},
			wantErr:  "head: invalid trailing option -- K\nTry 'head --help' for more information.\n",
			wantExit: 2,
		},
		{
			name: "a number alone is not the obsolete form after --",
			fsys: testFS(),
			args: []string{"--", "-5"},
			want: "dash-5\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunHeaders(t *testing.T) {
	tests := []testCase{
		{
			name: "quiet suppresses the headers",
			fsys: testFS(),
			args: []string{"-q", "a.txt", "b.txt"},
			want: "alpha\nbeta\n",
		},
		{
			name: "quiet on a long form",
			fsys: testFS(),
			args: []string{"--quiet", "a.txt", "b.txt"},
			want: "alpha\nbeta\n",
		},
		{
			name: "silent suppresses the headers",
			fsys: testFS(),
			args: []string{"--silent", "a.txt", "b.txt"},
			want: "alpha\nbeta\n",
		},
		{
			name:     "quiet still reads every file",
			fsys:     testFS(),
			args:     []string{"-q", "a.txt", "missing.txt"},
			want:     "alpha\n",
			wantErr:  "head: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name: "verbose prints a header for a single file",
			fsys: testFS(),
			args: []string{"-v", "a.txt"},
			want: "==> a.txt <==\nalpha\n",
		},
		{
			name:  "verbose prints a header for the standard input",
			stdin: strings.NewReader("stdin\n"),
			args:  []string{"-v"},
			want:  "==> standard input <==\nstdin\n",
		},
		{
			name: "verbose on a long form",
			fsys: testFS(),
			args: []string{"--verbose", "a.txt"},
			want: "==> a.txt <==\nalpha\n",
		},
		{
			name: "the last of quiet and verbose wins",
			fsys: testFS(),
			args: []string{"-q", "-v", "a.txt"},
			want: "==> a.txt <==\nalpha\n",
		},
		{
			name: "the last of verbose and quiet wins",
			fsys: testFS(),
			args: []string{"-v", "-q", "a.txt"},
			want: "alpha\n",
		},
		{
			name: "verbose prints headers with a byte count",
			fsys: testFS(),
			args: []string{"-c", "2", "-v", "a.txt"},
			want: "==> a.txt <==\nal",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunZeroTerminated(t *testing.T) {
	tests := []testCase{
		{
			name: "the first NUL-terminated items",
			fsys: testFS(),
			args: []string{"-z", "-n", "2", "nul.txt"},
			want: "a\x00b\x00",
		},
		{
			name: "a final item without a NUL counts as an item",
			fsys: testFS(),
			args: []string{"-z", "-n", "1", "nulpart.txt"},
			want: "x\x00",
		},
		{
			name: "the final item without a NUL is copied as is",
			fsys: testFS(),
			args: []string{"-z", "-n", "2", "nulpart.txt"},
			want: "x\x00y",
		},
		{
			name: "zero-terminated on a long form",
			fsys: testFS(),
			args: []string{"--zero-terminated", "-n", "1", "nul.txt"},
			want: "a\x00",
		},
		{
			name: "more items than the file has",
			fsys: testFS(),
			args: []string{"-z", "-n", "10", "nul.txt"},
			want: "a\x00b\x00c\x00",
		},
		{
			name: "all but the last NUL-terminated item",
			fsys: testFS(),
			args: []string{"-z", "-n", "-1", "nul.txt"},
			want: "a\x00b\x00",
		},
		{
			name: "bytes are unaffected by zero-termination",
			fsys: testFS(),
			args: []string{"-z", "-c", "3", "nul.txt"},
			want: "a\x00b",
		},
		{
			name: "lines are still newline-terminated without -z",
			fsys: testFS(),
			args: []string{"-n", "1", "nul.txt"},
			want: "a\x00b\x00c\x00",
		},
		{
			name:  "zero-terminated items of the standard input",
			stdin: strings.NewReader("a\x00b\x00c\x00"),
			args:  []string{"-z", "-n", "2"},
			want:  "a\x00b\x00",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunWriteError(t *testing.T) {
	rt := &coreutils.Runtime{
		Stdout: errorWriter{err: errors.New("boom")},
		Stderr: new(bytes.Buffer),
		FS:     testFS(),
	}
	err := New(rt).Run(t.Context(), "a.txt")
	if code := exitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if got, want := rt.Stderr.(*bytes.Buffer).String(), "head: write error: boom\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}

func TestRunContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	rt := &coreutils.Runtime{
		Stdin:  &cancelReader{cancel: cancel},
		Stdout: io.Discard,
		Stderr: io.Discard,
	}
	if err := New(rt).Run(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRunDoesNotReadUnusedStandardInput(t *testing.T) {
	stdin := &spyReader{}
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     testFS(),
	}
	if err := New(rt).Run(t.Context(), "a.txt"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdin.read {
		t.Error("the standard input was read")
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
		want := "head (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
