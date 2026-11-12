package rm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"runtime"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:   "removes a file",
			FS:     fsx.MemFS{"a.txt": {Data: []byte("hello")}},
			Args:   []string{"a.txt"},
			Absent: []string{"a.txt"},
		},
		{
			Name:   "removes every file in order",
			FS:     fsx.MemFS{"one": {Data: []byte("1")}, "two": {Data: []byte("2")}},
			Args:   []string{"one", "two"},
			Absent: []string{"one", "two"},
		},
		{
			Name:     "a missing file is an error",
			FS:       fsx.MemFS{},
			Args:     []string{"missing"},
			WantErr:  "rm: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "keeps removing the others after an error",
			FS:       fsx.MemFS{"b.txt": {Data: []byte("b")}},
			Args:     []string{"missing", "b.txt"},
			Absent:   []string{"b.txt"},
			WantErr:  "rm: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a directory without -r or -d is an error",
			FS:       fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			Args:     []string{"d"},
			Present:  []string{"d/f.txt"},
			WantErr:  "rm: d: is a directory\n",
			WantExit: 1,
		},
		{
			Name:   "a trailing slash is dropped",
			FS:     fsx.MemFS{"f.txt": {Data: []byte("f")}},
			Args:   []string{"f.txt/"},
			Absent: []string{"f.txt"},
		},
		{
			Name:   "redundant separators are dropped",
			FS:     fsx.MemFS{"a/f.txt": {Data: []byte("f")}},
			Args:   []string{"a//f.txt"},
			Absent: []string{"a/f.txt"},
		},
		{
			Name:   "a file named like an option after a double dash",
			FS:     fsx.MemFS{"-f": {Data: []byte("f")}},
			Args:   []string{"--", "-f"},
			Absent: []string{"-f"},
		},
		{
			Name:   "a file named with a dash",
			FS:     fsx.MemFS{"-": {Data: []byte("f")}},
			Args:   []string{"-"},
			Absent: []string{"-"},
		},
		{
			Name:     "the current directory is refused",
			FS:       fsx.MemFS{},
			Args:     []string{"."},
			WantErr:  "rm: .: refusing to remove '.' or '..' directory\n",
			WantExit: 1,
		},
		{
			Name:     "the parent directory is refused",
			FS:       fsx.MemFS{},
			Args:     []string{".."},
			WantErr:  "rm: ..: refusing to remove '.' or '..' directory\n",
			WantExit: 1,
		},
		{
			Name:     "a path ending in dot is refused",
			FS:       fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Args:     []string{"d/."},
			Present:  []string{"d"},
			WantErr:  "rm: d/.: refusing to remove '.' or '..' directory\n",
			WantExit: 1,
		},
		{
			Name:     "a path ending in dot-dot is refused",
			FS:       fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Args:     []string{"d/.."},
			Present:  []string{"d"},
			WantErr:  "rm: d/..: refusing to remove '.' or '..' directory\n",
			WantExit: 1,
		},
		{
			Name:     "the root directory is refused",
			FS:       fsx.MemFS{},
			Args:     []string{"/"},
			WantErr:  "rm: /: refusing to remove root directory\n",
			WantExit: 1,
		},
		{
			Name:     "the root directory with slashes is refused",
			FS:       fsx.MemFS{},
			Args:     []string{"//"},
			WantErr:  "rm: //: refusing to remove root directory\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunForce(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a missing file is silent",
			FS:   fsx.MemFS{},
			Args: []string{"-f", "missing"},
		},
		{
			Name: "no operands is silent",
			FS:   fsx.MemFS{},
			Args: []string{"-f"},
		},
		{
			Name:   "missing files don't stop the others",
			FS:     fsx.MemFS{"b.txt": {Data: []byte("b")}},
			Args:   []string{"-f", "missing", "b.txt"},
			Absent: []string{"b.txt"},
		},
		{
			Name:     "a directory is still an error",
			FS:       fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Args:     []string{"-f", "d"},
			Present:  []string{"d"},
			WantErr:  "rm: d: is a directory\n",
			WantExit: 1,
		},
		{
			Name:   "f cancels an earlier i",
			FS:     fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Args:   []string{"-if", "a.txt"},
			Absent: []string{"a.txt"},
		},
		{
			Name:    "i cancels an earlier f",
			FS:      fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Stdin:   strings.NewReader("n\n"),
			Args:    []string{"-fi", "a.txt"},
			Present: []string{"a.txt"},
			WantErr: "rm: remove 'a.txt'? ",
		},
		{
			Name:   "the last of f and i wins",
			FS:     fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Stdin:  strings.NewReader("n\n"),
			Args:   []string{"-i", "-f", "a.txt"},
			Absent: []string{"a.txt"},
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunDirectories(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:   "removes an empty directory",
			FS:     fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Args:   []string{"-d", "d"},
			Absent: []string{"d"},
		},
		{
			Name:     "a non-empty directory is an error",
			FS:       fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			Args:     []string{"-d", "d"},
			Present:  []string{"d/f.txt"},
			WantErr:  "rm: d: directory not empty\n",
			WantExit: 1,
		},
		{
			Name:   "a file is removed like without -d",
			FS:     fsx.MemFS{"f.txt": {Data: []byte("f")}},
			Args:   []string{"-d", "f.txt"},
			Absent: []string{"f.txt"},
		},
		{
			Name:     "a missing directory is an error",
			FS:       fsx.MemFS{},
			Args:     []string{"-d", "missing"},
			WantErr:  "rm: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:   "r takes precedence over d",
			FS:     fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			Args:   []string{"-rd", "d"},
			Absent: []string{"d", "d/f.txt"},
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunRecursive(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:   "removes a hierarchy with an implicit directory",
			FS:     fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			Args:   []string{"-r", "d"},
			Absent: []string{"d", "d/f.txt", "d/sub", "d/sub/g.txt"},
		},
		{
			Name:   "removes a hierarchy with an explicit directory",
			FS:     fsx.MemFS{"d": {Mode: fs.ModeDir}, "d/f.txt": {Data: []byte("f")}},
			Args:   []string{"-r", "d"},
			Absent: []string{"d", "d/f.txt"},
		},
		{
			Name:   "removes an explicit empty directory",
			FS:     fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Args:   []string{"-r", "d"},
			Absent: []string{"d"},
		},
		{
			Name:   "removes every operand",
			FS:     fsx.MemFS{"a/x.txt": {Data: []byte("x")}, "b/y.txt": {Data: []byte("y")}},
			Args:   []string{"-r", "a", "b"},
			Absent: []string{"a", "a/x.txt", "b", "b/y.txt"},
		},
		{
			Name:   "removes a file operand",
			FS:     fsx.MemFS{"f.txt": {Data: []byte("f")}},
			Args:   []string{"-r", "f.txt"},
			Absent: []string{"f.txt"},
		},
		{
			Name:   "R is equivalent to r",
			FS:     fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			Args:   []string{"-R", "d"},
			Absent: []string{"d", "d/f.txt"},
		},
		{
			Name:     "a missing operand is an error",
			FS:       fsx.MemFS{},
			Args:     []string{"-r", "missing"},
			WantErr:  "rm: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "keeps removing the others after an error",
			FS:       fsx.MemFS{"a/f.txt": {Data: []byte("f")}},
			Args:     []string{"-r", "missing", "a"},
			Absent:   []string{"a", "a/f.txt"},
			WantErr:  "rm: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:   "removes a deep hierarchy",
			FS:     fsx.MemFS{"a/b/c/d/e.txt": {Data: []byte("e")}},
			Args:   []string{"-r", "a"},
			Absent: []string{"a", "a/b", "a/b/c", "a/b/c/d", "a/b/c/d/e.txt"},
		},
		{
			Name:   "a trailing slash is dropped",
			FS:     fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			Args:   []string{"-r", "d/"},
			Absent: []string{"d", "d/f.txt"},
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunInteractive(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:    "prompts and removes when affirmative",
			FS:      fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Stdin:   strings.NewReader("y\n"),
			Args:    []string{"-i", "a.txt"},
			Absent:  []string{"a.txt"},
			WantErr: "rm: remove 'a.txt'? ",
		},
		{
			Name:    "a capital answer is affirmative too",
			FS:      fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Stdin:   strings.NewReader("Y\n"),
			Args:    []string{"-i", "a.txt"},
			Absent:  []string{"a.txt"},
			WantErr: "rm: remove 'a.txt'? ",
		},
		{
			Name:    "declines on a negative answer",
			FS:      fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Stdin:   strings.NewReader("n\n"),
			Args:    []string{"-i", "a.txt"},
			Present: []string{"a.txt"},
			WantErr: "rm: remove 'a.txt'? ",
		},
		{
			Name:    "declines on any other answer",
			FS:      fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Stdin:   strings.NewReader("maybe\n"),
			Args:    []string{"-i", "a.txt"},
			Present: []string{"a.txt"},
			WantErr: "rm: remove 'a.txt'? ",
		},
		{
			Name:    "declines at the end of the input",
			FS:      fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Args:    []string{"-i", "a.txt"},
			Present: []string{"a.txt"},
			WantErr: "rm: remove 'a.txt'? ",
		},
		{
			Name:    "prompts for every operand",
			FS:      fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}},
			Stdin:   strings.NewReader("y\nn\n"),
			Args:    []string{"-i", "a.txt", "b.txt"},
			Absent:  []string{"a.txt"},
			Present: []string{"b.txt"},
			WantErr: "rm: remove 'a.txt'? rm: remove 'b.txt'? ",
		},
		{
			Name:    "a file with r only prompts to remove it",
			FS:      fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Stdin:   strings.NewReader("y\n"),
			Args:    []string{"-ri", "a.txt"},
			Absent:  []string{"a.txt"},
			WantErr: "rm: remove 'a.txt'? ",
		},
		{
			Name:    "an empty directory with r only prompts to remove it",
			FS:      fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Stdin:   strings.NewReader("y\n"),
			Args:    []string{"-ri", "d"},
			Absent:  []string{"d"},
			WantErr: "rm: remove 'd'? ",
		},
		{
			Name:    "a hierarchy prompts to descend, for each entry, and to remove the directory",
			FS:      fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			Stdin:   strings.NewReader("y\ny\ny\n"),
			Args:    []string{"-ri", "d"},
			Absent:  []string{"d", "d/f.txt"},
			WantErr: "rm: descend into directory 'd'? rm: remove 'd/f.txt'? rm: remove 'd'? ",
		},
		{
			Name:    "declining the descend prompt skips the whole hierarchy",
			FS:      fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			Stdin:   strings.NewReader("n\n"),
			Args:    []string{"-ri", "d"},
			Present: []string{"d", "d/f.txt"},
			WantErr: "rm: descend into directory 'd'? ",
		},
		{
			Name:    "declining the last prompt leaves an emptied directory",
			FS:      fsx.MemFS{"d": {Mode: fs.ModeDir}, "d/f.txt": {Data: []byte("f")}},
			Stdin:   strings.NewReader("y\ny\nn\n"),
			Args:    []string{"-ri", "d"},
			Present: []string{"d"},
			Absent:  []string{"d/f.txt"},
			WantErr: "rm: descend into directory 'd'? rm: remove 'd/f.txt'? rm: remove 'd'? ",
		},
		{
			Name:     "a directory left non-empty by a declined entry fails to be removed",
			FS:       fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			Stdin:    strings.NewReader("y\nn\ny\n"),
			Args:     []string{"-ri", "d"},
			Present:  []string{"d", "d/f.txt"},
			WantErr:  "rm: descend into directory 'd'? rm: remove 'd/f.txt'? rm: remove 'd'? rm: d: directory not empty\n",
			WantExit: 1,
		},
		{
			Name:    "a directory with d prompts to remove it",
			FS:      fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Stdin:   strings.NewReader("n\n"),
			Args:    []string{"-di", "d"},
			Present: []string{"d"},
			WantErr: "rm: remove 'd'? ",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunInteractiveWithoutStdin(t *testing.T) {
	files := fsx.MemFS{"a.txt": {Data: []byte("a")}}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stderr: stderr, FS: files}

	if err := New(rt).Run(t.Context(), "-i", "a.txt"); err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
	if want := "rm: remove 'a.txt'? "; stderr.String() != want {
		t.Errorf("Run() stderr = %q, want %q", stderr.String(), want)
	}
	if _, err := fs.Stat(files, "a.txt"); err != nil {
		t.Errorf("Stat(%q) error = %v, want it to still exist", "a.txt", err)
	}
}

func TestRunVerbose(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:   "prints every removed file",
			FS:     fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}},
			Args:   []string{"-v", "a.txt", "b.txt"},
			Absent: []string{"a.txt", "b.txt"},
			Want:   "removed 'a.txt'\nremoved 'b.txt'\n",
		},
		{
			Name:   "prints every entry of a hierarchy, deepest first",
			FS:     fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			Args:   []string{"-rv", "d"},
			Absent: []string{"d", "d/f.txt", "d/sub", "d/sub/g.txt"},
			Want:   "removed 'd/f.txt'\nremoved 'd/sub/g.txt'\nremoved 'd/sub'\nremoved 'd'\n",
		},
		{
			Name:   "prints nothing without v",
			FS:     fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Args:   []string{"a.txt"},
			Absent: []string{"a.txt"},
			Want:   "",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:   "dir removes an empty directory",
			FS:     fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Args:   []string{"--dir", "d"},
			Absent: []string{"d"},
		},
		{
			Name: "force ignores a missing file",
			FS:   fsx.MemFS{},
			Args: []string{"--force", "missing"},
		},
		{
			Name:    "interactive prompts and removes when affirmative",
			FS:      fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Stdin:   strings.NewReader("y\n"),
			Args:    []string{"--interactive", "a.txt"},
			Absent:  []string{"a.txt"},
			WantErr: "rm: remove 'a.txt'? ",
		},
		{
			Name:   "recursive removes a hierarchy",
			FS:     fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			Args:   []string{"--recursive", "d"},
			Absent: []string{"d", "d/f.txt"},
		},
		{
			Name:   "verbose prints every removed file",
			FS:     fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Args:   []string{"--verbose", "a.txt"},
			Absent: []string{"a.txt"},
			Want:   "removed 'a.txt'\n",
		},
		{
			Name:     "a long option with a value it doesn't take is an error",
			FS:       fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Args:     []string{"--force=yes", "a.txt"},
			Present:  []string{"a.txt"},
			WantErr:  "rm: option '--force=yes' doesn't allow an argument\nTry 'rm --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "missing operand",
			FS:       fsx.MemFS{},
			Args:     []string{},
			WantErr:  "rm: missing operand\nTry 'rm --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option",
			FS:       fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Args:     []string{"-x", "a.txt"},
			Present:  []string{"a.txt"},
			WantErr:  "rm: invalid option -- 'x'\nTry 'rm --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unrecognized long option",
			FS:       fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Args:     []string{"--bogus", "a.txt"},
			Present:  []string{"a.txt"},
			WantErr:  "rm: unrecognized option '--bogus'\nTry 'rm --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty operand",
			FS:       fsx.MemFS{},
			Args:     []string{""},
			WantErr:  "rm: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "options are not parsed after an operand",
			FS:       fsx.MemFS{"a.txt": {Data: []byte("a")}},
			Args:     []string{"a.txt", "-x"},
			Absent:   []string{"a.txt"},
			WantErr:  "rm: -x: file does not exist\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunWorkingDir(t *testing.T) {
	testutil.Once(t, New, testutil.Case{
		Name:   "resolves operands relative to the working directory",
		FS:     fsx.MemFS{"work/a.txt": {Data: []byte("a")}},
		Dir:    "work",
		Args:   []string{"a.txt"},
		Absent: []string{"work/a.txt"},
	})
}

