package chgrp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// file returns a regular file with the given group.
func file(gid int) *fsx.MemoryFile {
	return &fsx.MemoryFile{Mode: 0o644, Gid: gid}
}

// dir returns a directory with the given group.
func dir(gid int) *fsx.MemoryFile {
	return &fsx.MemoryFile{Mode: fs.ModeDir | 0o755, Gid: gid}
}

// link returns a symbolic link to target with the given group.
func link(target string, gid int) *fsx.MemoryFile {
	return &fsx.MemoryFile{Data: []byte(target), Mode: fs.ModeSymlink | 0o777, Gid: gid}
}

// testCase is the fields of a testutil.Case plus the file fixtures of the
// group database free tests, and the groups the files must have after the run.
type testCase struct {
	Name     string
	Args     []string
	Want     string
	WantErr  string
	WantExit int

	Files    fsx.MemoryFS
	WantGids map[string]int
}

// runCase runs a single case, checking the groups of its files.
func runCase(t *testing.T, test testCase) {
	t.Helper()

	c := testutil.Case{
		Name:     test.Name,
		Args:     test.Args,
		Want:     test.Want,
		WantErr:  test.WantErr,
		WantExit: test.WantExit,
		FS:       test.Files,
	}
	wantGids := test.WantGids
	c.Check = func(t *testing.T, rt *coreutils.Runtime) {
		for name, want := range wantGids {
			fi, err := rt.FS.Lstat(name)
			if err != nil {
				t.Errorf("Lstat(%q) error = %v", name, err)
				continue
			}
			if _, gid, ok := fsx.Owner(fi); !ok || gid != want {
				t.Errorf("%s group = %d, want %d", name, gid, want)
			}
		}
	}
	testutil.Once(t, New, c)
}

// runTests runs the given cases as subtests.
func runTests(t *testing.T, tests []testCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunGroup(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:     "sets a numeric group",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"20", "file.txt"},
			WantGids: map[string]int{"file.txt": 20},
		},
		{
			Name:     "a leading plus forces the numeric group",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"+20", "file.txt"},
			WantGids: map[string]int{"file.txt": 20},
		},
		{
			Name:     "sets the group of every file",
			Files:    fsx.MemoryFS{"a.txt": file(10), "b.txt": file(11)},
			Args:     []string{"30", "a.txt", "b.txt"},
			WantGids: map[string]int{"a.txt": 30, "b.txt": 30},
		},
		{
			Name:     "group zero is a group of its own",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"0", "file.txt"},
			WantGids: map[string]int{"file.txt": 0},
		},
		{
			Name:     "a large numeric group",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"123456", "file.txt"},
			WantGids: map[string]int{"file.txt": 123456},
		},
		{
			Name:     "changes a directory without walking below it",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11)},
			Args:     []string{"20", "dir"},
			WantGids: map[string]int{"dir": 20, "dir/a.txt": 11},
		},
		{
			Name:     "a negative numeric group is invalid",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"--", "-1", "file.txt"},
			WantGids: map[string]int{"file.txt": 10},
			WantErr:  "chgrp: invalid group: '-1'\nTry 'chgrp --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "another negative numeric group is invalid",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"--", "-20", "file.txt"},
			WantGids: map[string]int{"file.txt": 10},
			WantErr:  "chgrp: invalid group: '-20'\nTry 'chgrp --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a numeric group too large for an ID is invalid",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"99999999999999999999", "file.txt"},
			WantGids: map[string]int{"file.txt": 10},
			WantErr:  "chgrp: invalid group: '99999999999999999999'\nTry 'chgrp --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an unknown group name is an error",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"no-such-group-xyz", "file.txt"},
			WantGids: map[string]int{"file.txt": 10},
			WantErr:  "chgrp: invalid group: 'no-such-group-xyz'\nTry 'chgrp --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a group which is neither a name nor a number",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"12abc", "file.txt"},
			WantGids: map[string]int{"file.txt": 10},
			WantErr:  "chgrp: invalid group: '12abc'\nTry 'chgrp --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an empty group is an error",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"", "file.txt"},
			WantGids: map[string]int{"file.txt": 10},
			WantErr:  "chgrp: invalid group: ''\nTry 'chgrp --help' for more information.\n",
			WantExit: 1,
		},
	})
}

