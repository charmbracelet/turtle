package wc

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// testFS holds the fixtures used by the tests:
//
//	file.txt:    "hello world\nfoo bar\n"  (2 lines, 4 words, 20 bytes, 11 longest)
//	one.txt:     "a b\nc"                  (1 line, 3 words, 5 bytes, 3 longest)
//	two.txt:     "one\ntwo\n"              (2 lines, 2 words, 8 bytes, 3 longest)
//	empty.txt:   ""                        (0 lines, 0 words, 0 bytes, 0 longest)
//	spaces.txt:  "   \t\n"                 (1 line, 0 words, 5 bytes, 8 longest)
//	utf8.txt:    "héllo wörld\n"           (1 line, 2 words, 14 bytes, 12 chars, 9 longest)
//	invalid.txt: "\xffa\xfe é\n"           (1 line, 2 words, 7 bytes, 6 chars, 2 longest)
//	"-l":        "x\n"                     (1 line, 1 word, 2 bytes, 1 longest)
//	sub/inner.txt: "inner\n"               (1 line, 1 word, 6 bytes, 5 longest)
func testFS() fsx.MemoryFS {
	return fsx.MemoryFS{
		"file.txt":      {Data: []byte("hello world\nfoo bar\n")},
		"one.txt":       {Data: []byte("a b\nc")},
		"two.txt":       {Data: []byte("one\ntwo\n")},
		"empty.txt":     {Data: []byte("")},
		"spaces.txt":    {Data: []byte("   \t\n")},
		"utf8.txt":      {Data: []byte("héllo wörld\n")},
		"invalid.txt":   {Data: []byte{0xff, 'a', 0xfe, ' ', 0xc3, 0xa9, '\n'}},
		"-l":            {Data: []byte("x\n")},
		"sub/inner.txt": {Data: []byte("inner\n")},
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no operands reads the standard input",
			Stdin: strings.NewReader("a b\nc"),
			Want:  "1 3 5\n",
		},
		{
			Name:  "empty standard input",
			Stdin: strings.NewReader(""),
			Want:  "0 0 0\n",
		},
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("a b\nc"),
			Args:  []string{"-"},
			Want:  "1 3 5 -\n",
		},
		{
			Name:  "dash combined with a file",
			Stdin: strings.NewReader("a b\nc"),
			FS:    testFS(),
			Args:  []string{"-", "one.txt"},
			Want:  "1 3 5 -\n1 3 5 one.txt\n2 6 10 total\n",
		},
		{
			Name:  "double dash with no operands reads the standard input",
			Stdin: strings.NewReader("a b\nc"),
			Args:  []string{"--"},
			Want:  "1 3 5\n",
		},
		{
			Name:  "repeated standard input operands",
			Stdin: strings.NewReader("a b\nc"),
			Args:  []string{"-", "-"},
			Want:  "1 3 5 -\n0 0 0 -\n1 3 5 total\n",
		},
		{
			Name: "single file",
			FS:   testFS(),
			Args: []string{"file.txt"},
			Want: "2 4 20 file.txt\n",
		},
		{
			Name: "empty file",
			FS:   testFS(),
			Args: []string{"empty.txt"},
			Want: "0 0 0 empty.txt\n",
		},
		{
			Name: "multiple files write a total",
			FS:   testFS(),
			Args: []string{"file.txt", "one.txt"},
			Want: "2 4 20 file.txt\n1 3 5 one.txt\n3 7 25 total\n",
		},
		{
			Name: "the same file twice",
			FS:   testFS(),
			Args: []string{"file.txt", "file.txt"},
			Want: "2 4 20 file.txt\n2 4 20 file.txt\n4 8 40 total\n",
		},
		{
			Name: "the pathname is echoed as given",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"inner.txt"},
			Want: "1 1 6 inner.txt\n",
		},
		{
			Name: "a file named like an option after a double dash",
			FS:   testFS(),
			Args: []string{"--", "-l"},
			Want: "1 1 2 -l\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "c writes only bytes",
			FS:   testFS(),
			Args: []string{"-c", "file.txt"},
			Want: "20 file.txt\n",
		},
		{
			Name: "l writes only newlines",
			FS:   testFS(),
			Args: []string{"-l", "file.txt"},
			Want: "2 file.txt\n",
		},
		{
			Name: "w writes only words",
			FS:   testFS(),
			Args: []string{"-w", "file.txt"},
			Want: "4 file.txt\n",
		},
		{
			Name: "m writes only characters",
			FS:   testFS(),
			Args: []string{"-m", "utf8.txt"},
			Want: "12 utf8.txt\n",
		},
		{
			Name: "grouped options",
			FS:   testFS(),
			Args: []string{"-lw", "file.txt"},
			Want: "2 4 file.txt\n",
		},
		{
			Name: "grouped options in any order",
			FS:   testFS(),
			Args: []string{"-wl", "file.txt"},
			Want: "2 4 file.txt\n",
		},
		{
			Name: "repeated options",
			FS:   testFS(),
			Args: []string{"-l", "-l", "file.txt"},
			Want: "2 file.txt\n",
		},
		{
			Name: "l and c",
			FS:   testFS(),
			Args: []string{"-lc", "file.txt"},
			Want: "2 20 file.txt\n",
		},
		{
			Name: "l, w and c",
			FS:   testFS(),
			Args: []string{"-lwc", "file.txt"},
			Want: "2 4 20 file.txt\n",
		},
		{
			Name: "full output with characters",
			FS:   testFS(),
			Args: []string{"-mlw", "utf8.txt"},
			Want: "1 2 12 utf8.txt\n",
		},
		{
			Name: "c and m together write both counts",
			FS:   testFS(),
			Args: []string{"-cm", "utf8.txt"},
			Want: "12 14 utf8.txt\n",
		},
		{
			Name: "c and m together in any order",
			FS:   testFS(),
			Args: []string{"-mc", "utf8.txt"},
			Want: "12 14 utf8.txt\n",
		},
		{
			Name: "options are not parsed after the first operand",
			FS:   testFS(),
			Args: []string{"file.txt", "-l"},
			Want: "2 4 20 file.txt\n1 1 2 -l\n3 5 22 total\n",
		},
		{
			Name: "options apply to every file and the total",
			FS:   testFS(),
			Args: []string{"-l", "file.txt", "one.txt"},
			Want: "2 file.txt\n1 one.txt\n3 total\n",
		},
		{
			Name: "a lone double dash is accepted",
			FS:   testFS(),
			Args: []string{"--", "file.txt"},
			Want: "2 4 20 file.txt\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunWords(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		want  string
	}{
		{
			name:  "leading and trailing white space",
			stdin: "  leading and trailing  ",
			want:  "0 3 24\n",
		},
		{
			name:  "every ASCII white space character separates words",
			stdin: "a\tb\vc\fd\re\nf",
			want:  "1 6 11\n",
		},
		{
			name:  "only white space",
			stdin: "   ",
			want:  "0 0 3\n",
		},
		{
			name:  "no trailing newline",
			stdin: "no-trailing-newline",
			want:  "0 1 19\n",
		},
		{
			name:  "consecutive newlines",
			stdin: "a b\nc\n\n",
			want:  "3 3 7\n",
		},
		{
			name:  "invalid UTF-8 bytes are counted as words",
			stdin: string([]byte{0xff, 0xfe}),
			want:  "0 1 2\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testutil.Once(t, New, testutil.Case{Stdin: strings.NewReader(test.stdin), Want: test.want})
		})
	}
}

