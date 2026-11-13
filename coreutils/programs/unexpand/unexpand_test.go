package unexpand

import (
	"bytes"
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
//	spaces.txt:    "        foo\n" (8 leading blanks)
//	mid.txt:       "a       b\n" (7 blanks after a character)
//	utf8.txt:      "á       b\n" (a multibyte character before the blanks)
//	invalid.txt:   "a\xff       b\n" (an invalid UTF-8 byte)
//	nonl.txt:      "        x" (no trailing newline)
//	empty.txt:     ""
//	"-a":          "        z\n" (a file named like an option)
//	sub/inner.txt: "    y\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"spaces.txt":    {Data: []byte("        foo\n")},
		"mid.txt":       {Data: []byte("a       b\n")},
		"utf8.txt":      {Data: []byte("á       b\n")},
		"invalid.txt":   {Data: []byte{'a', 0xff, ' ', ' ', ' ', ' ', ' ', ' ', ' ', 'b', '\n'}},
		"nonl.txt":      {Data: []byte("        x")},
		"empty.txt":     {Data: []byte("")},
		"-a":            {Data: []byte("        z\n")},
		"sub/inner.txt": {Data: []byte("    y\n")},
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no operands read the standard input",
			Stdin: strings.NewReader("        foo\n"),
			Want:  "\tfoo\n",
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
			Name:  "fewer than a tab stop of leading blanks",
			Stdin: strings.NewReader("       foo\n"),
			Want:  "       foo\n",
		},
		{
			Name:  "leading blanks become tabs and spaces",
			Stdin: strings.NewReader("         foo\n"),
			Want:  "\t foo\n",
		},
		{
			Name:  "two tab stops of leading blanks",
			Stdin: strings.NewReader("                foo\n"),
			Want:  "\t\tfoo\n",
		},
		{
			Name:  "leading blanks and tabs mix",
			Stdin: strings.NewReader("  \tfoo\n"),
			Want:  "\tfoo\n",
		},
		{
			Name:  "interior blanks are not converted",
			Stdin: strings.NewReader("a       b\n"),
			Want:  "a       b\n",
		},
		{
			Name:  "a line starting with a non blank is untouched",
			Stdin: strings.NewReader("x       y\n"),
			Want:  "x       y\n",
		},
		{
			Name:  "a missing final newline is not added",
			Stdin: strings.NewReader("        x"),
			Want:  "\tx",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunAll(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "interior blanks preceding a stop are converted",
			Stdin: strings.NewReader("a       b\n"),
			Args:  []string{"-a"},
			Want:  "a\tb\n",
		},
		{
			Name:  "interior blanks not reaching a stop stay",
			Stdin: strings.NewReader("a  b\n"),
			Args:  []string{"-a"},
			Want:  "a  b\n",
		},
		{
			Name:  "a run of blanks and tabs",
			Stdin: strings.NewReader("a \tb\n"),
			Args:  []string{"-a"},
			Want:  "a\tb\n",
		},
		{
			Name:  "only the runs reaching a stop are converted",
			Stdin: strings.NewReader("a       b     c\n"),
			Args:  []string{"-a"},
			Want:  "a\tb     c\n",
		},
		{
			Name:  "trailing blanks",
			Stdin: strings.NewReader("a       \n"),
			Args:  []string{"-a"},
			Want:  "a\t\n",
		},
		{
			Name:  "leading blanks are still converted",
			Stdin: strings.NewReader("        foo\n"),
			Args:  []string{"-a"},
			Want:  "\tfoo\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunTabStops(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a single number sets the distance between stops",
			Stdin: strings.NewReader("     foo\n"),
			Args:  []string{"-t", "4"},
			Want:  "\t foo\n",
		},
		{
			Name:  "a single number of eight is the default distance",
			Stdin: strings.NewReader("        foo\n"),
			Args:  []string{"-t", "8"},
			Want:  "\tfoo\n",
		},
		{
			Name:  "a list of stops",
			Stdin: strings.NewReader("    x\n"),
			Args:  []string{"-t", "4,8"},
			Want:  "\tx\n",
		},
		{
			Name:  "a list of stops with a remainder",
			Stdin: strings.NewReader("     x\n"),
			Args:  []string{"-t", "4,8"},
			Want:  "\t x\n",
		},
		{
			Name:  "multiple list stops",
			Stdin: strings.NewReader("         x\n"),
			Args:  []string{"-t", "4,8"},
			Want:  "\t\t x\n",
		},
		{
			Name:  "no conversions past the last stop",
			Stdin: strings.NewReader("          x\n"),
			Args:  []string{"-t", "4,8"},
			Want:  "\t\t  x\n",
		},
		{
			Name:  "a tab past the last stop is copied",
			Stdin: strings.NewReader("        \t  x\n"),
			Args:  []string{"-t", "4,8"},
			Want:  "\t\t\t  x\n",
		},
		{
			Name:  "a single blank just before a stop is not converted",
			Stdin: strings.NewReader("a b\n"),
			Args:  []string{"-t", "2"},
			Want:  "a b\n",
		},
		{
			Name:  "two blanks before a stop are converted",
			Stdin: strings.NewReader("a  b\n"),
			Args:  []string{"-t", "2"},
			Want:  "a\t b\n",
		},
		{
			Name:  "a lone leading blank on a stop is converted",
			Stdin: strings.NewReader(" x\n"),
			Args:  []string{"-t", "1"},
			Want:  "\tx\n",
		},
		{
			Name:  "t implies a",
			Stdin: strings.NewReader("a       b\n"),
			Args:  []string{"-t", "8"},
			Want:  "a\tb\n",
		},
		{
			Name:  "grouped a and t",
			Stdin: strings.NewReader("     foo\n"),
			Args:  []string{"-at", "4"},
			Want:  "\t foo\n",
		},
		{
			Name:  "the last list wins",
			Stdin: strings.NewReader("     foo\n"),
			Args:  []string{"-t", "8", "-t", "4"},
			Want:  "\t foo\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunFirstOnly(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "interior blanks are not converted",
			Stdin: strings.NewReader("a       b\n"),
			Args:  []string{"--first-only"},
			Want:  "a       b\n",
		},
		{
			Name:  "leading blanks are converted",
			Stdin: strings.NewReader("        foo\n"),
			Args:  []string{"--first-only"},
			Want:  "\tfoo\n",
		},
		{
			Name:  "the leading blanks and the interior ones are separated",
			Stdin: strings.NewReader("        a  b\n"),
			Args:  []string{"--first-only"},
			Want:  "\ta  b\n",
		},
		{
			Name:  "first-only overrides a given all",
			Stdin: strings.NewReader("a       b\n"),
			Args:  []string{"--all", "--first-only"},
			Want:  "a       b\n",
		},
		{
			Name:  "first-only overrides all given before it",
			Stdin: strings.NewReader("a       b\n"),
			Args:  []string{"--first-only", "--all"},
			Want:  "a       b\n",
		},
		{
			Name:  "first-only with tabs converts only the leading blanks",
			Stdin: strings.NewReader("        a  b\n"),
			Args:  []string{"--first-only", "-t", "4"},
			Want:  "\t\ta  b\n",
		},
		{
			Name:  "first-only cancels the all implied by tabs",
			Stdin: strings.NewReader("a       b\n"),
			Args:  []string{"--first-only", "-t", "8"},
			Want:  "a       b\n",
		},
		{
			Name:  "first-only cancels the all implied by tabs given before it",
			Stdin: strings.NewReader("a       b\n"),
			Args:  []string{"-t", "8", "--first-only"},
			Want:  "a       b\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunBackspace(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "backspaces are copied and decrement the column",
			Stdin: strings.NewReader("  \b        x\n"),
			Args:  []string{"-a"},
			Want:  "  \b\t x\n",
		},
		{
			Name:  "the column is not decremented below zero",
			Stdin: strings.NewReader("\b        x\n"),
			Args:  []string{"-a"},
			Want:  "\b\tx\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunUTF8(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a multibyte character counts as one column",
			FS:   testFS(),
			Args: []string{"-a", "utf8.txt"},
			Want: "á\tb\n",
		},
		{
			Name: "an invalid byte counts as one column",
			FS:   testFS(),
			Args: []string{"-a", "invalid.txt"},
			Want: "a\xff\t b\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunFiles(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"spaces.txt"},
			Want: "\tfoo\n",
		},
		{
			Name: "multiple files are concatenated in order",
			FS:   testFS(),
			Args: []string{"spaces.txt", "mid.txt"},
			Want: "\tfoo\na       b\n",
		},
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("        s\n"),
			FS:    testFS(),
			Args:  []string{"-"},
			Want:  "\ts\n",
		},
		{
			Name:  "dash between files reads the standard input at that point",
			Stdin: strings.NewReader("        s\n"),
			FS:    testFS(),
			Args:  []string{"spaces.txt", "-", "mid.txt"},
			Want:  "\tfoo\n\ts\na       b\n",
		},
		{
			Name: "pathnames are resolved from the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"inner.txt"},
			Want: "    y\n",
		},
		{
			Name: "a missing final newline is not added",
			FS:   testFS(),
			Args: []string{"nonl.txt"},
			Want: "\tx",
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
			Args: []string{"--", "-a"},
			Want: "\tz\n",
		},
		{
			Name:     "options are not parsed after the first operand",
			FS:       testFS(),
			Args:     []string{"spaces.txt", "-x"},
			Want:     "\tfoo\n",
			WantErr:  "unexpand: -x: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file doesn't stop the other files",
			FS:       testFS(),
			Args:     []string{"missing.txt", "spaces.txt"},
			Want:     "\tfoo\n",
			WantErr:  "unexpand: missing.txt: file does not exist\n",
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
			WantErr:  "unexpand: invalid option -- 'x'\nTry 'unexpand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unknown long option",
			Args:     []string{"--foo"},
			WantErr:  "unexpand: unrecognized option '--foo'\nTry 'unexpand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing tab list argument",
			Args:     []string{"-t"},
			WantErr:  "unexpand: option requires an argument -- 't'\nTry 'unexpand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "zero distance",
			Args:     []string{"-t", "0"},
			WantErr:  "unexpand: invalid tab stop: '0'\nTry 'unexpand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a non numeric stop",
			Args:     []string{"-t", "x"},
			WantErr:  "unexpand: invalid tab stop: 'x'\nTry 'unexpand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "not ascending stops",
			Args:     []string{"-t", "8,4"},
			WantErr:  "unexpand: tab sizes must be ascending\nTry 'unexpand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "first-only with a value",
			Args:     []string{"--first-only=1"},
			WantErr:  "unexpand: option '--first-only=1' doesn't allow an argument\nTry 'unexpand --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty tab list",
			Args:     []string{"-t", ""},
			WantErr:  "unexpand: empty tab list\nTry 'unexpand --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "all",
			Stdin: strings.NewReader("a       b\n"),
			Args:  []string{"--all"},
			Want:  "a\tb\n",
		},
		{
			Name:  "tabs",
			Stdin: strings.NewReader("     foo\n"),
			Args:  []string{"--tabs", "4"},
			Want:  "\t foo\n",
		},
		{
			Name:  "tabs with an attached value",
			Stdin: strings.NewReader("     foo\n"),
			Args:  []string{"--tabs=4"},
			Want:  "\t foo\n",
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
			WantErr:  "unexpand: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "directory operand",
			FS:       fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			Args:     []string{"dir"},
			WantErr:  "unexpand: dir: invalid argument\n",
			WantExit: 1,
		},
		{
			Name:     "zero-length file name",
			FS:       testFS(),
			Args:     []string{""},
			WantErr:  "unexpand: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "file operand without a file system",
			Args:     []string{"spaces.txt"},
			WantErr:  "unexpand: spaces.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			WantErr:  "unexpand: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "standard input read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			WantErr:  "unexpand: standard input: read failed\n",
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
	if got := stderr.String(); got != "unexpand: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "unexpand: write error: write failed\n")
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
	testutil.HelpVersion(t, New, "unexpand", usage)
}
