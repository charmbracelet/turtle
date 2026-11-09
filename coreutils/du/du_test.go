package du

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// tree is a hierarchy of a two-block file, a one-block one, and a
// subdirectory holding a four-block one:
//
//	tree
//	├── a (600 bytes, 2 blocks)
//	├── b (100 bytes, 1 block)
//	└── sub
//	    └── c (2000 bytes, 4 blocks)
//
// The sizes come from the file sizes rounded up to whole 512-byte blocks,
// as the memory file system reports no blocks.
var tree = fsx.MemFS{
	"tree/a":     {Data: make([]byte, 600)},
	"tree/b":     {Data: make([]byte, 100)},
	"tree/sub/c": {Data: make([]byte, 2000)},
}

// joined is the tree and a second hierarchy together, for the tests of
// multiple operands.
var joined = fsx.MemFS{
	"tree/a":     {Data: make([]byte, 600)},
	"tree/b":     {Data: make([]byte, 100)},
	"tree/sub/c": {Data: make([]byte, 2000)},
	"other/x":    {Data: make([]byte, 100)},
}

// deep is the tree above with one more level of directories.
var deep = fsx.MemFS{
	"tree/a":        {Data: make([]byte, 600)},
	"tree/b":        {Data: make([]byte, 100)},
	"tree/sub/c":    {Data: make([]byte, 2000)},
	"tree/sub/s2/x": {Data: make([]byte, 100)},
}

// linked holds a symbolic link to a file inside a directory, and one to a
// directory elsewhere.
var linked = fsx.MemFS{
	"f":       {Data: make([]byte, 5000)}, // 10 blocks
	"dir/a":   {Data: make([]byte, 600)},  // 2 blocks
	"dir/l":   link("../f"),               // 1 block on its own
	"point/a": {Data: make([]byte, 600)},
	"dir/p":   link("../point"),
	"lnk":     link("point"),
}

// link returns a symbolic entry pointing at target.
func link(target string) *fsx.MemFile {
	return &fsx.MemFile{Data: []byte(target), Mode: fs.ModeSymlink | 0o777}
}

// failFS is a memory file system whose named directory can't be read.
type failFS struct {
	fsx.MemFS
	fail string
}

// ReadDir fails for the named directory, like an unreadable one does.
func (f failFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == f.fail {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrPermission}
	}
	return f.MemFS.ReadDir(name)
}

type testCase struct {
	name     string
	files    fsx.MemFS
	sys      fsx.FS
	wd       string
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

func TestRunOperands(t *testing.T) {
	runCases(t, []testCase{
		{
			name: "no operand defaults to the working directory",
			files: fsx.MemFS{
				"a": {Data: make([]byte, 600)},
				"b": {Data: make([]byte, 100)},
			},
			wantOut: "3 .\n",
		},
		{
			name:    "a directory operand writes its subdirectories and itself",
			files:   tree,
			args:    []string{"tree"},
			wantOut: "4 tree/sub\n7 tree\n",
		},
		{
			name: "operands are written in the order given",
			files: fsx.MemFS{
				"t2/x": {Data: make([]byte, 100)},
				"t1/a": {Data: make([]byte, 600)},
			},
			args:    []string{"t1", "t2"},
			wantOut: "2 t1\n1 t2\n",
		},
		{
			name:    "a file operand is written alone",
			files:   tree,
			args:    []string{"tree/a"},
			wantOut: "2 tree/a\n",
		},
		{
			name:    "the trailing slash of an operand is kept",
			files:   tree,
			args:    []string{"tree/"},
			wantOut: "4 tree/sub\n7 tree/\n",
		},
		{
			name:    "operands resolve against the working directory",
			files:   tree,
			wd:      "tree/sub",
			args:    []string{"c"},
			wantOut: "4 c\n",
		},
		{
			name:     "a missing operand is an error",
			files:    tree,
			args:     []string{"missing"},
			wantErr:  "du: cannot access 'missing': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "an empty operand is an error",
			files:    tree,
			args:     []string{"", "tree"},
			wantOut:  "4 tree/sub\n7 tree\n",
			wantErr:  "du: cannot access '': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "the run fails when no file system is available",
			args:     []string{"tree"},
			wantErr:  "du: no file system available\n",
			wantExit: 1,
		},
	})
}

func TestRunAll(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "-a writes every file in the hierarchy",
			files:   tree,
			args:    []string{"-a", "tree"},
			wantOut: "2 tree/a\n1 tree/b\n4 tree/sub/c\n4 tree/sub\n7 tree\n",
		},
		{
			name:    "-a writes a file operand like any other",
			files:   tree,
			args:    []string{"-a", "tree/a"},
			wantOut: "2 tree/a\n",
		},
		{
			name:    "-a groups with other short options",
			files:   tree,
			args:    []string{"-ak", "tree"},
			wantOut: "1 tree/a\n1 tree/b\n2 tree/sub/c\n2 tree/sub\n4 tree\n",
		},
	})
}

