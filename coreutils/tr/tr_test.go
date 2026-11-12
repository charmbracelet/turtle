package tr

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
)

func TestRunTranslate(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "characters are replaced by position",
			Stdin: strings.NewReader("abc"),
			Args:  []string{"ab", "ba"},
			Want:  "bac",
		},
		{
			Name:  "characters without a counterpart are kept",
			Stdin: strings.NewReader("abc"),
			Args:  []string{"a", "x"},
			Want:  "xbc",
		},
		{
			Name:  "a longer string2 is truncated",
			Stdin: strings.NewReader("a"),
			Args:  []string{"a", "xy"},
			Want:  "x",
		},
		{
			Name:  "a shorter string2 is padded with its last character",
			Stdin: strings.NewReader("57"),
			Args:  []string{"0123456789", "d"},
			Want:  "dd",
		},
		{
			Name:  "a repeated character maps to its last counterpart",
			Stdin: strings.NewReader("a"),
			Args:  []string{"aa", "bx"},
			Want:  "x",
		},
		{
			Name:  "ranges",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"a-z", "A-Z"},
			Want:  "HELLO",
		},
		{
			Name:  "an octal range",
			Stdin: strings.NewReader("ABCDE"),
			Args:  []string{`\101-\105`, "abcde"},
			Want:  "abcde",
		},
		{
			Name:  "escape sequences",
			Stdin: strings.NewReader("a\tb"),
			Args:  []string{`\t`, " "},
			Want:  "a b",
		},
		{
			Name:  "an octal sequence",
			Stdin: strings.NewReader("a\x00b"),
			Args:  []string{`\000`, "x"},
			Want:  "axb",
		},
		{
			Name:  "an octal replacement is a single byte",
			Stdin: strings.NewReader("abc"),
			Args:  []string{"a", `\351`},
			Want:  "\xe9bc",
		},
		{
			Name:  "a multibyte character",
			Stdin: strings.NewReader("café"),
			Args:  []string{"é", "x"},
			Want:  "cafx",
		},
		{
			Name:  "a multibyte replacement",
			Stdin: strings.NewReader("abc"),
			Args:  []string{"a", "é"},
			Want:  "ébc",
		},
		{
			Name:  "an invalid byte is translated by its value",
			Stdin: strings.NewReader("a\xe9b"),
			Args:  []string{`\351`, "x"},
			Want:  "axb",
		},
		{
			Name:  "an invalid byte is kept when unmatched",
			Stdin: strings.NewReader("a\xffb"),
			Args:  []string{"a", "x"},
			Want:  "x\xffb",
		},
		{
			Name:  "an invalid byte at the end of the input is final",
			Stdin: strings.NewReader("a\xff"),
			Args:  []string{`\377`, "y"},
			Want:  "ay",
		},
		{
			Name:  "brackets around the operands are ordinary characters",
			Stdin: strings.NewReader("[a]"),
			Args:  []string{"[a]", "[b]"},
			Want:  "[b]",
		},
		{
			Name:  "a single bracket is ordinary",
			Stdin: strings.NewReader("x["),
			Args:  []string{"[", "y"},
			Want:  "xy",
		},
		{
			Name:  "the end of options",
			Stdin: strings.NewReader("-a"),
			Args:  []string{"--", "-a", "b"},
			Want:  "bb",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunCaseConversion(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "lower to upper",
			Stdin: strings.NewReader("hello"),
			Args:  []string{"[:lower:]", "[:upper:]"},
			Want:  "HELLO",
		},
		{
			Name:  "lower to upper beyond ASCII",
			Stdin: strings.NewReader("héllo"),
			Args:  []string{"[:lower:]", "[:upper:]"},
			Want:  "HÉLLO",
		},
		{
			Name:  "upper to lower",
			Stdin: strings.NewReader("HÉLLO Wörld"),
			Args:  []string{"[:upper:]", "[:lower:]"},
			Want:  "héllo wörld",
		},
		{
			Name:  "classes in brackets",
			Stdin: strings.NewReader("[a]"),
			Args:  []string{"[[:lower:]]", "[[:upper:]]"},
			Want:  "[A]",
		},
		{
			Name:  "classes beside characters",
			Stdin: strings.NewReader("1a"),
			Args:  []string{"1[:lower:]", "2[:upper:]"},
			Want:  "2A",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunComplement(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "translation of the other values",
			Stdin: strings.NewReader("banana"),
			Args:  []string{"-c", "a", "x"},
			Want:  "xaxaxa",
		},
		{
			Name:  "deletion of the other values",
			Stdin: strings.NewReader("a1b2c3"),
			Args:  []string{"-dc", "0-9"},
			Want:  "123",
		},
		{
			Name:  "squeezing of the other characters",
			Stdin: strings.NewReader("aabcc"),
			Args:  []string{"-cs", "b"},
			Want:  "abc",
		},
		{
			Name:  "characters beyond the byte values are translated too",
			Stdin: strings.NewReader("a漢"),
			Args:  []string{"-c", "a", "x"},
			Want:  "ax",
		},
		{
			Name:  "characters of the set are kept beyond the byte values",
			Stdin: strings.NewReader("漢"),
			Args:  []string{"-c", "[:alpha:]", "x"},
			Want:  "漢",
		},
		{
			Name:  "the class of a case conversion is complemented as a whole",
			Stdin: strings.NewReader("aéā"),
			Args:  []string{"-c", "[:lower:]", "[:upper:]"},
			Want:  "aéā",
		},
		{
			Name:  "-C complements like -c",
			Stdin: strings.NewReader("banana"),
			Args:  []string{"-C", "a", "x"},
			Want:  "xaxaxa",
		},
		{
			Name:  "the long option complements like -c",
			Stdin: strings.NewReader("banana"),
			Args:  []string{"--complement", "a", "x"},
			Want:  "xaxaxa",
		},
		{
			Name:  "the POSIX word list example",
			Stdin: strings.NewReader("Hello, world!"),
			Args:  []string{"-cs", "[:alpha:]", `[\n*]`},
			Want:  "Hello\nworld\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunDelete(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the characters of string1",
			Stdin: strings.NewReader("banana"),
			Args:  []string{"-d", "aeiou"},
			Want:  "bnn",
		},
		{
			Name:  "a class",
			Stdin: strings.NewReader("a1b2"),
			Args:  []string{"-d", "[:digit:]"},
			Want:  "ab",
		},
		{
			Name:  "an equivalence class",
			Stdin: strings.NewReader("café"),
			Args:  []string{"-d", "[=e=]"},
			Want:  "café",
		},
		{
			Name:  "NUL characters",
			Stdin: strings.NewReader("a\x00b\x00"),
			Args:  []string{"-d", `\000`},
			Want:  "ab",
		},
		{
			Name:  "the long option deletes like -d",
			Stdin: strings.NewReader("banana"),
			Args:  []string{"--delete", "aeiou"},
			Want:  "bnn",
		},
		{
			Name:  "invalid bytes are kept when unmatched",
			Stdin: strings.NewReader("a\xffb"),
			Args:  []string{"-d", "a"},
			Want:  "\xffb",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunSqueeze(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "repetitions of the characters of string1",
			Stdin: strings.NewReader("aaabaa"),
			Args:  []string{"-s", "a"},
			Want:  "aba",
		},
		{
			Name:  "a class",
			Stdin: strings.NewReader("a  \tb"),
			Args:  []string{"-s", "[:space:]"},
			Want:  "a \tb",
		},
		{
			Name:  "squeezing after translating",
			Stdin: strings.NewReader("aaabbb"),
			Args:  []string{"-s", "ab", "ba"},
			Want:  "ba",
		},
		{
			Name:  "squeezing after a case conversion",
			Stdin: strings.NewReader("AAAbb"),
			Args:  []string{"-s", "[:upper:]", "[:lower:]"},
			Want:  "ab",
		},
		{
			Name:  "squeezing the characters of string2 after deleting",
			Stdin: strings.NewReader("aabbb"),
			Args:  []string{"-ds", "a", "b"},
			Want:  "b",
		},
		{
			Name:  "classes in string2 with deletion and squeezing",
			Stdin: strings.NewReader("a1b22 3"),
			Args:  []string{"-ds", "[:alpha:]", "[:digit:]"},
			Want:  "12 3",
		},
		{
			Name:  "repetitions across chunks",
			Stdin: strings.NewReader(strings.Repeat("a", 2*chunkSize+1) + "b"),
			Args:  []string{"-s", "a"},
			Want:  "ab",
		},
		{
			Name:  "a character split across chunks is translated",
			Stdin: strings.NewReader(strings.Repeat("a", chunkSize-1) + "éb"),
			Args:  []string{"é", "x"},
			Want:  strings.Repeat("a", chunkSize-1) + "xb",
		},
		{
			Name:  "the long option squeezes like -s",
			Stdin: strings.NewReader("aaabaa"),
			Args:  []string{"--squeeze-repeats", "a"},
			Want:  "aba",
		},
		{
			Name:  "a wide character split across chunks is translated",
			Stdin: strings.NewReader(strings.Repeat("a", chunkSize-2) + "😀b"),
			Args:  []string{"😀", "z"},
			Want:  strings.Repeat("a", chunkSize-2) + "zb",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunTruncate(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the extra characters of string1 are dropped",
			Stdin: strings.NewReader("abcdef"),
			Args:  []string{"-t", "ab", "xy"},
			Want:  "xycdef",
		},
		{
			Name:  "a string1 which is not longer is kept whole",
			Stdin: strings.NewReader("ab"),
			Args:  []string{"-t", "ab", "xyz"},
			Want:  "xy",
		},
		{
			Name:  "the complement is truncated after being expanded",
			Stdin: strings.NewReader("\x00\x01\x02a"),
			Args:  []string{"-ct", "a", "xy"},
			Want:  "xy\x02a",
		},
		{
			Name:  "characters beyond the byte values are kept",
			Stdin: strings.NewReader("a漢\x00"),
			Args:  []string{"-ct", "a", "x"},
			Want:  "a漢x",
		},
		{
			Name:  "squeezing after a truncated translation",
			Stdin: strings.NewReader("aabbc"),
			Args:  []string{"-ts", "ab", "xy"},
			Want:  "xyc",
		},
		{
			Name:  "ignored with deletion",
			Stdin: strings.NewReader("aabbcc"),
			Args:  []string{"-td", "ab"},
			Want:  "cc",
		},
		{
			Name:  "ignored with squeezing only",
			Stdin: strings.NewReader("aabbbc"),
			Args:  []string{"-ts", "b"},
			Want:  "aabc",
		},
		{
			Name:  "the long option truncates like -t",
			Stdin: strings.NewReader("abcdef"),
			Args:  []string{"--truncate-set1", "ab", "xy"},
			Want:  "xycdef",
		},
		{
			Name:  "-u is accepted and ignored",
			Stdin: strings.NewReader("abc"),
			Args:  []string{"-u", "ab", "ba"},
			Want:  "bac",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "an invalid option",
			Args:     []string{"-z", "a", "b"},
			WantErr:  "tr: invalid option -- 'z'\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an unknown long option",
			Args:     []string{"--foo", "a", "b"},
			WantErr:  "tr: unrecognized option '--foo'\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing operand",
			Args:     []string{},
			WantErr:  "tr: missing operand\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing string2",
			Args:     []string{"a"},
			WantErr:  "tr: missing operand after \"a\"\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an extra operand",
			Args:     []string{"a", "b", "c"},
			WantErr:  "tr: extra operand \"c\"\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an extra operand with deletion",
			Args:     []string{"-d", "a", "b"},
			WantErr:  "tr: extra operand \"b\"\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing string2 with deletion and squeezing",
			Args:     []string{"-ds", "a"},
			WantErr:  "tr: missing operand after \"a\"\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid character class",
			Args:     []string{"[:foo:]", "a"},
			WantErr:  "tr: invalid character class \"foo\"\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a reversed range",
			Args:     []string{"z-a", "a-z"},
			WantErr:  "tr: range-endpoints of \"z-a\" are in reverse collating sequence order\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a repetition in string1",
			Args:     []string{"[a*2]", "b"},
			WantErr:  "tr: repetitions can only be used in string2\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a class in string2 without a case conversion",
			Args:     []string{"a", "[:digit:]"},
			WantErr:  "tr: string2 classes must be lower or upper, matching the one in string1\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an equivalence class in string2",
			Args:     []string{"a", "[=e=]"},
			WantErr:  "tr: equivalence classes in string2 are only valid with -d and -s\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an empty string2",
			Args:     []string{"a", ""},
			WantErr:  "tr: when translating, string2 must be non-empty\nTry 'tr --help' for more information.\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunIOErrors(t *testing.T) {
	t.Run("a write error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader("a"),
			Stdout: testutil.ErrorWriter{Err: errors.New("boom")},
			Stderr: new(bytes.Buffer),
		}
		err := New(rt).Run(t.Context(), "a", "b")
		if code := testutil.ExitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
	})

	t.Run("a read error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  testutil.ErrorReader{Err: errors.New("boom")},
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
		}
		err := New(rt).Run(t.Context(), "a", "b")
		if code := testutil.ExitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
	})

	t.Run("a done context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		rt := &coreutils.Runtime{
			Stdin:  &testutil.CancelReader{Cancel: cancel},
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
		if code := testutil.ExitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
		if got, want := stderr.String(), "tr: no standard input available\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "tr", usage)
}
