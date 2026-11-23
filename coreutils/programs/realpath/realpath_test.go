package realpath

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// tree returns a working directory with files, directories and symbolic
// links the tests resolve.
func tree() fsx.MemoryFS {
	return fsx.MemoryFS{
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

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "resolves a file to the working directory",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"a.txt"},
			Want: "work/a.txt\n",
		},
		{
			Name: "resolves each operand on its own line",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"a.txt", "d/keep.txt"},
			Want: "work/a.txt\nwork/d/keep.txt\n",
		},
		{
			Name: "the working directory is a prefix of the output",
			FS:   tree(),
			Dir:  "work/d",
			Args: []string{"keep.txt"},
			Want: "work/d/keep.txt\n",
		},
		{
			Name: "resolves the working directory itself",
			FS:   tree(),
			Dir:  "work/d",
			Args: []string{"."},
			Want: "work/d\n",
		},
		{
			Name: "the parent of the working directory is the file system root",
			FS:   tree(),
			Dir:  "work",
			Args: []string{".."},
			Want: ".\n",
		},
		{
			Name: "removes dot and extra slash components",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"d/./keep.txt", "d//keep.txt"},
			Want: "work/d/keep.txt\nwork/d/keep.txt\n",
		},
		{
			Name: "resolves the parent of a directory",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"d/.."},
			Want: "work\n",
		},
		{
			Name: "a missing last component is not an error",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"missing.txt"},
			Want: "work/missing.txt\n",
		},
		{
			Name:     "a missing component before the last is an error",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"missing/missing.txt"},
			WantErr:  "realpath: missing/missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "keeps going after a failing operand",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"missing/x", "a.txt"},
			Want:     "work/a.txt\n",
			WantErr:  "realpath: missing/x: file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "-E is the default",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-E", "missing.txt"},
			Want: "work/missing.txt\n",
		},
		{
			Name: "ignores trailing slashes",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"d/", "missing.txt/"},
			Want: "work/d\nwork/missing.txt\n",
		},
		{
			Name: "resolves a symbolic link to a file",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"link.txt"},
			Want: "work/a.txt\n",
		},
		{
			Name: "resolves a symbolic link to a directory",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"link-d/b.txt"},
			Want: "work/x/b.txt\n",
		},
		{
			Name: "the parent of a symbolic link is the link target",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"link-d/.."},
			Want: "work\n",
		},
		{
			Name: "a dangling symbolic link resolves to its missing target",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"dangling"},
			Want: "work/missing.txt\n",
		},
		{
			Name:     "a symbolic link loop is an error",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"loop"},
			WantErr:  "realpath: loop: too many levels of symbolic links\n",
			WantExit: 1,
		},
		{
			Name:     "a symbolic link cycle is an error",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"ping", "pong"},
			WantErr:  "realpath: ping: too many levels of symbolic links\nrealpath: pong: too many levels of symbolic links\n",
			WantExit: 1,
		},
		{
			Name: "an absolute operand is used as given",
			FS:   tree(),
			Args: []string{"work/a.txt"},
			Dir:  "work",
			// The memory file system has no names with a leading slash,
			// so the components of the absolute name don't resolve.
			WantErr:  "realpath: work/a.txt: file does not exist\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunExisting(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "an existing file resolves",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-e", "a.txt"},
			Want: "work/a.txt\n",
		},
		{
			Name:     "a missing last component is an error",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-e", "missing.txt"},
			WantErr:  "realpath: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing component before the last is an error",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-e", "missing/missing.txt"},
			WantErr:  "realpath: missing/missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a dangling symbolic link is an error",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-e", "dangling"},
			WantErr:  "realpath: dangling: file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "a trailing slash on a directory resolves",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-e", "d/"},
			Want: "work/d\n",
		},
		{
			Name:     "a trailing slash requires the name to be a directory",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-e", "a.txt/"},
			WantErr:  "realpath: a.txt/: not a directory\n",
			WantExit: 1,
		},
		{
			Name:     "a missing directory with a trailing slash is an error",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-e", "missing/"},
			WantErr:  "realpath: missing/: file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "resolves through a symbolic link",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-e", "link.txt"},
			Want: "work/a.txt\n",
		},
		{
			Name: "an existing symbolic link with -m like checks still resolves",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-e", "link-d/b.txt"},
			Want: "work/x/b.txt\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunMissing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "missing components are treated as directories",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-m", "none/missing.txt"},
			Want: "work/none/missing.txt\n",
		},
		{
			Name: "the target of a dangling link is treated as a directory",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-m", "dangling/keep"},
			Want: "work/missing.txt/keep\n",
		},
		{
			Name: "missing components apply to the parent operations",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-m", "none/missing/../keep"},
			Want: "work/none/keep\n",
		},
		{
			Name:     "an existing file in the middle is still an error",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-m", "a.txt/x"},
			WantErr:  "realpath: a.txt/x: not a directory\n",
			WantExit: 1,
		},
		{
			Name: "an existing file as the last component resolves",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-m", "a.txt"},
			Want: "work/a.txt\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunStrip(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a symbolic link is not resolved",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-s", "link.txt"},
			Want: "work/link.txt\n",
		},
		{
			Name: "a dangling symbolic link is left as is",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-s", "dangling"},
			Want: "work/dangling\n",
		},
		{
			Name:     "the components of a symbolic link are not followed",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-s", "link.txt/x"},
			WantErr:  "realpath: link.txt/x: not a directory\n",
			WantExit: 1,
		},
		{
			Name: "a missing last component is not an error",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-s", "missing.txt"},
			Want: "work/missing.txt\n",
		},
		{
			Name:     "a missing component before the last is an error",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-s", "missing/x"},
			WantErr:  "realpath: missing/x: file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "resolves only the dot components and extra slashes",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-s", "d/./../a.txt"},
			Want: "work/a.txt\n",
		},
		{
			Name: "with -m no existing file is touched at all",
			FS:   fsx.MemoryFS{"work/none": {Data: []byte("a.txt"), Mode: fs.ModeSymlink}},
			Dir:  "work",
			Args: []string{"-sm", "none/missing/../x"},
			// The link would resolve to a.txt, which is not a
			// directory; only the name is processed.
			Want: "work/none/x\n",
		},
		{
			Name: "with -m an existing file is not inspected at all",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-sm", "a.txt/x"},
			Want: "work/a.txt/x\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLogical(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the parent applies to the logical name",
			FS: fsx.MemoryFS{
				"work/x/real.txt": {Data: []byte("x")},
				"work/link-d":     {Data: []byte("x"), Mode: fs.ModeSymlink},
			},
			Dir:  "work",
			Args: []string{"-L", "link-d/missing/.."},
			Want: "work/x\n",
		},
		{
			Name: "the physical mode still requires the component to exist",
			FS: fsx.MemoryFS{
				"work/x/real.txt": {Data: []byte("x")},
				"work/link-d":     {Data: []byte("x"), Mode: fs.ModeSymlink},
			},
			Dir:      "work",
			Args:     []string{"-P", "link-d/missing/.."},
			WantErr:  "realpath: link-d/missing/..: file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "the parent applies to the resolved name of the links",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-L", "link-d/b.txt/../b.txt"},
			Want: "work/x/b.txt\n",
		},
		{
			Name: "dot components are removed before the links are resolved",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-L", "./link.txt"},
			Want: "work/a.txt\n",
		},
		{
			Name: "the links themselves are still resolved",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-L", "link.txt"},
			Want: "work/a.txt\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunQuiet(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "suppresses the diagnostic of a failing operand",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-q", "missing/x"},
			WantExit: 1,
		},
		{
			Name:     "the other operands are still resolved",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-q", "missing/x", "a.txt"},
			Want:     "work/a.txt\n",
			WantExit: 1,
		},
		{
			Name: "does not change the output of successful operands",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-q", "a.txt"},
			Want: "work/a.txt\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunDelimiters(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-z ends each line with NUL",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-z", "a.txt", "d"},
			Want: "work/a.txt\x00work/d\x00",
		},
		{
			Name: "-z keeps working with -q",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-qz", "a.txt"},
			Want: "work/a.txt\x00",
		},
		{
			Name:     "-n is not an option of realpath",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-n", "a.txt"},
			WantErr:  "realpath: invalid option -- 'n'\nTry 'realpath --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "--no-newline is not an option of realpath",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"--no-newline", "a.txt"},
			WantErr:  "realpath: unrecognized option '--no-newline'\nTry 'realpath --help' for more information.\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRelativeTo(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the directory itself prints as a dot",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-to=usr/bin", "usr/bin"},
			Want: ".\n",
		},
		{
			Name: "a file outside the directory climbs with the parent components",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-to=usr/bin", "tmp/foo"},
			Want: "../../tmp/foo\n",
		},
		{
			Name: "a file below the directory is relative to it",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-to=usr/bin", "usr/share/dict/american-english"},
			Want: "../share/dict/american-english\n",
		},
		{
			Name: "the directory may be relative to the working directory",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-to=d", "a.txt"},
			Want: "../a.txt\n",
		},
		{
			Name: "the directory follows the symbolic links of the operands",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-to=link-d", "x/b.txt"},
			Want: "b.txt\n",
		},
		{
			Name: "a missing directory is fine by default",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-to=none", "a.txt"},
			Want: "../a.txt\n",
		},
		{
			Name:     "a missing directory is an error with -e",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-e", "--relative-to=none", "a.txt"},
			WantErr:  "realpath: none: file does not exist\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRelativeBase(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a file below the base is relative to it",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-base=usr", "usr/bin"},
			Want: "bin\n",
		},
		{
			Name: "the base itself is relative",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-base=usr", "usr"},
			Want: ".\n",
		},
		{
			Name: "a file outside the base is absolute",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-base=usr", "tmp/foo"},
			Want: "work/tmp/foo\n",
		},
		{
			Name: "a file below the base is relative to the base",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-base=usr", "usr/share/dict/american-english"},
			Want: "share/dict/american-english\n",
		},
		{
			Name: "both options print relative to --relative-to when below --relative-base",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-to=usr/bin", "--relative-base=usr", "usr/bin", "tmp/foo", "usr/share/dict/american-english"},
			Want: ".\nwork/tmp/foo\n../share/dict/american-english\n",
		},
		{
			Name: "a --relative-to outside the base makes the names absolute",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-to=tmp", "--relative-base=usr", "usr/bin", "tmp/foo"},
			Want: "work/usr/bin\nwork/tmp/foo\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunUsageErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "missing operands",
			FS:       tree(),
			Dir:      "work",
			WantErr:  "realpath: missing file operand\nTry 'realpath --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid short option",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-X", "a.txt"},
			WantErr:  "realpath: invalid option -- 'X'\nTry 'realpath --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an unknown long option",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"--bogus", "a.txt"},
			WantErr:  "realpath: unrecognized option '--bogus'\nTry 'realpath --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a long option without its argument",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"--relative-to"},
			WantErr:  "realpath: option '--relative-to' requires an argument\nTry 'realpath --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an option argument held by the next argument",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"--relative-to", "usr", "a.txt"},
			Want:     "../a.txt\n",
			WantExit: 0,
		},
		{
			Name:     "an empty operand",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{""},
			WantErr:  "realpath: invalid zero-length file name\nTry 'realpath --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name: "the options end at the first operand",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--", "-e"},
			Want: "work/-e\n",
		},
		{
			Name: "the options end at --",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-s", "--", "link.txt"},
			Want: "work/link.txt\n",
		},
		{
			Name:     "options after the operands are operands themselves",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"a.txt", "-z"},
			Want:     "work/a.txt\nwork/-z\n",
			WantExit: 0,
		},
	}

	testutil.Run(t, New, tests)
}

