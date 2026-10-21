package ls

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// dirMode returns the mode of a directory with the given permissions.
func dirMode(perm fs.FileMode) fs.FileMode { return fs.ModeDir | perm }

// link returns a symbolic entry pointing at target.
func link(target string) *fsx.MemFile {
	return &fsx.MemFile{Data: []byte(target), Mode: fs.ModeSymlink | 0o777}
}

// linkAt returns a symbolic entry pointing at target, made at the given time.
func linkAt(target string, mod time.Time) *fsx.MemFile {
	return &fsx.MemFile{
		Data:    []byte(target),
		Mode:    fs.ModeSymlink | 0o777,
		ModTime: mod,
	}
}

// times holds the modification times the tests write listings with, so that
// the long format always lands on the same side of the six month boundary.
type times struct {
	recent time.Time
	old    time.Time
	future time.Time
}

// newTimes returns the times a test can build a listing around.
func newTimes() times {
	now := time.Now()
	return times{
		recent: now.Add(-24 * time.Hour),
		old:    now.AddDate(0, -7, 0),
		future: now.Add(24 * time.Hour),
	}
}

// day returns the date and time field the long format writes for a recent
// modification time.
func (t times) day(when time.Time) string {
	return when.Format("Jan _2 15:04")
}

// year returns the date and time field the long format writes for a time that
// is too old to show the time of day, or one in the future.
func (t times) year(when time.Time) string {
	return when.Format("Jan _2  2006")
}

type testCase struct {
	name     string
	files    fsx.MemFS
	sys      fsx.FS
	wd       string
	env      map[string]string
	args     []string
	wantOut  string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     stderr,
		WorkingDir: test.wd,
		LookupEnv:  envLookup(test.env),
	}
	switch {
	case test.sys != nil:
		rt.FS = test.sys
	case test.files != nil:
		rt.FS = test.files
	}

	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.wantOut {
		t.Errorf("Run() stdout = %q, want %q", got, test.wantOut)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
}

func runCases(t *testing.T, tests []testCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

// envLookup returns the lookup function of a test environment.
func envLookup(values map[string]string) coreutils.LookupEnvFunc {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
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

func TestRunDefault(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "lists the working directory when no operand is given",
			files:   fsx.MemFS{"b.txt": {}, "a.txt": {}},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "writes one entry per line by default",
			files:   fsx.MemFS{"a.txt": {}, "b.txt": {}},
			args:    []string{"."},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "hides the entries that begin with a dot",
			files:   fsx.MemFS{".hidden": {}, "a.txt": {}},
			wantOut: "a.txt\n",
		},
		{
			name:    "writes an operand naming a hidden file",
			files:   fsx.MemFS{".hidden": {}},
			args:    []string{".hidden"},
			wantOut: ".hidden\n",
		},
		{
			name:    "lists the contents of a directory operand",
			files:   fsx.MemFS{"dir/a.txt": {}, "dir/b.txt": {}},
			args:    []string{"dir"},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "sorts entries by their bytes",
			files:   fsx.MemFS{"b.txt": {}, "C.txt": {}, "a.txt": {}},
			wantOut: "C.txt\na.txt\nb.txt\n",
		},
		{
			name:    "writes nothing for an empty directory",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}},
			args:    []string{"dir"},
			wantOut: "",
		},
		{
			name:    "resolves an operand against the working directory",
			files:   fsx.MemFS{"work/a.txt": {}, "a.txt": {}},
			wd:      "work",
			args:    []string{"a.txt"},
			wantOut: "a.txt\n",
		},
		{
			name:    "lists the working directory of an operand ending in a separator",
			files:   fsx.MemFS{"dir/a.txt": {}},
			args:    []string{"dir/"},
			wantOut: "a.txt\n",
		},
		{
			name:    "lists a directory named through redundant separators",
			files:   fsx.MemFS{"dir/a.txt": {}, "other/a.txt": {}},
			args:    []string{"./dir/./"},
			wantOut: "a.txt\n",
		},
		{
			name:     "reports a missing operand",
			files:    fsx.MemFS{},
			args:     []string{"nope"},
			wantErr:  "ls: cannot access 'nope': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "lists the other operands after a failure",
			files:    fsx.MemFS{"a.txt": {}},
			args:     []string{"nope", "a.txt"},
			wantOut:  "a.txt\n",
			wantErr:  "ls: cannot access 'nope': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "reports a zero-length operand",
			files:    fsx.MemFS{},
			args:     []string{""},
			wantErr:  "ls: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:    "writes the name of a file operand as specified",
			files:   fsx.MemFS{"dir/a.txt": {}},
			args:    []string{"dir//a.txt"},
			wantOut: "dir//a.txt\n",
		},
	})
}

func TestRunAll(t *testing.T) {
	now := newTimes()

	runCases(t, []testCase{
		{
			name:    "-a writes the entries that begin with a dot",
			files:   fsx.MemFS{".hidden": {}, "a.txt": {}},
			args:    []string{"-a"},
			wantOut: ".hidden\na.txt\n",
		},
		{
			name:    "-A writes the hidden entries",
			files:   fsx.MemFS{".hidden": {}, "a.txt": {}},
			args:    []string{"-A"},
			wantOut: ".hidden\na.txt\n",
		},
		{
			name:    "-a writes the long listing of a hidden entry",
			files:   fsx.MemFS{".hidden": {Mode: 0o600, ModTime: now.recent}},
			args:    []string{"-al"},
			wantOut: "total 0\n-rw------- 1 0 0 0 " + now.day(now.recent) + " .hidden\n",
		},
	})
}

