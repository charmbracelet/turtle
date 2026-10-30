package base32

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

// testFS holds the fixtures used by the tests:
//
//	hello.txt:   "hello"
//	empty.txt:   ""
//	lines.txt:   "NBSWY3DP\nMZXW6YTBOI======\n"
//	garbage.txt: "NBSW Y3DP"
//	"-d":        "x"
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"hello.txt":     {Data: []byte("hello")},
		"empty.txt":     {Data: []byte("")},
		"lines.txt":     {Data: []byte("NBSWY3DP\nMZXW6YTBOI======\n")},
		"garbage.txt":   {Data: []byte("NBSW Y3DP")},
		"-d":            {Data: []byte("x")},
		"sub/inner.txt": {Data: []byte("inner\n")},
	}
}

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

// errorReader always fails to be read.
type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

// errorWriter always fails to be written to.
type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

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
			name:  "no operands reads the standard input",
			stdin: strings.NewReader("hello"),
			want:  "NBSWY3DP\n",
		},
		{
			name:  "empty input writes nothing",
			stdin: strings.NewReader(""),
			want:  "",
		},
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("hello"),
			args:  []string{"-"},
			want:  "NBSWY3DP\n",
		},
		{
			name:  "double dash with no operands reads the standard input",
			stdin: strings.NewReader("hello"),
			args:  []string{"--"},
			want:  "NBSWY3DP\n",
		},
		{
			name: "a file operand",
			fsys: testFS(),
			args: []string{"hello.txt"},
			want: "NBSWY3DP\n",
		},
		{
			name: "an empty file",
			fsys: testFS(),
			args: []string{"empty.txt"},
			want: "",
		},
		{
			name: "the file is resolved against the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt"},
			want: "NFXG4ZLSBI======\n",
		},
		{
			name: "a file named like an option after a double dash",
			fsys: testFS(),
			args: []string{"--", "-d"},
			want: "PA======\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
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
			runCase(t, testCase{
				stdin: strings.NewReader(test.text),
				want:  test.want + "\n",
			})
		})
	}
}