func TestRunGroupName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the group database is not available on Windows")
	}
	u, err := user.Current()
	if err != nil {
		t.Skipf("no current user: %v", err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil || g.Name == "" {
		t.Skip("the group of the current user has no name")
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		t.Skipf("group %q has no numeric ID", g.Name)
	}

	runCase(t, testCase{
		Name:     "a group name resolves through the group database",
		Files:    fsx.MemoryFS{"file.txt": file(10)},
		Args:     []string{g.Name, "file.txt"},
		WantGids: map[string]int{"file.txt": gid},
	})
}

func TestRunKeepsOwner(t *testing.T) {
	files := fsx.MemoryFS{"file.txt": {Mode: 0o644, Uid: 5, Gid: 10}}
	rt := &coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     files,
	}
	if err := New(rt).Run(t.Context(), "20", "file.txt"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	fi, err := files.Lstat("file.txt")
	if err != nil {
		t.Fatalf("Lstat error = %v", err)
	}
	if uid, gid, _ := fsx.Owner(fi); uid != 5 || gid != 20 {
		t.Errorf("owner = %d:%d, want 5:20", uid, gid)
	}
}

func TestRunDereference(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:     "a symbolic link stands for what it points at",
			Files:    fsx.MemoryFS{"target": file(10), "link": link("target", 5)},
			Args:     []string{"20", "link"},
			WantGids: map[string]int{"target": 20, "link": 5},
		},
		{
			Name:     "h leaves the symbolic link and its referent alone",
			Files:    fsx.MemoryFS{"target": file(10), "link": link("target", 5)},
			Args:     []string{"-h", "20", "link"},
			WantGids: map[string]int{"target": 10, "link": 5},
		},
		{
			Name:     "no-dereference is the long form of h",
			Files:    fsx.MemoryFS{"target": file(10), "link": link("target", 5)},
			Args:     []string{"--no-dereference", "20", "link"},
			WantGids: map[string]int{"target": 10, "link": 5},
		},
		{
			Name:     "dereference states the default",
			Files:    fsx.MemoryFS{"target": file(10), "link": link("target", 5)},
			Args:     []string{"--dereference", "20", "link"},
			WantGids: map[string]int{"target": 20, "link": 5},
		},
		{
			Name:     "the last of h and dereference wins",
			Files:    fsx.MemoryFS{"target": file(10), "link": link("target", 5)},
			Args:     []string{"-h", "--dereference", "20", "link"},
			WantGids: map[string]int{"target": 20, "link": 5},
		},
		{
			Name:     "the last of dereference and h wins",
			Files:    fsx.MemoryFS{"target": file(10), "link": link("target", 5)},
			Args:     []string{"--dereference", "-h", "20", "link"},
			WantGids: map[string]int{"target": 10, "link": 5},
		},
		{
			Name:     "a chain of symbolic links is followed",
			Files:    fsx.MemoryFS{"target": file(10), "l2": link("target", 5), "l1": link("l2", 6)},
			Args:     []string{"20", "l1"},
			WantGids: map[string]int{"target": 20, "l2": 5, "l1": 6},
		},
		{
			Name:     "h leaves a chain of links alone",
			Files:    fsx.MemoryFS{"target": file(10), "l2": link("target", 5), "l1": link("l2", 6)},
			Args:     []string{"-h", "20", "l1"},
			WantGids: map[string]int{"target": 10, "l2": 5, "l1": 6},
		},
		{
			Name:     "a dangling link cannot be dereferenced",
			Files:    fsx.MemoryFS{"dangling": link("missing", 5)},
			Args:     []string{"20", "dangling"},
			WantGids: map[string]int{"dangling": 5},
			WantErr:  "chgrp: dangling: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "h leaves a dangling link alone without an error",
			Files:    fsx.MemoryFS{"dangling": link("missing", 5)},
			Args:     []string{"-h", "20", "dangling"},
			WantGids: map[string]int{"dangling": 5},
		},
	})
}