func TestRunLong(t *testing.T) {
	now := newTimes()

	runCases(t, []testCase{
		{
			name: "-l writes the long format",
			files: fsx.MemFS{
				"dir":   {Mode: dirMode(0o755), ModTime: now.recent},
				"f.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: now.recent},
			},
			args: []string{"-l"},
			wantOut: "total 1\n" +
				"drwxr-xr-x 1 0 0 0 " + now.day(now.recent) + " dir\n" +
				"-rw-r--r-- 1 0 0 5 " + now.day(now.recent) + " f.txt\n",
		},
		{
			name: "-l separates the fields with a single space",
			files: fsx.MemFS{
				"one":   {Data: []byte("x"), Mode: 0o644, ModTime: now.recent},
				"three": {Data: []byte(strings.Repeat("x", 10000)), Mode: 0o444, ModTime: now.recent},
				"two":   {Data: []byte(strings.Repeat("x", 1000)), Mode: 0o644, ModTime: now.recent},
			},
			args: []string{"-l"},
			wantOut: "total 23\n" +
				"-rw-r--r-- 1 0 0 1 " + now.day(now.recent) + " one\n" +
				"-r--r--r-- 1 0 0 10000 " + now.day(now.recent) + " three\n" +
				"-rw-r--r-- 1 0 0 1000 " + now.day(now.recent) + " two\n",
		},
		{
			name:    "-l writes the year for an old modification time",
			files:   fsx.MemFS{"old.txt": {Mode: 0o644, ModTime: now.old}},
			args:    []string{"-l"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.year(now.old) + " old.txt\n",
		},
		{
			name:    "-l writes the year for a future modification time",
			files:   fsx.MemFS{"soon.txt": {Mode: 0o644, ModTime: now.future}},
			args:    []string{"-l"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.year(now.future) + " soon.txt\n",
		},
		{
			name: "-l writes every permission flag",
			files: fsx.MemFS{
				"deletable": {Mode: fs.ModeSticky | dirMode(0o757), ModTime: now.recent},
				"exec":      {Mode: fs.ModeSetuid | 0o744, ModTime: now.recent},
				"group":     {Mode: fs.ModeSetgid | 0o644, ModTime: now.recent},
				"no-exec":   {Mode: fs.ModeSetuid | 0o644, ModTime: now.recent},
				"nosearch":  {Mode: dirMode(0o500), ModTime: now.recent},
				"plain":     {Mode: 0o0000, ModTime: now.recent},
				"sticky":    {Mode: fs.ModeSticky | dirMode(0o750), ModTime: now.recent},
			},
			args: []string{"-l"},
			wantOut: "total 0\n" +
				"drwxr-xrwt 1 0 0 0 " + now.day(now.recent) + " deletable\n" +
				"-rwsr--r-- 1 0 0 0 " + now.day(now.recent) + " exec\n" +
				"-rw-r-Sr-- 1 0 0 0 " + now.day(now.recent) + " group\n" +
				"-rwSr--r-- 1 0 0 0 " + now.day(now.recent) + " no-exec\n" +
				"dr-x------ 1 0 0 0 " + now.day(now.recent) + " nosearch\n" +
				"---------- 1 0 0 0 " + now.day(now.recent) + " plain\n" +
				"drwxr-x--T 1 0 0 0 " + now.day(now.recent) + " sticky\n",
		},
		{
			name: "-l writes the type of each file",
			files: fsx.MemFS{
				"block":  {Mode: fs.ModeDevice | 0o640, ModTime: now.recent},
				"char":   {Mode: fs.ModeDevice | fs.ModeCharDevice | 0o640, ModTime: now.recent},
				"other":  {Mode: fs.ModeIrregular | 0o640, ModTime: now.recent},
				"pipe":   {Mode: fs.ModeNamedPipe | 0o640, ModTime: now.recent},
				"socket": {Mode: fs.ModeSocket | 0o640, ModTime: now.recent},
			},
			args: []string{"-l"},
			wantOut: "total 0\n" +
				"brw-r----- 1 0 0 0 " + now.day(now.recent) + " block\n" +
				"crw-r----- 1 0 0 0 " + now.day(now.recent) + " char\n" +
				"?rw-r----- 1 0 0 0 " + now.day(now.recent) + " other\n" +
				"prw-r----- 1 0 0 0 " + now.day(now.recent) + " pipe\n" +
				"srw-r----- 1 0 0 0 " + now.day(now.recent) + " socket\n",
		},
		{
			name:    "-l writes the file operand as specified",
			files:   fsx.MemFS{"dir/a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-l", "dir/a.txt"},
			wantOut: "-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " dir/a.txt\n",
		},
		{
			name:    "-l of an empty directory writes only the total",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}},
			args:    []string{"-l", "dir"},
			wantOut: "total 0\n",
		},
		{
			name:    "-C disables the long format",
			files:   fsx.MemFS{"a.txt": {}, "b.txt": {}},
			args:    []string{"-l", "-C"},
			wantOut: "a.txt  b.txt\n",
		},
		{
			name:    "-l after -C writes the long format again",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-C", "-l"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-1 keeps the long format",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-l", "-1"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-n writes the identifiers as numbers",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-n"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-g omits the owner",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-g"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-o omits the group",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-o"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-go omits both",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-go"},
			wantOut: "total 0\n-rw-r--r-- 1 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-li writes the file serial number first",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-li"},
			wantOut: "total 0\n0 -rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-ls writes the block count before the line",
			files:   fsx.MemFS{"a.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-ls"},
			wantOut: "total 1\n1 -rw-r--r-- 1 0 0 5 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-c writes the status change time",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.old}},
			args:    []string{"-l", "-c"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.year(now.old) + " a.txt\n",
		},
		{
			name:    "-u writes the access time",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.old}},
			args:    []string{"-l", "-u"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.year(now.old) + " a.txt\n",
		},
		{
			name:    "-u after -c chooses the access time",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-l", "-c", "-u"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-c after -u chooses the status change time",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-l", "-u", "-c"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
	})
}

func TestRunInode(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "-i writes the file serial number",
			files:   fsx.MemFS{"a.txt": {}, "b.txt": {}},
			args:    []string{"-i", "-1"},
			wantOut: "0 a.txt\n0 b.txt\n",
		},
		{
			name:    "-i writes the number before the columns",
			files:   fsx.MemFS{"a.txt": {}, "b.txt": {}},
			args:    []string{"-i", "-C"},
			wantOut: "0 a.txt  0 b.txt\n",
		},
	})
}

func TestRunBlocks(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "-s writes the blocks and their total",
			files:   fsx.MemFS{"a.txt": {Data: []byte("x")}, "b.txt": {Data: []byte(strings.Repeat("x", 512))}},
			args:    []string{"-s", "-1"},
			wantOut: "total 2\n1 a.txt\n1 b.txt\n",
		},
		{
			name:    "-s writes no block for an empty file",
			files:   fsx.MemFS{"a.txt": {}},
			args:    []string{"-s", "-1"},
			wantOut: "total 0\n0 a.txt\n",
		},
		{
			name:    "-k counts blocks in kibibytes",
			files:   fsx.MemFS{"a.txt": {Data: []byte("x")}, "b.txt": {Data: []byte(strings.Repeat("x", 512))}},
			args:    []string{"-s", "-k", "-1"},
			wantOut: "total 2\n1 a.txt\n1 b.txt\n",
		},
		{
			name:    "-s writes the blocks of a file operand without a total",
			files:   fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:    []string{"-s", "-1", "a.txt"},
			wantOut: "1 a.txt\n",
		},
		{
			name:    "-s writes the numbers of listings of different widths",
			files:   fsx.MemFS{"a.txt": {}, "b.txt": {Data: []byte(strings.Repeat("x", 5120))}},
			args:    []string{"-s", "-1"},
			wantOut: "total 10\n0 a.txt\n10 b.txt\n",
		},
	})
}

