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
//	skip1.txt:  "1234abcd" (skip2.txt with a prefix)
//	skip2.txt:  "abcd"
//	hi1.txt:    {0xff}
//	hi2.txt:    {0x80}
//	del1.txt:   {0x7f}
//	oct1.txt:   "12345678x"
//	oct2.txt:   "12345678y" (oct1.txt with a difference at byte 9)
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
		"skip1.txt":  {Data: []byte("1234abcd")},
		"skip2.txt":  {Data: []byte("abcd")},
		"hi1.txt":    {Data: []byte{0xff}},
		"hi2.txt":    {Data: []byte{0x80}},
		"del1.txt":   {Data: []byte{0x7f}},
		"oct1.txt":   {Data: []byte("12345678x")},
		"oct2.txt":   {Data: []byte("12345678y")},
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
			name:     "a file with itself and different skips still compares",
			fsys:     testFS(),
			args:     []string{"a.txt", "a.txt", "1"},
			want:     "a.txt a.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "the first difference is reported",
			fsys:     testFS(),
			args:     []string{"a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "a difference in the first byte is on line 1",
			fsys:     testFS(),
			args:     []string{"a.txt", "d.txt"},
			want:     "a.txt d.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "binary files are compared byte by byte",
			fsys:     testFS(),
			args:     []string{"bin1.txt", "bin2.txt"},
			want:     "bin1.txt bin2.txt differ: byte 3, line 1\n",
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
			want:     "- a.txt differ: byte 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "the second file can be the standard input",
			fsys:     testFS(),
			stdin:    strings.NewReader("xb\ncd\n"),
			args:     []string{"a.txt", "-"},
			want:     "a.txt - differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:  "a missing FILE2 is the standard input",
			fsys:  testFS(),
			stdin: strings.NewReader("ab\ncd\n"),
			args:  []string{"a.txt"},
		},
		{
			name:     "the missing FILE2 is reported as '-'",
			fsys:     testFS(),
			stdin:    strings.NewReader("ab\ncx\n"),
			args:     []string{"a.txt"},
			want:     "a.txt - differ: byte 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "the standard input as FILE2 can be the shorter file",
			fsys:     testFS(),
			stdin:    strings.NewReader("ab\n"),
			args:     []string{"a.txt"},
			wantErr:  "cmp: EOF on '-' after byte 3, line 1\n",
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
			want:     "a.txt b.txt differ: byte 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "operands resolve against the working directory",
			fsys:     testFS(),
			dir:      "sub",
			args:     []string{"a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "a difference beyond the first block is reported",
			fsys:     big,
			args:     []string{"big1.txt", "big2.txt"},
			want:     "big1.txt big2.txt differ: byte 40001, line 1\n",
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
			wantErr:  "cmp: options -l and -s are incompatible\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "-s and -l are incompatible",
			fsys:     testFS(),
			args:     []string{"-s", "-l", "a.txt", "b.txt"},
			wantErr:  "cmp: options -l and -s are incompatible\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "-l and -s grouped are incompatible",
			fsys:     testFS(),
			args:     []string{"-ls", "a.txt", "b.txt"},
			wantErr:  "cmp: options -l and -s are incompatible\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunPrintBytes(t *testing.T) {
	tests := []testCase{
		{
			name:     "-b appends the differing bytes to the message",
			fsys:     testFS(),
			args:     []string{"-b", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 5, line 2 is 144 d 170 x\n",
			wantExit: 1,
		},
		{
			name:     "-b quotes control bytes",
			fsys:     testFS(),
			args:     []string{"-b", "bin1.txt", "bin2.txt"},
			want:     "bin1.txt bin2.txt differ: byte 3, line 1 is   7 ^G  12 ^J\n",
			wantExit: 1,
		},
		{
			name:     "-b quotes the delete byte",
			fsys:     testFS(),
			args:     []string{"-b", "del1.txt", "a.txt"},
			want:     "del1.txt a.txt differ: byte 1, line 1 is 177 ^? 141 a\n",
			wantExit: 1,
		},
		{
			name:     "-b quotes meta bytes",
			fsys:     testFS(),
			args:     []string{"-b", "hi1.txt", "hi2.txt"},
			want:     "hi1.txt hi2.txt differ: byte 1, line 1 is 377 M-^? 200 M-^@\n",
			wantExit: 1,
		},
		{
			name:     "--print-bytes stands for -b",
			fsys:     testFS(),
			args:     []string{"--print-bytes", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 5, line 2 is 144 d 170 x\n",
			wantExit: 1,
		},
		{
			name:     "-b can be repeated",
			fsys:     testFS(),
			args:     []string{"-bb", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 5, line 2 is 144 d 170 x\n",
			wantExit: 1,
		},
		{
			name:     "-lb lists the bytes of every difference",
			fsys:     testFS(),
			args:     []string{"-lb", "multi1.txt", "multi2.txt"},
			want:     "1 141 a    170 x\n4 141 a    171 y\n6 143 c    172 z\n",
			wantExit: 1,
		},
		{
			name:     "-lb quotes the bytes of a difference",
			fsys:     testFS(),
			args:     []string{"-lb", "bin1.txt", "bin2.txt"},
			want:     "3   7 ^G    12 ^J\n",
			wantExit: 1,
		},
		{
			name:     "-lb quotes meta bytes without padding them",
			fsys:     testFS(),
			args:     []string{"-lb", "hi1.txt", "hi2.txt"},
			want:     "1 377 M-^? 200 M-^@\n",
			wantExit: 1,
		},
		{
			name:     "-b keeps the EOF diagnostic of a shorter file",
			fsys:     testFS(),
			args:     []string{"-b", "a.txt", "c.txt"},
			wantErr:  "cmp: EOF on 'c.txt' after byte 3, line 1\n",
			wantExit: 1,
		},
		{
			name:     "-b is silent with -s",
			fsys:     testFS(),
			args:     []string{"-sb", "a.txt", "b.txt"},
			wantExit: 1,
		},
		{
			name:     "-b reads the standard input",
			fsys:     testFS(),
			stdin:    strings.NewReader("ab\ncx\n"),
			args:     []string{"-b", "a.txt"},
			want:     "a.txt - differ: byte 5, line 2 is 144 d 170 x\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunIgnoreInitial(t *testing.T) {
	tests := []testCase{
		{
			name:     "-i skips the first bytes of both files",
			fsys:     testFS(),
			args:     []string{"-i", "4", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "-i resets the byte and line numbering",
			fsys:     testFS(),
			args:     []string{"-i", "1", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 4, line 2\n",
			wantExit: 1,
		},
		{
			name:     "-i with a count for each file",
			fsys:     testFS(),
			args:     []string{"-i", "1:3", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "--ignore-initial stands for -i",
			fsys:     testFS(),
			args:     []string{"--ignore-initial=4", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "--ignore-initial takes its argument separately",
			fsys:     testFS(),
			args:     []string{"--ignore-initial", "4", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "the skip can be grouped with other options",
			fsys:     testFS(),
			args:     []string{"-li3", "multi1.txt", "multi2.txt"},
			want:     "1 141 171\n3 143 172\n",
			wantExit: 1,
		},
		{
			name:     "the skip counts restart the -l numbering",
			fsys:     testFS(),
			args:     []string{"-l", "-i", "3", "multi1.txt", "multi2.txt"},
			want:     "1 141 171\n3 143 172\n",
			wantExit: 1,
		},
		{
			name:     "the skip can be hexadecimal",
			fsys:     testFS(),
			args:     []string{"-i", "0x4", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name: "the skip can be octal",
			fsys: testFS(),
			args: []string{"-i", "010", "a.txt", "b.txt"},
		},
		{
			name: "the skip can have a suffix",
			fsys: testFS(),
			args: []string{"-i", "1K", "a.txt", "b.txt"},
		},
		{
			name: "a skip beyond both files compares nothing",
			fsys: testFS(),
			args: []string{"-i", "100", "a.txt", "b.txt"},
		},
		{
			name:     "a skip beyond one file only reports it as empty",
			fsys:     testFS(),
			args:     []string{"-i", "100:0", "a.txt", "b.txt"},
			wantErr:  "cmp: EOF on 'a.txt' which is empty\n",
			wantExit: 1,
		},
		{
			name:     "repeated -i keeps the largest count",
			fsys:     testFS(),
			args:     []string{"-i", "4", "-i", "2", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "a single count of repeated -i applies to both files",
			fsys:     testFS(),
			args:     []string{"-i", "1:3", "-i", "2", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "-s keeps the status of a skipped difference",
			fsys:     testFS(),
			args:     []string{"-si", "4", "a.txt", "b.txt"},
			wantExit: 1,
		},
		{
			name: "-s keeps the identical status beyond a difference",
			fsys: testFS(),
			args: []string{"-si", "5", "a.txt", "b.txt"},
		},
		{
			name:     "-i reads the standard input",
			fsys:     testFS(),
			stdin:    strings.NewReader("xxabcd"),
			args:     []string{"-i", "2", "skip1.txt"},
			want:     "skip1.txt - differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "-i with a skip on the standard input only",
			fsys:     testFS(),
			stdin:    strings.NewReader("xxabcd"),
			args:     []string{"-i", "2:0", "-", "skip1.txt"},
			want:     "- skip1.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSkipOperands(t *testing.T) {
	tests := []testCase{
		{
			name:     "a SKIP1 operand skips FILE1 only",
			fsys:     testFS(),
			args:     []string{"skip1.txt", "skip2.txt", "4"},
			wantExit: 0,
		},
		{
			name:     "SKIP1 and SKIP2 skip each file",
			fsys:     testFS(),
			args:     []string{"skip1.txt", "skip2.txt", "4", "4"},
			wantErr:  "cmp: EOF on 'skip2.txt' which is empty\n",
			wantExit: 1,
		},
		{
			name:     "a skip beyond one file reports it as empty",
			fsys:     testFS(),
			args:     []string{"a.txt", "b.txt", "100"},
			wantErr:  "cmp: EOF on 'a.txt' which is empty\n",
			wantExit: 1,
		},
		{
			name:     "the skip operands combine with -i, keeping the largest",
			fsys:     testFS(),
			args:     []string{"-i", "2", "a.txt", "b.txt", "5"},
			wantErr:  "cmp: EOF on 'a.txt' after byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "the skip operand can be hexadecimal",
			fsys:     testFS(),
			args:     []string{"a.txt", "b.txt", "0x4"},
			want:     "a.txt b.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "the skip operands work with -l",
			fsys:     testFS(),
			args:     []string{"-l", "multi1.txt", "multi2.txt", "3"},
			want:     "1 141 170\n",
			wantErr:  "cmp: EOF on 'multi1.txt' after byte 3\n",
			wantExit: 1,
		},
		{
			name:     "the skip operands are accepted without a warning",
			fsys:     testFS(),
			args:     []string{"a.txt", "b.txt", "4", "4"},
			want:     "a.txt b.txt differ: byte 1, line 1\n",
			wantExit: 1,
		},
		{
			name:     "the skip operands read the standard input",
			fsys:     testFS(),
			stdin:    strings.NewReader("xx1234abcd"),
			args:     []string{"-", "skip2.txt", "2", "0"},
			want:     "- skip2.txt differ: byte 1, line 1\n",
			wantExit: 1,
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
			want:     "a.txt b.txt differ: byte 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "a limit beyond the files still reports the difference",
			fsys:     testFS(),
			args:     []string{"-n", "100", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 5, line 2\n",
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
			want:     "big1.txt big2.txt differ: byte 40001, line 1\n",
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
			want:     "a.txt b.txt differ: byte 5, line 2\n",
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
			want:     "a.txt b.txt differ: byte 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "--bytes takes its argument separately",
			fsys:     testFS(),
			args:     []string{"--bytes", "5", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "the limit can be hexadecimal",
			fsys:     testFS(),
			args:     []string{"-n", "0x5", "a.txt", "b.txt"},
			want:     "a.txt b.txt differ: byte 5, line 2\n",
			wantExit: 1,
		},
		{
			name:     "the limit can be octal",
			fsys:     testFS(),
			args:     []string{"-n", "011", "oct1.txt", "oct2.txt"},
			want:     "oct1.txt oct2.txt differ: byte 9, line 1\n",
			wantExit: 1,
		},
		{
			name: "an octal limit counts the leading zero",
			fsys: testFS(),
			args: []string{"-n", "010", "oct1.txt", "oct2.txt"},
		},
		{
			name:     "the limit can have a binary suffix",
			fsys:     testFS(),
			args:     []string{"-n", "1K", "oct1.txt", "oct2.txt"},
			want:     "oct1.txt oct2.txt differ: byte 9, line 1\n",
			wantExit: 1,
		},
		{
			name:     "the limit can have a decimal suffix",
			fsys:     testFS(),
			args:     []string{"-n", "1kB", "oct1.txt", "oct2.txt"},
			want:     "oct1.txt oct2.txt differ: byte 9, line 1\n",
			wantExit: 1,
		},
		{
			name:     "the limit can have an explicit binary suffix",
			fsys:     testFS(),
			args:     []string{"-n", "1KiB", "oct1.txt", "oct2.txt"},
			want:     "oct1.txt oct2.txt differ: byte 9, line 1\n",
			wantExit: 1,
		},
		{
			name:     "the limit can be signed with a plus",
			fsys:     testFS(),
			args:     []string{"-n", "+9", "oct1.txt", "oct2.txt"},
			want:     "oct1.txt oct2.txt differ: byte 9, line 1\n",
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
			wantErr:  "cmp: missing operand\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "the standard input may only be specified once",
			fsys:     testFS(),
			stdin:    strings.NewReader("ab\ncd\n"),
			args:     []string{"-", "-"},
			wantErr:  "cmp: the standard input may only be specified once\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a missing FILE2 makes the standard input twice",
			fsys:     testFS(),
			stdin:    strings.NewReader("ab\ncd\n"),
			args:     []string{"-"},
			wantErr:  "cmp: the standard input may only be specified once\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "the standard input is rejected twice with skips",
			fsys:     testFS(),
			stdin:    strings.NewReader("ab\ncd\n"),
			args:     []string{"-", "-", "1", "2"},
			wantErr:  "cmp: the standard input may only be specified once\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an extra operand",
			fsys:     testFS(),
			args:     []string{"a.txt", "b.txt", "1", "2", "third.txt"},
			wantErr:  "cmp: extra operand 'third.txt'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid option",
			fsys:     testFS(),
			args:     []string{"-x", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid option -- 'x'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an unsupported long option",
			fsys:     testFS(),
			args:     []string{"--initial", "a.txt", "b.txt"},
			wantErr:  "cmp: unrecognized option '--initial'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid limit",
			fsys:     testFS(),
			args:     []string{"-n", "x", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --bytes value 'x'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a negative limit",
			fsys:     testFS(),
			args:     []string{"-n", "-1", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --bytes value '-1'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an empty limit",
			fsys:     testFS(),
			args:     []string{"--bytes=", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --bytes value ''\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a limit with an unsupported suffix",
			fsys:     testFS(),
			args:     []string{"-n", "1b", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --bytes value '1b'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a limit with trailing junk",
			fsys:     testFS(),
			args:     []string{"-n", "5x", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --bytes value '5x'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a hexadecimal limit without digits",
			fsys:     testFS(),
			args:     []string{"-n", "0x", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --bytes value '0x'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a number with a non-octal digit after a zero",
			fsys:     testFS(),
			args:     []string{"-n", "09", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --bytes value '09'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid skip value",
			fsys:     testFS(),
			args:     []string{"-i", "x", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --ignore-initial value 'x'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a negative skip value",
			fsys:     testFS(),
			args:     []string{"-i", "-1", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --ignore-initial value '-1'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an empty skip value",
			fsys:     testFS(),
			args:     []string{"--ignore-initial=", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --ignore-initial value ''\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a skip value ending with a colon",
			fsys:     testFS(),
			args:     []string{"-i", "4:", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --ignore-initial value ''\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a skip value starting with a colon",
			fsys:     testFS(),
			args:     []string{"-i", ":4", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --ignore-initial value ':4'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a skip value with an invalid second count",
			fsys:     testFS(),
			args:     []string{"-i", "4:x", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --ignore-initial value 'x'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a skip value with two colons",
			fsys:     testFS(),
			args:     []string{"-i", "1:1:1", "a.txt", "b.txt"},
			wantErr:  "cmp: invalid --ignore-initial value '1:1'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid SKIP1 operand",
			fsys:     testFS(),
			args:     []string{"a.txt", "b.txt", "x"},
			wantErr:  "cmp: invalid --ignore-initial value 'x'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid SKIP2 operand",
			fsys:     testFS(),
			args:     []string{"a.txt", "b.txt", "1", "x"},
			wantErr:  "cmp: invalid --ignore-initial value 'x'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "-i without an argument",
			fsys:     testFS(),
			args:     []string{"-i"},
			wantErr:  "cmp: option requires an argument -- 'i'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "--ignore-initial without an argument",
			fsys:     testFS(),
			args:     []string{"--ignore-initial"},
			wantErr:  "cmp: option '--ignore-initial' requires an argument\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "--print-bytes doesn't take an argument",
			fsys:     testFS(),
			args:     []string{"--print-bytes=1", "a.txt", "b.txt"},
			wantErr:  "cmp: option '--print-bytes=1' doesn't allow an argument\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "-n without an argument",
			fsys:     testFS(),
			args:     []string{"-n"},
			wantErr:  "cmp: option requires an argument -- 'n'\nTry 'cmp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "--bytes without an argument",
			fsys:     testFS(),
			args:     []string{"--bytes"},
			wantErr:  "cmp: option '--bytes' requires an argument\nTry 'cmp --help' for more information.\n",
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

	t.Run("short version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "-v"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "cmp (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("short version wins over the operands", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "-v", "a.txt"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "cmp (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
