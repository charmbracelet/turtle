package base64

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
			want:  "aGVsbG8=\n",
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
			want:  "aGVsbG8=\n",
		},
		{
			name:  "double dash with no operands reads the standard input",
			stdin: strings.NewReader("hello"),
			args:  []string{"--"},
			want:  "aGVsbG8=\n",
		},
		{
			name: "a file operand",
			fsys: testFS(),
			args: []string{"hello.txt"},
			want: "aGVsbG8=\n",
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
			want: "aW5uZXIK\n",
		},
		{
			name: "a file named like an option after a double dash",
			fsys: testFS(),
			args: []string{"--", "-d"},
			want: "eA==\n",
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
		{"f", "Zg=="},
		{"fo", "Zm8="},
		{"foo", "Zm9v"},
		{"foob", "Zm9vYg=="},
		{"fooba", "Zm9vYmE="},
		{"foobar", "Zm9vYmFy"},
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
	a60 := strings.Repeat("a", 60)
	tests := []testCase{
		{
			name:  "the default wraps at the 76th column",
			stdin: strings.NewReader(a60),
			want:  strings.Repeat("YWFh", 19) + "\n" + "YWFh" + "\n",
		},
		{
			name:  "an exact multiple of the wrap column keeps the line break",
			stdin: strings.NewReader(strings.Repeat("a", 57)),
			want:  strings.Repeat("YWFh", 19) + "\n",
		},
		{
			name:  "zero disables line wrapping",
			stdin: strings.NewReader("hello"),
			args:  []string{"-w", "0"},
			want:  "aGVsbG8=",
		},
		{
			name:  "zero attached to the option",
			stdin: strings.NewReader("hello"),
			args:  []string{"-w0"},
			want:  "aGVsbG8=",
		},
		{
			name:  "the long option with a value",
			stdin: strings.NewReader("hello"),
			args:  []string{"--wrap=0"},
			want:  "aGVsbG8=",
		},
		{
			name:  "the long option with a separate value",
			stdin: strings.NewReader("hello"),
			args:  []string{"--wrap", "0"},
			want:  "aGVsbG8=",
		},
		{
			name:  "a wrap size bigger than an int disables wrapping like GNU",
			stdin: strings.NewReader("hello"),
			args:  []string{"-w", "99999999999999999999999"},
			want:  "aGVsbG8=",
		},
		{
			name:  "a short wrap column",
			stdin: strings.NewReader("foobar"),
			args:  []string{"-w4"},
			want:  "Zm9v\nYmFy\n",
		},
		{
			name:  "one character per line",
			stdin: strings.NewReader("foo"),
			args:  []string{"-w", "1"},
			want:  "Z\nm\n9\nv\n",
		},
		{
			name:  "zero wrapping an empty input",
			stdin: strings.NewReader(""),
			args:  []string{"-w", "0"},
			want:  "",
		},
		{
			name:  "wrapping is ignored when decoding",
			stdin: strings.NewReader("aGVsbG8=\n"),
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
			stdin: strings.NewReader("aGVsbG8=\n"),
			args:  []string{"-d"},
			want:  "hello",
		},
		{
			name:  "the long option",
			stdin: strings.NewReader("aGVsbG8=\n"),
			args:  []string{"--decode"},
			want:  "hello",
		},
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("aGVsbG8="),
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
			stdin: strings.NewReader("aGVs\nbG8=\n"),
			args:  []string{"-d"},
			want:  "hello",
		},
		{
			name: "a file operand",
			fsys: testFS(),
			args: []string{"-d", "lines.txt"},
			want: "hellofoobar",
		},
		{
			name:  "the last group is padded automatically",
			stdin: strings.NewReader("aGVsbG8"),
			args:  []string{"-d"},
			want:  "hello",
		},
		{
			name:  "auto padding of two characters",
			stdin: strings.NewReader("Zg"),
			args:  []string{"-d"},
			want:  "f",
		},
		{
			name:  "a padded group in the middle of the input",
			stdin: strings.NewReader("aGVsbG8=YWJj"),
			args:  []string{"-d"},
			want:  "helloabc",
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
		{"f", "Zg=="},
		{"fo", "Zm8="},
		{"foo", "Zm9v"},
		{"foob", "Zm9vYg=="},
		{"fooba", "Zm9vYmE="},
		{"foobar", "Zm9vYmFy"},
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
			stdin:    strings.NewReader("aGVsb"),
			args:     []string{"-d"},
			want:     "hel",
			wantErr:  "base64: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a group with a single character",
			stdin:    strings.NewReader("a"),
			args:     []string{"-d"},
			wantErr:  "base64: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "padding shorter than a whole group",
			stdin:    strings.NewReader("aGVsbG="),
			args:     []string{"-d"},
			want:     "hel",
			wantErr:  "base64: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a padding character in the middle of a group",
			stdin:    strings.NewReader("aGVsab=c"),
			args:     []string{"-d"},
			want:     "hel",
			wantErr:  "base64: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "only padding characters",
			stdin:    strings.NewReader("aGVs===="),
			args:     []string{"-d"},
			want:     "hel",
			wantErr:  "base64: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "non-canonical trailing bits",
			stdin:    strings.NewReader("aGVsbG9="),
			args:     []string{"-d"},
			want:     "hel",
			wantErr:  "base64: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "ignore garbage doesn't fix the padding",
			stdin:    strings.NewReader("aGVs bG9="),
			args:     []string{"-di"},
			want:     "hel",
			wantErr:  "base64: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a character outside the alphabet",
			stdin:    strings.NewReader("aGVs bG8="),
			args:     []string{"-d"},
			want:     "hel",
			wantErr:  "base64: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a carriage return is garbage",
			stdin:    strings.NewReader("aGVsbG8=\r"),
			args:     []string{"-d"},
			want:     "hello",
			wantErr:  "base64: invalid input\n",
			wantExit: 1,
		},
		{
			name:     "a file with garbage",
			fsys:     testFS(),
			args:     []string{"-d", "garbage.txt"},
			want:     "hel",
			wantErr:  "base64: invalid input\n",
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
			stdin: strings.NewReader("aGVs bG8="),
			args:  []string{"-d", "-i"},
			want:  "hello",
		},
		{
			name:  "the long option",
			stdin: strings.NewReader("aGVs*bG8="),
			args:  []string{"-d", "--ignore-garbage"},
			want:  "hello",
		},
		{
			name:  "grouped with decode",
			stdin: strings.NewReader("aGVsbG8=!!\n"),
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
			name:  "a carriage return",
			stdin: strings.NewReader("aGVsbG8=\r"),
			args:  []string{"-di"},
			want:  "hello",
		},
		{
			name:  "garbage that breaks the group alignment",
			stdin: strings.NewReader("aG*Vs*bG8="),
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
			wantErr:  "base64: invalid option -- 'q'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown option inside a group",
			args:     []string{"-dq"},
			wantErr:  "base64: invalid option -- 'q'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown long option",
			args:     []string{"--foo"},
			wantErr:  "base64: unrecognized option '--foo'\n",
			wantExit: 2,
		},
		{
			name:     "w requires an argument",
			args:     []string{"-w"},
			wantErr:  "base64: option requires an argument -- 'w'\n",
			wantExit: 2,
		},
		{
			name:     "wrap requires an argument",
			args:     []string{"--wrap"},
			wantErr:  "base64: option '--wrap' requires an argument\n",
			wantExit: 2,
		},
		{
			name:     "an invalid wrap size",
			args:     []string{"-w", "x"},
			wantErr:  "base64: invalid wrap size: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "a negative wrap size",
			args:     []string{"-w", "-1"},
			wantErr:  "base64: invalid wrap size: '-1'\n",
			wantExit: 2,
		},
		{
			name:     "a negative wrap size overflowing an int",
			args:     []string{"-w", "-99999999999999999999999"},
			wantErr:  "base64: invalid wrap size: '-99999999999999999999999'\n",
			wantExit: 2,
		},
		{
			name:     "an empty wrap size",
			args:     []string{"-w", ""},
			wantErr:  "base64: invalid wrap size: ''\n",
			wantExit: 2,
		},
		{
			name:     "an invalid wrap size attached to the option",
			args:     []string{"-wx"},
			wantErr:  "base64: invalid wrap size: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an invalid wrap size for the long option",
			args:     []string{"--wrap=x"},
			wantErr:  "base64: invalid wrap size: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an extra operand",
			fsys:     testFS(),
			args:     []string{"hello.txt", "empty.txt"},
			wantErr:  "base64: extra operand 'empty.txt'\n",
			wantExit: 2,
		},
		{
			name:     "an operand after the file is an extra operand",
			fsys:     testFS(),
			args:     []string{"hello.txt", "-d"},
			wantErr:  "base64: extra operand '-d'\n",
			wantExit: 2,
		},
		{
			name:     "the last wrap size wins",
			stdin:    strings.NewReader("hello"),
			args:     []string{"-w8", "-w0"},
			want:     "aGVsbG8=",
			wantExit: 0,
		},
		{
			name:     "repeated options",
			stdin:    strings.NewReader("aGVsbG8=\n"),
			args:     []string{"-d", "-d"},
			want:     "hello",
			wantExit: 0,
		},
		{
			name:     "ignore garbage is a no-op when encoding",
			stdin:    strings.NewReader("hello"),
			args:     []string{"-i"},
			want:     "aGVsbG8=\n",
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
			wantErr:  "base64: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing file when decoding",
			fsys:     testFS(),
			args:     []string{"-d", "missing.txt"},
			wantErr:  "base64: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a zero-length file name",
			fsys:     testFS(),
			args:     []string{""},
			wantErr:  "base64: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			args:     []string{"-w", "0"},
			wantErr:  "base64: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "no file system",
			args:     []string{"hello.txt"},
			wantErr:  "base64: hello.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "a read error on the standard input",
			stdin:    errorReader{},
			wantErr:  "base64: standard input: boom\n",
			wantExit: 1,
		},
		{
			name:     "a read error when decoding",
			stdin:    errorReader{},
			args:     []string{"-d"},
			wantErr:  "base64: standard input: boom\n",
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
			wantErr: "base64: write error: boom\n",
		},
		{
			name:    "decoding",
			stdin:   strings.NewReader("aGVsbG8="),
			args:    []string{"-d"},
			wantErr: "base64: write error: boom\n",
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
		runCancelled(t, ctx, strings.NewReader("hello"), nil)
	})

	t.Run("cancelled while reading", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runCancelled(t, ctx, &cancelReader{cancel: cancel}, nil)
	})
}

// runCancelled runs the utility with a context that is either already done or
// becomes done while reading, and checks that the context error is returned
// without any diagnostic.
func runCancelled(t *testing.T, ctx context.Context, stdin io.Reader, fsys fsx.FS) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsys,
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
		want := "base64 (charm.land/snail/coreutils) " + coreutils.Version +
			"\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