func TestRunChars(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "multibyte characters",
			FS:   testFS(),
			Args: []string{"-ml", "utf8.txt"},
			Want: "1 12 utf8.txt\n",
		},
		{
			Name: "invalid bytes count as one character each",
			FS:   testFS(),
			Args: []string{"-m", "invalid.txt"},
			Want: "6 invalid.txt\n",
		},
		{
			Name: "invalid bytes are counted in bytes mode",
			FS:   testFS(),
			Args: []string{"invalid.txt"},
			Want: "1 2 7 invalid.txt\n",
		},
		{
			Name: "words are also counted in characters mode",
			FS:   testFS(),
			Args: []string{"-mw", "invalid.txt"},
			Want: "2 6 invalid.txt\n",
		},
		{
			Name:  "characters from the standard input",
			Stdin: strings.NewReader("héllo wörld\n"),
			Args:  []string{"-m"},
			Want:  "12\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "lines counts the lines",
			FS:   testFS(),
			Args: []string{"--lines", "file.txt"},
			Want: "2 file.txt\n",
		},
		{
			Name: "words counts the words",
			FS:   testFS(),
			Args: []string{"--words", "file.txt"},
			Want: "4 file.txt\n",
		},
		{
			Name: "bytes counts the bytes",
			FS:   testFS(),
			Args: []string{"--bytes", "file.txt"},
			Want: "20 file.txt\n",
		},
		{
			Name: "chars counts the characters",
			FS:   testFS(),
			Args: []string{"--chars", "utf8.txt"},
			Want: "12 utf8.txt\n",
		},
		{
			Name: "max-line-length counts the longest line",
			FS:   testFS(),
			Args: []string{"--max-line-length", "file.txt"},
			Want: "11 file.txt\n",
		},
		{
			Name:     "a long option with a value it doesn't take is an error",
			FS:       testFS(),
			Args:     []string{"--lines=2", "file.txt"},
			WantErr:  "wc: option '--lines=2' doesn't allow an argument\nTry 'wc --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunMaxLineLength(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the longest line",
			FS:   testFS(),
			Args: []string{"-L", "file.txt"},
			Want: "11 file.txt\n",
		},
		{
			Name:  "empty input",
			Stdin: strings.NewReader(""),
			Args:  []string{"-L"},
			Want:  "0\n",
		},
		{
			Name:  "a trailing line without a newline is counted",
			Stdin: strings.NewReader("a\nlonger"),
			Args:  []string{"-L"},
			Want:  "6\n",
		},
		{
			Name:  "a tab advances to the next 8th column",
			Stdin: strings.NewReader("a\tb\n"),
			Args:  []string{"-L"},
			Want:  "9\n",
		},
		{
			Name:  "a tab at the end of a line rounds up to a multiple of 8",
			Stdin: strings.NewReader("a\t\n"),
			Args:  []string{"-L"},
			Want:  "8\n",
		},
		{
			Name:  "a carriage return ends a line",
			Stdin: strings.NewReader("ab\rcd\n"),
			Args:  []string{"-L"},
			Want:  "2\n",
		},
		{
			Name:  "a form feed ends a line",
			Stdin: strings.NewReader("ab\fcd\n"),
			Args:  []string{"-L"},
			Want:  "2\n",
		},
		{
			Name:  "a vertical tab counts no columns",
			Stdin: strings.NewReader("a\vb\n"),
			Args:  []string{"-L"},
			Want:  "2\n",
		},
		{
			Name:  "control bytes count no columns",
			Stdin: strings.NewReader("\x01ab\x7f\n"),
			Args:  []string{"-L"},
			Want:  "2\n",
		},
		{
			Name: "multibyte characters count no columns",
			FS:   testFS(),
			Args: []string{"-L", "utf8.txt"},
			Want: "9 utf8.txt\n",
		},
		{
			Name: "characters mode keeps the byte columns",
			FS:   testFS(),
			Args: []string{"-mL", "utf8.txt"},
			Want: "12 9 utf8.txt\n",
		},
		{
			Name: "invalid bytes count no columns",
			FS:   testFS(),
			Args: []string{"-L", "invalid.txt"},
			Want: "2 invalid.txt\n",
		},
		{
			Name: "the longest line is the fourth of the five columns",
			FS:   testFS(),
			Args: []string{"-lwmL", "utf8.txt"},
			Want: "1 2 12 9 utf8.txt\n",
		},
		{
			Name: "all counts in the manual order",
			FS:   testFS(),
			Args: []string{"-lcwmL", "utf8.txt"},
			Want: "1 2 12 14 9 utf8.txt\n",
		},
		{
			Name: "the total is the maximum, not the sum",
			FS:   testFS(),
			Args: []string{"-L", "file.txt", "one.txt"},
			Want: "11 file.txt\n3 one.txt\n11 total\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunTotal(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "auto is the default",
			FS:   testFS(),
			Args: []string{"--total=auto", "file.txt", "one.txt"},
			Want: "2 4 20 file.txt\n1 3 5 one.txt\n3 7 25 total\n",
		},
		{
			Name: "auto with a single file writes no total",
			FS:   testFS(),
			Args: []string{"--total=auto", "file.txt"},
			Want: "2 4 20 file.txt\n",
		},
		{
			Name: "always writes a total for a single file",
			FS:   testFS(),
			Args: []string{"--total=always", "file.txt"},
			Want: "2 4 20 file.txt\n2 4 20 total\n",
		},
		{
			Name:  "always writes a total for the standard input",
			Stdin: strings.NewReader("a b\nc"),
			Args:  []string{"--total=always"},
			Want:  "1 3 5\n1 3 5 total\n",
		},
		{
			Name: "only writes just the cumulative counts",
			FS:   testFS(),
			Args: []string{"--total=only", "file.txt", "one.txt"},
			Want: "3 7 25\n",
		},
		{
			Name: "only with a single file writes its counts",
			FS:   testFS(),
			Args: []string{"--total=only", "file.txt"},
			Want: "2 4 20\n",
		},
		{
			Name:  "only with the standard input",
			Stdin: strings.NewReader("a b\nc"),
			Args:  []string{"--total=only"},
			Want:  "1 3 5\n",
		},
		{
			Name: "never writes no total",
			FS:   testFS(),
			Args: []string{"--total=never", "file.txt", "one.txt"},
			Want: "2 4 20 file.txt\n1 3 5 one.txt\n",
		},
		{
			Name: "the word may be held by the next argument",
			FS:   testFS(),
			Args: []string{"--total", "never", "file.txt", "one.txt"},
			Want: "2 4 20 file.txt\n1 3 5 one.txt\n",
		},
		{
			Name: "the total keeps the selected counts",
			FS:   testFS(),
			Args: []string{"-L", "--total=always", "file.txt"},
			Want: "11 file.txt\n11 total\n",
		},
		{
			Name:     "an invalid word is an error",
			Args:     []string{"--total=bogus"},
			WantErr:  "wc: invalid argument 'bogus' for '--total'\nTry 'wc --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a missing word is an error",
			Args:     []string{"--total"},
			WantErr:  "wc: option '--total' requires an argument\nTry 'wc --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "missing file",
			FS:       testFS(),
			Args:     []string{"missing.txt"},
			WantErr:  "wc: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file doesn't stop the other files",
			FS:       testFS(),
			Args:     []string{"missing.txt", "file.txt"},
			Want:     "2 4 20 file.txt\n2 4 20 total\n",
			WantErr:  "wc: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:    "every missing file still writes a total",
			FS:      testFS(),
			Args:    []string{"missing.txt", "other.txt"},
			Want:    "0 0 0 total\n",
			WantErr: "wc: missing.txt: file does not exist\nwc: other.txt: file does not exist\n",
			// The total is written even when no file could be read.
			WantExit: 1,
		},
		{
			Name:     "invalid option",
			Args:     []string{"-x"},
			WantErr:  "wc: invalid option -- 'x'\nTry 'wc --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option in a group",
			Args:     []string{"-lx"},
			WantErr:  "wc: invalid option -- 'x'\nTry 'wc --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unknown long option",
			Args:     []string{"--bogus"},
			WantErr:  "wc: unrecognized option '--bogus'\nTry 'wc --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option before operands",
			Args:     []string{"-x", "file.txt"},
			WantErr:  "wc: invalid option -- 'x'\nTry 'wc --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "zero-length file name",
			FS:       testFS(),
			Args:     []string{""},
			WantErr:  "wc: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "file operand without a file system",
			Args:     []string{"file.txt"},
			WantErr:  "wc: file.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			FS:       testFS(),
			WantErr:  "wc: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "standard input read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			FS:       testFS(),
			WantErr:  "wc: standard input: read failed\n",
			WantExit: 1,
		},
		{
			Name:     "dash read error is labeled with the operand",
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			Args:     []string{"-"},
			WantErr:  "wc: -: read failed\n",
			WantExit: 1,
		},
		{
			Name:     "file read error",
			Stdin:    strings.NewReader(""),
			Args:     []string{"dir"},
			FS:       fsx.MemoryFS{"dir": {Mode: fs.ModeDir}},
			WantErr:  "wc: dir: invalid argument\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunDoesNotReadStdinWithFiles(t *testing.T) {
	stdin := &testutil.SpyReader{}

	testutil.Once(t, New, testutil.Case{
		Name:  "file operands don't read the standard input",
		Stdin: stdin,
		FS:    testFS(),
		Args:  []string{"file.txt"},
		Want:  "2 4 20 file.txt\n",
	})
	if stdin.Reads > 0 {
		t.Error("Run() read stdin, want it untouched")
	}
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("a b\nc"),
		Stdout: testutil.ErrorWriter{Err: writeErr},
		Stderr: new(bytes.Buffer),
	}
	err := New(rt).Run(t.Context())

	if code := testutil.ExitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		testutil.Cancelled(t, New, strings.NewReader("a b\nc"), nil)
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		_, cancel := context.WithCancel(t.Context())
		testutil.Cancelled(t, New, &testutil.CancelReader{Cancel: cancel, Data: []byte("a b\nc")}, nil)
	})

	t.Run("cancelled while reading a file", func(t *testing.T) {
		_, cancel := context.WithCancel(t.Context())
		testutil.Cancelled(t, New, strings.NewReader(""), testutil.ReaderFS(&testutil.CancelReader{Cancel: cancel, Data: []byte("a b\nc")}), "file")
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "wc", usage)
}