func TestRunFormat(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "-C writes entries down the columns",
			files:   fsx.MemFS{"a": {}, "b": {}, "c": {}, "d": {}, "e": {}, "f": {}, "g": {}},
			env:     map[string]string{"COLUMNS": "10"},
			args:    []string{"-C"},
			wantOut: "a  d  g\nb  e\nc  f\n",
		},
		{
			name:    "-C pads to the width of the longest name",
			files:   fsx.MemFS{"a": {}, "bbbb": {}},
			env:     map[string]string{"COLUMNS": "20"},
			args:    []string{"-C"},
			wantOut: "a     bbbb\n",
		},
		{
			name:    "-C writes as many columns as the width fits",
			files:   fsx.MemFS{"a": {}, "b": {}, "c": {}, "d": {}},
			env:     map[string]string{"COLUMNS": "80"},
			args:    []string{"-C"},
			wantOut: "a  b  c  d\n",
		},
		{
			name:    "-x writes entries across the columns",
			files:   fsx.MemFS{"a": {}, "b": {}, "c": {}, "d": {}},
			env:     map[string]string{"COLUMNS": "10"},
			args:    []string{"-x"},
			wantOut: "a  b  c\nd\n",
		},
		{
			name:    "-C pads by the characters of the names",
			files:   fsx.MemFS{"a": {}, "\u00e9\u00e9": {}},
			env:     map[string]string{"COLUMNS": "20"},
			args:    []string{"-C"},
			wantOut: "a   \u00e9\u00e9\n",
		},
		{
			name:    "-C writes the file operands in the same columns",
			files:   fsx.MemFS{"a.txt": {}, "bb.txt": {}},
			args:    []string{"-C", "bb.txt", "a.txt"},
			wantOut: "a.txt   bb.txt\n",
		},
		{
			name:    "-C pads the names by their characters",
			files:   fsx.MemFS{"a": {}, "\u00e9\u00e9": {}},
			env:     map[string]string{"COLUMNS": "20"},
			args:    []string{"-C"},
			wantOut: "a   \u00e9\u00e9\n",
		},
		{
			name:    "-C writes one column when a name is too wide",
			files:   fsx.MemFS{"aaaaaaaaaaaa": {}, "b": {}},
			env:     map[string]string{"COLUMNS": "10"},
			args:    []string{"-C"},
			wantOut: "aaaaaaaaaaaa\nb\n",
		},
		{
			name:    "-m writes the names separated by commas",
			files:   fsx.MemFS{"a": {}, "b": {}, "c": {}},
			args:    []string{"-m"},
			wantOut: "a, b, c\n",
		},
		{
			name:    "-m wraps when a name doesn't fit the width",
			files:   fsx.MemFS{"aaa": {}, "bbb": {}, "ccc": {}},
			env:     map[string]string{"COLUMNS": "7"},
			args:    []string{"-m"},
			wantOut: "aaa,\nbbb,\nccc\n",
		},
		{
			name:    "-1 writes one entry per line",
			files:   fsx.MemFS{"a": {}, "b": {}},
			args:    []string{"-1"},
			wantOut: "a\nb\n",
		},
		{
			name:    "-1 after -C wins",
			files:   fsx.MemFS{"a": {}, "b": {}},
			args:    []string{"-C", "-1"},
			wantOut: "a\nb\n",
		},
		{
			name:    "-C after -1 wins",
			files:   fsx.MemFS{"a": {}, "b": {}},
			args:    []string{"-1", "-C"},
			wantOut: "a  b\n",
		},
		{
			name:    "-C pads every column to the width of the longest name",
			files:   fsx.MemFS{"a": {}, "bb": {}, "cccc": {}},
			env:     map[string]string{"COLUMNS": "20"},
			args:    []string{"-C"},
			wantOut: "a     bb    cccc\n",
		},
		{
			name:    "-C writes one column when a name is wider than the width",
			files:   fsx.MemFS{"aaaaaaaa": {}, "b": {}},
			env:     map[string]string{"COLUMNS": "8"},
			args:    []string{"-C"},
			wantOut: "aaaaaaaa\nb\n",
		},
		{
			name:    "an invalid COLUMNS value falls back to the default width",
			files:   fsx.MemFS{"a": {}, "b": {}},
			env:     map[string]string{"COLUMNS": "banana"},
			args:    []string{"-C"},
			wantOut: "a  b\n",
		},
		{
			name:    "a zero COLUMNS value falls back to the default width",
			files:   fsx.MemFS{"a": {}, "b": {}},
			env:     map[string]string{"COLUMNS": "0"},
			args:    []string{"-C"},
			wantOut: "a  b\n",
		},
	})
}

func TestRunIndicators(t *testing.T) {
	now := newTimes()

	runCases(t, []testCase{
		{
			name: "-F marks the type of each file",
			files: fsx.MemFS{
				"dir":    {Mode: dirMode(0o755)},
				"fifo":   {Mode: fs.ModeNamedPipe | 0o644},
				"link":   link("dir"),
				"plain":  {Mode: 0o644},
				"script": {Mode: 0o755},
				"socket": {Mode: fs.ModeSocket | 0o644},
			},
			args: []string{"-F"},
			wantOut: "dir/\n" +
				"fifo|\n" +
				"link@\n" +
				"plain\n" +
				"script*\n" +
				"socket=\n",
		},
		{
			name:    "-p marks directories only",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"-p"},
			wantOut: "dir/\nscript\n",
		},
		{
			name:    "-F after -p writes every indicator",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"-p", "-F"},
			wantOut: "dir/\nscript*\n",
		},
		{
			name:    "-p after -F writes the directory indicator only",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"-F", "-p"},
			wantOut: "dir/\nscript\n",
		},
		{
			name: "-lp writes the link target without an indicator",
			files: fsx.MemFS{
				"dir":  {Mode: dirMode(0o755), ModTime: now.recent},
				"link": {Data: []byte("dir"), Mode: fs.ModeSymlink | 0o777, ModTime: now.recent},
			},
			args: []string{"-lp"},
			wantOut: "total 1\n" +
				"drwxr-xr-x 1 0 0 0 " + now.day(now.recent) + " dir/\n" +
				"lrwxrwxrwx 1 0 0 3 " + now.day(now.recent) + " link -> dir\n",
		},
		{
			name:    "-lF marks the link before its target",
			files:   fsx.MemFS{"link": {Data: []byte("dir"), Mode: fs.ModeSymlink | 0o777, ModTime: now.recent}},
			args:    []string{"-lF"},
			wantOut: "total 1\nlrwxrwxrwx 1 0 0 3 " + now.day(now.recent) + " link@ -> dir\n",
		},
		{
			name:    "-LF writes the indicator of the file a link refers to",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}, "link": link("dir")},
			args:    []string{"-L", "-F"},
			wantOut: "dir/\nlink/\n",
		},
	})
}

