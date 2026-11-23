package ls

import (
	"bytes"
	"fmt"
	"io/fs"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// dirMode returns the mode of a directory with the given permissions.
func dirMode(perm fs.FileMode) fs.FileMode { return fs.ModeDir | perm }

// link returns a symbolic entry pointing at target.
func link(target string) *fsx.MemoryFile {
	return &fsx.MemoryFile{Data: []byte(target), Mode: fs.ModeSymlink | 0o777}
}

// linkAt returns a symbolic entry pointing at target, made at the given time.
func linkAt(target string, mod time.Time) *fsx.MemoryFile {
	return &fsx.MemoryFile{
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
	files    fsx.MemoryFS
	sys      fsx.FS
	wd       string
	env      map[string]string
	terminal bool
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
		Environ:    func() []string { return testutil.Environ(test.env) },
		IsTerminal: func(fd int) bool { return test.terminal && fd == 1 },
	}
	switch {
	case test.sys != nil:
		rt.FS = test.sys
	case test.files != nil:
		rt.FS = test.files
	}

	err := New(rt).Run(t.Context(), test.args...)

	if code := testutil.ExitCode(t, err); code != test.wantExit {
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

func TestRunDefault(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "lists the working directory when no operand is given",
			files:   fsx.MemoryFS{"b.txt": {}, "a.txt": {}},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "writes one entry per line by default",
			files:   fsx.MemoryFS{"a.txt": {}, "b.txt": {}},
			args:    []string{"."},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "hides the entries that begin with a dot",
			files:   fsx.MemoryFS{".hidden": {}, "a.txt": {}},
			wantOut: "a.txt\n",
		},
		{
			name:    "writes an operand naming a hidden file",
			files:   fsx.MemoryFS{".hidden": {}},
			args:    []string{".hidden"},
			wantOut: ".hidden\n",
		},
		{
			name:    "lists the contents of a directory operand",
			files:   fsx.MemoryFS{"dir/a.txt": {}, "dir/b.txt": {}},
			args:    []string{"dir"},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "sorts entries by their bytes",
			files:   fsx.MemoryFS{"b.txt": {}, "C.txt": {}, "a.txt": {}},
			wantOut: "C.txt\na.txt\nb.txt\n",
		},
		{
			name:    "writes nothing for an empty directory",
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}},
			args:    []string{"dir"},
			wantOut: "",
		},
		{
			name:    "resolves an operand against the working directory",
			files:   fsx.MemoryFS{"work/a.txt": {}, "a.txt": {}},
			wd:      "work",
			args:    []string{"a.txt"},
			wantOut: "a.txt\n",
		},
		{
			name:    "lists the working directory of an operand ending in a separator",
			files:   fsx.MemoryFS{"dir/a.txt": {}},
			args:    []string{"dir/"},
			wantOut: "a.txt\n",
		},
		{
			name:    "lists a directory named through redundant separators",
			files:   fsx.MemoryFS{"dir/a.txt": {}, "other/a.txt": {}},
			args:    []string{"./dir/./"},
			wantOut: "a.txt\n",
		},
		{
			name:     "reports a missing operand",
			files:    fsx.MemoryFS{},
			args:     []string{"nope"},
			wantErr:  "ls: cannot access 'nope': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "lists the other operands after a failure",
			files:    fsx.MemoryFS{"a.txt": {}},
			args:     []string{"nope", "a.txt"},
			wantOut:  "a.txt\n",
			wantErr:  "ls: cannot access 'nope': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "reports a zero-length operand",
			files:    fsx.MemoryFS{},
			args:     []string{""},
			wantErr:  "ls: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:    "writes the name of a file operand as specified",
			files:   fsx.MemoryFS{"dir/a.txt": {}},
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
			files:   fsx.MemoryFS{".hidden": {}, "a.txt": {}},
			args:    []string{"-a"},
			wantOut: ".hidden\na.txt\n",
		},
		{
			name:    "-A writes the hidden entries",
			files:   fsx.MemoryFS{".hidden": {}, "a.txt": {}},
			args:    []string{"-A"},
			wantOut: ".hidden\na.txt\n",
		},
		{
			name:    "-a writes the long listing of a hidden entry",
			files:   fsx.MemoryFS{".hidden": {Mode: 0o600, ModTime: now.recent}},
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
			files: fsx.MemoryFS{
				"dir":   {Mode: dirMode(0o755), ModTime: now.recent},
				"f.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: now.recent},
			},
			args: []string{"-l"},
			wantOut: "total 1\n" +
				"drwxr-xr-x 1 0 0 0 " + now.day(now.recent) + " dir\n" +
				"-rw-r--r-- 1 0 0 5 " + now.day(now.recent) + " f.txt\n",
		},
		{
			name: "-l aligns the fields of the listing",
			files: fsx.MemoryFS{
				"one":   {Data: []byte("x"), Mode: 0o644, ModTime: now.recent},
				"three": {Data: []byte(strings.Repeat("x", 10000)), Mode: 0o444, ModTime: now.recent},
				"two":   {Data: []byte(strings.Repeat("x", 1000)), Mode: 0o644, ModTime: now.recent},
			},
			args: []string{"-l"},
			wantOut: "total 12\n" +
				"-rw-r--r-- 1 0 0     1 " + now.day(now.recent) + " one\n" +
				"-r--r--r-- 1 0 0 10000 " + now.day(now.recent) + " three\n" +
				"-rw-r--r-- 1 0 0  1000 " + now.day(now.recent) + " two\n",
		},
		{
			name:    "-l writes the year for an old modification time",
			files:   fsx.MemoryFS{"old.txt": {Mode: 0o644, ModTime: now.old}},
			args:    []string{"-l"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.year(now.old) + " old.txt\n",
		},
		{
			name:    "-l writes the year for a future modification time",
			files:   fsx.MemoryFS{"soon.txt": {Mode: 0o644, ModTime: now.future}},
			args:    []string{"-l"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.year(now.future) + " soon.txt\n",
		},
		{
			name: "-l writes every permission flag",
			files: fsx.MemoryFS{
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
			files: fsx.MemoryFS{
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
			files:   fsx.MemoryFS{"dir/a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-l", "dir/a.txt"},
			wantOut: "-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " dir/a.txt\n",
		},
		{
			name:    "-l of an empty directory writes only the total",
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}},
			args:    []string{"-l", "dir"},
			wantOut: "total 0\n",
		},
		{
			name:    "-C disables the long format",
			files:   fsx.MemoryFS{"a.txt": {}, "b.txt": {}},
			args:    []string{"-l", "-C"},
			wantOut: "a.txt  b.txt\n",
		},
		{
			name:    "-l after -C writes the long format again",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-C", "-l"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-1 keeps the long format",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-l", "-1"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-n writes the identifiers as numbers",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-n"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-g omits the owner",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-g"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-o omits the group",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-o"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-go omits both",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-go"},
			wantOut: "total 0\n-rw-r--r-- 1 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-li writes the file serial number first",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-li"},
			wantOut: "total 0\n0 -rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-ls writes the block count before the line",
			files:   fsx.MemoryFS{"a.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-ls"},
			wantOut: "total 1\n1 -rw-r--r-- 1 0 0 5 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-c writes the status change time",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.old}},
			args:    []string{"-l", "-c"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.year(now.old) + " a.txt\n",
		},
		{
			name:    "-u writes the access time",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.old}},
			args:    []string{"-l", "-u"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.year(now.old) + " a.txt\n",
		},
		{
			name:    "-u after -c chooses the access time",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-l", "-c", "-u"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "-c after -u chooses the status change time",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-l", "-u", "-c"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
	})
}

func TestRunInode(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "-i writes the file serial number",
			files:   fsx.MemoryFS{"a.txt": {}, "b.txt": {}},
			args:    []string{"-i", "-1"},
			wantOut: "0 a.txt\n0 b.txt\n",
		},
		{
			name:    "-i writes the number before the columns",
			files:   fsx.MemoryFS{"a.txt": {}, "b.txt": {}},
			args:    []string{"-i", "-C"},
			wantOut: "0 a.txt  0 b.txt\n",
		},
	})
}

