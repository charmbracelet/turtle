package tail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

// readerFS is an fs.FS whose only file reads from a reader, making the input
// non-seekable.
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
//	a.txt:     "a\nb\nc\nd\ne\n"
//	nonl.txt:  "a\nb\nc" (no trailing newline)
//	empty.txt: ""
//	bytes.txt: "0123456789"
//	lines.txt: "l1\nl2\nl3\nl4\nl5\n"
//	many.txt:  12 numbered lines, "01\n" to "12\n"
//	nl.txt:    "a\n"
//	"-f":      "dash-f\n" (a file named like an option)
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":         {Data: []byte("a\nb\nc\nd\ne\n")},
		"nonl.txt":      {Data: []byte("a\nb\nc")},
		"empty.txt":     {Data: []byte("")},
		"bytes.txt":     {Data: []byte("0123456789")},
		"lines.txt":     {Data: []byte("l1\nl2\nl3\nl4\nl5\n")},
		"many.txt":      {Data: []byte(manyLines())},
		"nl.txt":        {Data: []byte("a\n")},
		"-f":            {Data: []byte("dash-f\n")},
		"sub/inner.txt": {Data: []byte("inner\n")},
	}
}

// manyLines returns 12 numbered lines, from "01\n" to "12\n".
func manyLines() string {
	var b strings.Builder
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&b, "%02d\n", i)
	}
	return b.String()
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
			name:  "no operand reads the standard input",
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
			name:  "defaults to the last ten lines",
			stdin: strings.NewReader(manyLines()),
			want:  "03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n",
		},
		{
			name: "fewer than ten lines copies the whole file",
			fsys: testFS(),
			args: []string{"a.txt"},
			want: "a\nb\nc\nd\ne\n",
		},
		{
			name: "more than ten lines copies the last ten",
			fsys: testFS(),
			args: []string{"many.txt"},
			want: "03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n",
		},
		{
			name: "file without a trailing newline",
			fsys: testFS(),
			args: []string{"nonl.txt"},
			want: "a\nb\nc",
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
			args: []string{"--", "-f"},
			want: "dash-f\n",
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
			name: "the last lines of a file",
			fsys: testFS(),
			args: []string{"-n", "2", "lines.txt"},
			want: "l4\nl5\n",
		},
		{
			name: "number joined to the option",
			fsys: testFS(),
			args: []string{"-n2", "lines.txt"},
			want: "l4\nl5\n",
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
			name: "exactly the number of lines in the file",
			fsys: testFS(),
			args: []string{"-n", "5", "a.txt"},
			want: "a\nb\nc\nd\ne\n",
		},
		{
			name: "the final line without a newline counts as a line",
			fsys: testFS(),
			args: []string{"-n", "1", "nonl.txt"},
			want: "c",
		},
		{
			name: "counting the final incomplete line",
			fsys: testFS(),
			args: []string{"-n", "2", "nonl.txt"},
			want: "b\nc",
		},
		{
			name: "more lines than the file without a trailing newline",
			fsys: testFS(),
			args: []string{"-n", "4", "nonl.txt"},
			want: "a\nb\nc",
		},
		{
			name: "a negative sign counts from the end",
			fsys: testFS(),
			args: []string{"-n", "-2", "lines.txt"},
			want: "l4\nl5\n",
		},
		{
			name: "from the second line",
			fsys: testFS(),
			args: []string{"-n", "+2", "lines.txt"},
			want: "l2\nl3\nl4\nl5\n",
		},
		{
			name: "from the first line",
			fsys: testFS(),
			args: []string{"-n", "+1", "lines.txt"},
			want: "l1\nl2\nl3\nl4\nl5\n",
		},
		{
			name: "from the last line",
			fsys: testFS(),
			args: []string{"-n", "+5", "lines.txt"},
			want: "l5\n",
		},
		{
			name: "from beyond the last line",
			fsys: testFS(),
			args: []string{"-n", "+6", "lines.txt"},
			want: "",
		},
		{
			name: "from a line of a file without a trailing newline",
			fsys: testFS(),
			args: []string{"-n", "+2", "nonl.txt"},
			want: "b\nc",
		},
		{
			name: "single line file",
			fsys: testFS(),
			args: []string{"-n", "2", "nl.txt"},
			want: "a\n",
		},
		{
			name: "empty file",
			fsys: testFS(),
			args: []string{"-n", "2", "empty.txt"},
			want: "",
		},
		{
			name:  "lines of the standard input",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-n", "2"},
			want:  "b\nc\n",
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
			name: "the last bytes of a file",
			fsys: testFS(),
			args: []string{"-c", "3", "bytes.txt"},
			want: "789",
		},
		{
			name: "number joined to the option",
			fsys: testFS(),
			args: []string{"-c3", "bytes.txt"},
			want: "789",
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
			args: []string{"-c", "100", "bytes.txt"},
			want: "0123456789",
		},
		{
			name: "a negative sign counts from the end",
			fsys: testFS(),
			args: []string{"-c", "-3", "bytes.txt"},
			want: "789",
		},
		{
			name: "from the fourth byte",
			fsys: testFS(),
			args: []string{"-c", "+4", "bytes.txt"},
			want: "3456789",
		},
		{
			name: "from the first byte",
			fsys: testFS(),
			args: []string{"-c", "+1", "bytes.txt"},
			want: "0123456789",
		},
		{
			name: "from beyond the last byte",
			fsys: testFS(),
			args: []string{"-c", "+11", "bytes.txt"},
			want: "",
		},
		{
			name: "bytes can cut through a line",
			fsys: testFS(),
			args: []string{"-c", "3", "nonl.txt"},
			want: "b\nc",
		},
		{
			name: "a single byte",
			fsys: testFS(),
			args: []string{"-c", "1", "nonl.txt"},
			want: "c",
		},
		{
			name: "ending at a newline",
			fsys: testFS(),
			args: []string{"-c", "2", "lines.txt"},
			want: "5\n",
		},
		{
			name: "empty file",
			fsys: testFS(),
			args: []string{"-c", "3", "empty.txt"},
			want: "",
		},
		{
			name:  "bytes of the standard input",
			stdin: strings.NewReader("abcdef"),
			args:  []string{"-c", "3"},
			want:  "def",
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
			args: []string{"-n", "2", "-c", "2", "lines.txt"},
			want: "5\n",
		},
		{
			name: "lines after bytes",
			fsys: testFS(),
			args: []string{"-c", "2", "-n", "2", "lines.txt"},
			want: "l4\nl5\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunMultipleOperands(t *testing.T) {
	tests := []testCase{
		{
			name: "a header precedes the output of each file",
			fsys: testFS(),
			args: []string{"a.txt", "lines.txt"},
			want: "==> a.txt <==\na\nb\nc\nd\ne\n\n==> lines.txt <==\nl1\nl2\nl3\nl4\nl5\n",
		},
		{
			name: "the operands are processed in order",
			fsys: testFS(),
			args: []string{"lines.txt", "a.txt"},
			want: "==> lines.txt <==\nl1\nl2\nl3\nl4\nl5\n\n==> a.txt <==\na\nb\nc\nd\ne\n",
		},
		{
			name: "the options apply to every file",
			fsys: testFS(),
			args: []string{"-n", "1", "a.txt", "lines.txt"},
			want: "==> a.txt <==\ne\n\n==> lines.txt <==\nl5\n",
		},
		{
			name: "a single operand prints no header",
			fsys: testFS(),
			args: []string{"a.txt"},
			want: "a\nb\nc\nd\ne\n",
		},
		{
			name:  "the standard input among the operands",
			stdin: strings.NewReader("one\ntwo\n"),
			fsys:  testFS(),
			args:  []string{"a.txt", "-"},
			want:  "==> a.txt <==\na\nb\nc\nd\ne\n\n==> standard input <==\none\ntwo\n",
		},
		{
			name: "a file without output still prints its header",
			fsys: testFS(),
			args: []string{"empty.txt", "a.txt"},
			want: "==> empty.txt <==\n\n==> a.txt <==\na\nb\nc\nd\ne\n",
		},
		{
			name:     "a failing file doesn't stop the others",
			fsys:     testFS(),
			args:     []string{"missing.txt", "a.txt"},
			want:     "==> a.txt <==\na\nb\nc\nd\ne\n",
			wantErr:  "tail: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "an invalid zero-length name doesn't stop the others",
			fsys:     testFS(),
			args:     []string{"", "a.txt"},
			want:     "==> a.txt <==\na\nb\nc\nd\ne\n",
			wantErr:  "tail: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name: "every file failing exits nonzero",
			fsys: testFS(),
			args: []string{"missing.txt", "also-missing.txt"},
			wantErr: "tail: missing.txt: file does not exist\n" +
				"tail: also-missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a read error doesn't stop the others",
			stdin:    errorReader{err: errors.New("boom")},
			fsys:     testFS(),
			args:     []string{"-", "a.txt"},
			want:     "==> standard input <==\n\n==> a.txt <==\na\nb\nc\nd\ne\n",
			wantErr:  "tail: -: boom\n",
			wantExit: 1,
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
			args: []string{"-q", "a.txt", "lines.txt"},
			want: "a\nb\nc\nd\ne\nl1\nl2\nl3\nl4\nl5\n",
		},
		{
			name: "silent is the same as quiet",
			fsys: testFS(),
			args: []string{"--silent", "a.txt", "lines.txt"},
			want: "a\nb\nc\nd\ne\nl1\nl2\nl3\nl4\nl5\n",
		},
		{
			name:     "quiet keeps the headers off failing files too",
			fsys:     testFS(),
			args:     []string{"-q", "missing.txt", "a.txt"},
			want:     "a\nb\nc\nd\ne\n",
			wantErr:  "tail: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name: "verbose prints the header of a single file",
			fsys: testFS(),
			args: []string{"-v", "a.txt"},
			want: "==> a.txt <==\na\nb\nc\nd\ne\n",
		},
		{
			name:  "verbose names the standard input",
			stdin: strings.NewReader("one\n"),
			args:  []string{"-v"},
			want:  "==> standard input <==\none\n",
		},
		{
			name:  "verbose prints the header of the standard input operand",
			stdin: strings.NewReader("one\n"),
			fsys:  testFS(),
			args:  []string{"--verbose", "-"},
			want:  "==> standard input <==\none\n",
		},
		{
			name: "verbose prints the headers of several files",
			fsys: testFS(),
			args: []string{"-v", "a.txt", "lines.txt"},
			want: "==> a.txt <==\na\nb\nc\nd\ne\n\n==> lines.txt <==\nl1\nl2\nl3\nl4\nl5\n",
		},
		{
			name: "the last of quiet and verbose wins",
			fsys: testFS(),
			args: []string{"-q", "-v", "a.txt"},
			want: "==> a.txt <==\na\nb\nc\nd\ne\n",
		},
		{
			name: "quiet after verbose wins the other way",
			fsys: testFS(),
			args: []string{"-v", "-q", "a.txt", "lines.txt"},
			want: "a\nb\nc\nd\ne\nl1\nl2\nl3\nl4\nl5\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFollowMultipleFiles(t *testing.T) {
	t.Run("the followed files get a header when the output switches", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a.txt":     {Data: []byte("a\n")},
			"lines.txt": {Data: []byte("l1\n")},
		}
		steps := []func(){
			func() { fsys["a.txt"].Data = append(fsys["a.txt"].Data, "a2\n"...) },
			func() { fsys["lines.txt"].Data = append(fsys["lines.txt"].Data, "l2\n"...) },
			func() { fsys["a.txt"].Data = append(fsys["a.txt"].Data, "a3\n"...) },
		}
		got, stderr, err := runFollowSteps(t, fsys, []string{"-f", "a.txt", "lines.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		want := "==> a.txt <==\na\n" +
			"\n==> lines.txt <==\nl1\n" +
			"\n==> a.txt <==\na2\n" +
			"\n==> lines.txt <==\nl2\n" +
			"\n==> a.txt <==\na3\n"
		if got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if stderr != "" {
			t.Errorf("Run() stderr = %q, want %q", stderr, "")
		}
	})

	t.Run("-q never prints the headers", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a.txt":     {Data: []byte("a\n")},
			"lines.txt": {Data: []byte("l1\n")},
		}
		steps := []func(){
			func() { fsys["lines.txt"].Data = append(fsys["lines.txt"].Data, "l2\n"...) },
		}
		got, _, err := runFollowSteps(t, fsys, []string{"-fq", "a.txt", "lines.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "a\nl1\nl2\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}

// runFollowSteps runs tail with args on fsys, running each step on a wait,
// and cancelling the context once the steps run out.
func runFollowSteps(t *testing.T, fsys fsx.MemFS, args []string, steps []func()) (string, string, error) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	i := 0
	rt := &coreutils.Runtime{
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsys,
		Sleep: func(ctx context.Context, d time.Duration) error {
			if i >= len(steps) {
				cancel()
				return ctx.Err()
			}
			steps[i]()
			i++
			return nil
		},
	}
	err := New(rt).Run(ctx, args...)
	return stdout.String(), stderr.String(), err
}

func TestRunReverse(t *testing.T) {
	tests := []testCase{
		{
			name: "all lines in reverse",
			fsys: testFS(),
			args: []string{"-r", "a.txt"},
			want: "e\nd\nc\nb\na\n",
		},
		{
			name: "lines are concatenated as they are read",
			fsys: testFS(),
			args: []string{"-r", "nonl.txt"},
			want: "cb\na\n",
		},
		{
			name: "a single line",
			fsys: testFS(),
			args: []string{"-r", "nl.txt"},
			want: "a\n",
		},
		{
			name: "empty file",
			fsys: testFS(),
			args: []string{"-r", "empty.txt"},
			want: "",
		},
		{
			name: "the last lines in reverse",
			fsys: testFS(),
			args: []string{"-r", "-n", "2", "a.txt"},
			want: "e\nd\n",
		},
		{
			name: "number joined to the option",
			fsys: testFS(),
			args: []string{"-r", "-n2", "a.txt"},
			want: "e\nd\n",
		},
		{
			name: "the last lines of a file without a trailing newline",
			fsys: testFS(),
			args: []string{"-r", "-n", "2", "nonl.txt"},
			want: "cb\n",
		},
		{
			name: "more lines than the file in reverse",
			fsys: testFS(),
			args: []string{"-r", "-n", "100", "a.txt"},
			want: "e\nd\nc\nb\na\n",
		},
		{
			name: "zero lines in reverse",
			fsys: testFS(),
			args: []string{"-r", "-n", "0", "a.txt"},
			want: "",
		},
		{
			name:  "lines of the standard input in reverse",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-r"},
			want:  "c\nb\na\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunNonSeekableInput(t *testing.T) {
	tests := []struct {
		name string
		data string
		args []string
		want string
	}{
		{"last lines", "a\nb\nc\n", []string{"-n", "2"}, "b\nc\n"},
		{"last bytes", "abcde", []string{"-c", "3"}, "cde"},
		{"from a line", "a\nb\nc\n", []string{"-n", "+2"}, "b\nc\n"},
		{"from a byte", "abcde", []string{"-c", "+3"}, "cde"},
		{"all lines in reverse", "a\nb\nc\n", []string{"-r"}, "c\nb\na\n"},
		{"a single line without a newline", "abc", []string{"-r"}, "abc"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, testCase{
				fsys: readerFS{r: strings.NewReader(test.data)},
				args: append(test.args, "f.txt"),
				want: test.want,
			})
		})
	}
}

func TestRunLinesAcrossChunks(t *testing.T) {
	const line = "123456789\n" // 10 bytes
	data := strings.Repeat(line, 1000)

	tests := []testCase{
		{
			name: "more lines than a single backwards chunk",
			fsys: fsx.MemFS{"big.txt": {Data: []byte(data)}},
			args: []string{"-n", "500", "big.txt"},
			want: strings.Repeat(line, 500),
		},
		{
			name: "the last line",
			fsys: fsx.MemFS{"big.txt": {Data: []byte(data)}},
			args: []string{"-n", "1", "big.txt"},
			want: line,
		},
		{
			name: "more lines than the file",
			fsys: fsx.MemFS{"big.txt": {Data: []byte(data)}},
			args: []string{"-n", "1001", "big.txt"},
			want: data,
		},
		{
			name: "a single long line",
			fsys: fsx.MemFS{"long.txt": {Data: []byte(strings.Repeat("x", 10000))}},
			args: []string{"-n", "1", "long.txt"},
			want: strings.Repeat("x", 10000),
		},
		{
			name: "more lines than a long single line file",
			fsys: fsx.MemFS{"long.txt": {Data: []byte(strings.Repeat("x", 10000))}},
			args: []string{"-n", "2", "long.txt"},
			want: strings.Repeat("x", 10000),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFollowFile(t *testing.T) {
	tests := []struct {
		name   string
		data   string
		args   []string
		chunks []string
		want   string
	}{
		{
			name:   "copies data appended to the file",
			data:   "one\n",
			args:   []string{"-f", "f.txt"},
			chunks: []string{"two\n"},
			want:   "one\ntwo\n",
		},
		{
			name:   "copies data appended in several rounds",
			data:   "one\n",
			args:   []string{"-f", "f.txt"},
			chunks: []string{"two\n", "three\n"},
			want:   "one\ntwo\nthree\n",
		},
		{
			name:   "with a line count",
			data:   "one\ntwo\nthree\n",
			args:   []string{"-f", "-n", "2", "f.txt"},
			chunks: []string{"four\n"},
			want:   "two\nthree\nfour\n",
		},
		{
			name:   "with a byte count",
			data:   "abc",
			args:   []string{"-f", "-c", "2", "f.txt"},
			chunks: []string{"de"},
			want:   "bcde",
		},
		{
			name:   "with a zero byte count",
			data:   "abc\n",
			args:   []string{"-f", "-c", "0", "f.txt"},
			chunks: []string{"de\n"},
			want:   "de\n",
		},
		{
			name:   "from the beginning of the file",
			data:   "one\n",
			args:   []string{"-f", "-n", "+1", "f.txt"},
			chunks: []string{"two\n"},
			want:   "one\ntwo\n",
		},
		{
			name:   "appending before the first wait",
			data:   "one\n",
			args:   []string{"-f", "f.txt"},
			chunks: []string{},
			want:   "one\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, stderr, err := runFollow(t, test.data, test.args, test.chunks)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Run() error = %v, want context.Canceled", err)
			}
			if got != test.want {
				t.Errorf("Run() stdout = %q, want %q", got, test.want)
			}
			if stderr != "" {
				t.Errorf("Run() stderr = %q, want %q", stderr, "")
			}
		})
	}
}

// runFollow runs tail with -f, appending each chunk to the file on each wait,
// and cancelling the context once the chunks run out. The context error is
// returned as is.
func runFollow(t *testing.T, data string, args []string, chunks []string) (string, string, error) {
	t.Helper()

	fsys := fsx.MemFS{"f.txt": {Data: []byte(data)}}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	waits := 0
	rt := &coreutils.Runtime{
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsys,
		Sleep: func(ctx context.Context, d time.Duration) error {
			if waits >= len(chunks) {
				cancel()
				return ctx.Err()
			}
			f := fsys["f.txt"]
			f.Data = append(f.Data, chunks[waits]...)
			waits++
			return nil
		},
	}
	err := New(rt).Run(ctx, args...)
	return stdout.String(), stderr.String(), err
}

func TestRunFollowStandardInput(t *testing.T) {
	stdout := new(bytes.Buffer)
	ctx, cancel := context.WithCancel(t.Context())
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("one\ntwo\n"),
		Stdout: stdout,
		Stderr: io.Discard,
		Sleep: func(ctx context.Context, d time.Duration) error {
			cancel()
			return ctx.Err()
		},
	}
	err := New(rt).Run(ctx, "-f")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
	if want := "one\ntwo\n"; stdout.String() != want {
		t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunFollowErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		runCase(t, testCase{
			fsys:     testFS(),
			args:     []string{"-f", "missing.txt"},
			wantErr:  "tail: missing.txt: file does not exist\ntail: no files remaining\n",
			wantExit: 1,
		})
	})

	t.Run("a removed name is given up without --retry", func(t *testing.T) {
		fsys := fsx.MemFS{"f.txt": {Data: []byte("one\n")}}
		steps := []func(){
			func() { delete(fsys, "f.txt") },
		}
		got, stderr, err := runFollowSteps(t, fsys, []string{"--follow=name", "--max-unchanged-stats=1", "f.txt"}, steps)
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
		if want := "one\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		wantErr := "tail: 'f.txt' has become inaccessible: file does not exist; giving up on this name\n" +
			"tail: no files remaining\n"
		if stderr != wantErr {
			t.Errorf("Run() stderr = %q, want %q", stderr, wantErr)
		}
	})
	t.Run("a read error on the followed standard input", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  errorReader{err: errors.New("boom")},
			Stdout: stdout,
			Stderr: stderr,
			Sleep: func(ctx context.Context, d time.Duration) error {
				return nil
			},
		}
		err := New(rt).Run(t.Context(), "-f")
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
		if stdout.String() != "" {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), "")
		}
		wantErr := "tail: standard input: boom\ntail: no files remaining\n"
		if stderr.String() != wantErr {
			t.Errorf("Run() stderr = %q, want %q", stderr.String(), wantErr)
		}
	})
}

