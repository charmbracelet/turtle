package find

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/glob"
	"charm.land/snail/fsx"
)

var testNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "default expression prints the hierarchy",
			args:  []string{"dir"},
			files: fsx.MemFS{"dir/a": {}, "dir/sub/b": {}},
			want:  "dir\ndir/a\ndir/sub\ndir/sub/b\n",
		},
		{
			name:  "no path defaults to the working directory",
			args:  []string{},
			files: fsx.MemFS{"a": {}},
			want:  ".\n./a\n",
		},
		{
			name:  "more than one path",
			args:  []string{"a", "b"},
			files: fsx.MemFS{"a/x": {}, "b/y": {}},
			want:  "a\na/x\nb\nb/y\n",
		},
		{
			name:  "trailing slash of a path operand is kept",
			args:  []string{"dir/"},
			files: fsx.MemFS{"dir/a": {}},
			want:  "dir/\ndir/a\n",
		},
		{
			name:     "missing path",
			args:     []string{"missing"},
			files:    fsx.MemFS{},
			wantErr:  "find: 'missing': file does not exist\n",
			wantCode: 1,
		},
		{
			name:     "paths must precede the expression",
			args:     []string{"-print"},
			wantErr:  "find: paths must precede expression: '-print'\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "unknown predicate",
			args:     []string{".", "-frob"},
			wantErr:  "find: unknown predicate '-frob'\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "missing argument to a primary",
			args:     []string{".", "-name"},
			wantErr:  "find: missing argument to '-name'\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "missing closing parenthesis",
			args:     []string{".", "("},
			wantErr:  "find: missing expression\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "unexpected closing parenthesis",
			args:     []string{".", "-print", ")"},
			wantErr:  "find: unexpected predicate ')'\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "operator with nothing before it",
			args:     []string{".", "-a", "-print"},
			wantErr:  "find: invalid expression; '-a' with nothing before it\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunName(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "star pattern",
			args:  []string{"dir", "-name", "*.txt"},
			files: fsx.MemFS{"dir/a.txt": {}, "dir/b": {}, "dir/sub/c.txt": {}},
			want:  "dir/a.txt\ndir/sub/c.txt\n",
		},
		{
			name:  "question mark",
			args:  []string{"dir", "-name", "a?c"},
			files: fsx.MemFS{"dir/abc": {}, "dir/ac": {}, "dir/abbc": {}},
			want:  "dir/abc\n",
		},
		{
			name:  "bracket expression",
			args:  []string{"dir", "-name", "[ab]*"},
			files: fsx.MemFS{"dir/a": {}, "dir/b": {}, "dir/c": {}},
			want:  "dir/a\ndir/b\n",
		},
		{
			name:  "pattern matches the whole basename",
			args:  []string{"dir", "-name", "b*"},
			files: fsx.MemFS{"dir/abc": {}, "dir/b": {}},
			want:  "dir/b\n",
		},
		{
			name:  "iname ignores case",
			args:  []string{"dir", "-iname", "A*"},
			files: fsx.MemFS{"dir/a": {}, "dir/B": {}},
			want:  "dir/a\n",
		},
		{
			name:  "dot is not special",
			args:  []string{"dir", "-name", ".*"},
			files: fsx.MemFS{"dir/.hidden": {}, "dir/a": {}},
			want:  "dir/.hidden\n",
		},
		{
			name:  "expression without an action prints",
			args:  []string{"dir", "-name", "a"},
			files: fsx.MemFS{"dir/a": {}, "dir/b": {}},
			want:  "dir/a\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunPath(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "star matches slashes",
			args:  []string{"dir", "-path", "*sub*"},
			files: fsx.MemFS{"dir/a": {}, "dir/sub/b": {}, "dir/sub/deep/c": {}},
			want:  "dir/sub\ndir/sub/b\ndir/sub/deep\ndir/sub/deep/c\n",
		},
		{
			name:  "whole path match",
			args:  []string{"dir", "-path", "dir/*/b"},
			files: fsx.MemFS{"dir/sub/b": {}, "dir/sub/deep/b": {}},
			want:  "dir/sub/b\ndir/sub/deep/b\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunType(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "directories",
			args:  []string{".", "-type", "d"},
			files: fsx.MemFS{"dir/a": {}, "dir/sub/b": {}},
			want:  ".\n./dir\n./dir/sub\n",
		},
		{
			name:  "regular files",
			args:  []string{".", "-type", "f"},
			files: fsx.MemFS{"dir/a": {}, "dir/sub/b": {}},
			want:  "./dir/a\n./dir/sub/b\n",
		},
		{
			name:     "unknown type",
			args:     []string{".", "-type", "z"},
			wantErr:  "find: unknown type 'z'\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunTypeSymlink(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "default doesn't follow links",
			args:  []string{"d", "-type", "f"},
			files: symlinkFS(t, "d/link", "a", fsx.MemFS{"d/a": {}}),
			want:  "d/a\n",
		},
		{
			name:  "default reports links as links",
			args:  []string{"d", "-type", "l"},
			files: symlinkFS(t, "d/link", "a", fsx.MemFS{"d/a": {}}),
			want:  "d/link\n",
		},
		{
			name:  "L follows links found during the traversal",
			args:  []string{"-L", "d", "-type", "f"},
			files: symlinkFS(t, "d/link", "a", fsx.MemFS{"d/a": {}}),
			want:  "d/a\nd/link\n",
		},
		{
			name:  "L follows a link to a directory",
			args:  []string{"-L", "d"},
			files: symlinkFS(t, "d/dirlink", "sub", fsx.MemFS{"d/sub/a": {}}),
			want:  "d\nd/dirlink\nd/dirlink/a\nd/sub\nd/sub/a\n",
		},
		{
			name:  "H follows only the link given as a path",
			args:  []string{"-H", "d/link"},
			files: symlinkFS(t, "d/link", "a", fsx.MemFS{"d/a": {}}),
			want:  "d/link\n",
		},
		{
			name:  "L follows the link given as a path",
			args:  []string{"-L", "d/link", "-type", "f"},
			files: symlinkFS(t, "d/link", "a", fsx.MemFS{"d/a": {}}),
			want:  "d/link\n",
		},
		{
			name:  "a broken link is described as itself",
			args:  []string{"-L", "d", "-type", "l"},
			files: symlinkFS(t, "d/link", "missing", fsx.MemFS{"d/a": {}}),
			want:  "d/link\n",
		},
		{
			name:  "P is the default",
			args:  []string{"-P", "d", "-type", "l"},
			files: symlinkFS(t, "d/link", "a", fsx.MemFS{"d/a": {}}),
			want:  "d/link\n",
		},
		{
			name:  "the last of H, L and P wins",
			args:  []string{"-H", "-P", "d", "-type", "l"},
			files: symlinkFS(t, "d/link", "a", fsx.MemFS{"d/a": {}}),
			want:  "d/link\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunPruneDepth(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "prune skips a directory",
			args:  []string{".", "-name", "sub", "-prune", "-o", "-print"},
			files: fsx.MemFS{"a": {}, "sub/b": {}, "sub/deep/c": {}},
			want:  ".\n./a\n",
		},
		{
			name:  "prune is true",
			args:  []string{".", "-name", "sub", "-prune"},
			files: fsx.MemFS{"a": {}, "sub/b": {}},
			want:  "./sub\n",
		},
		{
			name:  "depth acts on the entries first",
			args:  []string{".", "-depth"},
			files: fsx.MemFS{"a": {}, "sub/b": {}},
			want:  "./a\n./sub/b\n./sub\n.\n",
		},
		{
			name:  "depth applies even when not evaluated",
			args:  []string{".", "-depth", "-name", "b"},
			files: fsx.MemFS{"a": {}, "sub/b": {}},
			want:  "./sub/b\n",
		},
		{
			name:  "prune has no effect with depth",
			args:  []string{".", "-depth", "-name", "sub", "-prune", "-o", "-print"},
			files: fsx.MemFS{"a": {}, "sub/b": {}},
			want:  "./a\n./sub/b\n.\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunPerm(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "exact octal mode",
			args:  []string{".", "-perm", "0600"},
			files: fsx.MemFS{"a": {Mode: 0o600}, "b": {Mode: 0o640}},
			want:  "./a\n",
		},
		{
			name:  "any octal mode",
			args:  []string{".", "-perm", "-040"},
			files: fsx.MemFS{"a": {Mode: 0o640}, "b": {Mode: 0o600}},
			want:  ".\n./a\n",
		},
		{
			name:  "symbolic mode",
			args:  []string{".", "-perm", "u+w"},
			files: fsx.MemFS{"a": {Mode: 0o200}, "b": {Mode: 0o600}},
			want:  "./a\n",
		},
		{
			name:  "any symbolic mode",
			args:  []string{".", "-perm", "-u+w"},
			files: fsx.MemFS{"a": {Mode: 0o600}, "b": {Mode: 0o400}},
			want:  ".\n./a\n",
		},
		{
			name:  "special bits",
			args:  []string{".", "-perm", "-2000"},
			files: fsx.MemFS{"a": {Mode: 0o755 | fs.ModeSetgid}, "b": {Mode: 0o755}},
			want:  "./a\n",
		},
		{
			name:     "invalid mode",
			args:     []string{".", "-perm", "z"},
			wantErr:  "find: invalid mode 'z'\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunLinksUserGroup(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemFS
		gate     string
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "link count defaults to one",
			args:  []string{".", "-links", "1"},
			files: fsx.MemFS{"a": {}},
			want:  ".\n./a\n",
		},
		{
			name:  "numeric user",
			args:  []string{".", "-user", "4242"},
			files: fsx.MemFS{"a": {Uid: 4242}, "b": {Uid: 0}},
			want:  "./a\n",
		},
		{
			name:  "numeric group",
			args:  []string{".", "-group", "4242"},
			files: fsx.MemFS{"a": {Gid: 4242}, "b": {Gid: 0}},
			want:  "./a\n",
		},
		{
			name:     "unknown user",
			args:     []string{".", "-user", "nosuchuser"},
			wantErr:  "find: 'nosuchuser' is not the name of an existing user\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "unknown group",
			args:     []string{".", "-group", "nosuchgroup"},
			wantErr:  "find: 'nosuchgroup' is not the name of an existing group\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunNoUserNoGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("user and group databases work differently on Windows")
	}
	tests := []struct {
		name  string
		args  []string
		files fsx.MemFS
		want  string
	}{
		{
			name:  "nouser",
			args:  []string{".", "-nouser"},
			files: fsx.MemFS{"a": {Uid: 424242}, "b": {Uid: 0}},
			want:  "./a\n",
		},
		{
			name:  "nogroup",
			args:  []string{".", "-nogroup"},
			files: fsx.MemFS{"a": {Gid: 424242}, "b": {Gid: 0}},
			want:  "./a\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, "", 0)
		})
	}
}