func TestRunQuote(t *testing.T) {
	now := newTimes()

	runCases(t, []testCase{
		{
			name:    "-q writes control characters as question marks",
			files:   fsx.MemFS{"a\nb": {}, "c\td": {}, "e": {}},
			args:    []string{"-q"},
			wantOut: "a?b\nc?d\ne\n",
		},
		{
			name:    "-q writes the long format with quoted names",
			files:   fsx.MemFS{"a\x01b": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-q", "-l"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a?b\n",
		},
		{
			name:    "without -q the name is written as it is",
			files:   fsx.MemFS{"a\nb": {}},
			args:    []string{"-1"},
			wantOut: "a\nb\n",
		},
		{
			name:    "-q writes the target of a link quoted",
			files:   fsx.MemFS{"link": linkAt("a\nb", now.recent)},
			args:    []string{"-l", "-q"},
			wantOut: "total 1\nlrwxrwxrwx 1 0 0 3 " + now.day(now.recent) + " link -> a?b\n",
		},
	})
}

func TestRunSort(t *testing.T) {
	now := newTimes()

	runCases(t, []testCase{
		{
			name: "-S sorts by size, largest first",
			files: fsx.MemFS{
				"small": {Data: []byte("x")},
				"big":   {Data: []byte("xxxxxxxxxx")},
				"zero":  {},
			},
			args:    []string{"-1", "-S"},
			wantOut: "big\nsmall\nzero\n",
		},
		{
			name: "-S sorts names up when the size is equal",
			files: fsx.MemFS{
				"b": {Data: []byte("x")},
				"a": {Data: []byte("x")},
			},
			args:    []string{"-1", "-S"},
			wantOut: "a\nb\n",
		},
		{
			name: "-t sorts by modification time, most recent first",
			files: fsx.MemFS{
				"old":  {ModTime: now.old},
				"new":  {ModTime: now.recent},
				"midd": {ModTime: now.recent.Add(-time.Hour)},
			},
			args:    []string{"-1", "-t"},
			wantOut: "new\nmidd\nold\n",
		},
		{
			name: "-t sorts names up when the time is equal",
			files: fsx.MemFS{
				"b": {ModTime: now.recent},
				"a": {ModTime: now.recent},
			},
			args:    []string{"-1", "-t"},
			wantOut: "a\nb\n",
		},
		{
			name:    "-r reverses the order of the names",
			files:   fsx.MemFS{"a": {}, "b": {}, "c": {}},
			args:    []string{"-1", "-r"},
			wantOut: "c\nb\na\n",
		},
		{
			name:    "-r reverses the order of the sizes",
			files:   fsx.MemFS{"big": {Data: []byte("xxx")}, "small": {Data: []byte("x")}},
			args:    []string{"-1", "-S", "-r"},
			wantOut: "small\nbig\n",
		},
		{
			name:    "-r reverses the order of the times",
			files:   fsx.MemFS{"new": {ModTime: now.recent}, "old": {ModTime: now.old}},
			args:    []string{"-1", "-t", "-r"},
			wantOut: "old\nnew\n",
		},
		{
			name:    "-S after -t wins the sort key",
			files:   fsx.MemFS{"big": {Data: []byte("xxx"), ModTime: now.old}, "small": {Data: []byte("x"), ModTime: now.recent}},
			args:    []string{"-1", "-t", "-S"},
			wantOut: "big\nsmall\n",
		},
		{
			name:    "-t after -S wins the sort key",
			files:   fsx.MemFS{"big": {Data: []byte("xxx"), ModTime: now.old}, "small": {Data: []byte("x"), ModTime: now.recent}},
			args:    []string{"-1", "-S", "-t"},
			wantOut: "small\nbig\n",
		},
		{
			name: "-t with -c sorts by the status change time",
			files: fsx.MemFS{
				"a.txt": {ModTime: now.recent},
				"b.txt": {ModTime: now.old},
			},
			args:    []string{"-1", "-t", "-c"},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "-f writes the entries in the order of the directory",
			files:   fsx.MemFS{"b.txt": {}, "a.txt": {}, ".hidden": {}},
			args:    []string{"-f"},
			wantOut: ".hidden\na.txt\nb.txt\n",
		},
		{
			name:    "-f ignores -r",
			files:   fsx.MemFS{"b.txt": {}, "a.txt": {}},
			args:    []string{"-f", "-r"},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "-f ignores -S",
			files:   fsx.MemFS{"a.txt": {Data: []byte("x")}, "b.txt": {}},
			args:    []string{"-S", "-f"},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "-f ignores -t",
			files:   fsx.MemFS{"a.txt": {ModTime: now.old}, "b.txt": {ModTime: now.recent}},
			args:    []string{"-t", "-f"},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "-f keeps the long format",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-f", "-l"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name: "-S uses the size of the link itself",
			files: fsx.MemFS{
				"dir":  {Mode: dirMode(0o755)},
				"link": link("dir"),
				"big":  {Data: []byte(strings.Repeat("x", 100))},
			},
			args:    []string{"-1", "-S"},
			wantOut: "big\nlink\ndir\n",
		},
	})
}

func TestRunSymbolicLinks(t *testing.T) {
	now := newTimes()

	files := fsx.MemFS{
		"dir":      {Mode: dirMode(0o755), ModTime: now.recent},
		"dir/in":   {Mode: 0o644, ModTime: now.recent},
		"file":     {Mode: 0o644, ModTime: now.recent},
		"to-dir":   linkAt("dir", now.recent),
		"to-file":  linkAt("file", now.recent),
		"broken":   linkAt("nope", now.recent),
		"sub":      {Mode: dirMode(0o755), ModTime: now.recent},
		"sub/link": linkAt("dir", now.recent),
	}

	runCases(t, []testCase{
		{
			name:    "an operand naming a link to a directory is listed",
			files:   files,
			args:    []string{"to-dir"},
			wantOut: "in\n",
		},
		{
			name:    "an operand naming a link to a file is written as itself",
			files:   files,
			args:    []string{"to-file"},
			wantOut: "to-file\n",
		},
		{
			name:    "an operand naming a broken link is written as itself",
			files:   files,
			args:    []string{"broken"},
			wantOut: "broken\n",
		},
		{
			name:    "-l writes an operand link as itself",
			files:   files,
			args:    []string{"-l", "to-dir"},
			wantOut: "lrwxrwxrwx 1 0 0 3 " + now.day(now.recent) + " to-dir -> dir\n",
		},
		{
			name:    "-p lists the contents of an operand link to a directory",
			files:   files,
			args:    []string{"-p", "to-dir"},
			wantOut: "in\n",
		},
		{
			name:    "-p lists the contents of an operand link to a directory",
			files:   fsx.MemFS{"dir/in": {Mode: 0o644, ModTime: now.recent}, "to-dir": link("dir")},
			args:    []string{"-p", "to-dir"},
			wantOut: "in\n",
		},
		{
			name:    "-d writes an operand link as itself",
			files:   files,
			args:    []string{"-d", "to-dir"},
			wantOut: "to-dir\n",
		},
		{
			name:    "-F writes an operand link as itself",
			files:   files,
			args:    []string{"-F", "to-dir"},
			wantOut: "to-dir@\n",
		},
		{
			name:    "the links of a listing are written as themselves",
			files:   files,
			args:    []string{"-1", "sub"},
			wantOut: "link\n",
		},
		{
			name:    "-l writes a listing link with its target",
			files:   files,
			args:    []string{"-l", "sub"},
			wantOut: "total 1\nlrwxrwxrwx 1 0 0 3 " + now.day(now.recent) + " link -> dir\n",
		},
		{
			name:    "-H lists the contents of an operand link to a directory",
			files:   files,
			args:    []string{"-l", "-H", "to-dir"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " in\n",
		},
		{
			name:    "-H lists an operand link to a directory",
			files:   files,
			args:    []string{"-1", "-H", "to-dir"},
			wantOut: "in\n",
		},
		{
			name:    "-H doesn't evaluate the links of a listing",
			files:   files,
			args:    []string{"-l", "-H", "sub"},
			wantOut: "total 1\nlrwxrwxrwx 1 0 0 3 " + now.day(now.recent) + " link -> dir\n",
		},
		{
			name:     "-H reports a broken operand link",
			files:    files,
			args:     []string{"-l", "-H", "broken"},
			wantOut:  "lrwxrwxrwx 1 0 0 4 " + now.day(now.recent) + " broken -> nope\n",
			wantErr:  "ls: cannot access 'broken': file does not exist\n",
			wantExit: 1,
		},
		{
			name:  "-lL writes the information of the file each link refers to",
			files: files,
			args:  []string{"-l", "-L"},
			wantOut: "total 1\n" +
				"lrwxrwxrwx 1 0 0 4 " + now.day(now.recent) + " broken -> nope\n" +
				"drwxr-xr-x 1 0 0 0 " + now.day(now.recent) + " dir\n" +
				"-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " file\n" +
				"drwxr-xr-x 1 0 0 0 " + now.day(now.recent) + " sub\n" +
				"drwxr-xr-x 1 0 0 0 " + now.day(now.recent) + " to-dir\n" +
				"-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " to-file\n",
			wantErr:  "ls: cannot access 'broken': file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "-L writes the listing of an operand link without the link",
			files:   files,
			args:    []string{"-l", "-L", "to-dir"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " in\n",
		},
		{
			name:     "-L reports a broken link of a listing",
			files:    files,
			args:     []string{"-1", "-L"},
			wantOut:  "broken\ndir\nfile\nsub\nto-dir\nto-file\n",
			wantErr:  "ls: cannot access 'broken': file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "-H after -L wins for the operands",
			files:   files,
			args:    []string{"-l", "-L", "-H", "to-dir"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " in\n",
		},
		{
			name:     "-P is not a POSIX option",
			files:    files,
			args:     []string{"-L", "-P"},
			wantErr:  "ls: invalid option -- 'P'\n",
			wantExit: 2,
		},
	})
}

func TestRunDirectories(t *testing.T) {
	now := newTimes()

	files := fsx.MemFS{
		"a.txt":         {Mode: 0o644, ModTime: now.recent},
		"dir":           {Mode: dirMode(0o755), ModTime: now.recent},
		"dir/b.txt":     {Mode: 0o644, ModTime: now.recent},
		"dir/sub":       {Mode: dirMode(0o755), ModTime: now.recent},
		"dir/sub/c.txt": {Mode: 0o644, ModTime: now.recent},
		"dir/.hidden":   {Mode: 0o644, ModTime: now.recent},
		"other":         {Mode: dirMode(0o755), ModTime: now.recent},
		"other/d.txt":   {Mode: 0o644, ModTime: now.recent},
		".dotdir":       {Mode: dirMode(0o755), ModTime: now.recent},
		".dotdir/e.txt": {Mode: 0o644, ModTime: now.recent},
	}

	runCases(t, []testCase{
		{
			name:    "-d writes a directory as a file",
			files:   files,
			args:    []string{"-d", "dir"},
			wantOut: "dir\n",
		},
		{
			name:    "-d writes the working directory as a file",
			files:   files,
			args:    []string{"-d"},
			wantOut: ".\n",
		},
		{
			name:    "-ld writes the long line of a directory",
			files:   files,
			args:    []string{"-ld", "dir"},
			wantOut: "drwxr-xr-x 1 0 0 0 " + now.day(now.recent) + " dir\n",
		},
		{
			name:    "-d after -R wins",
			files:   files,
			args:    []string{"-R", "-d", "dir"},
			wantOut: "dir\n",
		},
		{
			name:    "-R after -d wins",
			files:   files,
			args:    []string{"-d", "-R", "dir"},
			wantOut: "dir:\nb.txt\nsub\n\ndir/sub:\nc.txt\n",
		},
		{
			name:    "-R writes the header of a single directory operand",
			files:   files,
			args:    []string{"-1", "-R", "dir"},
			wantOut: "dir:\nb.txt\nsub\n\ndir/sub:\nc.txt\n",
		},
		{
			name:    "-R writes the subdirectory header below the operand name",
			files:   files,
			args:    []string{"-1", "-R", "./dir"},
			wantOut: "./dir:\nb.txt\nsub\n\n./dir/sub:\nc.txt\n",
		},
		{
			name:    "-R without operands writes no header for the working directory",
			files:   files,
			args:    []string{"-1", "-R"},
			wantOut: "a.txt\ndir\nother\n\n./dir:\nb.txt\nsub\n\n./dir/sub:\nc.txt\n\n./other:\nd.txt\n",
		},
		{
			name:    "-R doesn't recurse into a directory that wasn't listed",
			files:   files,
			args:    []string{"-1", "-R", "."},
			wantOut: ".:\na.txt\ndir\nother\n\n./dir:\nb.txt\nsub\n\n./dir/sub:\nc.txt\n\n./other:\nd.txt\n",
		},
		{
			name:    "-Ra recurses into the hidden directories",
			files:   files,
			args:    []string{"-1", "-Ra", "."},
			wantOut: ".:\n.dotdir\na.txt\ndir\nother\n\n./.dotdir:\ne.txt\n\n./dir:\n.hidden\nb.txt\nsub\n\n./dir/sub:\nc.txt\n\n./other:\nd.txt\n",
		},
		{
			name:    "-R doesn't recurse into a link to a directory",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}, "dir/a.txt": {}, "link": link("dir")},
			args:    []string{"-1", "-R"},
			wantOut: "dir\nlink\n\n./dir:\na.txt\n",
		},
		{
			name:    "-LR recurses into a link to a directory",
			files:   fsx.MemFS{"dir/a.txt": {}, "link": link("dir")},
			args:    []string{"-1", "-L", "-R"},
			wantOut: "dir\nlink\n\n./dir:\na.txt\n\n./link:\na.txt\n",
		},
		{
			name:    "multiple operands write the files first",
			files:   files,
			args:    []string{"-1", "dir", "a.txt", "other"},
			wantOut: "a.txt\n\ndir:\nb.txt\nsub\n\nother:\nd.txt\n",
		},
		{
			name:     "multiple operands are sorted separately",
			files:    files,
			args:     []string{"-1", "other", "dir", "z.txt", "a.txt"},
			wantOut:  "a.txt\n\ndir:\nb.txt\nsub\n\nother:\nd.txt\n",
			wantErr:  "ls: cannot access 'z.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "each directory operand gets a header",
			files:   files,
			args:    []string{"-1", "dir", "dir"},
			wantOut: "dir:\nb.txt\nsub\n\ndir:\nb.txt\nsub\n",
		},
		{
			name:     "a failed operand doesn't stop the other listings",
			files:    files,
			args:     []string{"-1", "nope", "dir"},
			wantOut:  "dir:\nb.txt\nsub\n",
			wantErr:  "ls: cannot access 'nope': file does not exist\n",
			wantExit: 1,
		},
		{
			name:  "-lR writes a total for each listing",
			files: fsx.MemFS{"dir": {Mode: dirMode(0o755), ModTime: now.recent}, "dir/sub": {Mode: dirMode(0o755), ModTime: now.recent}, "dir/sub/a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:  []string{"-l", "-R", "dir"},
			wantOut: "dir:\ntotal 0\ndrwxr-xr-x 1 0 0 0 " + now.day(now.recent) + " sub\n" +
				"\ndir/sub:\ntotal 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
	})
}

func TestRunOptions(t *testing.T) {
	now := newTimes()

	runCases(t, []testCase{
		{
			name:     "an invalid option is reported",
			files:    fsx.MemFS{"a.txt": {}},
			args:     []string{"-Z"},
			wantErr:  "ls: invalid option -- 'Z'\n",
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option is reported",
			files:    fsx.MemFS{"a.txt": {}},
			args:     []string{"--bogus"},
			wantErr:  "ls: unrecognized option '--bogus'\n",
			wantExit: 2,
		},
		{
			name:    "the operands may begin after a double dash",
			files:   fsx.MemFS{"-a": {}, "b.txt": {}},
			args:    []string{"--", "-a"},
			wantOut: "-a\n",
		},
		{
			name:    "a single dash is an operand",
			files:   fsx.MemFS{"-": {}},
			args:    []string{"-"},
			wantOut: "-\n",
		},
		{
			name:    "an operand keeps the options after it from parsing",
			files:   fsx.MemFS{"a.txt": {}, "-l": {}},
			args:    []string{"a.txt", "-l"},
			wantOut: "-l\na.txt\n",
		},
		{
			name:    "bundled options are parsed one by one",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-lq", "a.txt"},
			wantOut: "-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:     "an invalid option of a bundle is reported",
			files:    fsx.MemFS{},
			args:     []string{"-lZ"},
			wantErr:  "ls: invalid option -- 'Z'\n",
			wantExit: 2,
		},
		{
			name:     "without a file system nothing runs",
			files:    nil,
			args:     []string{"a.txt"},
			wantErr:  "ls: no file system available\n",
			wantExit: 1,
		},
	})
}

func TestRunLongOptions(t *testing.T) {
	now := newTimes()

	linked := fsx.MemFS{
		"dir":    {Mode: dirMode(0o755)},
		"dir/in": {Mode: 0o644},
		"to-dir": link("dir"),
	}

	runCases(t, []testCase{
		{
			name:    "all writes the entries that begin with a dot",
			files:   fsx.MemFS{".hidden": {}, "a.txt": {}},
			args:    []string{"--all"},
			wantOut: ".hidden\na.txt\n",
		},
		{
			name:    "almost-all writes the hidden entries",
			files:   fsx.MemFS{".hidden": {}, "a.txt": {}},
			args:    []string{"--almost-all"},
			wantOut: ".hidden\na.txt\n",
		},
		{
			name:    "classify marks the type of each file",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"--classify"},
			wantOut: "dir/\nscript*\n",
		},
		{
			name:    "dereference writes the indicator of the file a link refers to",
			files:   linked,
			args:    []string{"--dereference", "--classify"},
			wantOut: "dir/\nto-dir/\n",
		},
		{
			name:    "dereference-command-line lists the contents of an operand link to a directory",
			files:   linked,
			args:    []string{"--dereference-command-line", "to-dir"},
			wantOut: "in\n",
		},
		{
			name:    "directory writes a directory as a file",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}, "dir/b.txt": {}},
			args:    []string{"--directory", "dir"},
			wantOut: "dir\n",
		},
		{
			name:    "hide-control-chars writes control characters as question marks",
			files:   fsx.MemFS{"a\nb": {}, "e": {}},
			args:    []string{"--hide-control-chars"},
			wantOut: "a?b\ne\n",
		},
		{
			name:    "inode writes the file serial number",
			files:   fsx.MemFS{"a.txt": {}},
			args:    []string{"--inode"},
			wantOut: "0 a.txt\n",
		},
		{
			name:    "kibibytes counts blocks in kibibytes",
			files:   fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:    []string{"--kibibytes", "--size"},
			wantOut: "total 1\n1 a.txt\n",
		},
		{
			name:    "numeric-uid-gid writes the identifiers as numbers",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"--numeric-uid-gid"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "recursive lists the directories below the operand",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}, "dir/b.txt": {}, "dir/sub": {Mode: dirMode(0o755)}, "dir/sub/c.txt": {}},
			args:    []string{"--recursive", "dir"},
			wantOut: "dir:\nb.txt\nsub\n\ndir/sub:\nc.txt\n",
		},
		{
			name:    "reverse reverses the order of the names",
			files:   fsx.MemFS{"a": {}, "b": {}, "c": {}},
			args:    []string{"--reverse"},
			wantOut: "c\nb\na\n",
		},
		{
			name:    "size writes the blocks and their total",
			files:   fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:    []string{"--size"},
			wantOut: "total 1\n1 a.txt\n",
		},
		{
			name:    "format long writes the long format",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"--format=long"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "format single-column disables the long format",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-l", "--format=single-column"},
			wantOut: "a.txt\n",
		},
		{
			name:    "format long after single-column writes the long format again",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"--format=single-column", "--format=long"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "format takes its value in the next argument",
			files:   fsx.MemFS{"a": {}, "b": {}},
			args:    []string{"--format", "single-column"},
			wantOut: "a\nb\n",
		},
		{
			name:    "format vertical writes entries down the columns",
			files:   fsx.MemFS{"a": {}, "b": {}, "c": {}, "d": {}},
			env:     map[string]string{"COLUMNS": "80"},
			args:    []string{"--format=vertical"},
			wantOut: "a  b  c  d\n",
		},
		{
			name:    "format across writes entries across the columns",
			files:   fsx.MemFS{"a": {}, "b": {}, "c": {}, "d": {}},
			env:     map[string]string{"COLUMNS": "10"},
			args:    []string{"--format=across"},
			wantOut: "a  b  c\nd\n",
		},
		{
			name:    "format commas writes the names separated by commas",
			files:   fsx.MemFS{"a": {}, "b": {}, "c": {}},
			args:    []string{"--format=commas"},
			wantOut: "a, b, c\n",
		},
		{
			name:    "sort size sorts by size, largest first",
			files:   fsx.MemFS{"small": {Data: []byte("x")}, "big": {Data: []byte("xxxxxxxxxx")}, "zero": {}},
			args:    []string{"--sort=size"},
			wantOut: "big\nsmall\nzero\n",
		},
		{
			name:    "sort time sorts by modification time, most recent first",
			files:   fsx.MemFS{"old": {ModTime: now.old}, "new": {ModTime: now.recent}},
			args:    []string{"--sort=time"},
			wantOut: "new\nold\n",
		},
		{
			name:    "sort name keeps the order of the names",
			files:   fsx.MemFS{"b": {Data: []byte("xxx")}, "a": {}},
			args:    []string{"--sort=name"},
			wantOut: "a\nb\n",
		},
		{
			name:    "sort none keeps the order of the directory without showing hidden entries",
			files:   fsx.MemFS{"b": {Data: []byte("xxx")}, "a": {}, ".hidden": {}},
			args:    []string{"--sort=none"},
			wantOut: "a\nb\n",
		},
		{
			name:    "time atime writes the access time",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.old}},
			args:    []string{"--format=long", "--time=atime"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.year(now.old) + " a.txt\n",
		},
		{
			name:    "time ctime writes the status change time",
			files:   fsx.MemFS{"a.txt": {Mode: 0o644, ModTime: now.old}},
			args:    []string{"--format=long", "--time=ctime"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.year(now.old) + " a.txt\n",
		},
		{
			name:    "indicator-style slash marks directories only",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"--indicator-style=slash"},
			wantOut: "dir/\nscript\n",
		},
		{
			name:    "indicator-style classify marks the type of each file",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"--indicator-style=classify"},
			wantOut: "dir/\nscript*\n",
		},
		{
			name:    "indicator-style none writes no indicator",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"--classify", "--indicator-style=none"},
			wantOut: "dir\nscript\n",
		},
		{
			name:     "an unknown format word is an error",
			files:    fsx.MemFS{},
			args:     []string{"--format=bogus"},
			wantErr:  "ls: invalid argument 'bogus' for '--format'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown sort word is an error",
			files:    fsx.MemFS{},
			args:     []string{"--sort=bogus"},
			wantErr:  "ls: invalid argument 'bogus' for '--sort'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown time word is an error",
			files:    fsx.MemFS{},
			args:     []string{"--time=bogus"},
			wantErr:  "ls: invalid argument 'bogus' for '--time'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown indicator-style word is an error",
			files:    fsx.MemFS{},
			args:     []string{"--indicator-style=bogus"},
			wantErr:  "ls: invalid argument 'bogus' for '--indicator-style'\n",
			wantExit: 2,
		},
		{
			name:     "a valued long option requires an argument",
			files:    fsx.MemFS{},
			args:     []string{"--format"},
			wantErr:  "ls: option '--format' requires an argument\n",
			wantExit: 2,
		},
		{
			name:     "a long option with a value it doesn't take is an error",
			files:    fsx.MemFS{},
			args:     []string{"--all=yes"},
			wantErr:  "ls: option '--all=yes' doesn't allow an argument\n",
			wantExit: 2,
		},
	})
}