func TestRunSummarize(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "-s writes the operands alone",
			files:   tree,
			args:    []string{"-s", "tree"},
			wantOut: "7 tree\n",
		},
		{
			name:    "-s writes each of the operands",
			files:   joined,
			args:    []string{"-s", "tree", "other"},
			wantOut: "7 tree\n1 other\n",
		},
		{
			name:    "-s keeps a non-directory operand",
			files:   tree,
			args:    []string{"-s", "tree/a"},
			wantOut: "2 tree/a\n",
		},
		{
			name:     "-a and -s conflict",
			files:    tree,
			args:     []string{"-a", "-s", "tree"},
			wantErr:  "du: options --summarize and --all are mutually exclusive\nTry 'du --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "the conflict holds in either order",
			files:    tree,
			args:     []string{"-s", "-a", "tree"},
			wantErr:  "du: options --summarize and --all are mutually exclusive\nTry 'du --help' for more information.\n",
			wantExit: 1,
		},
	})
}

func TestRunKilo(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "-k counts in kibibyte blocks",
			files:   tree,
			args:    []string{"-k", "tree"},
			wantOut: "2 tree/sub\n4 tree\n",
		},
		{
			name:    "-k rounds the blocks of every file up with -a",
			files:   tree,
			args:    []string{"-k", "-a", "tree"},
			wantOut: "1 tree/a\n1 tree/b\n2 tree/sub/c\n2 tree/sub\n4 tree\n",
		},
	})
}

func TestRunFollow(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "a symbolic link counts as the link itself by default",
			files:   linked,
			args:    []string{"dir"},
			wantOut: "4 dir\n",
		},
		{
			name:    "-L counts the file a link points at",
			files:   linked,
			args:    []string{"-L", "dir"},
			wantOut: "2 dir/p\n14 dir\n",
		},
		{
			name:    "-L writes the file a link points at with -a",
			files:   linked,
			args:    []string{"-L", "-a", "dir"},
			wantOut: "2 dir/a\n10 dir/l\n2 dir/p/a\n2 dir/p\n14 dir\n",
		},
		{
			name:    "-H keeps the links found below an operand alone",
			files:   linked,
			args:    []string{"-H", "dir"},
			wantOut: "4 dir\n",
		},
		{
			name:    "an unmentioned link operand counts as the link itself",
			files:   linked,
			args:    []string{"lnk"},
			wantOut: "1 lnk\n",
		},
		{
			name:    "-H counts the hierarchy an operand link points at",
			files:   linked,
			args:    []string{"-H", "lnk"},
			wantOut: "2 lnk\n",
		},
		{
			name:    "-L counts an operand link the same way",
			files:   linked,
			args:    []string{"-L", "lnk"},
			wantOut: "2 lnk\n",
		},
		{
			name:    "a broken link operand counts as itself with -H",
			files:   fsx.MemFS{"lnk": link("missing")},
			args:    []string{"-H", "lnk"},
			wantOut: "1 lnk\n",
		},
		{
			name:    "a broken link operand counts as itself with -L",
			files:   fsx.MemFS{"lnk": link("missing")},
			args:    []string{"-L", "lnk"},
			wantOut: "1 lnk\n",
		},
		{
			name:    "the last of -H and -L wins",
			files:   linked,
			args:    []string{"-H", "-L", "dir"},
			wantOut: "2 dir/p\n14 dir\n",
		},
		{
			name:    "the last of -L and -H wins",
			files:   linked,
			args:    []string{"-L", "-H", "dir"},
			wantOut: "4 dir\n",
		},
	})
}

func TestRunOneFileSystem(t *testing.T) {
	// The memory file system reports no devices, so -x can't skip anything.
	runCase(t, testCase{
		name:    "-x changes nothing without device information",
		files:   tree,
		args:    []string{"-x", "tree"},
		wantOut: "4 tree/sub\n7 tree\n",
	})
}