func TestRunWindowsSeparators(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only")
	}

	testutil.Once(t, New, testutil.Case{
		Name:   "backslashes are treated as separators",
		FS:     fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
		Args:   []string{"d\\f.txt"},
		Absent: []string{"d/f.txt"},
	})
}

func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rt := &coreutils.Runtime{Stderr: io.Discard, FS: fsx.MemFS{"a.txt": {}}}
	err := New(rt).Run(ctx, "a.txt")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestProtectedErr(t *testing.T) {
	tests := []struct {
		name    string
		operand string
		wantErr error
	}{
		{name: "dot", operand: ".", wantErr: errDotDot},
		{name: "dot-dot", operand: "..", wantErr: errDotDot},
		{name: "trailing dot", operand: "d/.", wantErr: errDotDot},
		{name: "trailing dot-dot", operand: "d/..", wantErr: errDotDot},
		{name: "root", operand: "/", wantErr: errRoot},
		{name: "root with slashes", operand: "//", wantErr: errRoot},
		{name: "plain file", operand: "file", wantErr: nil},
		{name: "nested file", operand: "d/f", wantErr: nil},
		{name: "leading dots", operand: "..file", wantErr: nil},
		{name: "three dots", operand: "...", wantErr: nil},
		{name: "trailing slash", operand: "d/", wantErr: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := protectedErr(test.operand)
			if !errors.Is(err, test.wantErr) {
				t.Errorf("protectedErr(%q) = %v, want %v", test.operand, err, test.wantErr)
			}
		})
	}
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "rm", usage)
}
