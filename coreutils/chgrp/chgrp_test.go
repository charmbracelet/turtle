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

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// file returns a regular file with the given group.
func file(gid int) *fsx.MemFile {
	return &fsx.MemFile{Mode: 0o644, Gid: gid}
}

// dir returns a directory with the given group.
func dir(gid int) *fsx.MemFile {
	return &fsx.MemFile{Mode: fs.ModeDir | 0o755, Gid: gid}
}

// link returns a symbolic link to target with the given group.
func link(target string, gid int) *fsx.MemFile {
	return &fsx.MemFile{Data: []byte(target), Mode: fs.ModeSymlink | 0o777, Gid: gid}
}

type testCase struct {
	name     string
	files    fsx.MemFS
	args     []string
	want     map[string]int
	wantOut  string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: stdout,
		Stderr: stderr,
		FS:     test.files,
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

	for name, want := range test.want {
		fi, err := test.files.Lstat(name)
		if err != nil {
			t.Errorf("Lstat(%q) error = %v", name, err)
			continue
		}
		if _, gid, ok := fsx.Owner(fi); !ok || gid != want {
			t.Errorf("%s group = %d, want %d", name, gid, want)
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

// runTests runs the given cases as subtests.
func runTests(t *testing.T, tests []testCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunGroup(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "sets a numeric group",
			files: fsx.MemFS{"file.txt": file(10)},
			args:  []string{"20", "file.txt"},
			want:  map[string]int{"file.txt": 20},
		},
		{
			name:  "a leading plus forces the numeric group",
			files: fsx.MemFS{"file.txt": file(10)},
			args:  []string{"+20", "file.txt"},
			want:  map[string]int{"file.txt": 20},
		},
		{
			name:  "sets the group of every file",
			files: fsx.MemFS{"a.txt": file(10), "b.txt": file(11)},
			args:  []string{"30", "a.txt", "b.txt"},
			want:  map[string]int{"a.txt": 30, "b.txt": 30},
		},
		{
			name:  "group zero is a group of its own",
			files: fsx.MemFS{"file.txt": file(10)},
			args:  []string{"0", "file.txt"},
			want:  map[string]int{"file.txt": 0},
		},
		{
			name:  "a large numeric group",
			files: fsx.MemFS{"file.txt": file(10)},
			args:  []string{"123456", "file.txt"},
			want:  map[string]int{"file.txt": 123456},
		},
		{
			name:  "changes a directory without walking below it",
			files: fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11)},
			args:  []string{"20", "dir"},
			want:  map[string]int{"dir": 20, "dir/a.txt": 11},
		},
		{
			name:     "a negative numeric group is invalid",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"--", "-1", "file.txt"},
			want:     map[string]int{"file.txt": 10},
			wantErr:  "chgrp: invalid group: '-1'\nTry 'chgrp --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "another negative numeric group is invalid",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"--", "-20", "file.txt"},
			want:     map[string]int{"file.txt": 10},
			wantErr:  "chgrp: invalid group: '-20'\nTry 'chgrp --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a numeric group too large for an ID is invalid",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"99999999999999999999", "file.txt"},
			want:     map[string]int{"file.txt": 10},
			wantErr:  "chgrp: invalid group: '99999999999999999999'\nTry 'chgrp --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an unknown group name is an error",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"no-such-group-xyz", "file.txt"},
			want:     map[string]int{"file.txt": 10},
			wantErr:  "chgrp: invalid group: 'no-such-group-xyz'\nTry 'chgrp --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a group which is neither a name nor a number",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"12abc", "file.txt"},
			want:     map[string]int{"file.txt": 10},
			wantErr:  "chgrp: invalid group: '12abc'\nTry 'chgrp --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an empty group is an error",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"", "file.txt"},
			want:     map[string]int{"file.txt": 10},
			wantErr:  "chgrp: invalid group: ''\nTry 'chgrp --help' for more information.\n",
			wantExit: 1,
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
		name:  "a group name resolves through the group database",
		files: fsx.MemFS{"file.txt": file(10)},
		args:  []string{g.Name, "file.txt"},
		want:  map[string]int{"file.txt": gid},
	})
}