func TestRunMultipleOperands(t *testing.T) {
	now := newTimes()

	files := fsx.MemFS{
		"z.txt":   {Mode: 0o644, ModTime: now.recent},
		"a.txt":   {Mode: 0o644, ModTime: now.recent},
		"dir":     {Mode: dirMode(0o755), ModTime: now.recent},
		"dir/f":   {Mode: 0o644, ModTime: now.recent},
		"other":   {Mode: dirMode(0o755), ModTime: now.recent},
		"other/g": {Mode: 0o644, ModTime: now.recent},
	}

	runCases(t, []testCase{
		{
			name:    "the file operands share a column listing",
			files:   files,
			args:    []string{"-C", "z.txt", "a.txt"},
			wantOut: "a.txt  z.txt\n",
		},
		{
			name:  "the long format writes no total for file operands",
			files: files,
			args:  []string{"-l", "z.txt", "a.txt"},
			wantOut: "-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n" +
				"-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " z.txt\n",
		},
		{
			name:    "-d writes the directory operands without headers",
			files:   files,
			args:    []string{"-d", "other", "dir", "a.txt"},
			wantOut: "a.txt\ndir\nother\n",
		},
		{
			name:    "a directory operand is listed after the files",
			files:   files,
			args:    []string{"-1", "a.txt", "dir"},
			wantOut: "a.txt\n\ndir:\nf\n",
		},
		{
			name:    "a directory operand gets its own header",
			files:   files,
			args:    []string{"-1", "other", "dir"},
			wantOut: "dir:\nf\n\nother:\ng\n",
		},
	})
}

