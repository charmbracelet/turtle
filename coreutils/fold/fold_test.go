package fold

import (
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

// testFS holds the fixtures used by the tests:
//
//	long.txt:      90 "a" characters
//	tabs.txt:      "ab\tcd\n"
//	cr.txt:        "abc\rdef\n"
//	bs.txt:        "abc\b\b\bdefgh\n"
//	utf8.txt:      "áéç\n"
//	invalid.txt:   "a\xffbc\n" (an invalid UTF-8 byte)
//	nonl.txt:      "abcdef" (no trailing newline)
//	empty.txt:     ""
//	"-s":          "aa bb\n" (a file named like an option)
//	sub/inner.txt: "abcdefghi\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"long.txt":      {Data: []byte(strings.Repeat("a", 90) + "\n")},
		"tabs.txt":      {Data: []byte("ab\tcd\n")},
		"cr.txt":        {Data: []byte("abc\rdef\n")},
		"bs.txt":        {Data: []byte("abc\b\b\bdefgh\n")},
		"utf8.txt":      {Data: []byte("áéç\n")},
		"invalid.txt":   {Data: []byte{'a', 0xff, 'b', 'c', '\n'}},
		"nonl.txt":      {Data: []byte("abcdef")},
		"empty.txt":     {Data: []byte("")},
		"-s":            {Data: []byte("aa bb\n")},
		"sub/inner.txt": {Data: []byte("abcdefghi\n")},
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no operands read the standard input",
			Stdin: strings.NewReader("hello\n"),
			Want:  "hello\n",
		},
		{
			Name:  "an empty standard input",
			Stdin: strings.NewReader(""),
			Want:  "",
		},
		{
			Name:  "an empty line stays empty",
			Stdin: strings.NewReader("\n"),
			Want:  "\n",
		},
		{
			Name:  "short lines are unchanged",
			Stdin: strings.NewReader("ab\ncd\n"),
			Want:  "ab\ncd\n",
		},
		{
			Name:  "the default width is eighty",
			Stdin: strings.NewReader(strings.Repeat("a", 90) + "\n"),
			Want:  strings.Repeat("a", 80) + "\n" + strings.Repeat("a", 10) + "\n",
		},
		{
			Name:  "a line of the exact width is not broken",
			Stdin: strings.NewReader(strings.Repeat("a", 80) + "\n"),
			Want:  strings.Repeat("a", 80) + "\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunWidth(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "lines are broken at the width",
			Stdin: strings.NewReader("abcdefgh\n"),
			Args:  []string{"-w", "5"},
			Want:  "abcde\nfgh\n",
		},
		{
			Name:  "a line of the exact width is not broken",
			Stdin: strings.NewReader("abcde\n"),
			Args:  []string{"-w", "5"},
			Want:  "abcde\n",
		},
		{
			Name:  "a width of one breaks after every character",
			Stdin: strings.NewReader("abc\n"),
			Args:  []string{"-w", "1"},
			Want:  "a\nb\nc\n",
		},
		{
			Name:  "the last width wins",
			Stdin: strings.NewReader("abcdef\n"),
			Args:  []string{"-w", "5", "-w", "3"},
			Want:  "abc\ndef\n",
		},
		{
			Name:  "an attached width",
			Stdin: strings.NewReader("abcdefgh\n"),
			Args:  []string{"-w5"},
			Want:  "abcde\nfgh\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunBytes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "bytes count as columns",
			FS:   testFS(),
			Args: []string{"-b", "-w", "2", "utf8.txt"},
			Want: "á\né\nç\n",
		},
		{
			Name:  "a wide character is never split",
			Stdin: strings.NewReader("á\n"),
			Args:  []string{"-b", "-w", "1"},
			Want:  "á\n",
		},
		{
			Name: "tabs count one byte",
			FS:   testFS(),
			Args: []string{"-b", "-w", "3", "tabs.txt"},
			Want: "ab\t\ncd\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunColumns(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "tabs advance to the next multiple of eight",
			FS:   testFS(),
			Args: []string{"-w", "4", "tabs.txt"},
			Want: "ab\n\t\ncd\n",
		},
		{
			Name: "backspaces decrement the column",
			FS:   testFS(),
			Args: []string{"-w", "5", "bs.txt"},
			Want: "abc\b\b\bdefgh\n",
		},
		{
			Name: "a carriage return resets the column",
			FS:   testFS(),
			Args: []string{"-w", "5", "cr.txt"},
			Want: "abc\rdef\n",
		},
		{
			Name:  "the column is not decremented below zero",
			Stdin: strings.NewReader("\b\babc\n"),
			Args:  []string{"-w", "5"},
			Want:  "\b\babc\n",
		},
		{
			Name: "a multibyte character counts as one column",
			FS:   testFS(),
			Args: []string{"-w", "2", "utf8.txt"},
			Want: "áé\nç\n",
		},
		{
			Name: "an invalid byte counts as one column",
			FS:   testFS(),
			Args: []string{"-w", "3", "invalid.txt"},
			Want: "a\xffb\nc\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunSpaces(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the line breaks after the last blank",
			Stdin: strings.NewReader("aaa bbbb\n"),
			Args:  []string{"-w", "4", "-s"},
			Want:  "aaa \nbbbb\n",
		},
		{
			Name:  "the break happens after the last blank of the segment",
			Stdin: strings.NewReader("aa bb cc\n"),
			Args:  []string{"-w", "4", "-s"},
			Want:  "aa \nbb \ncc\n",
		},
		{
			Name:  "a segment without blanks breaks at the width",
			Stdin: strings.NewReader("aaaaa\n"),
			Args:  []string{"-w", "3", "-s"},
			Want:  "aaa\naa\n",
		},
		{
			Name:  "tabs are blanks for the break",
			Stdin: strings.NewReader("aa\tbb\n"),
			Args:  []string{"-w", "8", "-s"},
			Want:  "aa\t\nbb\n",
		},
		{
			Name:  "a blank at the start of the segment",
			Stdin: strings.NewReader(" abc\n"),
			Args:  []string{"-w", "2", "-s"},
			Want:  " \nab\nc\n",
		},
		{
			Name:  "blanks are counted in bytes with b",
			Stdin: strings.NewReader("á b\n"),
			Args:  []string{"-b", "-w", "3", "-s"},
			Want:  "á \nb\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunFiles(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"long.txt"},
			Want: strings.Repeat("a", 80) + "\n" + strings.Repeat("a", 10) + "\n",
		},
		{
			Name: "multiple files are concatenated in order",
			FS:   testFS(),
			Args: []string{"-w", "3", "tabs.txt", "cr.txt"},
			Want: "ab\n\t\ncd\nabc\rdef\n",
		},
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("abcdefgh\n"),
			FS:    testFS(),
			Args:  []string{"-w", "4", "-"},
			Want:  "abcd\nefgh\n",
		},
		{
			Name:  "dash between files reads the standard input at that point",
			Stdin: strings.NewReader("abcdefgh\n"),
			FS:    testFS(),
			Args:  []string{"-w", "4", "tabs.txt", "-"},
			Want:  "ab\n\t\ncd\nabcd\nefgh\n",
		},
		{
			Name: "pathnames are resolved from the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"-w", "4", "inner.txt"},
			Want: "abcd\nefgh\ni\n",
		},
		{
			Name: "a missing final newline is not added",
			FS:   testFS(),
			Args: []string{"-w", "3", "nonl.txt"},
			Want: "abc\ndef",
		},
		{
			Name: "an empty file produces no output",
			FS:   testFS(),
			Args: []string{"empty.txt"},
			Want: "",
		},
		{
			Name: "a file named like an option after a double dash",
			FS:   testFS(),
			Args: []string{"--", "-s"},
			Want: "aa bb\n",
		},
		{
			Name:     "options are not parsed after the first operand",
			FS:       testFS(),
			Args:     []string{"tabs.txt", "-x"},
			Want:     "ab\tcd\n",
			WantErr:  "fold: -x: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file doesn't stop the other files",
			FS:       testFS(),
			Args:     []string{"missing.txt", "tabs.txt"},
			Want:     "ab\tcd\n",
			WantErr:  "fold: missing.txt: file does not exist\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "invalid option",
			Args:     []string{"-x"},
			WantErr:  "fold: invalid option -- 'x'\nTry 'fold --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unknown long option",
			Args:     []string{"--foo"},
			WantErr:  "fold: unrecognized option '--foo'\nTry 'fold --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing width argument",
			Args:     []string{"-w"},
			WantErr:  "fold: option requires an argument -- 'w'\nTry 'fold --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a zero width",
			Args:     []string{"-w", "0"},
			WantErr:  "fold: invalid width: '0'\nTry 'fold --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative width",
			Args:     []string{"-w", "-1"},
			WantErr:  "fold: invalid width: '-1'\nTry 'fold --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a non numeric width",
			Args:     []string{"-w", "x"},
			WantErr:  "fold: invalid width: 'x'\nTry 'fold --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a decimal width",
			Args:     []string{"-w", "4.5"},
			WantErr:  "fold: invalid width: '4.5'\nTry 'fold --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "width",
			Stdin: strings.NewReader("abcdefgh\n"),
			Args:  []string{"--width", "5"},
			Want:  "abcde\nfgh\n",
		},
		{
			Name:  "width with an attached value",
			Stdin: strings.NewReader("abcdefgh\n"),
			Args:  []string{"--width=5"},
			Want:  "abcde\nfgh\n",
		},
		{
			Name:  "bytes",
			Stdin: strings.NewReader("áé\n"),
			Args:  []string{"--bytes", "--width", "2"},
			Want:  "á\né\n",
		},
		{
			Name:  "spaces",
			Stdin: strings.NewReader("aaa bbbb\n"),
			Args:  []string{"--spaces", "--width", "4"},
			Want:  "aaa \nbbbb\n",
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
			WantErr:  "fold: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "directory operand",
			FS:       fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			Args:     []string{"dir"},
			WantErr:  "fold: dir: invalid argument\n",
			WantExit: 1,
		},
		{
			Name:     "zero-length file name",
			FS:       testFS(),
			Args:     []string{""},
			WantErr:  "fold: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "file operand without a file system",
			Args:     []string{"tabs.txt"},
			WantErr:  "fold: tabs.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			WantErr:  "fold: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "standard input read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			WantErr:  "fold: standard input: read failed\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("hello\n"),
		Stdout: testutil.ErrorWriter{Err: writeErr},
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context())

	if code := testutil.ExitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
	if got := stderr.String(); got != "fold: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "fold: write error: write failed\n")
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		testutil.Cancelled(t, New, strings.NewReader("hello\n"), nil)
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		testutil.Cancelled(t, New, &testutil.CancelReader{}, nil)
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "fold", usage)
}
