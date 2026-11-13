package base64

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
//	lines.txt:   "aGVsbG8=\nZm9vYmFy\n"
//	garbage.txt: "aGVs bG8="
//	"-d":        "x"
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"hello.txt":     {Data: []byte("hello")},
		"empty.txt":     {Data: []byte("")},
		"lines.txt":     {Data: []byte("aGVsbG8=\nZm9vYmFy\n")},
		"garbage.txt":   {Data: []byte("aGVs bG8=")},
		"-d":            {Data: []byte("x")},
		"sub/inner.txt": {Data: []byte("inner\n")},
	}
}

func TestRunEncode(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no operands reads the standard input",
			Stdin: strings.NewReader("hello"),
			Want:  "aGVsbG8=\n",
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
			Want:  "aGVsbG8=\n",
		},
		{
			Name:  "double dash with no operands reads the standard input",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"--"},
			Want:  "aGVsbG8=\n",
		},
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"hello.txt"},
			Want: "aGVsbG8=\n",
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
			Want: "aW5uZXIK\n",
		},
		{
			Name: "a file named like an option after a double dash",
			FS:   testFS(),
			Args: []string{"--", "-d"},
			Want: "eA==\n",
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
		{"f", "Zg=="},
		{"fo", "Zm8="},
		{"foo", "Zm9v"},
		{"foob", "Zm9vYg=="},
		{"fooba", "Zm9vYmE="},
		{"foobar", "Zm9vYmFy"},
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
	a60 := strings.Repeat("a", 60)
	tests := []testutil.Case{
		{
			Name:  "the default wraps at the 76th column",
			Stdin: strings.NewReader(a60),
			Want:  strings.Repeat("YWFh", 19) + "\n" + "YWFh" + "\n",
		},
		{
			Name:  "an exact multiple of the wrap column keeps the line break",
			Stdin: strings.NewReader(strings.Repeat("a", 57)),
			Want:  strings.Repeat("YWFh", 19) + "\n",
		},
		{
			Name:  "zero disables line wrapping",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"-w", "0"},
			Want:  "aGVsbG8=",
		},
		{
			Name:  "zero attached to the option",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"-w0"},
			Want:  "aGVsbG8=",
		},
		{
			Name:  "the long option with a value",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"--wrap=0"},
			Want:  "aGVsbG8=",
		},
		{
			Name:  "the long option with a separate value",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"--wrap", "0"},
			Want:  "aGVsbG8=",
		},
		{
			Name:  "a wrap size bigger than an int disables wrapping like GNU",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"-w", "99999999999999999999999"},
			Want:  "aGVsbG8=",
		},
		{
			Name:  "a short wrap column",
			Stdin: strings.NewReader("foobar"),
			Args:  []string{"-w4"},
			Want:  "Zm9v\nYmFy\n",
		},
		{
			Name:  "one character per line",
			Stdin: strings.NewReader("foo"),
			Args:  []string{"-w", "1"},
			Want:  "Z\nm\n9\nv\n",
		},
		{
			Name:  "zero wrapping an empty input",
			Stdin: strings.NewReader(""),
			Args:  []string{"-w", "0"},
			Want:  "",
		},
		{
			Name:  "wrapping is ignored when decoding",
			Stdin: strings.NewReader("aGVsbG8=\n"),
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
			Stdin: strings.NewReader("aGVsbG8=\n"),
			Args:  []string{"-d"},
			Want:  "hello",
		},
		{
			Name:  "the long option",
			Stdin: strings.NewReader("aGVsbG8=\n"),
			Args:  []string{"--decode"},
			Want:  "hello",
		},
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("aGVsbG8="),
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
			Stdin: strings.NewReader("aGVs\nbG8=\n"),
			Args:  []string{"-d"},
			Want:  "hello",
		},
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"-d", "lines.txt"},
			Want: "hellofoobar",
		},
		{
			Name:  "the last group is padded automatically",
			Stdin: strings.NewReader("aGVsbG8"),
			Args:  []string{"-d"},
			Want:  "hello",
		},
		{
			Name:  "auto padding of two characters",
			Stdin: strings.NewReader("Zg"),
			Args:  []string{"-d"},
			Want:  "f",
		},
		{
			Name:  "a padded group in the middle of the input",
			Stdin: strings.NewReader("aGVsbG8=YWJj"),
			Args:  []string{"-d"},
			Want:  "helloabc",
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
		{"f", "Zg=="},
		{"fo", "Zm8="},
		{"foo", "Zm9v"},
		{"foob", "Zm9vYg=="},
		{"fooba", "Zm9vYmE="},
		{"foobar", "Zm9vYmFy"},
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
			Stdin:    strings.NewReader("aGVsb"),
			Args:     []string{"-d"},
			Want:     "hel",
			WantErr:  "base64: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a group with a single character",
			Stdin:    strings.NewReader("a"),
			Args:     []string{"-d"},
			WantErr:  "base64: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "padding shorter than a whole group",
			Stdin:    strings.NewReader("aGVsbG="),
			Args:     []string{"-d"},
			Want:     "hel",
			WantErr:  "base64: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a padding character in the middle of a group",
			Stdin:    strings.NewReader("aGVsab=c"),
			Args:     []string{"-d"},
			Want:     "hel",
			WantErr:  "base64: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "only padding characters",
			Stdin:    strings.NewReader("aGVs===="),
			Args:     []string{"-d"},
			Want:     "hel",
			WantErr:  "base64: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "non-canonical trailing bits",
			Stdin:    strings.NewReader("aGVsbG9="),
			Args:     []string{"-d"},
			Want:     "hel",
			WantErr:  "base64: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "ignore garbage doesn't fix the padding",
			Stdin:    strings.NewReader("aGVs bG9="),
			Args:     []string{"-di"},
			Want:     "hel",
			WantErr:  "base64: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a character outside the alphabet",
			Stdin:    strings.NewReader("aGVs bG8="),
			Args:     []string{"-d"},
			Want:     "hel",
			WantErr:  "base64: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a carriage return is garbage",
			Stdin:    strings.NewReader("aGVsbG8=\r"),
			Args:     []string{"-d"},
			Want:     "hello",
			WantErr:  "base64: invalid input\n",
			WantExit: 1,
		},
		{
			Name:     "a file with garbage",
			FS:       testFS(),
			Args:     []string{"-d", "garbage.txt"},
			Want:     "hel",
			WantErr:  "base64: invalid input\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunDecodeIgnoreGarbage(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the short option",
			Stdin: strings.NewReader("aGVs bG8="),
			Args:  []string{"-d", "-i"},
			Want:  "hello",
		},
		{
			Name:  "the long option",
			Stdin: strings.NewReader("aGVs*bG8="),
			Args:  []string{"-d", "--ignore-garbage"},
			Want:  "hello",
		},
		{
			Name:  "grouped with decode",
			Stdin: strings.NewReader("aGVsbG8=!!\n"),
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
			Name:  "a carriage return",
			Stdin: strings.NewReader("aGVsbG8=\r"),
			Args:  []string{"-di"},
			Want:  "hello",
		},
		{
			Name:  "garbage that breaks the group alignment",
			Stdin: strings.NewReader("aG*Vs*bG8="),
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
			WantErr:  "base64: invalid option -- 'q'\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown option inside a group",
			Args:     []string{"-dq"},
			WantErr:  "base64: invalid option -- 'q'\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown long option",
			Args:     []string{"--foo"},
			WantErr:  "base64: unrecognized option '--foo'\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "w requires an argument",
			Args:     []string{"-w"},
			WantErr:  "base64: option requires an argument -- 'w'\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "wrap requires an argument",
			Args:     []string{"--wrap"},
			WantErr:  "base64: option '--wrap' requires an argument\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid wrap size",
			Args:     []string{"-w", "x"},
			WantErr:  "base64: invalid wrap size: 'x'\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative wrap size",
			Args:     []string{"-w", "-1"},
			WantErr:  "base64: invalid wrap size: '-1'\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative wrap size overflowing an int",
			Args:     []string{"-w", "-99999999999999999999999"},
			WantErr:  "base64: invalid wrap size: '-99999999999999999999999'\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty wrap size",
			Args:     []string{"-w", ""},
			WantErr:  "base64: invalid wrap size: ''\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid wrap size attached to the option",
			Args:     []string{"-wx"},
			WantErr:  "base64: invalid wrap size: 'x'\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid wrap size for the long option",
			Args:     []string{"--wrap=x"},
			WantErr:  "base64: invalid wrap size: 'x'\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an extra operand",
			FS:       testFS(),
			Args:     []string{"hello.txt", "empty.txt"},
			WantErr:  "base64: extra operand 'empty.txt'\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an operand after the file is an extra operand",
			FS:       testFS(),
			Args:     []string{"hello.txt", "-d"},
			WantErr:  "base64: extra operand '-d'\nTry 'base64 --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "the last wrap size wins",
			Stdin:    strings.NewReader("hello"),
			Args:     []string{"-w8", "-w0"},
			Want:     "aGVsbG8=",
			WantExit: 0,
		},
		{
			Name:     "repeated options",
			Stdin:    strings.NewReader("aGVsbG8=\n"),
			Args:     []string{"-d", "-d"},
			Want:     "hello",
			WantExit: 0,
		},
		{
			Name:     "ignore garbage is a no-op when encoding",
			Stdin:    strings.NewReader("hello"),
			Args:     []string{"-i"},
			Want:     "aGVsbG8=\n",
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
			WantErr:  "base64: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file when decoding",
			FS:       testFS(),
			Args:     []string{"-d", "missing.txt"},
			WantErr:  "base64: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a zero-length file name",
			FS:       testFS(),
			Args:     []string{""},
			WantErr:  "base64: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			Args:     []string{"-w", "0"},
			WantErr:  "base64: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "no file system",
			Args:     []string{"hello.txt"},
			WantErr:  "base64: hello.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "a read error on the standard input",
			Stdin:    testutil.ErrorReader{},
			WantErr:  "base64: standard input: boom\n",
			WantExit: 1,
		},
		{
			Name:     "a read error when decoding",
			Stdin:    testutil.ErrorReader{},
			Args:     []string{"-d"},
			WantErr:  "base64: standard input: boom\n",
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
			WantErr: "base64: write error: boom\n",
		},
		{
			Name:    "decoding",
			Stdin:   strings.NewReader("aGVsbG8="),
			Args:    []string{"-d"},
			WantErr: "base64: write error: boom\n",
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

	// base64 only notices the cancellation after writing the chunk it had
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
	input := make([]byte, 100_000)
	for i := range input {
		input[i] = byte(i)
	}

	encoded := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdin: bytes.NewReader(input), Stdout: encoded, Stderr: io.Discard}
	if err := New(rt).Run(t.Context()); err != nil {
		t.Fatalf("Run() encoding error = %v", err)
	}
	if n := strings.Count(encoded.String(), "="); n > 2 {
		t.Errorf("encoded output has %d padding characters, want at most 2", n)
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
		want := "base64 (charm.land/turtle/coreutils) " + coreutils.Version +
			"\nhttps://charm.land/turtle\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
