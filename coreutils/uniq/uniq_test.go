package uniq

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
//	dups.txt:      "a\na\nb\nb\nb\nc\na\n"
//	fields.txt:    "x 1\ny 1\nz 2\n"
//	utf8.txt:      "áx\náy\n"
//	nonl.txt:      "a\na" (no trailing newline)
//	empty.txt:     ""
//	out.txt:       "old\n" (truncated by output tests)
//	"-d":          "p\np\n" (a file named like an option)
//	sub/inner.txt: "i\ni\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"dups.txt":      {Data: []byte("a\na\nb\nb\nb\nc\na\n")},
		"fields.txt":    {Data: []byte("x 1\ny 1\nz 2\n")},
		"utf8.txt":      {Data: []byte("áx\náy\n")},
		"nonl.txt":      {Data: []byte("a\na")},
		"empty.txt":     {Data: []byte("")},
		"out.txt":       {Data: []byte("old\n")},
		"-d":            {Data: []byte("p\np\n")},
		"sub/inner.txt": {Data: []byte("i\ni\n")},
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no operands read the standard input",
			Stdin: strings.NewReader("a\na\nb\n"),
			Want:  "a\nb\n",
		},
		{
			Name:  "adjacent repeated lines collapse",
			Stdin: strings.NewReader("a\na\nb\nb\nb\nc\na\n"),
			Want:  "a\nb\nc\na\n",
		},
		{
			Name:  "repeated lines that are not adjacent are kept",
			Stdin: strings.NewReader("a\nb\na\n"),
			Want:  "a\nb\na\n",
		},
		{
			Name:  "no repeated lines",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Want:  "a\nb\nc\n",
		},
		{
			Name:  "empty lines group together",
			Stdin: strings.NewReader("a\n\n\nb\n"),
			Want:  "a\n\nb\n",
		},
		{
			Name:  "an empty standard input",
			Stdin: strings.NewReader(""),
			Want:  "",
		},
		{
			Name:  "a missing final newline is not added",
			Stdin: strings.NewReader("a\nb"),
			Want:  "a\nb",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunCount(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "each line is preceded by its count",
			Stdin: strings.NewReader("a\na\nb\n"),
			Args:  []string{"-c"},
			Want:  "      2 a\n      1 b\n",
		},
		{
			Name:  "a wider count",
			Stdin: strings.NewReader(strings.Repeat("a\n", 10)),
			Args:  []string{"-c"},
			Want:  "     10 a\n",
		},
		{
			Name:  "counts of repeated lines with d",
			Stdin: strings.NewReader("a\na\nb\n"),
			Args:  []string{"-c", "-d"},
			Want:  "      2 a\n",
		},
		{
			Name:  "counts of unique lines with u",
			Stdin: strings.NewReader("a\na\nb\n"),
			Args:  []string{"-c", "-u"},
			Want:  "      1 b\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunRepeated(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "only repeated lines, one copy each",
			Stdin: strings.NewReader("a\na\nb\nb\nb\nc\n"),
			Args:  []string{"-d"},
			Want:  "a\nb\n",
		},
		{
			Name:  "no repeated lines",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-d"},
			Want:  "",
		},
		{
			Name:  "an unrepeated line is not written",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-d"},
			Want:  "",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunUnique(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "only non repeated lines",
			Stdin: strings.NewReader("a\na\nb\nb\nb\nc\n"),
			Args:  []string{"-u"},
			Want:  "c\n",
		},
		{
			Name:  "all lines repeated",
			Stdin: strings.NewReader("a\na\n"),
			Args:  []string{"-u"},
			Want:  "",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunFields(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the first field is ignored",
			FS:   testFS(),
			Args: []string{"-f", "1", "fields.txt"},
			Want: "x 1\nz 2\n",
		},
		{
			Name:  "zero fields is a no-op",
			Stdin: strings.NewReader("a\na\n"),
			Args:  []string{"-f", "0"},
			Want:  "a\n",
		},
		{
			Name:  "more fields than the line has compare as empty",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-f", "5"},
			Want:  "a\n",
		},
		{
			Name:  "the blanks before a field are part of it",
			Stdin: strings.NewReader("x   a\ny a\n"),
			Args:  []string{"-f", "1"},
			Want:  "x   a\ny a\n",
		},
		{
			Name:  "leading blanks belong to the first field",
			Stdin: strings.NewReader("  a x\n  b x\n"),
			Args:  []string{"-f", "1"},
			Want:  "  a x\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunChars(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the first characters are ignored",
			FS:   testFS(),
			Args: []string{"-s", "1", "utf8.txt"},
			Want: "áx\náy\n",
		},
		{
			Name:  "more characters than the line has compare as empty",
			Stdin: strings.NewReader("ab\ncd\n"),
			Args:  []string{"-s", "5"},
			Want:  "ab\n",
		},
		{
			Name:  "characters are skipped after the fields",
			Stdin: strings.NewReader("1x a\n2y a\n"),
			Args:  []string{"-f", "1", "-s", "1"},
			Want:  "1x a\n",
		},
		{
			Name:  "characters are characters, not bytes",
			Stdin: strings.NewReader("áax\nábx\n"),
			Args:  []string{"-s", "2"},
			Want:  "áax\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunIgnoreCase(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "lines differing only in case group together",
			Stdin: strings.NewReader("a\nA\nb\n"),
			Args:  []string{"-i"},
			Want:  "a\nb\n",
		},
		{
			Name:  "the case of the written line is the first one",
			Stdin: strings.NewReader("A\na\nA\n"),
			Args:  []string{"-i"},
			Want:  "A\n",
		},
		{
			Name:  "non ASCII letters are not folded",
			Stdin: strings.NewReader("á\nÁ\n"),
			Args:  []string{"-i"},
			Want:  "á\nÁ\n",
		},
		{
			Name:  "with counts",
			Stdin: strings.NewReader("a\nA\nb\n"),
			Args:  []string{"-i", "-c"},
			Want:  "      2 a\n      1 b\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunAllRepeated(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "every line of a repeated group is written",
			Stdin: strings.NewReader("a\na\nb\nb\nb\nc\na\n"),
			Args:  []string{"-D"},
			Want:  "a\na\nb\nb\nb\n",
		},
		{
			Name:  "no repeated lines",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-D"},
			Want:  "",
		},
		{
			Name:  "none delimits nothing",
			Stdin: strings.NewReader("a\na\nb\nb\n"),
			Args:  []string{"--all-repeated=none"},
			Want:  "a\na\nb\nb\n",
		},
		{
			Name:  "separate delimits between groups",
			Stdin: strings.NewReader("a\na\nb\nb\n"),
			Args:  []string{"--all-repeated=separate"},
			Want:  "a\na\n\nb\nb\n",
		},
		{
			Name:  "separate delimits nothing before the first group",
			Stdin: strings.NewReader("x\na\na\nb\nb\n"),
			Args:  []string{"--all-repeated=separate"},
			Want:  "a\na\n\nb\nb\n",
		},
		{
			Name:  "prepend delimits before each group",
			Stdin: strings.NewReader("a\na\nb\nb\n"),
			Args:  []string{"--all-repeated=prepend"},
			Want:  "\na\na\n\nb\nb\n",
		},
		{
			Name:  "separate delimits with a NUL byte with z",
			Stdin: strings.NewReader("a\x00a\x00b\x00b\x00"),
			Args:  []string{"-z", "--all-repeated=separate"},
			Want:  "a\x00a\x00\x00b\x00b\x00",
		},
		{
			Name:  "a group without a final newline",
			Stdin: strings.NewReader("a\na"),
			Args:  []string{"-D"},
			Want:  "a\na",
		},
		{
			Name:  "with a skipped field",
			Stdin: strings.NewReader("1 a\n2 a\n3 b\n"),
			Args:  []string{"-D", "-f", "1"},
			Want:  "1 a\n2 a\n",
		},
		{
			Name:  "ignoring case",
			Stdin: strings.NewReader("a\nA\nb\nb\n"),
			Args:  []string{"-D", "-i"},
			Want:  "a\nA\nb\nb\n",
		},
		{
			Name:     "an invalid delimit method",
			Stdin:    strings.NewReader("a\na\n"),
			Args:     []string{"--all-repeated=x"},
			WantErr:  "uniq: invalid argument 'x' for '--all-repeated'\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "count is meaningless with all repeated",
			Stdin:    strings.NewReader("a\na\n"),
			Args:     []string{"-c", "-D"},
			WantErr:  "uniq: printing all duplicated lines and repeat counts is meaningless\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unique is not supported with all repeated",
			Stdin:    strings.NewReader("a\na\n"),
			Args:     []string{"-u", "-D"},
			WantErr:  "uniq: printing all duplicated and unique lines is not supported\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunCheckChars(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "only the first characters are compared",
			Stdin: strings.NewReader("apple\napricot\nbanana\n"),
			Args:  []string{"-w", "2"},
			Want:  "apple\nbanana\n",
		},
		{
			Name:  "zero characters make every line equal",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-w", "0"},
			Want:  "a\n",
		},
		{
			Name:  "more characters than the line has",
			Stdin: strings.NewReader("a\nab\n"),
			Args:  []string{"-w", "5"},
			Want:  "a\nab\n",
		},
		{
			Name:  "characters are compared after the skipped ones",
			Stdin: strings.NewReader("xxab\nxxac\nxxb\n"),
			Args:  []string{"-s", "2", "-w", "1"},
			Want:  "xxab\nxxb\n",
		},
		{
			Name:  "characters are compared after the skipped fields",
			Stdin: strings.NewReader("1 ax\n2 ay\n"),
			Args:  []string{"-f", "1", "-w", "1"},
			Want:  "1 ax\n",
		},
		{
			Name:  "characters are characters, not bytes",
			Stdin: strings.NewReader("áa\náb\n"),
			Args:  []string{"-w", "1"},
			Want:  "áa\n",
		},
		{
			Name:  "ignoring case",
			Stdin: strings.NewReader("aB\nAb\n"),
			Args:  []string{"-w", "2", "-i"},
			Want:  "aB\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunZeroTerminated(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "items are delimited by a NUL byte",
			Stdin: strings.NewReader("a\x00a\x00b\x00"),
			Args:  []string{"-z"},
			Want:  "a\x00b\x00",
		},
		{
			Name:  "an item without a final NUL is kept as is",
			Stdin: strings.NewReader("a\x00a"),
			Args:  []string{"-z"},
			Want:  "a\x00",
		},
		{
			Name:  "newlines are part of the items",
			Stdin: strings.NewReader("a\nb\x00a\nb\x00"),
			Args:  []string{"-z"},
			Want:  "a\nb\x00",
		},
		{
			Name:  "counts are written before the items",
			Stdin: strings.NewReader("a\x00a\x00b\x00"),
			Args:  []string{"-z", "-c"},
			Want:  "      2 a\x00      1 b\x00",
		},
		{
			Name:  "only repeated items",
			Stdin: strings.NewReader("a\x00a\x00b\x00"),
			Args:  []string{"-z", "-d"},
			Want:  "a\x00",
		},
		{
			Name:  "a newline separates fields",
			Stdin: strings.NewReader("a\nx\x00b\nx\x00"),
			Args:  []string{"-z", "-f", "1"},
			Want:  "a\nx\x00",
		},
		{
			Name:  "an empty standard input",
			Stdin: strings.NewReader(""),
			Args:  []string{"-z"},
			Want:  "",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOperands(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "an input file",
			FS:   testFS(),
			Args: []string{"dups.txt"},
			Want: "a\nb\nc\na\n",
		},
		{
			Name:      "an input and an output file",
			FS:        testFS(),
			Args:      []string{"dups.txt", "out.txt"},
			Want:      "",
			WantFiles: map[string]string{"out.txt": "a\nb\nc\na\n"},
		},
		{
			Name:      "the output file is truncated",
			FS:        testFS(),
			Args:      []string{"fields.txt", "out.txt"},
			WantFiles: map[string]string{"out.txt": "x 1\ny 1\nz 2\n"},
		},
		{
			Name:  "a dash input reads the standard input",
			Stdin: strings.NewReader("s\ns\n"),
			Args:  []string{"-"},
			Want:  "s\n",
		},
		{
			Name: "a dash output writes the standard output",
			FS:   testFS(),
			Args: []string{"dups.txt", "-"},
			Want: "a\nb\nc\na\n",
		},
		{
			Name:      "pathnames are resolved from the working directory",
			FS:        testFS(),
			Dir:       "sub",
			Args:      []string{"inner.txt", "out.txt"},
			WantFiles: map[string]string{"sub/out.txt": "i\n"},
		},
		{
			Name: "a file named like an option after a double dash",
			FS:   testFS(),
			Args: []string{"--", "-d"},
			Want: "p\n",
		},
		{
			Name:      "options are not parsed after the first operand",
			FS:        testFS(),
			Args:      []string{"dups.txt", "-c"},
			Want:      "",
			WantFiles: map[string]string{"-c": "a\nb\nc\na\n"},
		},
		{
			Name:     "an extra operand",
			FS:       testFS(),
			Args:     []string{"dups.txt", "out.txt", "third.txt"},
			WantErr:  "uniq: extra operand 'third.txt'\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a zero-length input operand",
			FS:       testFS(),
			Args:     []string{""},
			WantErr:  "uniq: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "a zero-length output operand",
			FS:       testFS(),
			Args:     []string{"dups.txt", ""},
			WantErr:  "uniq: invalid zero-length file name\n",
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
			WantErr:  "uniq: invalid option -- 'x'\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unknown long option",
			Args:     []string{"--foo"},
			WantErr:  "uniq: unrecognized option '--foo'\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "d and u are incompatible",
			Args:     []string{"-d", "-u"},
			WantErr:  "uniq: printing only duplicated and unique lines is not supported\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing field count argument",
			Args:     []string{"-f"},
			WantErr:  "uniq: option requires an argument -- 'f'\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a non numeric field count",
			Args:     []string{"-f", "x"},
			WantErr:  "uniq: invalid number of fields: 'x'\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative field count",
			Args:     []string{"-f", "-1"},
			WantErr:  "uniq: invalid number of fields: '-1'\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a non numeric character count",
			Args:     []string{"-s", "x"},
			WantErr:  "uniq: invalid number of characters: 'x'\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing count of characters to compare argument",
			Args:     []string{"-w"},
			WantErr:  "uniq: option requires an argument -- 'w'\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a non numeric count of characters to compare",
			Args:     []string{"-w", "x"},
			WantErr:  "uniq: invalid number of characters to compare: 'x'\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative count of characters to compare",
			Args:     []string{"-w", "-1"},
			WantErr:  "uniq: invalid number of characters to compare: '-1'\nTry 'uniq --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "count",
			Stdin: strings.NewReader("a\na\nb\n"),
			Args:  []string{"--count"},
			Want:  "      2 a\n      1 b\n",
		},
		{
			Name:  "repeated",
			Stdin: strings.NewReader("a\na\nb\n"),
			Args:  []string{"--repeated"},
			Want:  "a\n",
		},
		{
			Name:  "unique",
			Stdin: strings.NewReader("a\na\nb\n"),
			Args:  []string{"--unique"},
			Want:  "b\n",
		},
		{
			Name: "skip-fields",
			FS:   testFS(),
			Args: []string{"--skip-fields", "1", "fields.txt"},
			Want: "x 1\nz 2\n",
		},
		{
			Name: "skip-chars with an attached value",
			FS:   testFS(),
			Args: []string{"--skip-chars=1", "utf8.txt"},
			Want: "áx\náy\n",
		},
		{
			Name:  "ignore-case",
			Stdin: strings.NewReader("a\nA\n"),
			Args:  []string{"--ignore-case"},
			Want:  "a\n",
		},
		{
			Name:  "all-repeated",
			Stdin: strings.NewReader("a\na\nb\n"),
			Args:  []string{"--all-repeated"},
			Want:  "a\na\n",
		},
		{
			Name:  "check-chars with an attached value",
			Stdin: strings.NewReader("ab\nac\n"),
			Args:  []string{"--check-chars=1"},
			Want:  "ab\n",
		},
		{
			Name:  "zero-terminated",
			Stdin: strings.NewReader("a\x00a\x00"),
			Args:  []string{"--zero-terminated"},
			Want:  "a\x00",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "missing input file",
			FS:       testFS(),
			Args:     []string{"missing.txt"},
			WantErr:  "uniq: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "directory input",
			FS:       fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			Args:     []string{"dir"},
			WantErr:  "uniq: dir: invalid argument\n",
			WantExit: 1,
		},
		{
			Name:     "directory output",
			FS:       fsx.MemFS{"dups.txt": {Data: []byte("a\n")}, "dir": {Mode: fs.ModeDir}},
			Args:     []string{"dups.txt", "dir"},
			WantErr:  "uniq: dir: invalid argument\n",
			WantExit: 1,
		},
		{
			Name:     "output with a missing parent directory",
			FS:       testFS(),
			Args:     []string{"dups.txt", "no/such/out.txt"},
			WantErr:  "uniq: no/such/out.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "input file without a file system",
			Args:     []string{"dups.txt"},
			WantErr:  "uniq: dups.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			WantErr:  "uniq: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "standard input read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			WantErr:  "uniq: standard input: read failed\n",
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
	if got := stderr.String(); got != "uniq: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "uniq: write error: write failed\n")
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
	testutil.HelpVersion(t, New, "uniq", usage)
}