func TestRunBlocks(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "-s writes the blocks and their total",
			files:   fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "b.txt": {Data: []byte(strings.Repeat("x", 512))}},
			args:    []string{"-s", "-1"},
			wantOut: "total 1\n1 a.txt\n1 b.txt\n",
		},
		{
			name:    "-s writes no block for an empty file",
			files:   fsx.MemoryFS{"a.txt": {}},
			args:    []string{"-s", "-1"},
			wantOut: "total 0\n0 a.txt\n",
		},
		{
			name:    "-k counts blocks in kibibytes",
			files:   fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "b.txt": {Data: []byte(strings.Repeat("x", 512))}},
			args:    []string{"-s", "-k", "-1"},
			wantOut: "total 1\n1 a.txt\n1 b.txt\n",
		},
		{
			name:    "-s writes the blocks of a file operand without a total",
			files:   fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			args:    []string{"-s", "-1", "a.txt"},
			wantOut: "1 a.txt\n",
		},
		{
			name:    "-s writes the numbers of listings of different widths",
			files:   fsx.MemoryFS{"a.txt": {}, "b.txt": {Data: []byte(strings.Repeat("x", 5120))}},
			args:    []string{"-s", "-1"},
			wantOut: "total 5\n0 a.txt\n5 b.txt\n",
		},
	})
}

func TestRunFormat(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "-C writes entries down the columns",
			files:   fsx.MemoryFS{"a": {}, "b": {}, "c": {}, "d": {}, "e": {}, "f": {}, "g": {}},
			env:     map[string]string{"COLUMNS": "10"},
			args:    []string{"-C"},
			wantOut: "a  d  g\nb  e\nc  f\n",
		},
		{
			name:    "-C pads to the width of the column",
			files:   fsx.MemoryFS{"a": {}, "bbbb": {}},
			env:     map[string]string{"COLUMNS": "20"},
			args:    []string{"-C"},
			wantOut: "a  bbbb\n",
		},
		{
			name:    "-C writes as many columns as the width fits",
			files:   fsx.MemoryFS{"a": {}, "b": {}, "c": {}, "d": {}},
			env:     map[string]string{"COLUMNS": "80"},
			args:    []string{"-C"},
			wantOut: "a  b  c  d\n",
		},
		{
			name:    "-x writes entries across the columns",
			files:   fsx.MemoryFS{"a": {}, "b": {}, "c": {}, "d": {}},
			env:     map[string]string{"COLUMNS": "10"},
			args:    []string{"-x"},
			wantOut: "a  b  c\nd\n",
		},
		{
			name:    "-C pads by the characters of the names",
			files:   fsx.MemoryFS{"a": {}, "\u00e9\u00e9": {}},
			env:     map[string]string{"COLUMNS": "20"},
			args:    []string{"-C"},
			wantOut: "a  \u00e9\u00e9\n",
		},
		{
			name:    "-C writes the file operands in the same columns",
			files:   fsx.MemoryFS{"a.txt": {}, "bb.txt": {}},
			args:    []string{"-C", "bb.txt", "a.txt"},
			wantOut: "a.txt  bb.txt\n",
		},
		{
			name:    "-C pads the names by their characters",
			files:   fsx.MemoryFS{"a": {}, "\u00e9\u00e9": {}},
			env:     map[string]string{"COLUMNS": "20"},
			args:    []string{"-C"},
			wantOut: "a  \u00e9\u00e9\n",
		},
		{
			name:    "-C writes one column when a name is too wide",
			files:   fsx.MemoryFS{"aaaaaaaaaaaa": {}, "b": {}},
			env:     map[string]string{"COLUMNS": "10"},
			args:    []string{"-C"},
			wantOut: "aaaaaaaaaaaa\nb\n",
		},
		{
			name:    "-m writes the names separated by commas",
			files:   fsx.MemoryFS{"a": {}, "b": {}, "c": {}},
			args:    []string{"-m"},
			wantOut: "a, b, c\n",
		},
		{
			name:    "-m wraps when a name doesn't fit the width",
			files:   fsx.MemoryFS{"aaa": {}, "bbb": {}, "ccc": {}},
			env:     map[string]string{"COLUMNS": "7"},
			args:    []string{"-m"},
			wantOut: "aaa,\nbbb,\nccc\n",
		},
		{
			name:    "-1 writes one entry per line",
			files:   fsx.MemoryFS{"a": {}, "b": {}},
			args:    []string{"-1"},
			wantOut: "a\nb\n",
		},
		{
			name:    "-1 after -C wins",
			files:   fsx.MemoryFS{"a": {}, "b": {}},
			args:    []string{"-C", "-1"},
			wantOut: "a\nb\n",
		},
		{
			name:    "-C after -1 wins",
			files:   fsx.MemoryFS{"a": {}, "b": {}},
			args:    []string{"-1", "-C"},
			wantOut: "a  b\n",
		},
		{
			name:    "-C pads every column to its own width",
			files:   fsx.MemoryFS{"a": {}, "bb": {}, "cccc": {}},
			env:     map[string]string{"COLUMNS": "20"},
			args:    []string{"-C"},
			wantOut: "a  bb  cccc\n",
		},
		{
			name:    "-C writes one column when a name is wider than the width",
			files:   fsx.MemoryFS{"aaaaaaaa": {}, "b": {}},
			env:     map[string]string{"COLUMNS": "8"},
			args:    []string{"-C"},
			wantOut: "aaaaaaaa\nb\n",
		},
		{
			name:    "an invalid COLUMNS value falls back to the default width",
			files:   fsx.MemoryFS{"a": {}, "b": {}},
			env:     map[string]string{"COLUMNS": "banana"},
			args:    []string{"-C"},
			wantOut: "a  b\n",
		},
		{
			name:    "a zero COLUMNS value falls back to the default width",
			files:   fsx.MemoryFS{"a": {}, "b": {}},
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
			files: fsx.MemoryFS{
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
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"-p"},
			wantOut: "dir/\nscript\n",
		},
		{
			name:    "-F after -p writes every indicator",
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"-p", "-F"},
			wantOut: "dir/\nscript*\n",
		},
		{
			name:    "-p after -F writes the directory indicator only",
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"-F", "-p"},
			wantOut: "dir/\nscript\n",
		},
		{
			name: "-lp writes the link target without an indicator",
			files: fsx.MemoryFS{
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
			files:   fsx.MemoryFS{"link": {Data: []byte("dir"), Mode: fs.ModeSymlink | 0o777, ModTime: now.recent}},
			args:    []string{"-lF"},
			wantOut: "total 1\nlrwxrwxrwx 1 0 0 3 " + now.day(now.recent) + " link@ -> dir\n",
		},
		{
			name:    "-LF writes the indicator of the file a link refers to",
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}, "link": link("dir")},
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
			files:   fsx.MemoryFS{"a\nb": {}, "c\td": {}, "e": {}},
			args:    []string{"-q"},
			wantOut: "a?b\nc?d\ne\n",
		},
		{
			name:    "-q writes the long format with quoted names",
			files:   fsx.MemoryFS{"a\x01b": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-q", "-l"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a?b\n",
		},
		{
			name:    "without -q the name is written as it is",
			files:   fsx.MemoryFS{"a\nb": {}},
			args:    []string{"-1"},
			wantOut: "a\nb\n",
		},
		{
			name:    "-q writes the target of a link quoted",
			files:   fsx.MemoryFS{"link": linkAt("a\nb", now.recent)},
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
			files: fsx.MemoryFS{
				"small": {Data: []byte("x")},
				"big":   {Data: []byte("xxxxxxxxxx")},
				"zero":  {},
			},
			args:    []string{"-1", "-S"},
			wantOut: "big\nsmall\nzero\n",
		},
		{
			name: "-S sorts names up when the size is equal",
			files: fsx.MemoryFS{
				"b": {Data: []byte("x")},
				"a": {Data: []byte("x")},
			},
			args:    []string{"-1", "-S"},
			wantOut: "a\nb\n",
		},
		{
			name: "-t sorts by modification time, most recent first",
			files: fsx.MemoryFS{
				"old":  {ModTime: now.old},
				"new":  {ModTime: now.recent},
				"midd": {ModTime: now.recent.Add(-time.Hour)},
			},
			args:    []string{"-1", "-t"},
			wantOut: "new\nmidd\nold\n",
		},
		{
			name: "-t sorts names up when the time is equal",
			files: fsx.MemoryFS{
				"b": {ModTime: now.recent},
				"a": {ModTime: now.recent},
			},
			args:    []string{"-1", "-t"},
			wantOut: "a\nb\n",
		},
		{
			name:    "-X sorts by the suffix of a name after its last dot",
			files:   fsx.MemoryFS{"b.txt": {}, "a.md": {}, "plain": {}, "c.txt": {}},
			args:    []string{"-1", "-X"},
			wantOut: "plain\na.md\nb.txt\nc.txt\n",
		},
		{
			name:    "-v sorts the runs of digits of a name by value",
			files:   fsx.MemoryFS{"f10": {}, "f9": {}, "f2": {}, "f1": {}},
			args:    []string{"-1", "-v"},
			wantOut: "f1\nf2\nf9\nf10\n",
		},
		{
			name:    "-v sorts a tilde first and the letters before the other bytes",
			files:   fsx.MemoryFS{"a-b": {}, "a.b": {}, "ab": {}, "a~": {}, "a": {}},
			args:    []string{"-1", "-v"},
			wantOut: "a~\na\na.b\nab\na-b\n",
		},
		{
			name:    "-U keeps the order of the directory",
			files:   fsx.MemoryFS{"b": {}, "a": {}},
			args:    []string{"-1", "-U"},
			wantOut: "a\nb\n",
		},
		{
			name:    "-r reverses the order of the names",
			files:   fsx.MemoryFS{"a": {}, "b": {}, "c": {}},
			args:    []string{"-1", "-r"},
			wantOut: "c\nb\na\n",
		},
		{
			name:    "-r reverses the order of the sizes",
			files:   fsx.MemoryFS{"big": {Data: []byte("xxx")}, "small": {Data: []byte("x")}},
			args:    []string{"-1", "-S", "-r"},
			wantOut: "small\nbig\n",
		},
		{
			name:    "-r reverses the order of the times",
			files:   fsx.MemoryFS{"new": {ModTime: now.recent}, "old": {ModTime: now.old}},
			args:    []string{"-1", "-t", "-r"},
			wantOut: "old\nnew\n",
		},
		{
			name:    "-S after -t wins the sort key",
			files:   fsx.MemoryFS{"big": {Data: []byte("xxx"), ModTime: now.old}, "small": {Data: []byte("x"), ModTime: now.recent}},
			args:    []string{"-1", "-t", "-S"},
			wantOut: "big\nsmall\n",
		},
		{
			name:    "-t after -S wins the sort key",
			files:   fsx.MemoryFS{"big": {Data: []byte("xxx"), ModTime: now.old}, "small": {Data: []byte("x"), ModTime: now.recent}},
			args:    []string{"-1", "-S", "-t"},
			wantOut: "small\nbig\n",
		},
		{
			name: "-t with -c sorts by the status change time",
			files: fsx.MemoryFS{
				"a.txt": {ModTime: now.recent},
				"b.txt": {ModTime: now.old},
			},
			args:    []string{"-1", "-t", "-c"},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "-f writes the entries in the order of the directory",
			files:   fsx.MemoryFS{"b.txt": {}, "a.txt": {}, ".hidden": {}},
			args:    []string{"-f"},
			wantOut: ".hidden\na.txt\nb.txt\n",
		},
		{
			name:    "-f ignores -r",
			files:   fsx.MemoryFS{"b.txt": {}, "a.txt": {}},
			args:    []string{"-f", "-r"},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "-f ignores -S",
			files:   fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "b.txt": {}},
			args:    []string{"-S", "-f"},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "-f ignores -t",
			files:   fsx.MemoryFS{"a.txt": {ModTime: now.old}, "b.txt": {ModTime: now.recent}},
			args:    []string{"-t", "-f"},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "-f keeps the long format",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-f", "-l"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name: "-S uses the size of the link itself",
			files: fsx.MemoryFS{
				"dir":  {Mode: dirMode(0o755)},
				"link": link("dir"),
				"big":  {Data: []byte(strings.Repeat("x", 100))},
			},
			args:    []string{"-1", "-S"},
			wantOut: "big\nlink\ndir\n",
		},
	})
}