func TestRunRecursive(t *testing.T) {
	tree := func() fsx.MemoryFS {
		return fsx.MemoryFS{
			"dir":           dir(10),
			"dir/a.txt":     file(11),
			"dir/sub":       dir(12),
			"dir/sub/b.txt": file(13),
		}
	}

	runTests(t, []testCase{
		{
			Name:  "R changes every file below the directory",
			Files: tree(),
			Args:  []string{"-R", "20", "dir"},
			WantGids: map[string]int{
				"dir":           20,
				"dir/a.txt":     20,
				"dir/sub":       20,
				"dir/sub/b.txt": 20,
			},
		},
		{
			Name:     "R on a plain file works like without R",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"-R", "20", "file.txt"},
			WantGids: map[string]int{"file.txt": 20},
		},
		{
			Name:     "R works from the working directory",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11)},
			Args:     []string{"-R", "20", "."},
			WantGids: map[string]int{"dir": 20, "dir/a.txt": 20},
		},
		{
			Name:     "R keeps going after a missing file",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"-R", "20", "missing", "file.txt"},
			WantGids: map[string]int{"file.txt": 20},
			WantErr:  "chgrp: missing: file does not exist\n",
			WantExit: 1,
		},
	})
}

func TestRunRecursiveSymlinks(t *testing.T) {
	runTests(t, []testCase{
		{
			Name: "R leaves a symbolic link of the walk alone",
			Files: fsx.MemoryFS{
				"dir":       dir(10),
				"dir/a.txt": file(11),
				"dir/link":  link("../target", 5),
				"target":    file(9),
			},
			Args: []string{"-R", "20", "dir"},
			WantGids: map[string]int{
				"dir":       20,
				"dir/a.txt": 20,
				"dir/link":  5,
				"target":    9,
			},
		},
		{
			Name: "R leaves a dangling link of the walk alone",
			Files: fsx.MemoryFS{
				"dir":          dir(10),
				"dir/a.txt":    file(11),
				"dir/dangling": link("missing", 5),
			},
			Args: []string{"-R", "20", "dir"},
			WantGids: map[string]int{
				"dir":          20,
				"dir/a.txt":    20,
				"dir/dangling": 5,
			},
		},
		{
			Name:     "R does not follow a symbolic link operand to a directory",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			Args:     []string{"-R", "20", "dlink"},
			WantGids: map[string]int{"dir": 10, "dir/a.txt": 11, "dlink": 5},
		},
		{
			Name:     "P is the same as the default",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			Args:     []string{"-R", "-P", "20", "dlink"},
			WantGids: map[string]int{"dir": 10, "dir/a.txt": 11, "dlink": 5},
		},
		{
			Name:     "H follows a symbolic link operand to a directory",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			Args:     []string{"-RH", "20", "dlink"},
			WantGids: map[string]int{"dir": 20, "dir/a.txt": 20, "dlink": 5},
		},
		{
			Name:     "H follows a symbolic link operand to a file",
			Files:    fsx.MemoryFS{"target": file(9), "flink": link("target", 5)},
			Args:     []string{"-RH", "20", "flink"},
			WantGids: map[string]int{"target": 20, "flink": 5},
		},
		{
			Name: "L follows every symbolic link to a directory",
			Files: fsx.MemoryFS{
				"dir":           dir(10),
				"dir/a.txt":     file(11),
				"dir/dlink":     link("sub", 5),
				"dir/sub":       dir(12),
				"dir/sub/b.txt": file(13),
			},
			Args: []string{"-RL", "20", "dir"},
			WantGids: map[string]int{
				"dir":           20,
				"dir/a.txt":     20,
				"dir/dlink":     5,
				"dir/sub":       20,
				"dir/sub/b.txt": 20,
			},
		},
		{
			Name:     "L follows a symbolic link to a plain file",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/flink": link("../target", 5), "target": file(9)},
			Args:     []string{"-RL", "20", "dir"},
			WantGids: map[string]int{"dir": 20, "dir/flink": 5, "target": 20},
		},
		{
			Name:     "L reports a dangling link as an error",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/dangling": link("missing", 5)},
			Args:     []string{"-RL", "20", "dir"},
			WantGids: map[string]int{"dir": 20, "dir/dangling": 5},
			WantErr:  "chgrp: dir/dangling: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "the last of L and P wins",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			Args:     []string{"-R", "-L", "-P", "20", "dlink"},
			WantGids: map[string]int{"dir": 10, "dir/a.txt": 11, "dlink": 5},
		},
		{
			Name:     "the last of P and H wins",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			Args:     []string{"-R", "-P", "-H", "20", "dlink"},
			WantGids: map[string]int{"dir": 20, "dir/a.txt": 20, "dlink": 5},
		},
		{
			Name:     "hR leaves the links of the walk alone",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11), "dir/link": link("a.txt", 5)},
			Args:     []string{"-hR", "20", "dir"},
			WantGids: map[string]int{"dir": 20, "dir/a.txt": 20, "dir/link": 5},
		},
		{
			Name:     "hR leaves a symbolic link operand alone",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			Args:     []string{"-hR", "20", "dlink"},
			WantGids: map[string]int{"dir": 10, "dir/a.txt": 11, "dlink": 5},
		},
		{
			// With -h, only the files below the operand link change: the
			// link itself is left alone, and so is the directory it names.
			Name:     "h with H changes only the files below the operand link",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			Args:     []string{"-hRH", "20", "dlink"},
			WantGids: map[string]int{"dir": 10, "dir/a.txt": 20, "dlink": 5},
		},
		{
			Name:     "h with H leaves a dangling operand link alone",
			Files:    fsx.MemoryFS{"dangling": link("missing", 5)},
			Args:     []string{"-hRH", "20", "dangling"},
			WantGids: map[string]int{"dangling": 5},
		},
		{
			Name:     "h with L leaves a dangling link of the walk alone",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/dangling": link("missing", 5)},
			Args:     []string{"-hRL", "20", "dir"},
			WantGids: map[string]int{"dir": 20, "dir/dangling": 5},
		},
		{
			Name:     "dereference with R acts on the referent without walking it",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/link": link("../target", 5), "target": file(9)},
			Args:     []string{"-R", "--dereference", "20", "dir"},
			WantGids: map[string]int{"dir": 20, "dir/link": 5, "target": 20},
		},
		{
			Name:     "L detects a file system loop",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11), "dir/loop": link(".", 5)},
			Args:     []string{"-RL", "20", "dir"},
			WantGids: map[string]int{"dir": 20, "dir/a.txt": 20, "dir/loop": 5},
			WantErr:  "chgrp: dir/loop: file system loop detected; 'dir/loop' is part of the same file system loop as 'dir'\n",
			WantExit: 1,
		},
		{
			Name:     "h with L still detects a file system loop",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11), "dir/loop": link(".", 5)},
			Args:     []string{"-hRL", "20", "dir"},
			WantGids: map[string]int{"dir": 20, "dir/a.txt": 20, "dir/loop": 5},
			WantErr:  "chgrp: dir/loop: file system loop detected; 'dir/loop' is part of the same file system loop as 'dir'\n",
			WantExit: 1,
		},
		{
			Name: "R leaves the links of a loop it doesn't follow alone",
			Files: fsx.MemoryFS{
				"dir":        dir(10),
				"dir/link":   link("sub", 5),
				"dir/sub":    dir(12),
				"dir/sub/up": link("..", 6),
			},
			Args: []string{"-R", "20", "dir"},
			WantGids: map[string]int{
				"dir":        20,
				"dir/link":   5,
				"dir/sub":    20,
				"dir/sub/up": 6,
			},
		},
	})
}

