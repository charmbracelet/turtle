package cut

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
//	a.txt:         "abcdefg\nxyz\n"
//	passwd.txt:    passwd-like lines with ":" fields
//	tabs.txt:      lines with tab-separated fields, and a plain one
//	utf8.txt:      "ábçde\n日本語x\n" (multi-byte characters)
//	invalid.txt:   "a\xffb\n" (an invalid UTF-8 byte)
//	empty.txt:     ""
//	nonl.txt:      "no newline" (no trailing newline)
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":         {Data: []byte("abcdefg\nxyz\n")},
		"passwd.txt":    {Data: []byte("root:x:0:0:root:/root:/bin/sh\ndaemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin\n")},
		"tabs.txt":      {Data: []byte("f1\tf2\tf3\nplain\n")},
		"utf8.txt":      {Data: []byte("ábçde\n日本語x\n")},
		"invalid.txt":   {Data: []byte{'a', 0xff, 'b', '\n'}},
		"empty.txt":     {Data: []byte("")},
		"nonl.txt":      {Data: []byte("no newline")},
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

func TestRunBytes(t *testing.T) {
	tests := []testCase{
		{
			name:  "a single byte",
			stdin: strings.NewReader("abcdefg\nxyz\n"),
			args:  []string{"-b", "1"},
			want:  "a\nx\n",
		},
		{
			name:  "a list of bytes",
			stdin: strings.NewReader("abcdefg\n"),
			args:  []string{"-b", "1,3,5"},
			want:  "ace\n",
		},
		{
			name:  "a range of bytes",
			stdin: strings.NewReader("abcdefg\n"),
			args:  []string{"-b", "2-4"},
			want:  "bcd\n",
		},
		{
			name:  "a range from the start",
			stdin: strings.NewReader("abcdefg\n"),
			args:  []string{"-b", "-3"},
			want:  "abc\n",
		},
		{
			name:  "a range to the end",
			stdin: strings.NewReader("abcdefg\n"),
			args:  []string{"-b", "5-"},
			want:  "efg\n",
		},
		{
			name:  "an attached list",
			stdin: strings.NewReader("abcdefg\n"),
			args:  []string{"-b2-4"},
			want:  "bcd\n",
		},
		{
			name:  "a blank-separated list",
			stdin: strings.NewReader("abcdefg\n"),
			args:  []string{"-b", "1 3"},
			want:  "ac\n",
		},
		{
			name:  "positions follow the order of the input data",
			stdin: strings.NewReader("abcdefghi\n"),
			args:  []string{"-b", "6,2,4-7,1"},
			want:  "abdefg\n",
		},
		{
			name:  "overlapping ranges are written once",
			stdin: strings.NewReader("abcdefg\n"),
			args:  []string{"-b", "1-3,2-4"},
			want:  "abcd\n",
		},
		{
			name:  "repeated positions are written once",
			stdin: strings.NewReader("abcdefg\n"),
			args:  []string{"-b", "2,2,2"},
			want:  "b\n",
		},
		{
			name:  "positions past the end of the line are not an error",
			stdin: strings.NewReader("abc\n"),
			args:  []string{"-b", "5-10"},
			want:  "\n",
		},
		{
			name:  "a range partially past the end of the line is clamped",
			stdin: strings.NewReader("abc\n"),
			args:  []string{"-b", "2-10"},
			want:  "bc\n",
		},
		{
			name:  "an empty line stays empty",
			stdin: strings.NewReader("\n"),
			args:  []string{"-b", "1"},
			want:  "\n",
		},
		{
			name: "a missing final newline is added",
			fsys: testFS(),
			args: []string{"-b", "1-2", "nonl.txt"},
			want: "no\n",
		},
		{
			name: "an empty file produces no output",
			fsys: testFS(),
			args: []string{"-b", "1", "empty.txt"},
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunBytesNoSplit(t *testing.T) {
	tests := []testCase{
		{
			name: "a byte inside a character selects the whole character",
			fsys: testFS(),
			args: []string{"-b", "2", "-n", "utf8.txt"},
			want: "á\n\n",
		},
		{
			name: "the first byte of a multi-byte character alone selects nothing",
			fsys: testFS(),
			args: []string{"-b", "1", "-n", "utf8.txt"},
			want: "\n\n",
		},
		{
			name: "a whole character is selected when its last byte is",
			fsys: testFS(),
			args: []string{"-b", "3", "-n", "utf8.txt"},
			want: "b\n日\n",
		},
		{
			name: "a range ending mid character stops before it",
			fsys: testFS(),
			args: []string{"-b", "2-3", "-n", "utf8.txt"},
			want: "áb\n日\n",
		},
		{
			name:  "ranges are partitioned at character boundaries",
			stdin: strings.NewReader("aé日\n"),
			args:  []string{"-b", "1-2", "-n"},
			want:  "a\n",
		},
		{
			name:  "the complementary range gets the rest",
			stdin: strings.NewReader("aé日\n"),
			args:  []string{"-b", "3-", "-n"},
			want:  "é日\n",
		},
		{
			name: "a range that ends before the first character is dropped",
			fsys: testFS(),
			args: []string{"-b", "1-1", "-n", "utf8.txt"},
			want: "\n\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunChars(t *testing.T) {
	tests := []testCase{
		{
			name: "a multi-byte character",
			fsys: testFS(),
			args: []string{"-c", "1", "utf8.txt"},
			want: "á\n日\n",
		},
		{
			name: "a range of characters",
			fsys: testFS(),
			args: []string{"-c", "2-3", "utf8.txt"},
			want: "bç\n本語\n",
		},
		{
			name: "characters follow the order of the input data",
			fsys: testFS(),
			args: []string{"-c", "4,2", "utf8.txt"},
			want: "bd\n本x\n",
		},
		{
			name: "positions past the end of the line are not an error",
			fsys: testFS(),
			args: []string{"-c", "3-10", "utf8.txt"},
			want: "çde\n語x\n",
		},
		{
			name: "a byte that starts no valid encoding counts as one character",
			fsys: testFS(),
			args: []string{"-c", "2", "invalid.txt"},
			want: "\xff\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFields(t *testing.T) {
	tests := []testCase{
		{
			name: "the default delimiter is a tab",
			fsys: testFS(),
			args: []string{"-f", "2", "tabs.txt"},
			want: "f2\nplain\n",
		},
		{
			name: "a custom delimiter",
			fsys: testFS(),
			args: []string{"-d", ":", "-f", "1", "passwd.txt"},
			want: "root\ndaemon\n",
		},
		{
			name: "an attached delimiter",
			fsys: testFS(),
			args: []string{"-d:", "-f", "6", "passwd.txt"},
			want: "/root\n/usr/sbin\n",
		},
		{
			name: "a list of fields",
			fsys: testFS(),
			args: []string{"-d:", "-f", "1,6", "passwd.txt"},
			want: "root:/root\ndaemon:/usr/sbin\n",
		},
		{
			name: "a range of fields",
			fsys: testFS(),
			args: []string{"-d:", "-f", "2-3", "passwd.txt"},
			want: "x:0\nx:1\n",
		},
		{
			name: "a range to the last field",
			fsys: testFS(),
			args: []string{"-d:", "-f", "6-", "passwd.txt"},
			want: "/root:/bin/sh\n/usr/sbin:/usr/sbin/nologin\n",
		},
		{
			name: "fields follow the order of the input data",
			fsys: testFS(),
			args: []string{"-d:", "-f", "3,1", "passwd.txt"},
			want: "root:0\ndaemon:1\n",
		},
		{
			name:  "missing fields are skipped",
			stdin: strings.NewReader("a:b:c\n"),
			args:  []string{"-d:", "-f", "1,4,2"},
			want:  "a:b\n",
		},
		{
			name:  "a line with only missing fields comes out empty",
			stdin: strings.NewReader("a:b\n"),
			args:  []string{"-d:", "-f", "3"},
			want:  "\n",
		},
		{
			name:  "empty fields are preserved",
			stdin: strings.NewReader("a::c\n"),
			args:  []string{"-d:", "-f", "1-3"},
			want:  "a::c\n",
		},
		{
			name: "a line without the delimiter is passed through",
			fsys: testFS(),
			args: []string{"-f", "2", "tabs.txt"},
			want: "f2\nplain\n",
		},
		{
			name: "s suppresses lines without the delimiter",
			fsys: testFS(),
			args: []string{"-f", "2", "-s", "tabs.txt"},
			want: "f2\n",
		},
		{
			name:  "an empty line is passed through",
			stdin: strings.NewReader("\n"),
			args:  []string{"-f", "1"},
			want:  "\n",
		},
		{
			name:  "s suppresses an empty line",
			stdin: strings.NewReader("\n"),
			args:  []string{"-f", "1", "-s"},
			want:  "",
		},
		{
			name:  "the delimiter is not special in the middle of quotes",
			stdin: strings.NewReader(`a:"b:c"` + "\n"),
			args:  []string{"-d:", "-f", "2"},
			want:  "\"b\n",
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
			name: "multiple files are concatenated in order",
			fsys: testFS(),
			args: []string{"-b", "1", "a.txt", "nonl.txt"},
			want: "a\nx\nn\n",
		},
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("stdin\n"),
			args:  []string{"-b", "1-3", "-"},
			want:  "std\n",
		},
		{
			name:  "dash between files reads the standard input at that point",
			stdin: strings.NewReader("stdin\n"),
			fsys:  testFS(),
			args:  []string{"-b", "1", "a.txt", "-", "nonl.txt"},
			want:  "a\nx\ns\nn\n",
		},
		{
			name: "pathnames are resolved from the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"-b", "1-5", "inner.txt"},
			want: "inner\n",
		},
		{
			name: "the last list wins when the same option is repeated",
			fsys: testFS(),
			args: []string{"-b", "1", "-b", "2", "a.txt"},
			want: "b\ny\n",
		},
		{
			name:     "options are not parsed after the first operand",
			fsys:     testFS(),
			args:     []string{"-b", "1", "a.txt", "-n"},
			want:     "a\nx\n",
			wantErr:  "cut: -n: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing file doesn't stop the other files",
			fsys:     testFS(),
			args:     []string{"-b", "1", "missing.txt", "a.txt"},
			want:     "a\nx\n",
			wantErr:  "cut: missing.txt: file does not exist\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunListErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "zero is not a valid position",
			args:     []string{"-b", "0"},
			wantErr:  "cut: invalid position: '0'\n",
			wantExit: 2,
		},
		{
			name:     "a zero range end is not valid",
			args:     []string{"-b", "1-0"},
			wantErr:  "cut: invalid position: '0'\n",
			wantExit: 2,
		},
		{
			name:     "a non-numeric position",
			args:     []string{"-b", "x"},
			wantErr:  "cut: invalid position: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an empty list",
			args:     []string{"-b", ""},
			wantErr:  "cut: invalid position: ''\n",
			wantExit: 2,
		},
		{
			name:     "an empty element",
			args:     []string{"-b", "1,,2"},
			wantErr:  "cut: invalid position: ''\n",
			wantExit: 2,
		},
		{
			name:     "a lone dash",
			args:     []string{"-b", "-"},
			wantErr:  "cut: invalid list: '-'\n",
			wantExit: 2,
		},
		{
			name:     "a decreasing range",
			args:     []string{"-b", "3-2"},
			wantErr:  "cut: invalid decreasing range: '3-2'\n",
			wantExit: 2,
		},
		{
			name:     "a position with a sign",
			args:     []string{"-b", "+1"},
			wantErr:  "cut: invalid position: '+1'\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOptionErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "no list option",
			args:     []string{},
			wantErr:  "cut: you must specify a list of bytes, characters, or fields\n",
			wantExit: 2,
		},
		{
			name:     "b and c conflict",
			args:     []string{"-b", "1", "-c", "1"},
			wantErr:  "cut: only one of -b, -c or -f may be specified\n",
			wantExit: 2,
		},
		{
			name:     "b and f conflict",
			args:     []string{"-f", "1", "-b", "1"},
			wantErr:  "cut: only one of -b, -c or -f may be specified\n",
			wantExit: 2,
		},
		{
			name:     "d is only meaningful with f",
			args:     []string{"-b", "1", "-d", ":"},
			wantErr:  "cut: the delimiter is only meaningful with the -f option\n",
			wantExit: 2,
		},
		{
			name:     "s is only meaningful with f",
			args:     []string{"-c", "1", "-s"},
			wantErr:  "cut: the -s option is only meaningful with the -f option\n",
			wantExit: 2,
		},
		{
			name:     "n is only meaningful with b",
			args:     []string{"-f", "1", "-n"},
			wantErr:  "cut: the -n option is only meaningful with the -b option\n",
			wantExit: 2,
		},
		{
			name:     "the delimiter must be a single character",
			args:     []string{"-f", "1", "-d", "::"},
			wantErr:  "cut: the delimiter must be a single character\n",
			wantExit: 2,
		},
		{
			name:     "an empty delimiter",
			args:     []string{"-f", "1", "-d", ""},
			wantErr:  "cut: the delimiter must be a single character\n",
			wantExit: 2,
		},
		{
			name:     "a multi-byte delimiter",
			args:     []string{"-f", "1", "-d", "é"},
			wantErr:  "cut: the delimiter must be a single character\n",
			wantExit: 2,
		},
		{
			name:     "invalid option",
			args:     []string{"-x"},
			wantErr:  "cut: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "missing option argument",
			args:     []string{"-b"},
			wantErr:  "cut: option requires an argument -- 'b'\n",
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
			name: "bytes",
			fsys: testFS(),
			args: []string{"--bytes", "1", "a.txt"},
			want: "a\nx\n",
		},
		{
			name: "characters",
			fsys: testFS(),
			args: []string{"--characters", "1", "utf8.txt"},
			want: "á\n日\n",
		},
		{
			name: "fields and delimiter",
			fsys: testFS(),
			args: []string{"--fields", "1", "--delimiter", ":", "passwd.txt"},
			want: "root\ndaemon\n",
		},
		{
			name: "only-delimited",
			fsys: testFS(),
			args: []string{"--fields", "2", "--only-delimited", "tabs.txt"},
			want: "f2\n",
		},
		{
			name:     "unknown long option",
			args:     []string{"--complement", "-b", "1"},
			wantErr:  "cut: unrecognized option '--complement'\n",
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
			args:     []string{"-b", "1", "missing.txt"},
			wantErr:  "cut: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "directory operand",
			fsys:     fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			args:     []string{"-b", "1", "dir"},
			wantErr:  "cut: dir: invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "zero-length file name",
			fsys:     testFS(),
			args:     []string{"-b", "1", ""},
			wantErr:  "cut: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "file operand without a file system",
			args:     []string{"-b", "1", "a.txt"},
			wantErr:  "cut: a.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			args:     []string{"-b", "1"},
			wantErr:  "cut: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "standard input read error",
			stdin:    errorReader{err: errors.New("read failed")},
			args:     []string{"-b", "1"},
			wantErr:  "cut: standard input: read failed\n",
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
	err := New(rt).Run(t.Context(), "-b", "1")

	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
	if got := stderr.String(); got != "cut: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "cut: write error: write failed\n")
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
	err := New(rt).Run(ctx, "-b", "1")

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
