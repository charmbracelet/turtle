package uudecode

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/uuencode"
	"charm.land/snail/fsx"
)

// The historical file holds the output of uuencode for "hello", and the
// base64 file the output of uuencode -m for it, with the modes the headers
// announce.
const historical = "begin 644 out.txt\n%:&5L;&\\`\n`\nend\n"

const base64Body = "begin-base64 600 b64.txt\naGVsbG8=\n====\n"

// testFS holds the fixtures used by the tests:
//
//	encoded.uue:   the historical file above
//	base64.uue:    the base64 file above
//	garbage.uue:   the historical file with leading text
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"encoded.uue":   {Data: []byte(historical)},
		"base64.uue":    {Data: []byte(base64Body)},
		"garbage.uue":   {Data: []byte("junk\n" + historical)},
		"sub/inner.txt": {Data: []byte("inner\n")},
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

// wantFile describes a file which the run recreated.
type wantFile struct {
	data string
	mode uint32
}

type testCase struct {
	name      string
	stdin     io.Reader
	fsys      fsx.FS
	dir       string
	args      []string
	want      string
	wantErr   string
	wantExit  int
	wantFiles map[string]wantFile
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
	for name, want := range test.wantFiles {
		checkFile(t, test.fsys, name, want)
	}
}

// checkFile checks a recreated file against what the test wants.
func checkFile(t *testing.T, fsys fsx.FS, name string, want wantFile) {
	t.Helper()

	mem, ok := fsys.(fsx.MemFS)
	if !ok {
		t.Fatalf("a MemFS is needed to check the file %s", name)
	}
	entry, ok := mem[name]
	if !ok {
		t.Errorf("the file %s wasn't created", name)
		return
	}
	if got := string(entry.Data); got != want.data {
		t.Errorf("the file %s = %q, want %q", name, got, want.data)
	}
	if got := entry.Mode.Perm(); uint32(got) != want.mode {
		t.Errorf("the file %s has mode %o, want %o", name, got, want.mode)
	}
}