func TestRunSilent(t *testing.T) {
	tests := []testCase{
		{
			Name:     "f",
			Args:     []string{"-f", "20", "missing", "file.txt"},
			WantGids: map[string]int{"file.txt": 20, "dir": 10},
		},
		{
			Name:     "silent",
			Args:     []string{"--silent", "20", "missing", "file.txt"},
			WantGids: map[string]int{"file.txt": 20, "dir": 10},
		},
		{
			Name:     "quiet",
			Args:     []string{"--quiet", "20", "missing", "file.txt"},
			WantGids: map[string]int{"file.txt": 20, "dir": 10},
		},
		{
			Name:     "f grouped with R",
			Args:     []string{"-fR", "20", "missing", "dir"},
			WantGids: map[string]int{"file.txt": 10, "dir": 20},
		},
	}
	for i := range tests {
		tests[i].Files = fsx.MemoryFS{"file.txt": file(10), "dir": dir(10)}
		tests[i].WantExit = 1
	}
	runTests(t, tests)

	t.Run("f keeps reporting an invalid group", func(t *testing.T) {
		runCase(t, testCase{
			Name:     "f",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"-f", "no-such-group-xyz", "file.txt"},
			WantGids: map[string]int{"file.txt": 10},
			WantErr:  "chgrp: invalid group: 'no-such-group-xyz'\nTry 'chgrp --help' for more information.\n",
			WantExit: 1,
		})
	})
}