// TestRunRealFS checks the listing of the information that only a real file
// system reports.
func TestRunRealFS(t *testing.T) {
	sys := fsx.OsFS(t.TempDir())
	if err := sys.Mkdir("dir", 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	for _, name := range []string{"a.txt", "dir/b.txt"} {
		f, err := sys.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("OpenFile(%q) error = %v", name, err)
		}
		if _, err := f.Write([]byte("hello")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}

	old := time.Now().AddDate(0, -8, 0)
	if err := sys.Chtimes("a.txt", old, old); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	// list runs the utility against the real file system and returns what it
	// wrote to the standard output.
	list := func(t *testing.T, args ...string) string {
		t.Helper()
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdout:    stdout,
			Stderr:    stderr,
			FS:        sys,
			LookupEnv: envLookup(nil),
		}
		if err := New(rt).Run(t.Context(), args...); err != nil {
			t.Fatalf("Run() error = %v, stderr = %q", err, stderr)
		}
		return stdout.String()
	}

	tests := []struct {
		name        string
		args        []string
		want        string
		wantWindows string // the expectation on windows, when it differs
		needsChange bool   // whether the file system has to report a change time
	}{
		{
			name: "writes the mode and the link count of the file",
			args: []string{"-l", "a.txt"},
			want: "-rw-r--r-- 1 ",
			// Windows keeps no permission bits, so a writable file is 0666.
			wantWindows: "-rw-rw-rw- 1 ",
		},
		{
			name: "writes the modification time of the file",
			args: []string{"-l", "a.txt"},
			want: old.Format("Jan _2  2006"),
		},
		{
			name:        "writes the status change time with -c",
			args:        []string{"-l", "-c", "a.txt"},
			want:        time.Now().Format("Jan _2 15:04"),
			needsChange: true,
		},
		{
			name: "writes a serial number before the name",
			args: []string{"-i", "-1", "a.txt"},
			want: " a.txt\n",
		},
		{
			name: "writes the entries of a subdirectory",
			args: []string{"-1", "-R", "."},
			want: "./dir:\nb.txt",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.needsChange && runtime.GOOS == "windows" {
				t.Skip("windows reports no status change time")
			}
			want := test.want
			if runtime.GOOS == "windows" && test.wantWindows != "" {
				want = test.wantWindows
			}
			if got := list(t, test.args...); !strings.Contains(got, want) {
				t.Errorf("Run() stdout = %q, want it to hold %q", got, want)
			}
		})
	}

	t.Run("writes the serial number a unix file system reports", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("windows reports no serial number for a file")
		}
		if got := list(t, "-i", "-1", "a.txt"); strings.HasPrefix(got, "0 ") {
			t.Errorf("Run() stdout = %q, want the serial number of the file", got)
		}
	})
}

