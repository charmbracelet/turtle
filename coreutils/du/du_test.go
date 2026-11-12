package du

import (
	"io/fs"
	"testing"

	"charm.land/snail/coreutils/internal/testutil"
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

func TestRunOperands(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "no operand defaults to the working directory",
			FS: fsx.MemFS{
				"a": {Data: make([]byte, 600)},
				"b": {Data: make([]byte, 100)},
			},
			Want: "3 .\n",
		},
		{
			Name: "a directory operand writes its subdirectories and itself",
			FS:   tree,
			Args: []string{"tree"},
			Want: "4 tree/sub\n7 tree\n",
		},
		{
			Name: "operands are written in the order given",
			FS: fsx.MemFS{
				"t2/x": {Data: make([]byte, 100)},
				"t1/a": {Data: make([]byte, 600)},
			},
			Args: []string{"t1", "t2"},
			Want: "2 t1\n1 t2\n",
		},
		{
			Name: "a file operand is written alone",
			FS:   tree,
			Args: []string{"tree/a"},
			Want: "2 tree/a\n",
		},
		{
			Name: "the trailing slash of an operand is kept",
			FS:   tree,
			Args: []string{"tree/"},
			Want: "4 tree/sub\n7 tree/\n",
		},
		{
			Name: "operands resolve against the working directory",
			FS:   tree,
			Dir:  "tree/sub",
			Args: []string{"c"},
			Want: "4 c\n",
		},
		{
			Name:     "a missing operand is an error",
			FS:       tree,
			Args:     []string{"missing"},
			WantErr:  "du: cannot access 'missing': file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "an empty operand is an error",
			FS:       tree,
			Args:     []string{"", "tree"},
			Want:     "4 tree/sub\n7 tree\n",
			WantErr:  "du: cannot access '': file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "the run fails when no file system is available",
			Args:     []string{"tree"},
			WantErr:  "du: no file system available\n",
			WantExit: 1,
		},
	})
}

func TestRunAll(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "-a writes every file in the hierarchy",
			FS:   tree,
			Args: []string{"-a", "tree"},
			Want: "2 tree/a\n1 tree/b\n4 tree/sub/c\n4 tree/sub\n7 tree\n",
		},
		{
			Name: "-a writes a file operand like any other",
			FS:   tree,
			Args: []string{"-a", "tree/a"},
			Want: "2 tree/a\n",
		},
		{
			Name: "-a groups with other short options",
			FS:   tree,
			Args: []string{"-ak", "tree"},
			Want: "1 tree/a\n1 tree/b\n2 tree/sub/c\n2 tree/sub\n4 tree\n",
		},
	})
}

func TestRunSummarize(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "-s writes the operands alone",
			FS:   tree,
			Args: []string{"-s", "tree"},
			Want: "7 tree\n",
		},
		{
			Name: "-s writes each of the operands",
			FS:   joined,
			Args: []string{"-s", "tree", "other"},
			Want: "7 tree\n1 other\n",
		},
		{
			Name: "-s keeps a non-directory operand",
			FS:   tree,
			Args: []string{"-s", "tree/a"},
			Want: "2 tree/a\n",
		},
		{
			Name:     "-a and -s conflict",
			FS:       tree,
			Args:     []string{"-a", "-s", "tree"},
			WantErr:  "du: options --summarize and --all are mutually exclusive\nTry 'du --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "the conflict holds in either order",
			FS:       tree,
			Args:     []string{"-s", "-a", "tree"},
			WantErr:  "du: options --summarize and --all are mutually exclusive\nTry 'du --help' for more information.\n",
			WantExit: 1,
		},
	})
}

func TestRunKilo(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "-k counts in kibibyte blocks",
			FS:   tree,
			Args: []string{"-k", "tree"},
			Want: "2 tree/sub\n4 tree\n",
		},
		{
			Name: "-k rounds the blocks of every file up with -a",
			FS:   tree,
			Args: []string{"-k", "-a", "tree"},
			Want: "1 tree/a\n1 tree/b\n2 tree/sub/c\n2 tree/sub\n4 tree\n",
		},
	})
}

func TestRunFollow(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "a symbolic link counts as the link itself by default",
			FS:   linked,
			Args: []string{"dir"},
			Want: "4 dir\n",
		},
		{
			Name: "-L counts the file a link points at",
			FS:   linked,
			Args: []string{"-L", "dir"},
			Want: "2 dir/p\n14 dir\n",
		},
		{
			Name: "-L writes the file a link points at with -a",
			FS:   linked,
			Args: []string{"-L", "-a", "dir"},
			Want: "2 dir/a\n10 dir/l\n2 dir/p/a\n2 dir/p\n14 dir\n",
		},
		{
			Name: "-H keeps the links found below an operand alone",
			FS:   linked,
			Args: []string{"-H", "dir"},
			Want: "4 dir\n",
		},
		{
			Name: "an unmentioned link operand counts as the link itself",
			FS:   linked,
			Args: []string{"lnk"},
			Want: "1 lnk\n",
		},
		{
			Name: "-H counts the hierarchy an operand link points at",
			FS:   linked,
			Args: []string{"-H", "lnk"},
			Want: "2 lnk\n",
		},
		{
			Name: "-L counts an operand link the same way",
			FS:   linked,
			Args: []string{"-L", "lnk"},
			Want: "2 lnk\n",
		},
		{
			Name: "a broken link operand counts as itself with -H",
			FS:   fsx.MemFS{"lnk": link("missing")},
			Args: []string{"-H", "lnk"},
			Want: "1 lnk\n",
		},
		{
			Name: "a broken link operand counts as itself with -L",
			FS:   fsx.MemFS{"lnk": link("missing")},
			Args: []string{"-L", "lnk"},
			Want: "1 lnk\n",
		},
		{
			Name: "the last of -H and -L wins",
			FS:   linked,
			Args: []string{"-H", "-L", "dir"},
			Want: "2 dir/p\n14 dir\n",
		},
		{
			Name: "the last of -L and -H wins",
			FS:   linked,
			Args: []string{"-L", "-H", "dir"},
			Want: "4 dir\n",
		},
	})
}

