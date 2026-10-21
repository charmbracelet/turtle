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
	"charm.land/snail/fsx"
)

type testCase struct {
	name     string
	files    fsx.MemFS
	wd       string
	stdin    string
	args     []string
	wantOut  string
	present  []string
	absent   []string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      strings.NewReader(test.stdin),
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         test.files,
		WorkingDir: test.wd,
	}

	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
	if got := stdout.String(); got != test.wantOut {
		t.Errorf("Run() stdout = %q, want %q", got, test.wantOut)
	}

	for _, name := range test.present {
		if _, err := fs.Stat(test.files, name); err != nil {
			t.Errorf("Stat(%q) error = %v, want it to still exist", name, err)
		}
	}
	for _, name := range test.absent {
		if _, err := fs.Stat(test.files, name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat(%q) error = %v, want fs.ErrNotExist", name, err)
		}
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

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			name:   "removes a file",
			files:  fsx.MemFS{"a.txt": {Data: []byte("hello")}},
			args:   []string{"a.txt"},
			absent: []string{"a.txt"},
		},
		{
			name:   "removes every file in order",
			files:  fsx.MemFS{"one": {Data: []byte("1")}, "two": {Data: []byte("2")}},
			args:   []string{"one", "two"},
			absent: []string{"one", "two"},
		},
		{
			name:     "a missing file is an error",
			files:    fsx.MemFS{},
			args:     []string{"missing"},
			wantErr:  "rm: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "keeps removing the others after an error",
			files:    fsx.MemFS{"b.txt": {Data: []byte("b")}},
			args:     []string{"missing", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "rm: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a directory without -r or -d is an error",
			files:    fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:     []string{"d"},
			present:  []string{"d/f.txt"},
			wantErr:  "rm: d: is a directory\n",
			wantExit: 1,
		},
		{
			name:   "a trailing slash is dropped",
			files:  fsx.MemFS{"f.txt": {Data: []byte("f")}},
			args:   []string{"f.txt/"},
			absent: []string{"f.txt"},
		},
		{
			name:   "redundant separators are dropped",
			files:  fsx.MemFS{"a/f.txt": {Data: []byte("f")}},
			args:   []string{"a//f.txt"},
			absent: []string{"a/f.txt"},
		},
		{
			name:   "a file named like an option after a double dash",
			files:  fsx.MemFS{"-f": {Data: []byte("f")}},
			args:   []string{"--", "-f"},
			absent: []string{"-f"},
		},
		{
			name:   "a file named with a dash",
			files:  fsx.MemFS{"-": {Data: []byte("f")}},
			args:   []string{"-"},
			absent: []string{"-"},
		},
		{
			name:     "the current directory is refused",
			files:    fsx.MemFS{},
			args:     []string{"."},
			wantErr:  "rm: .: refusing to remove '.' or '..' directory\n",
			wantExit: 1,
		},
		{
			name:     "the parent directory is refused",
			files:    fsx.MemFS{},
			args:     []string{".."},
			wantErr:  "rm: ..: refusing to remove '.' or '..' directory\n",
			wantExit: 1,
		},
		{
			name:     "a path ending in dot is refused",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:     []string{"d/."},
			present:  []string{"d"},
			wantErr:  "rm: d/.: refusing to remove '.' or '..' directory\n",
			wantExit: 1,
		},
		{
			name:     "a path ending in dot-dot is refused",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:     []string{"d/.."},
			present:  []string{"d"},
			wantErr:  "rm: d/..: refusing to remove '.' or '..' directory\n",
			wantExit: 1,
		},
		{
			name:     "the root directory is refused",
			files:    fsx.MemFS{},
			args:     []string{"/"},
			wantErr:  "rm: /: refusing to remove root directory\n",
			wantExit: 1,
		},
		{
			name:     "the root directory with slashes is refused",
			files:    fsx.MemFS{},
			args:     []string{"//"},
			wantErr:  "rm: //: refusing to remove root directory\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunForce(t *testing.T) {
	tests := []testCase{
		{
			name:  "a missing file is silent",
			files: fsx.MemFS{},
			args:  []string{"-f", "missing"},
		},
		{
			name:  "no operands is silent",
			files: fsx.MemFS{},
			args:  []string{"-f"},
		},
		{
			name:   "missing files don't stop the others",
			files:  fsx.MemFS{"b.txt": {Data: []byte("b")}},
			args:   []string{"-f", "missing", "b.txt"},
			absent: []string{"b.txt"},
		},
		{
			name:     "a directory is still an error",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:     []string{"-f", "d"},
			present:  []string{"d"},
			wantErr:  "rm: d: is a directory\n",
			wantExit: 1,
		},
		{
			name:   "f cancels an earlier i",
			files:  fsx.MemFS{"a.txt": {Data: []byte("a")}},
			args:   []string{"-if", "a.txt"},
			absent: []string{"a.txt"},
		},
		{
			name:    "i cancels an earlier f",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
			stdin:   "n\n",
			args:    []string{"-fi", "a.txt"},
			present: []string{"a.txt"},
			wantErr: "rm: remove 'a.txt'? ",
		},
		{
			name:   "the last of f and i wins",
			files:  fsx.MemFS{"a.txt": {Data: []byte("a")}},
			stdin:  "n\n",
			args:   []string{"-i", "-f", "a.txt"},
			absent: []string{"a.txt"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDirectories(t *testing.T) {
	tests := []testCase{
		{
			name:   "removes an empty directory",
			files:  fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:   []string{"-d", "d"},
			absent: []string{"d"},
		},
		{
			name:     "a non-empty directory is an error",
			files:    fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:     []string{"-d", "d"},
			present:  []string{"d/f.txt"},
			wantErr:  "rm: d: directory not empty\n",
			wantExit: 1,
		},
		{
			name:   "a file is removed like without -d",
			files:  fsx.MemFS{"f.txt": {Data: []byte("f")}},
			args:   []string{"-d", "f.txt"},
			absent: []string{"f.txt"},
		},
		{
			name:     "a missing directory is an error",
			files:    fsx.MemFS{},
			args:     []string{"-d", "missing"},
			wantErr:  "rm: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:   "r takes precedence over d",
			files:  fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:   []string{"-rd", "d"},
			absent: []string{"d", "d/f.txt"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRecursive(t *testing.T) {
	tests := []testCase{
		{
			name:   "removes a hierarchy with an implicit directory",
			files:  fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			args:   []string{"-r", "d"},
			absent: []string{"d", "d/f.txt", "d/sub", "d/sub/g.txt"},
		},
		{
			name:   "removes a hierarchy with an explicit directory",
			files:  fsx.MemFS{"d": {Mode: fs.ModeDir}, "d/f.txt": {Data: []byte("f")}},
			args:   []string{"-r", "d"},
			absent: []string{"d", "d/f.txt"},
		},
		{
			name:   "removes an explicit empty directory",
			files:  fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:   []string{"-r", "d"},
			absent: []string{"d"},
		},
		{
			name:   "removes every operand",
			files:  fsx.MemFS{"a/x.txt": {Data: []byte("x")}, "b/y.txt": {Data: []byte("y")}},
			args:   []string{"-r", "a", "b"},
			absent: []string{"a", "a/x.txt", "b", "b/y.txt"},
		},
		{
			name:   "removes a file operand",
			files:  fsx.MemFS{"f.txt": {Data: []byte("f")}},
			args:   []string{"-r", "f.txt"},
			absent: []string{"f.txt"},
		},
		{
			name:   "R is equivalent to r",
			files:  fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:   []string{"-R", "d"},
			absent: []string{"d", "d/f.txt"},
		},
		{
			name:     "a missing operand is an error",
			files:    fsx.MemFS{},
			args:     []string{"-r", "missing"},
			wantErr:  "rm: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "keeps removing the others after an error",
			files:    fsx.MemFS{"a/f.txt": {Data: []byte("f")}},
			args:     []string{"-r", "missing", "a"},
			absent:   []string{"a", "a/f.txt"},
			wantErr:  "rm: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:   "removes a deep hierarchy",
			files:  fsx.MemFS{"a/b/c/d/e.txt": {Data: []byte("e")}},
			args:   []string{"-r", "a"},
			absent: []string{"a", "a/b", "a/b/c", "a/b/c/d", "a/b/c/d/e.txt"},
		},
		{
			name:   "a trailing slash is dropped",
			files:  fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:   []string{"-r", "d/"},
			absent: []string{"d", "d/f.txt"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunInteractive(t *testing.T) {
	tests := []testCase{
		{
			name:    "prompts and removes when affirmative",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
			stdin:   "y\n",
			args:    []string{"-i", "a.txt"},
			absent:  []string{"a.txt"},
			wantErr: "rm: remove 'a.txt'? ",
		},
		{
			name:    "a capital answer is affirmative too",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
			stdin:   "Y\n",
			args:    []string{"-i", "a.txt"},
			absent:  []string{"a.txt"},
			wantErr: "rm: remove 'a.txt'? ",
		},
		{
			name:    "declines on a negative answer",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
			stdin:   "n\n",
			args:    []string{"-i", "a.txt"},
			present: []string{"a.txt"},
			wantErr: "rm: remove 'a.txt'? ",
		},
		{
			name:    "declines on any other answer",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
			stdin:   "maybe\n",
			args:    []string{"-i", "a.txt"},
			present: []string{"a.txt"},
			wantErr: "rm: remove 'a.txt'? ",
		},
		{
			name:    "declines at the end of the input",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
			args:    []string{"-i", "a.txt"},
			present: []string{"a.txt"},
			wantErr: "rm: remove 'a.txt'? ",
		},
		{
			name:    "prompts for every operand",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}},
			stdin:   "y\nn\n",
			args:    []string{"-i", "a.txt", "b.txt"},
			absent:  []string{"a.txt"},
			present: []string{"b.txt"},
			wantErr: "rm: remove 'a.txt'? rm: remove 'b.txt'? ",
		},
		{
			name:    "a file with r only prompts to remove it",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
			stdin:   "y\n",
			args:    []string{"-ri", "a.txt"},
			absent:  []string{"a.txt"},
			wantErr: "rm: remove 'a.txt'? ",
		},
		{
			name:    "an empty directory with r only prompts to remove it",
			files:   fsx.MemFS{"d": {Mode: fs.ModeDir}},
			stdin:   "y\n",
			args:    []string{"-ri", "d"},
			absent:  []string{"d"},
			wantErr: "rm: remove 'd'? ",
		},
		{
			name:    "a hierarchy prompts to descend, for each entry, and to remove the directory",
			files:   fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			stdin:   "y\ny\ny\n",
			args:    []string{"-ri", "d"},
			absent:  []string{"d", "d/f.txt"},
			wantErr: "rm: descend into directory 'd'? rm: remove 'd/f.txt'? rm: remove 'd'? ",
		},
		{
			name:    "declining the descend prompt skips the whole hierarchy",
			files:   fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			stdin:   "n\n",
			args:    []string{"-ri", "d"},
			present: []string{"d", "d/f.txt"},
			wantErr: "rm: descend into directory 'd'? ",
		},
		{
			name:    "declining the last prompt leaves an emptied directory",
			files:   fsx.MemFS{"d": {Mode: fs.ModeDir}, "d/f.txt": {Data: []byte("f")}},
			stdin:   "y\ny\nn\n",
			args:    []string{"-ri", "d"},
			present: []string{"d"},
			absent:  []string{"d/f.txt"},
			wantErr: "rm: descend into directory 'd'? rm: remove 'd/f.txt'? rm: remove 'd'? ",
		},
		{
			name:     "a directory left non-empty by a declined entry fails to be removed",
			files:    fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			stdin:    "y\nn\ny\n",
			args:     []string{"-ri", "d"},
			present:  []string{"d", "d/f.txt"},
			wantErr:  "rm: descend into directory 'd'? rm: remove 'd/f.txt'? rm: remove 'd'? rm: d: directory not empty\n",
			wantExit: 1,
		},
		{
			name:    "a directory with d prompts to remove it",
			files:   fsx.MemFS{"d": {Mode: fs.ModeDir}},
			stdin:   "n\n",
			args:    []string{"-di", "d"},
			present: []string{"d"},
			wantErr: "rm: remove 'd'? ",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
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
	tests := []testCase{
		{
			name:    "prints every removed file",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}},
			args:    []string{"-v", "a.txt", "b.txt"},
			absent:  []string{"a.txt", "b.txt"},
			wantOut: "removed 'a.txt'\nremoved 'b.txt'\n",
		},
		{
			name:    "prints every entry of a hierarchy, deepest first",
			files:   fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			args:    []string{"-rv", "d"},
			absent:  []string{"d", "d/f.txt", "d/sub", "d/sub/g.txt"},
			wantOut: "removed 'd/f.txt'\nremoved 'd/sub/g.txt'\nremoved 'd/sub'\nremoved 'd'\n",
		},
		{
			name:    "prints nothing without v",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
			args:    []string{"a.txt"},
			absent:  []string{"a.txt"},
			wantOut: "",
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
			name:   "dir removes an empty directory",
			files:  fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:   []string{"--dir", "d"},
			absent: []string{"d"},
		},
		{
			name:  "force ignores a missing file",
			files: fsx.MemFS{},
			args:  []string{"--force", "missing"},
		},
		{
			name:    "interactive prompts and removes when affirmative",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
			stdin:   "y\n",
			args:    []string{"--interactive", "a.txt"},
			absent:  []string{"a.txt"},
			wantErr: "rm: remove 'a.txt'? ",
		},
		{
			name:   "recursive removes a hierarchy",
			files:  fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:   []string{"--recursive", "d"},
			absent: []string{"d", "d/f.txt"},
		},
		{
			name:    "verbose prints every removed file",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
			args:    []string{"--verbose", "a.txt"},
			absent:  []string{"a.txt"},
			wantOut: "removed 'a.txt'\n",
		},
		{
			name:     "a long option with a value it doesn't take is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("a")}},
			args:     []string{"--force=yes", "a.txt"},
			present:  []string{"a.txt"},
			wantErr:  "rm: option '--force=yes' doesn't allow an argument\n",
			wantExit: 2,
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
			name:     "missing operand",
			files:    fsx.MemFS{},
			args:     []string{},
			wantErr:  "rm: missing operand\n",
			wantExit: 2,
		},
		{
			name:     "invalid option",
			files:    fsx.MemFS{"a.txt": {Data: []byte("a")}},
			args:     []string{"-x", "a.txt"},
			present:  []string{"a.txt"},
			wantErr:  "rm: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option",
			files:    fsx.MemFS{"a.txt": {Data: []byte("a")}},
			args:     []string{"--bogus", "a.txt"},
			present:  []string{"a.txt"},
			wantErr:  "rm: unrecognized option '--bogus'\n",
			wantExit: 2,
		},
		{
			name:     "an empty operand",
			files:    fsx.MemFS{},
			args:     []string{""},
			wantErr:  "rm: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "options are not parsed after an operand",
			files:    fsx.MemFS{"a.txt": {Data: []byte("a")}},
			args:     []string{"a.txt", "-x"},
			absent:   []string{"a.txt"},
			wantErr:  "rm: -x: file does not exist\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunWorkingDir(t *testing.T) {
	runCase(t, testCase{
		name:   "resolves operands relative to the working directory",
		files:  fsx.MemFS{"work/a.txt": {Data: []byte("a")}},
		wd:     "work",
		args:   []string{"a.txt"},
		absent: []string{"work/a.txt"},
	})
}

func TestRunWindowsSeparators(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only")
	}

	runCase(t, testCase{
		name:   "backslashes are treated as separators",
		files:  fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
		args:   []string{"d\\f.txt"},
		absent: []string{"d/f.txt"},
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