func TestRunKeepsOwner(t *testing.T) {
	files := fsx.MemFS{"file.txt": {Mode: 0o644, Uid: 5, Gid: 10}}
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
			name:  "a symbolic link stands for what it points at",
			files: fsx.MemFS{"target": file(10), "link": link("target", 5)},
			args:  []string{"20", "link"},
			want:  map[string]int{"target": 20, "link": 5},
		},
		{
			name:  "h leaves the symbolic link and its referent alone",
			files: fsx.MemFS{"target": file(10), "link": link("target", 5)},
			args:  []string{"-h", "20", "link"},
			want:  map[string]int{"target": 10, "link": 5},
		},
		{
			name:  "no-dereference is the long form of h",
			files: fsx.MemFS{"target": file(10), "link": link("target", 5)},
			args:  []string{"--no-dereference", "20", "link"},
			want:  map[string]int{"target": 10, "link": 5},
		},
		{
			name:  "dereference states the default",
			files: fsx.MemFS{"target": file(10), "link": link("target", 5)},
			args:  []string{"--dereference", "20", "link"},
			want:  map[string]int{"target": 20, "link": 5},
		},
		{
			name:  "the last of h and dereference wins",
			files: fsx.MemFS{"target": file(10), "link": link("target", 5)},
			args:  []string{"-h", "--dereference", "20", "link"},
			want:  map[string]int{"target": 20, "link": 5},
		},
		{
			name:  "the last of dereference and h wins",
			files: fsx.MemFS{"target": file(10), "link": link("target", 5)},
			args:  []string{"--dereference", "-h", "20", "link"},
			want:  map[string]int{"target": 10, "link": 5},
		},
		{
			name:  "a chain of symbolic links is followed",
			files: fsx.MemFS{"target": file(10), "l2": link("target", 5), "l1": link("l2", 6)},
			args:  []string{"20", "l1"},
			want:  map[string]int{"target": 20, "l2": 5, "l1": 6},
		},
		{
			name:  "h leaves a chain of links alone",
			files: fsx.MemFS{"target": file(10), "l2": link("target", 5), "l1": link("l2", 6)},
			args:  []string{"-h", "20", "l1"},
			want:  map[string]int{"target": 10, "l2": 5, "l1": 6},
		},
		{
			name:     "a dangling link cannot be dereferenced",
			files:    fsx.MemFS{"dangling": link("missing", 5)},
			args:     []string{"20", "dangling"},
			want:     map[string]int{"dangling": 5},
			wantErr:  "chgrp: dangling: file does not exist\n",
			wantExit: 1,
		},
		{
			name:  "h leaves a dangling link alone without an error",
			files: fsx.MemFS{"dangling": link("missing", 5)},
			args:  []string{"-h", "20", "dangling"},
			want:  map[string]int{"dangling": 5},
		},
	})
}

func TestRunRecursive(t *testing.T) {
	tree := func() fsx.MemFS {
		return fsx.MemFS{
			"dir":           dir(10),
			"dir/a.txt":     file(11),
			"dir/sub":       dir(12),
			"dir/sub/b.txt": file(13),
		}
	}

	runTests(t, []testCase{
		{
			name:  "R changes every file below the directory",
			files: tree(),
			args:  []string{"-R", "20", "dir"},
			want: map[string]int{
				"dir":           20,
				"dir/a.txt":     20,
				"dir/sub":       20,
				"dir/sub/b.txt": 20,
			},
		},
		{
			name:  "R on a plain file works like without R",
			files: fsx.MemFS{"file.txt": file(10)},
			args:  []string{"-R", "20", "file.txt"},
			want:  map[string]int{"file.txt": 20},
		},
		{
			name:  "R works from the working directory",
			files: fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11)},
			args:  []string{"-R", "20", "."},
			want:  map[string]int{"dir": 20, "dir/a.txt": 20},
		},
		{
			name:     "R keeps going after a missing file",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"-R", "20", "missing", "file.txt"},
			want:     map[string]int{"file.txt": 20},
			wantErr:  "chgrp: missing: file does not exist\n",
			wantExit: 1,
		},
	})
}