func TestRunOneFileSystem(t *testing.T) {
	// The memory file system reports no devices, so -x can't skip anything.
	testutil.Once(t, New, testutil.Case{
		Name: "-x changes nothing without device information",
		FS:   tree,
		Args: []string{"-x", "tree"},
		Want: "4 tree/sub\n7 tree\n",
	})
}

func TestRunGnu(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "-h writes human readable sizes",
			FS:   tree,
			Args: []string{"-h", "tree"},
			Want: "2.0K tree/sub\n3.5K tree\n",
		},
		{
			Name: "-h sizes files with -a",
			FS:   tree,
			Args: []string{"-h", "-a", "tree"},
			Want: "1.0K tree/a\n512 tree/b\n2.0K tree/sub/c\n2.0K tree/sub\n3.5K tree\n",
		},
		{
			Name: "-S excludes the subdirectories of each directory",
			FS:   tree,
			Args: []string{"-S", "tree"},
			Want: "4 tree/sub\n3 tree\n",
		},
		{
			Name: "-S keeps the files held directly",
			FS:   fsx.MemFS{"d/x": {Data: make([]byte, 600)}, "d/s/y": {Data: make([]byte, 100)}},
			Args: []string{"-S", "d"},
			Want: "1 d/s\n2 d\n",
		},
		{
			Name: "the same hierarchy without -S",
			FS:   fsx.MemFS{"d/x": {Data: make([]byte, 600)}, "d/s/y": {Data: make([]byte, 100)}},
			Args: []string{"d"},
			Want: "1 d/s\n3 d\n",
		},
		{
			Name: "-b counts apparent sizes in bytes",
			FS:   tree,
			Args: []string{"-b", "tree"},
			Want: "2000 tree/sub\n2700 tree\n",
		},
		{
			Name: "-b counts the apparent size of a link",
			FS:   fsx.MemFS{"l": link("point")},
			Args: []string{"-b", "l"},
			Want: "5 l\n",
		},
		{
			Name: "-b and -k: the last one wins",
			FS:   tree,
			Args: []string{"-b", "-k", "tree"},
			Want: "2 tree/sub\n4 tree\n",
		},
		{
			Name: "-k and -b: the last one wins",
			FS:   tree,
			Args: []string{"-k", "-b", "tree"},
			Want: "2000 tree/sub\n2700 tree\n",
		},
		{
			Name: "-c writes a grand total",
			FS:   tree,
			Args: []string{"-c", "tree"},
			Want: "4 tree/sub\n7 tree\n7 total\n",
		},
		{
			Name: "-c totals the operands together",
			FS:   joined,
			Args: []string{"-c", "tree", "other"},
			Want: "4 tree/sub\n7 tree\n1 other\n8 total\n",
		},
		{
			Name: "-c totals a non-directory operand",
			FS:   tree,
			Args: []string{"-c", "tree/a"},
			Want: "2 tree/a\n2 total\n",
		},
		{
			Name:     "-c writes a total even after an error",
			FS:       tree,
			Args:     []string{"-c", "missing", "tree"},
			Want:     "4 tree/sub\n7 tree\n7 total\n",
			WantErr:  "du: cannot access 'missing': file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "-c works with -b",
			FS:   joined,
			Args: []string{"-c", "-b", "tree", "other"},
			Want: "2000 tree/sub\n2700 tree\n100 other\n2800 total\n",
		},
	})
}

