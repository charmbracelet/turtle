package expand

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
//	tabs.txt:      "\tfoo\nbar\tbaz\n"
//	utf8.txt:      "á\tb\n日本\t語\n"
//	invalid.txt:   "a\xff\tb\n" (an invalid UTF-8 byte)
//	nonl.txt:      "a\tb" (no trailing newline)
//	empty.txt:     ""
//	"-t":          "z\tw\n" (a file named like an option)
//	sub/inner.txt: "x\ty\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"tabs.txt":      {Data: []byte("\tfoo\nbar\tbaz\n")},
		"utf8.txt":      {Data: []byte("á\tb\n日本\t語\n")},
		"invalid.txt":   {Data: []byte{'a', 0xff, '\t', 'b', '\n'}},
		"nonl.txt":      {Data: []byte("a\tb")},
		"empty.txt":     {Data: []byte("")},
		"-t":            {Data: []byte("z\tw\n")},
		"sub/inner.txt": {Data: []byte("x\ty\n")},
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no operands read the standard input",
			Stdin: strings.NewReader("\tfoo\n"),
			Want:  "        foo\n",
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
			Name:  "a tab jumps to the next stop",
			Stdin: strings.NewReader("a\tb\n"),
			Want:  "a       b\n",
		},
		{
			Name:  "a tab on a stop jumps to the next one",
			Stdin: strings.NewReader("12345678\tb\n"),
			Want:  "12345678        b\n",
		},
		{
			Name:  "consecutive tabs",
			Stdin: strings.NewReader("\t\tx\n"),
			Want:  "                x\n",
		},
		{
			Name:  "a line without tabs is copied",
			Stdin: strings.NewReader("plain text\n"),
			Want:  "plain text\n",
		},
		{
			Name:  "a carriage return is an ordinary character",
			Stdin: strings.NewReader("ab\rc\td\n"),
			Want:  "ab\rc    d\n",
		},
		{
			Name:  "a missing final newline is not added",
			Stdin: strings.NewReader("a\tb"),
			Want:  "a       b",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunTabStops(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a single number sets the distance between stops",
			Stdin: strings.NewReader("a\tb\tc\n"),
			Args:  []string{"-t", "4"},
			Want:  "a   b   c\n",
		},
		{
			Name:  "a distance of one",
			Stdin: strings.NewReader("\ta\tb\n"),
			Args:  []string{"-t", "1"},
			Want:  " a b\n",
		},
		{
			Name:  "an attached distance",
			Stdin: strings.NewReader("a\tb\n"),
			Args:  []string{"-t4"},
			Want:  "a   b\n",
		},
		{
			Name:  "a list of stops",
			Stdin: strings.NewReader("\ta\tb\n"),
			Args:  []string{"-t", "4,8"},
			Want:  "    a   b\n",
		},
		{
			Name:  "a blank separated list of stops",
			Stdin: strings.NewReader("\ta\tb\n"),
			Args:  []string{"-t", "4 8"},
			Want:  "    a   b\n",
		},
		{
			Name:  "tabs past the last stop become single spaces",
			Stdin: strings.NewReader("\ta\tb\tc\td\n"),
			Args:  []string{"-t", "4,8"},
			Want:  "    a   b c d\n",
		},
		{
			Name:  "the last list wins",
			Stdin: strings.NewReader("\ta\n"),
			Args:  []string{"-t", "4", "-t", "8"},
			Want:  "        a\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunInitial(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a leading tab is converted",
			Stdin: strings.NewReader("\tfoo\n"),
			Args:  []string{"-i"},
			Want:  "        foo\n",
		},
		{
			Name:  "consecutive leading tabs are converted",
			Stdin: strings.NewReader("\t\tx\n"),
			Args:  []string{"-i"},
			Want:  "                x\n",
		},
		{
			Name:  "a tab after a non blank character is kept",
			Stdin: strings.NewReader("bar\tbaz\n"),
			Args:  []string{"-i"},
			Want:  "bar\tbaz\n",
		},
		{
			Name:  "spaces before a leading tab count as blanks",
			Stdin: strings.NewReader(" \tb\n"),
			Args:  []string{"-i"},
			Want:  "        b\n",
		},
		{
			Name:  "the initial blanks are reset on each line",
			Stdin: strings.NewReader("a\tb\n\tc\n"),
			Args:  []string{"-i"},
			Want:  "a\tb\n        c\n",
		},
		{
			Name:  "a backspace ends the initial blanks",
			Stdin: strings.NewReader("\b\ta\n"),
			Args:  []string{"-i"},
			Want:  "\b\ta\n",
		},
		{
			Name:  "a backspace after blanks ends them",
			Stdin: strings.NewReader(" \b\ta\n"),
			Args:  []string{"-i"},
			Want:  " \b\ta\n",
		},
		{
			Name:  "a line without tabs is copied",
			Stdin: strings.NewReader("plain text\n"),
			Args:  []string{"-i"},
			Want:  "plain text\n",
		},
		{
			Name:  "a line with blanks only is converted",
			Stdin: strings.NewReader(" \t\n"),
			Args:  []string{"-i"},
			Want:  "        \n",
		},
		{
			Name:  "a missing final newline is not added",
			Stdin: strings.NewReader("a\tb"),
			Args:  []string{"-i"},
			Want:  "a\tb",
		},
		{
			Name:  "a single distance",
			Stdin: strings.NewReader("\ta\tb\n"),
			Args:  []string{"-i", "-t", "4"},
			Want:  "    a\tb\n",
		},
		{
			Name:  "a list of stops",
			Stdin: strings.NewReader("\t\ta\n"),
			Args:  []string{"-i", "-t", "4,8"},
			Want:  "        a\n",
		},
		{
			Name:  "a leading tab past the last stop becomes a single space",
			Stdin: strings.NewReader("\t\t\ta\n"),
			Args:  []string{"-i", "-t", "4,8"},
			Want:  "         a\n",
		},
		{
			Name:  "grouped with a valued option",
			Stdin: strings.NewReader("\ta\tb\n"),
			Args:  []string{"-it4"},
			Want:  "    a\tb\n",
		},
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"-i", "tabs.txt"},
			Want: "        foo\nbar\tbaz\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunBackspace(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a backspace decrements the column",
			Stdin: strings.NewReader("ab\b\tc\n"),
			Want:  "ab\b       c\n",
		},
		{
			Name:  "the column is not decremented below zero",
			Stdin: strings.NewReader("\b\ta\n"),
			Want:  "\b        a\n",
		},
		{
			Name:  "a tab is recomputed after a backspace",
			Stdin: strings.NewReader("12345678\b\ta\n"),
			Want:  "12345678\b a\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunUTF8(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a multibyte character counts as one column",
			Stdin: strings.NewReader("á\tb\n"),
			Want:  "á       b\n",
		},
		{
			Name: "an invalid byte counts as one column",
			FS:   testFS(),
			Args: []string{"invalid.txt"},
			Want: "a\xff      b\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunFiles(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"tabs.txt"},
			Want: "        foo\nbar     baz\n",
		},
		{
			Name: "multiple files are concatenated in order",
			FS:   testFS(),
			Args: []string{"tabs.txt", "utf8.txt"},
			Want: "        foo\nbar     baz\ná       b\n日本      語\n",
		},
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("s\tt\n"),
			FS:    testFS(),
			Args:  []string{"-"},
			Want:  "s       t\n",
		},
		{
			Name:  "dash between files reads the standard input at that point",
			Stdin: strings.NewReader("s\tt\n"),
			FS:    testFS(),
			Args:  []string{"nonl.txt", "-"},
			Want:  "a       bs       t\n",
		},
		{
			Name: "pathnames are resolved from the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"inner.txt"},
			Want: "x       y\n",
		},
		{
			Name: "a missing final newline is not added",
			FS:   testFS(),
			Args: []string{"nonl.txt"},
			Want: "a       b",
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
			Args: []string{"--", "-t"},
			Want: "z       w\n",
		},
		{
			Name:     "options are not parsed after the first operand",
			FS:       testFS(),
			Args:     []string{"tabs.txt", "-x"},
			Want:     "        foo\nbar     baz\n",
			WantErr:  "expand: -x: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file doesn't stop the other files",
			FS:       testFS(),
			Args:     []string{"missing.txt", "tabs.txt"},
			Want:     "        foo\nbar     baz\n",
			WantErr:  "expand: missing.txt: file does not exist\n",
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
			WantErr:  "expand: invalid option -- 'x'\nTry 'expand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unknown long option",
			Args:     []string{"--foo"},
			WantErr:  "expand: unrecognized option '--foo'\nTry 'expand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing tab list argument",
			Args:     []string{"-t"},
			WantErr:  "expand: option requires an argument -- 't'\nTry 'expand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "zero distance",
			Args:     []string{"-t", "0"},
			WantErr:  "expand: invalid tab stop: '0'\nTry 'expand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative stop",
			Args:     []string{"-t", "-4"},
			WantErr:  "expand: invalid tab stop: '-4'\nTry 'expand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a non numeric stop",
			Args:     []string{"-t", "x"},
			WantErr:  "expand: invalid tab stop: 'x'\nTry 'expand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "not ascending stops",
			Args:     []string{"-t", "8,4"},
			WantErr:  "expand: tab sizes must be ascending\nTry 'expand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "repeated stops",
			Args:     []string{"-t", "4,4"},
			WantErr:  "expand: tab sizes must be ascending\nTry 'expand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty tab list",
			Args:     []string{"-t", ""},
			WantErr:  "expand: empty tab list\nTry 'expand --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "tabs",
			Stdin: strings.NewReader("a\tb\n"),
			Args:  []string{"--tabs", "4"},
			Want:  "a   b\n",
		},
		{
			Name:  "tabs with an attached value",
			Stdin: strings.NewReader("a\tb\n"),
			Args:  []string{"--tabs=4"},
			Want:  "a   b\n",
		},
		{
			Name:  "initial",
			Stdin: strings.NewReader("\tfoo\nbar\tbaz\n"),
			Args:  []string{"--initial"},
			Want:  "        foo\nbar\tbaz\n",
		},
		{
			Name:  "initial and tabs",
			Stdin: strings.NewReader("\ta\tb\n"),
			Args:  []string{"--initial", "--tabs", "4"},
			Want:  "    a\tb\n",
		},
		{
			Name:     "initial with an attached value",
			Args:     []string{"--initial=1"},
			WantErr:  "expand: option '--initial=1' doesn't allow an argument\nTry 'expand --help' for more information.\n",
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
			WantErr:  "expand: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "directory operand",
			FS:       fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			Args:     []string{"dir"},
			WantErr:  "expand: dir: invalid argument\n",
			WantExit: 1,
		},
		{
			Name:     "zero-length file name",
			FS:       testFS(),
			Args:     []string{""},
			WantErr:  "expand: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "file operand without a file system",
			Args:     []string{"tabs.txt"},
			WantErr:  "expand: tabs.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			WantErr:  "expand: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "standard input read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			WantErr:  "expand: standard input: read failed\n",
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
	if got := stderr.String(); got != "expand: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "expand: write error: write failed\n")
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
	testutil.HelpVersion(t, New, "expand", usage)
}