func TestRunRecursiveSymlinks(t *testing.T) {
	runTests(t, []testCase{
		{
			name: "R leaves a symbolic link of the walk alone",
			files: fsx.MemFS{
				"dir":       dir(10),
				"dir/a.txt": file(11),
				"dir/link":  link("../target", 5),
				"target":    file(9),
			},
			args: []string{"-R", "20", "dir"},
			want: map[string]int{
				"dir":       20,
				"dir/a.txt": 20,
				"dir/link":  5,
				"target":    9,
			},
		},
		{
			name: "R leaves a dangling link of the walk alone",
			files: fsx.MemFS{
				"dir":          dir(10),
				"dir/a.txt":    file(11),
				"dir/dangling": link("missing", 5),
			},
			args: []string{"-R", "20", "dir"},
			want: map[string]int{
				"dir":          20,
				"dir/a.txt":    20,
				"dir/dangling": 5,
			},
		},
		{
			name:  "R does not follow a symbolic link operand to a directory",
			files: fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			args:  []string{"-R", "20", "dlink"},
			want:  map[string]int{"dir": 10, "dir/a.txt": 11, "dlink": 5},
		},
		{
			name:  "P is the same as the default",
			files: fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			args:  []string{"-R", "-P", "20", "dlink"},
			want:  map[string]int{"dir": 10, "dir/a.txt": 11, "dlink": 5},
		},
		{
			name:  "H follows a symbolic link operand to a directory",
			files: fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			args:  []string{"-RH", "20", "dlink"},
			want:  map[string]int{"dir": 20, "dir/a.txt": 20, "dlink": 5},
		},
		{
			name:  "H follows a symbolic link operand to a file",
			files: fsx.MemFS{"target": file(9), "flink": link("target", 5)},
			args:  []string{"-RH", "20", "flink"},
			want:  map[string]int{"target": 20, "flink": 5},
		},
		{
			name: "L follows every symbolic link to a directory",
			files: fsx.MemFS{
				"dir":           dir(10),
				"dir/a.txt":     file(11),
				"dir/dlink":     link("sub", 5),
				"dir/sub":       dir(12),
				"dir/sub/b.txt": file(13),
			},
			args: []string{"-RL", "20", "dir"},
			want: map[string]int{
				"dir":           20,
				"dir/a.txt":     20,
				"dir/dlink":     5,
				"dir/sub":       20,
				"dir/sub/b.txt": 20,
			},
		},
		{
			name:  "L follows a symbolic link to a plain file",
			files: fsx.MemFS{"dir": dir(10), "dir/flink": link("../target", 5), "target": file(9)},
			args:  []string{"-RL", "20", "dir"},
			want:  map[string]int{"dir": 20, "dir/flink": 5, "target": 20},
		},
		{
			name:     "L reports a dangling link as an error",
			files:    fsx.MemFS{"dir": dir(10), "dir/dangling": link("missing", 5)},
			args:     []string{"-RL", "20", "dir"},
			want:     map[string]int{"dir": 20, "dir/dangling": 5},
			wantErr:  "chgrp: dir/dangling: file does not exist\n",
			wantExit: 1,
		},
		{
			name:  "the last of L and P wins",
			files: fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			args:  []string{"-R", "-L", "-P", "20", "dlink"},
			want:  map[string]int{"dir": 10, "dir/a.txt": 11, "dlink": 5},
		},
		{
			name:  "the last of P and H wins",
			files: fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			args:  []string{"-R", "-P", "-H", "20", "dlink"},
			want:  map[string]int{"dir": 20, "dir/a.txt": 20, "dlink": 5},
		},
		{
			name:  "hR leaves the links of the walk alone",
			files: fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11), "dir/link": link("a.txt", 5)},
			args:  []string{"-hR", "20", "dir"},
			want:  map[string]int{"dir": 20, "dir/a.txt": 20, "dir/link": 5},
		},
		{
			name:  "hR leaves a symbolic link operand alone",
			files: fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			args:  []string{"-hR", "20", "dlink"},
			want:  map[string]int{"dir": 10, "dir/a.txt": 11, "dlink": 5},
		},
		{
			// With -h, only the files below the operand link change: the
			// link itself is left alone, and so is the directory it names.
			name:  "h with H changes only the files below the operand link",
			files: fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			args:  []string{"-hRH", "20", "dlink"},
			want:  map[string]int{"dir": 10, "dir/a.txt": 20, "dlink": 5},
		},
		{
			name:  "h with H leaves a dangling operand link alone",
			files: fsx.MemFS{"dangling": link("missing", 5)},
			args:  []string{"-hRH", "20", "dangling"},
			want:  map[string]int{"dangling": 5},
		},
		{
			name:  "h with L leaves a dangling link of the walk alone",
			files: fsx.MemFS{"dir": dir(10), "dir/dangling": link("missing", 5)},
			args:  []string{"-hRL", "20", "dir"},
			want:  map[string]int{"dir": 20, "dir/dangling": 5},
		},
		{
			name:  "dereference with R acts on the referent without walking it",
			files: fsx.MemFS{"dir": dir(10), "dir/link": link("../target", 5), "target": file(9)},
			args:  []string{"-R", "--dereference", "20", "dir"},
			want:  map[string]int{"dir": 20, "dir/link": 5, "target": 20},
		},
		{
			name:     "L detects a file system loop",
			files:    fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11), "dir/loop": link(".", 5)},
			args:     []string{"-RL", "20", "dir"},
			want:     map[string]int{"dir": 20, "dir/a.txt": 20, "dir/loop": 5},
			wantErr:  "chgrp: dir/loop: file system loop detected; 'dir/loop' is part of the same file system loop as 'dir'\n",
			wantExit: 1,
		},
		{
			name:     "h with L still detects a file system loop",
			files:    fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11), "dir/loop": link(".", 5)},
			args:     []string{"-hRL", "20", "dir"},
			want:     map[string]int{"dir": 20, "dir/a.txt": 20, "dir/loop": 5},
			wantErr:  "chgrp: dir/loop: file system loop detected; 'dir/loop' is part of the same file system loop as 'dir'\n",
			wantExit: 1,
		},
		{
			name: "R leaves the links of a loop it doesn't follow alone",
			files: fsx.MemFS{
				"dir":        dir(10),
				"dir/link":   link("sub", 5),
				"dir/sub":    dir(12),
				"dir/sub/up": link("..", 6),
			},
			args: []string{"-R", "20", "dir"},
			want: map[string]int{
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
			name: "f",
			args: []string{"-f", "20", "missing", "file.txt"},
			want: map[string]int{"file.txt": 20, "dir": 10},
		},
		{
			name: "silent",
			args: []string{"--silent", "20", "missing", "file.txt"},
			want: map[string]int{"file.txt": 20, "dir": 10},
		},
		{
			name: "quiet",
			args: []string{"--quiet", "20", "missing", "file.txt"},
			want: map[string]int{"file.txt": 20, "dir": 10},
		},
		{
			name: "f grouped with R",
			args: []string{"-fR", "20", "missing", "dir"},
			want: map[string]int{"file.txt": 10, "dir": 20},
		},
	}
	for i := range tests {
		tests[i].files = fsx.MemFS{"file.txt": file(10), "dir": dir(10)}
		tests[i].wantExit = 1
	}
	runTests(t, tests)

	t.Run("f keeps reporting an invalid group", func(t *testing.T) {
		runCase(t, testCase{
			name:     "f",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"-f", "no-such-group-xyz", "file.txt"},
			want:     map[string]int{"file.txt": 10},
			wantErr:  "chgrp: invalid group: 'no-such-group-xyz'\nTry 'chgrp --help' for more information.\n",
			wantExit: 1,
		})
	})
}

