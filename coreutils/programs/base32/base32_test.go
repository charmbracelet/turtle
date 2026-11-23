package base32

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// testFS holds the fixtures used by the tests:
//
//	hello.txt:   "hello"
//	empty.txt:   ""
//	lines.txt:   "NBSWY3DP\nMZXW6YTBOI======\n"
//	garbage.txt: "NBSW Y3DP"
//	"-d":        "x"
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemoryFS {
	return fsx.MemoryFS{
		"hello.txt":     {Data: []byte("hello")},
		"empty.txt":     {Data: []byte("")},
		"lines.txt":     {Data: []byte("NBSWY3DP\nMZXW6YTBOI======\n")},
		"garbage.txt":   {Data: []byte("NBSW Y3DP")},
		"-d":            {Data: []byte("x")},
		"sub/inner.txt": {Data: []byte("inner\n")},
	}
}

func TestRunEncode(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no operands reads the standard input",
			Stdin: strings.NewReader("hello"),
			Want:  "NBSWY3DP\n",
		},
		{
			Name:  "empty input writes nothing",
			Stdin: strings.NewReader(""),
			Want:  "",
		},
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"-"},
			Want:  "NBSWY3DP\n",
		},
		{
			Name:  "double dash with no operands reads the standard input",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"--"},
			Want:  "NBSWY3DP\n",
		},
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"hello.txt"},
			Want: "NBSWY3DP\n",
		},
		{
			Name: "an empty file",
			FS:   testFS(),
			Args: []string{"empty.txt"},
			Want: "",
		},
		{
			Name: "the file is resolved against the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"inner.txt"},
			Want: "NFXG4ZLSBI======\n",
		},
		{
			Name: "a file named like an option after a double dash",
			FS:   testFS(),
			Args: []string{"--", "-d"},
			Want: "PA======\n",
		},
	}

	testutil.Run(t, New, tests)
}

// TestRunEncodeRFC4648 checks the encoding against the examples of RFC 4648.
func TestRunEncodeRFC4648(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"f", "MY======"},
		{"fo", "MZXQ===="},
		{"foo", "MZXW6==="},
		{"foob", "MZXW6YQ="},
		{"fooba", "MZXW6YTB"},
		{"foobar", "MZXW6YTBOI======"},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			testutil.Once(t, New, testutil.Case{
				Stdin: strings.NewReader(test.text),
				Want:  test.want + "\n",
			})
		})
	}
}

func TestRunEncodeWrap(t *testing.T) {
	groups := strings.Repeat("MFQWCYLB", 19)
	tests := []testutil.Case{
		{
			Name:  "the default wraps at the 76th column",
			Stdin: strings.NewReader(strings.Repeat("a", 95)),
			Want:  groups[:76] + "\n" + groups[76:] + "\n",
		},
		{
			Name:  "zero disables line wrapping",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"-w", "0"},
			Want:  "NBSWY3DP",
		},
		{
			Name:  "zero attached to the option",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"-w0"},
			Want:  "NBSWY3DP",
		},
		{
			Name:  "the long option with a value",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"--wrap=0"},
			Want:  "NBSWY3DP",
		},
		{
			Name:  "the long option with a separate value",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"--wrap", "0"},
			Want:  "NBSWY3DP",
		},
		{
			Name:  "a wrap size bigger than an int disables wrapping like GNU",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"-w", "99999999999999999999999"},
			Want:  "NBSWY3DP",
		},
		{
			Name:  "a wrap column holding two groups",
			Stdin: strings.NewReader(strings.Repeat("a", 20)),
			Args:  []string{"-w16"},
			Want:  "MFQWCYLBMFQWCYLB\nMFQWCYLBMFQWCYLB\n",
		},
		{
			Name:  "one character per line",
			Stdin: strings.NewReader("f"),
			Args:  []string{"-w", "1"},
			Want:  "M\nY\n=\n=\n=\n=\n=\n=\n",
		},
		{
			Name:  "zero wrapping an empty input",
			Stdin: strings.NewReader(""),
			Args:  []string{"-w", "0"},
			Want:  "",
		},
		{
			Name:  "wrapping is ignored when decoding",
			Stdin: strings.NewReader("NBSWY3DP\n"),
			Args:  []string{"-d", "-w", "4"},
			Want:  "hello",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunDecode(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "reads the standard input",
			Stdin: strings.NewReader("NBSWY3DP\n"),
			Args:  []string{"-d"},
			Want:  "hello",
		},
		{
			Name:  "the long option",
			Stdin: strings.NewReader("NBSWY3DP\n"),
			Args:  []string{"--decode"},
			Want:  "hello",
		},
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("NBSWY3DP"),
			Args:  []string{"-d", "-"},
			Want:  "hello",
		},
		{
			Name:  "empty input writes nothing",
			Stdin: strings.NewReader(""),
			Args:  []string{"-d"},
			Want:  "",
		},
		{
			Name:  "newlines are always ignored",
			Stdin: strings.NewReader("MZXW6\nYTB\n"),
			Args:  []string{"-d"},
			Want:  "fooba",
		},
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"-d", "lines.txt"},
			Want: "hellofoobar",
		},
		{
			Name:  "the last group is padded automatically",
			Stdin: strings.NewReader("MZXW6YTBOI"),
			Args:  []string{"-d"},
			Want:  "foobar",
		},
		{
			Name:  "auto padding of a single byte",
			Stdin: strings.NewReader("MY"),
			Args:  []string{"-d"},
			Want:  "f",
		},
		{
			Name:  "a padded group in the middle of the input",
			Stdin: strings.NewReader("MZXW6===NBSWY3DP"),
			Args:  []string{"-d"},
			Want:  "foohello",
		},
		{
			Name:  "only newlines",
			Stdin: strings.NewReader("\n\n\n"),
			Args:  []string{"-d"},
			Want:  "",
		},
	}

	testutil.Run(t, New, tests)
}

