package sort

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
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
	name      string
	stdin     io.Reader
	fsys      fsx.MemFS
	dir       string
	args      []string
	want      string
	wantErr   string
	wantExit  int
	wantFiles map[string]string
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      test.stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		WorkingDir: test.dir,
	}
	if test.fsys != nil {
		rt.FS = test.fsys
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
	for name, want := range test.wantFiles {
		f := test.fsys[name]
		if f == nil {
			t.Errorf("Run() didn't create file %q", name)
			continue
		}
		if got := string(f.Data); got != want {
			t.Errorf("Run() file %q = %q, want %q", name, got, want)
		}
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			name:  "no operands read the standard input",
			stdin: strings.NewReader("b\na\nc\n"),
			want:  "a\nb\nc\n",
		},
		{
			name:  "an already sorted input",
			stdin: strings.NewReader("a\nb\nc\n"),
			want:  "a\nb\nc\n",
		},
		{
			name:  "an empty standard input",
			stdin: strings.NewReader(""),
			want:  "",
		},
		{
			name:  "an empty line sorts first",
			stdin: strings.NewReader("b\n\na\n"),
			want:  "\na\nb\n",
		},
		{
			name:  "duplicate lines are kept",
			stdin: strings.NewReader("b\na\na\n"),
			want:  "a\na\nb\n",
		},
		{
			name:  "the comparison is by byte",
			stdin: strings.NewReader("Z\na\nB\n"),
			want:  "B\nZ\na\n",
		},
		{
			name:  "numbers sort as text",
			stdin: strings.NewReader("10\n2\n"),
			want:  "10\n2\n",
		},
		{
			name:  "multibyte characters sort by byte",
			stdin: strings.NewReader("á\nz\n"),
			want:  "z\ná\n",
		},
		{
			name:  "a missing final newline is added",
			stdin: strings.NewReader("b\na"),
			want:  "a\nb\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFiles(t *testing.T) {
	tests := []testCase{
		{
			name: "a file operand",
			fsys: testFS(),
			args: []string{"names.txt"},
			want: "amy\neve\nzoe\n",
		},
		{
			name: "multiple files are sorted together",
			fsys: testFS(),
			args: []string{"sorted2.txt", "sorted1.txt"},
			want: "a\nb\nc\nd\n",
		},
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("b\na\n"),
			fsys:  testFS(),
			args:  []string{"-"},
			want:  "a\nb\n",
		},
		{
			name:  "dash between files reads the standard input at that point",
			stdin: strings.NewReader("bb\n"),
			fsys:  testFS(),
			args:  []string{"sorted1.txt", "-", "sorted2.txt"},
			want:  "a\nb\nbb\nc\nd\n",
		},
		{
			name: "pathnames are resolved from the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt"},
			want: "a\nb\n",
		},
		{
			name: "a file named like an option after a double dash",
			fsys: testFS(),
			args: []string{"--", "-n"},
			want: "1\n3\n",
		},
		{
			name:     "options are not parsed after the first operand",
			fsys:     testFS(),
			args:     []string{"sorted1.txt", "-x"},
			want:     "",
			wantErr:  "sort: -x: file does not exist\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunReverse(t *testing.T) {
	tests := []testCase{
		{
			name:  "the comparison is reversed",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-r"},
			want:  "c\nb\na\n",
		},
		{
			name:  "the fallback comparison is reversed",
			stdin: strings.NewReader("a 1\nb 1\n"),
			args:  []string{"-r", "-k", "2"},
			want:  "b 1\na 1\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunNumeric(t *testing.T) {
	tests := []testCase{
		{
			name:  "numbers sort by arithmetic value",
			stdin: strings.NewReader("10\n2\n1\n"),
			args:  []string{"-n"},
			want:  "1\n2\n10\n",
		},
		{
			name:  "negative numbers",
			stdin: strings.NewReader("3\n-1\n-20\n2\n"),
			args:  []string{"-n"},
			want:  "-20\n-1\n2\n3\n",
		},
		{
			name:  "decimal numbers",
			stdin: strings.NewReader("1.5\n1.25\n"),
			args:  []string{"-n"},
			want:  "1.25\n1.5\n",
		},
		{
			name:  "leading blanks are skipped",
			stdin: strings.NewReader("  10\n 2\n"),
			args:  []string{"-n"},
			want:  " 2\n  10\n",
		},
		{
			name:  "a key without digits is zero",
			stdin: strings.NewReader("2\nabc\n"),
			args:  []string{"-n"},
			want:  "abc\n2\n",
		},
		{
			name:  "zeros compare equal no matter the sign",
			stdin: strings.NewReader("0\n-0\n"),
			args:  []string{"-n"},
			want:  "-0\n0\n",
		},
		{
			name:  "equal values fall back to the whole lines",
			stdin: strings.NewReader("1.0\n1\n"),
			args:  []string{"-n"},
			want:  "1\n1.0\n",
		},
		{
			name:  "huge numbers compare exactly",
			stdin: strings.NewReader("100000000000000000000\n99999999999999999999\n"),
			args:  []string{"-n"},
			want:  "99999999999999999999\n100000000000000000000\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunHumanNumeric(t *testing.T) {
	tests := []testCase{
		{
			name:  "sizes sort by unit and then by value",
			stdin: strings.NewReader("1023M\n1G\n1K\n2K\n"),
			args:  []string{"-h"},
			want:  "1K\n2K\n1023M\n1G\n",
		},
		{
			name:  "a plain number sorts before a suffixed one",
			stdin: strings.NewReader("2000\n1K\n"),
			args:  []string{"-h"},
			want:  "2000\n1K\n",
		},
		{
			name:  "the suffixes are case insensitive",
			stdin: strings.NewReader("2m\n1M\n5M\n1g\n"),
			args:  []string{"-h"},
			want:  "1M\n2m\n5M\n1g\n",
		},
		{
			name:  "decimal values are compared",
			stdin: strings.NewReader("1.5\n1.25\n"),
			args:  []string{"-h"},
			want:  "1.25\n1.5\n",
		},
		{
			name:  "leading blanks are skipped",
			stdin: strings.NewReader("  10\n2\n"),
			args:  []string{"-h"},
			want:  "2\n  10\n",
		},
		{
			name:  "a negative number sorts by its negated unit order",
			stdin: strings.NewReader("-1K\n5\n"),
			args:  []string{"-h"},
			want:  "-1K\n5\n",
		},
		{
			name:  "a zero number has no unit",
			stdin: strings.NewReader("0K\n5\n1K\n"),
			args:  []string{"-h"},
			want:  "0K\n5\n1K\n",
		},
		{
			name:  "missing and invalid numbers sort as zero",
			stdin: strings.NewReader("abc\n0\n"),
			args:  []string{"-h"},
			want:  "0\nabc\n",
		},
		{
			name:  "a human numeric key",
			stdin: strings.NewReader("a 1K\nb 2\n"),
			args:  []string{"-k", "2h"},
			want:  "b 2\na 1K\n",
		},
		{
			name:  "a key inherits the global human numeric ordering",
			stdin: strings.NewReader("a 1K\nb 2\n"),
			args:  []string{"-h", "-k", "2"},
			want:  "b 2\na 1K\n",
		},
		{
			name:  "the sort=human-numeric long form",
			stdin: strings.NewReader("10\n2\n"),
			args:  []string{"--sort=human-numeric"},
			want:  "2\n10\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunMonthSort(t *testing.T) {
	tests := []testCase{
		{
			name:  "months compare in calendar order",
			stdin: strings.NewReader("MAR\nJAN\nFEB\n"),
			args:  []string{"-M"},
			want:  "JAN\nFEB\nMAR\n",
		},
		{
			name:  "the comparison is case insensitive",
			stdin: strings.NewReader("jan\nFeb\nDEC\nMAR\n"),
			args:  []string{"-M"},
			want:  "jan\nFeb\nMAR\nDEC\n",
		},
		{
			name:  "only the month abbreviation is significant",
			stdin: strings.NewReader("MARCH\nJANUARY\nAPR\n"),
			args:  []string{"-M"},
			want:  "JANUARY\nMARCH\nAPR\n",
		},
		{
			name:  "unknown months sort low to known ones",
			stdin: strings.NewReader("xyz\nJAN\n"),
			args:  []string{"-M"},
			want:  "xyz\nJAN\n",
		},
		{
			name:  "equal unknown months fall back to the whole lines",
			stdin: strings.NewReader("zzz\naaa\nJAN\n"),
			args:  []string{"-M"},
			want:  "aaa\nzzz\nJAN\n",
		},
		{
			name:  "blanks before the month are skipped",
			stdin: strings.NewReader("  FEB\nJAN\n"),
			args:  []string{"-M"},
			want:  "JAN\n  FEB\n",
		},
		{
			name:  "a month key",
			stdin: strings.NewReader("a MAR\nb JAN\n"),
			args:  []string{"-k", "2,2M"},
			want:  "b JAN\na MAR\n",
		},
		{
			name:  "the sort=month long form",
			stdin: strings.NewReader("MAR\nJAN\n"),
			args:  []string{"--sort=month"},
			want:  "JAN\nMAR\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunVersionSort(t *testing.T) {
	tests := []testCase{
		{
			name:  "digits compare numerically",
			stdin: strings.NewReader("1.10\n1.2\n1\n"),
			args:  []string{"-V"},
			want:  "1\n1.2\n1.10\n",
		},
		{
			name:  "a digit run longer than the other wins",
			stdin: strings.NewReader("10\n9\n"),
			args:  []string{"-V"},
			want:  "9\n10\n",
		},
		{
			name:  "letters sort before other bytes",
			stdin: strings.NewReader("foo07.7z\nfoo7a.7z\n"),
			args:  []string{"-V"},
			want:  "foo7a.7z\nfoo07.7z\n",
		},
		{
			name:  "a tilde sorts before anything",
			stdin: strings.NewReader("1.0~rc1\n1.0\n"),
			args:  []string{"-V"},
			want:  "1.0~rc1\n1.0\n",
		},
		{
			name:  "equal numeric values fall back to the whole lines",
			stdin: strings.NewReader("00123\n123\n"),
			args:  []string{"-V"},
			want:  "00123\n123\n",
		},
		{
			name:  "the empty string counts as zero",
			stdin: strings.NewReader("0\n\n"),
			args:  []string{"-V"},
			want:  "\n0\n",
		},
		{
			name:  "a version key",
			stdin: strings.NewReader("a 1.10\nb 1.2\n"),
			args:  []string{"-k", "2V"},
			want:  "b 1.2\na 1.10\n",
		},
		{
			name:  "the sort=version long form",
			stdin: strings.NewReader("1.10\n1.2\n"),
			args:  []string{"--sort=version"},
			want:  "1.2\n1.10\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRandomSort(t *testing.T) {
	tests := []testCase{
		{
			name:  "the permutation is deterministic",
			stdin: strings.NewReader("b\na\nd\nc\n"),
			args:  []string{"-R"},
			want:  "d\nb\nc\na\n",
		},
		{
			name:  "a longer permutation",
			stdin: strings.NewReader("a\nb\nc\nd\ne\nf\ng\nh\n"),
			args:  []string{"-R"},
			want:  "f\ng\nd\ne\nb\nc\na\nh\n",
		},
		{
			name:  "equal keys sort together",
			stdin: strings.NewReader("b\na\nb\n"),
			args:  []string{"-R"},
			want:  "b\nb\na\n",
		},
		{
			name:  "a random key",
			stdin: strings.NewReader("1 b\n2 a\n"),
			args:  []string{"-k", "2,2R"},
			want:  "1 b\n2 a\n",
		},
		{
			name:  "the sort=random long form",
			stdin: strings.NewReader("b\na\nd\nc\n"),
			args:  []string{"--sort=random"},
			want:  "d\nb\nc\na\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDictionary(t *testing.T) {
	tests := []testCase{
		{
			name:  "only blanks and alphanumerics are significant",
			stdin: strings.NewReader("a!z\nab\n"),
			args:  []string{"-d"},
			want:  "ab\na!z\n",
		},
		{
			name:  "without d the punctuation is significant",
			stdin: strings.NewReader("a!z\nab\n"),
			want:  "a!z\nab\n",
		},
		{
			name:  "blanks are significant",
			stdin: strings.NewReader("ab\na b\n"),
			args:  []string{"-d"},
			want:  "a b\nab\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFoldCase(t *testing.T) {
	tests := []testCase{
		{
			name:  "lowercase folds to uppercase",
			stdin: strings.NewReader("b\nA\n"),
			args:  []string{"-f"},
			want:  "A\nb\n",
		},
		{
			name:  "equal folded lines fall back to the whole lines",
			stdin: strings.NewReader("a\nA\n"),
			args:  []string{"-f"},
			want:  "A\na\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunIgnoreNonPrint(t *testing.T) {
	tests := []testCase{
		{
			name:  "non printable characters are ignored",
			stdin: strings.NewReader("a\tx\nab\n"),
			args:  []string{"-i"},
			want:  "ab\na\tx\n",
		},
		{
			name:  "without i the tab is significant",
			stdin: strings.NewReader("a\tx\nab\n"),
			want:  "a\tx\nab\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunKeys(t *testing.T) {
	tests := []testCase{
		{
			name:  "the second field as the key",
			stdin: strings.NewReader("z b\ny a\n"),
			args:  []string{"-k", "2"},
			want:  "y a\nz b\n",
		},
		{
			name:  "a restricted key",
			stdin: strings.NewReader("z b\ny a\n"),
			args:  []string{"-k", "2,2"},
			want:  "y a\nz b\n",
		},
		{
			name:  "the field separator is part of the field",
			stdin: strings.NewReader("y a\nz  b\n"),
			args:  []string{"-k", "2.2"},
			want:  "z  b\ny a\n",
		},
		{
			name:  "leading blanks are ignored with the global b",
			stdin: strings.NewReader("y a\nz  b\n"),
			args:  []string{"-b", "-k", "2"},
			want:  "y a\nz  b\n",
		},
		{
			name:  "leading blanks are ignored with an attached b",
			stdin: strings.NewReader("y a\nz  b\n"),
			args:  []string{"-k", "2b"},
			want:  "y a\nz  b\n",
		},
		{
			name: "a character separator",
			fsys: testFS(),
			args: []string{"-t", ":", "-k", "2,2", "ids.txt"},
			want: "c:1\na:10\nb:2\n",
		},
		{
			name:  "empty fields with a character separator",
			stdin: strings.NewReader("a:\n:b\n"),
			args:  []string{"-t", ":", "-k", "1,1"},
			want:  ":b\na:\n",
		},
		{
			name:  "a key past the end of the line is empty",
			stdin: strings.NewReader("b 1\na\n"),
			args:  []string{"-k", "2"},
			want:  "a\nb 1\n",
		},
		{
			name:  "multiple keys break ties in order",
			stdin: strings.NewReader("a 2\nb 1\na 1\n"),
			args:  []string{"-k", "2,2", "-k", "1,1"},
			want:  "a 1\nb 1\na 2\n",
		},
		{
			name:  "a numeric key",
			stdin: strings.NewReader("a 10\nb 2\n"),
			args:  []string{"-k", "2n"},
			want:  "b 2\na 10\n",
		},
		{
			name:  "a reversed key",
			stdin: strings.NewReader("a 1\nb 2\n"),
			args:  []string{"-k", "2r"},
			want:  "b 2\na 1\n",
		},
		{
			name:  "a key inherits the global ordering",
			stdin: strings.NewReader("a 10\nb 2\n"),
			args:  []string{"-n", "-k", "2"},
			want:  "b 2\na 10\n",
		},
		{
			name:  "a key with modifiers doesn't inherit the global ordering",
			stdin: strings.NewReader("a 1\nb 2\n"),
			args:  []string{"-r", "-k", "2n,2"},
			want:  "a 1\nb 2\n",
		},
		{
			name:  "character positions",
			stdin: strings.NewReader("xb\nya\n"),
			args:  []string{"-k", "1.2,1.2"},
			want:  "ya\nxb\n",
		},
		{
			name:  "a key end past the end of the field",
			stdin: strings.NewReader("ab ef\nab cd\n"),
			args:  []string{"-k", "1.1,1.5"},
			want:  "ab cd\nab ef\n",
		},
		{
			name:  "a zero end character is the end of the field",
			stdin: strings.NewReader("z bq\ny ap\n"),
			args:  []string{"-k", "2.1,2.0"},
			want:  "y ap\nz bq\n",
		},
		{
			name:  "a key start past the key end is empty",
			stdin: strings.NewReader("b\na\n"),
			args:  []string{"-k", "2,1"},
			want:  "a\nb\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunUnique(t *testing.T) {
	tests := []testCase{
		{
			name:  "only one line of each set of equal lines",
			stdin: strings.NewReader("b\na\na\n"),
			args:  []string{"-u"},
			want:  "a\nb\n",
		},
		{
			name:  "lines with equal keys, not equal lines",
			stdin: strings.NewReader("x 1\ny 1\nz 2\n"),
			args:  []string{"-u", "-k", "2"},
			want:  "x 1\nz 2\n",
		},
		{
			name:  "keys folded by f keep the first of each set",
			stdin: strings.NewReader("a\nA\nb\n"),
			args:  []string{"-u", "-f"},
			want:  "a\nb\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunStable(t *testing.T) {
	tests := []testCase{
		{
			name:  "lines with equal keys keep their input order",
			stdin: strings.NewReader("b 2\na 2\n"),
			args:  []string{"-s", "-k", "2,2n"},
			want:  "b 2\na 2\n",
		},
		{
			name:  "without s the last resort comparison applies",
			stdin: strings.NewReader("b 2\na 2\n"),
			args:  []string{"-k", "2,2n"},
			want:  "a 2\nb 2\n",
		},
		{
			name:  "the whole line is still the key without keys",
			stdin: strings.NewReader("b\na\n"),
			args:  []string{"-s"},
			want:  "a\nb\n",
		},
		{
			name:  "the random ordering can be made stable",
			stdin: strings.NewReader("1 b\n2 a\n1 a\n"),
			args:  []string{"-R", "-s", "-k", "1,1"},
			want:  "2 a\n1 b\n1 a\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunZeroTerminated(t *testing.T) {
	tests := []testCase{
		{
			name:  "items are NUL terminated",
			stdin: strings.NewReader("b\x00a\x00"),
			args:  []string{"-z"},
			want:  "a\x00b\x00",
		},
		{
			name:  "a missing final NUL is added",
			stdin: strings.NewReader("b\x00a"),
			args:  []string{"-z"},
			want:  "a\x00b\x00",
		},
		{
			name:  "items can contain newlines",
			stdin: strings.NewReader("b x\ny\x00a\x00"),
			args:  []string{"-z"},
			want:  "a\x00b x\ny\x00",
		},
		{
			name:  "a sorted input passes the check",
			stdin: strings.NewReader("a\x00b\x00"),
			args:  []string{"-z", "-c"},
		},
		{
			name:     "a disordered input fails the check",
			stdin:    strings.NewReader("b\x00a\x00"),
			args:     []string{"-z", "-c"},
			wantErr:  "sort: -:2: disorder: a\n",
			wantExit: 1,
		},
		{
			name:      "the output goes to a file",
			stdin:     strings.NewReader("b\x00a\x00"),
			fsys:      testFS(),
			args:      []string{"-z", "-o", "out.txt"},
			wantFiles: map[string]string{"out.txt": "a\x00b\x00"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunCheck(t *testing.T) {
	tests := []testCase{
		{
			name:  "a sorted input",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-c"},
		},
		{
			name:     "a disordered input",
			stdin:    strings.NewReader("b\na\n"),
			args:     []string{"-c"},
			wantErr:  "sort: -:2: disorder: a\n",
			wantExit: 1,
		},
		{
			name:     "a disordered file",
			fsys:     testFS(),
			args:     []string{"-c", "bad.txt"},
			wantErr:  "sort: bad.txt:2: disorder: a\n",
			wantExit: 1,
		},
		{
			name:     "a silent check",
			stdin:    strings.NewReader("b\na\n"),
			args:     []string{"-C"},
			wantExit: 1,
		},
		{
			name:     "a check with the numeric ordering",
			stdin:    strings.NewReader("10\n2\n"),
			args:     []string{"-c", "-n"},
			wantErr:  "sort: -:2: disorder: 2\n",
			wantExit: 1,
		},
		{
			name:     "a duplicate key is a disorder with u",
			stdin:    strings.NewReader("a\na\n"),
			args:     []string{"-c", "-u"},
			wantErr:  "sort: -:2: disorder: a\n",
			wantExit: 1,
		},
		{
			name:  "duplicate lines are ordered without u",
			stdin: strings.NewReader("a\na\n"),
			args:  []string{"-c"},
		},
		{
			name:     "an extra operand",
			fsys:     testFS(),
			args:     []string{"-c", "sorted1.txt", "sorted2.txt"},
			wantErr:  "sort: extra operand 'sorted2.txt' not allowed with -c\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "c and m are incompatible",
			fsys:     testFS(),
			args:     []string{"-c", "-m", "sorted1.txt"},
			wantErr:  "sort: options -c and -m are incompatible\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a missing file",
			fsys:     testFS(),
			args:     []string{"-c", "missing.txt"},
			wantErr:  "sort: missing.txt: file does not exist\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunMerge(t *testing.T) {
	tests := []testCase{
		{
			name: "presorted files are merged",
			fsys: testFS(),
			args: []string{"-m", "sorted1.txt", "sorted2.txt"},
			want: "a\nb\nc\nd\n",
		},
		{
			name: "merged lines keep duplicates",
			fsys: fsx.MemFS{
				"one.txt": {Data: []byte("a\nb\n")},
				"two.txt": {Data: []byte("b\nc\n")},
			},
			args: []string{"-m", "one.txt", "two.txt"},
			want: "a\nb\nb\nc\n",
		},
		{
			name: "a merged unique",
			fsys: fsx.MemFS{
				"one.txt": {Data: []byte("a\nb\n")},
				"two.txt": {Data: []byte("b\nc\n")},
			},
			args: []string{"-m", "-u", "one.txt", "two.txt"},
			want: "a\nb\nc\n",
		},
		{
			name:  "merge from the standard input",
			stdin: strings.NewReader("a\nc\n"),
			fsys:  testFS(),
			args:  []string{"-m", "-", "sorted2.txt"},
			want:  "a\nb\nc\nd\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOutput(t *testing.T) {
	tests := []testCase{
		{
			name:      "the output goes to a file",
			stdin:     strings.NewReader("b\na\n"),
			fsys:      testFS(),
			args:      []string{"-o", "out.txt"},
			wantFiles: map[string]string{"out.txt": "a\nb\n"},
		},
		{
			name:      "the output file can be the input file",
			fsys:      testFS(),
			args:      []string{"-o", "bad.txt", "bad.txt"},
			wantFiles: map[string]string{"bad.txt": "a\nb\n"},
		},
		{
			name:      "the output file is truncated",
			fsys:      testFS(),
			args:      []string{"-o", "out.txt", "sorted1.txt"},
			wantFiles: map[string]string{"out.txt": "a\nc\n"},
		},
		{
			name:     "an output with a missing parent directory",
			stdin:    strings.NewReader("a\n"),
			fsys:     testFS(),
			args:     []string{"-o", "no/such/out.txt"},
			wantErr:  "sort: no/such/out.txt: file does not exist\n",
			wantExit: 2,
		},
		{
			name:     "a zero-length output name",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-o", ""},
			wantErr:  "sort: invalid zero-length file name\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOptions(t *testing.T) {
	tests := []testCase{
		{
			name:     "invalid option",
			args:     []string{"-x"},
			wantErr:  "sort: invalid option -- 'x'\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "unknown long option",
			args:     []string{"--foo"},
			wantErr:  "sort: unrecognized option '--foo'\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "missing key argument",
			args:     []string{"-k"},
			wantErr:  "sort: option requires an argument -- 'k'\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "missing output argument",
			args:     []string{"-o"},
			wantErr:  "sort: option requires an argument -- 'o'\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "missing separator argument",
			args:     []string{"-t"},
			wantErr:  "sort: option requires an argument -- 't'\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a separator of two characters",
			args:     []string{"-t", "::"},
			wantErr:  "sort: the separator must be a single character\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an empty key definition",
			args:     []string{"-k", ""},
			wantErr:  "sort: invalid key definition: ''\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a zero field",
			args:     []string{"-k", "0"},
			wantErr:  "sort: invalid key definition: '0'\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a zero start character",
			args:     []string{"-k", "1.0"},
			wantErr:  "sort: invalid key definition: '1.0'\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a non numeric field",
			args:     []string{"-k", "x"},
			wantErr:  "sort: invalid key definition: 'x'\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an unknown key modifier",
			args:     []string{"-k", "1x"},
			wantErr:  "sort: invalid key definition: '1x'\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a zero end field",
			args:     []string{"-k", "1,0"},
			wantErr:  "sort: invalid key definition: '1,0'\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an empty key end",
			args:     []string{"-k", "1,"},
			wantErr:  "sort: invalid key definition: '1,'\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid sort word",
			args:     []string{"--sort=bogus"},
			wantErr:  "sort: invalid argument 'bogus' for '--sort'\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a missing sort argument",
			args:     []string{"--sort"},
			wantErr:  "sort: option '--sort' requires an argument\nTry 'sort --help' for more information.\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLongOptions(t *testing.T) {
	tests := []testCase{
		{
			name:  "reverse",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"--reverse"},
			want:  "b\na\n",
		},
		{
			name:  "numeric-sort",
			stdin: strings.NewReader("10\n2\n"),
			args:  []string{"--numeric-sort"},
			want:  "2\n10\n",
		},
		{
			name:  "sort word",
			stdin: strings.NewReader("10\n2\n"),
			args:  []string{"--sort=numeric"},
			want:  "2\n10\n",
		},
		{
			name:  "unique",
			stdin: strings.NewReader("a\na\nb\n"),
			args:  []string{"--unique"},
			want:  "a\nb\n",
		},
		{
			name:  "key",
			stdin: strings.NewReader("z b\ny a\n"),
			args:  []string{"--key", "2"},
			want:  "y a\nz b\n",
		},
		{
			name: "field-separator",
			fsys: testFS(),
			args: []string{"--field-separator", ":", "--key=2,2", "ids.txt"},
			want: "c:1\na:10\nb:2\n",
		},
		{
			name:     "check",
			stdin:    strings.NewReader("b\na\n"),
			args:     []string{"--check"},
			wantErr:  "sort: -:2: disorder: a\n",
			wantExit: 1,
		},
		{
			name: "merge",
			fsys: testFS(),
			args: []string{"--merge", "sorted1.txt", "sorted2.txt"},
			want: "a\nb\nc\nd\n",
		},
		{
			name:      "output with an attached value",
			stdin:     strings.NewReader("b\na\n"),
			fsys:      testFS(),
			args:      []string{"--output=out.txt"},
			wantFiles: map[string]string{"out.txt": "a\nb\n"},
		},
		{
			name:  "ignore-leading-blanks",
			stdin: strings.NewReader("y a\nz  b\n"),
			args:  []string{"--ignore-leading-blanks", "--key", "2"},
			want:  "y a\nz  b\n",
		},
		{
			name:  "dictionary-order",
			stdin: strings.NewReader("a!z\nab\n"),
			args:  []string{"--dictionary-order"},
			want:  "ab\na!z\n",
		},
		{
			name:  "ignore-case",
			stdin: strings.NewReader("b\nA\n"),
			args:  []string{"--ignore-case"},
			want:  "A\nb\n",
		},
		{
			name:  "ignore-nonprinting",
			stdin: strings.NewReader("a\tx\nab\n"),
			args:  []string{"--ignore-nonprinting"},
			want:  "ab\na\tx\n",
		},
		{
			name:  "human-numeric-sort",
			stdin: strings.NewReader("10\n2\n"),
			args:  []string{"--human-numeric-sort"},
			want:  "2\n10\n",
		},
		{
			name:  "month-sort",
			stdin: strings.NewReader("MAR\nJAN\n"),
			args:  []string{"--month-sort"},
			want:  "JAN\nMAR\n",
		},
		{
			name:  "version-sort",
			stdin: strings.NewReader("1.10\n1.2\n"),
			args:  []string{"--version-sort"},
			want:  "1.2\n1.10\n",
		},
		{
			name:  "random-sort",
			stdin: strings.NewReader("b\na\nd\nc\n"),
			args:  []string{"--random-sort"},
			want:  "d\nb\nc\na\n",
		},
		{
			name:  "zero-terminated",
			stdin: strings.NewReader("b\x00a\x00"),
			args:  []string{"--zero-terminated"},
			want:  "a\x00b\x00",
		},
		{
			name:  "stable",
			stdin: strings.NewReader("b 2\na 2\n"),
			args:  []string{"--stable", "-k", "2,2n"},
			want:  "b 2\na 2\n",
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
			name:     "missing file",
			fsys:     testFS(),
			args:     []string{"missing.txt"},
			wantErr:  "sort: missing.txt: file does not exist\n",
			wantExit: 2,
		},
		{
			name:     "directory operand",
			fsys:     fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			args:     []string{"dir"},
			wantErr:  "sort: dir: invalid argument\n",
			wantExit: 2,
		},
		{
			name:     "zero-length file name",
			fsys:     testFS(),
			args:     []string{""},
			wantErr:  "sort: invalid zero-length file name\n",
			wantExit: 2,
		},
		{
			name:     "file operand without a file system",
			args:     []string{"names.txt"},
			wantErr:  "sort: names.txt: no file system available\n",
			wantExit: 2,
		},
		{
			name:     "no standard input",
			wantErr:  "sort: standard input: no standard input available\n",
			wantExit: 2,
		},
		{
			name:     "standard input read error",
			stdin:    errorReader{err: errors.New("read failed")},
			wantErr:  "sort: standard input: read failed\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("b\na\n"),
		Stdout: errorWriter{err: writeErr},
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context())

	if code := exitCode(t, err); code != 2 {
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
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		runCancelled(t, ctx, strings.NewReader("b\na\n"))
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runCancelled(t, ctx, &cancelReader{cancel: cancel})
	})
}

// runCancelled runs the utility with a context that is either already done or
// becomes done while reading, and checks that the context error is returned
// without any output.
func runCancelled(t *testing.T, ctx context.Context, stdin io.Reader) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
	err := New(rt).Run(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
}

func TestRunSortTypeLastWins(t *testing.T) {
	tests := []testCase{
		{
			name:  "a later month sort replaces the numeric one",
			stdin: strings.NewReader("2\n10\n1\n"),
			args:  []string{"-n", "-M"},
			want:  "1\n10\n2\n",
		},
		{
			name:  "a later numeric sort replaces the month one",
			stdin: strings.NewReader("2\n10\n1\n"),
			args:  []string{"-M", "-n"},
			want:  "1\n2\n10\n",
		},
		{
			name:  "the long forms replace each other too",
			stdin: strings.NewReader("2\n10\n1\n"),
			args:  []string{"--sort=numeric", "--sort=month"},
			want:  "1\n10\n2\n",
		},
		{
			name:  "a key modifier replaces the global type",
			stdin: strings.NewReader("2\n10\n1\n"),
			args:  []string{"-M", "-k1,1n"},
			want:  "1\n2\n10\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "sort (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
