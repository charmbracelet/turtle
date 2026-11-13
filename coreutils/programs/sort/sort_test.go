package sort

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
//	names.txt:     "zoe\neve\namy\n"
//	nums.txt:      "10\n2\n1\n"
//	ids.txt:       "b:2\na:10\nc:1\n"
//	sorted1.txt:   "a\nc\n"
//	sorted2.txt:   "b\nd\n"
//	bad.txt:       "b\na\n"
//	nonl.txt:      "b\na" (no trailing newline)
//	empty.txt:     ""
//	out.txt:       "old\n" (truncated by output tests)
//	"-n":          "3\n1\n" (a file named like an option)
//	sub/inner.txt: "b\na\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"names.txt":     {Data: []byte("zoe\neve\namy\n")},
		"nums.txt":      {Data: []byte("10\n2\n1\n")},
		"ids.txt":       {Data: []byte("b:2\na:10\nc:1\n")},
		"sorted1.txt":   {Data: []byte("a\nc\n")},
		"sorted2.txt":   {Data: []byte("b\nd\n")},
		"bad.txt":       {Data: []byte("b\na\n")},
		"nonl.txt":      {Data: []byte("b\na")},
		"empty.txt":     {Data: []byte("")},
		"out.txt":       {Data: []byte("old\n")},
		"-n":            {Data: []byte("3\n1\n")},
		"sub/inner.txt": {Data: []byte("b\na\n")},
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no operands read the standard input",
			Stdin: strings.NewReader("b\na\nc\n"),
			Want:  "a\nb\nc\n",
		},
		{
			Name:  "an already sorted input",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Want:  "a\nb\nc\n",
		},
		{
			Name:  "an empty standard input",
			Stdin: strings.NewReader(""),
			Want:  "",
		},
		{
			Name:  "an empty line sorts first",
			Stdin: strings.NewReader("b\n\na\n"),
			Want:  "\na\nb\n",
		},
		{
			Name:  "duplicate lines are kept",
			Stdin: strings.NewReader("b\na\na\n"),
			Want:  "a\na\nb\n",
		},
		{
			Name:  "the comparison is by byte",
			Stdin: strings.NewReader("Z\na\nB\n"),
			Want:  "B\nZ\na\n",
		},
		{
			Name:  "numbers sort as text",
			Stdin: strings.NewReader("10\n2\n"),
			Want:  "10\n2\n",
		},
		{
			Name:  "multibyte characters sort by byte",
			Stdin: strings.NewReader("á\nz\n"),
			Want:  "z\ná\n",
		},
		{
			Name:  "a missing final newline is added",
			Stdin: strings.NewReader("b\na"),
			Want:  "a\nb\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunFiles(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"names.txt"},
			Want: "amy\neve\nzoe\n",
		},
		{
			Name: "multiple files are sorted together",
			FS:   testFS(),
			Args: []string{"sorted2.txt", "sorted1.txt"},
			Want: "a\nb\nc\nd\n",
		},
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("b\na\n"),
			FS:    testFS(),
			Args:  []string{"-"},
			Want:  "a\nb\n",
		},
		{
			Name:  "dash between files reads the standard input at that point",
			Stdin: strings.NewReader("bb\n"),
			FS:    testFS(),
			Args:  []string{"sorted1.txt", "-", "sorted2.txt"},
			Want:  "a\nb\nbb\nc\nd\n",
		},
		{
			Name: "pathnames are resolved from the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"inner.txt"},
			Want: "a\nb\n",
		},
		{
			Name: "a file named like an option after a double dash",
			FS:   testFS(),
			Args: []string{"--", "-n"},
			Want: "1\n3\n",
		},
		{
			Name:     "options are not parsed after the first operand",
			FS:       testFS(),
			Args:     []string{"sorted1.txt", "-x"},
			Want:     "",
			WantErr:  "sort: -x: file does not exist\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunReverse(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the comparison is reversed",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-r"},
			Want:  "c\nb\na\n",
		},
		{
			Name:  "the fallback comparison is reversed",
			Stdin: strings.NewReader("a 1\nb 1\n"),
			Args:  []string{"-r", "-k", "2"},
			Want:  "b 1\na 1\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunNumeric(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "numbers sort by arithmetic value",
			Stdin: strings.NewReader("10\n2\n1\n"),
			Args:  []string{"-n"},
			Want:  "1\n2\n10\n",
		},
		{
			Name:  "negative numbers",
			Stdin: strings.NewReader("3\n-1\n-20\n2\n"),
			Args:  []string{"-n"},
			Want:  "-20\n-1\n2\n3\n",
		},
		{
			Name:  "decimal numbers",
			Stdin: strings.NewReader("1.5\n1.25\n"),
			Args:  []string{"-n"},
			Want:  "1.25\n1.5\n",
		},
		{
			Name:  "leading blanks are skipped",
			Stdin: strings.NewReader("  10\n 2\n"),
			Args:  []string{"-n"},
			Want:  " 2\n  10\n",
		},
		{
			Name:  "a key without digits is zero",
			Stdin: strings.NewReader("2\nabc\n"),
			Args:  []string{"-n"},
			Want:  "abc\n2\n",
		},
		{
			Name:  "zeros compare equal no matter the sign",
			Stdin: strings.NewReader("0\n-0\n"),
			Args:  []string{"-n"},
			Want:  "-0\n0\n",
		},
		{
			Name:  "equal values fall back to the whole lines",
			Stdin: strings.NewReader("1.0\n1\n"),
			Args:  []string{"-n"},
			Want:  "1\n1.0\n",
		},
		{
			Name:  "huge numbers compare exactly",
			Stdin: strings.NewReader("100000000000000000000\n99999999999999999999\n"),
			Args:  []string{"-n"},
			Want:  "99999999999999999999\n100000000000000000000\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunHumanNumeric(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "sizes sort by unit and then by value",
			Stdin: strings.NewReader("1023M\n1G\n1K\n2K\n"),
			Args:  []string{"-h"},
			Want:  "1K\n2K\n1023M\n1G\n",
		},
		{
			Name:  "a plain number sorts before a suffixed one",
			Stdin: strings.NewReader("2000\n1K\n"),
			Args:  []string{"-h"},
			Want:  "2000\n1K\n",
		},
		{
			Name:  "the suffixes are case insensitive",
			Stdin: strings.NewReader("2m\n1M\n5M\n1g\n"),
			Args:  []string{"-h"},
			Want:  "1M\n2m\n5M\n1g\n",
		},
		{
			Name:  "decimal values are compared",
			Stdin: strings.NewReader("1.5\n1.25\n"),
			Args:  []string{"-h"},
			Want:  "1.25\n1.5\n",
		},
		{
			Name:  "leading blanks are skipped",
			Stdin: strings.NewReader("  10\n2\n"),
			Args:  []string{"-h"},
			Want:  "2\n  10\n",
		},
		{
			Name:  "a negative number sorts by its negated unit order",
			Stdin: strings.NewReader("-1K\n5\n"),
			Args:  []string{"-h"},
			Want:  "-1K\n5\n",
		},
		{
			Name:  "a zero number has no unit",
			Stdin: strings.NewReader("0K\n5\n1K\n"),
			Args:  []string{"-h"},
			Want:  "0K\n5\n1K\n",
		},
		{
			Name:  "missing and invalid numbers sort as zero",
			Stdin: strings.NewReader("abc\n0\n"),
			Args:  []string{"-h"},
			Want:  "0\nabc\n",
		},
		{
			Name:  "a human numeric key",
			Stdin: strings.NewReader("a 1K\nb 2\n"),
			Args:  []string{"-k", "2h"},
			Want:  "b 2\na 1K\n",
		},
		{
			Name:  "a key inherits the global human numeric ordering",
			Stdin: strings.NewReader("a 1K\nb 2\n"),
			Args:  []string{"-h", "-k", "2"},
			Want:  "b 2\na 1K\n",
		},
		{
			Name:  "the sort=human-numeric long form",
			Stdin: strings.NewReader("10\n2\n"),
			Args:  []string{"--sort=human-numeric"},
			Want:  "2\n10\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunMonthSort(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "months compare in calendar order",
			Stdin: strings.NewReader("MAR\nJAN\nFEB\n"),
			Args:  []string{"-M"},
			Want:  "JAN\nFEB\nMAR\n",
		},
		{
			Name:  "the comparison is case insensitive",
			Stdin: strings.NewReader("jan\nFeb\nDEC\nMAR\n"),
			Args:  []string{"-M"},
			Want:  "jan\nFeb\nMAR\nDEC\n",
		},
		{
			Name:  "only the month abbreviation is significant",
			Stdin: strings.NewReader("MARCH\nJANUARY\nAPR\n"),
			Args:  []string{"-M"},
			Want:  "JANUARY\nMARCH\nAPR\n",
		},
		{
			Name:  "unknown months sort low to known ones",
			Stdin: strings.NewReader("xyz\nJAN\n"),
			Args:  []string{"-M"},
			Want:  "xyz\nJAN\n",
		},
		{
			Name:  "equal unknown months fall back to the whole lines",
			Stdin: strings.NewReader("zzz\naaa\nJAN\n"),
			Args:  []string{"-M"},
			Want:  "aaa\nzzz\nJAN\n",
		},
		{
			Name:  "blanks before the month are skipped",
			Stdin: strings.NewReader("  FEB\nJAN\n"),
			Args:  []string{"-M"},
			Want:  "JAN\n  FEB\n",
		},
		{
			Name:  "a month key",
			Stdin: strings.NewReader("a MAR\nb JAN\n"),
			Args:  []string{"-k", "2,2M"},
			Want:  "b JAN\na MAR\n",
		},
		{
			Name:  "the sort=month long form",
			Stdin: strings.NewReader("MAR\nJAN\n"),
			Args:  []string{"--sort=month"},
			Want:  "JAN\nMAR\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunVersionSort(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "digits compare numerically",
			Stdin: strings.NewReader("1.10\n1.2\n1\n"),
			Args:  []string{"-V"},
			Want:  "1\n1.2\n1.10\n",
		},
		{
			Name:  "a digit run longer than the other wins",
			Stdin: strings.NewReader("10\n9\n"),
			Args:  []string{"-V"},
			Want:  "9\n10\n",
		},
		{
			Name:  "letters sort before other bytes",
			Stdin: strings.NewReader("foo07.7z\nfoo7a.7z\n"),
			Args:  []string{"-V"},
			Want:  "foo7a.7z\nfoo07.7z\n",
		},
		{
			Name:  "a tilde sorts before anything",
			Stdin: strings.NewReader("1.0~rc1\n1.0\n"),
			Args:  []string{"-V"},
			Want:  "1.0~rc1\n1.0\n",
		},
		{
			Name:  "equal numeric values fall back to the whole lines",
			Stdin: strings.NewReader("00123\n123\n"),
			Args:  []string{"-V"},
			Want:  "00123\n123\n",
		},
		{
			Name:  "the empty string counts as zero",
			Stdin: strings.NewReader("0\n\n"),
			Args:  []string{"-V"},
			Want:  "\n0\n",
		},
		{
			Name:  "a version key",
			Stdin: strings.NewReader("a 1.10\nb 1.2\n"),
			Args:  []string{"-k", "2V"},
			Want:  "b 1.2\na 1.10\n",
		},
		{
			Name:  "the sort=version long form",
			Stdin: strings.NewReader("1.10\n1.2\n"),
			Args:  []string{"--sort=version"},
			Want:  "1.2\n1.10\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunRandomSort(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the permutation is deterministic",
			Stdin: strings.NewReader("b\na\nd\nc\n"),
			Args:  []string{"-R"},
			Want:  "d\nb\nc\na\n",
		},
		{
			Name:  "a longer permutation",
			Stdin: strings.NewReader("a\nb\nc\nd\ne\nf\ng\nh\n"),
			Args:  []string{"-R"},
			Want:  "f\ng\nd\ne\nb\nc\na\nh\n",
		},
		{
			Name:  "equal keys sort together",
			Stdin: strings.NewReader("b\na\nb\n"),
			Args:  []string{"-R"},
			Want:  "b\nb\na\n",
		},
		{
			Name:  "a random key",
			Stdin: strings.NewReader("1 b\n2 a\n"),
			Args:  []string{"-k", "2,2R"},
			Want:  "1 b\n2 a\n",
		},
		{
			Name:  "the sort=random long form",
			Stdin: strings.NewReader("b\na\nd\nc\n"),
			Args:  []string{"--sort=random"},
			Want:  "d\nb\nc\na\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunDictionary(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "only blanks and alphanumerics are significant",
			Stdin: strings.NewReader("a!z\nab\n"),
			Args:  []string{"-d"},
			Want:  "ab\na!z\n",
		},
		{
			Name:  "without d the punctuation is significant",
			Stdin: strings.NewReader("a!z\nab\n"),
			Want:  "a!z\nab\n",
		},
		{
			Name:  "blanks are significant",
			Stdin: strings.NewReader("ab\na b\n"),
			Args:  []string{"-d"},
			Want:  "a b\nab\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunFoldCase(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "lowercase folds to uppercase",
			Stdin: strings.NewReader("b\nA\n"),
			Args:  []string{"-f"},
			Want:  "A\nb\n",
		},
		{
			Name:  "equal folded lines fall back to the whole lines",
			Stdin: strings.NewReader("a\nA\n"),
			Args:  []string{"-f"},
			Want:  "A\na\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunIgnoreNonPrint(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "non printable characters are ignored",
			Stdin: strings.NewReader("a\tx\nab\n"),
			Args:  []string{"-i"},
			Want:  "ab\na\tx\n",
		},
		{
			Name:  "without i the tab is significant",
			Stdin: strings.NewReader("a\tx\nab\n"),
			Want:  "a\tx\nab\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunKeys(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the second field as the key",
			Stdin: strings.NewReader("z b\ny a\n"),
			Args:  []string{"-k", "2"},
			Want:  "y a\nz b\n",
		},
		{
			Name:  "a restricted key",
			Stdin: strings.NewReader("z b\ny a\n"),
			Args:  []string{"-k", "2,2"},
			Want:  "y a\nz b\n",
		},
		{
			Name:  "the field separator is part of the field",
			Stdin: strings.NewReader("y a\nz  b\n"),
			Args:  []string{"-k", "2.2"},
			Want:  "z  b\ny a\n",
		},
		{
			Name:  "leading blanks are ignored with the global b",
			Stdin: strings.NewReader("y a\nz  b\n"),
			Args:  []string{"-b", "-k", "2"},
			Want:  "y a\nz  b\n",
		},
		{
			Name:  "leading blanks are ignored with an attached b",
			Stdin: strings.NewReader("y a\nz  b\n"),
			Args:  []string{"-k", "2b"},
			Want:  "y a\nz  b\n",
		},
		{
			Name: "a character separator",
			FS:   testFS(),
			Args: []string{"-t", ":", "-k", "2,2", "ids.txt"},
			Want: "c:1\na:10\nb:2\n",
		},
		{
			Name:  "empty fields with a character separator",
			Stdin: strings.NewReader("a:\n:b\n"),
			Args:  []string{"-t", ":", "-k", "1,1"},
			Want:  ":b\na:\n",
		},
		{
			Name:  "a key past the end of the line is empty",
			Stdin: strings.NewReader("b 1\na\n"),
			Args:  []string{"-k", "2"},
			Want:  "a\nb 1\n",
		},
		{
			Name:  "multiple keys break ties in order",
			Stdin: strings.NewReader("a 2\nb 1\na 1\n"),
			Args:  []string{"-k", "2,2", "-k", "1,1"},
			Want:  "a 1\nb 1\na 2\n",
		},
		{
			Name:  "a numeric key",
			Stdin: strings.NewReader("a 10\nb 2\n"),
			Args:  []string{"-k", "2n"},
			Want:  "b 2\na 10\n",
		},
		{
			Name:  "a reversed key",
			Stdin: strings.NewReader("a 1\nb 2\n"),
			Args:  []string{"-k", "2r"},
			Want:  "b 2\na 1\n",
		},
		{
			Name:  "a key inherits the global ordering",
			Stdin: strings.NewReader("a 10\nb 2\n"),
			Args:  []string{"-n", "-k", "2"},
			Want:  "b 2\na 10\n",
		},
		{
			Name:  "a key with modifiers doesn't inherit the global ordering",
			Stdin: strings.NewReader("a 1\nb 2\n"),
			Args:  []string{"-r", "-k", "2n,2"},
			Want:  "a 1\nb 2\n",
		},
		{
			Name:  "character positions",
			Stdin: strings.NewReader("xb\nya\n"),
			Args:  []string{"-k", "1.2,1.2"},
			Want:  "ya\nxb\n",
		},
		{
			Name:  "a key end past the end of the field",
			Stdin: strings.NewReader("ab ef\nab cd\n"),
			Args:  []string{"-k", "1.1,1.5"},
			Want:  "ab cd\nab ef\n",
		},
		{
			Name:  "a zero end character is the end of the field",
			Stdin: strings.NewReader("z bq\ny ap\n"),
			Args:  []string{"-k", "2.1,2.0"},
			Want:  "y ap\nz bq\n",
		},
		{
			Name:  "a key start past the key end is empty",
			Stdin: strings.NewReader("b\na\n"),
			Args:  []string{"-k", "2,1"},
			Want:  "a\nb\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunUnique(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "only one line of each set of equal lines",
			Stdin: strings.NewReader("b\na\na\n"),
			Args:  []string{"-u"},
			Want:  "a\nb\n",
		},
		{
			Name:  "lines with equal keys, not equal lines",
			Stdin: strings.NewReader("x 1\ny 1\nz 2\n"),
			Args:  []string{"-u", "-k", "2"},
			Want:  "x 1\nz 2\n",
		},
		{
			Name:  "keys folded by f keep the first of each set",
			Stdin: strings.NewReader("a\nA\nb\n"),
			Args:  []string{"-u", "-f"},
			Want:  "a\nb\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunStable(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "lines with equal keys keep their input order",
			Stdin: strings.NewReader("b 2\na 2\n"),
			Args:  []string{"-s", "-k", "2,2n"},
			Want:  "b 2\na 2\n",
		},
		{
			Name:  "without s the last resort comparison applies",
			Stdin: strings.NewReader("b 2\na 2\n"),
			Args:  []string{"-k", "2,2n"},
			Want:  "a 2\nb 2\n",
		},
		{
			Name:  "the whole line is still the key without keys",
			Stdin: strings.NewReader("b\na\n"),
			Args:  []string{"-s"},
			Want:  "a\nb\n",
		},
		{
			Name:  "the random ordering can be made stable",
			Stdin: strings.NewReader("1 b\n2 a\n1 a\n"),
			Args:  []string{"-R", "-s", "-k", "1,1"},
			Want:  "2 a\n1 b\n1 a\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunZeroTerminated(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "items are NUL terminated",
			Stdin: strings.NewReader("b\x00a\x00"),
			Args:  []string{"-z"},
			Want:  "a\x00b\x00",
		},
		{
			Name:  "a missing final NUL is added",
			Stdin: strings.NewReader("b\x00a"),
			Args:  []string{"-z"},
			Want:  "a\x00b\x00",
		},
		{
			Name:  "items can contain newlines",
			Stdin: strings.NewReader("b x\ny\x00a\x00"),
			Args:  []string{"-z"},
			Want:  "a\x00b x\ny\x00",
		},
		{
			Name:  "a sorted input passes the check",
			Stdin: strings.NewReader("a\x00b\x00"),
			Args:  []string{"-z", "-c"},
		},
		{
			Name:     "a disordered input fails the check",
			Stdin:    strings.NewReader("b\x00a\x00"),
			Args:     []string{"-z", "-c"},
			WantErr:  "sort: -:2: disorder: a\n",
			WantExit: 1,
		},
		{
			Name:      "the output goes to a file",
			Stdin:     strings.NewReader("b\x00a\x00"),
			FS:        testFS(),
			Args:      []string{"-z", "-o", "out.txt"},
			WantFiles: map[string]string{"out.txt": "a\x00b\x00"},
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunCheck(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a sorted input",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-c"},
		},
		{
			Name:     "a disordered input",
			Stdin:    strings.NewReader("b\na\n"),
			Args:     []string{"-c"},
			WantErr:  "sort: -:2: disorder: a\n",
			WantExit: 1,
		},
		{
			Name:     "a disordered file",
			FS:       testFS(),
			Args:     []string{"-c", "bad.txt"},
			WantErr:  "sort: bad.txt:2: disorder: a\n",
			WantExit: 1,
		},
		{
			Name:     "a silent check",
			Stdin:    strings.NewReader("b\na\n"),
			Args:     []string{"-C"},
			WantExit: 1,
		},
		{
			Name:     "a check with the numeric ordering",
			Stdin:    strings.NewReader("10\n2\n"),
			Args:     []string{"-c", "-n"},
			WantErr:  "sort: -:2: disorder: 2\n",
			WantExit: 1,
		},
		{
			Name:     "a duplicate key is a disorder with u",
			Stdin:    strings.NewReader("a\na\n"),
			Args:     []string{"-c", "-u"},
			WantErr:  "sort: -:2: disorder: a\n",
			WantExit: 1,
		},
		{
			Name:  "duplicate lines are ordered without u",
			Stdin: strings.NewReader("a\na\n"),
			Args:  []string{"-c"},
		},
		{
			Name:     "an extra operand",
			FS:       testFS(),
			Args:     []string{"-c", "sorted1.txt", "sorted2.txt"},
			WantErr:  "sort: extra operand 'sorted2.txt' not allowed with -c\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "c and m are incompatible",
			FS:       testFS(),
			Args:     []string{"-c", "-m", "sorted1.txt"},
			WantErr:  "sort: options -c and -m are incompatible\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a missing file",
			FS:       testFS(),
			Args:     []string{"-c", "missing.txt"},
			WantErr:  "sort: missing.txt: file does not exist\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunMerge(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "presorted files are merged",
			FS:   testFS(),
			Args: []string{"-m", "sorted1.txt", "sorted2.txt"},
			Want: "a\nb\nc\nd\n",
		},
		{
			Name: "merged lines keep duplicates",
			FS: fsx.MemFS{
				"one.txt": {Data: []byte("a\nb\n")},
				"two.txt": {Data: []byte("b\nc\n")},
			},
			Args: []string{"-m", "one.txt", "two.txt"},
			Want: "a\nb\nb\nc\n",
		},
		{
			Name: "a merged unique",
			FS: fsx.MemFS{
				"one.txt": {Data: []byte("a\nb\n")},
				"two.txt": {Data: []byte("b\nc\n")},
			},
			Args: []string{"-m", "-u", "one.txt", "two.txt"},
			Want: "a\nb\nc\n",
		},
		{
			Name:  "merge from the standard input",
			Stdin: strings.NewReader("a\nc\n"),
			FS:    testFS(),
			Args:  []string{"-m", "-", "sorted2.txt"},
			Want:  "a\nb\nc\nd\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOutput(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:      "the output goes to a file",
			Stdin:     strings.NewReader("b\na\n"),
			FS:        testFS(),
			Args:      []string{"-o", "out.txt"},
			WantFiles: map[string]string{"out.txt": "a\nb\n"},
		},
		{
			Name:      "the output file can be the input file",
			FS:        testFS(),
			Args:      []string{"-o", "bad.txt", "bad.txt"},
			WantFiles: map[string]string{"bad.txt": "a\nb\n"},
		},
		{
			Name:      "the output file is truncated",
			FS:        testFS(),
			Args:      []string{"-o", "out.txt", "sorted1.txt"},
			WantFiles: map[string]string{"out.txt": "a\nc\n"},
		},
		{
			Name:     "an output with a missing parent directory",
			Stdin:    strings.NewReader("a\n"),
			FS:       testFS(),
			Args:     []string{"-o", "no/such/out.txt"},
			WantErr:  "sort: no/such/out.txt: file does not exist\n",
			WantExit: 2,
		},
		{
			Name:     "a zero-length output name",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-o", ""},
			WantErr:  "sort: invalid zero-length file name\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "invalid option",
			Args:     []string{"-x"},
			WantErr:  "sort: invalid option -- 'x'\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unknown long option",
			Args:     []string{"--foo"},
			WantErr:  "sort: unrecognized option '--foo'\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing key argument",
			Args:     []string{"-k"},
			WantErr:  "sort: option requires an argument -- 'k'\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing output argument",
			Args:     []string{"-o"},
			WantErr:  "sort: option requires an argument -- 'o'\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing separator argument",
			Args:     []string{"-t"},
			WantErr:  "sort: option requires an argument -- 't'\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a separator of two characters",
			Args:     []string{"-t", "::"},
			WantErr:  "sort: the separator must be a single character\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty key definition",
			Args:     []string{"-k", ""},
			WantErr:  "sort: invalid key definition: ''\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a zero field",
			Args:     []string{"-k", "0"},
			WantErr:  "sort: invalid key definition: '0'\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a zero start character",
			Args:     []string{"-k", "1.0"},
			WantErr:  "sort: invalid key definition: '1.0'\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a non numeric field",
			Args:     []string{"-k", "x"},
			WantErr:  "sort: invalid key definition: 'x'\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown key modifier",
			Args:     []string{"-k", "1x"},
			WantErr:  "sort: invalid key definition: '1x'\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a zero end field",
			Args:     []string{"-k", "1,0"},
			WantErr:  "sort: invalid key definition: '1,0'\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty key end",
			Args:     []string{"-k", "1,"},
			WantErr:  "sort: invalid key definition: '1,'\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid sort word",
			Args:     []string{"--sort=bogus"},
			WantErr:  "sort: invalid argument 'bogus' for '--sort'\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a missing sort argument",
			Args:     []string{"--sort"},
			WantErr:  "sort: option '--sort' requires an argument\nTry 'sort --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "reverse",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"--reverse"},
			Want:  "b\na\n",
		},
		{
			Name:  "numeric-sort",
			Stdin: strings.NewReader("10\n2\n"),
			Args:  []string{"--numeric-sort"},
			Want:  "2\n10\n",
		},
		{
			Name:  "sort word",
			Stdin: strings.NewReader("10\n2\n"),
			Args:  []string{"--sort=numeric"},
			Want:  "2\n10\n",
		},
		{
			Name:  "unique",
			Stdin: strings.NewReader("a\na\nb\n"),
			Args:  []string{"--unique"},
			Want:  "a\nb\n",
		},
		{
			Name:  "key",
			Stdin: strings.NewReader("z b\ny a\n"),
			Args:  []string{"--key", "2"},
			Want:  "y a\nz b\n",
		},
		{
			Name: "field-separator",
			FS:   testFS(),
			Args: []string{"--field-separator", ":", "--key=2,2", "ids.txt"},
			Want: "c:1\na:10\nb:2\n",
		},
		{
			Name:     "check",
			Stdin:    strings.NewReader("b\na\n"),
			Args:     []string{"--check"},
			WantErr:  "sort: -:2: disorder: a\n",
			WantExit: 1,
		},
		{
			Name: "merge",
			FS:   testFS(),
			Args: []string{"--merge", "sorted1.txt", "sorted2.txt"},
			Want: "a\nb\nc\nd\n",
		},
		{
			Name:      "output with an attached value",
			Stdin:     strings.NewReader("b\na\n"),
			FS:        testFS(),
			Args:      []string{"--output=out.txt"},
			WantFiles: map[string]string{"out.txt": "a\nb\n"},
		},
		{
			Name:  "ignore-leading-blanks",
			Stdin: strings.NewReader("y a\nz  b\n"),
			Args:  []string{"--ignore-leading-blanks", "--key", "2"},
			Want:  "y a\nz  b\n",
		},
		{
			Name:  "dictionary-order",
			Stdin: strings.NewReader("a!z\nab\n"),
			Args:  []string{"--dictionary-order"},
			Want:  "ab\na!z\n",
		},
		{
			Name:  "ignore-case",
			Stdin: strings.NewReader("b\nA\n"),
			Args:  []string{"--ignore-case"},
			Want:  "A\nb\n",
		},
		{
			Name:  "ignore-nonprinting",
			Stdin: strings.NewReader("a\tx\nab\n"),
			Args:  []string{"--ignore-nonprinting"},
			Want:  "ab\na\tx\n",
		},
		{
			Name:  "human-numeric-sort",
			Stdin: strings.NewReader("10\n2\n"),
			Args:  []string{"--human-numeric-sort"},
			Want:  "2\n10\n",
		},
		{
			Name:  "month-sort",
			Stdin: strings.NewReader("MAR\nJAN\n"),
			Args:  []string{"--month-sort"},
			Want:  "JAN\nMAR\n",
		},
		{
			Name:  "version-sort",
			Stdin: strings.NewReader("1.10\n1.2\n"),
			Args:  []string{"--version-sort"},
			Want:  "1.2\n1.10\n",
		},
		{
			Name:  "random-sort",
			Stdin: strings.NewReader("b\na\nd\nc\n"),
			Args:  []string{"--random-sort"},
			Want:  "d\nb\nc\na\n",
		},
		{
			Name:  "zero-terminated",
			Stdin: strings.NewReader("b\x00a\x00"),
			Args:  []string{"--zero-terminated"},
			Want:  "a\x00b\x00",
		},
		{
			Name:  "stable",
			Stdin: strings.NewReader("b 2\na 2\n"),
			Args:  []string{"--stable", "-k", "2,2n"},
			Want:  "b 2\na 2\n",
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
			WantErr:  "sort: missing.txt: file does not exist\n",
			WantExit: 2,
		},
		{
			Name:     "directory operand",
			FS:       fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			Args:     []string{"dir"},
			WantErr:  "sort: dir: invalid argument\n",
			WantExit: 2,
		},
		{
			Name:     "zero-length file name",
			FS:       testFS(),
			Args:     []string{""},
			WantErr:  "sort: invalid zero-length file name\n",
			WantExit: 2,
		},
		{
			Name:     "file operand without a file system",
			Args:     []string{"names.txt"},
			WantErr:  "sort: names.txt: no file system available\n",
			WantExit: 2,
		},
		{
			Name:     "no standard input",
			WantErr:  "sort: standard input: no standard input available\n",
			WantExit: 2,
		},
		{
			Name:     "standard input read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			WantErr:  "sort: standard input: read failed\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("b\na\n"),
		Stdout: testutil.ErrorWriter{Err: writeErr},
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context())

	if code := testutil.ExitCode(t, err); code != 2 {
		t.Fatalf("Run() exit code = %d, want 2 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
	if got := stderr.String(); got != "sort: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "sort: write error: write failed\n")
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		testutil.Cancelled(t, New, strings.NewReader("b\na\n"), nil)
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		testutil.Cancelled(t, New, &testutil.CancelReader{}, nil)
	})
}

func TestRunSortTypeLastWins(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a later month sort replaces the numeric one",
			Stdin: strings.NewReader("2\n10\n1\n"),
			Args:  []string{"-n", "-M"},
			Want:  "1\n10\n2\n",
		},
		{
			Name:  "a later numeric sort replaces the month one",
			Stdin: strings.NewReader("2\n10\n1\n"),
			Args:  []string{"-M", "-n"},
			Want:  "1\n2\n10\n",
		},
		{
			Name:  "the long forms replace each other too",
			Stdin: strings.NewReader("2\n10\n1\n"),
			Args:  []string{"--sort=numeric", "--sort=month"},
			Want:  "1\n10\n2\n",
		},
		{
			Name:  "a key modifier replaces the global type",
			Stdin: strings.NewReader("2\n10\n1\n"),
			Args:  []string{"-M", "-k1,1n"},
			Want:  "1\n2\n10\n",
		},
	}
	testutil.Run(t, New, tests)
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "sort", usage)
}
