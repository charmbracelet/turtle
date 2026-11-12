package cat

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
//	a.txt:    "alpha\n"
//	b.txt:    "beta\n"
//	empty.txt: ""
//	bin.txt:  arbitrary bytes
//	"-u":     "dash-u\n" (a file named like an option)
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":         {Data: []byte("alpha\n")},
		"b.txt":         {Data: []byte("beta\n")},
		"empty.txt":     {Data: []byte("")},
		"bin.txt":       {Data: []byte{0x00, 0x01, 0xff, '\n'}},
		"-u":            {Data: []byte("dash-u\n")},
		"sub/inner.txt": {Data: []byte("inner\n")},
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no operands reads the standard input",
			Stdin: strings.NewReader("hello\n"),
			Want:  "hello\n",
		},
		{
			Name:  "empty standard input",
			Stdin: strings.NewReader(""),
			Want:  "",
		},
		{
			Name:  "standard input without a trailing newline",
			Stdin: strings.NewReader("no trailing newline"),
			Want:  "no trailing newline",
		},
		{
			Name: "single file",
			FS:   testFS(),
			Args: []string{"a.txt"},
			Want: "alpha\n",
		},
		{
			Name: "multiple files are concatenated in order",
			FS:   testFS(),
			Args: []string{"a.txt", "b.txt"},
			Want: "alpha\nbeta\n",
		},
		{
			Name: "the same file twice",
			FS:   testFS(),
			Args: []string{"a.txt", "a.txt"},
			Want: "alpha\nalpha\n",
		},
		{
			Name: "empty file",
			FS:   testFS(),
			Args: []string{"empty.txt"},
			Want: "",
		},
		{
			Name: "arbitrary bytes are written untouched",
			FS:   testFS(),
			Args: []string{"bin.txt"},
			Want: string([]byte{0x00, 0x01, 0xff, '\n'}),
		},
		{
			Name: "pathnames are resolved from the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"inner.txt"},
			Want: "inner\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunStandardInputOperand(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("stdin\n"),
			Args:  []string{"-"},
			Want:  "stdin\n",
		},
		{
			Name:  "dash combined with a file",
			Stdin: strings.NewReader("stdin\n"),
			FS:    testFS(),
			Args:  []string{"-", "a.txt"},
			Want:  "stdin\nalpha\n",
		},
		{
			Name:  "dash between files reads the standard input at that point",
			Stdin: strings.NewReader("stdin\n"),
			FS:    testFS(),
			Args:  []string{"a.txt", "-", "b.txt"},
			Want:  "alpha\nstdin\nbeta\n",
		},
		{
			Name:  "repeated standard input operands",
			Stdin: strings.NewReader("stdin\n"),
			Args:  []string{"-", "-"},
			Want:  "stdin\n",
		},
		{
			Name:  "double dash with no operands reads the standard input",
			Stdin: strings.NewReader("stdin\n"),
			Args:  []string{"--"},
			Want:  "stdin\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "u is accepted",
			FS:   testFS(),
			Args: []string{"-u", "a.txt"},
			Want: "alpha\n",
		},
		{
			Name:  "u with the standard input",
			Stdin: strings.NewReader("stdin\n"),
			Args:  []string{"-u"},
			Want:  "stdin\n",
		},
		{
			Name: "repeated u",
			FS:   testFS(),
			Args: []string{"-u", "-u", "a.txt"},
			Want: "alpha\n",
		},
		{
			Name: "grouped u",
			FS:   testFS(),
			Args: []string{"-uu", "a.txt"},
			Want: "alpha\n",
		},
		{
			Name: "a lone double dash is accepted",
			FS:   testFS(),
			Args: []string{"--", "a.txt"},
			Want: "alpha\n",
		},
		{
			Name: "a file named like an option after a double dash",
			FS:   testFS(),
			Args: []string{"--", "-u"},
			Want: "dash-u\n",
		},
		{
			Name: "options are not parsed after the first operand",
			FS:   testFS(),
			Args: []string{"a.txt", "-u"},
			Want: "alpha\ndash-u\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunNumber(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "all lines are numbered",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-n"},
			Want:  "     1\ta\n     2\tb\n",
		},
		{
			Name:  "blank lines are numbered",
			Stdin: strings.NewReader("a\n\nb\n"),
			Args:  []string{"-n"},
			Want:  "     1\ta\n     2\t\n     3\tb\n",
		},
		{
			Name:  "a missing trailing newline is numbered",
			Stdin: strings.NewReader("a"),
			Args:  []string{"-n"},
			Want:  "     1\ta",
		},
		{
			Name:  "the long form numbers too",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"--number"},
			Want:  "     1\ta\n",
		},
		{
			Name:  "only non-blank lines are numbered",
			Stdin: strings.NewReader("a\n\nb\n"),
			Args:  []string{"-b"},
			Want:  "     1\ta\n\n     2\tb\n",
		},
		{
			Name:  "b overrides n",
			Stdin: strings.NewReader("a\n\nb\n"),
			Args:  []string{"-nb"},
			Want:  "     1\ta\n\n     2\tb\n",
		},
		{
			Name: "numbering continues across files",
			FS:   fsx.MemFS{"a": {Data: []byte("a\n")}, "b": {Data: []byte("b\n")}},
			Args: []string{"-n", "a", "b"},
			Want: "     1\ta\n     2\tb\n",
		},
		{
			Name: "an unfinished line spans file boundaries",
			FS:   fsx.MemFS{"a": {Data: []byte("a")}, "b": {Data: []byte("\n")}},
			Args: []string{"-n", "a", "b"},
			Want: "     1\ta\n",
		},
		{
			Name:  "an unfinished line spans the standard input and a file",
			Stdin: strings.NewReader("a"),
			FS:    fsx.MemFS{"b": {Data: []byte("b\n")}},
			Args:  []string{"-nE", "-", "b"},
			Want:  "     1\tab$\n",
		},
	}

	testutil.Run(t, New, tests)

	// The counter widens past the six-digit field.
	t.Run("numbers widen past six digits", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		pr := newPrinter(options{number: true}, stdout)
		for range 999998 {
			if err := pr.line([]byte("fill\n")); err != nil {
				t.Fatal(err)
			}
		}
		if err := pr.flush(); err != nil {
			t.Fatal(err)
		}
		stdout.Reset()
		for _, line := range [][]byte{[]byte("x\n"), []byte("y\n")} {
			if err := pr.line(line); err != nil {
				t.Fatal(err)
			}
		}
		if err := pr.flush(); err != nil {
			t.Fatal(err)
		}
		if want := "999999\tx\n1000000\ty\n"; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
	})
}