func TestCompareVersion(t *testing.T) {
	t.Run("the well sorted GNU examples", func(t *testing.T) {
		// The sorted examples of the filevercmp test of GNU, where each
		// name sorts before every name after it.
		sorted := []string{
			"", ".", "..", ".0", ".9", ".A", ".Z", ".a~", ".a", ".b~", ".b",
			".z", ".zz~", ".zz", ".zz.~1~", ".zz.0", ".\x01", ".\x01.txt",
			".\x01x", ".\x01x\x01", ".\x01.0", "0", "9", "A", "Z", "a~", "a",
			"a.b~", "a.b", "a.bc~", "a.bc", "a+", "a.", "a..a", "a.+", "b~", "b",
			"gcc-c++-10.fc9.tar.gz", "gcc-c++-10.fc9.tar.gz.~1~",
			"gcc-c++-10.fc9.tar.gz.~2~", "gcc-c++-10.8.12-0.7rc2.fc9.tar.bz2",
			"gcc-c++-10.8.12-0.7rc2.fc9.tar.bz2.~1~", "glibc-2-0.1.beta1.fc10.rpm",
			"glibc-common-5-0.2.beta2.fc9.ebuild", "glibc-common-5-0.2b.deb",
			"glibc-common-11b.ebuild", "glibc-common-11-0.6rc2.ebuild",
			"libstdc++-0.5.8.11-0.7rc2.fc10.tar.gz", "libstdc++-4a.fc8.tar.gz",
			"libstdc++-4.10.4.20040204svn.rpm", "libstdc++-devel-3.fc8.ebuild",
			"libstdc++-devel-3a.fc9.tar.gz", "libstdc++-devel-8.fc8.deb",
			"libstdc++-devel-8.6.2-0.4b.fc8", "nss_ldap-1-0.2b.fc9.tar.bz2",
			"nss_ldap-1-0.6rc2.fc8.tar.gz", "nss_ldap-1.0-0.1a.tar.gz",
			"nss_ldap-10beta1.fc8.tar.gz", "nss_ldap-10.11.8.6.20040204cvs.fc10.ebuild",
			"z", "zz~", "zz", "zz.~1~", "zz.0", "zz.0.txt",
			"\x01", "\x01.txt", "\x01x", "\x01x\x01", "\x01.0", "#\x01.b#", "#.b#",
		}
		sign := func(n int) int { return min(max(n, -1), 1) }
		for i, a := range sorted {
			for j, b := range sorted {
				if got, want := sign(compareVersion(a, b)), sign(i-j); got != want {
					t.Errorf("compareVersion(%q, %q) = %d, want %d", a, b, got, want)
				}
			}
		}
	})

	t.Run("names padded with zeros compare equal", func(t *testing.T) {
		groups := [][]string{
			{"a", "a0", "a0000"},
			{"a\x01c-27.txt", "a\x01c-027.txt", "a\x01c-00000000000000000000000000000000000000000000000000000027.txt"},
			{".a\x01c-27.txt", ".a\x01c-027.txt", ".a\x01c-00000000000000000000000000000000000000000000000000000027.txt"},
			{"a\x01c-", "a\x01c-0", "a\x01c-00"},
			{".a\x01c-", ".a\x01c-0", ".a\x01c-00"},
			{"a\x01c-0.txt", "a\x01c-00.txt"},
			{".a\x01c-1\x01.txt", ".a\x01c-001\x01.txt"},
		}
		for _, group := range groups {
			for _, a := range group {
				for _, b := range group {
					if got := compareVersion(a, b); got != 0 {
						t.Errorf("compareVersion(%q, %q) = %d, want 0", a, b, got)
					}
				}
			}
		}
	})
}