// TestRunUnreadable checks the diagnostic of a directory that can't be read.
func TestRunUnreadable(t *testing.T) {
	runCases(t, []testCase{
		{
			name:     "reports a directory that can't be opened",
			sys:      unreadableFS{FS: fsx.MemFS{"hidden/a.txt": {}, "a.txt": {}}, denied: "hidden"},
			args:     []string{"hidden"},
			wantErr:  "ls: cannot open directory 'hidden': permission denied\n",
			wantExit: 1,
		},
		{
			name:     "lists the other operands after the failure",
			sys:      unreadableFS{FS: fsx.MemFS{"hidden/a.txt": {}, "dir/b.txt": {}}, denied: "hidden"},
			args:     []string{"-1", "dir", "hidden"},
			wantOut:  "dir:\nb.txt\n\nhidden:\n",
			wantErr:  "ls: cannot open directory 'hidden': permission denied\n",
			wantExit: 1,
		},
		{
			name:     "reports a directory of a recursive listing",
			sys:      unreadableFS{FS: fsx.MemFS{"dir/hidden/a.txt": {}}, denied: "dir/hidden"},
			args:     []string{"-1", "-R", "dir"},
			wantOut:  "dir:\nhidden\n\ndir/hidden:\n",
			wantErr:  "ls: cannot open directory 'dir/hidden': permission denied\n",
			wantExit: 1,
		},
	})
}

