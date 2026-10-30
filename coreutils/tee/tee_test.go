package tee

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
//	a.txt:         "alpha\n"
//	out.txt:       "old contents\n" (truncated or appended to)
//	"-a":          "dash-a\n" (a file named like an option)
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":         {Data: []byte("alpha\n")},
		"out.txt":       {Data: []byte("old contents\n")},
		"-a":            {Data: []byte("dash-a\n")},
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
	name      string
	stdin     io.Reader
	fsys      fsx.MemFS
	dir       string
	args      []string
	want      string
	wantErr   string
	wantExit  int
	wantFiles map[string]string
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
	for name, want := range test.wantFiles {
		f := test.fsys[name]
		if f == nil {
			t.Errorf("Run() didn't create file %q", name)
			continue
		}
		if got := string(f.Data); got != want {
			t.Errorf("Run() file %q = %q, want %q", name, got, want)
		}
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			name:  "no operands copies the standard input to the standard output",
			stdin: strings.NewReader("hello\n"),
			want:  "hello\n",
		},
		{
			name:  "empty standard input",
			stdin: strings.NewReader(""),
			want:  "",
		},
		{
			name:      "one file gets a copy of the standard input",
			stdin:     strings.NewReader("hello\n"),
			fsys:      testFS(),
			args:      []string{"new.txt"},
			want:      "hello\n",
			wantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			name:  "multiple files each get the full input",
			stdin: strings.NewReader("hello\n"),
			fsys:  testFS(),
			args:  []string{"one.txt", "two.txt", "three.txt"},
			want:  "hello\n",
			wantFiles: map[string]string{
				"one.txt":   "hello\n",
				"two.txt":   "hello\n",
				"three.txt": "hello\n",
			},
		},
		{
			name:      "empty standard input creates an empty file",
			stdin:     strings.NewReader(""),
			fsys:      testFS(),
			args:      []string{"new.txt"},
			want:      "",
			wantFiles: map[string]string{"new.txt": ""},
		},
		{
			name:      "arbitrary bytes are copied untouched",
			stdin:     strings.NewReader(string([]byte{0x00, 0x01, 0xff, '\n'})),
			fsys:      testFS(),
			args:      []string{"new.txt"},
			want:      string([]byte{0x00, 0x01, 0xff, '\n'}),
			wantFiles: map[string]string{"new.txt": string([]byte{0x00, 0x01, 0xff, '\n'})},
		},
		{
			name:      "an existing file is truncated before copying",
			stdin:     strings.NewReader("new\n"),
			fsys:      testFS(),
			args:      []string{"out.txt"},
			want:      "new\n",
			wantFiles: map[string]string{"out.txt": "new\n"},
		},
		{
			name:      "pathnames are resolved from the working directory",
			stdin:     strings.NewReader("inner copy\n"),
			fsys:      testFS(),
			dir:       "sub",
			args:      []string{"out.txt"},
			want:      "inner copy\n",
			wantFiles: map[string]string{"sub/out.txt": "inner copy\n"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunAppend(t *testing.T) {
	tests := []testCase{
		{
			name:      "a appends to an existing file",
			stdin:     strings.NewReader("more\n"),
			fsys:      testFS(),
			args:      []string{"-a", "out.txt"},
			want:      "more\n",
			wantFiles: map[string]string{"out.txt": "old contents\nmore\n"},
		},
		{
			name:      "a creates a missing file",
			stdin:     strings.NewReader("hello\n"),
			fsys:      testFS(),
			args:      []string{"-a", "new.txt"},
			want:      "hello\n",
			wantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			name:  "a appends to multiple files",
			stdin: strings.NewReader("more\n"),
			fsys:  testFS(),
			args:  []string{"-a", "out.txt", "-a"},
			want:  "more\n",
			wantFiles: map[string]string{
				"out.txt": "old contents\nmore\n",
				"-a":      "dash-a\nmore\n",
			},
		},
		{
			name:      "repeated a",
			stdin:     strings.NewReader("more\n"),
			fsys:      testFS(),
			args:      []string{"-a", "-a", "out.txt"},
			want:      "more\n",
			wantFiles: map[string]string{"out.txt": "old contents\nmore\n"},
		},
		{
			name:      "grouped a and i",
			stdin:     strings.NewReader("more\n"),
			fsys:      testFS(),
			args:      []string{"-ai", "out.txt"},
			want:      "more\n",
			wantFiles: map[string]string{"out.txt": "old contents\nmore\n"},
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
			name:  "i is accepted and has no effect",
			stdin: strings.NewReader("hello\n"),
			fsys:  testFS(),
			args:  []string{"-i", "new.txt"},
			want:  "hello\n",
			wantFiles: map[string]string{
				"new.txt": "hello\n",
			},
		},
		{
			name:      "a file named like an option after a double dash",
			stdin:     strings.NewReader("hello\n"),
			fsys:      testFS(),
			args:      []string{"--", "-i"},
			want:      "hello\n",
			wantFiles: map[string]string{"-i": "hello\n"},
		},
		{
			name:  "options are not parsed after the first operand",
			stdin: strings.NewReader("hello\n"),
			fsys:  testFS(),
			args:  []string{"new.txt", "-a"},
			want:  "hello\n",
			wantFiles: map[string]string{
				"new.txt": "hello\n",
				"-a":      "hello\n",
			},
		},
		{
			name:     "invalid option",
			args:     []string{"-x"},
			wantErr:  "tee: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "invalid option in a group",
			args:     []string{"-ax"},
			wantErr:  "tee: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "unknown long option",
			args:     []string{"--foo"},
			wantErr:  "tee: unrecognized option '--foo'\n",
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
			name:      "append",
			stdin:     strings.NewReader("more\n"),
			fsys:      testFS(),
			args:      []string{"--append", "out.txt"},
			want:      "more\n",
			wantFiles: map[string]string{"out.txt": "old contents\nmore\n"},
		},
		{
			name:  "ignore-interrupts",
			stdin: strings.NewReader("hello\n"),
			fsys:  testFS(),
			args:  []string{"--ignore-interrupts", "new.txt"},
			want:  "hello\n",
			wantFiles: map[string]string{
				"new.txt": "hello\n",
			},
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
			name:     "missing parent directory",
			stdin:    strings.NewReader("hello\n"),
			fsys:     testFS(),
			args:     []string{"no/such/out.txt"},
			want:     "hello\n",
			wantErr:  "tee: no/such/out.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a file as parent directory",
			stdin:    strings.NewReader("hello\n"),
			fsys:     testFS(),
			args:     []string{"a.txt/out.txt"},
			want:     "hello\n",
			wantErr:  "tee: a.txt/out.txt: not a directory\n",
			wantExit: 1,
		},
		{
			name:     "directory operand",
			stdin:    strings.NewReader("hello\n"),
			fsys:     fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			args:     []string{"dir"},
			want:     "hello\n",
			wantErr:  "tee: dir: invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "zero-length file name",
			stdin:    strings.NewReader("hello\n"),
			fsys:     testFS(),
			args:     []string{""},
			want:     "hello\n",
			wantErr:  "tee: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "every unwritable file reports an error",
			stdin:    strings.NewReader("hello\n"),
			fsys:     testFS(),
			args:     []string{"no/such/one.txt", "no/such/two.txt"},
			want:     "hello\n",
			wantErr:  "tee: no/such/one.txt: file does not exist\ntee: no/such/two.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:      "an unwritable file doesn't stop the other files",
			stdin:     strings.NewReader("hello\n"),
			fsys:      testFS(),
			args:      []string{"no/such/out.txt", "good.txt"},
			want:      "hello\n",
			wantErr:   "tee: no/such/out.txt: file does not exist\n",
			wantExit:  1,
			wantFiles: map[string]string{"good.txt": "hello\n"},
		},
		{
			name:     "file operand without a file system",
			stdin:    strings.NewReader("hello\n"),
			args:     []string{"out.txt"},
			want:     "hello\n",
			wantErr:  "tee: out.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			fsys:     testFS(),
			wantErr:  "tee: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "standard input read error",
			stdin:    errorReader{err: errors.New("read failed")},
			fsys:     testFS(),
			wantErr:  "tee: standard input: read failed\n",
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

	fsys := testFS()
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("hello\n"),
		Stdout: errorWriter{err: writeErr},
		Stderr: stderr,
		FS:     fsys,
	}
	err := New(rt).Run(t.Context(), "out.txt")

	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if got := stderr.String(); got != "tee: standard output: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "tee: standard output: write failed\n")
	}
	if got := string(fsys["out.txt"].Data); got != "hello\n" {
		t.Errorf("Run() file out.txt = %q, want %q", got, "hello\n")
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
		want := "tee (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