func TestRunSqueeze(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "repeated blank lines become one",
			Stdin: strings.NewReader("a\n\n\n\nb\n"),
			Args:  []string{"-s"},
			Want:  "a\n\nb\n",
		},
		{
			Name:  "a single blank line is kept",
			Stdin: strings.NewReader("a\n\nb\n"),
			Args:  []string{"-s"},
			Want:  "a\n\nb\n",
		},
		{
			Name:  "leading blanks are squeezed",
			Stdin: strings.NewReader("\n\n\na\n"),
			Args:  []string{"-s"},
			Want:  "\na\n",
		},
		{
			Name:  "blank lines without a newline still squeeze",
			Stdin: strings.NewReader("a\n\n\n"),
			Args:  []string{"-s"},
			Want:  "a\n\n",
		},
		{
			Name:  "the long form squeezes too",
			Stdin: strings.NewReader("a\n\n\nb\n"),
			Args:  []string{"--squeeze-blank"},
			Want:  "a\n\nb\n",
		},
		{
			Name: "squeezing spans file boundaries",
			FS:   fsx.MemFS{"a": {Data: []byte("a\n\n")}, "b": {Data: []byte("\nb\n")}},
			Args: []string{"-s", "a", "b"},
			Want: "a\n\nb\n",
		},
		{
			Name:  "squeeze with numbering",
			Stdin: strings.NewReader("a\n\n\n\nb\n"),
			Args:  []string{"-sn"},
			Want:  "     1\ta\n     2\t\n     3\tb\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunShow(t *testing.T) {
	tests := []testutil.Case{
		// -E.
		{
			Name:  "line ends are marked",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-E"},
			Want:  "a$\n",
		},
		{
			Name:  "a missing trailing newline is still marked",
			Stdin: strings.NewReader("a"),
			Args:  []string{"-E"},
			Want:  "a$",
		},
		// -T.
		{
			Name:  "tabs are shown",
			Stdin: strings.NewReader("a\tb\n"),
			Args:  []string{"-T"},
			Want:  "a^Ib\n",
		},
		// -v.
		{
			Name:  "control characters are shown",
			Stdin: strings.NewReader("a\x01b\n"),
			Args:  []string{"-v"},
			Want:  "a^Ab\n",
		},
		{
			Name:  "nul is shown",
			Stdin: strings.NewReader("\x00\n"),
			Args:  []string{"-v"},
			Want:  "^@\n",
		},
		{
			Name:  "delete is shown",
			Stdin: strings.NewReader("\x7f\n"),
			Args:  []string{"-v"},
			Want:  "^?\n",
		},
		{
			Name:  "tabs are kept by v",
			Stdin: strings.NewReader("a\tb\n"),
			Args:  []string{"-v"},
			Want:  "a\tb\n",
		},
		{
			Name:  "high bytes use the M- notation",
			Stdin: strings.NewReader("\x80\xff\n"),
			Args:  []string{"-v"},
			Want:  "M-^@M-^?\n",
		},
		{
			Name:  "printable multibyte characters pass through",
			Stdin: strings.NewReader("café\n"),
			Args:  []string{"-v"},
			Want:  "café\n",
		},
		{
			Name:  "invalid multibyte bytes use the M- notation",
			Stdin: strings.NewReader("\xc3\x29\n"),
			Args:  []string{"-v"},
			Want:  "M-C)\n",
		},
		// Combinations.
		{
			Name:  "e is v plus E",
			Stdin: strings.NewReader("a\x01\n"),
			Args:  []string{"-e"},
			Want:  "a^A$\n",
		},
		{
			Name:  "t is v plus T",
			Stdin: strings.NewReader("a\tb\n"),
			Args:  []string{"-t"},
			Want:  "a^Ib\n",
		},
		{
			Name:  "A is vET",
			Stdin: strings.NewReader("a\tb\x7f\n"),
			Args:  []string{"-A"},
			Want:  "a^Ib^?$\n",
		},
		{
			Name:  "the long form of A",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"--show-all"},
			Want:  "a$\n",
		},
		{
			Name:  "show ends with numbering",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-nE"},
			Want:  "     1\ta$\n",
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
			WantErr:  "cat: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file doesn't stop the other files",
			FS:       testFS(),
			Args:     []string{"missing.txt", "a.txt"},
			Want:     "alpha\n",
			WantErr:  "cat: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "every missing file reports an error",
			FS:       testFS(),
			Args:     []string{"missing.txt", "other.txt"},
			WantErr:  "cat: missing.txt: file does not exist\ncat: other.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "directory operand",
			FS:       fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			Args:     []string{"dir"},
			WantErr:  "cat: dir: invalid argument\n",
			WantExit: 1,
		},
		{
			Name:     "invalid option",
			Args:     []string{"-x"},
			WantErr:  "cat: invalid option -- 'x'\nTry 'cat --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option in a group",
			Args:     []string{"-ux"},
			WantErr:  "cat: invalid option -- 'x'\nTry 'cat --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unknown long option",
			Args:     []string{"--partial"},
			WantErr:  "cat: unrecognized option '--partial'\nTry 'cat --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option before operands",
			Args:     []string{"-x", "a.txt"},
			WantErr:  "cat: invalid option -- 'x'\nTry 'cat --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "zero-length file name",
			FS:       testFS(),
			Args:     []string{""},
			WantErr:  "cat: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "file operand without a file system",
			Args:     []string{"a.txt"},
			WantErr:  "cat: a.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			FS:       testFS(),
			WantErr:  "cat: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "dash without a standard input",
			FS:       testFS(),
			Args:     []string{"-"},
			WantErr:  "cat: -: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "standard input read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			FS:       testFS(),
			WantErr:  "cat: standard input: read failed\n",
			WantExit: 1,
		},
		{
			Name:     "dash read error is labeled with the operand",
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			Args:     []string{"-"},
			WantErr:  "cat: -: read failed\n",
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
		Args:  []string{"a.txt"},
		Want:  "alpha\n",
	})
	if stdin.Reads > 0 {
		t.Error("Run() read stdin, want it untouched")
	}
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
	if got := stderr.String(); got != "cat: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "cat: write error: write failed\n")
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		testutil.Cancelled(t, New, strings.NewReader("hello\n"), nil)
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		testutil.Cancelled(t, New, &testutil.CancelReader{}, nil)
	})

	t.Run("cancelled while reading a file", func(t *testing.T) {
		// cat doesn't read the standard input when it has a file, so the
		// reader is only there for testutil.Cancelled to hand its cancel to
		// the copy of it inside the file.
		reader := &testutil.CancelReader{}
		testutil.Cancelled(t, New, reader, testutil.ReaderFS(reader), "file")
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "cat", usage)
}
