package tr

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
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
	name     string
	stdin    io.Reader
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
		Stdin:  test.stdin,
		Stdout: stdout,
		Stderr: stderr,
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
}

func TestRunTranslate(t *testing.T) {
	tests := []testCase{
		{
			name:  "characters are replaced by position",
			stdin: strings.NewReader("abc"),
			args:  []string{"ab", "ba"},
			want:  "bac",
		},
		{
			name:  "characters without a counterpart are kept",
			stdin: strings.NewReader("abc"),
			args:  []string{"a", "x"},
			want:  "xbc",
		},
		{
			name:  "a longer string2 is truncated",
			stdin: strings.NewReader("a"),
			args:  []string{"a", "xy"},
			want:  "x",
		},
		{
			name:  "a shorter string2 is padded with its last character",
			stdin: strings.NewReader("57"),
			args:  []string{"0123456789", "d"},
			want:  "dd",
		},
		{
			name:  "a repeated character maps to its last counterpart",
			stdin: strings.NewReader("a"),
			args:  []string{"aa", "bx"},
			want:  "x",
		},
		{
			name:  "ranges",
			stdin: strings.NewReader("hello"),
			args:  []string{"a-z", "A-Z"},
			want:  "HELLO",
		},
		{
			name:  "an octal range",
			stdin: strings.NewReader("ABCDE"),
			args:  []string{`\101-\105`, "abcde"},
			want:  "abcde",
		},
		{
			name:  "escape sequences",
			stdin: strings.NewReader("a\tb"),
			args:  []string{`\t`, " "},
			want:  "a b",
		},
		{
			name:  "an octal sequence",
			stdin: strings.NewReader("a\x00b"),
			args:  []string{`\000`, "x"},
			want:  "axb",
		},
		{
			name:  "an octal replacement is a single byte",
			stdin: strings.NewReader("abc"),
			args:  []string{"a", `\351`},
			want:  "\xe9bc",
		},
		{
			name:  "a multibyte character",
			stdin: strings.NewReader("café"),
			args:  []string{"é", "x"},
			want:  "cafx",
		},
		{
			name:  "a multibyte replacement",
			stdin: strings.NewReader("abc"),
			args:  []string{"a", "é"},
			want:  "ébc",
		},
		{
			name:  "an invalid byte is translated by its value",
			stdin: strings.NewReader("a\xe9b"),
			args:  []string{`\351`, "x"},
			want:  "axb",
		},
		{
			name:  "an invalid byte is kept when unmatched",
			stdin: strings.NewReader("a\xffb"),
			args:  []string{"a", "x"},
			want:  "x\xffb",
		},
		{
			name:  "an invalid byte at the end of the input is final",
			stdin: strings.NewReader("a\xff"),
			args:  []string{`\377`, "y"},
			want:  "ay",
		},
		{
			name:  "brackets around the operands are ordinary characters",
			stdin: strings.NewReader("[a]"),
			args:  []string{"[a]", "[b]"},
			want:  "[b]",
		},
		{
			name:  "a single bracket is ordinary",
			stdin: strings.NewReader("x["),
			args:  []string{"[", "y"},
			want:  "xy",
		},
		{
			name:  "the end of options",
			stdin: strings.NewReader("-a"),
			args:  []string{"--", "-a", "b"},
			want:  "bb",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunCaseConversion(t *testing.T) {
	tests := []testCase{
		{
			name:  "lower to upper",
			stdin: strings.NewReader("hello"),
			args:  []string{"[:lower:]", "[:upper:]"},
			want:  "HELLO",
		},
		{
			name:  "lower to upper beyond ASCII",
			stdin: strings.NewReader("héllo"),
			args:  []string{"[:lower:]", "[:upper:]"},
			want:  "HÉLLO",
		},
		{
			name:  "upper to lower",
			stdin: strings.NewReader("HÉLLO Wörld"),
			args:  []string{"[:upper:]", "[:lower:]"},
			want:  "héllo wörld",
		},
		{
			name:  "classes in brackets",
			stdin: strings.NewReader("[a]"),
			args:  []string{"[[:lower:]]", "[[:upper:]]"},
			want:  "[A]",
		},
		{
			name:  "classes beside characters",
			stdin: strings.NewReader("1a"),
			args:  []string{"1[:lower:]", "2[:upper:]"},
			want:  "2A",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunComplement(t *testing.T) {
	tests := []testCase{
		{
			name:  "translation of the other values",
			stdin: strings.NewReader("banana"),
			args:  []string{"-c", "a", "x"},
			want:  "xaxaxa",
		},
		{
			name:  "deletion of the other values",
			stdin: strings.NewReader("a1b2c3"),
			args:  []string{"-dc", "0-9"},
			want:  "123",
		},
		{
			name:  "squeezing of the other characters",
			stdin: strings.NewReader("aabcc"),
			args:  []string{"-cs", "b"},
			want:  "abc",
		},
		{
			name:  "characters beyond the byte values are translated too",
			stdin: strings.NewReader("a漢"),
			args:  []string{"-c", "a", "x"},
			want:  "ax",
		},
		{
			name:  "characters of the set are kept beyond the byte values",
			stdin: strings.NewReader("漢"),
			args:  []string{"-c", "[:alpha:]", "x"},
			want:  "漢",
		},
		{
			name:  "the class of a case conversion is complemented as a whole",
			stdin: strings.NewReader("aéā"),
			args:  []string{"-c", "[:lower:]", "[:upper:]"},
			want:  "aéā",
		},
		{
			name:  "-C complements like -c",
			stdin: strings.NewReader("banana"),
			args:  []string{"-C", "a", "x"},
			want:  "xaxaxa",
		},
		{
			name:  "the POSIX word list example",
			stdin: strings.NewReader("Hello, world!"),
			args:  []string{"-cs", "[:alpha:]", `[\n*]`},
			want:  "Hello\nworld\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDelete(t *testing.T) {
	tests := []testCase{
		{
			name:  "the characters of string1",
			stdin: strings.NewReader("banana"),
			args:  []string{"-d", "aeiou"},
			want:  "bnn",
		},
		{
			name:  "a class",
			stdin: strings.NewReader("a1b2"),
			args:  []string{"-d", "[:digit:]"},
			want:  "ab",
		},
		{
			name:  "an equivalence class",
			stdin: strings.NewReader("café"),
			args:  []string{"-d", "[=e=]"},
			want:  "café",
		},
		{
			name:  "NUL characters",
			stdin: strings.NewReader("a\x00b\x00"),
			args:  []string{"-d", `\000`},
			want:  "ab",
		},
		{
			name:  "invalid bytes are kept when unmatched",
			stdin: strings.NewReader("a\xffb"),
			args:  []string{"-d", "a"},
			want:  "\xffb",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSqueeze(t *testing.T) {
	tests := []testCase{
		{
			name:  "repetitions of the characters of string1",
			stdin: strings.NewReader("aaabaa"),
			args:  []string{"-s", "a"},
			want:  "aba",
		},
		{
			name:  "a class",
			stdin: strings.NewReader("a  \tb"),
			args:  []string{"-s", "[:space:]"},
			want:  "a \tb",
		},
		{
			name:  "squeezing after translating",
			stdin: strings.NewReader("aaabbb"),
			args:  []string{"-s", "ab", "ba"},
			want:  "ba",
		},
		{
			name:  "squeezing after a case conversion",
			stdin: strings.NewReader("AAAbb"),
			args:  []string{"-s", "[:upper:]", "[:lower:]"},
			want:  "ab",
		},
		{
			name:  "squeezing the characters of string2 after deleting",
			stdin: strings.NewReader("aabbb"),
			args:  []string{"-ds", "a", "b"},
			want:  "b",
		},
		{
			name:  "classes in string2 with deletion and squeezing",
			stdin: strings.NewReader("a1b22 3"),
			args:  []string{"-ds", "[:alpha:]", "[:digit:]"},
			want:  "12 3",
		},
		{
			name:  "repetitions across chunks",
			stdin: strings.NewReader(strings.Repeat("a", 2*chunkSize+1) + "b"),
			args:  []string{"-s", "a"},
			want:  "ab",
		},
		{
			name:  "a character split across chunks is translated",
			stdin: strings.NewReader(strings.Repeat("a", chunkSize-1) + "éb"),
			args:  []string{"é", "x"},
			want:  strings.Repeat("a", chunkSize-1) + "xb",
		},
		{
			name:  "a wide character split across chunks is translated",
			stdin: strings.NewReader(strings.Repeat("a", chunkSize-2) + "😀b"),
			args:  []string{"😀", "z"},
			want:  strings.Repeat("a", chunkSize-2) + "zb",
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
			name:     "an invalid option",
			args:     []string{"-z", "a", "b"},
			wantErr:  "tr: invalid option -- 'z'\n",
			wantExit: 1,
		},
		{
			name:     "a missing operand",
			args:     []string{},
			wantErr:  "tr: missing operand\n",
			wantExit: 1,
		},
		{
			name:     "a missing string2",
			args:     []string{"a"},
			wantErr:  "tr: missing operand after \"a\"\n",
			wantExit: 1,
		},
		{
			name:     "an extra operand",
			args:     []string{"a", "b", "c"},
			wantErr:  "tr: extra operand \"c\"\n",
			wantExit: 1,
		},
		{
			name:     "an extra operand with deletion",
			args:     []string{"-d", "a", "b"},
			wantErr:  "tr: extra operand \"b\"\n",
			wantExit: 1,
		},
		{
			name:     "a missing string2 with deletion and squeezing",
			args:     []string{"-ds", "a"},
			wantErr:  "tr: missing operand after \"a\"\n",
			wantExit: 1,
		},
		{
			name:     "an invalid character class",
			args:     []string{"[:foo:]", "a"},
			wantErr:  "tr: invalid character class \"foo\"\n",
			wantExit: 1,
		},
		{
			name:     "a reversed range",
			args:     []string{"z-a", "a-z"},
			wantErr:  "tr: range-endpoints of \"z-a\" are in reverse collating sequence order\n",
			wantExit: 1,
		},
		{
			name:     "a repetition in string1",
			args:     []string{"[a*2]", "b"},
			wantErr:  "tr: repetitions can only be used in string2\n",
			wantExit: 1,
		},
		{
			name:     "a class in string2 without a case conversion",
			args:     []string{"a", "[:digit:]"},
			wantErr:  "tr: string2 classes must be lower or upper, matching the one in string1\n",
			wantExit: 1,
		},
		{
			name:     "an equivalence class in string2",
			args:     []string{"a", "[=e=]"},
			wantErr:  "tr: equivalence classes in string2 are only valid with -d and -s\n",
			wantExit: 1,
		},
		{
			name:     "an empty string2",
			args:     []string{"a", ""},
			wantErr:  "tr: when translating, string2 must be non-empty\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunIOErrors(t *testing.T) {
	t.Run("a write error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader("a"),
			Stdout: errorWriter{err: errors.New("boom")},
			Stderr: new(bytes.Buffer),
		}
		err := New(rt).Run(t.Context(), "a", "b")
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
	})

	t.Run("a read error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  errorReader{err: errors.New("boom")},
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
		}
		err := New(rt).Run(t.Context(), "a", "b")
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
	})

	t.Run("a done context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		rt := &coreutils.Runtime{
			Stdin:  &cancelReader{cancel: cancel},
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
		}
		err := New(rt).Run(ctx, "a", "b")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})

	t.Run("no standard input", func(t *testing.T) {
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdout: new(bytes.Buffer),
			Stderr: stderr,
		}
		err := New(rt).Run(t.Context(), "a", "b")
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
		if got, want := stderr.String(), "tr: no standard input available\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})
}