func TestRunMaxDepth(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "a hierarchy with three levels of directories",
			FS:   deep,
			Args: []string{"tree"},
			Want: "1 tree/sub/s2\n5 tree/sub\n8 tree\n",
		},
		{
			Name: "-d 0 is the same as -s",
			FS:   deep,
			Args: []string{"-d", "0", "tree"},
			Want: "8 tree\n",
		},
		{
			Name: "--max-depth=0 works the same",
			FS:   deep,
			Args: []string{"--max-depth=0", "tree"},
			Want: "8 tree\n",
		},
		{
			Name: "-d 1 writes one level of directories",
			FS:   deep,
			Args: []string{"-d", "1", "tree"},
			Want: "5 tree/sub\n8 tree\n",
		},
		{
			Name: "-d 2 writes two levels",
			FS:   deep,
			Args: []string{"-d", "2", "tree"},
			Want: "1 tree/sub/s2\n5 tree/sub\n8 tree\n",
		},
		{
			Name: "-d hides the files too",
			FS:   deep,
			Args: []string{"-d", "0", "-a", "tree"},
			Want: "8 tree\n",
		},
		{
			Name: "-d 1 with -a writes the files of the first level",
			FS:   deep,
			Args: []string{"-d", "1", "-a", "tree"},
			Want: "2 tree/a\n1 tree/b\n5 tree/sub\n8 tree\n",
		},
		{
			Name: "-d 2 with -a writes the files of two levels",
			FS:   deep,
			Args: []string{"-d", "2", "-a", "tree"},
			Want: "2 tree/a\n1 tree/b\n4 tree/sub/c\n1 tree/sub/s2\n5 tree/sub\n8 tree\n",
		},
		{
			Name: "-d takes an attached argument",
			FS:   deep,
			Args: []string{"-d0", "tree"},
			Want: "8 tree\n",
		},
		{
			Name: "-d takes a separated argument",
			FS:   deep,
			Args: []string{"-d", "1", "tree"},
			Want: "5 tree/sub\n8 tree\n",
		},
		{
			Name: "--max-depth takes an attached argument",
			FS:   deep,
			Args: []string{"--max-depth=1", "tree"},
			Want: "5 tree/sub\n8 tree\n",
		},
		{
			Name: "--max-depth takes a separated argument",
			FS:   deep,
			Args: []string{"--max-depth", "1", "tree"},
			Want: "5 tree/sub\n8 tree\n",
		},
		{
			Name:     "-s and a depth conflict",
			FS:       deep,
			Args:     []string{"-s", "-d", "1", "tree"},
			WantErr:  "du: warning: summarizing conflicts with --max-depth=1\nTry 'du --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "the conflict holds in either order",
			FS:       deep,
			Args:     []string{"-d", "1", "-s", "tree"},
			WantErr:  "du: warning: summarizing conflicts with --max-depth=1\nTry 'du --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:    "-d 0 and -s warn but run",
			FS:      deep,
			Args:    []string{"-s", "-d", "0", "tree"},
			Want:    "8 tree\n",
			WantErr: "du: warning: summarizing is the same as using --max-depth=0\n",
		},
		{
			Name:     "an invalid maximum depth is a usage error",
			FS:       deep,
			Args:     []string{"-d", "x", "tree"},
			WantErr:  "du: invalid maximum depth 'x'\nTry 'du --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a negative maximum depth is invalid",
			FS:       deep,
			Args:     []string{"-d", "-1", "tree"},
			WantErr:  "du: invalid maximum depth '-1'\nTry 'du --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid maximum depth long option is a usage error",
			FS:       deep,
			Args:     []string{"--max-depth=2x", "tree"},
			WantErr:  "du: invalid maximum depth '2x'\nTry 'du --help' for more information.\n",
			WantExit: 1,
		},
	})
}

func TestRunUsage(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name:     "-z is not an option of du",
			Args:     []string{"-z"},
			WantErr:  "du: invalid option -- 'z'\nTry 'du --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "--bogus is not an option of du",
			Args:     []string{"--bogus"},
			WantErr:  "du: unrecognized option '--bogus'\nTry 'du --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "--all doesn't take an argument",
			Args:     []string{"--all=yes"},
			WantErr:  "du: option '--all=yes' doesn't allow an argument\nTry 'du --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "-d requires an argument",
			Args:     []string{"-d"},
			WantErr:  "du: option requires an argument -- 'd'\nTry 'du --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "--max-depth requires an argument",
			Args:     []string{"--max-depth"},
			WantErr:  "du: option '--max-depth' requires an argument\nTry 'du --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name: "-- ends the options",
			FS:   tree,
			Args: []string{"--", "tree"},
			Want: "4 tree/sub\n7 tree\n",
		},
		{
			Name:     "options stop at the first operand",
			FS:       tree,
			Args:     []string{"-k", "tree", "-a"},
			Want:     "2 tree/sub\n4 tree\n",
			WantErr:  "du: cannot access '-a': file does not exist\n",
			WantExit: 1,
		},
	})
}

func TestRunUnreadable(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name:     "an unreadable operand is an error",
			FS:       failFS{MemFS: tree, fail: "tree"},
			Args:     []string{"tree"},
			Want:     "0 tree\n",
			WantErr:  "du: cannot read directory 'tree': permission denied\n",
			WantExit: 1,
		},
		{
			Name:     "an unreadable subdirectory keeps the rest counted",
			FS:       failFS{MemFS: tree, fail: "tree/sub"},
			Args:     []string{"tree"},
			Want:     "0 tree/sub\n3 tree\n",
			WantErr:  "du: cannot read directory 'tree/sub': permission denied\n",
			WantExit: 1,
		},
	})
}

func TestRunContext(t *testing.T) {
	testutil.Cancelled(t, New, nil, tree, "tree")
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "du", usage)
}
