package uuencode

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/uudecode"
	"charm.land/snail/fsx"
)

// testFS holds the fixtures used by the tests:
//
//	hello.txt:     "hello"
//	empty.txt:     ""
//	bin.dat:       "\x00\x01\x02\x03\x04\x05"
//	"-m":          "x"
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"hello.txt":     {Data: []byte("hello"), Mode: 0o644},
		"empty.txt":     {Data: []byte(""), Mode: 0o600},
		"bin.dat":       {Data: []byte{0, 1, 2, 3, 4, 5}, Mode: 0o750},
		"-m":            {Data: []byte("x")},
		"sub/inner.txt": {Data: []byte("inner\n"), Mode: 0o644},
	}
}

// errorReader always fails to be read.
type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

// errorWriter always fails to be written to.
type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

// cancelReader cancels its context on the first read, simulating a context
// that is done while reading the input.
type cancelReader struct {
	cancel context.CancelFunc
	reads  int
}

func (r *cancelReader) Read(p []byte) (int, error) {
	if r.reads > 0 {
		return 0, io.EOF
	}
	r.reads++
	r.cancel()
	return copy(p, "hello"), nil
}

type testCase struct {
	name     string
	stdin    io.Reader
	fsys     fsx.FS
	dir      string
	umask    fs.FileMode
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
		Umask:      test.umask,
	}
	err := New(rt).Run(t.Context(), test.args...)

	code := 0
	if err != nil {
		var exitErr *coreutils.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
		}
		code = exitErr.Code
	}
	if code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.want {
		t.Errorf("Run() stdout = %q, want %q", got, test.want)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
}