func TestRunGnu(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "-h writes human readable sizes",
			files:   tree,
			args:    []string{"-h", "tree"},
			wantOut: "2.0K tree/sub\n3.5K tree\n",
		},
		{
			name:    "-h sizes files with -a",
			files:   tree,
			args:    []string{"-h", "-a", "tree"},
			wantOut: "1.0K tree/a\n512 tree/b\n2.0K tree/sub/c\n2.0K tree/sub\n3.5K tree\n",
		},
		{
			name:    "-S excludes the subdirectories of each directory",
			files:   tree,
			args:    []string{"-S", "tree"},
			wantOut: "4 tree/sub\n3 tree\n",
		},
		{
			name:    "-S keeps the files held directly",
			files:   fsx.MemFS{"d/x": {Data: make([]byte, 600)}, "d/s/y": {Data: make([]byte, 100)}},
			args:    []string{"-S", "d"},
			wantOut: "1 d/s\n2 d\n",
		},
		{
			name:    "the same hierarchy without -S",
			files:   fsx.MemFS{"d/x": {Data: make([]byte, 600)}, "d/s/y": {Data: make([]byte, 100)}},
			args:    []string{"d"},
			wantOut: "1 d/s\n3 d\n",
		},
		{
			name:    "-b counts apparent sizes in bytes",
			files:   tree,
			args:    []string{"-b", "tree"},
			wantOut: "2000 tree/sub\n2700 tree\n",
		},
		{
			name:    "-b counts the apparent size of a link",
			files:   fsx.MemFS{"l": link("point")},
			args:    []string{"-b", "l"},
			wantOut: "5 l\n",
		},
		{
			name:    "-b and -k: the last one wins",
			files:   tree,
			args:    []string{"-b", "-k", "tree"},
			wantOut: "2 tree/sub\n4 tree\n",
		},
		{
			name:    "-k and -b: the last one wins",
			files:   tree,
			args:    []string{"-k", "-b", "tree"},
			wantOut: "2000 tree/sub\n2700 tree\n",
		},
		{
			name:    "-c writes a grand total",
			files:   tree,
			args:    []string{"-c", "tree"},
			wantOut: "4 tree/sub\n7 tree\n7 total\n",
		},
		{
			name:    "-c totals the operands together",
			files:   joined,
			args:    []string{"-c", "tree", "other"},
			wantOut: "4 tree/sub\n7 tree\n1 other\n8 total\n",
		},
		{
			name:    "-c totals a non-directory operand",
			files:   tree,
			args:    []string{"-c", "tree/a"},
			wantOut: "2 tree/a\n2 total\n",
		},
		{
			name:     "-c writes a total even after an error",
			files:    tree,
			args:     []string{"-c", "missing", "tree"},
			wantOut:  "4 tree/sub\n7 tree\n7 total\n",
			wantErr:  "du: cannot access 'missing': file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "-c works with -b",
			files:   joined,
			args:    []string{"-c", "-b", "tree", "other"},
			wantOut: "2000 tree/sub\n2700 tree\n100 other\n2800 total\n",
		},
	})
}

