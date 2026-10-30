package cmp

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
//	a.txt:      "ab\ncd\n"
//	b.txt:      "ab\ncx\n" (a.txt with a difference at byte 5, line 2)
//	c.txt:      "ab\n" (a prefix of a.txt)
//	nonl.txt:   "ab\ncd" (a prefix of a.txt, without trailing newline)
//	d.txt:      "xb\ncd\n" (a.txt with a difference at byte 1)
//	same1.txt:  "hello\nworld\n"
//	same2.txt:  "hello\nworld\n"
//	empty.txt:  ""
//	bin1.txt:   {0x01, 'A', 0x07}
//	bin2.txt:   {0x01, 'A', 0x0a}
//	multi1.txt: "abcabc"
//	multi2.txt: "xbcybz"
//	sub/a.txt:  "z\n"
//	sub/b.txt:  "y\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":      {Data: []byte("ab\ncd\n")},
		"b.txt":      {Data: []byte("ab\ncx\n")},
		"c.txt":      {Data: []byte("ab\n")},
		"nonl.txt":   {Data: []byte("ab\ncd")},
		"d.txt":      {Data: []byte("xb\ncd\n")},
		"same1.txt":  {Data: []byte("hello\nworld\n")},
		"same2.txt":  {Data: []byte("hello\nworld\n")},
		"empty.txt":  {Data: []byte("")},
		"bin1.txt":   {Data: []byte{0x01, 'A', 0x07}},
		"bin2.txt":   {Data: []byte{0x01, 'A', 0x0a}},
		"multi1.txt": {Data: []byte("abcabc")},
		"multi2.txt": {Data: []byte("xbcybz")},
		"sub/a.txt":  {Data: []byte("z\n")},
		"sub/b.txt":  {Data: []byte("y\n")},
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
	fsys     fsx.MemFS
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
		WorkingDir: test.dir,
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