func TestRunEncodeWrap(t *testing.T) {
	groups := strings.Repeat("MFQWCYLB", 19)
	tests := []testCase{
		{
			name:  "the default wraps at the 76th column",
			stdin: strings.NewReader(strings.Repeat("a", 95)),
			want:  groups[:76] + "\n" + groups[76:] + "\n",
		},
		{
			name:  "zero disables line wrapping",
			stdin: strings.NewReader("hello"),
			args:  []string{"-w", "0"},
			want:  "NBSWY3DP",
		},
		{
			name:  "zero attached to the option",
			stdin: strings.NewReader("hello"),
			args:  []string{"-w0"},
			want:  "NBSWY3DP",
		},
		{
			name:  "the long option with a value",
			stdin: strings.NewReader("hello"),
			args:  []string{"--wrap=0"},
			want:  "NBSWY3DP",
		},
		{
			name:  "the long option with a separate value",
			stdin: strings.NewReader("hello"),
			args:  []string{"--wrap", "0"},
			want:  "NBSWY3DP",
		},
		{
			name:  "a wrap size bigger than an int disables wrapping like GNU",
			stdin: strings.NewReader("hello"),
			args:  []string{"-w", "99999999999999999999999"},
			want:  "NBSWY3DP",
		},
		{
			name:  "a wrap column holding two groups",
			stdin: strings.NewReader(strings.Repeat("a", 20)),
			args:  []string{"-w16"},
			want:  "MFQWCYLBMFQWCYLB\nMFQWCYLBMFQWCYLB\n",
		},
		{
			name:  "one character per line",
			stdin: strings.NewReader("f"),
			args:  []string{"-w", "1"},
			want:  "M\nY\n=\n=\n=\n=\n=\n=\n",
		},
		{
			name:  "zero wrapping an empty input",
			stdin: strings.NewReader(""),
			args:  []string{"-w", "0"},
			want:  "",
		},
		{
			name:  "wrapping is ignored when decoding",
			stdin: strings.NewReader("NBSWY3DP\n"),
			args:  []string{"-d", "-w", "4"},
			want:  "hello",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDecode(t *testing.T) {
	tests := []testCase{
		{
			name:  "reads the standard input",
			stdin: strings.NewReader("NBSWY3DP\n"),
			args:  []string{"-d"},
			want:  "hello",
		},
		{
			name:  "the long option",
			stdin: strings.NewReader("NBSWY3DP\n"),
			args:  []string{"--decode"},
			want:  "hello",
		},
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("NBSWY3DP"),
			args:  []string{"-d", "-"},
			want:  "hello",
		},
		{
			name:  "empty input writes nothing",
			stdin: strings.NewReader(""),
			args:  []string{"-d"},
			want:  "",
		},
		{
			name:  "newlines are always ignored",
			stdin: strings.NewReader("MZXW6\nYTB\n"),
			args:  []string{"-d"},
			want:  "fooba",
		},
		{
			name: "a file operand",
			fsys: testFS(),
			args: []string{"-d", "lines.txt"},
			want: "hellofoobar",
		},
		{
			name:  "the last group is padded automatically",
			stdin: strings.NewReader("MZXW6YTBOI"),
			args:  []string{"-d"},
			want:  "foobar",
		},
		{
			name:  "auto padding of a single byte",
			stdin: strings.NewReader("MY"),
			args:  []string{"-d"},
			want:  "f",
		},
		{
			name:  "a padded group in the middle of the input",
			stdin: strings.NewReader("MZXW6===NBSWY3DP"),
			args:  []string{"-d"},
			want:  "foohello",
		},
		{
			name:  "only newlines",
			stdin: strings.NewReader("\n\n\n"),
			args:  []string{"-d"},
			want:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
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
			runCase(t, testCase{
				stdin: strings.NewReader(test.input),
				args:  []string{"-d"},
				want:  test.text,
			})
		})
	}
}

func TestRunDecodeInvalid(t *testing.T) {
	tests := []testCase{
		{
			name:     "a single character can't be padded",
			stdin:    strings.NewReader("NBSWY3DPO"),
			args:     []string{"-d"},
			want:     "hello",
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a group with a single character",
			stdin:    strings.NewReader("M"),
			args:     []string{"-d"},
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a padding length that base32 doesn't use",
			stdin:    strings.NewReader("MZXW6Y"),
			args:     []string{"-d"},
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "padding shorter than a whole group",
			stdin:    strings.NewReader("MZXW6Y="),
			args:     []string{"-d"},
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a padding character in the middle of a group",
			stdin:    strings.NewReader("MZX=W6YTB"),
			args:     []string{"-d"},
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "non-canonical trailing bits of a single byte",
			stdin:    strings.NewReader("MX======"),
			args:     []string{"-d"},
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "non-canonical trailing bits of two bytes",
			stdin:    strings.NewReader("MZXR===="),
			args:     []string{"-d"},
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "non-canonical auto padding",
			stdin:    strings.NewReader("MX"),
			args:     []string{"-d"},
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "lowercase is garbage",
			stdin:    strings.NewReader("nbswy3dp"),
			args:     []string{"-d"},
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a character outside the alphabet",
			stdin:    strings.NewReader("NBSW Y3DP"),
			args:     []string{"-d"},
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a carriage return is garbage",
			stdin:    strings.NewReader("NBSWY3DP\r"),
			args:     []string{"-d"},
			want:     "hello",
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a file with garbage",
			fsys:     testFS(),
			args:     []string{"-d", "garbage.txt"},
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDecodeIgnoreGarbage(t *testing.T) {
	tests := []testCase{
		{
			name:  "the short option",
			stdin: strings.NewReader("NBSW Y3DP"),
			args:  []string{"-d", "-i"},
			want:  "hello",
		},
		{
			name:  "the long option",
			stdin: strings.NewReader("NBSW*Y3DP"),
			args:  []string{"-d", "--ignore-garbage"},
			want:  "hello",
		},
		{
			name:  "grouped with decode",
			stdin: strings.NewReader("NBSWY3DP!!\n"),
			args:  []string{"-di"},
			want:  "hello",
		},
		{
			name: "a file with garbage",
			fsys: testFS(),
			args: []string{"-di", "garbage.txt"},
			want: "hello",
		},
		{
			name:  "garbage that breaks the group alignment",
			stdin: strings.NewReader("NB*SW*Y3DP"),
			args:  []string{"-di"},
			want:  "hello",
		},
		{
			name:     "garbage leaving an incomplete group",
			stdin:    strings.NewReader("nbswy3dp"),
			args:     []string{"-di"},
			wantErr:  "base32: invalid input\n",
			wantExit: 1,
		},
		{
			name:  "everything is garbage",
			stdin: strings.NewReader("nbswyp"),
			args:  []string{"-di"},
			want:  "",
		},
		{
			name:  "a carriage return",
			stdin: strings.NewReader("NBSWY3DP\r"),
			args:  []string{"-di"},
			want:  "hello",
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
			name:     "an unknown short option",
			args:     []string{"-q"},
			wantErr:  "base32: invalid option -- 'q'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown option inside a group",
			args:     []string{"-dq"},
			wantErr:  "base32: invalid option -- 'q'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown long option",
			args:     []string{"--foo"},
			wantErr:  "base32: unrecognized option '--foo'\n",
			wantExit: 2,
		},
		{
			name:     "w requires an argument",
			args:     []string{"-w"},
			wantErr:  "base32: option requires an argument -- 'w'\n",
			wantExit: 2,
		},
		{
			name:     "wrap requires an argument",
			args:     []string{"--wrap"},
			wantErr:  "base32: option '--wrap' requires an argument\n",
			wantExit: 2,
		},
		{
			name:     "an invalid wrap size",
			args:     []string{"-w", "x"},
			wantErr:  "base32: invalid wrap size: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "a negative wrap size",
			args:     []string{"-w", "-1"},
			wantErr:  "base32: invalid wrap size: '-1'\n",
			wantExit: 2,
		},
		{
			name:     "a negative wrap size overflowing an int",
			args:     []string{"-w", "-99999999999999999999999"},
			wantErr:  "base32: invalid wrap size: '-99999999999999999999999'\n",
			wantExit: 2,
		},
		{
			name:     "an empty wrap size",
			args:     []string{"-w", ""},
			wantErr:  "base32: invalid wrap size: ''\n",
			wantExit: 2,
		},
		{
			name:     "an invalid wrap size attached to the option",
			args:     []string{"-wx"},
			wantErr:  "base32: invalid wrap size: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an invalid wrap size for the long option",
			args:     []string{"--wrap=x"},
			wantErr:  "base32: invalid wrap size: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an extra operand",
			fsys:     testFS(),
			args:     []string{"hello.txt", "empty.txt"},
			wantErr:  "base32: extra operand 'empty.txt'\n",
			wantExit: 2,
		},
		{
			name:     "an operand after the file is an extra operand",
			fsys:     testFS(),
			args:     []string{"hello.txt", "-d"},
			wantErr:  "base32: extra operand '-d'\n",
			wantExit: 2,
		},
		{
			name:     "the last wrap size wins",
			stdin:    strings.NewReader("hello"),
			args:     []string{"-w8", "-w0"},
			want:     "NBSWY3DP",
			wantExit: 0,
		},
		{
			name:     "repeated options",
			stdin:    strings.NewReader("NBSWY3DP\n"),
			args:     []string{"-d", "-d"},
			want:     "hello",
			wantExit: 0,
		},
		{
			name:     "ignore garbage is a no-op when encoding",
			stdin:    strings.NewReader("hello"),
			args:     []string{"-i"},
			want:     "NBSWY3DP\n",
			wantExit: 0,
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
			name:     "a missing file",
			fsys:     testFS(),
			args:     []string{"missing.txt"},
			wantErr:  "base32: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing file when decoding",
			fsys:     testFS(),
			args:     []string{"-d", "missing.txt"},
			wantErr:  "base32: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a zero-length file name",
			fsys:     testFS(),
			args:     []string{""},
			wantErr:  "base32: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			args:     []string{"-w", "0"},
			wantErr:  "base32: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "no file system",
			args:     []string{"hello.txt"},
			wantErr:  "base32: hello.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "a read error on the standard input",
			stdin:    errorReader{},
			wantErr:  "base32: standard input: boom\n",
			wantExit: 1,
		},
		{
			name:     "a read error when decoding",
			stdin:    errorReader{},
			args:     []string{"-d"},
			wantErr:  "base32: standard input: boom\n",
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
			args:    []string{"-w", "0"},
			wantErr: "base32: write error: boom\n",
		},
		{
			name:    "decoding",
			stdin:   strings.NewReader("NBSWY3DP"),
			args:    []string{"-d"},
			wantErr: "base32: write error: boom\n",
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

// runCancelled runs the utility with a context that is either already done or
// becomes done while reading, and checks that the context error is returned
// without any diagnostic.
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
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
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
		want := "base32 (charm.land/snail/coreutils) " + coreutils.Version +
			"\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
