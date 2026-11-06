package find

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"strings"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

func TestRunPrintf(t *testing.T) {
	mtime := time.Date(2025, 12, 31, 13, 14, 15, 123456789, time.UTC)
	files := fsx.MemFS{
		"dir/a": {Data: []byte("hello"), Mode: 0o644, ModTime: mtime, Uid: 424242, Gid: 424243},
	}
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name: "the whole path",
			args: []string{"dir", "-printf", "%p\n"},
			want: "dir\ndir/a\n",
		},
		{
			name: "the basename",
			args: []string{"dir", "-printf", "%f\n"},
			want: "dir\na\n",
		},
		{
			name: "the leading directories",
			args: []string{"dir", "-printf", "%h\n"},
			want: ".\ndir\n",
		},
		{
			name: "the path below the starting point",
			args: []string{"dir", "-printf", "%P\n"},
			want: "\na\n",
		},
		{
			name: "the starting point",
			args: []string{"dir", "-printf", "%H\n"},
			want: "dir\ndir\n",
		},
		{
			name: "the starting point below the working directory",
			args: []string{"./dir", "-printf", "%H %p %P\n"},
			want: "./dir ./dir \n./dir ./dir/a a\n",
		},
		{
			name: "owner names fall back to the identifiers",
			args: []string{"dir/a", "-printf", "%u %U %g %G\n"},
			want: "424242 424242 424243 424243\n",
		},
		{
			name: "the mode",
			args: []string{"dir/a", "-printf", "%m %#m\n"},
			want: "644 0644\n",
		},
		{
			name: "the symbolic mode",
			args: []string{"dir/a", "-printf", "%M\n"},
			want: "-rw-r--r--\n",
		},
		{
			name: "the number of links",
			args: []string{"dir/a", "-printf", "%n\n"},
			want: "0\n",
		},
		{
			name: "the size",
			args: []string{"dir/a", "-printf", "%s %b %k\n"},
			want: "5 1 1\n",
		},
		{
			name: "the sparseness",
			args: []string{"dir/a", "-printf", "%S\n"},
			want: "1.0\n",
		},
		{
			name: "the depth",
			args: []string{"dir", "-printf", "%d %d\n"},
			want: "0 0\n1 1\n",
		},
		{
			name: "the type",
			args: []string{"dir", "-printf", "%y\n"},
			want: "d\nf\n",
		},
		{
			name: "the times in the ctime format",
			args: []string{"dir/a", "-printf", "%t|%a|%c\n"},
			want: "Wed Dec 31 13:14:15 2025|Wed Dec 31 13:14:15 2025|Wed Dec 31 13:14:15 2025\n",
		},
		{
			name: "the modification time",
			args: []string{"dir/a", "-printf", "%TY-%Tm-%Td %TH:%TM:%TS\n"},
			want: "2025-12-31 13:14:15.123456789\n",
		},
		{
			name: "the seconds since the epoch",
			args: []string{"dir/a", "-printf", "%T@\n"},
			want: fmt.Sprintf("%d.123456789\n", mtime.Unix()),
		},
		{
			name: "the combined format",
			args: []string{"dir/a", "-printf", "%T+\n"},
			want: "2025-12-31+13:14:15.123456789\n",
		},
		{
			name: "a strftime component",
			args: []string{"dir/a", "-printf", "%Ta %Tb %Tj %TZ\n"},
			want: "Wed Dec 365 UTC\n",
		},
		{
			name: "the birth time is empty",
			args: []string{"dir/a", "-printf", "a%BTb\n"},
			want: "ab\n",
		},
		{
			name: "the link target",
			args: []string{"ln", "-printf", "%l|%y|%Y\n"},
			want: "dir/a|l|f\n",
		},
		{
			name: "a broken link target",
			args: []string{"ln", "-printf", "%y|%Y\n"},
			want: "l|N\n",
		},
		{
			name: "no link target for a regular file",
			args: []string{"dir/a", "-printf", "a%lb\n"},
			want: "ab\n",
		},
		{
			name: "a literal percent",
			args: []string{"dir/a", "-printf", "100%%\n"},
			want: "100%\n",
		},
		{
			name: "escapes",
			args: []string{"dir/a", "-printf", "a\\tb\\nc\\040d\\\\e\n"},
			want: "a\tb\nc d\\e\n",
		},
		{
			name: "a null byte",
			args: []string{"dir/a", "-printf", "%p\\0"},
			want: "dir/a\x00",
		},
		{
			name: "c stops the format",
			args: []string{"dir/a", "-printf", "%p\\n\\cX% p\n"},
			want: "dir/a\n",
		},
		{
			name: "field width and left alignment",
			args: []string{"dir/a", "-printf", "[%8p][%-8p]|\n"},
			want: "[   dir/a][dir/a   ]|\n",
		},
		{
			name: "precision",
			args: []string{"dir/a", "-printf", "%.3p|%.0p|\n"},
			want: "dir||\n",
		},
		{
			name: "numeric flags on d",
			args: []string{"dir", "-printf", "[%03d][%+d]\n"},
			want: "[000][+0]\n[001][+1]\n",
		},
		{
			name:    "an unknown directive prints its character",
			args:    []string{"dir/a", "-printf", "a%Qb\n"},
			want:    "aQb\n",
			wantErr: "find: warning: unrecognised directive '%Q'\n",
		},
		{
			name:    "an unknown escape prints both characters",
			args:    []string{"dir/a", "-printf", "a\\Qb\n"},
			want:    "a\\Qb\n",
			wantErr: "find: warning: unrecognised escape '\\Q'\n",
		},
		{
			name:     "a reserved directive",
			args:     []string{"dir/a", "-printf", "%(x\n"},
			wantErr:  "find: the '%(' directive is reserved for future use for -printf\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
		{
			name: "a trailing percent is undefined, taken literally",
			args: []string{"dir/a", "-printf", "100%"},
			want: "100%",
		},
		{
			name:  "the whole path with a dot starting point",
			args:  []string{".", "-printf", "%p %P\n"},
			files: fsx.MemFS{"dir/a": {ModTime: mtime}},
			want:  ". \n./dir dir\n./dir/a dir/a\n",
		},
	}
	// The link tests need a symbolic link, and a memory file system is a
	// map the helper mutates, so every test takes a fresh copy of it.
	withLink := func() fsx.MemFS {
		return symlinkFS(t, "ln", "dir/a", copyFiles(files))
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := test.files
			switch test.name {
			case "the link target":
				f = withLink()
			case "a broken link target":
				f = symlinkFS(t, "ln", "missing", copyFiles(files))
			default:
				if f == nil {
					f = copyFiles(files)
				}
			}
			runTest(t, test.args, f, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

// copyFiles returns a copy of a memory file system.
func copyFiles(files fsx.MemFS) fsx.MemFS {
	c := fsx.MemFS{}
	maps.Copy(c, files)
	return c
}

func TestRunPrintfMissingArgument(t *testing.T) {
	runTest(t, []string{".", "-printf"}, fsx.MemFS{}, nil, nil, "",
		"find: missing argument to '-printf'\nTry 'find --help' for more information.\n", 1)
}

func TestRunLS(t *testing.T) {
	mtime := time.Date(2025, 12, 31, 13, 14, 15, 0, time.UTC)
	files := fsx.MemFS{
		"dir/a": {Data: []byte("hello"), Mode: 0o644, ModTime: mtime, Uid: 424242, Gid: 424243},
	}
	want := fmt.Sprintf("%7d %6d %s %3d %-8s %-8s %8s %s %s\n",
		0, 1, "-rw-r--r--", 1, "424242", "424243", "5", "Dec 31 13:14", "dir/a")
	runTest(t, []string{"dir/a", "-ls"}, files, nil, nil, want, "", 0)
}

func TestRunLSSymlink(t *testing.T) {
	mtime := time.Date(2025, 12, 31, 13, 14, 15, 0, time.UTC)
	files := symlinkFS(t, "ln", "dir/a", fsx.MemFS{
		"dir/a": {Data: []byte("hello"), Mode: 0o644, ModTime: mtime, Uid: 424242, Gid: 424243},
	})
	files["ln"].Uid = 424242
	files["ln"].Gid = 424243
	files["ln"].ModTime = mtime
	want := fmt.Sprintf("%7d %6d %s %3d %-8s %-8s %8s %s %s -> %s\n",
		0, 1, "lrwxrwxrwx", 1, "424242", "424243", "5", "Dec 31 13:14", "ln", "dir/a")
	runTest(t, []string{"ln", "-ls"}, files, nil, nil, want, "", 0)
}

func TestRunIgnoreReaddirRace(t *testing.T) {
	// The options take effect immediately, wherever they sit in the
	// expression, so even the error of a missing starting point is
	// suppressed.
	runTest(t, []string{"missing", "-ignore_readdir_race", "-print"}, fsx.MemFS{}, nil, nil, "", "", 0)
	runTest(t, []string{"missing", "-print"}, fsx.MemFS{}, nil, nil, "",
		"find: 'missing': file does not exist\n", 1)
}

func TestRunLSQuotes(t *testing.T) {
	files := fsx.MemFS{"a b\tc": {}}
	stderr := new(bytes.Buffer)
	runtime := &coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: stderr,
		FS:     files,
		Clock:  func() time.Time { return testNow },
	}
	_ = New(runtime).Run(t.Context(), ".", "-ls", "-name", "a*")
	if got, want := stderr.String(), ""; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestRunFprintFamily(t *testing.T) {
	t.Run("fprint writes to a file", func(t *testing.T) {
		files := fsx.MemFS{"a": {}, "out": {Data: []byte("old")}}
		runTest(t, []string{".", "-name", "a", "-fprint", "out"}, files, nil, nil, "", "", 0)
		if got, want := string(files["out"].Data), "./a\n"; got != want {
			t.Errorf("out = %q, want %q", got, want)
		}
	})

	t.Run("fprint0 writes null bytes", func(t *testing.T) {
		files := fsx.MemFS{"a": {}}
		runTest(t, []string{".", "-name", "a", "-fprint0", "out"}, files, nil, nil, "", "", 0)
		if got, want := string(files["out"].Data), "./a\x00"; got != want {
			t.Errorf("out = %q, want %q", got, want)
		}
	})

	t.Run("fprintf takes a format", func(t *testing.T) {
		files := fsx.MemFS{"a": {}}
		runTest(t, []string{".", "-name", "a", "-fprintf", "out", "%f:%y\n"}, files, nil, nil, "", "", 0)
		if got, want := string(files["out"].Data), "a:f\n"; got != want {
			t.Errorf("out = %q, want %q", got, want)
		}
	})

	t.Run("fls writes a listing", func(t *testing.T) {
		files := fsx.MemFS{"a": {}}
		runTest(t, []string{".", "-name", "a", "-fls", "out"}, files, nil, nil, "", "", 0)
		if got := string(files["out"].Data); !strings.HasSuffix(got, "/a\n") {
			t.Errorf("out = %q, want a listing ending in '/a\\n'", got)
		}
	})

	t.Run("the standard output and error names are special", func(t *testing.T) {
		files := fsx.MemFS{"a": {}}
		runTest(t, []string{".", "-name", "a", "-fprint", "/dev/stdout"}, files, nil, nil, "./a\n", "", 0)
	})

	t.Run("an output file that can't be opened", func(t *testing.T) {
		files := fsx.MemFS{"a": {}, "d/x": {}}
		runTest(t, []string{".", "-name", "a", "-fprint", "d"}, files, nil, nil, "",
			"find: cannot open 'd' for writing: invalid argument\n", 1)
	})

	t.Run("missing argument", func(t *testing.T) {
		runTest(t, []string{".", "-fprint"}, fsx.MemFS{}, nil, nil, "",
			"find: missing argument to '-fprint'\nTry 'find --help' for more information.\n", 1)
	})
}

func TestRunDelete(t *testing.T) {
	t.Run("the named files are removed", func(t *testing.T) {
		files := fsx.MemFS{"d/a": {}, "d/sub/b": {}, "keep": {}}
		runTest(t, []string{"d", "-name", "a", "-delete"}, files, nil, nil, "", "", 0)
		if _, ok := files["d/a"]; ok {
			t.Errorf("d/a was not deleted")
		}
		if _, ok := files["d/sub/b"]; !ok {
			t.Errorf("d/sub/b was deleted")
		}
	})

	t.Run("a directory is deleted after its entries", func(t *testing.T) {
		files := fsx.MemFS{"d": {Mode: dirMode()}, "d/a": {}}
		runTest(t, []string{"d", "-delete"}, files, nil, nil, "", "", 0)
		if len(files) != 0 {
			t.Errorf("files = %v, want empty", files)
		}
	})

	t.Run("a non-empty directory is not removed", func(t *testing.T) {
		files := fsx.MemFS{"d": {Mode: dirMode()}, "d/y": {Data: []byte("z")}}
		runTest(t, []string{"d", "-maxdepth", "0", "-delete"}, files, nil, nil, "",
			"find: cannot delete 'd': directory not empty\n", 1)
	})
}

func TestRunExecdir(t *testing.T) {
	type invocation struct {
		dir  string
		args []string
	}
	var invocations []invocation
	runtime := &coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     fsx.MemFS{"d/a": {}, "d/b": {}, "d/sub/c": {}},
		Clock:  func() time.Time { return testNow },
		Exec: func(_ context.Context, opts coreutils.ExecOpts) (int, error) {
			invocations = append(invocations, invocation{dir: opts.Dir, args: append([]string{opts.Name}, opts.Args...)})
			return 0, nil
		},
	}
	if err := New(runtime).Run(t.Context(), ".", "-type", "f", "-execdir", "touch", "{}", ";"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := []invocation{
		{dir: "d", args: []string{"touch", "./a"}},
		{dir: "d", args: []string{"touch", "./b"}},
		{dir: "d/sub", args: []string{"touch", "./c"}},
	}
	if len(invocations) != len(want) {
		t.Fatalf("invocations = %v, want %v", invocations, want)
	}
	for i, got := range invocations {
		if got.dir != want[i].dir || strings.Join(got.args, " ") != strings.Join(want[i].args, " ") {
			t.Errorf("invocations[%d] = %v, want %v", i, got, want[i])
		}
	}
}

func TestRunExecdirAggregating(t *testing.T) {
	var groups [][]string
	var dirs []string
	runtime := &coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     fsx.MemFS{"d/a": {}, "d/b": {}, "d/sub/c": {}},
		Clock:  func() time.Time { return testNow },
		Exec: func(_ context.Context, opts coreutils.ExecOpts) (int, error) {
			dirs = append(dirs, opts.Dir)
			groups = append(groups, opts.Args)
			return 0, nil
		},
	}
	if err := New(runtime).Run(t.Context(), ".", "-type", "f", "-execdir", "touch", "{}", "+"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	wantDirs := []string{"d", "d/sub"}
	wantArgs := [][]string{{"./a", "./b"}, {"./c"}}
	if len(dirs) != len(wantDirs) {
		t.Fatalf("dirs = %v, want %v", dirs, wantDirs)
	}
	for i := range dirs {
		if dirs[i] != wantDirs[i] || strings.Join(groups[i], " ") != strings.Join(wantArgs[i], " ") {
			t.Errorf("group %d = (%q, %v), want (%q, %v)", i, dirs[i], groups[i], wantDirs[i], wantArgs[i])
		}
	}
}

func TestRunOkdir(t *testing.T) {
	var args []string
	runtime := &coreutils.Runtime{
		Stdin:  strings.NewReader("y\n"),
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
		FS:     fsx.MemFS{"d/a": {}},
		Clock:  func() time.Time { return testNow },
		Exec: func(_ context.Context, opts coreutils.ExecOpts) (int, error) {
			args = append([]string{opts.Name}, opts.Args...)
			return 0, nil
		},
	}
	if err := New(runtime).Run(t.Context(), ".", "-name", "a", "-okdir", "echo", "{}", ";"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := strings.Join(args, " "), "echo ./a"; got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
}

// dirMode returns the directory mode.
func dirMode() fs.FileMode {
	return fs.ModeDir
}
