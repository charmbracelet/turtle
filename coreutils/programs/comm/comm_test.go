package comm

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
//	left.txt:      "apple\nbanana\ncherry\ndate\n"
//	right.txt:     "banana\ncherry\nelderberry\n"
//	same1.txt:     "a\nb\nc\n"
//	same2.txt:     "a\nb\nc\n"
//	empty.txt:     ""
//	nonl1.txt:     "a\nb" (no trailing newline)
//	nonl2.txt:     "a\nb\n"
//	"-1":          "x\n" (a file named like an option)
//	sub/inner.txt: "i\nj\n"
//	sub/other.txt: "j\nk\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"left.txt":      {Data: []byte("apple\nbanana\ncherry\ndate\n")},
		"right.txt":     {Data: []byte("banana\ncherry\nelderberry\n")},
		"same1.txt":     {Data: []byte("a\nb\nc\n")},
		"same2.txt":     {Data: []byte("a\nb\nc\n")},
		"empty.txt":     {Data: []byte("")},
		"nonl1.txt":     {Data: []byte("a\nb")},
		"nonl2.txt":     {Data: []byte("a\nb\n")},
		"-1":            {Data: []byte("x\n")},
		"sub/inner.txt": {Data: []byte("i\nj\n")},
		"sub/other.txt": {Data: []byte("j\nk\n")},
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "three columns",
			FS:   testFS(),
			Args: []string{"left.txt", "right.txt"},
			Want: "apple\n\t\tbanana\n\t\tcherry\ndate\n\telderberry\n",
		},
		{
			// A missing final newline is treated as present: the
			// unterminated "b" equals the terminated one, and the output
			// is still newline-terminated.
			Name: "a missing final newline takes part in comparisons",
			FS:   testFS(),
			Args: []string{"nonl1.txt", "nonl2.txt"},
			Want: "\t\ta\n\t\tb\n",
		},
		{
			Name: "a missing final newline is terminated in the output",
			FS:   testFS(),
			Args: []string{"nonl1.txt", "nonl1.txt"},
			Want: "\t\ta\n\t\tb\n",
		},
		{
			Name: "identical files are all in the third column",
			FS:   testFS(),
			Args: []string{"same1.txt", "same2.txt"},
			Want: "\t\ta\n\t\tb\n\t\tc\n",
		},
		{
			Name: "an empty second file leaves every line unique to the first",
			FS:   testFS(),
			Args: []string{"left.txt", "empty.txt"},
			Want: "apple\nbanana\ncherry\ndate\n",
		},
		{
			Name: "an empty first file leaves every line unique to the second",
			FS:   testFS(),
			Args: []string{"empty.txt", "right.txt"},
			Want: "\tbanana\n\tcherry\n\telderberry\n",
		},
		{
			Name: "two empty files produce no output",
			FS:   testFS(),
			Args: []string{"empty.txt", "empty.txt"},
		},
		{
			Name:  "the first file can be the standard input",
			FS:    testFS(),
			Stdin: strings.NewReader("apple\ncherry\n"),
			Args:  []string{"-", "right.txt"},
			Want:  "apple\n\tbanana\n\t\tcherry\n\telderberry\n",
		},
		{
			Name:  "the second file can be the standard input",
			FS:    testFS(),
			Stdin: strings.NewReader("cherry\nelderberry\n"),
			Args:  []string{"left.txt", "-"},
			Want:  "apple\nbanana\n\t\tcherry\ndate\n\telderberry\n",
		},
		{
			Name: "a double dash ends the options",
			FS:   testFS(),
			Args: []string{"--", "left.txt", "right.txt"},
			Want: "apple\n\t\tbanana\n\t\tcherry\ndate\n\telderberry\n",
		},
		{
			Name: "options are not parsed after the first operand",
			FS:   testFS(),
			Args: []string{"left.txt", "-1"},
			Want: "apple\nbanana\ncherry\ndate\n\tx\n",
		},
		{
			Name: "operands resolve against the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"inner.txt", "other.txt"},
			Want: "i\n\t\tj\n\tk\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-1 suppresses the first column",
			FS:   testFS(),
			Args: []string{"-1", "left.txt", "right.txt"},
			Want: "\tbanana\n\tcherry\nelderberry\n",
		},
		{
			Name: "-2 suppresses the second column",
			FS:   testFS(),
			Args: []string{"-2", "left.txt", "right.txt"},
			Want: "apple\n\tbanana\n\tcherry\ndate\n",
		},
		{
			Name: "-3 suppresses the third column",
			FS:   testFS(),
			Args: []string{"-3", "left.txt", "right.txt"},
			Want: "apple\ndate\n\telderberry\n",
		},
		{
			Name: "-12 writes only the common lines",
			FS:   testFS(),
			Args: []string{"-12", "left.txt", "right.txt"},
			Want: "banana\ncherry\n",
		},
		{
			Name: "-13 writes only the lines unique to the second file",
			FS:   testFS(),
			Args: []string{"-13", "left.txt", "right.txt"},
			Want: "elderberry\n",
		},
		{
			Name: "-23 writes only the lines unique to the first file",
			FS:   testFS(),
			Args: []string{"-23", "left.txt", "right.txt"},
			Want: "apple\ndate\n",
		},
		{
			Name: "-123 writes nothing",
			FS:   testFS(),
			Args: []string{"-123", "left.txt", "right.txt"},
		},
		{
			Name: "options can be given one by one",
			FS:   testFS(),
			Args: []string{"-1", "-2", "left.txt", "right.txt"},
			Want: "banana\ncherry\n",
		},
		{
			Name: "an option can be repeated",
			FS:   testFS(),
			Args: []string{"-1", "-1", "left.txt", "right.txt"},
			Want: "\tbanana\n\tcherry\nelderberry\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOutputDelimiter(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "--output-delimiter replaces the tabs between columns",
			FS:   testFS(),
			Args: []string{"--output-delimiter=|", "left.txt", "right.txt"},
			Want: "apple\n||banana\n||cherry\ndate\n|elderberry\n",
		},
		{
			Name: "the delimiter can be given in the next argument",
			FS:   testFS(),
			Args: []string{"--output-delimiter", "|", "left.txt", "right.txt"},
			Want: "apple\n||banana\n||cherry\ndate\n|elderberry\n",
		},
		{
			Name: "an empty delimiter selects NUL",
			FS:   testFS(),
			Args: []string{"--output-delimiter=", "left.txt", "right.txt"},
			Want: "apple\n\x00\x00banana\n\x00\x00cherry\ndate\n\x00elderberry\n",
		},
		{
			Name: "a suppressed column removes its delimiter",
			FS:   testFS(),
			Args: []string{"-1", "--output-delimiter=|", "left.txt", "right.txt"},
			Want: "|banana\n|cherry\nelderberry\n",
		},
		{
			Name: "two suppressed columns leave no delimiters",
			FS:   testFS(),
			Args: []string{"-12", "--output-delimiter=|", "left.txt", "right.txt"},
			Want: "banana\ncherry\n",
		},
		{
			Name: "the last delimiter given is the one used",
			FS:   testFS(),
			Args: []string{"--output-delimiter=|", "--output-delimiter=+", "left.txt", "right.txt"},
			Want: "apple\n++banana\n++cherry\ndate\n+elderberry\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunZeroTerminated(t *testing.T) {
	zfs := fsx.MemFS{
		"z1.txt": {Data: []byte("apple\x00date\x00")},
		"z2.txt": {Data: []byte("date\x00elderberry\x00")},
	}
	tests := []testutil.Case{
		{
			Name: "-z reads and writes NUL-terminated items",
			FS:   zfs,
			Args: []string{"-z", "z1.txt", "z2.txt"},
			Want: "apple\x00\t\tdate\x00\telderberry\x00",
		},
		{
			Name: "--zero-terminated is the long form of -z",
			FS:   zfs,
			Args: []string{"--zero-terminated", "z1.txt", "z2.txt"},
			Want: "apple\x00\t\tdate\x00\telderberry\x00",
		},
		{
			Name: "-z can be grouped with other options",
			FS:   zfs,
			Args: []string{"-1z", "z1.txt", "z2.txt"},
			Want: "\tdate\x00elderberry\x00",
		},
		{
			Name: "-z can use a custom output delimiter",
			FS:   zfs,
			Args: []string{"-z", "--output-delimiter=|", "z1.txt", "z2.txt"},
			Want: "apple\x00||date\x00|elderberry\x00",
		},
		{
			// A missing final NUL is treated as present, so the
			// unterminated "b" equals the terminated one.
			Name: "a missing final NUL takes part in comparisons",
			FS: fsx.MemFS{
				"z1.txt": {Data: []byte("a\x00b")},
				"z2.txt": {Data: []byte("a\x00b\x00")},
			},
			Args: []string{"-z", "z1.txt", "z2.txt"},
			Want: "\t\ta\x00\t\tb\x00",
		},
		{
			Name: "newlines are ordinary bytes under -z",
			FS:   fsx.MemFS{"z1.txt": {Data: []byte("a\nb\x00")}, "z2.txt": {Data: []byte("a\nb\x00")}},
			Args: []string{"-z", "z1.txt", "z2.txt"},
			Want: "\t\ta\nb\x00",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunInvalidUsage(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "no operands",
			FS:       testFS(),
			WantErr:  "comm: missing operand\nTry 'comm --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "one operand",
			FS:       testFS(),
			Args:     []string{"left.txt"},
			WantErr:  "comm: missing operand\nTry 'comm --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an extra operand",
			FS:       testFS(),
			Args:     []string{"left.txt", "right.txt", "third.txt"},
			WantErr:  "comm: extra operand 'third.txt'\nTry 'comm --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid option",
			FS:       testFS(),
			Args:     []string{"-x", "left.txt", "right.txt"},
			WantErr:  "comm: invalid option -- 'x'\nTry 'comm --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a long option",
			FS:       testFS(),
			Args:     []string{"--total", "left.txt", "right.txt"},
			WantErr:  "comm: unrecognized option '--total'\nTry 'comm --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a short option that only exists in long form",
			FS:       testFS(),
			Args:     []string{"-o", "left.txt", "right.txt"},
			WantErr:  "comm: invalid option -- 'o'\nTry 'comm --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a long option missing its argument",
			FS:       testFS(),
			Args:     []string{"--output-delimiter"},
			WantErr:  "comm: option '--output-delimiter' requires an argument\nTry 'comm --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "missing first file",
			FS:       testFS(),
			Args:     []string{"missing.txt", "right.txt"},
			WantErr:  "comm: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "missing second file",
			FS:       testFS(),
			Args:     []string{"left.txt", "missing.txt"},
			WantErr:  "comm: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "the first missing file is reported when both are missing",
			FS:       testFS(),
			Args:     []string{"missing1.txt", "missing2.txt"},
			WantErr:  "comm: missing1.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "directory input",
			FS:       fsx.MemFS{"dir": {Mode: fs.ModeDir}, "right.txt": {Data: []byte("a\n")}},
			Args:     []string{"dir", "right.txt"},
			WantErr:  "comm: dir: invalid argument\n",
			WantExit: 1,
		},
		{
			Name:     "input file without a file system",
			Args:     []string{"left.txt", "right.txt"},
			WantErr:  "comm: left.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			FS:       testFS(),
			Args:     []string{"-", "right.txt"},
			WantErr:  "comm: -: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "standard input read error",
			FS:       testFS(),
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			Args:     []string{"-", "right.txt"},
			WantErr:  "comm: -: read failed\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("a\n"),
		Stdout: testutil.ErrorWriter{Err: writeErr},
		Stderr: stderr,
		FS:     fsx.MemFS{"empty.txt": {Data: []byte("")}},
	}
	err := New(rt).Run(t.Context(), "-", "empty.txt")

	if code := testutil.ExitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
	if got := stderr.String(); got != "comm: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "comm: write error: write failed\n")
	}
}

func TestRunContext(t *testing.T) {
	files := fsx.MemFS{"right.txt": {Data: []byte("hello\n")}}

	t.Run("cancelled before running", func(t *testing.T) {
		testutil.Cancelled(t, New, strings.NewReader("hello\n"), files, "-", "right.txt")
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		testutil.Cancelled(t, New, &testutil.CancelReader{}, files, "-", "right.txt")
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "comm", usage)
}