func TestRunVerbose(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:     "v reports a change",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"-v", "20", "file.txt"},
			WantGids: map[string]int{"file.txt": 20},
			Want:     "changed group of 'file.txt' from 10 to 20\n",
		},
		{
			Name:     "v reports a retained group",
			Files:    fsx.MemoryFS{"file.txt": file(20)},
			Args:     []string{"-v", "20", "file.txt"},
			WantGids: map[string]int{"file.txt": 20},
			Want:     "group of 'file.txt' retained as 20\n",
		},
		{
			Name:     "c reports only the files whose group changed",
			Files:    fsx.MemoryFS{"a.txt": file(10), "b.txt": file(20)},
			Args:     []string{"-c", "20", "a.txt", "b.txt"},
			WantGids: map[string]int{"a.txt": 20, "b.txt": 20},
			Want:     "changed group of 'a.txt' from 10 to 20\n",
		},
		{
			Name:     "c reports nothing when no group changes",
			Files:    fsx.MemoryFS{"file.txt": file(20)},
			Args:     []string{"-c", "20", "file.txt"},
			WantGids: map[string]int{"file.txt": 20},
		},
		{
			Name:     "the last of c and v wins",
			Files:    fsx.MemoryFS{"file.txt": file(20)},
			Args:     []string{"-c", "-v", "20", "file.txt"},
			WantGids: map[string]int{"file.txt": 20},
			Want:     "group of 'file.txt' retained as 20\n",
		},
		{
			Name:     "the last of v and c wins",
			Files:    fsx.MemoryFS{"file.txt": file(20)},
			Args:     []string{"-v", "-c", "20", "file.txt"},
			WantGids: map[string]int{"file.txt": 20},
		},
		{
			Name:     "the long options work",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"--changes", "20", "file.txt"},
			WantGids: map[string]int{"file.txt": 20},
			Want:     "changed group of 'file.txt' from 10 to 20\n",
		},
		{
			Name:     "v reports the operand as given",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"-v", "20", "./file.txt"},
			WantGids: map[string]int{"file.txt": 20},
			Want:     "changed group of './file.txt' from 10 to 20\n",
		},
		{
			Name:     "v reports the group the link points at under its own name",
			Files:    fsx.MemoryFS{"target": file(10), "link": link("target", 5)},
			Args:     []string{"-v", "20", "link"},
			WantGids: map[string]int{"target": 20, "link": 5},
			Want:     "changed group of 'link' from 10 to 20\n",
		},
		{
			Name:     "v says nothing about a link h leaves alone",
			Files:    fsx.MemoryFS{"target": file(10), "link": link("target", 5)},
			Args:     []string{"-vh", "20", "link"},
			WantGids: map[string]int{"target": 10, "link": 5},
		},
	})
}

func TestRunVerboseRecursive(t *testing.T) {
	tree := func() fsx.MemoryFS {
		return fsx.MemoryFS{
			"dir":           dir(10),
			"dir/a.txt":     file(11),
			"dir/sub":       dir(12),
			"dir/sub/b.txt": file(13),
		}
	}

	runTests(t, []testCase{
		{
			Name:  "v reports every file of the walk",
			Files: tree(),
			Args:  []string{"-Rv", "20", "dir"},
			WantGids: map[string]int{
				"dir":           20,
				"dir/a.txt":     20,
				"dir/sub":       20,
				"dir/sub/b.txt": 20,
			},
			Want: "changed group of 'dir' from 10 to 20\n" +
				"changed group of 'dir/a.txt' from 11 to 20\n" +
				"changed group of 'dir/sub' from 12 to 20\n" +
				"changed group of 'dir/sub/b.txt' from 13 to 20\n",
		},
		{
			Name:     "v reports the files below a link followed with H",
			Files:    fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			Args:     []string{"-RHv", "20", "dlink"},
			WantGids: map[string]int{"dir": 20, "dir/a.txt": 20, "dlink": 5},
			Want: "changed group of 'dlink' from 10 to 20\n" +
				"changed group of 'dlink/a.txt' from 11 to 20\n",
		},
		{
			Name:  "c reports only the files whose group changed",
			Files: tree(),
			Args:  []string{"-Rc", "20", "dir", "dir"},
			WantGids: map[string]int{
				"dir":           20,
				"dir/a.txt":     20,
				"dir/sub":       20,
				"dir/sub/b.txt": 20,
			},
			Want: "changed group of 'dir' from 10 to 20\n" +
				"changed group of 'dir/a.txt' from 11 to 20\n" +
				"changed group of 'dir/sub' from 12 to 20\n" +
				"changed group of 'dir/sub/b.txt' from 13 to 20\n",
		},
	})
}