func TestRunSymbolicLinks(t *testing.T) {
	now := newTimes()

	files := fsx.MemoryFS{
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
			files:   fsx.MemoryFS{"dir/in": {Mode: 0o644, ModTime: now.recent}, "to-dir": link("dir")},
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
			name:    "--file-type writes an operand link as itself",
			files:   files,
			args:    []string{"--file-type", "to-dir"},
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
			wantErr:  "ls: invalid option -- 'P'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "ls", usage)
}

func TestRunDirectories(t *testing.T) {
	now := newTimes()

	files := fsx.MemoryFS{
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
			name:    "-R without operands writes the header of the working directory",
			files:   files,
			args:    []string{"-1", "-R"},
			wantOut: ".:\na.txt\ndir\nother\n\n./dir:\nb.txt\nsub\n\n./dir/sub:\nc.txt\n\n./other:\nd.txt\n",
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
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}, "dir/a.txt": {}, "link": link("dir")},
			args:    []string{"-1", "-R"},
			wantOut: ".:\ndir\nlink\n\n./dir:\na.txt\n",
		},
		{
			name:    "-LR recurses into a link to a directory",
			files:   fsx.MemoryFS{"dir/a.txt": {}, "link": link("dir")},
			args:    []string{"-1", "-L", "-R"},
			wantOut: ".:\ndir\nlink\n\n./dir:\na.txt\n\n./link:\na.txt\n",
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
			files: fsx.MemoryFS{"dir": {Mode: dirMode(0o755), ModTime: now.recent}, "dir/sub": {Mode: dirMode(0o755), ModTime: now.recent}, "dir/sub/a.txt": {Mode: 0o644, ModTime: now.recent}},
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
			files:    fsx.MemoryFS{"a.txt": {}},
			args:     []string{"-Z"},
			wantErr:  "ls: invalid option -- 'Z'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option is reported",
			files:    fsx.MemoryFS{"a.txt": {}},
			args:     []string{"--bogus"},
			wantErr:  "ls: unrecognized option '--bogus'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:    "the operands may begin after a double dash",
			files:   fsx.MemoryFS{"-a": {}, "b.txt": {}},
			args:    []string{"--", "-a"},
			wantOut: "-a\n",
		},
		{
			name:    "a single dash is an operand",
			files:   fsx.MemoryFS{"-": {}},
			args:    []string{"-"},
			wantOut: "-\n",
		},
		{
			name:    "an operand keeps the options after it from parsing",
			files:   fsx.MemoryFS{"a.txt": {}, "-l": {}},
			args:    []string{"a.txt", "-l"},
			wantOut: "-l\na.txt\n",
		},
		{
			name:    "bundled options are parsed one by one",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-lq", "a.txt"},
			wantOut: "-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:     "an invalid option of a bundle is reported",
			files:    fsx.MemoryFS{},
			args:     []string{"-lZ"},
			wantErr:  "ls: invalid option -- 'Z'\nTry 'ls --help' for more information.\n",
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

	linked := fsx.MemoryFS{
		"dir":    {Mode: dirMode(0o755)},
		"dir/in": {Mode: 0o644},
		"to-dir": link("dir"),
	}

	runCases(t, []testCase{
		{
			name:    "all writes the entries that begin with a dot",
			files:   fsx.MemoryFS{".hidden": {}, "a.txt": {}},
			args:    []string{"--all"},
			wantOut: ".hidden\na.txt\n",
		},
		{
			name:    "almost-all writes the hidden entries",
			files:   fsx.MemoryFS{".hidden": {}, "a.txt": {}},
			args:    []string{"--almost-all"},
			wantOut: ".hidden\na.txt\n",
		},
		{
			name:    "classify marks the type of each file",
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
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
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}, "dir/b.txt": {}},
			args:    []string{"--directory", "dir"},
			wantOut: "dir\n",
		},
		{
			name:    "hide-control-chars writes control characters as question marks",
			files:   fsx.MemoryFS{"a\nb": {}, "e": {}},
			args:    []string{"--hide-control-chars"},
			wantOut: "a?b\ne\n",
		},
		{
			name:    "inode writes the file serial number",
			files:   fsx.MemoryFS{"a.txt": {}},
			args:    []string{"--inode"},
			wantOut: "0 a.txt\n",
		},
		{
			name:    "kibibytes counts blocks in kibibytes",
			files:   fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			args:    []string{"--kibibytes", "--size"},
			wantOut: "total 1\n1 a.txt\n",
		},
		{
			name:    "numeric-uid-gid writes the identifiers as numbers",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"--numeric-uid-gid"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "recursive lists the directories below the operand",
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}, "dir/b.txt": {}, "dir/sub": {Mode: dirMode(0o755)}, "dir/sub/c.txt": {}},
			args:    []string{"--recursive", "dir"},
			wantOut: "dir:\nb.txt\nsub\n\ndir/sub:\nc.txt\n",
		},
		{
			name:    "reverse reverses the order of the names",
			files:   fsx.MemoryFS{"a": {}, "b": {}, "c": {}},
			args:    []string{"--reverse"},
			wantOut: "c\nb\na\n",
		},
		{
			name:    "size writes the blocks and their total",
			files:   fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			args:    []string{"--size"},
			wantOut: "total 1\n1 a.txt\n",
		},
		{
			name:    "format long writes the long format",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"--format=long"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "format single-column disables the long format",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-l", "--format=single-column"},
			wantOut: "a.txt\n",
		},
		{
			name:    "format long after single-column writes the long format again",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"--format=single-column", "--format=long"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a.txt\n",
		},
		{
			name:    "format takes its value in the next argument",
			files:   fsx.MemoryFS{"a": {}, "b": {}},
			args:    []string{"--format", "single-column"},
			wantOut: "a\nb\n",
		},
		{
			name:    "format vertical writes entries down the columns",
			files:   fsx.MemoryFS{"a": {}, "b": {}, "c": {}, "d": {}},
			env:     map[string]string{"COLUMNS": "80"},
			args:    []string{"--format=vertical"},
			wantOut: "a  b  c  d\n",
		},
		{
			name:    "format across writes entries across the columns",
			files:   fsx.MemoryFS{"a": {}, "b": {}, "c": {}, "d": {}},
			env:     map[string]string{"COLUMNS": "10"},
			args:    []string{"--format=across"},
			wantOut: "a  b  c\nd\n",
		},
		{
			name:    "format commas writes the names separated by commas",
			files:   fsx.MemoryFS{"a": {}, "b": {}, "c": {}},
			args:    []string{"--format=commas"},
			wantOut: "a, b, c\n",
		},
		{
			name:    "sort size sorts by size, largest first",
			files:   fsx.MemoryFS{"small": {Data: []byte("x")}, "big": {Data: []byte("xxxxxxxxxx")}, "zero": {}},
			args:    []string{"--sort=size"},
			wantOut: "big\nsmall\nzero\n",
		},
		{
			name:    "sort time sorts by modification time, most recent first",
			files:   fsx.MemoryFS{"old": {ModTime: now.old}, "new": {ModTime: now.recent}},
			args:    []string{"--sort=time"},
			wantOut: "new\nold\n",
		},
		{
			name:    "sort version orders the runs of digits of a name by value",
			files:   fsx.MemoryFS{"f10": {}, "f9": {}, "f2": {}, "f1": {}},
			args:    []string{"--sort=version"},
			wantOut: "f1\nf2\nf9\nf10\n",
		},
		{
			name:    "sort extension sorts by the suffix of a name after its last dot",
			files:   fsx.MemoryFS{"b.txt": {}, "a.md": {}, "plain": {}, "c.txt": {}},
			args:    []string{"--sort=extension"},
			wantOut: "plain\na.md\nb.txt\nc.txt\n",
		},
		{
			name:    "sort width sorts the narrowest names first",
			files:   fsx.MemoryFS{"ccc": {}, "dd": {}, "b": {}, "aa": {}},
			args:    []string{"--sort=width"},
			wantOut: "b\naa\ndd\nccc\n",
		},
		{
			name:    "sort width measures a wide character as two columns",
			files:   fsx.MemoryFS{"界": {}, "aa": {}, "b": {}},
			args:    []string{"--sort=width"},
			wantOut: "b\naa\n界\n",
		},
		{
			name:    "sort name keeps the order of the names",
			files:   fsx.MemoryFS{"b": {Data: []byte("xxx")}, "a": {}},
			args:    []string{"--sort=name"},
			wantOut: "a\nb\n",
		},
		{
			name:    "sort none keeps the order of the directory without showing hidden entries",
			files:   fsx.MemoryFS{"b": {Data: []byte("xxx")}, "a": {}, ".hidden": {}},
			args:    []string{"--sort=none"},
			wantOut: "a\nb\n",
		},
		{
			name:    "time atime writes the access time",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.old}},
			args:    []string{"--format=long", "--time=atime"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.year(now.old) + " a.txt\n",
		},
		{
			name:    "time ctime writes the status change time",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.old}},
			args:    []string{"--format=long", "--time=ctime"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.year(now.old) + " a.txt\n",
		},
		{
			name:    "indicator-style slash marks directories only",
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"--indicator-style=slash"},
			wantOut: "dir/\nscript\n",
		},
		{
			name:    "indicator-style classify marks the type of each file",
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"--indicator-style=classify"},
			wantOut: "dir/\nscript*\n",
		},
		{
			name: "indicator-style file-type marks the type of each file but executables",
			files: fsx.MemoryFS{
				"dir":    {Mode: dirMode(0o755)},
				"link":   link("dir"),
				"plain":  {},
				"script": {Mode: 0o755},
			},
			args:    []string{"--indicator-style=file-type"},
			wantOut: "dir/\nlink@\nplain\nscript\n",
		},
		{
			name:    "file-type marks the type of each file but executables",
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"--file-type"},
			wantOut: "dir/\nscript\n",
		},
		{
			name:    "indicator-style none writes no indicator",
			files:   fsx.MemoryFS{"dir": {Mode: dirMode(0o755)}, "script": {Mode: 0o755}},
			args:    []string{"--classify", "--indicator-style=none"},
			wantOut: "dir\nscript\n",
		},
		{
			name:     "an unknown format word is an error",
			files:    fsx.MemoryFS{},
			args:     []string{"--format=bogus"},
			wantErr:  "ls: invalid argument 'bogus' for '--format'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an unknown sort word is an error",
			files:    fsx.MemoryFS{},
			args:     []string{"--sort=bogus"},
			wantErr:  "ls: invalid argument 'bogus' for '--sort'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an unknown time word is an error",
			files:    fsx.MemoryFS{},
			args:     []string{"--time=bogus"},
			wantErr:  "ls: invalid argument 'bogus' for '--time'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an unknown indicator-style word is an error",
			files:    fsx.MemoryFS{},
			args:     []string{"--indicator-style=bogus"},
			wantErr:  "ls: invalid argument 'bogus' for '--indicator-style'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a valued long option requires an argument",
			files:    fsx.MemoryFS{},
			args:     []string{"--format"},
			wantErr:  "ls: option '--format' requires an argument\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a long option with a value it doesn't take is an error",
			files:    fsx.MemoryFS{},
			args:     []string{"--all=yes"},
			wantErr:  "ls: option '--all=yes' doesn't allow an argument\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
	})
}