func TestRunSize(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "512-byte blocks",
			args:  []string{".", "-size", "1"},
			files: fsx.MemFS{"a": {Data: []byte("1234")}, "b": {Data: []byte(strings.Repeat("x", 512))}, "c": {Data: []byte(strings.Repeat("y", 513))}},
			want:  "./a\n./b\n",
		},
		{
			name:  "bytes with c",
			args:  []string{".", "-size", "4c"},
			files: fsx.MemFS{"a": {Data: []byte("1234")}, "b": {Data: []byte("123")}},
			want:  "./a\n",
		},
		{
			name:  "more than",
			args:  []string{".", "-size", "+1c"},
			files: fsx.MemFS{"a": {Data: []byte("1234")}, "b": {Data: []byte("1")}},
			want:  "./a\n",
		},
		{
			name:  "less than",
			args:  []string{".", "-size", "-2c"},
			files: fsx.MemFS{"a": {Data: []byte("1234")}, "b": {Data: []byte("1")}},
			want:  ".\n./b\n",
		},
		{
			name:     "invalid size",
			args:     []string{".", "-size", "x"},
			wantErr:  "find: invalid argument 'x' to -size\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunTimes(t *testing.T) {
	files := fsx.MemFS{
		"a": {ModTime: testNow.Add(-48 * time.Hour)},
		"b": {ModTime: testNow.Add(-12 * time.Hour)},
	}
	tests := []struct {
		name     string
		args     []string
		want     string
		wantErr  string
		wantCode int
	}{
		{name: "exactly two days", args: []string{".", "-mtime", "2"}, want: "./a\n"},
		{name: "more than one day", args: []string{".", "-mtime", "+1"}, want: ".\n./a\n"},
		{name: "less than one day", args: []string{".", "-mtime", "-1"}, want: "./b\n"},
		{name: "atime falls back to mtime", args: []string{".", "-atime", "2"}, want: "./a\n"},
		{name: "ctime falls back to mtime", args: []string{".", "-ctime", "2"}, want: "./a\n"},
		{
			name:     "invalid argument",
			args:     []string{".", "-mtime", "x"},
			wantErr:  "find: invalid argument 'x' to -mtime\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunNewer(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "modified more recently",
			args:  []string{".", "-newer", "ref"},
			files: fsx.MemFS{"ref": {ModTime: testNow.Add(-48 * time.Hour)}, "a": {ModTime: testNow.Add(-24 * time.Hour)}, "b": {ModTime: testNow.Add(-72 * time.Hour)}},
			want:  "./a\n",
		},
		{
			name:     "missing reference file",
			args:     []string{".", "-newer", "missing"},
			files:    fsx.MemFS{"a": {}},
			wantErr:  "find: cannot stat 'missing': file does not exist\nTry 'find --help' for more information.\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunOperators(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "negation",
			args:  []string{".", "!", "-name", "b"},
			files: fsx.MemFS{"a": {}, "b": {}},
			want:  ".\n./a\n",
		},
		{
			name:  "explicit and",
			args:  []string{".", "-name", "a", "-a", "-type", "f"},
			files: fsx.MemFS{"a": {}, "b": {}},
			want:  "./a\n",
		},
		{
			name:  "or",
			args:  []string{".", "-name", "a", "-o", "-name", "b"},
			files: fsx.MemFS{"a": {}, "b": {}, "c": {}},
			want:  "./a\n./b\n",
		},
		{
			name:  "and binds tighter than or",
			args:  []string{".", "-name", "a", "-o", "-name", "b", "-a", "-type", "d"},
			files: fsx.MemFS{"a": {}, "b": {}, "dir/b": {}},
			want:  "./a\n",
		},
		{
			name:  "parentheses group",
			args:  []string{".", "(", "-name", "a", "-o", "-name", "b", ")", "-type", "f"},
			files: fsx.MemFS{"a": {}, "dir/b": {}},
			want:  "./a\n./dir/b\n",
		},
		{
			name:  "parentheses token",
			args:  []string{".", "(", "-name", "a", "-o", "-name", "b", ")", "-type", "f"},
			files: fsx.MemFS{"a": {}, "dir/b": {}},
			want:  "./a\n./dir/b\n",
		},
		{
			name:  "negated group",
			args:  []string{".", "!", "(", "-name", "a", "-o", "-name", "b", ")"},
			files: fsx.MemFS{"a": {}, "c": {}},
			want:  ".\n./c\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, test.files, nil, nil, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunExec(t *testing.T) {
	t.Run("one invocation per file", func(t *testing.T) {
		var calls []string
		files := fsx.MemFS{"a": {}, "b": {}}
		exec := func(ctx context.Context, dir, name string, args, env []string) (int, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return 0, nil
		}
		runTest(t, []string{".", "-type", "f", "-exec", "echo", "{}", ";"}, files, nil, exec, "", "", 0)
		want := []string{"echo ./a", "echo ./b"}
		if strings.Join(calls, "\x00") != strings.Join(want, "\x00") {
			t.Errorf("exec calls = %q, want %q", calls, want)
		}
	})

	t.Run("embedded braces are replaced", func(t *testing.T) {
		var calls []string
		files := fsx.MemFS{"a": {}}
		exec := func(ctx context.Context, dir, name string, args, env []string) (int, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return 0, nil
		}
		runTest(t, []string{".", "-type", "f", "-exec", "cp", "{}.src", "{}", ";"}, files, nil, exec, "", "", 0)
		if len(calls) != 1 || calls[0] != "cp ./a.src ./a" {
			t.Errorf("exec calls = %q, want [\"cp ./a.src ./a\"]", calls)
		}
	})

	t.Run("a nonzero status makes the primary false", func(t *testing.T) {
		files := fsx.MemFS{"a": {}}
		exec := func(ctx context.Context, dir, name string, args, env []string) (int, error) {
			return 1, nil
		}
		runTest(t, []string{".", "-exec", "false", "{}", ";"}, files, nil, exec, "", "", 0)
	})

	t.Run("aggregating form", func(t *testing.T) {
		var calls []string
		files := fsx.MemFS{"a": {}, "b": {}}
		exec := func(ctx context.Context, dir, name string, args, env []string) (int, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return 0, nil
		}
		runTest(t, []string{".", "-type", "f", "-exec", "echo", "{}", "+"}, files, nil, exec, "", "", 0)
		if len(calls) != 1 || calls[0] != "echo ./a ./b" {
			t.Errorf("exec calls = %q, want [\"echo ./a ./b\"]", calls)
		}
	})

	t.Run("aggregating form failure exits nonzero", func(t *testing.T) {
		files := fsx.MemFS{"a": {}}
		exec := func(ctx context.Context, dir, name string, args, env []string) (int, error) {
			return 1, nil
		}
		runTest(t, []string{".", "-exec", "false", "{}", "+"}, files, nil, exec, "", "", 1)
	})

	t.Run("no execution available", func(t *testing.T) {
		files := fsx.MemFS{"a": {}}
		runTest(t, []string{".", "-exec", "echo", "{}", ";"}, files, nil, nil, "",
			"find: no command execution available\nfind: no command execution available\n", 1)
	})

	t.Run("missing terminator", func(t *testing.T) {
		runTest(t, []string{".", "-exec", "echo"}, fsx.MemFS{}, nil, nil, "",
			"find: missing argument to '-exec'\nTry 'find --help' for more information.\n", 1)
	})
}

func TestRunOk(t *testing.T) {
	files := fsx.MemFS{"a": {}, "b": {}}

	t.Run("affirmative response runs the utility", func(t *testing.T) {
		var calls []string
		exec := func(ctx context.Context, dir, name string, args, env []string) (int, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return 0, nil
		}
		runTest(t, []string{".", "-type", "f", "-ok", "echo", "{}", ";"}, files, strings.NewReader("y\nn\n"), exec, "",
			"echo ./a ? echo ./b ? ", 0)
		if len(calls) != 1 || calls[0] != "echo ./a" {
			t.Errorf("exec calls = %q, want [\"echo ./a\"]", calls)
		}
	})

	t.Run("negative response skips the utility", func(t *testing.T) {
		stderr := runTestErr(t, []string{".", "-type", "f", "-ok", "echo", "{}", ";"}, files, strings.NewReader("n\n"), nil)
		if want := "echo ./a ? echo ./b ? "; !strings.Contains(stderr, want) {
			t.Errorf("stderr = %q, want it to hold %q", stderr, want)
		}
	})

	t.Run("ok doesn't support the plus terminator", func(t *testing.T) {
		runTest(t, []string{".", "-ok", "echo", "{}", "+"}, files, nil, nil, "",
			"find: '-ok' doesn't support the '+' terminator\nTry 'find --help' for more information.\n", 1)
	})
}

func TestRunPrint0(t *testing.T) {
	files := fsx.MemFS{"a": {}, "sub/b": {}}
	runTest(t, []string{".", "-print0"}, files, nil, nil, ".\x00./a\x00./sub\x00./sub/b\x00", "", 0)
}

func TestRunCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	prog := New(&coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     fsx.MemFS{"a": {}},
	})
	if err := prog.Run(ctx, "."); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
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
		want := "find (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}

func TestCompilePattern(t *testing.T) {
	tests := []struct {
		pattern string
		text    string
		want    bool
	}{
		{pattern: "a*", text: "abc", want: true},
		{pattern: "a*", text: "ba", want: false},
		{pattern: "*c", text: "abc", want: true},
		{pattern: "a?c", text: "abc", want: true},
		{pattern: "a?c", text: "ac", want: false},
		{pattern: "[a-c]x", text: "bx", want: true},
		{pattern: "[^a-c]x", text: "dx", want: true},
		{pattern: `a\*b`, text: "a*b", want: true},
		{pattern: `a\*b`, text: "axb", want: false},
		{pattern: "a.b", text: "axb", want: false},
		{pattern: "[", text: "[", want: true},
	}
	for _, test := range tests {
		re, err := glob.Compile(test.pattern, false)
		if err != nil {
			t.Fatalf("glob.Compile(%q) error = %v", test.pattern, err)
		}
		if got := re.MatchString(test.text); got != test.want {
			t.Errorf("glob.Compile(%q).Match(%q) = %v, want %v", test.pattern, test.text, got, test.want)
		}
	}
}

// symlinkFS returns files with a symbolic link added, failing the test when
// it can't be created.
func symlinkFS(t *testing.T, newName, oldName string, files fsx.MemFS) fsx.MemFS {
	t.Helper()
	if err := files.Symlink(oldName, newName); err != nil {
		t.Fatalf("Symlink(%q, %q) error = %v", oldName, newName, err)
	}
	return files
}

// runTest runs find with the given arguments and compares its output, its
// standard error and its exit status.
func runTest(t *testing.T, args []string, files fsx.MemFS, stdin io.Reader, exec coreutils.ExecFunc, want, wantErr string, wantCode int) {
	t.Helper()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	runtime := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		FS:     files,
		Clock:  func() time.Time { return testNow },
		Exec:   exec,
	}
	err := New(runtime).Run(t.Context(), args...)
	code := 0
	if exitErr, ok := errors.AsType[*coreutils.ExitError](err); ok {
		code = exitErr.Code
	} else if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if code != wantCode {
		t.Errorf("Run() exit code = %d, want %d", code, wantCode)
	}
	if got := stdout.String(); got != want {
		t.Errorf("Run() stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, wantErr)
	}
}

// runTestErr is runTest, returning the standard error for inspection.
func runTestErr(t *testing.T, args []string, files fsx.MemFS, stdin io.Reader, exec coreutils.ExecFunc) string {
	t.Helper()
	stderr := new(bytes.Buffer)
	runtime := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: io.Discard,
		Stderr: stderr,
		FS:     files,
		Clock:  func() time.Time { return testNow },
		Exec:   exec,
	}
	_ = New(runtime).Run(t.Context(), args...)
	return stderr.String()
}