func TestRunReference(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:     "uses the group of the reference file",
			Files:    fsx.MemoryFS{"ref": file(30), "file.txt": file(10)},
			Args:     []string{"--reference=ref", "file.txt"},
			WantGids: map[string]int{"ref": 30, "file.txt": 30},
		},
		{
			Name:     "the reference can be a separate argument",
			Files:    fsx.MemoryFS{"ref": file(30), "file.txt": file(10)},
			Args:     []string{"--reference", "ref", "file.txt"},
			WantGids: map[string]int{"ref": 30, "file.txt": 30},
		},
		{
			Name:     "the group applies to every file",
			Files:    fsx.MemoryFS{"ref": file(30), "a.txt": file(10), "b.txt": file(20)},
			Args:     []string{"--reference=ref", "a.txt", "b.txt"},
			WantGids: map[string]int{"ref": 30, "a.txt": 30, "b.txt": 30},
		},
		{
			Name:     "a reference to a symbolic link is dereferenced",
			Files:    fsx.MemoryFS{"ref": file(30), "rlink": link("ref", 5), "file.txt": file(10)},
			Args:     []string{"--reference=rlink", "file.txt"},
			WantGids: map[string]int{"ref": 30, "file.txt": 30},
		},
		{
			Name:     "a reference can be combined with R",
			Files:    fsx.MemoryFS{"ref": file(30), "dir": dir(10), "dir/a.txt": file(11)},
			Args:     []string{"-R", "--reference=ref", "dir"},
			WantGids: map[string]int{"ref": 30, "dir": 30, "dir/a.txt": 30},
		},
		{
			// With --reference, every operand is a file: a group-looking
			// name is just a missing file.
			Name:     "a group-looking operand is a file with --reference",
			Files:    fsx.MemoryFS{"ref": file(30), "file.txt": file(10)},
			Args:     []string{"--reference=ref", "20", "file.txt"},
			WantGids: map[string]int{"ref": 30, "file.txt": 30},
			WantErr:  "chgrp: 20: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing reference file is an error",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"--reference=missing", "file.txt"},
			WantGids: map[string]int{"file.txt": 10},
			WantErr:  "chgrp: failed to get attributes of missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "no operands is a usage error",
			Files:    fsx.MemoryFS{"ref": file(30)},
			Args:     []string{"--reference=ref"},
			WantGids: map[string]int{"ref": 30},
			WantErr:  "chgrp: missing operand\nTry 'chgrp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a lone operand is a file, not a group",
			Files:    fsx.MemoryFS{"ref": file(30), "x+y": file(10)},
			Args:     []string{"--reference=ref", "x+y"},
			WantGids: map[string]int{"ref": 30, "x+y": 30},
		},
	})
}

func TestRunGeneral(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:     "a missing operand is an error",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     nil,
			WantGids: map[string]int{"file.txt": 10},
			WantErr:  "chgrp: missing operand\nTry 'chgrp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a missing file operand is an error",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"20"},
			WantGids: map[string]int{"file.txt": 10},
			WantErr:  "chgrp: missing file operand\nTry 'chgrp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid option is an error",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"-x", "20", "file.txt"},
			WantGids: map[string]int{"file.txt": 10},
			WantErr:  "chgrp: invalid option -- 'x'\nTry 'chgrp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unrecognized long option",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"--bogus", "20", "file.txt"},
			WantGids: map[string]int{"file.txt": 10},
			WantErr:  "chgrp: unrecognized option '--bogus'\nTry 'chgrp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a file named like an option can follow a double dash",
			Files:    fsx.MemoryFS{"-w": file(10)},
			Args:     []string{"--", "20", "-w"},
			WantGids: map[string]int{"-w": 20},
		},
		{
			Name:     "a missing file is an error, but the others are changed",
			Files:    fsx.MemoryFS{"file.txt": file(10)},
			Args:     []string{"20", "missing", "file.txt"},
			WantGids: map[string]int{"file.txt": 20},
			WantErr:  "chgrp: missing: file does not exist\n",
			WantExit: 1,
		},
	})
}

func TestRunNoFileSystem(t *testing.T) {
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: new(bytes.Buffer)}
	err := New(rt).Run(t.Context(), "20", "file.txt")
	if code := testutil.ExitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1", code)
	}
	if got := rt.Stderr.(*bytes.Buffer).String(); got != "chgrp: no file system available\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "chgrp: no file system available\n")
	}
}