func TestRunVerbose(t *testing.T) {
	runTests(t, []testCase{
		{
			name:    "v reports a change",
			files:   fsx.MemFS{"file.txt": file(10)},
			args:    []string{"-v", "20", "file.txt"},
			want:    map[string]int{"file.txt": 20},
			wantOut: "changed group of 'file.txt' from 10 to 20\n",
		},
		{
			name:    "v reports a retained group",
			files:   fsx.MemFS{"file.txt": file(20)},
			args:    []string{"-v", "20", "file.txt"},
			want:    map[string]int{"file.txt": 20},
			wantOut: "group of 'file.txt' retained as 20\n",
		},
		{
			name:    "c reports only the files whose group changed",
			files:   fsx.MemFS{"a.txt": file(10), "b.txt": file(20)},
			args:    []string{"-c", "20", "a.txt", "b.txt"},
			want:    map[string]int{"a.txt": 20, "b.txt": 20},
			wantOut: "changed group of 'a.txt' from 10 to 20\n",
		},
		{
			name:  "c reports nothing when no group changes",
			files: fsx.MemFS{"file.txt": file(20)},
			args:  []string{"-c", "20", "file.txt"},
			want:  map[string]int{"file.txt": 20},
		},
		{
			name:    "the last of c and v wins",
			files:   fsx.MemFS{"file.txt": file(20)},
			args:    []string{"-c", "-v", "20", "file.txt"},
			want:    map[string]int{"file.txt": 20},
			wantOut: "group of 'file.txt' retained as 20\n",
		},
		{
			name:  "the last of v and c wins",
			files: fsx.MemFS{"file.txt": file(20)},
			args:  []string{"-v", "-c", "20", "file.txt"},
			want:  map[string]int{"file.txt": 20},
		},
		{
			name:    "the long options work",
			files:   fsx.MemFS{"file.txt": file(10)},
			args:    []string{"--changes", "20", "file.txt"},
			want:    map[string]int{"file.txt": 20},
			wantOut: "changed group of 'file.txt' from 10 to 20\n",
		},
		{
			name:    "v reports the operand as given",
			files:   fsx.MemFS{"file.txt": file(10)},
			args:    []string{"-v", "20", "./file.txt"},
			want:    map[string]int{"file.txt": 20},
			wantOut: "changed group of './file.txt' from 10 to 20\n",
		},
		{
			name:    "v reports the group the link points at under its own name",
			files:   fsx.MemFS{"target": file(10), "link": link("target", 5)},
			args:    []string{"-v", "20", "link"},
			want:    map[string]int{"target": 20, "link": 5},
			wantOut: "changed group of 'link' from 10 to 20\n",
		},
		{
			name:  "v says nothing about a link h leaves alone",
			files: fsx.MemFS{"target": file(10), "link": link("target", 5)},
			args:  []string{"-vh", "20", "link"},
			want:  map[string]int{"target": 10, "link": 5},
		},
	})
}