// unreadableFS is a file system that denies reading one of its directories.
type unreadableFS struct {
	fsx.FS
	denied string
}

// ReadDir implements the fs.ReadDirFS of the denied directory.
func (f unreadableFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == f.denied {
		return nil, fs.ErrPermission
	}
	return fs.ReadDir(f.FS, name)
}

// TestRunHiddenRecursion checks which directories a recursive listing enters.
func TestRunHiddenRecursion(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "-R doesn't enter a hidden directory that wasn't listed",
			files:   fsx.MemFS{"dir/sub/a.txt": {}, "dir/.hidden/b.txt": {}},
			args:    []string{"-1", "-R", "dir"},
			wantOut: "dir:\nsub\n\ndir/sub:\na.txt\n",
		},
		{
			name:    "-aR enters the hidden directories it listed",
			files:   fsx.MemFS{"dir/sub/a.txt": {}, "dir/.hidden/b.txt": {}},
			args:    []string{"-1", "-aR"},
			wantOut: "dir\n\n./dir:\n.hidden\nsub\n\n./dir/.hidden:\nb.txt\n\n./dir/sub:\na.txt\n",
		},
		{
			name:    "-R enters a directory named as an operand",
			files:   fsx.MemFS{"dir/sub/a.txt": {}},
			args:    []string{"-1", "-R", "dir/sub"},
			wantOut: "dir/sub:\na.txt\n",
		},
	})
}

// TestRunFastOrder checks that -f keeps the order a real directory reports.
func TestRunFastOrder(t *testing.T) {
	dir := t.TempDir()
	sys := fsx.OsFS(dir)
	for _, name := range []string{"z.txt", "a.txt", "m.txt"} {
		f, err := sys.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("OpenFile(%q) error = %v", name, err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}

	file, err := os.Open(dir)
	if err != nil {
		t.Fatalf("os.Open() error = %v", err)
	}
	defer file.Close()
	raw, err := file.ReadDir(-1)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	names := make([]string, len(raw))
	for i, entry := range raw {
		names[i] = entry.Name()
	}

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout:    stdout,
		Stderr:    stderr,
		FS:        sys,
		LookupEnv: envLookup(nil),
	}
	if err := New(rt).Run(t.Context(), "-f"); err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", err, stderr)
	}
	if got, want := stdout.String(), strings.Join(names, "\n")+"\n"; got != want {
		t.Errorf("Run() stdout = %q, want the order of the directory %q", got, want)
	}
}

// TestRunCycle checks that a recursive listing of symbolic links doesn't
// follow one back into a directory it already entered.
func TestRunCycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the file system here reports no serial number to detect a cycle with")
	}

	sys := fsx.OsFS(t.TempDir())
	if err := sys.Mkdir("x", 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := sys.Mkdir("x/y", 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := sys.Symlink("..", "x/y/back"); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout:    stdout,
		Stderr:    stderr,
		FS:        sys,
		LookupEnv: envLookup(nil),
	}
	err := New(rt).Run(t.Context(), "-1", "-L", "-R", "x")
	if code := exitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if got, want := stdout.String(), "x:\ny\n\nx/y:\nback\n"; got != want {
		t.Errorf("Run() stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "ls: x/y/back: directory causes a cycle\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}

// TestRunWindows checks the listing of paths written the Windows way.
func TestRunWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("this test only makes sense on Windows")
	}

	runCases(t, []testCase{
		{
			name:    "an operand with backslashes names a directory",
			files:   fsx.MemFS{"dir/a.txt": {}, "dir/b.txt": {}},
			args:    []string{`dir\`},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "an operand with backslashes names a file",
			files:   fsx.MemFS{"dir/a.txt": {}},
			args:    []string{`dir\a.txt`},
			wantOut: "dir\\a.txt\n",
		},
	})
}

// TestRunContext checks that a listing stops with the context it's given.
func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rt := &coreutils.Runtime{
		Stdout:    io.Discard,
		Stderr:    io.Discard,
		FS:        fsx.MemFS{"a.txt": {}},
		LookupEnv: envLookup(nil),
	}
	if err := New(rt).Run(ctx, "a.txt"); !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}
