package comm

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
//	left.txt:      "apple\nbanana\ncherry\ndate\n"
//	right.txt:     "banana\ncherry\nelderberry\n"
//	same1.txt:     "a\nb\nc\n"
//	same2.txt:     "a\nb\nc\n"
//	empty.txt:     ""
//	nonl1.txt:     "a\nb" (no trailing newline)
//	nonl2.txt:     "a\nb\n"
//	"-1":          "x\n" (a file named like an option)
//	sub/inner.txt: "i\nj\n"
//	sub/other.txt: "j\nk\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"left.txt":      {Data: []byte("apple\nbanana\ncherry\ndate\n")},
		"right.txt":     {Data: []byte("banana\ncherry\nelderberry\n")},
		"same1.txt":     {Data: []byte("a\nb\nc\n")},
		"same2.txt":     {Data: []byte("a\nb\nc\n")},
		"empty.txt":     {Data: []byte("")},
		"nonl1.txt":     {Data: []byte("a\nb")},
		"nonl2.txt":     {Data: []byte("a\nb\n")},
		"-1":            {Data: []byte("x\n")},
		"sub/inner.txt": {Data: []byte("i\nj\n")},
		"sub/other.txt": {Data: []byte("j\nk\n")},
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
	tests := []testCase{
		{
			name: "three columns",
			fsys: testFS(),
			args: []string{"left.txt", "right.txt"},
			want: "apple\n\t\tbanana\n\t\tcherry\ndate\n\telderberry\n",
		},
		{
			// A missing final newline is treated as present: the
			// unterminated "b" equals the terminated one, and the output
			// is still newline-terminated.
			name: "a missing final newline takes part in comparisons",
			fsys: testFS(),
			args: []string{"nonl1.txt", "nonl2.txt"},
			want: "\t\ta\n\t\tb\n",
		},
		{
			name: "a missing final newline is terminated in the output",
			fsys: testFS(),
			args: []string{"nonl1.txt", "nonl1.txt"},
			want: "\t\ta\n\t\tb\n",
		},
		{
			name: "identical files are all in the third column",
			fsys: testFS(),
			args: []string{"same1.txt", "same2.txt"},
			want: "\t\ta\n\t\tb\n\t\tc\n",
		},
		{
			name: "an empty second file leaves every line unique to the first",
			fsys: testFS(),
			args: []string{"left.txt", "empty.txt"},
			want: "apple\nbanana\ncherry\ndate\n",
		},
		{
			name: "an empty first file leaves every line unique to the second",
			fsys: testFS(),
			args: []string{"empty.txt", "right.txt"},
			want: "\tbanana\n\tcherry\n\telderberry\n",
		},
		{
			name: "two empty files produce no output",
			fsys: testFS(),
			args: []string{"empty.txt", "empty.txt"},
		},
		{
			name:  "the first file can be the standard input",
			fsys:  testFS(),
			stdin: strings.NewReader("apple\ncherry\n"),
			args:  []string{"-", "right.txt"},
			want:  "apple\n\tbanana\n\t\tcherry\n\telderberry\n",
		},
		{
			name:  "the second file can be the standard input",
			fsys:  testFS(),
			stdin: strings.NewReader("cherry\nelderberry\n"),
			args:  []string{"left.txt", "-"},
			want:  "apple\nbanana\n\t\tcherry\ndate\n\telderberry\n",
		},
		{
			name: "a double dash ends the options",
			fsys: testFS(),
			args: []string{"--", "left.txt", "right.txt"},
			want: "apple\n\t\tbanana\n\t\tcherry\ndate\n\telderberry\n",
		},
		{
			name: "options are not parsed after the first operand",
			fsys: testFS(),
			args: []string{"left.txt", "-1"},
			want: "apple\nbanana\ncherry\ndate\n\tx\n",
		},
		{
			name: "operands resolve against the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt", "other.txt"},
			want: "i\n\t\tj\n\tk\n",
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
			name: "-1 suppresses the first column",
			fsys: testFS(),
			args: []string{"-1", "left.txt", "right.txt"},
			want: "\tbanana\n\tcherry\nelderberry\n",
		},
		{
			name: "-2 suppresses the second column",
			fsys: testFS(),
			args: []string{"-2", "left.txt", "right.txt"},
			want: "apple\n\tbanana\n\tcherry\ndate\n",
		},
		{
			name: "-3 suppresses the third column",
			fsys: testFS(),
			args: []string{"-3", "left.txt", "right.txt"},
			want: "apple\ndate\n\telderberry\n",
		},
		{
			name: "-12 writes only the common lines",
			fsys: testFS(),
			args: []string{"-12", "left.txt", "right.txt"},
			want: "banana\ncherry\n",
		},
		{
			name: "-13 writes only the lines unique to the second file",
			fsys: testFS(),
			args: []string{"-13", "left.txt", "right.txt"},
			want: "elderberry\n",
		},
		{
			name: "-23 writes only the lines unique to the first file",
			fsys: testFS(),
			args: []string{"-23", "left.txt", "right.txt"},
			want: "apple\ndate\n",
		},
		{
			name: "-123 writes nothing",
			fsys: testFS(),
			args: []string{"-123", "left.txt", "right.txt"},
		},
		{
			name: "options can be given one by one",
			fsys: testFS(),
			args: []string{"-1", "-2", "left.txt", "right.txt"},
			want: "banana\ncherry\n",
		},
		{
			name: "an option can be repeated",
			fsys: testFS(),
			args: []string{"-1", "-1", "left.txt", "right.txt"},
			want: "\tbanana\n\tcherry\nelderberry\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOutputDelimiter(t *testing.T) {
	tests := []testCase{
		{
			name: "--output-delimiter replaces the tabs between columns",
			fsys: testFS(),
			args: []string{"--output-delimiter=|", "left.txt", "right.txt"},
			want: "apple\n||banana\n||cherry\ndate\n|elderberry\n",
		},
		{
			name: "the delimiter can be given in the next argument",
			fsys: testFS(),
			args: []string{"--output-delimiter", "|", "left.txt", "right.txt"},
			want: "apple\n||banana\n||cherry\ndate\n|elderberry\n",
		},
		{
			name: "an empty delimiter selects NUL",
			fsys: testFS(),
			args: []string{"--output-delimiter=", "left.txt", "right.txt"},
			want: "apple\n\x00\x00banana\n\x00\x00cherry\ndate\n\x00elderberry\n",
		},
		{
			name: "a suppressed column removes its delimiter",
			fsys: testFS(),
			args: []string{"-1", "--output-delimiter=|", "left.txt", "right.txt"},
			want: "|banana\n|cherry\nelderberry\n",
		},
		{
			name: "two suppressed columns leave no delimiters",
			fsys: testFS(),
			args: []string{"-12", "--output-delimiter=|", "left.txt", "right.txt"},
			want: "banana\ncherry\n",
		},
		{
			name: "the last delimiter given is the one used",
			fsys: testFS(),
			args: []string{"--output-delimiter=|", "--output-delimiter=+", "left.txt", "right.txt"},
			want: "apple\n++banana\n++cherry\ndate\n+elderberry\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunZeroTerminated(t *testing.T) {
	zfs := fsx.MemFS{
		"z1.txt": {Data: []byte("apple\x00date\x00")},
		"z2.txt": {Data: []byte("date\x00elderberry\x00")},
	}
	tests := []testCase{
		{
			name: "-z reads and writes NUL-terminated items",
			fsys: zfs,
			args: []string{"-z", "z1.txt", "z2.txt"},
			want: "apple\x00\t\tdate\x00\telderberry\x00",
		},
		{
			name: "--zero-terminated is the long form of -z",
			fsys: zfs,
			args: []string{"--zero-terminated", "z1.txt", "z2.txt"},
			want: "apple\x00\t\tdate\x00\telderberry\x00",
		},
		{
			name: "-z can be grouped with other options",
			fsys: zfs,
			args: []string{"-1z", "z1.txt", "z2.txt"},
			want: "\tdate\x00elderberry\x00",
		},
		{
			name: "-z can use a custom output delimiter",
			fsys: zfs,
			args: []string{"-z", "--output-delimiter=|", "z1.txt", "z2.txt"},
			want: "apple\x00||date\x00|elderberry\x00",
		},
		{
			// A missing final NUL is treated as present, so the
			// unterminated "b" equals the terminated one.
			name: "a missing final NUL takes part in comparisons",
			fsys: fsx.MemFS{
				"z1.txt": {Data: []byte("a\x00b")},
				"z2.txt": {Data: []byte("a\x00b\x00")},
			},
			args: []string{"-z", "z1.txt", "z2.txt"},
			want: "\t\ta\x00\t\tb\x00",
		},
		{
			name: "newlines are ordinary bytes under -z",
			fsys: fsx.MemFS{"z1.txt": {Data: []byte("a\nb\x00")}, "z2.txt": {Data: []byte("a\nb\x00")}},
			args: []string{"-z", "z1.txt", "z2.txt"},
			want: "\t\ta\nb\x00",
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
			wantErr:  "comm: missing operand\n",
			wantExit: 2,
		},
		{
			name:     "one operand",
			fsys:     testFS(),
			args:     []string{"left.txt"},
			wantErr:  "comm: missing operand\n",
			wantExit: 2,
		},
		{
			name:     "an extra operand",
			fsys:     testFS(),
			args:     []string{"left.txt", "right.txt", "third.txt"},
			wantErr:  "comm: extra operand 'third.txt'\n",
			wantExit: 2,
		},
		{
			name:     "an invalid option",
			fsys:     testFS(),
			args:     []string{"-x", "left.txt", "right.txt"},
			wantErr:  "comm: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "a long option",
			fsys:     testFS(),
			args:     []string{"--total", "left.txt", "right.txt"},
			wantErr:  "comm: unrecognized option '--total'\n",
			wantExit: 2,
		},
		{
			name:     "a short option that only exists in long form",
			fsys:     testFS(),
			args:     []string{"-o", "left.txt", "right.txt"},
			wantErr:  "comm: invalid option -- 'o'\n",
			wantExit: 2,
		},
		{
			name:     "a long option missing its argument",
			fsys:     testFS(),
			args:     []string{"--output-delimiter"},
			wantErr:  "comm: option '--output-delimiter' requires an argument\n",
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
			args:     []string{"missing.txt", "right.txt"},
			wantErr:  "comm: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "missing second file",
			fsys:     testFS(),
			args:     []string{"left.txt", "missing.txt"},
			wantErr:  "comm: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "the first missing file is reported when both are missing",
			fsys:     testFS(),
			args:     []string{"missing1.txt", "missing2.txt"},
			wantErr:  "comm: missing1.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "directory input",
			fsys:     fsx.MemFS{"dir": {Mode: fs.ModeDir}, "right.txt": {Data: []byte("a\n")}},
			args:     []string{"dir", "right.txt"},
			wantErr:  "comm: dir: invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "input file without a file system",
			args:     []string{"left.txt", "right.txt"},
			wantErr:  "comm: left.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			fsys:     testFS(),
			args:     []string{"-", "right.txt"},
			wantErr:  "comm: -: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "standard input read error",
			fsys:     testFS(),
			stdin:    errorReader{err: errors.New("read failed")},
			args:     []string{"-", "right.txt"},
			wantErr:  "comm: -: read failed\n",
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
		Stdin:  strings.NewReader("a\n"),
		Stdout: errorWriter{err: writeErr},
		Stderr: stderr,
		FS:     fsx.MemFS{"empty.txt": {Data: []byte("")}},
	}
	err := New(rt).Run(t.Context(), "-", "empty.txt")

	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
	if got := stderr.String(); got != "comm: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "comm: write error: write failed\n")
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
		FS:     fsx.MemFS{"right.txt": {Data: []byte("hello\n")}},
	}
	err := New(rt).Run(ctx, "-", "right.txt")

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