func TestCommonOptions(t *testing.T) {
	t.Run("--help prints the usage", func(t *testing.T) {
		testutil.Once(t, New, testutil.Case{
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--help"},
			Want: usage,
		})
	})
	t.Run("--version prints the program name", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer), FS: tree()}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if out := stdout.String(); !strings.Contains(out, "realpath (charm.land/turtle/coreutils)") {
			t.Errorf("Run() stdout = %q, want the version line", out)
		}
	})
	t.Run("--help after an operand is an operand", func(t *testing.T) {
		testutil.Once(t, New, testutil.Case{
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"a.txt", "--help"},
			Want:     "work/a.txt\nwork/--help\n",
			WantExit: 0,
		})
	})
}

func TestRunInteractions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "a missing component with a parent after it is an error",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"d/missing/../keep.txt"},
			WantErr:  "realpath: d/missing/../keep.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "the logical mode applies the parent to the missing component",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-Le", "link-d/missing/.."},
			Want: "work/x\n",
		},
		{
			Name: "the missing mode leaves a looping link as it is",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-m", "loop"},
			Want: "work/loop\n",
		},
		{
			Name:     "a stripped name with a trailing slash is still a directory",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-s", "a.txt/"},
			WantErr:  "realpath: a.txt/: not a directory\n",
			WantExit: 1,
		},
		{
			Name: "a stripped directory with a trailing slash resolves",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-s", "d/"},
			Want: "work/d\n",
		},
		{
			Name:     "-e requires the --relative-to directory to exist as a directory",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-e", "--relative-to=a.txt", "a.txt"},
			WantErr:  "realpath: a.txt: not a directory\n",
			WantExit: 1,
		},
		{
			Name:     "-e requires the --relative-base directory to be a directory",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-e", "--relative-base=d/keep.txt", "a.txt"},
			WantErr:  "realpath: d/keep.txt: not a directory\n",
			WantExit: 1,
		},
		{
			Name: "the last --relative-to wins",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--relative-to=usr/share", "--relative-to=d", "a.txt"},
			Want: "../a.txt\n",
		},
		{
			Name: "-z ends the relative names with NUL",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-z", "--relative-to=d", "a.txt"},
			Want: "../a.txt\x00",
		},
		{
			Name: "the last of -L, -P and -s wins: sP",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-sP", "link.txt"},
			Want: "work/a.txt\n",
		},
		{
			Name: "the last of -L, -P and -s wins: Ps",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-Ps", "link.txt"},
			Want: "work/link.txt\n",
		},
	}

	testutil.Run(t, New, tests)
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
		tests := []testutil.Case{
			{
				Name: "--relative-to=/",
				FS:   tree(),
				Dir:  "work",
				Args: []string{"--relative-to=/", "d/keep.txt"},
				Want: "work/d/keep.txt\n",
			},
			{
				Name: "--relative-base=/",
				FS:   tree(),
				Dir:  "work",
				Args: []string{"--relative-base=/", "d/keep.txt"},
				Want: "work/d/keep.txt\n",
			},
			{
				Name: "both options with a root base",
				FS:   tree(),
				Dir:  "work",
				Args: []string{"--relative-to=/", "--relative-base=/", "d/keep.txt"},
				Want: "work/d/keep.txt\n",
			},
		}
		testutil.Run(t, New, tests)
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
					FS:         fsx.SystemFS(""),
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
