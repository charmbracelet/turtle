package uuencode

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
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

func TestRunEncode(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the standard input is encoded with the default creation mode",
			Stdin: strings.NewReader("hello"),
			Umask: 0o022,
			Args:  []string{"dest"},
			Want:  "begin 644 dest\n%:&5L;&\\`\n`\nend\n",
		},
		{
			Name:  "the standard input with no umask gets mode 666",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"dest"},
			Want:  "begin 666 dest\n%:&5L;&\\`\n`\nend\n",
		},
		{
			Name:  "empty input writes an empty body",
			Stdin: strings.NewReader(""),
			Umask: 0o022,
			Args:  []string{"dest"},
			Want:  "begin 644 dest\n`\nend\n",
		},
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"hello.txt", "dest"},
			Want: "begin 644 dest\n%:&5L;&\\`\n`\nend\n",
		},
		{
			Name: "the header holds the mode of the input file",
			FS:   testFS(),
			Args: []string{"bin.dat", "dest"},
			Want: "begin 750 dest\n&``$\"`P0%\n`\nend\n",
		},
		{
			Name: "an empty file",
			FS:   testFS(),
			Args: []string{"empty.txt", "dest"},
			Want: "begin 600 dest\n`\nend\n",
		},
		{
			Name: "the file is resolved against the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"inner.txt", "dest"},
			Want: "begin 644 dest\n&:6YN97(*\n`\nend\n",
		},
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("hello"),
			Umask: 0o022,
			Args:  []string{"-", "dest"},
			Want:  "begin 644 dest\n%:&5L;&\\`\n`\nend\n",
		},
		{
			Name: "a file named like an option after a double dash",
			FS:   testFS(),
			Args: []string{"--", "-m", "dest"},
			Want: "begin 000 dest\n!>```\n`\nend\n",
		},
		{
			Name:  "a name of dash is kept verbatim",
			Stdin: strings.NewReader("x"),
			Args:  []string{"-"},
			Want:  "begin 666 -\n!>```\n`\nend\n",
		},
		{
			Name:  "a name of /dev/stdout is kept verbatim",
			Stdin: strings.NewReader("x"),
			Args:  []string{"/dev/stdout"},
			Want:  "begin 666 /dev/stdout\n!>```\n`\nend\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunEncodeBase64(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "-m encodes with base64",
			Stdin: strings.NewReader("hello"),
			Umask: 0o022,
			Args:  []string{"-m", "dest"},
			Want:  "begin-base64 644 dest\naGVsbG8=\n====\n",
		},
		{
			Name:  "the long base64 option",
			Stdin: strings.NewReader("hello"),
			Umask: 0o022,
			Args:  []string{"--base64", "dest"},
			Want:  "begin-base64 644 dest\naGVsbG8=\n====\n",
		},
		{
			Name:  "empty input writes an empty body",
			Stdin: strings.NewReader(""),
			Umask: 0o022,
			Args:  []string{"-m", "dest"},
			Want:  "begin-base64 644 dest\n====\n",
		},
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"-m", "hello.txt", "dest"},
			Want: "begin-base64 644 dest\naGVsbG8=\n====\n",
		},
		{
			Name:  "lines wrap at 76 columns",
			Stdin: strings.NewReader(strings.Repeat("a", 60)),
			Args:  []string{"-m", "dest"},
			Want: "begin-base64 666 dest\n" +
				strings.Repeat("YWFh", 19) + "\n" +
				"YWFh" + "\n" +
				"====\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "the name operand is required",
			Stdin:    strings.NewReader("hello"),
			WantErr:  "uuencode: missing operand\nTry 'uuencode --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "at most two operands",
			FS:       testFS(),
			Args:     []string{"hello.txt", "dest", "extra"},
			WantErr:  "uuencode: extra operand 'extra'\nTry 'uuencode --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown option",
			Args:     []string{"-x", "dest"},
			WantErr:  "uuencode: invalid option -- 'x'\nTry 'uuencode --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown long option",
			Args:     []string{"--nope", "dest"},
			WantErr:  "uuencode: unrecognized option '--nope'\nTry 'uuencode --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a missing input file",
			FS:       testFS(),
			Args:     []string{"nope.txt", "dest"},
			WantErr:  "uuencode: nope.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "an empty input name",
			FS:       testFS(),
			Args:     []string{"", "dest"},
			WantErr:  "uuencode: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			Args:     []string{"dest"},
			WantErr:  "uuencode: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "no file system",
			Args:     []string{"hello.txt", "dest"},
			WantErr:  "uuencode: hello.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "a read error",
			Stdin:    testutil.ErrorReader{},
			Args:     []string{"dest"},
			Want:     "begin 666 dest\n",
			WantErr:  "uuencode: standard input: boom\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunWriteError(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:    "encoding",
			Stdin:   strings.NewReader("hello"),
			Args:    []string{"dest"},
			WantErr: "uuencode: write error: boom\n",
		},
		{
			Name:    "encoding with base64",
			Stdin:   strings.NewReader("hello"),
			Args:    []string{"-m", "dest"},
			WantErr: "uuencode: write error: boom\n",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			stderr := new(bytes.Buffer)
			rt := &coreutils.Runtime{
				Stdin:  test.Stdin,
				Stdout: testutil.ErrorWriter{},
				Stderr: stderr,
			}
			err := New(rt).Run(t.Context(), test.Args...)

			var exitErr *coreutils.ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 1 {
				t.Fatalf("Run() error = %v, want exit code 1", err)
			}
			if got := stderr.String(); got != test.WantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.WantErr)
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
	}
	err := New(rt).Run(ctx, "dest")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
}

// TestRunStdoutRoundTrip encodes an input with a name of dash and checks
// that uudecode writes the decoded bytes to the standard output, like POSIX
// specifies for the cookie.
func TestRunStdoutRoundTrip(t *testing.T) {
	encoded := new(bytes.Buffer)
	enc := New(&coreutils.Runtime{
		Stdin:  strings.NewReader("hello"),
		Stdout: encoded,
		Stderr: new(bytes.Buffer),
		Umask:  0o022,
	})
	if err := enc.Run(t.Context(), "-"); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	decoded := new(bytes.Buffer)
	dec := uudecode.New(&coreutils.Runtime{
		Stdin:  bytes.NewReader(encoded.Bytes()),
		Stdout: decoded,
		Stderr: new(bytes.Buffer),
	})
	if err := dec.Run(t.Context()); err != nil {
		t.Fatalf("uudecode.Run() error = %v, want nil", err)
	}
	if got := decoded.String(); got != "hello" {
		t.Errorf("uudecode wrote %q, want %q", got, "hello")
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
			"uuencode (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testutil.Once(t, New, testutil.Case{Name: test.name, Args: test.args, Want: test.want})
		})
	}
}