func TestRunFollowTruncate(t *testing.T) {
	fsys := fsx.MemFS{"f.txt": {Data: []byte("one\ntwo\nthree\n")}}
	steps := []func(){
		func() { fsys["f.txt"].Data = fsys["f.txt"].Data[:4] },
	}
	got, stderr, err := runFollowSteps(t, fsys, []string{"-f", "f.txt"}, steps)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
	if want := "one\ntwo\nthree\none\n"; got != want {
		t.Errorf("Run() stdout = %q, want %q", got, want)
	}
	if want := "tail: f.txt: file truncated\n"; stderr != want {
		t.Errorf("Run() stderr = %q, want %q", stderr, want)
	}
}

func TestRunFollowName(t *testing.T) {
	t.Run("a replaced file is followed from its beginning", func(t *testing.T) {
		fsys := fsx.MemFS{"f.txt": {Data: []byte("old\n")}}
		steps := []func(){
			func() { fsys["f.txt"] = &fsx.MemFile{Data: []byte("new\n")} },
		}
		got, stderr, err := runFollowSteps(t, fsys, []string{"--follow=name", "--max-unchanged-stats=1", "f.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "old\nnew\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if want := "tail: 'f.txt' has been replaced;  following new file\n"; stderr != want {
			t.Errorf("Run() stderr = %q, want %q", stderr, want)
		}
	})

	t.Run("a reappeared file is followed from its beginning with --retry", func(t *testing.T) {
		fsys := fsx.MemFS{"f.txt": {Data: []byte("old\n")}}
		steps := []func(){
			func() { delete(fsys, "f.txt") },
			func() { fsys["f.txt"] = &fsx.MemFile{Data: []byte("new\n")} },
		}
		got, stderr, err := runFollowSteps(t, fsys, []string{"--follow=name", "--retry", "--max-unchanged-stats=1", "f.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "old\nnew\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		wantErr := "tail: 'f.txt' has become inaccessible: file does not exist\n" +
			"tail: 'f.txt' has appeared;  following new file\n"
		if stderr != wantErr {
			t.Errorf("Run() stderr = %q, want %q", stderr, wantErr)
		}
	})

	t.Run("-F follows a missing file once it appears", func(t *testing.T) {
		fsys := fsx.MemFS{}
		steps := []func(){
			func() { fsys["f.txt"] = &fsx.MemFile{Data: []byte("hello\n")} },
		}
		got, stderr, err := runFollowSteps(t, fsys, []string{"-F", "f.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "hello\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		wantErr := "tail: f.txt: file does not exist\n" +
			"tail: 'f.txt' has appeared;  following new file\n"
		if stderr != wantErr {
			t.Errorf("Run() stderr = %q, want %q", stderr, wantErr)
		}
	})

	t.Run("a stdin operand can't be followed by name", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader("one\n"),
			Stdout: io.Discard,
			Stderr: io.Discard,
			FS:     testFS(),
		}
		err := New(rt).Run(t.Context(), "--follow=name", "-")
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
	})

	t.Run("the standard input is followed by descriptor by name", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		ctx, cancel := context.WithCancel(t.Context())
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader("one\ntwo\n"),
			Stdout: stdout,
			Stderr: io.Discard,
			Sleep: func(ctx context.Context, d time.Duration) error {
				cancel()
				return ctx.Err()
			},
		}
		err := New(rt).Run(ctx, "--follow=name")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "one\ntwo\n"; stdout.String() != want {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	})
}

func TestRunFollowPID(t *testing.T) {
	t.Run("the follow stops when all the writers exit", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdout: stdout,
			Stderr: io.Discard,
			FS:     fsx.MemFS{"f.txt": {Data: []byte("one\n")}},
			ProcessAlive: func(pid int) bool {
				return false
			},
			Sleep: func(ctx context.Context, d time.Duration) error {
				return nil
			},
		}
		err := New(rt).Run(t.Context(), "-f", "--pid=1234", "f.txt")
		if err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
		if want := "one\n"; stdout.String() != want {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	})

	t.Run("--pid is not supported on this system", func(t *testing.T) {
		fsys := fsx.MemFS{"f.txt": {Data: []byte("one\n")}}
		steps := []func(){}
		got, stderr, err := runFollowSteps(t, fsys, []string{"-f", "--pid=1234", "f.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "one\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		wantErr := "tail: warning: --pid=PID is not supported on this system\n"
		if stderr != wantErr {
			t.Errorf("Run() stderr = %q, want %q", stderr, wantErr)
		}
	})

	t.Run("--pid is ignored without following", func(t *testing.T) {
		runCase(t, testCase{
			fsys:     testFS(),
			args:     []string{"--pid=1", "a.txt"},
			want:     "a\nb\nc\nd\ne\n",
			wantErr:  "tail: warning: PID ignored; --pid=PID is useful only when following\n",
			wantExit: 0,
		})
	})
}

func TestRunFollowWarnings(t *testing.T) {
	t.Run("--retry is useless without following", func(t *testing.T) {
		runCase(t, testCase{
			fsys:     testFS(),
			args:     []string{"--retry", "a.txt"},
			want:     "a\nb\nc\nd\ne\n",
			wantErr:  "tail: warning: --retry ignored; --retry is useful only when following\n",
			wantExit: 0,
		})
	})

	t.Run("--retry only affects the initial open by descriptor", func(t *testing.T) {
		fsys := fsx.MemFS{"f.txt": {Data: []byte("one\n")}}
		steps := []func(){}
		_, stderr, err := runFollowSteps(t, fsys, []string{"-f", "--retry", "f.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "tail: warning: --retry only effective for the initial open\n"; stderr != want {
			t.Errorf("Run() stderr = %q, want %q", stderr, want)
		}
	})
}

func TestRunFollowSleepInterval(t *testing.T) {
	t.Run("the default is one second", func(t *testing.T) {
		var got time.Duration
		fsys := fsx.MemFS{"f.txt": {Data: []byte("one\n")}}
		stdout := new(bytes.Buffer)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		rt := &coreutils.Runtime{
			Stdout: stdout,
			Stderr: io.Discard,
			FS:     fsys,
			Sleep: func(ctx context.Context, d time.Duration) error {
				got = d
				cancel()
				return ctx.Err()
			},
		}
		err := New(rt).Run(ctx, "-f", "f.txt")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := time.Second; got != want {
			t.Errorf("sleep interval = %v, want %v", got, want)
		}
	})

	for name, value := range map[string]string{
		"a decimal number":  "0.5",
		"a comma separator": "0,5",
		"zero seconds":      "0",
	} {
		t.Run(name, func(t *testing.T) {
			var got time.Duration
			fsys := fsx.MemFS{"f.txt": {Data: []byte("one\n")}}
			stdout := new(bytes.Buffer)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			rt := &coreutils.Runtime{
				Stdout: stdout,
				Stderr: io.Discard,
				FS:     fsys,
				Sleep: func(ctx context.Context, d time.Duration) error {
					got = d
					cancel()
					return ctx.Err()
				},
			}
			err := New(rt).Run(ctx, "-f", "-s", value, "f.txt")
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Run() error = %v, want context.Canceled", err)
			}
			want := time.Duration(0)
			if value != "0" {
				want = 500 * time.Millisecond
			}
			if got != want {
				t.Errorf("sleep interval = %v, want %v", got, want)
			}
		})
	}
}

func TestRunFollowDebug(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "following by descriptor polls",
			args:    []string{"--debug", "-f", "f.txt"},
			wantErr: "tail: using polling mode\n",
		},
		{
			name:    "following by name polls",
			args:    []string{"--debug", "-F", "f.txt"},
			wantErr: "tail: using polling mode\n",
		},
		{
			name:    "several files poll",
			args:    []string{"--debug", "-f", "f.txt", "g.txt"},
			wantErr: "tail: using polling mode\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fsys := fsx.MemFS{"f.txt": {Data: []byte("one\n")}, "g.txt": {Data: []byte("1\n")}}
			_, stderr, err := runFollowSteps(t, fsys, test.args, nil)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Run() error = %v, want context.Canceled", err)
			}
			if stderr != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", stderr, test.wantErr)
			}
		})
	}
}