func TestRunVerboseRecursive(t *testing.T) {
	tree := func() fsx.MemFS {
		return fsx.MemFS{
			"dir":           dir(10),
			"dir/a.txt":     file(11),
			"dir/sub":       dir(12),
			"dir/sub/b.txt": file(13),
		}
	}

	runTests(t, []testCase{
		{
			name:  "v reports every file of the walk",
			files: tree(),
			args:  []string{"-Rv", "20", "dir"},
			want: map[string]int{
				"dir":           20,
				"dir/a.txt":     20,
				"dir/sub":       20,
				"dir/sub/b.txt": 20,
			},
			wantOut: "changed group of 'dir' from 10 to 20\n" +
				"changed group of 'dir/a.txt' from 11 to 20\n" +
				"changed group of 'dir/sub' from 12 to 20\n" +
				"changed group of 'dir/sub/b.txt' from 13 to 20\n",
		},
		{
			name:  "v reports the files below a link followed with H",
			files: fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11), "dlink": link("dir", 5)},
			args:  []string{"-RHv", "20", "dlink"},
			want:  map[string]int{"dir": 20, "dir/a.txt": 20, "dlink": 5},
			wantOut: "changed group of 'dlink' from 10 to 20\n" +
				"changed group of 'dlink/a.txt' from 11 to 20\n",
		},
		{
			name:  "c reports only the files whose group changed",
			files: tree(),
			args:  []string{"-Rc", "20", "dir", "dir"},
			want: map[string]int{
				"dir":           20,
				"dir/a.txt":     20,
				"dir/sub":       20,
				"dir/sub/b.txt": 20,
			},
			wantOut: "changed group of 'dir' from 10 to 20\n" +
				"changed group of 'dir/a.txt' from 11 to 20\n" +
				"changed group of 'dir/sub' from 12 to 20\n" +
				"changed group of 'dir/sub/b.txt' from 13 to 20\n",
		},
	})
}

