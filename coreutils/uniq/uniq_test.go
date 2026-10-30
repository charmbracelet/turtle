package uniq

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
			stdin: strings.NewReader("a\na\nb\n"),
			want:  "a\nb\n",
		},
		{
			name:  "adjacent repeated lines collapse",
			stdin: strings.NewReader("a\na\nb\nb\nb\nc\na\n"),
			want:  "a\nb\nc\na\n",
		},
		{
			name:  "repeated lines that are not adjacent are kept",
			stdin: strings.NewReader("a\nb\na\n"),
			want:  "a\nb\na\n",
		},
		{
			name:  "no repeated lines",
			stdin: strings.NewReader("a\nb\nc\n"),
			want:  "a\nb\nc\n",
		},
		{
			name:  "empty lines group together",
			stdin: strings.NewReader("a\n\n\nb\n"),
			want:  "a\n\nb\n",
		},
		{
			name:  "an empty standard input",
			stdin: strings.NewReader(""),
			want:  "",
		},
		{
			name:  "a missing final newline is not added",
			stdin: strings.NewReader("a\nb"),
			want:  "a\nb",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunCount(t *testing.T) {
	tests := []testCase{
		{
			name:  "each line is preceded by its count",
			stdin: strings.NewReader("a\na\nb\n"),
			args:  []string{"-c"},
			want:  "      2 a\n      1 b\n",
		},
		{
			name:  "a wider count",
			stdin: strings.NewReader(strings.Repeat("a\n", 10)),
			args:  []string{"-c"},
			want:  "     10 a\n",
		},
		{
			name:  "counts of repeated lines with d",
			stdin: strings.NewReader("a\na\nb\n"),
			args:  []string{"-c", "-d"},
			want:  "      2 a\n",
		},
		{
			name:  "counts of unique lines with u",
			stdin: strings.NewReader("a\na\nb\n"),
			args:  []string{"-c", "-u"},
			want:  "      1 b\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRepeated(t *testing.T) {
	tests := []testCase{
		{
			name:  "only repeated lines, one copy each",
			stdin: strings.NewReader("a\na\nb\nb\nb\nc\n"),
			args:  []string{"-d"},
			want:  "a\nb\n",
		},
		{
			name:  "no repeated lines",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-d"},
			want:  "",
		},
		{
			name:  "an unrepeated line is not written",
			stdin: strings.NewReader("a\n"),
			args:  []string{"-d"},
			want:  "",
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
			name:  "only non repeated lines",
			stdin: strings.NewReader("a\na\nb\nb\nb\nc\n"),
			args:  []string{"-u"},
			want:  "c\n",
		},
		{
			name:  "all lines repeated",
			stdin: strings.NewReader("a\na\n"),
			args:  []string{"-u"},
			want:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFields(t *testing.T) {
	tests := []testCase{
		{
			name: "the first field is ignored",
			fsys: testFS(),
			args: []string{"-f", "1", "fields.txt"},
			want: "x 1\nz 2\n",
		},
		{
			name:  "zero fields is a no-op",
			stdin: strings.NewReader("a\na\n"),
			args:  []string{"-f", "0"},
			want:  "a\n",
		},
		{
			name:  "more fields than the line has compare as empty",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-f", "5"},
			want:  "a\n",
		},
		{
			name:  "the blanks before a field are part of it",
			stdin: strings.NewReader("x   a\ny a\n"),
			args:  []string{"-f", "1"},
			want:  "x   a\ny a\n",
		},
		{
			name:  "leading blanks belong to the first field",
			stdin: strings.NewReader("  a x\n  b x\n"),
			args:  []string{"-f", "1"},
			want:  "  a x\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunChars(t *testing.T) {
	tests := []testCase{
		{
			name: "the first characters are ignored",
			fsys: testFS(),
			args: []string{"-s", "1", "utf8.txt"},
			want: "áx\náy\n",
		},
		{
			name:  "more characters than the line has compare as empty",
			stdin: strings.NewReader("ab\ncd\n"),
			args:  []string{"-s", "5"},
			want:  "ab\n",
		},
		{
			name:  "characters are skipped after the fields",
			stdin: strings.NewReader("1x a\n2y a\n"),
			args:  []string{"-f", "1", "-s", "1"},
			want:  "1x a\n",
		},
		{
			name:  "characters are characters, not bytes",
			stdin: strings.NewReader("áax\nábx\n"),
			args:  []string{"-s", "2"},
			want:  "áax\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunIgnoreCase(t *testing.T) {
	tests := []testCase{
		{
			name:  "lines differing only in case group together",
			stdin: strings.NewReader("a\nA\nb\n"),
			args:  []string{"-i"},
			want:  "a\nb\n",
		},
		{
			name:  "the case of the written line is the first one",
			stdin: strings.NewReader("A\na\nA\n"),
			args:  []string{"-i"},
			want:  "A\n",
		},
		{
			name:  "non ASCII letters are not folded",
			stdin: strings.NewReader("á\nÁ\n"),
			args:  []string{"-i"},
			want:  "á\nÁ\n",
		},
		{
			name:  "with counts",
			stdin: strings.NewReader("a\nA\nb\n"),
			args:  []string{"-i", "-c"},
			want:  "      2 a\n      1 b\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunAllRepeated(t *testing.T) {
	tests := []testCase{
		{
			name:  "every line of a repeated group is written",
			stdin: strings.NewReader("a\na\nb\nb\nb\nc\na\n"),
			args:  []string{"-D"},
			want:  "a\na\nb\nb\nb\n",
		},
		{
			name:  "no repeated lines",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-D"},
			want:  "",
		},
		{
			name:  "none delimits nothing",
			stdin: strings.NewReader("a\na\nb\nb\n"),
			args:  []string{"--all-repeated=none"},
			want:  "a\na\nb\nb\n",
		},
		{
			name:  "separate delimits between groups",
			stdin: strings.NewReader("a\na\nb\nb\n"),
			args:  []string{"--all-repeated=separate"},
			want:  "a\na\n\nb\nb\n",
		},
		{
			name:  "separate delimits nothing before the first group",
			stdin: strings.NewReader("x\na\na\nb\nb\n"),
			args:  []string{"--all-repeated=separate"},
			want:  "a\na\n\nb\nb\n",
		},
		{
			name:  "prepend delimits before each group",
			stdin: strings.NewReader("a\na\nb\nb\n"),
			args:  []string{"--all-repeated=prepend"},
			want:  "\na\na\n\nb\nb\n",
		},
		{
			name:  "separate delimits with a NUL byte with z",
			stdin: strings.NewReader("a\x00a\x00b\x00b\x00"),
			args:  []string{"-z", "--all-repeated=separate"},
			want:  "a\x00a\x00\x00b\x00b\x00",
		},
		{
			name:  "a group without a final newline",
			stdin: strings.NewReader("a\na"),
			args:  []string{"-D"},
			want:  "a\na",
		},
		{
			name:  "with a skipped field",
			stdin: strings.NewReader("1 a\n2 a\n3 b\n"),
			args:  []string{"-D", "-f", "1"},
			want:  "1 a\n2 a\n",
		},
		{
			name:  "ignoring case",
			stdin: strings.NewReader("a\nA\nb\nb\n"),
			args:  []string{"-D", "-i"},
			want:  "a\nA\nb\nb\n",
		},
		{
			name:     "an invalid delimit method",
			stdin:    strings.NewReader("a\na\n"),
			args:     []string{"--all-repeated=x"},
			wantErr:  "uniq: invalid argument 'x' for '--all-repeated'\n",
			wantExit: 2,
		},
		{
			name:     "count is meaningless with all repeated",
			stdin:    strings.NewReader("a\na\n"),
			args:     []string{"-c", "-D"},
			wantErr:  "uniq: printing all duplicated lines and repeat counts is meaningless\n",
			wantExit: 2,
		},
		{
			name:     "unique is not supported with all repeated",
			stdin:    strings.NewReader("a\na\n"),
			args:     []string{"-u", "-D"},
			wantErr:  "uniq: printing all duplicated and unique lines is not supported\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunCheckChars(t *testing.T) {
	tests := []testCase{
		{
			name:  "only the first characters are compared",
			stdin: strings.NewReader("apple\napricot\nbanana\n"),
			args:  []string{"-w", "2"},
			want:  "apple\nbanana\n",
		},
		{
			name:  "zero characters make every line equal",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-w", "0"},
			want:  "a\n",
		},
		{
			name:  "more characters than the line has",
			stdin: strings.NewReader("a\nab\n"),
			args:  []string{"-w", "5"},
			want:  "a\nab\n",
		},
		{
			name:  "characters are compared after the skipped ones",
			stdin: strings.NewReader("xxab\nxxac\nxxb\n"),
			args:  []string{"-s", "2", "-w", "1"},
			want:  "xxab\nxxb\n",
		},
		{
			name:  "characters are compared after the skipped fields",
			stdin: strings.NewReader("1 ax\n2 ay\n"),
			args:  []string{"-f", "1", "-w", "1"},
			want:  "1 ax\n",
		},
		{
			name:  "characters are characters, not bytes",
			stdin: strings.NewReader("áa\náb\n"),
			args:  []string{"-w", "1"},
			want:  "áa\n",
		},
		{
			name:  "ignoring case",
			stdin: strings.NewReader("aB\nAb\n"),
			args:  []string{"-w", "2", "-i"},
			want:  "aB\n",
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
			name:  "items are delimited by a NUL byte",
			stdin: strings.NewReader("a\x00a\x00b\x00"),
			args:  []string{"-z"},
			want:  "a\x00b\x00",
		},
		{
			name:  "an item without a final NUL is kept as is",
			stdin: strings.NewReader("a\x00a"),
			args:  []string{"-z"},
			want:  "a\x00",
		},
		{
			name:  "newlines are part of the items",
			stdin: strings.NewReader("a\nb\x00a\nb\x00"),
			args:  []string{"-z"},
			want:  "a\nb\x00",
		},
		{
			name:  "counts are written before the items",
			stdin: strings.NewReader("a\x00a\x00b\x00"),
			args:  []string{"-z", "-c"},
			want:  "      2 a\x00      1 b\x00",
		},
		{
			name:  "only repeated items",
			stdin: strings.NewReader("a\x00a\x00b\x00"),
			args:  []string{"-z", "-d"},
			want:  "a\x00",
		},
		{
			name:  "a newline separates fields",
			stdin: strings.NewReader("a\nx\x00b\nx\x00"),
			args:  []string{"-z", "-f", "1"},
			want:  "a\nx\x00",
		},
		{
			name:  "an empty standard input",
			stdin: strings.NewReader(""),
			args:  []string{"-z"},
			want:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOperands(t *testing.T) {
	tests := []testCase{
		{
			name: "an input file",
			fsys: testFS(),
			args: []string{"dups.txt"},
			want: "a\nb\nc\na\n",
		},
		{
			name:      "an input and an output file",
			fsys:      testFS(),
			args:      []string{"dups.txt", "out.txt"},
			want:      "",
			wantFiles: map[string]string{"out.txt": "a\nb\nc\na\n"},
		},
		{
			name:      "the output file is truncated",
			fsys:      testFS(),
			args:      []string{"fields.txt", "out.txt"},
			wantFiles: map[string]string{"out.txt": "x 1\ny 1\nz 2\n"},
		},
		{
			name:  "a dash input reads the standard input",
			stdin: strings.NewReader("s\ns\n"),
			args:  []string{"-"},
			want:  "s\n",
		},
		{
			name: "a dash output writes the standard output",
			fsys: testFS(),
			args: []string{"dups.txt", "-"},
			want: "a\nb\nc\na\n",
		},
		{
			name:      "pathnames are resolved from the working directory",
			fsys:      testFS(),
			dir:       "sub",
			args:      []string{"inner.txt", "out.txt"},
			wantFiles: map[string]string{"sub/out.txt": "i\n"},
		},
		{
			name: "a file named like an option after a double dash",
			fsys: testFS(),
			args: []string{"--", "-d"},
			want: "p\n",
		},
		{
			name:      "options are not parsed after the first operand",
			fsys:      testFS(),
			args:      []string{"dups.txt", "-c"},
			want:      "",
			wantFiles: map[string]string{"-c": "a\nb\nc\na\n"},
		},
		{
			name:     "an extra operand",
			fsys:     testFS(),
			args:     []string{"dups.txt", "out.txt", "third.txt"},
			wantErr:  "uniq: extra operand 'third.txt'\n",
			wantExit: 2,
		},
		{
			name:     "a zero-length input operand",
			fsys:     testFS(),
			args:     []string{""},
			wantErr:  "uniq: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "a zero-length output operand",
			fsys:     testFS(),
			args:     []string{"dups.txt", ""},
			wantErr:  "uniq: invalid zero-length file name\n",
			wantExit: 1,
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
			wantErr:  "uniq: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "unknown long option",
			args:     []string{"--foo"},
			wantErr:  "uniq: unrecognized option '--foo'\n",
			wantExit: 2,
		},
		{
			name:     "d and u are incompatible",
			args:     []string{"-d", "-u"},
			wantErr:  "uniq: printing only duplicated and unique lines is not supported\n",
			wantExit: 2,
		},
		{
			name:     "missing field count argument",
			args:     []string{"-f"},
			wantErr:  "uniq: option requires an argument -- 'f'\n",
			wantExit: 2,
		},
		{
			name:     "a non numeric field count",
			args:     []string{"-f", "x"},
			wantErr:  "uniq: invalid number of fields: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "a negative field count",
			args:     []string{"-f", "-1"},
			wantErr:  "uniq: invalid number of fields: '-1'\n",
			wantExit: 2,
		},
		{
			name:     "a non numeric character count",
			args:     []string{"-s", "x"},
			wantErr:  "uniq: invalid number of characters: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "missing count of characters to compare argument",
			args:     []string{"-w"},
			wantErr:  "uniq: option requires an argument -- 'w'\n",
			wantExit: 2,
		},
		{
			name:     "a non numeric count of characters to compare",
			args:     []string{"-w", "x"},
			wantErr:  "uniq: invalid number of characters to compare: 'x'\n",
			wantExit: 2,
		},
		{
			name:     "a negative count of characters to compare",
			args:     []string{"-w", "-1"},
			wantErr:  "uniq: invalid number of characters to compare: '-1'\n",
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
			name:  "count",
			stdin: strings.NewReader("a\na\nb\n"),
			args:  []string{"--count"},
			want:  "      2 a\n      1 b\n",
		},
		{
			name:  "repeated",
			stdin: strings.NewReader("a\na\nb\n"),
			args:  []string{"--repeated"},
			want:  "a\n",
		},
		{
			name:  "unique",
			stdin: strings.NewReader("a\na\nb\n"),
			args:  []string{"--unique"},
			want:  "b\n",
		},
		{
			name: "skip-fields",
			fsys: testFS(),
			args: []string{"--skip-fields", "1", "fields.txt"},
			want: "x 1\nz 2\n",
		},
		{
			name: "skip-chars with an attached value",
			fsys: testFS(),
			args: []string{"--skip-chars=1", "utf8.txt"},
			want: "áx\náy\n",
		},
		{
			name:  "ignore-case",
			stdin: strings.NewReader("a\nA\n"),
			args:  []string{"--ignore-case"},
			want:  "a\n",
		},
		{
			name:  "all-repeated",
			stdin: strings.NewReader("a\na\nb\n"),
			args:  []string{"--all-repeated"},
			want:  "a\na\n",
		},
		{
			name:  "check-chars with an attached value",
			stdin: strings.NewReader("ab\nac\n"),
			args:  []string{"--check-chars=1"},
			want:  "ab\n",
		},
		{
			name:  "zero-terminated",
			stdin: strings.NewReader("a\x00a\x00"),
			args:  []string{"--zero-terminated"},
			want:  "a\x00",
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
			name:     "missing input file",
			fsys:     testFS(),
			args:     []string{"missing.txt"},
			wantErr:  "uniq: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "directory input",
			fsys:     fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			args:     []string{"dir"},
			wantErr:  "uniq: dir: invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "directory output",
			fsys:     fsx.MemFS{"dups.txt": {Data: []byte("a\n")}, "dir": {Mode: fs.ModeDir}},
			args:     []string{"dups.txt", "dir"},
			wantErr:  "uniq: dir: invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "output with a missing parent directory",
			fsys:     testFS(),
			args:     []string{"dups.txt", "no/such/out.txt"},
			wantErr:  "uniq: no/such/out.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "input file without a file system",
			args:     []string{"dups.txt"},
			wantErr:  "uniq: dups.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "no standard input",
			wantErr:  "uniq: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "standard input read error",
			stdin:    errorReader{err: errors.New("read failed")},
			wantErr:  "uniq: standard input: read failed\n",
			wantExit: 1,
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
		Stdin:  strings.NewReader("hello\n"),
		Stdout: errorWriter{err: writeErr},
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context())

	if code := exitCode(t, err); code != 1 {
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
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		runCancelled(t, ctx, strings.NewReader("hello\n"))
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