// pipeStdin is a standard input which stats as a FIFO or a pipe.
type pipeStdin struct{ io.Reader }

func (pipeStdin) Stat() (fs.FileInfo, error) { return pipeInfo{}, nil }

// pipeInfo describes a FIFO.
type pipeInfo struct{}

func (pipeInfo) Name() string       { return "stdin" }
func (pipeInfo) Size() int64        { return 0 }
func (pipeInfo) Mode() fs.FileMode  { return fs.ModeNamedPipe | 0o600 }
func (pipeInfo) ModTime() time.Time { return time.Time{} }
func (pipeInfo) IsDir() bool        { return false }
func (pipeInfo) Sys() any           { return nil }

func TestRunFollowStdinPipe(t *testing.T) {
	t.Run("-f is ignored with no file operand", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  pipeStdin{strings.NewReader("hi\n")},
			Stdout: stdout,
			Stderr: io.Discard,
			Sleep: func(ctx context.Context, d time.Duration) error {
				t.Error("the follow waited")
				return nil
			},
		}
		err := New(rt).Run(t.Context(), "-f")
		if err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
		if want := "hi\n"; stdout.String() != want {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	})

	t.Run("a stdin operand isn't followed", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		rt := &coreutils.Runtime{
			Stdin:  pipeStdin{strings.NewReader("hi\n")},
			Stdout: stdout,
			Stderr: io.Discard,
			FS:     fsx.MemFS{"a.txt": {Data: []byte("a\n")}},
			Sleep: func(ctx context.Context, d time.Duration) error {
				cancel()
				return ctx.Err()
			},
		}
		err := New(rt).Run(ctx, "-f", "-", "a.txt")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		want := "==> standard input <==\nhi\n\n==> a.txt <==\na\n"
		if stdout.String() != want {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	})

	t.Run("a stdin operand alone returns after copying", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  pipeStdin{strings.NewReader("hi\n")},
			Stdout: stdout,
			Stderr: io.Discard,
			Sleep: func(ctx context.Context, d time.Duration) error {
				t.Error("the follow waited")
				return nil
			},
		}
		err := New(rt).Run(t.Context(), "-f", "-")
		if err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
		if want := "hi\n"; stdout.String() != want {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	})

	t.Run("a stdin operand with a pipe isn't followed by name", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  pipeStdin{strings.NewReader("hi\n")},
			Stdout: stdout,
			Stderr: io.Discard,
			Sleep: func(ctx context.Context, d time.Duration) error {
				t.Error("the follow waited")
				return nil
			},
		}
		err := New(rt).Run(t.Context(), "--follow=name", "-")
		if err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
		if want := "hi\n"; stdout.String() != want {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	})
}