func TestRunWorkingDir(t *testing.T) {
	stdout := new(bytes.Buffer)
	runtime := &coreutils.Runtime{
		Stdout:     stdout,
		Stderr:     io.Discard,
		FS:         fsx.MemFS{"base/dir/a": {}},
		WorkingDir: "base",
		Clock:      func() time.Time { return testNow },
	}
	if err := New(runtime).Run(t.Context(), "dir"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "dir\ndir/a\n"; got != want {
		t.Errorf("Run() stdout = %q, want %q", got, want)
	}
}

func TestRunMountXdev(t *testing.T) {
	// A memory file system reports no device IDs, so -mount and -xdev can
	// only be checked to be always true and to leave the traversal alone
	// here; cutting at a device boundary needs the system to report one.
	tests := []struct {
		name string
		args []string
	}{
		{name: "xdev is always true", args: []string{".", "-xdev"}},
		{name: "mount is always true", args: []string{".", "-mount"}},
		{name: "mount and xdev together", args: []string{".", "-mount", "-xdev"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test.args, fsx.MemFS{"a": {}, "sub/b": {}}, nil, nil,
				".\n./a\n./sub\n./sub/b\n", "", 0)
		})
	}
}

func TestRunLoopDetection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("loop detection needs the device and inode numbers")
	}
	dir := t.TempDir()
	if err := os.Symlink(dir, filepath.Join(dir, "loop")); err != nil {
		t.Skipf("Symlink() error = %v", err)
	}
	stderr := new(bytes.Buffer)
	runtime := &coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: stderr,
		FS:     fsx.OsFS(dir),
	}
	err := New(runtime).Run(t.Context(), "-L", ".")
	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok || exitErr.Code != 1 {
		t.Fatalf("Run() error = %v, want exit status 1", err)
	}
	if want := "file system loop detected"; !strings.Contains(stderr.String(), want) {
		t.Errorf("Run() stderr = %q, want it to hold %q", stderr.String(), want)
	}
}

func TestRunSelfReferentialSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink loops report no error on Windows")
	}
	dir := t.TempDir()
	if err := os.Symlink("self", filepath.Join(dir, "self")); err != nil {
		t.Skipf("Symlink() error = %v", err)
	}
	stderr := new(bytes.Buffer)
	runtime := &coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: stderr,
		FS:     fsx.OsFS(dir),
	}
	err := New(runtime).Run(t.Context(), "-L", ".")
	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok || exitErr.Code != 1 {
		t.Fatalf("Run() error = %v, want exit status 1", err)
	}
	if want := "find: './self': "; !strings.HasPrefix(stderr.String(), want) {
		t.Errorf("Run() stderr = %q, want it to start with %q", stderr.String(), want)
	}
}