func TestRunDefaults(t *testing.T) {
	big := fsx.MemFS{
		"big1.txt": {Data: []byte(strings.Repeat("a", 40000) + "b")},
		"big2.txt": {Data: []byte(strings.Repeat("a", 40000) + "c")},
		"big0.txt": {Data: []byte(strings.Repeat("a", 40000))},
	}

	tests := []testCase{
		{
			name: "identical files produce no output",
			fsys: testFS(),
			args: []string{"same1.txt", "same2.txt"},
		},
		{
			name: "a file compares identical to itself",
			fsys: testFS(),
			args: []string{"a.txt", "a.txt"},
		},
		{
			name:     "the first difference is reported",
			fsys:     testFS(),
			args:     []string{"a.txt", "b.txt"},
			want:     "a.txt b.txt differ: char 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "a difference in the first byte is on line 1",
			fsys:     testFS(),
			args:     []string{"a.txt", "d.txt"},
			want:     "a.txt d.txt differ: char 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "binary files are compared byte by byte",
			fsys:     testFS(),
			args:     []string{"bin1.txt", "bin2.txt"},
			want:     "bin1.txt bin2.txt differ: char 3, line 1\n",
			wantExit: 1,
		},
		{
			name:     "a shorter file ending after a newline reports the line",
			fsys:     testFS(),
			args:     []string{"a.txt", "c.txt"},
			wantErr:  "cmp: EOF on 'c.txt' after byte 3, line 1\n",
			wantExit: 1,
		},
		{
			name:     "a shorter file ending inside a line reports being in the line",
			fsys:     testFS(),
			args:     []string{"a.txt", "nonl.txt"},
			wantErr:  "cmp: EOF on 'nonl.txt' after byte 5, in line 2\n",
			wantExit: 1,
		},
		{
			name:     "an empty first file",
			fsys:     testFS(),
			args:     []string{"empty.txt", "a.txt"},
			wantErr:  "cmp: EOF on 'empty.txt' which is empty\n",
			wantExit: 1,
		},
		{
			name:     "an empty second file",
			fsys:     testFS(),
			args:     []string{"a.txt", "empty.txt"},
			wantErr:  "cmp: EOF on 'empty.txt' which is empty\n",
			wantExit: 1,
		},
		{
			name: "two empty files are identical",
			fsys: testFS(),
			args: []string{"empty.txt", "empty.txt"},
		},
		{
			name:     "the first file can be the standard input",
			fsys:     testFS(),
			stdin:    strings.NewReader("ab\ncx\n"),
			args:     []string{"-", "a.txt"},
			want:     "- a.txt differ: char 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "the second file can be the standard input",
			fsys:     testFS(),
			stdin:    strings.NewReader("xb\ncd\n"),
			args:     []string{"a.txt", "-"},
			want:     "a.txt - differ: char 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "the standard input is the shorter file",
			fsys:     testFS(),
			stdin:    strings.NewReader("ab\n"),
			args:     []string{"-", "a.txt"},
			wantErr:  "cmp: EOF on '-' after byte 3, line 1\n",
			wantExit: 1,
		},
		{
			name:     "the standard input is the longer file",
			fsys:     testFS(),
			stdin:    strings.NewReader("ab\ncd\nextra\n"),
			args:     []string{"-", "a.txt"},
			wantErr:  "cmp: EOF on 'a.txt' after byte 6, line 2\n",
			wantExit: 1,
		},
		{
			name:     "a double dash ends the options",
			fsys:     testFS(),
			args:     []string{"--", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: char 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "operands resolve against the working directory",
			fsys:     testFS(),
			dir:      "sub",
			args:     []string{"a.txt", "b.txt"},
			want:     "a.txt b.txt differ: char 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "a difference beyond the first block is reported",
			fsys:     big,
			args:     []string{"big1.txt", "big2.txt"},
			want:     "big1.txt big2.txt differ: char 40001, line 1\n",
			wantExit: 1,
		},
		{
			name: "identical files bigger than a block",
			fsys: big,
			args: []string{"big1.txt", "big1.txt"},
		},
		{
			name:     "a shorter file bigger than a block",
			fsys:     big,
			args:     []string{"big1.txt", "big0.txt"},
			wantErr:  "cmp: EOF on 'big0.txt' after byte 40000, in line 1\n",
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
			name: "-l lists every differing byte in octal",
			fsys: testFS(),
			args: []string{"-l", "bin1.txt", "bin2.txt"},
			// The byte number and values are unpadded, as POSIX specifies.
			want:     "3 7 12\n",
			wantExit: 1,
		},
		{
			name:     "-l lists multiple differences",
			fsys:     testFS(),
			args:     []string{"-l", "multi1.txt", "multi2.txt"},
			want:     "1 141 170\n4 141 171\n6 143 172\n",
			wantExit: 1,
		},
		{
			name: "-l reports identical files with no output",
			fsys: testFS(),
			args: []string{"-l", "same1.txt", "same2.txt"},
		},
		{
			name:     "-l reports the end of a shorter file without a line number",
			fsys:     testFS(),
			args:     []string{"-l", "a.txt", "c.txt"},
			wantErr:  "cmp: EOF on 'c.txt' after byte 3\n",
			wantExit: 1,
		},
		{
			name:     "-l reports an empty file",
			fsys:     testFS(),
			args:     []string{"-l", "a.txt", "empty.txt"},
			wantErr:  "cmp: EOF on 'empty.txt' which is empty\n",
			wantExit: 1,
		},
		{
			name:     "-l reads the standard input",
			fsys:     testFS(),
			stdin:    strings.NewReader("ab\ncx\n"),
			args:     []string{"-l", "-", "a.txt"},
			want:     "5 170 144\n",
			wantExit: 1,
		},
		{
			name:     "--verbose stands for -l",
			fsys:     testFS(),
			args:     []string{"--verbose", "bin1.txt", "bin2.txt"},
			want:     "3 7 12\n",
			wantExit: 1,
		},
		{
			name:     "-s suppresses the difference message",
			fsys:     testFS(),
			args:     []string{"-s", "a.txt", "b.txt"},
			wantExit: 1,
		},
		{
			name: "-s keeps the identical status",
			fsys: testFS(),
			args: []string{"-s", "same1.txt", "same2.txt"},
		},
		{
			name:     "-s suppresses the end of a shorter file",
			fsys:     testFS(),
			args:     []string{"-s", "a.txt", "c.txt"},
			wantExit: 1,
		},
		{
			name:     "--silent stands for -s",
			fsys:     testFS(),
			args:     []string{"--silent", "a.txt", "b.txt"},
			wantExit: 1,
		},
		{
			name:     "--quiet stands for -s",
			fsys:     testFS(),
			args:     []string{"--quiet", "a.txt", "b.txt"},
			wantExit: 1,
		},
		{
			name:     "an option can be repeated",
			fsys:     testFS(),
			args:     []string{"-s", "-s", "a.txt", "b.txt"},
			wantExit: 1,
		},
		{
			name:     "-l can be repeated",
			fsys:     testFS(),
			args:     []string{"-l", "-l", "bin1.txt", "bin2.txt"},
			want:     "3 7 12\n",
			wantExit: 1,
		},
		{
			name:     "-l and -s are incompatible",
			fsys:     testFS(),
			args:     []string{"-l", "-s", "a.txt", "b.txt"},
			wantErr:  "cmp: options -l and -s are incompatible\n",
			wantExit: 2,
		},
		{
			name:     "-s and -l are incompatible",
			fsys:     testFS(),
			args:     []string{"-s", "-l", "a.txt", "b.txt"},
			wantErr:  "cmp: options -l and -s are incompatible\n",
			wantExit: 2,
		},
		{
			name:     "-l and -s grouped are incompatible",
			fsys:     testFS(),
			args:     []string{"-ls", "a.txt", "b.txt"},
			wantErr:  "cmp: options -l and -s are incompatible\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunBytesLimit(t *testing.T) {
	big := fsx.MemFS{
		"big1.txt": {Data: []byte(strings.Repeat("a", 40000) + "b")},
		"big2.txt": {Data: []byte(strings.Repeat("a", 40000) + "c")},
		"big0.txt": {Data: []byte(strings.Repeat("a", 40000))},
	}

	tests := []testCase{
		{
			name:     "a difference within the limit is reported",
			fsys:     testFS(),
			args:     []string{"-n", "5", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: char 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "a limit beyond the files still reports the difference",
			fsys:     testFS(),
			args:     []string{"-n", "100", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: char 5, line 2\n",
			wantExit: 1,
		},
		{
			name: "a difference beyond the limit is not reported",
			fsys: testFS(),
			args: []string{"-n", "4", "a.txt", "b.txt"},
		},
		{
			name: "identical files compare equal within the limit",
			fsys: testFS(),
			args: []string{"-n", "3", "same1.txt", "same2.txt"},
		},
		{
			name: "a zero limit compares nothing",
			fsys: testFS(),
			args: []string{"-n", "0", "a.txt", "b.txt"},
		},
		{
			name:     "an empty file is reported within the limit",
			fsys:     testFS(),
			args:     []string{"-n", "3", "empty.txt", "a.txt"},
			wantErr:  "cmp: EOF on 'empty.txt' which is empty\n",
			wantExit: 1,
		},
		{
			name:     "a prefix ending within the limit is reported",
			fsys:     testFS(),
			args:     []string{"-n", "5", "a.txt", "c.txt"},
			wantErr:  "cmp: EOF on 'c.txt' after byte 3, line 1\n",
			wantExit: 1,
		},
		{
			name: "a prefix ending beyond the limit is not reported",
			fsys: testFS(),
			args: []string{"-n", "2", "a.txt", "c.txt"},
		},
		{
			name: "a file ending exactly at the limit is not reported",
			fsys: testFS(),
			args: []string{"-n", "3", "a.txt", "c.txt"},
		},
		{
			name:     "a difference after a block within the limit is reported",
			fsys:     big,
			args:     []string{"-n", "40001", "big1.txt", "big2.txt"},
			want:     "big1.txt big2.txt differ: char 40001, line 1\n",
			wantExit: 1,
		},
		{
			name: "a difference after a block beyond the limit is not reported",
			fsys: big,
			args: []string{"-n", "40000", "big1.txt", "big2.txt"},
		},
		{
			name:     "a prefix after a block within the limit is reported",
			fsys:     big,
			args:     []string{"-n", "40001", "big1.txt", "big0.txt"},
			wantErr:  "cmp: EOF on 'big0.txt' after byte 40000, in line 1\n",
			wantExit: 1,
		},
		{
			name:     "-l stops listing differences after the limit",
			fsys:     testFS(),
			args:     []string{"-l", "-n", "3", "multi1.txt", "multi2.txt"},
			want:     "1 141 170\n",
			wantExit: 1,
		},
		{
			name:     "-l lists every difference within the limit",
			fsys:     testFS(),
			args:     []string{"-l", "-n", "5", "multi1.txt", "multi2.txt"},
			want:     "1 141 170\n4 141 171\n",
			wantExit: 1,
		},
		{
			name:     "-l with a limit beyond the files",
			fsys:     testFS(),
			args:     []string{"-l", "-n", "10", "multi1.txt", "multi2.txt"},
			want:     "1 141 170\n4 141 171\n6 143 172\n",
			wantExit: 1,
		},
		{
			name:     "-l reports the end of a prefix within the limit",
			fsys:     testFS(),
			args:     []string{"-l", "-n", "5", "a.txt", "c.txt"},
			wantErr:  "cmp: EOF on 'c.txt' after byte 3\n",
			wantExit: 1,
		},
		{
			name:     "-s keeps the status of a difference within the limit",
			fsys:     testFS(),
			args:     []string{"-s", "-n", "6", "a.txt", "b.txt"},
			wantExit: 1,
		},
		{
			name: "-s keeps the identical status beyond a difference",
			fsys: testFS(),
			args: []string{"-s", "-n", "4", "a.txt", "b.txt"},
		},
		{
			name:     "the limit can be attached",
			fsys:     testFS(),
			args:     []string{"-n5", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: char 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "the limit can be grouped with other options",
			fsys:     testFS(),
			args:     []string{"-ln3", "multi1.txt", "multi2.txt"},
			want:     "1 141 170\n",
			wantExit: 1,
		},
		{
			name:     "--bytes stands for -n",
			fsys:     testFS(),
			args:     []string{"--bytes=5", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: char 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "--bytes takes its argument separately",
			fsys:     testFS(),
			args:     []string{"--bytes", "5", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: char 5, line 2\n",
			wantExit: 1,
		},
		{
			name: "the least of repeated limits is used",
			fsys: testFS(),
			args: []string{"-n", "5", "-n", "4", "a.txt", "b.txt"},
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
			name:     "no operands",
			fsys:     testFS(),
			wantErr:  "cmp: missing operand\n",
			wantExit: 2,
		},
		{
			name:     "one operand",
			fsys:     testFS(),
			args:     []string{"a.txt"},
			wantErr:  "cmp: missing operand\n",
			wantExit: 2,
		},
		{
			name:     "an extra operand",
			fsys:     testFS(),
			args:     []string{"a.txt", "b.txt", "third.txt"},
			wantErr:  "cmp: extra operand 'third.txt'\n",
			wantExit: 2,
		},
		{
			name:     "an invalid option",
			fsys:     testFS(),
			args:     []string{"-x", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an unsupported long option",
			fsys:     testFS(),
			args:     []string{"--initial", "a.txt", "b.txt"},
			wantErr:  "cmp: unrecognized option '--initial'\n",
			wantExit: 2,
		},
		{
			name:     "an invalid limit",
			fsys:     testFS(),
			args:     []string{"-n", "x", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --bytes value 'x'\n",
			wantExit: 2,
		},
		{
			name:     "a negative limit",
			fsys:     testFS(),
			args:     []string{"-n", "-1", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --bytes value '-1'\n",
			wantExit: 2,
		},
		{
			name:     "an empty limit",
			fsys:     testFS(),
			args:     []string{"--bytes=", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --bytes value ''\n",
			wantExit: 2,
		},
		{
			name:     "-n without an argument",
			fsys:     testFS(),
			args:     []string{"-n"},
			wantErr:  "cmp: option requires an argument -- 'n'\n",
			wantExit: 2,
		},
		{
			name:     "--bytes without an argument",
			fsys:     testFS(),
			args:     []string{"--bytes"},
			wantErr:  "cmp: option '--bytes' requires an argument\n",
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
			name:     "missing first file",
			fsys:     testFS(),
			args:     []string{"missing.txt", "a.txt"},
			wantErr:  "cmp: missing.txt: file does not exist\n",
			wantExit: 2,
		},
		{
			name:     "missing second file",
			fsys:     testFS(),
			args:     []string{"a.txt", "missing.txt"},
			wantErr:  "cmp: missing.txt: file does not exist\n",
			wantExit: 2,
		},
		{
			name:     "the first missing file is reported when both are missing",
			fsys:     testFS(),
			args:     []string{"missing1.txt", "missing2.txt"},
			wantErr:  "cmp: missing1.txt: file does not exist\n",
			wantExit: 2,
		},
		{
			name:     "directory input",
			fsys:     fsx.MemFS{"dir": {Mode: fs.ModeDir}, "a.txt": {Data: []byte("a\n")}},
			args:     []string{"dir", "a.txt"},
			wantErr:  "cmp: dir: invalid argument\n",
			wantExit: 2,
		},
		{
			name:     "input file without a file system",
			args:     []string{"a.txt", "b.txt"},
			wantErr:  "cmp: a.txt: no file system available\n",
			wantExit: 2,
		},
		{
			name:     "no standard input",
			fsys:     testFS(),
			args:     []string{"-", "a.txt"},
			wantErr:  "cmp: -: no standard input available\n",
			wantExit: 2,
		},
		{
			name:     "standard input read error",
			fsys:     testFS(),
			stdin:    errorReader{err: errors.New("read failed")},
			args:     []string{"-", "a.txt"},
			wantErr:  "cmp: -: read failed\n",
			wantExit: 2,
		},
		{
			name:     "-s suppresses the message of an opening error",
			fsys:     testFS(),
			args:     []string{"-s", "missing.txt", "a.txt"},
			wantExit: 2,
		},
		{
			name:     "-s doesn't suppress read errors",
			fsys:     testFS(),
			stdin:    errorReader{err: errors.New("read failed")},
			args:     []string{"-s", "-", "a.txt"},
			wantErr:  "cmp: -: read failed\n",
			wantExit: 2,
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
		Stdout: errorWriter{err: writeErr},
		Stderr: stderr,
		FS:     testFS(),
	}
	err := New(rt).Run(t.Context(), "a.txt", "b.txt")

	if code := exitCode(t, err); code != 2 {
		t.Fatalf("Run() exit code = %d, want 2 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
	if got := stderr.String(); got != "cmp: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "cmp: write error: write failed\n")
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
		FS:     fsx.MemFS{"a.txt": {Data: []byte("hello\n")}},
	}
	err := New(rt).Run(ctx, "-", "a.txt")

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
		want := "cmp (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