func TestRunMultipleOperands(t *testing.T) {
	now := newTimes()

	files := fsx.MemoryFS{
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
	sys := fsx.SystemFS(t.TempDir())
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
			Stdout:  stdout,
			Stderr:  stderr,
			FS:      sys,
			Environ: func() []string { return testutil.Environ(nil) },
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
			sys:      unreadableFS{FS: fsx.MemoryFS{"hidden/a.txt": {}, "a.txt": {}}, denied: "hidden"},
			args:     []string{"hidden"},
			wantErr:  "ls: cannot open directory 'hidden': permission denied\n",
			wantExit: 1,
		},
		{
			name:     "lists the other operands after the failure",
			sys:      unreadableFS{FS: fsx.MemoryFS{"hidden/a.txt": {}, "dir/b.txt": {}}, denied: "hidden"},
			args:     []string{"-1", "dir", "hidden"},
			wantOut:  "dir:\nb.txt\n\nhidden:\n",
			wantErr:  "ls: cannot open directory 'hidden': permission denied\n",
			wantExit: 1,
		},
		{
			name:     "reports a directory of a recursive listing",
			sys:      unreadableFS{FS: fsx.MemoryFS{"dir/hidden/a.txt": {}}, denied: "dir/hidden"},
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
			files:   fsx.MemoryFS{"dir/sub/a.txt": {}, "dir/.hidden/b.txt": {}},
			args:    []string{"-1", "-R", "dir"},
			wantOut: "dir:\nsub\n\ndir/sub:\na.txt\n",
		},
		{
			name:    "-aR enters the hidden directories it listed",
			files:   fsx.MemoryFS{"dir/sub/a.txt": {}, "dir/.hidden/b.txt": {}},
			args:    []string{"-1", "-aR"},
			wantOut: ".:\ndir\n\n./dir:\n.hidden\nsub\n\n./dir/.hidden:\nb.txt\n\n./dir/sub:\na.txt\n",
		},
		{
			name:    "-R enters a directory named as an operand",
			files:   fsx.MemoryFS{"dir/sub/a.txt": {}},
			args:    []string{"-1", "-R", "dir/sub"},
			wantOut: "dir/sub:\na.txt\n",
		},
	})
}

// TestRunFastOrder checks that -f keeps the order a real directory reports.
func TestRunFastOrder(t *testing.T) {
	dir := t.TempDir()
	sys := fsx.SystemFS(dir)
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
		Stdout:  stdout,
		Stderr:  stderr,
		FS:      sys,
		Environ: func() []string { return testutil.Environ(nil) },
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

	sys := fsx.SystemFS(t.TempDir())
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
		Stdout:  stdout,
		Stderr:  stderr,
		FS:      sys,
		Environ: func() []string { return testutil.Environ(nil) },
	}
	err := New(rt).Run(t.Context(), "-1", "-L", "-R", "x")
	if code := testutil.ExitCode(t, err); code != 1 {
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
			files:   fsx.MemoryFS{"dir/a.txt": {}, "dir/b.txt": {}},
			args:    []string{`dir\`},
			wantOut: "a.txt\nb.txt\n",
		},
		{
			name:    "an operand with backslashes names a file",
			files:   fsx.MemoryFS{"dir/a.txt": {}},
			args:    []string{`dir\a.txt`},
			wantOut: "dir\\a.txt\n",
		},
	})
}

// TestRunContext checks that a listing stops with the context it's given.
func TestRunContext(t *testing.T) {
	testutil.Cancelled(t, New, nil, fsx.MemoryFS{"a.txt": {}}, "a.txt")
}

// TestRunIgnore checks the options that drop entries from a listing.
func TestRunIgnore(t *testing.T) {
	files := fsx.MemoryFS{"a.txt": {}, "b.txt": {}, "bak.txt~": {}, ".bak~": {}, "f1": {}, "f10": {}}

	runCases(t, []testCase{
		{
			name:    "-B drops the entries ending with a tilde",
			files:   files,
			args:    []string{"-B"},
			wantOut: "a.txt\nb.txt\nf1\nf10\n",
		},
		{
			name:    "-B drops the hidden backups too with -a",
			files:   files,
			args:    []string{"-a", "-B"},
			wantOut: "a.txt\nb.txt\nf1\nf10\n",
		},
		{
			name:    "--ignore drops the names matching a pattern",
			files:   files,
			args:    []string{"--ignore", "b*"},
			wantOut: "a.txt\nf1\nf10\n",
		},
		{
			name:    "an initial dot isn't matched by a leading wildcard",
			files:   fsx.MemoryFS{".b": {}, "b.txt": {}},
			args:    []string{"-a", "--ignore", "b*"},
			wantOut: ".b\n",
		},
		{
			name:    "--ignore works with -a",
			files:   fsx.MemoryFS{".b": {}, "b.txt": {}},
			args:    []string{"-a", "--ignore", ".b"},
			wantOut: "b.txt\n",
		},
		{
			name:    "--ignore may be given several patterns",
			files:   files,
			args:    []string{"--ignore", "b*", "--ignore", "*.txt"},
			wantOut: "f1\nf10\n",
		},
		{
			name:    "--hide drops like --ignore",
			files:   files,
			args:    []string{"--hide", "b*"},
			wantOut: "a.txt\nf1\nf10\n",
		},
		{
			name:    "--hide is overridden by -a",
			files:   files,
			args:    []string{"-a", "--hide", "b*"},
			wantOut: ".bak~\na.txt\nb.txt\nbak.txt~\nf1\nf10\n",
		},
		{
			name:    "a command line operand isn't ignored",
			files:   files,
			args:    []string{"--ignore", "b*", "b.txt"},
			wantOut: "b.txt\n",
		},
		{
			name:    "the pattern of a valued short option is attached",
			files:   files,
			args:    []string{"-Ib*"},
			wantOut: "a.txt\nf1\nf10\n",
		},
	})
}

// TestRunQuoting checks the options that quote a name.
func TestRunQuoting(t *testing.T) {
	now := newTimes()

	runCases(t, []testCase{
		{
			name:    "-b escapes the control characters and the space",
			files:   fsx.MemoryFS{"a\nb": {}, "a b": {}, "plain": {}},
			args:    []string{"-b"},
			wantOut: "a\\nb\na\\ b\nplain\n",
		},
		{
			name:    "-b escapes the space of a name",
			files:   fsx.MemoryFS{"a b": {}},
			args:    []string{"-b", "-F"},
			wantOut: "a\\ b\n",
		},
		{
			name:    "-Q quotes a name the C way",
			files:   fsx.MemoryFS{"a\nb": {}, "plain": {}},
			args:    []string{"-Q"},
			wantOut: "\"a\\nb\"\n\"plain\"\n",
		},
		{
			name:    "-N writes the names as they are",
			files:   fsx.MemoryFS{"a b": {}},
			args:    []string{"-N"},
			wantOut: "a b\n",
		},
		{
			name:    "--quoting-style=shell quotes what the shell needs",
			files:   fsx.MemoryFS{"a b": {}, "plain": {}},
			args:    []string{"--quoting-style=shell"},
			wantOut: "'a b'\nplain\n",
		},
		{
			name:    "--quoting-style=shell-escape quotes an apostrophe the C way",
			files:   fsx.MemoryFS{"a'b": {}},
			args:    []string{"--quoting-style=shell-escape"},
			wantOut: "\"a'b\"\n",
		},
		{
			name:    "--quoting-style=shell-escape escapes a control character",
			files:   fsx.MemoryFS{"a\nb": {}},
			args:    []string{"--quoting-style=shell-escape"},
			wantOut: "'a'$'\\n''b'\n",
		},
		{
			name:    "--quoting-style=c-maybe quotes only what needs it",
			files:   fsx.MemoryFS{"a\nb": {}, "plain": {}},
			args:    []string{"--quoting-style=c-maybe"},
			wantOut: "\"a\\nb\"\nplain\n",
		},
		{
			name:  "an unquoted name is padded for a quoted one in the long format",
			files: fsx.MemoryFS{"a b": {Mode: 0o644, ModTime: now.recent}, "plain": {Mode: 0o644, ModTime: now.recent}},
			args:  []string{"-l", "-n", "--quoting-style=shell"},
			wantOut: "total 0\n" +
				"-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " 'a b'\n" +
				"-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + "  plain\n",
		},
		{
			name:    "--show-control-chars writes a nongraphic character as it is",
			files:   fsx.MemoryFS{"a\nb": {}},
			args:    []string{"-q", "--show-control-chars"},
			wantOut: "a\nb\n",
		},
		{
			name:    "-q wins over an earlier --show-control-chars",
			files:   fsx.MemoryFS{"a\nb": {}},
			args:    []string{"--show-control-chars", "-q"},
			wantOut: "a?b\n",
		},
		{
			name:     "an unknown quoting style is an error",
			args:     []string{"--quoting-style=bogus"},
			files:    fsx.MemoryFS{"a": {}},
			wantErr:  "ls: invalid argument 'bogus' for '--quoting-style'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:    "QUOTING_STYLE sets the default style",
			files:   fsx.MemoryFS{"a b": {}},
			env:     map[string]string{"QUOTING_STYLE": "shell"},
			args:    []string{},
			wantOut: "'a b'\n",
		},
		{
			name:    "an option overrides QUOTING_STYLE",
			files:   fsx.MemoryFS{"a b": {}},
			env:     map[string]string{"QUOTING_STYLE": "shell"},
			args:    []string{"-N"},
			wantOut: "a b\n",
		},
		{
			name:    "an invalid QUOTING_STYLE is ignored with a warning",
			files:   fsx.MemoryFS{"a": {}},
			env:     map[string]string{"QUOTING_STYLE": "bogus"},
			args:    []string{},
			wantOut: "a\n",
			wantErr: "ls: ignoring invalid value of environment variable QUOTING_STYLE: bogus\n",
		},
	})
}

// TestQuotingStyles checks the quoting engine against the output of GNU
// coreutils 9.6 for the same names, in a locale with UTF-8.
func TestQuotingStyles(t *testing.T) {
	tests := []struct {
		name  string
		style quotingStyle
		want  string
	}{
		{"plain", quoteLiteral, "plain"},
		{"plain", quoteShell, "plain"},
		{"plain", quoteC, `"plain"`},
		{"plain", quoteEscape, "plain"},
		{"a b", quoteLiteral, "a b"},
		{"a b", quoteShell, "'a b'"},
		{"a b", quoteShellAlways, "'a b'"},
		{"a b", quoteC, `"a b"`},
		{"a b", quoteEscape, "a b"},
		{"a b", quoteShellEscape, "'a b'"},
		{"a b", quoteShellEscapeAlways, "'a b'"},
		{"a b", quoteCMaybe, "a b"},
		{"a b", quoteLocale, "‘a b’"},
		{"a b", quoteCLocale, "‘a b’"},
		{"a'b", quoteShell, `"a'b"`},
		{"a'b", quoteShellEscapeAlways, `"a'b"`},
		{"a'b", quoteShellEscape, `"a'b"`},
		{"a'b", quoteC, `"a'b"`},
		{"a'b", quoteLocale, "‘a'b’"},
		{"a\nb", quoteShell, "'a\nb'"},
		{"a\nb", quoteShellEscape, `'a'$'\n''b'`},
		{"a\nb", quoteShellEscapeAlways, `'a'$'\n''b'`},
		{"a\nb", quoteC, `"a\nb"`},
		{"a\nb", quoteEscape, "a\\nb"},
		{"a\nb", quoteCMaybe, `"a\nb"`},
		{"a\tb", quoteEscape, "a\\tb"},
		{"a\x01b", quoteEscape, "a\\001b"},
		{"a\x01b", quoteShellEscape, `'a'$'\001''b'`},
		{"a?b", quoteShell, "'a?b'"},
		{"a?b", quoteC, `"a?b"`},
		{"café", quoteLiteral, "café"},
		{"café", quoteShell, "café"},
		{"café", quoteShellEscape, "café"},
		{"café", quoteC, `"café"`},
		{`a"b`, quoteC, `"a\"b"`},
		{`a"b`, quoteShell, `'a"b'`},
		{`a"b`, quoteCMaybe, `"a\"b"`},
		{`a\b`, quoteShell, `'a\b'`},
		{`a\b`, quoteC, `"a\\b"`},
		{`a\b`, quoteEscape, `a\\b`},
		{"a#b", quoteShell, "a#b"},
		{"a#b", quoteLiteral, "a#b"},
		{"", quoteShell, "''"},
		{"", quoteLiteral, ""},
		{"", quoteC, `""`},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s/%d", test.name, test.style), func(t *testing.T) {
			got, _ := quoteName(test.name, test.style, false, "")
			if got != test.want {
				t.Errorf("quoteName(%q, style %d) = %q, want %q", test.name, test.style, got, test.want)
			}
		})
	}
}