func TestRunMaxDepth(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "a hierarchy with three levels of directories",
			files:   deep,
			args:    []string{"tree"},
			wantOut: "1 tree/sub/s2\n5 tree/sub\n8 tree\n",
		},
		{
			name:    "-d 0 is the same as -s",
			files:   deep,
			args:    []string{"-d", "0", "tree"},
			wantOut: "8 tree\n",
		},
		{
			name:    "--max-depth=0 works the same",
			files:   deep,
			args:    []string{"--max-depth=0", "tree"},
			wantOut: "8 tree\n",
		},
		{
			name:    "-d 1 writes one level of directories",
			files:   deep,
			args:    []string{"-d", "1", "tree"},
			wantOut: "5 tree/sub\n8 tree\n",
		},
		{
			name:    "-d 2 writes two levels",
			files:   deep,
			args:    []string{"-d", "2", "tree"},
			wantOut: "1 tree/sub/s2\n5 tree/sub\n8 tree\n",
		},
		{
			name:    "-d hides the files too",
			files:   deep,
			args:    []string{"-d", "0", "-a", "tree"},
			wantOut: "8 tree\n",
		},
		{
			name:    "-d 1 with -a writes the files of the first level",
			files:   deep,
			args:    []string{"-d", "1", "-a", "tree"},
			wantOut: "2 tree/a\n1 tree/b\n5 tree/sub\n8 tree\n",
		},
		{
			name:    "-d 2 with -a writes the files of two levels",
			files:   deep,
			args:    []string{"-d", "2", "-a", "tree"},
			wantOut: "2 tree/a\n1 tree/b\n4 tree/sub/c\n1 tree/sub/s2\n5 tree/sub\n8 tree\n",
		},
		{
			name:    "-d takes an attached argument",
			files:   deep,
			args:    []string{"-d0", "tree"},
			wantOut: "8 tree\n",
		},
		{
			name:    "-d takes a separated argument",
			files:   deep,
			args:    []string{"-d", "1", "tree"},
			wantOut: "5 tree/sub\n8 tree\n",
		},
		{
			name:    "--max-depth takes an attached argument",
			files:   deep,
			args:    []string{"--max-depth=1", "tree"},
			wantOut: "5 tree/sub\n8 tree\n",
		},
		{
			name:    "--max-depth takes a separated argument",
			files:   deep,
			args:    []string{"--max-depth", "1", "tree"},
			wantOut: "5 tree/sub\n8 tree\n",
		},
		{
			name:     "-s and a depth conflict",
			files:    deep,
			args:     []string{"-s", "-d", "1", "tree"},
			wantErr:  "du: warning: summarizing conflicts with --max-depth=1\nTry 'du --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "the conflict holds in either order",
			files:    deep,
			args:     []string{"-d", "1", "-s", "tree"},
			wantErr:  "du: warning: summarizing conflicts with --max-depth=1\nTry 'du --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:    "-d 0 and -s warn but run",
			files:   deep,
			args:    []string{"-s", "-d", "0", "tree"},
			wantOut: "8 tree\n",
			wantErr: "du: warning: summarizing is the same as using --max-depth=0\n",
		},
		{
			name:     "an invalid maximum depth is a usage error",
			files:    deep,
			args:     []string{"-d", "x", "tree"},
			wantErr:  "du: invalid maximum depth 'x'\nTry 'du --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a negative maximum depth is invalid",
			files:    deep,
			args:     []string{"-d", "-1", "tree"},
			wantErr:  "du: invalid maximum depth '-1'\nTry 'du --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid maximum depth long option is a usage error",
			files:    deep,
			args:     []string{"--max-depth=2x", "tree"},
			wantErr:  "du: invalid maximum depth '2x'\nTry 'du --help' for more information.\n",
			wantExit: 1,
		},
	})
}

func TestRunUsage(t *testing.T) {
	runCases(t, []testCase{
		{
			name:     "-z is not an option of du",
			args:     []string{"-z"},
			wantErr:  "du: invalid option -- 'z'\nTry 'du --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "--bogus is not an option of du",
			args:     []string{"--bogus"},
			wantErr:  "du: unrecognized option '--bogus'\nTry 'du --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "--all doesn't take an argument",
			args:     []string{"--all=yes"},
			wantErr:  "du: option '--all=yes' doesn't allow an argument\nTry 'du --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "-d requires an argument",
			args:     []string{"-d"},
			wantErr:  "du: option requires an argument -- 'd'\nTry 'du --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "--max-depth requires an argument",
			args:     []string{"--max-depth"},
			wantErr:  "du: option '--max-depth' requires an argument\nTry 'du --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:    "-- ends the options",
			files:   tree,
			args:    []string{"--", "tree"},
			wantOut: "4 tree/sub\n7 tree\n",
		},
		{
			name:     "options stop at the first operand",
			files:    tree,
			args:     []string{"-k", "tree", "-a"},
			wantOut:  "2 tree/sub\n4 tree\n",
			wantErr:  "du: cannot access '-a': file does not exist\n",
			wantExit: 1,
		},
	})
}

func TestRunUnreadable(t *testing.T) {
	runCases(t, []testCase{
		{
			name:     "an unreadable operand is an error",
			sys:      failFS{MemFS: tree, fail: "tree"},
			args:     []string{"tree"},
			wantOut:  "0 tree\n",
			wantErr:  "du: cannot read directory 'tree': permission denied\n",
			wantExit: 1,
		},
		{
			name:     "an unreadable subdirectory keeps the rest counted",
			sys:      failFS{MemFS: tree, fail: "tree/sub"},
			args:     []string{"tree"},
			wantOut:  "0 tree/sub\n3 tree\n",
			wantErr:  "du: cannot read directory 'tree/sub': permission denied\n",
			wantExit: 1,
		},
	})
}

func TestRunContext(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr, FS: tree}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := New(rt).Run(ctx, "tree")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "du (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
