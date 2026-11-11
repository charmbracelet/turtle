package realpath

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// tree returns a working directory with files, directories and symbolic
// links the tests resolve.
func tree() fsx.MemFS {
	return fsx.MemFS{
		"work/a.txt":                           {Data: []byte("hello")},
		"work/d/keep.txt":                      {},
		"work/x/b.txt":                         {Data: []byte("b")},
		"work/usr/share/dict/american-english": {Data: []byte("words")},
		"work/tmp/foo":                         {Data: []byte("tmp")},
		"work/link.txt":                        {Data: []byte("a.txt"), Mode: fs.ModeSymlink},
		"work/link-d":                          {Data: []byte("x"), Mode: fs.ModeSymlink},
		"work/dangling":                        {Data: []byte("missing.txt"), Mode: fs.ModeSymlink},
		"work/loop":                            {Data: []byte("loop"), Mode: fs.ModeSymlink},
		"work/ping":                            {Data: []byte("pong"), Mode: fs.ModeSymlink},
		"work/pong":                            {Data: []byte("ping"), Mode: fs.ModeSymlink},
	}
}

type testCase struct {
	name     string
	files    fsx.MemFS
	wd       string
	args     []string
	environ  []string
	wantOut  string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         test.files,
		WorkingDir: test.wd,
	}
	if test.environ != nil {
		rt.Environ = func() []string { return test.environ }
	}

	err := New(rt).Run(t.Context(), test.args...)

	if got := stdout.String(); got != test.wantOut {
		t.Errorf("Run() stdout = %q, want %q", got, test.wantOut)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
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
			name:    "resolves a file to the working directory",
			files:   tree(),
			wd:      "work",
			args:    []string{"a.txt"},
			wantOut: "work/a.txt\n",
		},
		{
			name:    "resolves each operand on its own line",
			files:   tree(),
			wd:      "work",
			args:    []string{"a.txt", "d/keep.txt"},
			wantOut: "work/a.txt\nwork/d/keep.txt\n",
		},
		{
			name:    "the working directory is a prefix of the output",
			files:   tree(),
			wd:      "work/d",
			args:    []string{"keep.txt"},
			wantOut: "work/d/keep.txt\n",
		},
		{
			name:    "resolves the working directory itself",
			files:   tree(),
			wd:      "work/d",
			args:    []string{"."},
			wantOut: "work/d\n",
		},
		{
			name:    "the parent of the working directory is the file system root",
			files:   tree(),
			wd:      "work",
			args:    []string{".."},
			wantOut: ".\n",
		},
		{
			name:    "removes dot and extra slash components",
			files:   tree(),
			wd:      "work",
			args:    []string{"d/./keep.txt", "d//keep.txt"},
			wantOut: "work/d/keep.txt\nwork/d/keep.txt\n",
		},
		{
			name:    "resolves the parent of a directory",
			files:   tree(),
			wd:      "work",
			args:    []string{"d/.."},
			wantOut: "work\n",
		},
		{
			name:    "a missing last component is not an error",
			files:   tree(),
			wd:      "work",
			args:    []string{"missing.txt"},
			wantOut: "work/missing.txt\n",
		},
		{
			name:     "a missing component before the last is an error",
			files:    tree(),
			wd:       "work",
			args:     []string{"missing/missing.txt"},
			wantErr:  "realpath: missing/missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "keeps going after a failing operand",
			files:    tree(),
			wd:       "work",
			args:     []string{"missing/x", "a.txt"},
			wantOut:  "work/a.txt\n",
			wantErr:  "realpath: missing/x: file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "-E is the default",
			files:   tree(),
			wd:      "work",
			args:    []string{"-E", "missing.txt"},
			wantOut: "work/missing.txt\n",
		},
		{
			name:    "ignores trailing slashes",
			files:   tree(),
			wd:      "work",
			args:    []string{"d/", "missing.txt/"},
			wantOut: "work/d\nwork/missing.txt\n",
		},
		{
			name:    "resolves a symbolic link to a file",
			files:   tree(),
			wd:      "work",
			args:    []string{"link.txt"},
			wantOut: "work/a.txt\n",
		},
		{
			name:    "resolves a symbolic link to a directory",
			files:   tree(),
			wd:      "work",
			args:    []string{"link-d/b.txt"},
			wantOut: "work/x/b.txt\n",
		},
		{
			name:    "the parent of a symbolic link is the link target",
			files:   tree(),
			wd:      "work",
			args:    []string{"link-d/.."},
			wantOut: "work\n",
		},
		{
			name:    "a dangling symbolic link resolves to its missing target",
			files:   tree(),
			wd:      "work",
			args:    []string{"dangling"},
			wantOut: "work/missing.txt\n",
		},
		{
			name:     "a symbolic link loop is an error",
			files:    tree(),
			wd:       "work",
			args:     []string{"loop"},
			wantErr:  "realpath: loop: too many levels of symbolic links\n",
			wantExit: 1,
		},
		{
			name:     "a symbolic link cycle is an error",
			files:    tree(),
			wd:       "work",
			args:     []string{"ping", "pong"},
			wantErr:  "realpath: ping: too many levels of symbolic links\nrealpath: pong: too many levels of symbolic links\n",
			wantExit: 1,
		},
		{
			name:  "an absolute operand is used as given",
			files: tree(),
			args:  []string{"work/a.txt"},
			wd:    "work",
			// The memory file system has no names with a leading slash,
			// so the components of the absolute name don't resolve.
			wantErr:  "realpath: work/a.txt: file does not exist\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunExisting(t *testing.T) {
	tests := []testCase{
		{
			name:    "an existing file resolves",
			files:   tree(),
			wd:      "work",
			args:    []string{"-e", "a.txt"},
			wantOut: "work/a.txt\n",
		},
		{
			name:     "a missing last component is an error",
			files:    tree(),
			wd:       "work",
			args:     []string{"-e", "missing.txt"},
			wantErr:  "realpath: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing component before the last is an error",
			files:    tree(),
			wd:       "work",
			args:     []string{"-e", "missing/missing.txt"},
			wantErr:  "realpath: missing/missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a dangling symbolic link is an error",
			files:    tree(),
			wd:       "work",
			args:     []string{"-e", "dangling"},
			wantErr:  "realpath: dangling: file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "a trailing slash on a directory resolves",
			files:   tree(),
			wd:      "work",
			args:    []string{"-e", "d/"},
			wantOut: "work/d\n",
		},
		{
			name:     "a trailing slash requires the name to be a directory",
			files:    tree(),
			wd:       "work",
			args:     []string{"-e", "a.txt/"},
			wantErr:  "realpath: a.txt/: not a directory\n",
			wantExit: 1,
		},
		{
			name:     "a missing directory with a trailing slash is an error",
			files:    tree(),
			wd:       "work",
			args:     []string{"-e", "missing/"},
			wantErr:  "realpath: missing/: file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "resolves through a symbolic link",
			files:   tree(),
			wd:      "work",
			args:    []string{"-e", "link.txt"},
			wantOut: "work/a.txt\n",
		},
		{
			name:    "an existing symbolic link with -m like checks still resolves",
			files:   tree(),
			wd:      "work",
			args:    []string{"-e", "link-d/b.txt"},
			wantOut: "work/x/b.txt\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunMissing(t *testing.T) {
	tests := []testCase{
		{
			name:    "missing components are treated as directories",
			files:   tree(),
			wd:      "work",
			args:    []string{"-m", "none/missing.txt"},
			wantOut: "work/none/missing.txt\n",
		},
		{
			name:    "the target of a dangling link is treated as a directory",
			files:   tree(),
			wd:      "work",
			args:    []string{"-m", "dangling/keep"},
			wantOut: "work/missing.txt/keep\n",
		},
		{
			name:    "missing components apply to the parent operations",
			files:   tree(),
			wd:      "work",
			args:    []string{"-m", "none/missing/../keep"},
			wantOut: "work/none/keep\n",
		},
		{
			name:     "an existing file in the middle is still an error",
			files:    tree(),
			wd:       "work",
			args:     []string{"-m", "a.txt/x"},
			wantErr:  "realpath: a.txt/x: not a directory\n",
			wantExit: 1,
		},
		{
			name:    "an existing file as the last component resolves",
			files:   tree(),
			wd:      "work",
			args:    []string{"-m", "a.txt"},
			wantOut: "work/a.txt\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunStrip(t *testing.T) {
	tests := []testCase{
		{
			name:    "a symbolic link is not resolved",
			files:   tree(),
			wd:      "work",
			args:    []string{"-s", "link.txt"},
			wantOut: "work/link.txt\n",
		},
		{
			name:    "a dangling symbolic link is left as is",
			files:   tree(),
			wd:      "work",
			args:    []string{"-s", "dangling"},
			wantOut: "work/dangling\n",
		},
		{
			name:     "the components of a symbolic link are not followed",
			files:    tree(),
			wd:       "work",
			args:     []string{"-s", "link.txt/x"},
			wantErr:  "realpath: link.txt/x: not a directory\n",
			wantExit: 1,
		},
		{
			name:    "a missing last component is not an error",
			files:   tree(),
			wd:      "work",
			args:    []string{"-s", "missing.txt"},
			wantOut: "work/missing.txt\n",
		},
		{
			name:     "a missing component before the last is an error",
			files:    tree(),
			wd:       "work",
			args:     []string{"-s", "missing/x"},
			wantErr:  "realpath: missing/x: file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "resolves only the dot components and extra slashes",
			files:   tree(),
			wd:      "work",
			args:    []string{"-s", "d/./../a.txt"},
			wantOut: "work/a.txt\n",
		},
		{
			name:  "with -m no existing file is touched at all",
			files: fsx.MemFS{"work/none": {Data: []byte("a.txt"), Mode: fs.ModeSymlink}},
			wd:    "work",
			args:  []string{"-sm", "none/missing/../x"},
			// The link would resolve to a.txt, which is not a
			// directory; only the name is processed.
			wantOut: "work/none/x\n",
		},
		{
			name:    "with -m an existing file is not inspected at all",
			files:   tree(),
			wd:      "work",
			args:    []string{"-sm", "a.txt/x"},
			wantOut: "work/a.txt/x\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLogical(t *testing.T) {
	tests := []testCase{
		{
			name: "the parent applies to the logical name",
			files: fsx.MemFS{
				"work/x/real.txt": {Data: []byte("x")},
				"work/link-d":     {Data: []byte("x"), Mode: fs.ModeSymlink},
			},
			wd:      "work",
			args:    []string{"-L", "link-d/missing/.."},
			wantOut: "work/x\n",
		},
		{
			name: "the physical mode still requires the component to exist",
			files: fsx.MemFS{
				"work/x/real.txt": {Data: []byte("x")},
				"work/link-d":     {Data: []byte("x"), Mode: fs.ModeSymlink},
			},
			wd:       "work",
			args:     []string{"-P", "link-d/missing/.."},
			wantErr:  "realpath: link-d/missing/..: file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "the parent applies to the resolved name of the links",
			files:   tree(),
			wd:      "work",
			args:    []string{"-L", "link-d/b.txt/../b.txt"},
			wantOut: "work/x/b.txt\n",
		},
		{
			name:    "dot components are removed before the links are resolved",
			files:   tree(),
			wd:      "work",
			args:    []string{"-L", "./link.txt"},
			wantOut: "work/a.txt\n",
		},
		{
			name:    "the links themselves are still resolved",
			files:   tree(),
			wd:      "work",
			args:    []string{"-L", "link.txt"},
			wantOut: "work/a.txt\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunQuiet(t *testing.T) {
	tests := []testCase{
		{
			name:     "suppresses the diagnostic of a failing operand",
			files:    tree(),
			wd:       "work",
			args:     []string{"-q", "missing/x"},
			wantExit: 1,
		},
		{
			name:     "the other operands are still resolved",
			files:    tree(),
			wd:       "work",
			args:     []string{"-q", "missing/x", "a.txt"},
			wantOut:  "work/a.txt\n",
			wantExit: 1,
		},
		{
			name:    "does not change the output of successful operands",
			files:   tree(),
			wd:      "work",
			args:    []string{"-q", "a.txt"},
			wantOut: "work/a.txt\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDelimiters(t *testing.T) {
	tests := []testCase{
		{
			name:    "-z ends each line with NUL",
			files:   tree(),
			wd:      "work",
			args:    []string{"-z", "a.txt", "d"},
			wantOut: "work/a.txt\x00work/d\x00",
		},
		{
			name:    "-z keeps working with -q",
			files:   tree(),
			wd:      "work",
			args:    []string{"-qz", "a.txt"},
			wantOut: "work/a.txt\x00",
		},
		{
			name:     "-n is not an option of realpath",
			files:    tree(),
			wd:       "work",
			args:     []string{"-n", "a.txt"},
			wantErr:  "realpath: invalid option -- 'n'\nTry 'realpath --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "--no-newline is not an option of realpath",
			files:    tree(),
			wd:       "work",
			args:     []string{"--no-newline", "a.txt"},
			wantErr:  "realpath: unrecognized option '--no-newline'\nTry 'realpath --help' for more information.\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRelativeTo(t *testing.T) {
	tests := []testCase{
		{
			name:    "the directory itself prints as a dot",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-to=usr/bin", "usr/bin"},
			wantOut: ".\n",
		},
		{
			name:    "a file outside the directory climbs with the parent components",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-to=usr/bin", "tmp/foo"},
			wantOut: "../../tmp/foo\n",
		},
		{
			name:    "a file below the directory is relative to it",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-to=usr/bin", "usr/share/dict/american-english"},
			wantOut: "../share/dict/american-english\n",
		},
		{
			name:    "the directory may be relative to the working directory",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-to=d", "a.txt"},
			wantOut: "../a.txt\n",
		},
		{
			name:    "the directory follows the symbolic links of the operands",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-to=link-d", "x/b.txt"},
			wantOut: "b.txt\n",
		},
		{
			name:    "a missing directory is fine by default",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-to=none", "a.txt"},
			wantOut: "../a.txt\n",
		},
		{
			name:     "a missing directory is an error with -e",
			files:    tree(),
			wd:       "work",
			args:     []string{"-e", "--relative-to=none", "a.txt"},
			wantErr:  "realpath: none: file does not exist\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRelativeBase(t *testing.T) {
	tests := []testCase{
		{
			name:    "a file below the base is relative to it",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-base=usr", "usr/bin"},
			wantOut: "bin\n",
		},
		{
			name:    "the base itself is relative",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-base=usr", "usr"},
			wantOut: ".\n",
		},
		{
			name:    "a file outside the base is absolute",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-base=usr", "tmp/foo"},
			wantOut: "work/tmp/foo\n",
		},
		{
			name:    "a file below the base is relative to the base",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-base=usr", "usr/share/dict/american-english"},
			wantOut: "share/dict/american-english\n",
		},
		{
			name:    "both options print relative to --relative-to when below --relative-base",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-to=usr/bin", "--relative-base=usr", "usr/bin", "tmp/foo", "usr/share/dict/american-english"},
			wantOut: ".\nwork/tmp/foo\n../share/dict/american-english\n",
		},
		{
			name:    "a --relative-to outside the base makes the names absolute",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-to=tmp", "--relative-base=usr", "usr/bin", "tmp/foo"},
			wantOut: "work/usr/bin\nwork/tmp/foo\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunUsageErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "missing operands",
			files:    tree(),
			wd:       "work",
			wantErr:  "realpath: missing file operand\nTry 'realpath --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid short option",
			files:    tree(),
			wd:       "work",
			args:     []string{"-X", "a.txt"},
			wantErr:  "realpath: invalid option -- 'X'\nTry 'realpath --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an unknown long option",
			files:    tree(),
			wd:       "work",
			args:     []string{"--bogus", "a.txt"},
			wantErr:  "realpath: unrecognized option '--bogus'\nTry 'realpath --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a long option without its argument",
			files:    tree(),
			wd:       "work",
			args:     []string{"--relative-to"},
			wantErr:  "realpath: option '--relative-to' requires an argument\nTry 'realpath --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an option argument held by the next argument",
			files:    tree(),
			wd:       "work",
			args:     []string{"--relative-to", "usr", "a.txt"},
			wantOut:  "../a.txt\n",
			wantExit: 0,
		},
		{
			name:     "an empty operand",
			files:    tree(),
			wd:       "work",
			args:     []string{""},
			wantErr:  "realpath: invalid zero-length file name\nTry 'realpath --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:    "the options end at the first operand",
			files:   tree(),
			wd:      "work",
			args:    []string{"--", "-e"},
			wantOut: "work/-e\n",
		},
		{
			name:    "the options end at --",
			files:   tree(),
			wd:      "work",
			args:    []string{"-s", "--", "link.txt"},
			wantOut: "work/link.txt\n",
		},
		{
			name:     "options after the operands are operands themselves",
			files:    tree(),
			wd:       "work",
			args:     []string{"a.txt", "-z"},
			wantOut:  "work/a.txt\nwork/-z\n",
			wantExit: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestCommonOptions(t *testing.T) {
	t.Run("--help prints the usage", func(t *testing.T) {
		runCase(t, testCase{
			files:   tree(),
			wd:      "work",
			args:    []string{"--help"},
			wantOut: usage,
		})
	})
	t.Run("--version prints the program name", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer), FS: tree()}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if out := stdout.String(); !strings.Contains(out, "realpath (charm.land/snail/coreutils)") {
			t.Errorf("Run() stdout = %q, want the version line", out)
		}
	})
	t.Run("--help after an operand is an operand", func(t *testing.T) {
		runCase(t, testCase{
			files:    tree(),
			wd:       "work",
			args:     []string{"a.txt", "--help"},
			wantOut:  "work/a.txt\nwork/--help\n",
			wantExit: 0,
		})
	})
}

func TestRunInteractions(t *testing.T) {
	tests := []testCase{
		{
			name:     "a missing component with a parent after it is an error",
			files:    tree(),
			wd:       "work",
			args:     []string{"d/missing/../keep.txt"},
			wantErr:  "realpath: d/missing/../keep.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "the logical mode applies the parent to the missing component",
			files:   tree(),
			wd:      "work",
			args:    []string{"-Le", "link-d/missing/.."},
			wantOut: "work/x\n",
		},
		{
			name:    "the missing mode leaves a looping link as it is",
			files:   tree(),
			wd:      "work",
			args:    []string{"-m", "loop"},
			wantOut: "work/loop\n",
		},
		{
			name:     "a stripped name with a trailing slash is still a directory",
			files:    tree(),
			wd:       "work",
			args:     []string{"-s", "a.txt/"},
			wantErr:  "realpath: a.txt/: not a directory\n",
			wantExit: 1,
		},
		{
			name:    "a stripped directory with a trailing slash resolves",
			files:   tree(),
			wd:      "work",
			args:    []string{"-s", "d/"},
			wantOut: "work/d\n",
		},
		{
			name:     "-e requires the --relative-to directory to exist as a directory",
			files:    tree(),
			wd:       "work",
			args:     []string{"-e", "--relative-to=a.txt", "a.txt"},
			wantErr:  "realpath: a.txt: not a directory\n",
			wantExit: 1,
		},
		{
			name:     "-e requires the --relative-base directory to be a directory",
			files:    tree(),
			wd:       "work",
			args:     []string{"-e", "--relative-base=d/keep.txt", "a.txt"},
			wantErr:  "realpath: d/keep.txt: not a directory\n",
			wantExit: 1,
		},
		{
			name:    "the last --relative-to wins",
			files:   tree(),
			wd:      "work",
			args:    []string{"--relative-to=usr/share", "--relative-to=d", "a.txt"},
			wantOut: "../a.txt\n",
		},
		{
			name:    "-z ends the relative names with NUL",
			files:   tree(),
			wd:      "work",
			args:    []string{"-z", "--relative-to=d", "a.txt"},
			wantOut: "../a.txt\x00",
		},
		{
			name:    "the last of -L, -P and -s wins: sP",
			files:   tree(),
			wd:      "work",
			args:    []string{"-sP", "link.txt"},
			wantOut: "work/a.txt\n",
		},
		{
			name:    "the last of -L, -P and -s wins: Ps",
			files:   tree(),
			wd:      "work",
			args:    []string{"-Ps", "link.txt"},
			wantOut: "work/link.txt\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestPathPrefix(t *testing.T) {
	tests := []struct {
		dir, name string
		want      bool
	}{
		{"/", "/", true},
		{"/", "/a", true},
		{"/", "/a/b", true},
		{"/", "a/b", true},
		{"/a", "/a", true},
		{"/a", "/a/b", true},
		{"/a", "/ab", false},
		{"/a", "/b", false},
		{"work", "work", true},
		{"work", "work/a", true},
		{"work", "other", false},
	}
	for _, test := range tests {
		if got := pathPrefix(test.dir, test.name); got != test.want {
			t.Errorf("pathPrefix(%q, %q) = %v, want %v", test.dir, test.name, got, test.want)
		}
	}
}

func TestRunRootBase(t *testing.T) {
	t.Run("a root base keeps the names relative to the file system root", func(t *testing.T) {
		tests := []testCase{
			{
				name:    "--relative-to=/",
				files:   tree(),
				wd:      "work",
				args:    []string{"--relative-to=/", "d/keep.txt"},
				wantOut: "work/d/keep.txt\n",
			},
			{
				name:    "--relative-base=/",
				files:   tree(),
				wd:      "work",
				args:    []string{"--relative-base=/", "d/keep.txt"},
				wantOut: "work/d/keep.txt\n",
			},
			{
				name:    "both options with a root base",
				files:   tree(),
				wd:      "work",
				args:    []string{"--relative-to=/", "--relative-base=/", "d/keep.txt"},
				wantOut: "work/d/keep.txt\n",
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("a root base makes absolute names relative", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the test needs names rooted at /")
		}
		// The temporary directory may sit behind a symbolic link, as
		// /tmp and /var do on macOS; resolve it, so the canonicalized
		// operands keep the same prefix.
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatalf("EvalSymlinks() error = %v", err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "usr", "bin"), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		operand := filepath.ToSlash(filepath.Join(dir, "usr", "bin"))

		for _, test := range []struct {
			name string
			args []string
		}{
			{"relative-to", []string{"--relative-to=/", operand}},
			{"relative-base", []string{"--relative-base=/", operand}},
			{"both options", []string{"--relative-to=/", "--relative-base=/", operand}},
		} {
			t.Run(test.name, func(t *testing.T) {
				stdout := new(bytes.Buffer)
				stderr := new(bytes.Buffer)
				rt := &coreutils.Runtime{
					Stdout:     stdout,
					Stderr:     stderr,
					FS:         fsx.OsFS(""),
					WorkingDir: dir,
				}
				if err := New(rt).Run(t.Context(), test.args...); err != nil {
					t.Fatalf("Run() error = %v", err)
				}
				// Relative to the root, the name loses its leading
				// slash.
				want := strings.TrimPrefix(operand, "/") + "\n"
				if got := stdout.String(); got != want {
					t.Errorf("Run() stdout = %q, want %q", got, want)
				}
			})
		}
	})
}

// optionToken matches a usage option token: a short form alone, a combined
// short and long form, as in "-a, --all", or a long-only form, each with an
// optional "=ARGUMENT" suffix.
var optionToken = regexp.MustCompile(`^(-[a-zA-Z], )?--[a-z-]+(=[A-Z]+)?$|^-[a-zA-Z]$`)

// TestUsageLayout checks that the option table keeps the hanging layout of
// the GNU usage texts: an option token alone on its line at 2 or 6 spaces,
// with its description below at 9 spaces, all wrapped at 80 columns.
func TestUsageLayout(t *testing.T) {
	for i, line := range strings.Split(usage, "\n") {
		if len(line) > 80 {
			t.Errorf("usage line %d (%q) is %d columns long, want at most 80",
				i+1, line, len(line))
		}
		if line == "" || !strings.HasPrefix(line, " ") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		token := strings.TrimLeft(line, " ")
		switch {
		case (indent == 2 || indent == 6) && strings.HasPrefix(token, "-"):
			if !optionToken.MatchString(token) {
				t.Errorf("usage line %d (%q) is not a lone option token", i+1, line)
			}
		case indent == 9:
			// A description line; an option token may not hide here.
			if optionToken.MatchString(token) {
				t.Errorf("usage line %d (%q) looks like an option token, want a description", i+1, line)
			}
		default:
			t.Errorf("usage line %d (%q) is neither an option token at 2 or 6 spaces nor a description at 9", i+1, line)
		}
	}
}