// TestHideControl checks the writing of nongraphic characters as question
// marks.
func TestHideControl(t *testing.T) {
	for _, test := range []struct{ in, want string }{
		{"plain", "plain"},
		{"a\nb", "a?b"},
		{"a\x01b", "a?b"},
		{"café", "café"},
		{"a\nb\n", "a?b?"},
		{"'\n'", "'?'"},
	} {
		if got := hideControl(test.in); got != test.want {
			t.Errorf("hideControl(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

// TestRunHumanSizes checks the options that scale the sizes of the long
// format.
func TestRunHumanSizes(t *testing.T) {
	now := newTimes()
	files := fsx.MemoryFS{
		"a": {Data: bytes.Repeat([]byte("x"), 1024), Mode: 0o644, ModTime: now.recent},
		"b": {Data: bytes.Repeat([]byte("x"), 10240), Mode: 0o644, ModTime: now.recent},
	}

	runCases(t, []testCase{
		{
			name:    "-h writes the sizes with a size letter",
			files:   files,
			args:    []string{"-lh", "-n"},
			wantOut: "total 11K\n-rw-r--r-- 1 0 0 1.0K " + now.day(now.recent) + " a\n-rw-r--r-- 1 0 0  10K " + now.day(now.recent) + " b\n",
		},
		{
			name:    "--si writes the sizes in the powers of 1000",
			files:   files,
			args:    []string{"-l", "--si", "-n"},
			wantOut: "total 12k\n-rw-r--r-- 1 0 0 1.1k " + now.day(now.recent) + " a\n-rw-r--r-- 1 0 0  11k " + now.day(now.recent) + " b\n",
		},
		{
			name:    "--block-size scales the sizes by a fixed unit",
			files:   files,
			args:    []string{"-l", "--block-size=K", "-n"},
			wantOut: "total 11K\n-rw-r--r-- 1 0 0  1K " + now.day(now.recent) + " a\n-rw-r--r-- 1 0 0 10K " + now.day(now.recent) + " b\n",
		},
		{
			name:    "--block-size=kB scales by a thousand and appends kB",
			files:   files,
			args:    []string{"-l", "--block-size=kB", "-n"},
			wantOut: "total 12kB\n-rw-r--r-- 1 0 0  2kB " + now.day(now.recent) + " a\n-rw-r--r-- 1 0 0 11kB " + now.day(now.recent) + " b\n",
		},
		{
			name:    "-k doesn't scale the sizes of the long format",
			files:   files,
			args:    []string{"-l", "-k", "-n"},
			wantOut: "total 11\n-rw-r--r-- 1 0 0  1024 " + now.day(now.recent) + " a\n-rw-r--r-- 1 0 0 10240 " + now.day(now.recent) + " b\n",
		},
		{
			name:    "-h wins over -k wherever it's given",
			files:   files,
			args:    []string{"-k", "-lh", "-n"},
			wantOut: "total 11K\n-rw-r--r-- 1 0 0 1.0K " + now.day(now.recent) + " a\n-rw-r--r-- 1 0 0  10K " + now.day(now.recent) + " b\n",
		},
		{
			name:    "LS_BLOCK_SIZE scales the block counts and the sizes",
			files:   files,
			env:     map[string]string{"LS_BLOCK_SIZE": "1M"},
			args:    []string{"-ln"},
			wantOut: "total 1\n-rw-r--r-- 1 0 0 1 " + now.day(now.recent) + " a\n-rw-r--r-- 1 0 0 1 " + now.day(now.recent) + " b\n",
		},
		{
			name:    "-k overrides the block size of the environment",
			files:   files,
			env:     map[string]string{"BLOCK_SIZE": "1M"},
			args:    []string{"-s", "-k", "-1"},
			wantOut: "total 11\n 1 a\n10 b\n",
		},
		{
			name:    "BLOCKSIZE scales the block counts only",
			files:   fsx.MemoryFS{"a": {Data: bytes.Repeat([]byte("x"), 1024), Mode: 0o644, ModTime: now.recent}},
			env:     map[string]string{"BLOCKSIZE": "1M"},
			args:    []string{"-sln"},
			wantOut: "total 1\n1 -rw-r--r-- 1 0 0 1024 " + now.day(now.recent) + " a\n",
		},
		{
			name:    "POSIXLY_CORRECT counts the blocks in units of 512",
			files:   fsx.MemoryFS{"a": {Data: bytes.Repeat([]byte("x"), 1024)}},
			env:     map[string]string{"POSIXLY_CORRECT": "1"},
			args:    []string{"-s", "-1"},
			wantOut: "total 2\n2 a\n",
		},
		{
			name:     "a zero block size is an error",
			files:    files,
			args:     []string{"--block-size=0"},
			wantErr:  "ls: invalid --block-size argument '0'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid block size is an error",
			files:    files,
			args:     []string{"--block-size=banana"},
			wantErr:  "ls: invalid --block-size argument 'banana'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid LS_BLOCK_SIZE is an error",
			files:    files,
			env:      map[string]string{"LS_BLOCK_SIZE": "banana"},
			args:     []string{"-1"},
			wantErr:  "ls: invalid suffix in LS_BLOCK_SIZE: 'banana'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
	})
}

// TestRunTimeStyle checks the options that change the timestamps of the
// long format.
func TestRunTimeStyle(t *testing.T) {
	now := newTimes()
	files := fsx.MemoryFS{"new": {Mode: 0o644, ModTime: now.recent}, "old": {Mode: 0o644, ModTime: now.old}}

	runCases(t, []testCase{
		{
			name:  "long-iso writes the date and the minute",
			files: files,
			args:  []string{"-l", "--time-style=long-iso", "-n"},
			wantOut: "total 0\n" +
				"-rw-r--r-- 1 0 0 0 " + now.recent.Format("2006-01-02 15:04") + " new\n" +
				"-rw-r--r-- 1 0 0 0 " + now.old.Format("2006-01-02 15:04") + " old\n",
		},
		{
			name:  "iso writes the year for the old ones",
			files: files,
			args:  []string{"-l", "--time-style=iso", "-n"},
			wantOut: "total 0\n" +
				"-rw-r--r-- 1 0 0 0 " + now.recent.Format("01-02 15:04") + " new\n" +
				"-rw-r--r-- 1 0 0 0 " + now.old.Format("2006-01-02") + "  old\n",
		},
		{
			name:  "full-iso writes the nanoseconds and the zone",
			files: files,
			args:  []string{"-l", "--time-style=full-iso", "-n"},
			wantOut: "total 0\n" +
				"-rw-r--r-- 1 0 0 0 " + now.recent.Format("2006-01-02 15:04:05.000000000 -0700") + " new\n" +
				"-rw-r--r-- 1 0 0 0 " + now.old.Format("2006-01-02 15:04:05.000000000 -0700") + " old\n",
		},
		{
			name:  "--full-time is -l with full-iso",
			files: files,
			args:  []string{"--full-time", "-n"},
			wantOut: "total 0\n" +
				"-rw-r--r-- 1 0 0 0 " + now.recent.Format("2006-01-02 15:04:05.000000000 -0700") + " new\n" +
				"-rw-r--r-- 1 0 0 0 " + now.old.Format("2006-01-02 15:04:05.000000000 -0700") + " old\n",
		},
		{
			name:  "a format with a newline splits the recent and the old ones",
			files: files,
			args:  []string{"-l", "--time-style=+%Y-%m-%d\n%Y-%m-%d %H:%M", "-n"},
			wantOut: "total 0\n" +
				"-rw-r--r-- 1 0 0 0 " + now.recent.Format("2006-01-02 15:04") + " new\n" +
				"-rw-r--r-- 1 0 0 0 " + now.old.Format("2006-01-02") + " old\n",
		},
		{
			name:  "TIME_STYLE sets the default style",
			files: files,
			env:   map[string]string{"TIME_STYLE": "long-iso"},
			args:  []string{"-ln"},
			wantOut: "total 0\n" +
				"-rw-r--r-- 1 0 0 0 " + now.recent.Format("2006-01-02 15:04") + " new\n" +
				"-rw-r--r-- 1 0 0 0 " + now.old.Format("2006-01-02 15:04") + " old\n",
		},
		{
			name:  "posix-long-iso uses the locale form in the C locale",
			files: files,
			env:   map[string]string{"LC_ALL": "C"},
			args:  []string{"-ln", "--time-style=posix-long-iso"},
			wantOut: "total 0\n" +
				"-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " new\n" +
				"-rw-r--r-- 1 0 0 0 " + now.year(now.old) + " old\n",
		},
		{
			name:  "posix-long-iso uses the style outside the C locale",
			files: files,
			env:   map[string]string{"LC_ALL": "C.UTF-8"},
			args:  []string{"-ln", "--time-style=posix-long-iso"},
			wantOut: "total 0\n" +
				"-rw-r--r-- 1 0 0 0 " + now.recent.Format("2006-01-02 15:04") + " new\n" +
				"-rw-r--r-- 1 0 0 0 " + now.old.Format("2006-01-02 15:04") + " old\n",
		},
		{
			name:     "an invalid time style is an error",
			files:    files,
			args:     []string{"-l", "--time-style=bogus"},
			wantErr:  "ls: invalid --time-style argument 'bogus'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid TIME_STYLE is an error",
			files:    files,
			env:      map[string]string{"TIME_STYLE": "bogus"},
			args:     []string{"-l"},
			wantErr:  "ls: invalid --time-style argument 'bogus'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
	})
}

// TestRunBirthTime checks --time=birth.
func TestRunBirthTime(t *testing.T) {
	now := newTimes()
	runCases(t, []testCase{
		{
			name:    "a birth time a memory file system doesn't have is a question mark",
			files:   fsx.MemoryFS{"a.txt": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-l", "--time=birth", "-n"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0            ? a.txt\n",
		},
		{
			name:    "sorting by birth falls back to the modification time",
			files:   fsx.MemoryFS{"a.txt": {ModTime: now.old}, "b.txt": {ModTime: now.recent}},
			args:    []string{"-t", "--time=birth"},
			wantOut: "b.txt\na.txt\n",
		},
	})
}

// TestRunGroupDirectories checks --group-directories-first.
func TestRunGroupDirectories(t *testing.T) {
	files := fsx.MemoryFS{
		"a":     {},
		"b":     {},
		"dir":   {Mode: dirMode(0o755)},
		"zdir":  {Mode: dirMode(0o755)},
		"link":  link("dir"),
		"blink": link("missing"),
	}

	runCases(t, []testCase{
		{
			name:    "the directories and their links are written first",
			files:   files,
			args:    []string{"-1", "--group-directories-first"},
			wantOut: "dir\nlink\nzdir\na\nb\nblink\n",
		},
		{
			name:    "the sort order stays within each group",
			files:   files,
			args:    []string{"-1", "-r", "--group-directories-first"},
			wantOut: "zdir\nlink\ndir\nblink\nb\na\n",
		},
		{
			name:    "-U disables the grouping",
			files:   files,
			args:    []string{"-1", "-U", "--group-directories-first"},
			wantOut: "a\nb\nblink\ndir\nlink\nzdir\n",
		},
		{
			name:    "-f disables the grouping",
			files:   files,
			args:    []string{"-f", "--group-directories-first"},
			wantOut: "a\nb\nblink\ndir\nlink\nzdir\n",
		},
	})
}

// TestRunWidthTabsize checks -w and -T.
func TestRunWidthTabsize(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "-w sets the width the columns fill",
			files:   fsx.MemoryFS{"aaaa": {}, "bb": {}, "cc": {}},
			args:    []string{"-C", "-w", "10"},
			wantOut: "aaaa  cc\nbb\n",
		},
		{
			name:    "-w 0 writes the entries on one line",
			files:   fsx.MemoryFS{"aaaa": {}, "bb": {}},
			args:    []string{"-C", "-w", "0"},
			wantOut: "aaaa  bb\n",
		},
		{
			name:    "-w 0 fills the -m line without wrapping",
			files:   fsx.MemoryFS{"aaaa": {}, "bb": {}},
			args:    []string{"-m", "-w", "0"},
			wantOut: "aaaa, bb\n",
		},
		{
			name:    "the width of -w overrides COLUMNS",
			files:   fsx.MemoryFS{"aaaa": {}, "bb": {}, "cc": {}},
			env:     map[string]string{"COLUMNS": "80"},
			args:    []string{"-C", "-w", "10"},
			wantOut: "aaaa  cc\nbb\n",
		},
		{
			name:    "a padding that spans a tab stop is written with a tab",
			files:   fsx.MemoryFS{"aaaaaa": {}, "b": {}},
			env:     map[string]string{"COLUMNS": "20"},
			args:    []string{"-C"},
			wantOut: "aaaaaa\tb\n",
		},
		{
			name:    "-T 0 writes only spaces",
			files:   fsx.MemoryFS{"aaaaaa": {}, "b": {}},
			env:     map[string]string{"COLUMNS": "20"},
			args:    []string{"-C", "-T", "0"},
			wantOut: "aaaaaa  b\n",
		},
		{
			name:    "TABSIZE sets the distance between the stops",
			files:   fsx.MemoryFS{"aaaaaa": {}, "b": {}},
			env:     map[string]string{"COLUMNS": "20", "TABSIZE": "0"},
			args:    []string{"-C"},
			wantOut: "aaaaaa  b\n",
		},
		{
			name:    "an invalid TABSIZE is ignored with a warning",
			files:   fsx.MemoryFS{"a": {}},
			env:     map[string]string{"TABSIZE": "banana"},
			args:    []string{"-1"},
			wantOut: "a\n",
			wantErr: "ls: ignoring invalid tab size in environment variable TABSIZE: banana\n",
		},
		{
			name:     "an invalid tab size is an error",
			files:    fsx.MemoryFS{"a": {}},
			args:     []string{"-T", "banana"},
			wantErr:  "ls: invalid tab size 'banana'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid width is an error",
			files:    fsx.MemoryFS{"a": {}},
			args:     []string{"-w", "banana"},
			wantErr:  "ls: invalid line width 'banana'\nTry 'ls --help' for more information.\n",
			wantExit: 2,
		},
	})
}

// TestRunZero checks --zero.
func TestRunZero(t *testing.T) {
	now := newTimes()

	runCases(t, []testCase{
		{
			name:    "--zero ends the lines with NUL",
			files:   fsx.MemoryFS{"a": {}, "b": {}},
			args:    []string{"--zero"},
			wantOut: "a\x00b\x00",
		},
		{
			name:    "--zero ends the long lines with NUL",
			files:   fsx.MemoryFS{"a": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"--zero", "-ln"},
			wantOut: "total 0\x00-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a\x00",
		},
		{
			name:    "--zero keeps the newline of the headers",
			files:   fsx.MemoryFS{"dir/a": {}},
			args:    []string{"--zero", "-R"},
			wantOut: ".:\ndir\x00\n./dir:\na\x00",
		},
		{
			name:    "--zero implies the literal quoting",
			files:   fsx.MemoryFS{"a b": {}},
			args:    []string{"--zero"},
			wantOut: "a b\x00",
		},
		{
			name:    "a later quoting option overrides --zero",
			files:   fsx.MemoryFS{"a b": {}},
			args:    []string{"--zero", "--quoting-style=shell"},
			wantOut: "'a b'\x00",
		},
		{
			name:    "a later -C overrides the single column of --zero",
			files:   fsx.MemoryFS{"a": {}, "b": {}},
			args:    []string{"--zero", "-C"},
			wantOut: "a  b\x00",
		},
		{
			name:    "a later -l keeps the long format with --zero",
			files:   fsx.MemoryFS{"a": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-1", "--zero", "-ln"},
			wantOut: "total 0\x00-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " a\x00",
		},
		{
			name:    "-m writes the frills without padding",
			files:   fsx.MemoryFS{"a": {Data: []byte("x")}, "bb": {}},
			args:    []string{"-m", "-i", "-s"},
			wantOut: "total 1\n0 1 a, 0 0 bb\n",
		},
		{
			name:    "--zero writes the -m list on one line",
			files:   fsx.MemoryFS{"a": {}, "b": {}},
			args:    []string{"--zero", "-m"},
			wantOut: "a, b\x00",
		},
	})
}

// TestRunTerminal checks the defaults of a listing written to a terminal.
func TestRunTerminal(t *testing.T) {
	now := newTimes()
	files := fsx.MemoryFS{"a b": {}, "a\nb": {}, "plain": {Mode: 0o644, ModTime: now.recent}}
	for _, test := range []testCase{
		{
			name:    "the entries fill the columns and escape the control characters",
			files:   files,
			args:    []string{},
			wantOut: "'a'$'\\n''b'   plain\n'a b'\n",
		},
		{
			name:    "an explicit -1 keeps one entry per line",
			files:   files,
			args:    []string{"-1"},
			wantOut: "'a'$'\\n''b'\n'a b'\nplain\n",
		},
		{
			name:    "an explicit quoting option overrides the default",
			files:   fsx.MemoryFS{"a b": {}},
			args:    []string{"-N"},
			wantOut: "a b\n",
		},
		{
			name:    "--show-control-chars keeps the characters of a literal name as they are",
			files:   fsx.MemoryFS{"a\nb": {}},
			args:    []string{"-N", "--show-control-chars"},
			wantOut: "a\nb\n",
		},
		{
			name:    "the long format stays one entry per line",
			files:   fsx.MemoryFS{"plain": {Mode: 0o644, ModTime: now.recent}},
			args:    []string{"-ln"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " plain\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.env = map[string]string{"COLUMNS": "20"}
			test.terminal = true
			runCase(t, test)
		})
	}
}

// TestRunDerefArgDirs checks --dereference-command-line-symlink-to-dir.
func TestRunDerefArgDirs(t *testing.T) {
	now := newTimes()
	linked := fsx.MemoryFS{
		"dir":    {Mode: dirMode(0o755)},
		"dir/in": {Mode: 0o644, ModTime: now.recent},
		"to-dir": linkAt("dir", now.recent),
	}

	runCases(t, []testCase{
		{
			name:    "the contents of an operand link to a directory are listed",
			files:   linked,
			args:    []string{"--dereference-command-line-symlink-to-dir", "to-dir"},
			wantOut: "in\n",
		},
		{
			name:    "-d keeps the link itself with the option",
			files:   linked,
			args:    []string{"-d", "--dereference-command-line-symlink-to-dir", "to-dir"},
			wantOut: "to-dir\n",
		},
		{
			name:    "-l lists the contents of the link with the option",
			files:   linked,
			args:    []string{"-l", "--dereference-command-line-symlink-to-dir", "to-dir"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 " + now.day(now.recent) + " in\n",
		},
		{
			name:    "-F lists the contents of the link with the option",
			files:   linked,
			args:    []string{"-F", "--dereference-command-line-symlink-to-dir", "to-dir"},
			wantOut: "in\n",
		},
	})
}

// TestRunAuthorGroup checks --author and -G.
func TestRunAuthorGroup(t *testing.T) {
	now := newTimes()
	files := fsx.MemoryFS{"a": {Mode: 0o644, ModTime: now.recent}}

	runCases(t, []testCase{
		{
			name:    "--author writes the owner again as the author",
			files:   files,
			args:    []string{"-l", "--author", "-n"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 0 0 " + now.day(now.recent) + " a\n",
		},
		{
			name:    "-G drops the group without turning the format long",
			files:   fsx.MemoryFS{"a": {}},
			args:    []string{"-G"},
			wantOut: "a\n",
		},
		{
			name:    "-G drops the group of the long format",
			files:   files,
			args:    []string{"-lG", "-n"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 " + now.day(now.recent) + " a\n",
		},
		{
			name:    "-o is -l with -G",
			files:   files,
			args:    []string{"-o"},
			wantOut: "total 0\n-rw-r--r-- 1 0 0 " + now.day(now.recent) + " a\n",
		},
	})
}

// fakeInfo is a file info with a chosen size, for the block accounting of
// files a file system measures by their size.
type fakeInfo struct {
	name string
	size int64
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return f.size }
func (f fakeInfo) Mode() fs.FileMode  { return 0o644 }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return false }
func (f fakeInfo) Sys() any           { return nil }

// TestBlocksOfSaturates checks that a file too large to count keeps a
// count that fits instead of overflowing.
func TestBlocksOfSaturates(t *testing.T) {
	for _, test := range []struct {
		size int64
		want uint64
	}{
		{0, 0},
		{1, 512},
		{511, 512},
		{512, 512},
		{513, 1024},
		{1 << 40, 1 << 40},
		{1<<62 - 1, 1 << 62},
		{math.MaxInt64, math.MaxInt64},
	} {
		if got := blocksOf(fakeInfo{size: test.size}); got != test.want {
			t.Errorf("blocksOf(size %d) = %d, want %d", test.size, got, test.want)
		}
	}
}
