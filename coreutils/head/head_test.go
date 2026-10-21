package head

import (
	"bytes"
	"context"
	"errors"
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
//	"-n":     "dash-n\n" (a file named like an option)
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
		"-n":            {Data: []byte("dash-n\n")},
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
			wantErr:  "head: option '--lines' requires an argument\n",
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
			wantErr:  "head: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "unrecognized long option",
			args:     []string{"--number", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: unrecognized option '--number'\n",
			wantExit: 2,
		},
		{
			name:     "invalid number",
			args:     []string{"-n", "x", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: invalid number: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "negative number",
			args:     []string{"-n", "-5", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: invalid number: '-5'\n",
			wantExit: 2,
		},
		{
			name:     "signed number",
			args:     []string{"-n", "+5", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: invalid number: '+5'\n",
			wantExit: 2,
		},
		{
			name:     "missing option argument",
			args:     []string{"-n"},
			fsys:     testFS(),
			wantErr:  "head: option requires an argument -- 'n'\n",
			wantExit: 2,
		},
		{
			name:     "non-decimal number",
			args:     []string{"-c", "1.5", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: invalid number: '1.5'\n",
			wantExit: 2,
		},
		{
			name:     "empty number",
			args:     []string{"-n", "", "a.txt"},
			fsys:     testFS(),
			wantErr:  "head: invalid number: ''\n",
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