func TestRunLongOptions(t *testing.T) {
	tests := []testCase{
		{
			name: "lines takes its value in the next argument",
			fsys: testFS(),
			args: []string{"--lines", "2", "lines.txt"},
			want: "l4\nl5\n",
		},
		{
			name: "lines takes its value glued to it",
			fsys: testFS(),
			args: []string{"--lines=2", "lines.txt"},
			want: "l4\nl5\n",
		},
		{
			name: "bytes takes its value glued to it",
			fsys: testFS(),
			args: []string{"--bytes=3", "bytes.txt"},
			want: "789",
		},
		{
			name:     "lines requires an argument",
			fsys:     testFS(),
			args:     []string{"--lines"},
			wantErr:  "tail: option '--lines' requires an argument\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}

	t.Run("follow copies data appended to the file", func(t *testing.T) {
		got, stderr, err := runFollow(t, "one\n", []string{"--follow", "f.txt"}, []string{"two\n"})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "one\ntwo\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if stderr != "" {
			t.Errorf("Run() stderr = %q, want %q", stderr, "")
		}
	})
}

func TestRunErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "missing file",
			fsys:     testFS(),
			args:     []string{"missing.txt"},
			wantErr:  "tail: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "read error",
			stdin:    errorReader{err: errors.New("boom")},
			args:     []string{"-"},
			wantErr:  "tail: -: boom\n",
			wantExit: 1,
		},
		{
			name:     "zero-length file name",
			fsys:     testFS(),
			args:     []string{""},
			wantErr:  "tail: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "no file system available",
			args:     []string{"a.txt"},
			wantErr:  "tail: a.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "no standard input available",
			wantErr:  "tail: standard input: no standard input available\n",
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
			fsys:     testFS(),
			args:     []string{"-x", "a.txt"},
			wantErr:  "tail: invalid option -- 'x'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "unrecognized long option",
			fsys:     testFS(),
			args:     []string{"--bogus", "a.txt"},
			wantErr:  "tail: unrecognized option '--bogus'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "invalid number",
			fsys:     testFS(),
			args:     []string{"-n", "x", "a.txt"},
			wantErr:  "tail: invalid number: 'x'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "non-decimal number",
			fsys:     testFS(),
			args:     []string{"-c", "1.5", "a.txt"},
			wantErr:  "tail: invalid number: '1.5'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "empty number",
			fsys:     testFS(),
			args:     []string{"-n", "", "a.txt"},
			wantErr:  "tail: invalid number: ''\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "plus zero",
			fsys:     testFS(),
			args:     []string{"-n", "+0", "a.txt"},
			wantErr:  "tail: invalid number: '+0'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "plus zero for bytes",
			fsys:     testFS(),
			args:     []string{"-c", "+0", "a.txt"},
			wantErr:  "tail: invalid number: '+0'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "missing option argument",
			fsys:     testFS(),
			args:     []string{"-n"},
			wantErr:  "tail: option requires an argument -- 'n'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "bytes with reverse",
			fsys:     testFS(),
			args:     []string{"-r", "-c", "2", "a.txt"},
			wantErr:  "tail: the -c option is not valid with -r\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "follow with reverse",
			fsys:     testFS(),
			args:     []string{"-r", "-f", "a.txt"},
			wantErr:  "tail: the -f option is not valid with -r\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "signed number with reverse",
			fsys:     testFS(),
			args:     []string{"-r", "-n", "+2", "a.txt"},
			wantErr:  "tail: invalid number: '+2'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a NaN number of seconds",
			fsys:     testFS(),
			args:     []string{"-s", "nan", "-f", "a.txt"},
			wantErr:  "tail: invalid number of seconds: 'nan'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid follow method",
			fsys:     testFS(),
			args:     []string{"--follow=bogus", "a.txt"},
			wantErr:  "tail: invalid argument 'bogus' for '--follow'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid number of seconds",
			fsys:     testFS(),
			args:     []string{"-s", "x", "-f", "a.txt"},
			wantErr:  "tail: invalid number of seconds: 'x'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a negative number of seconds",
			fsys:     testFS(),
			args:     []string{"-s", "-1", "-f", "a.txt"},
			wantErr:  "tail: invalid number of seconds: '-1'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid process id",
			fsys:     testFS(),
			args:     []string{"--pid=x", "-f", "a.txt"},
			wantErr:  "tail: invalid process id: 'x'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a non-positive process id",
			fsys:     testFS(),
			args:     []string{"--pid=0", "-f", "a.txt"},
			wantErr:  "tail: invalid process id: '0'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid max-unchanged-stats",
			fsys:     testFS(),
			args:     []string{"--max-unchanged-stats=x", "-F", "a.txt"},
			wantErr:  "tail: invalid number: 'x'\nTry 'tail --help' for more information.\n",
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
	rt := &coreutils.Runtime{
		Stdout: errorWriter{err: errors.New("boom")},
		Stderr: new(bytes.Buffer),
		FS:     testFS(),
	}
	err := New(rt).Run(t.Context(), "-n", "2", "a.txt")
	if code := exitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if got, want := rt.Stderr.(*bytes.Buffer).String(), "tail: write error: boom\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}

func TestRunContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	rt := &coreutils.Runtime{
		Stdin:  &cancelReader{cancel: cancel},
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     testFS(),
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

func TestRunSizeSuffixes(t *testing.T) {
	// big.txt holds 3000 bytes.
	fsys := fsx.MemFS{"big.txt": {Data: []byte(strings.Repeat("x", 3000))}}
	last := func(n int) string { return strings.Repeat("x", n) }
	tests := []testCase{
		{
			name: "bytes with a binary suffix",
			fsys: fsys,
			args: []string{"-c", "1K", "big.txt"},
			want: last(1024),
		},
		{
			name: "bytes with a binary suffix glued to the option",
			fsys: fsys,
			args: []string{"-c1K", "big.txt"},
			want: last(1024),
		},
		{
			name: "bytes with a binary i suffix",
			fsys: fsys,
			args: []string{"-c", "1KiB", "big.txt"},
			want: last(1024),
		},
		{
			name: "bytes with a decimal suffix",
			fsys: fsys,
			args: []string{"-c", "1KB", "big.txt"},
			want: last(1000),
		},
		{
			name: "bytes with a megabyte suffix counting from the end",
			fsys: fsys,
			args: []string{"-c", "-1K", "big.txt"},
			want: last(1024),
		},
		{
			name: "bytes with a megabyte suffix larger than the file",
			fsys: fsys,
			args: []string{"-c", "1M", "big.txt"},
			want: last(3000),
		},
		{
			name: "bytes with a block suffix",
			fsys: fsys,
			args: []string{"-c", "2b", "big.txt"},
			want: last(1024),
		},
		{
			name: "bytes with a block suffix counting from the start",
			fsys: fsys,
			args: []string{"-c", "+2b", "big.txt"},
			want: last(3000 - 1023),
		},
		{
			name: "lines with a suffix larger than the file",
			fsys: testFS(),
			args: []string{"-n", "1K", "many.txt"},
			want: manyLines(),
		},
		{
			name: "lines with a megabyte suffix",
			fsys: testFS(),
			args: []string{"-n", "2M", "many.txt"},
			want: manyLines(),
		},
		{
			name: "the long forms take suffixes too",
			fsys: fsys,
			args: []string{"--bytes=1K", "big.txt"},
			want: last(1024),
		},
		{
			name:     "an unknown suffix is an invalid number",
			fsys:     testFS(),
			args:     []string{"-n", "5x", "a.txt"},
			wantErr:  "tail: invalid number: '5x'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a suffix without digits is an invalid number",
			fsys:     testFS(),
			args:     []string{"-c", "K", "a.txt"},
			wantErr:  "tail: invalid number: 'K'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a fraction with a suffix is an invalid number",
			fsys:     testFS(),
			args:     []string{"-c", "1.5K", "a.txt"},
			wantErr:  "tail: invalid number: '1.5K'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunZeroTerminated(t *testing.T) {
	fsys := fsx.MemFS{
		"z.txt":     {Data: []byte("a\x00b\x00c\x00")},
		"znonl.txt": {Data: []byte("a\x00b\x00c")},
	}
	tests := []testCase{
		{
			name: "the last items",
			fsys: fsys,
			args: []string{"-z", "-n", "2", "z.txt"},
			want: "b\x00c\x00",
		},
		{
			name: "the final item without a terminator counts as an item",
			fsys: fsys,
			args: []string{"-z", "-n", "1", "znonl.txt"},
			want: "c",
		},
		{
			name: "the last items without a final terminator",
			fsys: fsys,
			args: []string{"-z", "-n", "2", "znonl.txt"},
			want: "b\x00c",
		},
		{
			name: "from an item of the start",
			fsys: fsys,
			args: []string{"-z", "-n", "+2", "znonl.txt"},
			want: "b\x00c",
		},
		{
			name: "from beyond the last item",
			fsys: fsys,
			args: []string{"-z", "-n", "+4", "z.txt"},
			want: "",
		},
		{
			name: "zero items",
			fsys: fsys,
			args: []string{"-z", "-n", "0", "z.txt"},
			want: "",
		},
		{
			name: "the items in reverse",
			fsys: fsys,
			args: []string{"-z", "-r", "znonl.txt"},
			want: "cb\x00a\x00",
		},
		{
			name: "the last items in reverse",
			fsys: fsys,
			args: []string{"-z", "-r", "-n", "2", "z.txt"},
			want: "c\x00b\x00",
		},
		{
			name: "bytes are unaffected",
			fsys: fsys,
			args: []string{"-z", "-c", "2", "znonl.txt"},
			want: "\x00c",
		},
		{
			name: "the long form",
			fsys: fsys,
			args: []string{"--zero-terminated", "-n", "2", "z.txt"},
			want: "b\x00c\x00",
		},
		{
			name:  "the last items of the standard input",
			stdin: strings.NewReader("a\x00b\x00c"),
			args:  []string{"-z", "-n", "2"},
			want:  "b\x00c",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunZeroTerminatedNonSeekable(t *testing.T) {
	tests := []struct {
		name string
		data string
		args []string
		want string
	}{
		{"last items", "a\x00b\x00c", []string{"-z", "-n", "2"}, "b\x00c"},
		{"from an item", "a\x00b\x00c", []string{"-z", "-n", "+2"}, "b\x00c"},
		{"all items in reverse", "a\x00b\x00c", []string{"-z", "-r"}, "cb\x00a\x00"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, testCase{
				fsys: readerFS{r: strings.NewReader(test.data)},
				args: append(test.args, "f.txt"),
				want: test.want,
			})
		})
	}
}

func TestRunObsolete(t *testing.T) {
	tests := []testCase{
		{
			name: "the last lines",
			fsys: testFS(),
			args: []string{"-5", "many.txt"},
			want: "08\n09\n10\n11\n12\n",
		},
		{
			name: "no count defaults to ten lines",
			fsys: testFS(),
			args: []string{"-l", "many.txt"},
			want: "03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n",
		},
		{
			name: "from a line of the start",
			fsys: testFS(),
			args: []string{"+3", "many.txt"},
			want: "03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n",
		},
		{
			name: "plus zero is the whole input",
			fsys: testFS(),
			args: []string{"+0", "lines.txt"},
			want: "l1\nl2\nl3\nl4\nl5\n",
		},
		{
			name: "a line count with a size letter",
			fsys: testFS(),
			args: []string{"-2l", "lines.txt"},
			want: "l4\nl5\n",
		},
		{
			name: "a byte count with a size letter",
			fsys: testFS(),
			args: []string{"-3c", "bytes.txt"},
			want: "789",
		},
		{
			name: "a byte count from the start",
			fsys: testFS(),
			args: []string{"+4c", "bytes.txt"},
			want: "3456789",
		},
		{
			name: "the letter alone defaults to ten",
			fsys: fsx.MemFS{"big.txt": {Data: []byte(strings.Repeat("x", 3000))}},
			args: []string{"-b", "big.txt"},
			want: strings.Repeat("x", 3000),
		},
		{
			name: "a block count",
			fsys: fsx.MemFS{"big.txt": {Data: []byte(strings.Repeat("x", 3000))}},
			args: []string{"-2b", "big.txt"},
			want: strings.Repeat("x", 1024),
		},
		{
			name: "a block count from the start",
			fsys: fsx.MemFS{"big.txt": {Data: []byte(strings.Repeat("x", 3000))}},
			args: []string{"+2b", "big.txt"},
			want: strings.Repeat("x", 3000-1023),
		},
		{
			name:  "a count of the standard input operand",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-2", "-"},
			want:  "b\nc\n",
		},
		{
			name: "double dash still ends the options",
			fsys: testFS(),
			args: []string{"-2", "--", "lines.txt"},
			want: "l4\nl5\n",
		},
		{
			name: "double dash allows a file named like an option",
			fsys: testFS(),
			args: []string{"-2", "--", "-f"},
			want: "dash-f\n",
		},
		{
			name: "the options of the standard syntax still apply",
			fsys: testFS(),
			args: []string{"-2", "a.txt"},
			want: "d\ne\n",
		},
		{
			name: "options after the obsolete form are rejected",
			fsys: fsx.MemFS{
				"a.txt": {Data: []byte("a\n")},
				"b.txt": {Data: []byte("b\n")},
			},
			args:     []string{"-1", "--", "-q", "a.txt", "b.txt"},
			wantErr:  "tail: option used in invalid context -- '1'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunObsoleteFollow(t *testing.T) {
	t.Run("a trailing f follows the file", func(t *testing.T) {
		got, stderr, err := runFollow(t, "one\ntwo\nthree\n", []string{"-2f", "f.txt"}, []string{"four\n"})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "two\nthree\nfour\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if stderr != "" {
			t.Errorf("Run() stderr = %q, want %q", stderr, "")
		}
	})

	t.Run("a trailing f after a size letter follows the file", func(t *testing.T) {
		got, _, err := runFollow(t, "one\ntwo\nthree\n", []string{"-2lf", "f.txt"}, []string{"four\n"})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "two\nthree\nfour\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("a leading plus follows from the start", func(t *testing.T) {
		got, _, err := runFollow(t, "one\ntwo\n", []string{"+2f", "f.txt"}, []string{"three\n"})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "two\nthree\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}

func TestRunObsoleteInvalid(t *testing.T) {
	tests := []testCase{
		{
			name:     "a digit after other options",
			fsys:     testFS(),
			args:     []string{"-n", "2", "-5", "a.txt"},
			wantErr:  "tail: option used in invalid context -- '5'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a digit before other options",
			fsys:     testFS(),
			args:     []string{"-5", "-n", "2", "a.txt"},
			wantErr:  "tail: option used in invalid context -- '5'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a digit with two file operands",
			fsys:     testFS(),
			args:     []string{"-5", "a.txt", "lines.txt"},
			wantErr:  "tail: option used in invalid context -- '5'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a letter which is not a size letter",
			fsys:     testFS(),
			args:     []string{"-5x", "a.txt"},
			wantErr:  "tail: option used in invalid context -- '5'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "the standard form takes over a glued count",
			fsys:     testFS(),
			args:     []string{"-c5", "bytes.txt"},
			want:     "56789",
			wantExit: 0,
		},
		{
			name:     "an option needing its argument is left alone",
			fsys:     testFS(),
			args:     []string{"-c"},
			wantErr:  "tail: option requires an argument -- 'c'\nTry 'tail --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an operand which starts with a digit",
			fsys:     testFS(),
			args:     []string{"-n", "2", "5"},
			wantErr:  "tail: 5: file does not exist\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
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
		want := "tail (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