func TestRunReference(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "uses the group of the reference file",
			files: fsx.MemFS{"ref": file(30), "file.txt": file(10)},
			args:  []string{"--reference=ref", "file.txt"},
			want:  map[string]int{"ref": 30, "file.txt": 30},
		},
		{
			name:  "the reference can be a separate argument",
			files: fsx.MemFS{"ref": file(30), "file.txt": file(10)},
			args:  []string{"--reference", "ref", "file.txt"},
			want:  map[string]int{"ref": 30, "file.txt": 30},
		},
		{
			name:  "the group applies to every file",
			files: fsx.MemFS{"ref": file(30), "a.txt": file(10), "b.txt": file(20)},
			args:  []string{"--reference=ref", "a.txt", "b.txt"},
			want:  map[string]int{"ref": 30, "a.txt": 30, "b.txt": 30},
		},
		{
			name:  "a reference to a symbolic link is dereferenced",
			files: fsx.MemFS{"ref": file(30), "rlink": link("ref", 5), "file.txt": file(10)},
			args:  []string{"--reference=rlink", "file.txt"},
			want:  map[string]int{"ref": 30, "file.txt": 30},
		},
		{
			name:  "a reference can be combined with R",
			files: fsx.MemFS{"ref": file(30), "dir": dir(10), "dir/a.txt": file(11)},
			args:  []string{"-R", "--reference=ref", "dir"},
			want:  map[string]int{"ref": 30, "dir": 30, "dir/a.txt": 30},
		},
		{
			// With --reference, every operand is a file: a group-looking
			// name is just a missing file.
			name:     "a group-looking operand is a file with --reference",
			files:    fsx.MemFS{"ref": file(30), "file.txt": file(10)},
			args:     []string{"--reference=ref", "20", "file.txt"},
			want:     map[string]int{"ref": 30, "file.txt": 30},
			wantErr:  "chgrp: 20: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing reference file is an error",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"--reference=missing", "file.txt"},
			want:     map[string]int{"file.txt": 10},
			wantErr:  "chgrp: failed to get attributes of missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "no operands is a usage error",
			files:    fsx.MemFS{"ref": file(30)},
			args:     []string{"--reference=ref"},
			want:     map[string]int{"ref": 30},
			wantErr:  "chgrp: missing operand\nTry 'chgrp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:  "a lone operand is a file, not a group",
			files: fsx.MemFS{"ref": file(30), "x+y": file(10)},
			args:  []string{"--reference=ref", "x+y"},
			want:  map[string]int{"ref": 30, "x+y": 30},
		},
	})
}

func TestRunGeneral(t *testing.T) {
	runTests(t, []testCase{
		{
			name:     "a missing operand is an error",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     nil,
			want:     map[string]int{"file.txt": 10},
			wantErr:  "chgrp: missing operand\nTry 'chgrp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a missing file operand is an error",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"20"},
			want:     map[string]int{"file.txt": 10},
			wantErr:  "chgrp: missing file operand\nTry 'chgrp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid option is an error",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"-x", "20", "file.txt"},
			want:     map[string]int{"file.txt": 10},
			wantErr:  "chgrp: invalid option -- 'x'\nTry 'chgrp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"--bogus", "20", "file.txt"},
			want:     map[string]int{"file.txt": 10},
			wantErr:  "chgrp: unrecognized option '--bogus'\nTry 'chgrp --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:  "a file named like an option can follow a double dash",
			files: fsx.MemFS{"-w": file(10)},
			args:  []string{"--", "20", "-w"},
			want:  map[string]int{"-w": 20},
		},
		{
			name:     "a missing file is an error, but the others are changed",
			files:    fsx.MemFS{"file.txt": file(10)},
			args:     []string{"20", "missing", "file.txt"},
			want:     map[string]int{"file.txt": 20},
			wantErr:  "chgrp: missing: file does not exist\n",
			wantExit: 1,
		},
	})
}

func TestRunNoFileSystem(t *testing.T) {
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: new(bytes.Buffer)}
	err := New(rt).Run(t.Context(), "20", "file.txt")
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1", code)
	}
	if got := rt.Stderr.(*bytes.Buffer).String(); got != "chgrp: no file system available\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "chgrp: no file system available\n")
	}
}

// chgrplessFS is an fsx.FS which cannot change owners: a MemFS without its
// Chown method.
type chgrplessFS struct {
	inner fsx.MemFS
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
	files := fsx.MemFS{"file.txt": file(10)}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: stderr, FS: chgrplessFS{inner: files}}
	err := New(rt).Run(t.Context(), "20", "file.txt")
	if code := exitCode(t, err); code != 1 {
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

func mustLstat(t *testing.T, files fsx.MemFS, name string) fs.FileInfo {
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
	files := fsx.MemFS{"dir": dir(10), "dir/a.txt": file(11)}
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: new(bytes.Buffer), FS: files}
	err := New(rt).Run(ctx, "-R", "20", "dir")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if _, gid, _ := fsx.Owner(mustLstat(t, files, "dir")); gid != 10 {
		t.Errorf("dir group = %d, want 10", gid)
	}
}

func TestRunOsFS(t *testing.T) {
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
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: stderr, FS: fsx.OsFS(dir)}
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
		rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: stderr, FS: fsx.OsFS(dir)}
		err := New(rt).Run(t.Context(), strconv.Itoa(gid), "missing")
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
		if got, want := stderr.String(), "chgrp: missing: no such file or directory\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})
}

func TestRunWindowsOsFS(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows group ownership is not modeled on other systems")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr, FS: fsx.OsFS(dir)}
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
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "chgrp (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