func TestRunEncode(t *testing.T) {
	tests := []testCase{
		{
			name:  "the standard input is encoded with the default creation mode",
			stdin: strings.NewReader("hello"),
			umask: 0o022,
			args:  []string{"dest"},
			want:  "begin 644 dest\n%:&5L;&\\`\n`\nend\n",
		},
		{
			name:  "the standard input with no umask gets mode 666",
			stdin: strings.NewReader("hello"),
			args:  []string{"dest"},
			want:  "begin 666 dest\n%:&5L;&\\`\n`\nend\n",
		},
		{
			name:  "empty input writes an empty body",
			stdin: strings.NewReader(""),
			umask: 0o022,
			args:  []string{"dest"},
			want:  "begin 644 dest\n`\nend\n",
		},
		{
			name: "a file operand",
			fsys: testFS(),
			args: []string{"hello.txt", "dest"},
			want: "begin 644 dest\n%:&5L;&\\`\n`\nend\n",
		},
		{
			name: "the header holds the mode of the input file",
			fsys: testFS(),
			args: []string{"bin.dat", "dest"},
			want: "begin 750 dest\n&``$\"`P0%\n`\nend\n",
		},
		{
			name: "an empty file",
			fsys: testFS(),
			args: []string{"empty.txt", "dest"},
			want: "begin 600 dest\n`\nend\n",
		},
		{
			name: "the file is resolved against the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt", "dest"},
			want: "begin 644 dest\n&:6YN97(*\n`\nend\n",
		},
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("hello"),
			umask: 0o022,
			args:  []string{"-", "dest"},
			want:  "begin 644 dest\n%:&5L;&\\`\n`\nend\n",
		},
		{
			name: "a file named like an option after a double dash",
			fsys: testFS(),
			args: []string{"--", "-m", "dest"},
			want: "begin 000 dest\n!>```\n`\nend\n",
		},
		{
			name:  "a name of dash is escaped for uudecode",
			stdin: strings.NewReader("x"),
			args:  []string{"-"},
			want:  "begin 666 ./-\n!>```\n`\nend\n",
		},
		{
			name:  "a name of /dev/stdout is escaped for uudecode",
			stdin: strings.NewReader("x"),
			args:  []string{"/dev/stdout"},
			want:  "begin 666 ///dev/stdout\n!>```\n`\nend\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunEncodeBase64(t *testing.T) {
	tests := []testCase{
		{
			name:  "-m encodes with base64",
			stdin: strings.NewReader("hello"),
			umask: 0o022,
			args:  []string{"-m", "dest"},
			want:  "begin-base64 644 dest\naGVsbG8=\n====\n",
		},
		{
			name:  "the long base64 option",
			stdin: strings.NewReader("hello"),
			umask: 0o022,
			args:  []string{"--base64", "dest"},
			want:  "begin-base64 644 dest\naGVsbG8=\n====\n",
		},
		{
			name:  "empty input writes an empty body",
			stdin: strings.NewReader(""),
			umask: 0o022,
			args:  []string{"-m", "dest"},
			want:  "begin-base64 644 dest\n====\n",
		},
		{
			name: "a file operand",
			fsys: testFS(),
			args: []string{"-m", "hello.txt", "dest"},
			want: "begin-base64 644 dest\naGVsbG8=\n====\n",
		},
		{
			name:  "lines wrap at 76 columns",
			stdin: strings.NewReader(strings.Repeat("a", 60)),
			args:  []string{"-m", "dest"},
			want: "begin-base64 666 dest\n" +
				strings.Repeat("YWFh", 19) + "\n" +
				"YWFh" + "\n" +
				"====\n",
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
			name:     "the name operand is required",
			stdin:    strings.NewReader("hello"),
			wantErr:  "uuencode: missing operand\n",
			wantExit: 2,
		},
		{
			name:     "at most two operands",
			fsys:     testFS(),
			args:     []string{"hello.txt", "dest", "extra"},
			wantErr:  "uuencode: extra operand 'extra'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown option",
			args:     []string{"-x", "dest"},
			wantErr:  "uuencode: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown long option",
			args:     []string{"--nope", "dest"},
			wantErr:  "uuencode: unrecognized option '--nope'\n",
			wantExit: 2,
		},
		{
			name:     "a missing input file",
			fsys:     testFS(),
			args:     []string{"nope.txt", "dest"},
			wantErr:  "uuencode: nope.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "an empty input name",
			fsys:     testFS(),
			args:     []string{"", "dest"},
			wantErr:  "uuencode: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			args:     []string{"dest"},
			wantErr:  "uuencode: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "no file system",
			args:     []string{"hello.txt", "dest"},
			wantErr:  "uuencode: hello.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "a read error",
			stdin:    errorReader{},
			args:     []string{"dest"},
			want:     "begin 666 dest\n",
			wantErr:  "uuencode: standard input: boom\n",
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
	tests := []testCase{
		{
			name:    "encoding",
			stdin:   strings.NewReader("hello"),
			args:    []string{"dest"},
			wantErr: "uuencode: write error: boom\n",
		},
		{
			name:    "encoding with base64",
			stdin:   strings.NewReader("hello"),
			args:    []string{"-m", "dest"},
			wantErr: "uuencode: write error: boom\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stderr := new(bytes.Buffer)
			rt := &coreutils.Runtime{
				Stdin:  test.stdin,
				Stdout: errorWriter{},
				Stderr: stderr,
			}
			err := New(rt).Run(t.Context(), test.args...)

			var exitErr *coreutils.ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 1 {
				t.Fatalf("Run() error = %v, want exit code 1", err)
			}
			if got := stderr.String(); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
		})
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		runCancelled(t, ctx, strings.NewReader("hello"))
	})

	t.Run("cancelled while reading", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runCancelled(t, ctx, &cancelReader{cancel: cancel})
	})
}

// runCancelled runs the utility with a context that is either already done
// or becomes done while reading, and checks that the context error is
// returned without any diagnostic.
func runCancelled(t *testing.T, ctx context.Context, stdin io.Reader) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
	err := New(rt).Run(ctx, "dest")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
}

// TestRunEscapedNameRoundTrip encodes an input with a name of dash and
// checks that uudecode recreates a literal file with that name rather than
// writing it to the standard output.
func TestRunEscapedNameRoundTrip(t *testing.T) {
	stdout := new(bytes.Buffer)
	enc := New(&coreutils.Runtime{
		Stdin:  strings.NewReader("hello"),
		Stdout: stdout,
		Stderr: new(bytes.Buffer),
		Umask:  0o022,
	})
	if err := enc.Run(t.Context(), "-"); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	fsys := fsx.MemFS{"sub/keep.txt": {Data: []byte("x")}}
	dec := uudecode.New(&coreutils.Runtime{
		Stdin:      bytes.NewReader(stdout.Bytes()),
		Stdout:     new(bytes.Buffer),
		Stderr:     new(bytes.Buffer),
		FS:         fsys,
		WorkingDir: "sub",
	})
	if err := dec.Run(t.Context()); err != nil {
		t.Fatalf("uudecode.Run() error = %v, want nil", err)
	}
	entry, ok := fsys["sub/-"]
	if !ok {
		t.Fatal("uudecode didn't create the file -")
	}
	if got := string(entry.Data); got != "hello" {
		t.Errorf("the file - = %q, want %q", got, "hello")
	}
}
