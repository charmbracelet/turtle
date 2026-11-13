package uudecode

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/coreutils/programs/uuencode"
	"charm.land/turtle/fsx"
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

func TestRunDecode(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "a historical input recreates the file",
			Stdin:     strings.NewReader(historical),
			FS:        fsx.MemFS{},
			WantFiles: map[string]string{"out.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"out.txt": 0o644},
		},
		{
			Name:      "a base64 input recreates the file",
			Stdin:     strings.NewReader(base64Body),
			FS:        fsx.MemFS{},
			WantFiles: map[string]string{"b64.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"b64.txt": 0o600},
		},
		{
			Name:      "leading text is skipped",
			Stdin:     strings.NewReader("junk\n" + historical),
			FS:        fsx.MemFS{},
			WantFiles: map[string]string{"out.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"out.txt": 0o644},
		},
		{
			Name:      "a space encodes a zero as well as a backquote",
			Stdin:     strings.NewReader("begin 644 out.txt\n%:&5L;&\\ \n \nend\n"),
			FS:        fsx.MemFS{},
			WantFiles: map[string]string{"out.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"out.txt": 0o644},
		},
		{
			Name:      "the end line may be missing after the terminator",
			Stdin:     strings.NewReader("begin 644 out.txt\n%:&5L;&\\`\n`\n"),
			FS:        fsx.MemFS{},
			WantFiles: map[string]string{"out.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"out.txt": 0o644},
		},
		{
			Name:      "Windows line endings are accepted",
			Stdin:     strings.NewReader("begin 644 out.txt\r\n%:&5L;&\\`\r\n`\r\nend\r\n"),
			FS:        fsx.MemFS{},
			WantFiles: map[string]string{"out.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"out.txt": 0o644},
		},
		{
			Name:      "a symbolic mode is accepted",
			Stdin:     strings.NewReader("begin u=rwx,go=r out.txt\n%:&5L;&\\`\n`\nend\n"),
			FS:        fsx.MemFS{},
			WantFiles: map[string]string{"out.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"out.txt": 0o744},
		},
		{
			Name:      "base64 ignores the garbage of a mail header",
			Stdin:     strings.NewReader("begin-base64 644 out.txt\naG Vs bG8=\n====\n"),
			FS:        fsx.MemFS{},
			WantFiles: map[string]string{"out.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"out.txt": 0o644},
		},
		{
			Name:      "a file operand",
			FS:        testFS(),
			Args:      []string{"encoded.uue"},
			WantFiles: map[string]string{"out.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"out.txt": 0o644},
		},
		{
			Name:      "the output is resolved against the working directory",
			Stdin:     strings.NewReader(historical),
			FS:        testFS(),
			Dir:       "sub",
			WantFiles: map[string]string{"sub/out.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"sub/out.txt": 0o644},
		},
		{
			Name:      "an existing file is overwritten",
			FS:        fsx.MemFS{"out.txt": {Data: []byte("old"), Mode: 0o755}},
			Stdin:     strings.NewReader(historical),
			WantFiles: map[string]string{"out.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"out.txt": 0o644},
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOutput(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a header name of dash writes the standard output",
			Stdin: strings.NewReader("begin 644 -\n%:&5L;&\\`\n`\nend\n"),
			Want:  "hello",
		},
		{
			Name:  "a header name of /dev/stdout writes the standard output",
			Stdin: strings.NewReader("begin 644 /dev/stdout\n%:&5L;&\\`\n`\nend\n"),
			Want:  "hello",
		},
		{
			Name:      "-o overrides the header name",
			Stdin:     strings.NewReader(historical),
			FS:        fsx.MemFS{},
			Args:      []string{"-o", "mine.txt"},
			WantFiles: map[string]string{"mine.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"mine.txt": 0o644},
		},
		{
			Name:  "-o with dash writes the standard output",
			Stdin: strings.NewReader(historical),
			Args:  []string{"-o", "-"},
			Want:  "hello",
		},
		{
			Name:      "-o attached to the option",
			Stdin:     strings.NewReader(historical),
			FS:        fsx.MemFS{},
			Args:      []string{"-omine.txt"},
			WantFiles: map[string]string{"mine.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"mine.txt": 0o644},
		},
		{
			Name:      "the long output-file option",
			Stdin:     strings.NewReader(historical),
			FS:        fsx.MemFS{},
			Args:      []string{"--output-file", "mine.txt"},
			WantFiles: map[string]string{"mine.txt": "hello"},
			WantMode:  map[string]fs.FileMode{"mine.txt": 0o644},
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "no begin line",
			Stdin:    strings.NewReader("just junk\n"),
			WantErr:  "uudecode: standard input: no 'begin' line\n",
			WantExit: 1,
		},
		{
			Name:     "an empty input",
			Stdin:    strings.NewReader(""),
			WantErr:  "uudecode: standard input: no 'begin' line\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid mode",
			Stdin:    strings.NewReader("begin xyz out.txt\n"),
			WantErr:  "uudecode: standard input: invalid mode: \"xyz\"\n",
			WantExit: 1,
		},
		{
			Name:     "a short body line",
			Stdin:    strings.NewReader("begin 644 out.txt\nMab\nend\n"),
			FS:       fsx.MemFS{},
			WantErr:  "uudecode: standard input: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "an end line without the terminator",
			Stdin:    strings.NewReader("begin 644 out.txt\n%:&5L;&\\`\nend\n"),
			FS:       fsx.MemFS{},
			WantErr:  "uudecode: standard input: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "an empty body line",
			Stdin:    strings.NewReader("begin 644 out.txt\n%:&5L;&\\`\n\n`\nend\n"),
			FS:       fsx.MemFS{},
			WantErr:  "uudecode: standard input: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a character outside of the alphabet",
			Stdin:    strings.NewReader("begin 644 out.txt\n%:&5L;&~\n`\nend\n"),
			FS:       fsx.MemFS{},
			WantErr:  "uudecode: standard input: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a line with trailing characters",
			Stdin:    strings.NewReader("begin 644 out.txt\n%:&5L;&\\`QQ\n`\nend\n"),
			FS:       fsx.MemFS{},
			WantErr:  "uudecode: standard input: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a terminator with trailing characters",
			Stdin:    strings.NewReader("begin 644 out.txt\n%:&5L;&\\`\n`abc\nend\n"),
			FS:       fsx.MemFS{},
			WantErr:  "uudecode: standard input: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a truncated historical body",
			Stdin:    strings.NewReader("begin 644 out.txt\n%:&5L;&\\`\n"),
			FS:       fsx.MemFS{},
			WantErr:  "uudecode: standard input: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a body of no lines",
			Stdin:    strings.NewReader("begin 644 out.txt\n"),
			FS:       fsx.MemFS{},
			WantErr:  "uudecode: standard input: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "base64 without its terminator",
			Stdin:    strings.NewReader("begin-base64 644 out.txt\naGVsbG8=\n"),
			FS:       fsx.MemFS{},
			WantErr:  "uudecode: standard input: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "non-canonical base64",
			Stdin:    strings.NewReader("begin-base64 644 out.txt\naGVsbG9=\n====\n"),
			FS:       fsx.MemFS{},
			WantErr:  "uudecode: standard input: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "at most one operand",
			FS:       testFS(),
			Args:     []string{"encoded.uue", "extra.uue"},
			WantErr:  "uudecode: extra operand 'extra.uue'\nTry 'uudecode --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown option",
			Args:     []string{"-x"},
			WantErr:  "uudecode: invalid option -- 'x'\nTry 'uudecode --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown long option",
			Args:     []string{"--nope"},
			WantErr:  "uudecode: unrecognized option '--nope'\nTry 'uudecode --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "-o requires an argument",
			Args:     []string{"-o"},
			WantErr:  "uudecode: option requires an argument -- 'o'\nTry 'uudecode --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty -o argument",
			Stdin:    strings.NewReader(historical),
			FS:       fsx.MemFS{},
			Args:     []string{"-o", ""},
			WantErr:  "uudecode: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "an empty --output-file value",
			Stdin:    strings.NewReader(historical),
			FS:       fsx.MemFS{},
			Args:     []string{"--output-file="},
			WantErr:  "uudecode: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "an empty input name",
			Args:     []string{""},
			WantErr:  "uudecode: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			WantErr:  "uudecode: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "a missing input file",
			FS:       testFS(),
			Args:     []string{"nope.uue"},
			WantErr:  "uudecode: nope.uue: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a read error",
			Stdin:    testutil.ErrorReader{},
			WantErr:  "uudecode: standard input: boom\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
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
				Stdout: testutil.ErrorWriter{},
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
		runCancelled(t, ctx, &testutil.CancelReader{Cancel: cancel, Data: []byte("hello")})
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

func TestHelpVersion(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"help", []string{"--help"}, usage},
		{
			"version",
			[]string{"--version"},
			"uudecode (charm.land/turtle/coreutils) " + coreutils.Version + "\nhttps://charm.land/turtle\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testutil.Once(t, New, testutil.Case{Name: test.name, Args: test.args, Want: test.want})
		})
	}
}