// TestRunDecodeRFC4648 checks the decoding against the examples of RFC 4648.
func TestRunDecodeRFC4648(t *testing.T) {
	tests := []struct {
		text  string
		input string
	}{
		{"f", "MY======"},
		{"fo", "MZXQ===="},
		{"foo", "MZXW6==="},
		{"foob", "MZXW6YQ="},
		{"fooba", "MZXW6YTB"},
		{"foobar", "MZXW6YTBOI======"},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			testutil.Once(t, New, testutil.Case{
				Stdin: strings.NewReader(test.input),
				Args:  []string{"-d"},
				Want:  test.text,
			})
		})
	}
}

func TestRunDecodeInvalid(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "a single character can't be padded",
			Stdin:    strings.NewReader("NBSWY3DPO"),
			Args:     []string{"-d"},
			Want:     "hello",
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a group with a single character",
			Stdin:    strings.NewReader("M"),
			Args:     []string{"-d"},
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a padding length that base32 doesn't use",
			Stdin:    strings.NewReader("MZXW6Y"),
			Args:     []string{"-d"},
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "padding shorter than a whole group",
			Stdin:    strings.NewReader("MZXW6Y="),
			Args:     []string{"-d"},
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a padding character in the middle of a group",
			Stdin:    strings.NewReader("MZX=W6YTB"),
			Args:     []string{"-d"},
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "non-canonical trailing bits of a single byte",
			Stdin:    strings.NewReader("MX======"),
			Args:     []string{"-d"},
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "non-canonical trailing bits of two bytes",
			Stdin:    strings.NewReader("MZXR===="),
			Args:     []string{"-d"},
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "non-canonical auto padding",
			Stdin:    strings.NewReader("MX"),
			Args:     []string{"-d"},
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "lowercase is garbage",
			Stdin:    strings.NewReader("nbswy3dp"),
			Args:     []string{"-d"},
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a character outside the alphabet",
			Stdin:    strings.NewReader("NBSW Y3DP"),
			Args:     []string{"-d"},
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a carriage return is garbage",
			Stdin:    strings.NewReader("NBSWY3DP\r"),
			Args:     []string{"-d"},
			Want:     "hello",
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a file with garbage",
			FS:       testFS(),
			Args:     []string{"-d", "garbage.txt"},
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunDecodeIgnoreGarbage(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the short option",
			Stdin: strings.NewReader("NBSW Y3DP"),
			Args:  []string{"-d", "-i"},
			Want:  "hello",
		},
		{
			Name:  "the long option",
			Stdin: strings.NewReader("NBSW*Y3DP"),
			Args:  []string{"-d", "--ignore-garbage"},
			Want:  "hello",
		},
		{
			Name:  "grouped with decode",
			Stdin: strings.NewReader("NBSWY3DP!!\n"),
			Args:  []string{"-di"},
			Want:  "hello",
		},
		{
			Name: "a file with garbage",
			FS:   testFS(),
			Args: []string{"-di", "garbage.txt"},
			Want: "hello",
		},
		{
			Name:  "garbage that breaks the group alignment",
			Stdin: strings.NewReader("NB*SW*Y3DP"),
			Args:  []string{"-di"},
			Want:  "hello",
		},
		{
			Name:     "garbage leaving an incomplete group",
			Stdin:    strings.NewReader("nbswy3dp"),
			Args:     []string{"-di"},
			WantErr:  "base32: invalid input\n",
			WantExit: 1,
		},
		{
			Name:  "everything is garbage",
			Stdin: strings.NewReader("nbswyp"),
			Args:  []string{"-di"},
			Want:  "",
		},
		{
			Name:  "a carriage return",
			Stdin: strings.NewReader("NBSWY3DP\r"),
			Args:  []string{"-di"},
			Want:  "hello",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "an unknown short option",
			Args:     []string{"-q"},
			WantErr:  "base32: invalid option -- 'q'\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown option inside a group",
			Args:     []string{"-dq"},
			WantErr:  "base32: invalid option -- 'q'\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown long option",
			Args:     []string{"--foo"},
			WantErr:  "base32: unrecognized option '--foo'\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "w requires an argument",
			Args:     []string{"-w"},
			WantErr:  "base32: option requires an argument -- 'w'\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "wrap requires an argument",
			Args:     []string{"--wrap"},
			WantErr:  "base32: option '--wrap' requires an argument\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid wrap size",
			Args:     []string{"-w", "x"},
			WantErr:  "base32: invalid wrap size: 'x'\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative wrap size",
			Args:     []string{"-w", "-1"},
			WantErr:  "base32: invalid wrap size: '-1'\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative wrap size overflowing an int",
			Args:     []string{"-w", "-99999999999999999999999"},
			WantErr:  "base32: invalid wrap size: '-99999999999999999999999'\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty wrap size",
			Args:     []string{"-w", ""},
			WantErr:  "base32: invalid wrap size: ''\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid wrap size attached to the option",
			Args:     []string{"-wx"},
			WantErr:  "base32: invalid wrap size: 'x'\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid wrap size for the long option",
			Args:     []string{"--wrap=x"},
			WantErr:  "base32: invalid wrap size: 'x'\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an extra operand",
			FS:       testFS(),
			Args:     []string{"hello.txt", "empty.txt"},
			WantErr:  "base32: extra operand 'empty.txt'\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an operand after the file is an extra operand",
			FS:       testFS(),
			Args:     []string{"hello.txt", "-d"},
			WantErr:  "base32: extra operand '-d'\nTry 'base32 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "the last wrap size wins",
			Stdin:    strings.NewReader("hello"),
			Args:     []string{"-w8", "-w0"},
			Want:     "NBSWY3DP",
			WantExit: 0,
		},
		{
			Name:     "repeated options",
			Stdin:    strings.NewReader("NBSWY3DP\n"),
			Args:     []string{"-d", "-d"},
			Want:     "hello",
			WantExit: 0,
		},
		{
			Name:     "ignore garbage is a no-op when encoding",
			Stdin:    strings.NewReader("hello"),
			Args:     []string{"-i"},
			Want:     "NBSWY3DP\n",
			WantExit: 0,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "a missing file",
			FS:       testFS(),
			Args:     []string{"missing.txt"},
			WantErr:  "base32: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file when decoding",
			FS:       testFS(),
			Args:     []string{"-d", "missing.txt"},
			WantErr:  "base32: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a zero-length file name",
			FS:       testFS(),
			Args:     []string{""},
			WantErr:  "base32: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			Args:     []string{"-w", "0"},
			WantErr:  "base32: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "no file system",
			Args:     []string{"hello.txt"},
			WantErr:  "base32: hello.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "a read error on the standard input",
			Stdin:    testutil.ErrorReader{},
			WantErr:  "base32: standard input: boom\n",
			WantExit: 1,
		},
		{
			Name:     "a read error when decoding",
			Stdin:    testutil.ErrorReader{},
			Args:     []string{"-d"},
			WantErr:  "base32: standard input: boom\n",
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
			Args:    []string{"-w", "0"},
			WantErr: "base32: write error: boom\n",
		},
		{
			Name:    "decoding",
			Stdin:   strings.NewReader("NBSWY3DP"),
			Args:    []string{"-d"},
			WantErr: "base32: write error: boom\n",
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
		testutil.Cancelled(t, New, strings.NewReader("hello"), nil)
	})

	// base32 only notices the cancellation after writing the chunk it had
	// already read, so this one can't use testutil.Cancelled: some output is
	// expected, unlike in testutil.Cancelled's assertion of no output at all.
	t.Run("cancelled while reading", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  &testutil.CancelReader{Cancel: cancel, Data: []byte("hello")},
			Stdout: new(bytes.Buffer),
			Stderr: stderr,
		}
		err := New(rt).Run(ctx)

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
	})
}

// TestRunRoundTrip encodes an input big enough to span several read chunks and
// decodes it back.
func TestRunRoundTrip(t *testing.T) {
	input := make([]byte, 100_001)
	for i := range input {
		input[i] = byte(i)
	}

	encoded := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdin: bytes.NewReader(input), Stdout: encoded, Stderr: io.Discard}
	if err := New(rt).Run(t.Context()); err != nil {
		t.Fatalf("Run() encoding error = %v", err)
	}
	if n := strings.Count(encoded.String(), "="); n > 6 {
		t.Errorf("encoded output has %d padding characters, want at most 6", n)
	}

	decoded := new(bytes.Buffer)
	rt = &coreutils.Runtime{Stdin: encoded, Stdout: decoded, Stderr: io.Discard}
	if err := New(rt).Run(t.Context(), "-d"); err != nil {
		t.Fatalf("Run() decoding error = %v", err)
	}
	if !bytes.Equal(decoded.Bytes(), input) {
		t.Errorf("round trip decoded %d bytes, want the %d bytes of the input", decoded.Len(), len(input))
	}
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
		want := "base32 (charm.land/turtle/coreutils) " + coreutils.Version +
			"\nhttps://charm.land/turtle\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