func TestRunDecode(t *testing.T) {
	tests := []testCase{
		{
			name:  "a historical input recreates the file",
			stdin: strings.NewReader(historical),
			fsys:  fsx.MemFS{},
			wantFiles: map[string]wantFile{
				"out.txt": {data: "hello", mode: 0o644},
			},
		},
		{
			name:  "a base64 input recreates the file",
			stdin: strings.NewReader(base64Body),
			fsys:  fsx.MemFS{},
			wantFiles: map[string]wantFile{
				"b64.txt": {data: "hello", mode: 0o600},
			},
		},
		{
			name:  "leading text is skipped",
			stdin: strings.NewReader("junk\n" + historical),
			fsys:  fsx.MemFS{},
			wantFiles: map[string]wantFile{
				"out.txt": {data: "hello", mode: 0o644},
			},
		},
		{
			name:  "a space encodes a zero as well as a backquote",
			stdin: strings.NewReader("begin 644 out.txt\n%:&5L;&\\ \n \nend\n"),
			fsys:  fsx.MemFS{},
			wantFiles: map[string]wantFile{
				"out.txt": {data: "hello", mode: 0o644},
			},
		},
		{
			name:  "the end line may be missing after the terminator",
			stdin: strings.NewReader("begin 644 out.txt\n%:&5L;&\\`\n`\n"),
			fsys:  fsx.MemFS{},
			wantFiles: map[string]wantFile{
				"out.txt": {data: "hello", mode: 0o644},
			},
		},
		{
			name:  "Windows line endings are accepted",
			stdin: strings.NewReader("begin 644 out.txt\r\n%:&5L;&\\`\r\n`\r\nend\r\n"),
			fsys:  fsx.MemFS{},
			wantFiles: map[string]wantFile{
				"out.txt": {data: "hello", mode: 0o644},
			},
		},
		{
			name:  "a symbolic mode is accepted",
			stdin: strings.NewReader("begin u=rwx,go=r out.txt\n%:&5L;&\\`\n`\nend\n"),
			fsys:  fsx.MemFS{},
			wantFiles: map[string]wantFile{
				"out.txt": {data: "hello", mode: 0o744},
			},
		},
		{
			name:  "base64 ignores the garbage of a mail header",
			stdin: strings.NewReader("begin-base64 644 out.txt\naG Vs bG8=\n====\n"),
			fsys:  fsx.MemFS{},
			wantFiles: map[string]wantFile{
				"out.txt": {data: "hello", mode: 0o644},
			},
		},
		{
			name: "a file operand",
			fsys: testFS(),
			args: []string{"encoded.uue"},
			wantFiles: map[string]wantFile{
				"out.txt": {data: "hello", mode: 0o644},
			},
		},
		{
			name:  "the output is resolved against the working directory",
			stdin: strings.NewReader(historical),
			fsys:  testFS(),
			dir:   "sub",
			wantFiles: map[string]wantFile{
				"sub/out.txt": {data: "hello", mode: 0o644},
			},
		},
		{
			name:  "an existing file is overwritten",
			fsys:  fsx.MemFS{"out.txt": {Data: []byte("old"), Mode: 0o755}},
			stdin: strings.NewReader(historical),
			wantFiles: map[string]wantFile{
				"out.txt": {data: "hello", mode: 0o644},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOutput(t *testing.T) {
	tests := []testCase{
		{
			name:  "a header name of dash writes the standard output",
			stdin: strings.NewReader("begin 644 -\n%:&5L;&\\`\n`\nend\n"),
			want:  "hello",
		},
		{
			name:  "a header name of /dev/stdout writes the standard output",
			stdin: strings.NewReader("begin 644 /dev/stdout\n%:&5L;&\\`\n`\nend\n"),
			want:  "hello",
		},
		{
			name:  "-o overrides the header name",
			stdin: strings.NewReader(historical),
			fsys:  fsx.MemFS{},
			args:  []string{"-o", "mine.txt"},
			wantFiles: map[string]wantFile{
				"mine.txt": {data: "hello", mode: 0o644},
			},
		},
		{
			name:  "-o with dash writes the standard output",
			stdin: strings.NewReader(historical),
			args:  []string{"-o", "-"},
			want:  "hello",
		},
		{
			name:  "-o attached to the option",
			stdin: strings.NewReader(historical),
			fsys:  fsx.MemFS{},
			args:  []string{"-omine.txt"},
			wantFiles: map[string]wantFile{
				"mine.txt": {data: "hello", mode: 0o644},
			},
		},
		{
			name:  "the long output-file option",
			stdin: strings.NewReader(historical),
			fsys:  fsx.MemFS{},
			args:  []string{"--output-file", "mine.txt"},
			wantFiles: map[string]wantFile{
				"mine.txt": {data: "hello", mode: 0o644},
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
			name:     "no begin line",
			stdin:    strings.NewReader("just junk\n"),
			wantErr:  "uudecode: standard input: no 'begin' line\n",
			wantExit: 1,
		},
		{
			name:     "an empty input",
			stdin:    strings.NewReader(""),
			wantErr:  "uudecode: standard input: no 'begin' line\n",
			wantExit: 1,
		},
		{
			name:     "an invalid mode",
			stdin:    strings.NewReader("begin xyz out.txt\n"),
			wantErr:  "uudecode: standard input: invalid mode: \"xyz\"\n",
			wantExit: 1,
		},
		{
			name:     "a short body line",
			stdin:    strings.NewReader("begin 644 out.txt\nMab\nend\n"),
			fsys:     fsx.MemFS{},
			wantErr:  "uudecode: standard input: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "an end line without the terminator",
			stdin:    strings.NewReader("begin 644 out.txt\n%:&5L;&\\`\nend\n"),
			fsys:     fsx.MemFS{},
			wantErr:  "uudecode: standard input: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "an empty body line",
			stdin:    strings.NewReader("begin 644 out.txt\n%:&5L;&\\`\n\n`\nend\n"),
			fsys:     fsx.MemFS{},
			wantErr:  "uudecode: standard input: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a character outside of the alphabet",
			stdin:    strings.NewReader("begin 644 out.txt\n%:&5L;&~\n`\nend\n"),
			fsys:     fsx.MemFS{},
			wantErr:  "uudecode: standard input: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a truncated historical body",
			stdin:    strings.NewReader("begin 644 out.txt\n%:&5L;&\\`\n"),
			fsys:     fsx.MemFS{},
			wantErr:  "uudecode: standard input: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a body of no lines",
			stdin:    strings.NewReader("begin 644 out.txt\n"),
			fsys:     fsx.MemFS{},
			wantErr:  "uudecode: standard input: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "base64 without its terminator",
			stdin:    strings.NewReader("begin-base64 644 out.txt\naGVsbG8=\n"),
			fsys:     fsx.MemFS{},
			wantErr:  "uudecode: standard input: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "non-canonical base64",
			stdin:    strings.NewReader("begin-base64 644 out.txt\naGVsbG9=\n====\n"),
			fsys:     fsx.MemFS{},
			wantErr:  "uudecode: standard input: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "at most one operand",
			fsys:     testFS(),
			args:     []string{"encoded.uue", "extra.uue"},
			wantErr:  "uudecode: extra operand 'extra.uue'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown option",
			args:     []string{"-x"},
			wantErr:  "uudecode: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown long option",
			args:     []string{"--nope"},
			wantErr:  "uudecode: unrecognized option '--nope'\n",
			wantExit: 2,
		},
		{
			name:     "-o requires an argument",
			args:     []string{"-o"},
			wantErr:  "uudecode: option requires an argument -- 'o'\n",
			wantExit: 2,
		},
		{
			name:     "an empty input name",
			args:     []string{""},
			wantErr:  "uudecode: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			wantErr:  "uudecode: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "a missing input file",
			fsys:     testFS(),
			args:     []string{"nope.uue"},
			wantErr:  "uudecode: nope.uue: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a read error",
			stdin:    errorReader{},
			wantErr:  "uudecode: standard input: boom\n",
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
	tests := []struct {
		name    string
		stdin   string
		wantErr string
	}{
		{
			name:    "to the standard output",
			stdin:   "begin 644 -\n%:&5L;&\\`\n`\nend\n",
			wantErr: "uudecode: -: boom\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stderr := new(bytes.Buffer)
			rt := &coreutils.Runtime{
				Stdin:  strings.NewReader(test.stdin),
				Stdout: errorWriter{},
				Stderr: stderr,
				FS:     fsx.MemFS{},
			}
			err := New(rt).Run(t.Context())

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
		runCancelled(t, ctx, strings.NewReader(historical))
	})

	t.Run("cancelled while scanning", func(t *testing.T) {
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
		FS:     fsx.MemFS{},
	}
	err := New(rt).Run(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
}

// TestRunRoundTrip encodes an input big enough to span several lines with
// uuencode, with both algorithms, and decodes it back.
func TestRunRoundTrip(t *testing.T) {
	input := make([]byte, 100_000)
	for i := range input {
		input[i] = byte(i)
	}

	for _, test := range []struct {
		name string
		args []string
	}{
		{"historical", nil},
		{"base64", []string{"-m"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded := new(bytes.Buffer)
			enc := uuencode.New(&coreutils.Runtime{
				Stdin:  bytes.NewReader(input),
				Stdout: encoded,
				Stderr: new(bytes.Buffer),
				Umask:  0o022,
			})
			if err := enc.Run(t.Context(), append(test.args, "out.txt")...); err != nil {
				t.Fatalf("uuencode.Run() error = %v, want nil", err)
			}

			fsys := fsx.MemFS{}
			stderr := new(bytes.Buffer)
			rt := &coreutils.Runtime{
				Stdin:  bytes.NewReader(encoded.Bytes()),
				Stdout: new(bytes.Buffer),
				Stderr: stderr,
				FS:     fsys,
			}
			if err := New(rt).Run(t.Context()); err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}
			if got := string(fsys["out.txt"].Data); got != string(input) {
				t.Errorf("decoded data doesn't match the input")
			}
			if got := fsys["out.txt"].Mode.Perm(); got != 0o644 {
				t.Errorf("decoded file has mode %o, want 644", got)
			}
		})
	}
}