// chgrplessFS is an fsx.FS which cannot change owners: a MemoryFS without its
// Chown method.
type chgrplessFS struct {
	inner fsx.MemoryFS
}

func (f chgrplessFS) Open(name string) (fs.File, error) { return f.inner.Open(name) }

func (f chgrplessFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	return f.inner.OpenFile(name, flag, perm)
}

func (f chgrplessFS) Chtimes(name string, atime, mtime time.Time) error {
	return f.inner.Chtimes(name, atime, mtime)
}

func (f chgrplessFS) Chmod(name string, mode fs.FileMode) error {
	return f.inner.Chmod(name, mode)
}

func (f chgrplessFS) Mkdir(name string, perm fs.FileMode) error {
	return f.inner.Mkdir(name, perm)
}

func (f chgrplessFS) Remove(name string) error { return f.inner.Remove(name) }

func (f chgrplessFS) Rename(oldName, newName string) error {
	return f.inner.Rename(oldName, newName)
}

func (f chgrplessFS) Link(oldName, newName string) error {
	return f.inner.Link(oldName, newName)
}

func (f chgrplessFS) Symlink(oldName, newName string) error {
	return f.inner.Symlink(oldName, newName)
}

func (f chgrplessFS) ReadLink(name string) (string, error) {
	return f.inner.ReadLink(name)
}

func (f chgrplessFS) Lstat(name string) (fs.FileInfo, error) {
	return f.inner.Lstat(name)
}

var _ fsx.FS = chgrplessFS{}

func TestRunNoChownFS(t *testing.T) {
	files := fsx.MemoryFS{"file.txt": file(10)}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: stderr, FS: chgrplessFS{inner: files}}
	err := New(rt).Run(t.Context(), "20", "file.txt")
	if code := testutil.ExitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1", code)
	}
	want := "chgrp: group ownership is not supported by this file system\n"
	if got := stderr.String(); got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
	if _, gid, _ := fsx.Owner(mustLstat(t, files, "file.txt")); gid != 10 {
		t.Errorf("file.txt group = %d, want 10", gid)
	}
}

func mustLstat(t *testing.T, files fsx.MemoryFS, name string) fs.FileInfo {
	t.Helper()
	fi, err := files.Lstat(name)
	if err != nil {
		t.Fatalf("Lstat(%q) error = %v", name, err)
	}
	return fi
}

func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	files := fsx.MemoryFS{"dir": dir(10), "dir/a.txt": file(11)}
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: new(bytes.Buffer), FS: files}
	err := New(rt).Run(ctx, "-R", "20", "dir")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if _, gid, _ := fsx.Owner(mustLstat(t, files, "dir")); gid != 10 {
		t.Errorf("dir group = %d, want 10", gid)
	}
}

func TestRunSystemFS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("group ownership is not modeled on Windows")
	}

	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o644))
	must(os.WriteFile(filepath.Join(dir, "sub", "b.txt"), nil, 0o600))
	must(os.Symlink("a.txt", filepath.Join(dir, "link.txt")))

	gid := os.Getgid()
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: stderr, FS: fsx.SystemFS(dir)}
	if err := New(rt).Run(t.Context(), "-R", strconv.Itoa(gid), "."); err != nil {
		t.Errorf("Run() error = %v, stderr = %q", err, stderr)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}

	for _, name := range []string{".", "a.txt", "sub", "sub/b.txt"} {
		fi, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("Lstat(%q) error = %v", name, err)
			continue
		}
		if _, got, ok := fsx.Owner(fi); !ok || got != gid {
			t.Errorf("%s group = %d, want %d", name, got, gid)
		}
	}

	t.Run("a dangling link cannot be dereferenced", func(t *testing.T) {
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: stderr, FS: fsx.SystemFS(dir)}
		err := New(rt).Run(t.Context(), strconv.Itoa(gid), "missing")
		if code := testutil.ExitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
		if got, want := stderr.String(), "chgrp: missing: no such file or directory\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})
}

func TestRunWindowsSystemFS(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows group ownership is not modeled on other systems")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr, FS: fsx.SystemFS(dir)}
	if err := New(rt).Run(t.Context(), "20", "a.txt"); err != nil {
		t.Errorf("Run() error = %v, stderr = %q", err, stderr)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "chgrp", usage)
}
