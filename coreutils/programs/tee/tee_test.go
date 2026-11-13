package tee

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// pipeInput holds a few one byte chunks and then ends, with io.EOF, or with
// an error when fail is set: only a run which fails to stop copying early, as
// the pipe modes ask, ever sees that error.
type pipeInput struct {
	left int
	fail bool
}

func (r *pipeInput) Read(b []byte) (int, error) {
	if r.left == 0 {
		if r.fail {
			return 0, errors.New("input still open")
		}
		return 0, io.EOF
	}
	r.left--
	b[0] = 'x'
	return 1, nil
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

// testCase is the fields of a testutil.Case plus a standard output the case
// brings itself: testutil.Once always captures the output on a buffer, so the
// tests of a failing writer, which take an errorWriter, need this one. Want
// is then unchecked, like in the runner.
type testCase struct {
	Name      string
	Stdin     io.Reader
	FS        fsx.MemFS
	Dir       string
	Args      []string
	Want      string
	WantErr   string
	WantExit  int
	WantFiles map[string]string
	Absent    []string

	Stdout io.Writer
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	if test.Stdout == nil {
		c := testutil.Case{
			Name:      test.Name,
			Stdin:     test.Stdin,
			Dir:       test.Dir,
			Args:      test.Args,
			Want:      test.Want,
			WantErr:   test.WantErr,
			WantExit:  test.WantExit,
			WantFiles: test.WantFiles,
			Absent:    test.Absent,
		}
		if test.FS != nil {
			c.FS = test.FS
		}
		testutil.Once(t, New, c)
		return
	}

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      test.Stdin,
		Stdout:     test.Stdout,
		Stderr:     stderr,
		FS:         test.FS,
		WorkingDir: test.Dir,
	}
	err := New(rt).Run(t.Context(), test.Args...)

	if code := testutil.ExitCode(t, err); code != test.WantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.WantExit, err)
	}
	if got := stderr.String(); got != test.WantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.WantErr)
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			Name:  "no operands copies the standard input to the standard output",
			Stdin: strings.NewReader("hello\n"),
			Want:  "hello\n",
		},
		{
			Name:  "empty standard input",
			Stdin: strings.NewReader(""),
			Want:  "",
		},
		{
			Name:      "one file gets a copy of the standard input",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Args:      []string{"new.txt"},
			Want:      "hello\n",
			WantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			Name:  "multiple files each get the full input",
			Stdin: strings.NewReader("hello\n"),
			FS:    testFS(),
			Args:  []string{"one.txt", "two.txt", "three.txt"},
			Want:  "hello\n",
			WantFiles: map[string]string{
				"one.txt":   "hello\n",
				"two.txt":   "hello\n",
				"three.txt": "hello\n",
			},
		},
		{
			Name:      "empty standard input creates an empty file",
			Stdin:     strings.NewReader(""),
			FS:        testFS(),
			Args:      []string{"new.txt"},
			Want:      "",
			WantFiles: map[string]string{"new.txt": ""},
		},
		{
			Name:      "arbitrary bytes are copied untouched",
			Stdin:     strings.NewReader(string([]byte{0x00, 0x01, 0xff, '\n'})),
			FS:        testFS(),
			Args:      []string{"new.txt"},
			Want:      string([]byte{0x00, 0x01, 0xff, '\n'}),
			WantFiles: map[string]string{"new.txt": string([]byte{0x00, 0x01, 0xff, '\n'})},
		},
		{
			Name:      "an existing file is truncated before copying",
			Stdin:     strings.NewReader("new\n"),
			FS:        testFS(),
			Args:      []string{"out.txt"},
			Want:      "new\n",
			WantFiles: map[string]string{"out.txt": "new\n"},
		},
		{
			Name:      "pathnames are resolved from the working directory",
			Stdin:     strings.NewReader("inner copy\n"),
			FS:        testFS(),
			Dir:       "sub",
			Args:      []string{"out.txt"},
			Want:      "inner copy\n",
			WantFiles: map[string]string{"sub/out.txt": "inner copy\n"},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunAppend(t *testing.T) {
	tests := []testCase{
		{
			Name:      "a appends to an existing file",
			Stdin:     strings.NewReader("more\n"),
			FS:        testFS(),
			Args:      []string{"-a", "out.txt"},
			Want:      "more\n",
			WantFiles: map[string]string{"out.txt": "old contents\nmore\n"},
		},
		{
			Name:      "a creates a missing file",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Args:      []string{"-a", "new.txt"},
			Want:      "hello\n",
			WantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			Name:  "a appends to multiple files",
			Stdin: strings.NewReader("more\n"),
			FS:    testFS(),
			Args:  []string{"-a", "out.txt", "-a"},
			Want:  "more\n",
			WantFiles: map[string]string{
				"out.txt": "old contents\nmore\n",
				"-a":      "dash-a\nmore\n",
			},
		},
		{
			Name:      "repeated a",
			Stdin:     strings.NewReader("more\n"),
			FS:        testFS(),
			Args:      []string{"-a", "-a", "out.txt"},
			Want:      "more\n",
			WantFiles: map[string]string{"out.txt": "old contents\nmore\n"},
		},
		{
			Name:      "grouped a and i",
			Stdin:     strings.NewReader("more\n"),
			FS:        testFS(),
			Args:      []string{"-ai", "out.txt"},
			Want:      "more\n",
			WantFiles: map[string]string{"out.txt": "old contents\nmore\n"},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOptions(t *testing.T) {
	tests := []testCase{
		{
			Name:  "i is accepted and has no effect",
			Stdin: strings.NewReader("hello\n"),
			FS:    testFS(),
			Args:  []string{"-i", "new.txt"},
			Want:  "hello\n",
			WantFiles: map[string]string{
				"new.txt": "hello\n",
			},
		},
		{
			Name:      "a file named like an option after a double dash",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Args:      []string{"--", "-i"},
			Want:      "hello\n",
			WantFiles: map[string]string{"-i": "hello\n"},
		},
		{
			Name:  "options are not parsed after the first operand",
			Stdin: strings.NewReader("hello\n"),
			FS:    testFS(),
			Args:  []string{"new.txt", "-a"},
			Want:  "hello\n",
			WantFiles: map[string]string{
				"new.txt": "hello\n",
				"-a":      "hello\n",
			},
		},
		{
			Name:     "invalid option",
			Args:     []string{"-x"},
			WantErr:  "tee: invalid option -- 'x'\nTry 'tee --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option in a group",
			Args:     []string{"-ax"},
			WantErr:  "tee: invalid option -- 'x'\nTry 'tee --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unknown long option",
			Args:     []string{"--foo"},
			WantErr:  "tee: unrecognized option '--foo'\nTry 'tee --help' for more information.\n",
			WantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLongOptions(t *testing.T) {
	tests := []testCase{
		{
			Name:      "append",
			Stdin:     strings.NewReader("more\n"),
			FS:        testFS(),
			Args:      []string{"--append", "out.txt"},
			Want:      "more\n",
			WantFiles: map[string]string{"out.txt": "old contents\nmore\n"},
		},
		{
			Name:  "ignore-interrupts",
			Stdin: strings.NewReader("hello\n"),
			FS:    testFS(),
			Args:  []string{"--ignore-interrupts", "new.txt"},
			Want:  "hello\n",
			WantFiles: map[string]string{
				"new.txt": "hello\n",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOutputErrorOption(t *testing.T) {
	tests := []testCase{
		{
			Name:      "p is accepted",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Args:      []string{"-p", "new.txt"},
			Want:      "hello\n",
			WantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			Name:      "p in a group",
			Stdin:     strings.NewReader("more\n"),
			FS:        testFS(),
			Args:      []string{"-ap", "out.txt"},
			Want:      "more\n",
			WantFiles: map[string]string{"out.txt": "old contents\nmore\n"},
		},
		{
			Name:      "p takes no option-argument",
			Stdin:     strings.NewReader("more\n"),
			FS:        testFS(),
			Args:      []string{"-p", "-a", "out.txt"},
			Want:      "more\n",
			WantFiles: map[string]string{"out.txt": "old contents\nmore\n"},
		},
		{
			Name:      "output-error=warn",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Args:      []string{"--output-error=warn", "new.txt"},
			Want:      "hello\n",
			WantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			Name:      "output-error=warn-nopipe",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Args:      []string{"--output-error=warn-nopipe", "new.txt"},
			Want:      "hello\n",
			WantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			Name:      "output-error=exit",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Args:      []string{"--output-error=exit", "new.txt"},
			Want:      "hello\n",
			WantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			Name:      "output-error=exit-nopipe",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Args:      []string{"--output-error=exit-nopipe", "new.txt"},
			Want:      "hello\n",
			WantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			Name:      "output-error without a mode is warn-nopipe",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Args:      []string{"--output-error", "new.txt"},
			Want:      "hello\n",
			WantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			Name:     "an invalid output error mode",
			Args:     []string{"--output-error=bogus"},
			WantErr:  "tee: invalid argument 'bogus' for '--output-error'\nTry 'tee --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an output error mode with the wrong case",
			Args:     []string{"--output-error=WARN"},
			WantErr:  "tee: invalid argument 'WARN' for '--output-error'\nTry 'tee --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an output error mode held by the short option",
			Args:     []string{"-p=bogus"},
			WantErr:  "tee: invalid option -- '='\nTry 'tee --help' for more information.\n",
			WantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOutputErrorModes(t *testing.T) {
	writeErr := errors.New("write failed")

	// The standard output of every case fails, and it's never a pipe, so
	// only the exit status of each mode is at stake here.
	tests := []testCase{
		{
			Name:      "the default mode diagnoses and keeps writing",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Stdout:    testutil.ErrorWriter{Err: writeErr},
			Args:      []string{"new.txt"},
			WantErr:   "tee: standard output: write failed\n",
			WantExit:  1,
			WantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			Name:      "warn diagnoses and keeps writing",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Stdout:    testutil.ErrorWriter{Err: writeErr},
			Args:      []string{"--output-error=warn", "new.txt"},
			WantErr:   "tee: standard output: write failed\n",
			WantExit:  1,
			WantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			Name:      "warn-nopipe diagnoses a non pipe and keeps writing",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Stdout:    testutil.ErrorWriter{Err: writeErr},
			Args:      []string{"-p", "new.txt"},
			WantErr:   "tee: standard output: write failed\n",
			WantExit:  1,
			WantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			Name:      "exit stops at the first error",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Stdout:    testutil.ErrorWriter{Err: writeErr},
			Args:      []string{"--output-error=exit", "new.txt"},
			WantErr:   "tee: standard output: write failed\n",
			WantExit:  1,
			WantFiles: map[string]string{"new.txt": ""},
		},
		{
			Name:      "exit-nopipe stops at the error of a non pipe",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Stdout:    testutil.ErrorWriter{Err: writeErr},
			Args:      []string{"--output-error=exit-nopipe", "new.txt"},
			WantErr:   "tee: standard output: write failed\n",
			WantExit:  1,
			WantFiles: map[string]string{"new.txt": ""},
		},
		{
			Name:      "the last mode given wins with p last",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Stdout:    testutil.ErrorWriter{Err: writeErr},
			Args:      []string{"--output-error=exit", "-p", "new.txt"},
			WantErr:   "tee: standard output: write failed\n",
			WantExit:  1,
			WantFiles: map[string]string{"new.txt": "hello\n"},
		},
		{
			Name:      "the last mode given wins with exit last",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Stdout:    testutil.ErrorWriter{Err: writeErr},
			Args:      []string{"-p", "--output-error=exit", "new.txt"},
			WantErr:   "tee: standard output: write failed\n",
			WantExit:  1,
			WantFiles: map[string]string{"new.txt": ""},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOutputErrorOpen(t *testing.T) {
	tests := []testCase{
		{
			Name:      "warn keeps the other files after a failed open",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Args:      []string{"--output-error=warn", "no/such/out.txt", "good.txt"},
			Want:      "hello\n",
			WantErr:   "tee: no/such/out.txt: file does not exist\n",
			WantExit:  1,
			WantFiles: map[string]string{"good.txt": "hello\n"},
		},
		{
			Name:     "exit stops at a failed open",
			Stdin:    strings.NewReader("hello\n"),
			FS:       testFS(),
			Args:     []string{"--output-error=exit", "no/such/out.txt", "good.txt"},
			WantErr:  "tee: no/such/out.txt: file does not exist\n",
			WantExit: 1,
			Absent:   []string{"good.txt"},
		},
		{
			Name:     "exit-nopipe stops at a failed open",
			Stdin:    strings.NewReader("hello\n"),
			FS:       testFS(),
			Args:     []string{"--output-error=exit-nopipe", "no/such/out.txt", "good.txt"},
			WantErr:  "tee: no/such/out.txt: file does not exist\n",
			WantExit: 1,
			Absent:   []string{"good.txt"},
		},
		{
			Name:     "exit stops at a zero-length file name",
			Stdin:    strings.NewReader("hello\n"),
			FS:       testFS(),
			Args:     []string{"--output-error=exit", ""},
			WantErr:  "tee: invalid zero-length file name\n",
			WantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOutputErrorPipes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the standard streams of a test aren't pipes on Windows")
	}

	tests := []struct {
		Name     string
		Args     []string
		Stdin    io.Reader
		file     bool
		WantErr  string
		WantExit int
		wantFile string
	}{
		{
			Name:     "the default mode stops at a broken pipe",
			file:     true,
			WantExit: 1,
			wantFile: "",
		},
		{
			Name:     "warn diagnoses a broken pipe and keeps writing",
			Args:     []string{"--output-error=warn"},
			file:     true,
			WantErr:  "tee: standard output: ",
			WantExit: 1,
			wantFile: "xxx",
		},
		{
			Name:     "p writes around a broken pipe",
			Args:     []string{"-p"},
			file:     true,
			WantExit: 0,
			wantFile: "xxx",
		},
		{
			Name:     "warn-nopipe writes around a broken pipe",
			Args:     []string{"--output-error=warn-nopipe"},
			file:     true,
			WantExit: 0,
			wantFile: "xxx",
		},
		{
			Name:     "exit stops at a broken pipe",
			Args:     []string{"--output-error=exit"},
			file:     true,
			WantErr:  "tee: standard output: ",
			WantExit: 1,
			wantFile: "",
		},
		{
			Name:     "exit-nopipe writes around a broken pipe",
			Args:     []string{"--output-error=exit-nopipe"},
			file:     true,
			WantExit: 0,
			wantFile: "xxx",
		},
		{
			Name:     "p stops once the only output is a broken pipe",
			Args:     []string{"-p"},
			Stdin:    &pipeInput{left: 3, fail: true},
			WantExit: 0,
		},
		{
			Name:     "exit-nopipe stops once the only output is a broken pipe",
			Args:     []string{"--output-error=exit-nopipe"},
			Stdin:    &pipeInput{left: 3, fail: true},
			WantExit: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatalf("os.Pipe() error = %v", err)
			}
			defer write.Close()
			if err := read.Close(); err != nil {
				t.Fatalf("Close() the read end error = %v", err)
			}

			args := test.Args
			fsys := testFS()
			if test.file {
				args = slices.Concat(args, []string{"new.txt"})
			}

			stderr := new(bytes.Buffer)
			stdin := test.Stdin
			if stdin == nil {
				stdin = &pipeInput{left: 3}
			}
			rt := &coreutils.Runtime{
				Stdin:  stdin,
				Stdout: write,
				Stderr: stderr,
				FS:     fsys,
			}
			err = New(rt).Run(t.Context(), args...)

			if code := testutil.ExitCode(t, err); code != test.WantExit {
				t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.WantExit, err)
			}
			// The message of a broken pipe names its own file descriptor,
			// so only its beginning is checked.
			switch got := stderr.String(); {
			case test.WantErr == "" && got != "":
				t.Errorf("Run() stderr = %q, want empty", got)
			case test.WantErr != "" && !strings.HasPrefix(got, test.WantErr):
				t.Errorf("Run() stderr = %q, want it to start with %q", got, test.WantErr)
			}
			if test.file {
				f := fsys["new.txt"]
				if f == nil {
					t.Error("Run() didn't create file \"new.txt\"")
				} else if got := string(f.Data); got != test.wantFile {
					t.Errorf("Run() file \"new.txt\" = %q, want %q", got, test.wantFile)
				}
			}
		})
	}
}

func TestRunErrors(t *testing.T) {
	tests := []testCase{
		{
			Name:     "missing parent directory",
			Stdin:    strings.NewReader("hello\n"),
			FS:       testFS(),
			Args:     []string{"no/such/out.txt"},
			Want:     "hello\n",
			WantErr:  "tee: no/such/out.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a file as parent directory",
			Stdin:    strings.NewReader("hello\n"),
			FS:       testFS(),
			Args:     []string{"a.txt/out.txt"},
			Want:     "hello\n",
			WantErr:  "tee: a.txt/out.txt: not a directory\n",
			WantExit: 1,
		},
		{
			Name:     "directory operand",
			Stdin:    strings.NewReader("hello\n"),
			FS:       fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			Args:     []string{"dir"},
			Want:     "hello\n",
			WantErr:  "tee: dir: invalid argument\n",
			WantExit: 1,
		},
		{
			Name:     "zero-length file name",
			Stdin:    strings.NewReader("hello\n"),
			FS:       testFS(),
			Args:     []string{""},
			Want:     "hello\n",
			WantErr:  "tee: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "every unwritable file reports an error",
			Stdin:    strings.NewReader("hello\n"),
			FS:       testFS(),
			Args:     []string{"no/such/one.txt", "no/such/two.txt"},
			Want:     "hello\n",
			WantErr:  "tee: no/such/one.txt: file does not exist\ntee: no/such/two.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:      "an unwritable file doesn't stop the other files",
			Stdin:     strings.NewReader("hello\n"),
			FS:        testFS(),
			Args:      []string{"no/such/out.txt", "good.txt"},
			Want:      "hello\n",
			WantErr:   "tee: no/such/out.txt: file does not exist\n",
			WantExit:  1,
			WantFiles: map[string]string{"good.txt": "hello\n"},
		},
		{
			Name:     "file operand without a file system",
			Stdin:    strings.NewReader("hello\n"),
			Args:     []string{"out.txt"},
			Want:     "hello\n",
			WantErr:  "tee: out.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			FS:       testFS(),
			WantErr:  "tee: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "standard input read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			FS:       testFS(),
			WantErr:  "tee: standard input: read failed\n",
			WantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
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
		Stdout: testutil.ErrorWriter{Err: writeErr},
		Stderr: stderr,
		FS:     fsys,
	}
	err := New(rt).Run(t.Context(), "out.txt")

	if code := testutil.ExitCode(t, err); code != 1 {
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
		testutil.Cancelled(t, New, strings.NewReader("hello\n"), nil)
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		testutil.Cancelled(t, New, &testutil.CancelReader{}, nil)
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "tee", usage)
}
