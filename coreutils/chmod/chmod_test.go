package chmod

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// dir returns the full mode of a directory with the given permission bits.
func dir(mode fs.FileMode) fs.FileMode {
	return fs.ModeDir | mode
}

type testCase struct {
	name     string
	files    fsx.MemFS
	umask    fs.FileMode
	args     []string
	want     map[string]fs.FileMode
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
		Umask:  test.umask,
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
		var fi fs.FileInfo
		var err error
		if want.Type()&fs.ModeSymlink != 0 {
			fi, err = test.files.Lstat(name)
		} else {
			fi, err = fs.Stat(test.files, name)
		}
		if err != nil {
			t.Errorf("Stat(%q) error = %v", name, err)
			continue
		}
		if got := fi.Mode(); got != want {
			t.Errorf("%s mode = %v, want %v", name, got, want)
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

func TestRunOctal(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "sets an absolute mode",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"755", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o755},
		},
		{
			name:  "sets the mode of every file",
			files: fsx.MemFS{"a.txt": {Mode: 0o600}, "b.txt": {Mode: 0o600}},
			args:  []string{"644", "a.txt", "b.txt"},
			want:  map[string]fs.FileMode{"a.txt": 0o644, "b.txt": 0o644},
		},
		{
			name:  "a leading zero is valid",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"0755", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o755},
		},
		{
			name:  "a single digit sets the other bits",
			files: fsx.MemFS{"file.txt": {Mode: 0o777}},
			args:  []string{"4", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o004},
		},
		{
			name:  "clears every bit",
			files: fsx.MemFS{"file.txt": {Mode: 0o755 | fs.ModeSetuid}},
			args:  []string{"0", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0},
		},
		{
			name:  "sets the set-user-ID bit",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"4755", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o755 | fs.ModeSetuid},
		},
		{
			name:  "sets the set-group-ID bit",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"2750", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o750 | fs.ModeSetgid},
		},
		{
			name:  "sets the sticky bit",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"1755", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o755 | fs.ModeSticky},
		},
		{
			name:  "clears the special bits that are not given",
			files: fsx.MemFS{"file.txt": {Mode: 0o755 | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky}},
			args:  []string{"755", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o755},
		},
		{
			name:  "changes a directory",
			files: fsx.MemFS{"dir": {Mode: dir(0o755)}},
			args:  []string{"700", "dir"},
			want:  map[string]fs.FileMode{"dir": dir(0o700)},
		},
	})
}

func TestRunSymbolic(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "sets the bits of one class",
			files: fsx.MemFS{"file.txt": {Mode: 0}},
			args:  []string{"u+x", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o100},
		},
		{
			name:  "clears the bits of the given classes",
			files: fsx.MemFS{"file.txt": {Mode: 0o666}},
			args:  []string{"go-w", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			name:  "a means every class",
			files: fsx.MemFS{"file.txt": {Mode: 0}},
			args:  []string{"a+rw", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o666},
		},
		{
			name:  "a wholist can join classes",
			files: fsx.MemFS{"file.txt": {Mode: 0}},
			args:  []string{"ug+rw", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o660},
		},
		{
			name:  "a clause can have multiple actions",
			files: fsx.MemFS{"file.txt": {Mode: 0o777}},
			args:  []string{"u+x-w", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o577},
		},
		{
			name:  "multiple clauses are applied in order",
			files: fsx.MemFS{"file.txt": {Mode: 0o777}},
			args:  []string{"u=rwx,g=rx,o=", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o750},
		},
		{
			name:  "equals clears the selected classes first",
			files: fsx.MemFS{"file.txt": {Mode: 0o777}},
			args:  []string{"u=rw", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o677},
		},
		{
			name:  "equals with no perm clears the class",
			files: fsx.MemFS{"file.txt": {Mode: 0o777}},
			args:  []string{"u=", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o077},
		},
		{
			name:  "a with no perm clears every class",
			files: fsx.MemFS{"file.txt": {Mode: 0o777}},
			args:  []string{"a=", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0},
		},
		{
			name:  "an action with no perm changes nothing",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"u+", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			name:  "bits are relative to the mode of each file",
			files: fsx.MemFS{"a.txt": {Mode: 0o600}, "b.txt": {Mode: 0o060}},
			args:  []string{"u+x", "a.txt", "b.txt"},
			want:  map[string]fs.FileMode{"a.txt": 0o700, "b.txt": 0o160},
		},
		{
			name:  "changes a directory",
			files: fsx.MemFS{"dir": {Mode: dir(0o775)}},
			args:  []string{"g-w", "dir"},
			want:  map[string]fs.FileMode{"dir": dir(0o755)},
		},
	})
}

func TestRunSymbolicWithoutWho(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "sets the bits of every class",
			files: fsx.MemFS{"file.txt": {Mode: 0}},
			args:  []string{"+x", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o111},
		},
		{
			name:  "the umask filters the bits that are set",
			files: fsx.MemFS{"file.txt": {Mode: 0}},
			umask: 0o022,
			args:  []string{"+w", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o200},
		},
		{
			name:  "the umask filters the bits that are cleared",
			files: fsx.MemFS{"file.txt": {Mode: 0o666}},
			umask: 0o022,
			args:  []string{"--", "-w", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o466},
		},
		{
			name:  "the umask filters the bits that equals sets",
			files: fsx.MemFS{"file.txt": {Mode: 0o777}},
			umask: 0o022,
			args:  []string{"=rw", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			name:  "a who is not filtered by the umask",
			files: fsx.MemFS{"file.txt": {Mode: 0}},
			umask: 0o022,
			args:  []string{"a+rw", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o666},
		},
		{
			name:  "the special bits are not filtered by the umask",
			files: fsx.MemFS{"file.txt": {Mode: 0}},
			umask: 0o077,
			args:  []string{"+s", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": fs.ModeSetuid | fs.ModeSetgid},
		},
	})
}

func TestRunSymbolicX(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "X sets the execute bits of a directory",
			files: fsx.MemFS{"dir": {Mode: dir(0o644)}},
			args:  []string{"a+X", "dir"},
			want:  map[string]fs.FileMode{"dir": dir(0o755)},
		},
		{
			name:  "X sets the execute bits of a file with an execute bit",
			files: fsx.MemFS{"file.txt": {Mode: 0o741}},
			args:  []string{"a+X", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o751},
		},
		{
			name:  "X is ignored on a file without execute bits",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"a+X", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			name:  "X clears the execute bits of a file with an execute bit",
			files: fsx.MemFS{"file.txt": {Mode: 0o755}},
			args:  []string{"a-X", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			name:  "X is ignored when clearing on a file without execute bits",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"a-X", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
	})
}

func TestRunSymbolicSpecial(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "u+s sets the set-user-ID bit",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"u+s", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644 | fs.ModeSetuid},
		},
		{
			name:  "g+s sets the set-group-ID bit",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"g+s", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644 | fs.ModeSetgid},
		},
		{
			name:  "o+s changes nothing",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"o+s", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			name:  "go+s sets only the set-group-ID bit",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"go+s", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644 | fs.ModeSetgid},
		},
		{
			name:  "s without a who sets both bits",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"+s", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644 | fs.ModeSetuid | fs.ModeSetgid},
		},
		{
			name:  "u-s clears the set-user-ID bit",
			files: fsx.MemFS{"file.txt": {Mode: 0o644 | fs.ModeSetuid}},
			args:  []string{"u-s", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			name:  "+t sets the sticky bit",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"+t", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644 | fs.ModeSticky},
		},
		{
			name:  "a-t clears the sticky bit",
			files: fsx.MemFS{"file.txt": {Mode: 0o644 | fs.ModeSticky}},
			args:  []string{"a-t", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			name:  "equals without a who clears the special bits",
			files: fsx.MemFS{"file.txt": {Mode: 0o755 | fs.ModeSetuid}},
			umask: 0o022,
			args:  []string{"=rw", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			name:  "equals keeps the special bits of other classes",
			files: fsx.MemFS{"file.txt": {Mode: 0o755 | fs.ModeSetuid}},
			args:  []string{"u=rw", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o655 | fs.ModeSetuid},
		},
	})
}

func TestRunSymbolicCopy(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "g=u copies the user permissions",
			files: fsx.MemFS{"file.txt": {Mode: 0o741}},
			args:  []string{"g=u", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o771},
		},
		{
			name:  "o=u copies the user permissions",
			files: fsx.MemFS{"file.txt": {Mode: 0o750}},
			args:  []string{"o=u", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o757},
		},
		{
			name:  "a permcopy with no who applies to every class",
			files: fsx.MemFS{"file.txt": {Mode: 0o640}},
			args:  []string{"=u", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o666},
		},
		{
			name:  "actions are applied in order, like o=u-g from the spec",
			files: fsx.MemFS{"file.txt": {Mode: 0o750}},
			args:  []string{"o=u-g", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o752},
		},
		{
			name:  "uo=g matches the spec example",
			files: fsx.MemFS{"file.txt": {Mode: 0o641}},
			args:  []string{"uo=g", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o444},
		},
		{
			name:  "the permissions are copied from each file",
			files: fsx.MemFS{"a.txt": {Mode: 0o600}, "b.txt": {Mode: 0o060}},
			args:  []string{"g=u", "a.txt", "b.txt"},
			want:  map[string]fs.FileMode{"a.txt": 0o660, "b.txt": 0},
		},
	})
}

func TestRunInvalidModes(t *testing.T) {
	tests := []testCase{
		{name: "an invalid octal digit", args: []string{"888", "file.txt"}},
		{name: "too many digits", args: []string{"12345", "file.txt"}},
		{name: "octal and symbolic mixed", args: []string{"77+x", "file.txt"}},
		{name: "an unknown perm letter", args: []string{"q+x", "file.txt"}},
		{name: "a who without actions", args: []string{"u", "file.txt"}},
		{name: "an empty clause", args: []string{"u+x,,g+w", "file.txt"}},
		{name: "a permcopy followed by perms", args: []string{"o=ug", "file.txt"}},
		{name: "a missing op", args: []string{"ur", "file.txt"}},
		{name: "a perm without an op", args: []string{"rw", "file.txt"}},
		{name: "an empty mode", args: []string{"", "file.txt"}},
	}
	for i := range tests {
		tests[i].files = fsx.MemFS{"file.txt": {Mode: 0o644}}
		tests[i].want = map[string]fs.FileMode{"file.txt": 0o644}
		tests[i].wantErr = "chmod: invalid mode: \"" + tests[i].args[0] + "\"\nTry 'chmod --help' for more information.\n"
		tests[i].wantExit = 1
	}
	runTests(t, tests)
}

func TestRunRecursive(t *testing.T) {
	tree := func() fsx.MemFS {
		return fsx.MemFS{
			"dir":           {Mode: dir(0o755)},
			"dir/a.txt":     {Mode: 0o644},
			"dir/sub":       {Mode: dir(0o755)},
			"dir/sub/b.txt": {Mode: 0o600},
		}
	}

	runTests(t, []testCase{
		{
			name:  "R changes every file below the directory",
			files: tree(),
			args:  []string{"-R", "700", "dir"},
			want: map[string]fs.FileMode{
				"dir":           dir(0o700),
				"dir/a.txt":     0o700,
				"dir/sub":       dir(0o700),
				"dir/sub/b.txt": 0o700,
			},
		},
		{
			name:  "R on a plain file works like without R",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"-R", "600", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o600},
		},
		{
			name:  "R with a symbolic mode",
			files: tree(),
			args:  []string{"-R", "go-w", "dir"},
			want: map[string]fs.FileMode{
				"dir":           dir(0o755),
				"dir/a.txt":     0o644,
				"dir/sub":       dir(0o755),
				"dir/sub/b.txt": 0o600,
			},
		},
		{
			name:  "R with X only touches files that have an execute bit",
			files: tree(),
			args:  []string{"-R", "a+X", "dir"},
			want: map[string]fs.FileMode{
				"dir":           dir(0o755),
				"dir/a.txt":     0o644,
				"dir/sub":       dir(0o755),
				"dir/sub/b.txt": 0o600,
			},
		},
		{
			name:  "R with X adds the execute bits to directories",
			files: fsx.MemFS{"dir": {Mode: dir(0o644)}, "dir/a.txt": {Mode: 0o744}},
			args:  []string{"-R", "a+X", "dir"},
			want: map[string]fs.FileMode{
				"dir":       dir(0o755),
				"dir/a.txt": 0o755,
			},
		},
		{
			name:     "R keeps going after a missing file",
			files:    fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:     []string{"-R", "700", "missing", "file.txt"},
			want:     map[string]fs.FileMode{"file.txt": 0o700},
			wantErr:  "chmod: missing: file does not exist\n",
			wantExit: 1,
		},
	})
}

// symlink is a symbolic link to a file of the tree.
func symlink(target string) *fsx.MemFile {
	return &fsx.MemFile{Data: []byte(target), Mode: fs.ModeSymlink | 0o777}
}

func TestRunLongOptions(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "recursive changes every file below the directory",
			files: fsx.MemFS{"dir": {Mode: dir(0o755)}, "dir/a.txt": {Mode: 0o644}},
			args:  []string{"--recursive", "700", "dir"},
			want: map[string]fs.FileMode{
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
			},
		},
	})
}

func TestRunGeneral(t *testing.T) {
	runTests(t, []testCase{
		{
			name:     "a missing operand is an error",
			files:    fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:     nil,
			want:     map[string]fs.FileMode{"file.txt": 0o644},
			wantErr:  "chmod: missing operand\nTry 'chmod --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a missing file operand is an error",
			files:    fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:     []string{"755"},
			want:     map[string]fs.FileMode{"file.txt": 0o644},
			wantErr:  "chmod: missing file operand\nTry 'chmod --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an invalid option is an error",
			files:    fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:     []string{"-x", "755", "file.txt"},
			want:     map[string]fs.FileMode{"file.txt": 0o644},
			wantErr:  "chmod: invalid option -- 'x'\nTry 'chmod --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option",
			files:    fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:     []string{"--bogus", "755", "file.txt"},
			want:     map[string]fs.FileMode{"file.txt": 0o644},
			wantErr:  "chmod: unrecognized option '--bogus'\nTry 'chmod --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:  "a mode starting with a dash can follow a double dash",
			files: fsx.MemFS{"file.txt": {Mode: 0o666}},
			args:  []string{"--", "-w", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o444},
		},
		{
			name:  "a file named like an option can follow a double dash",
			files: fsx.MemFS{"-w": {Mode: 0o644}},
			args:  []string{"--", "600", "-w"},
			want:  map[string]fs.FileMode{"-w": 0o600},
		},
		{
			name:     "a missing file is an error, but the others are changed",
			files:    fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:     []string{"755", "missing", "file.txt"},
			want:     map[string]fs.FileMode{"file.txt": 0o755},
			wantErr:  "chmod: missing: file does not exist\n",
			wantExit: 1,
		},
	})
}

func TestRunReference(t *testing.T) {
	runTests(t, []testCase{
		{
			name:  "uses the mode of the reference file",
			files: fsx.MemFS{"ref": {Mode: 0o640}, "file.txt": {Mode: 0o600}},
			args:  []string{"--reference=ref", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o640},
		},
		{
			name:  "the reference can be a separate argument",
			files: fsx.MemFS{"ref": {Mode: 0o640}, "file.txt": {Mode: 0o600}},
			args:  []string{"--reference", "ref", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o640},
		},
		{
			name:  "the mode applies to every file",
			files: fsx.MemFS{"ref": {Mode: 0o600}, "a.txt": {Mode: 0o644}, "b.txt": {Mode: 0o755}},
			args:  []string{"--reference=ref", "a.txt", "b.txt"},
			want:  map[string]fs.FileMode{"a.txt": 0o600, "b.txt": 0o600},
		},
		{
			name:  "the special bits are copied",
			files: fsx.MemFS{"ref": {Mode: 0o755 | fs.ModeSetuid | fs.ModeSticky}, "file.txt": {Mode: 0o644}},
			args:  []string{"--reference=ref", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o755 | fs.ModeSetuid | fs.ModeSticky},
		},
		{
			name:  "a reference to a symbolic link is dereferenced",
			files: fsx.MemFS{"ref": {Mode: 0o640}, "link": symlink("ref"), "file.txt": {Mode: 0o600}},
			args:  []string{"--reference=link", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o640},
		},
		{
			name:  "a reference can be combined with R",
			files: fsx.MemFS{"ref": {Mode: 0o700}, "dir": {Mode: dir(0o755)}, "dir/a.txt": {Mode: 0o644}},
			args:  []string{"-R", "--reference=ref", "dir"},
			want: map[string]fs.FileMode{
				"ref":       0o700,
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
			},
		},
		{
			// With --reference, every operand is a file: a mode-looking
			// name is just a missing file.
			name:     "a mode-looking operand is a file with --reference",
			files:    fsx.MemFS{"ref": {Mode: 0o640}, "file.txt": {Mode: 0o644}},
			args:     []string{"--reference=ref", "644", "file.txt"},
			want:     map[string]fs.FileMode{"ref": 0o640, "file.txt": 0o640},
			wantErr:  "chmod: 644: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing reference file is an error",
			files:    fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:     []string{"--reference=missing", "file.txt"},
			want:     map[string]fs.FileMode{"file.txt": 0o644},
			wantErr:  "chmod: failed to get attributes of missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "no operands is a usage error",
			files:    fsx.MemFS{"ref": {Mode: 0o640}},
			args:     []string{"--reference=ref"},
			want:     map[string]fs.FileMode{"ref": 0o640},
			wantErr:  "chmod: missing operand\nTry 'chmod --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:  "a lone operand is a file, not a mode",
			files: fsx.MemFS{"ref": {Mode: 0o640}, "u+x": {Mode: 0o600}},
			args:  []string{"--reference=ref", "u+x"},
			want:  map[string]fs.FileMode{"u+x": 0o640},
		},
		{
			name:  "--reference works with the mode-like file x+y", // sanity: non-mode operand
			files: fsx.MemFS{"ref": {Mode: 0o640}, "x+y": {Mode: 0o600}},
			args:  []string{"--reference=ref", "x+y"},
			want:  map[string]fs.FileMode{"x+y": 0o640},
		},
	})
}

func TestRunVerbose(t *testing.T) {
	runTests(t, []testCase{
		{
			name:    "v reports a change",
			files:   fsx.MemFS{"file.txt": {Mode: 0o600}},
			args:    []string{"-v", "644", "file.txt"},
			want:    map[string]fs.FileMode{"file.txt": 0o644},
			wantOut: "mode of 'file.txt' changed from 0600 (rw-------) to 0644 (rw-r--r--)\n",
		},
		{
			name:    "v reports a retained mode",
			files:   fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:    []string{"-v", "644", "file.txt"},
			want:    map[string]fs.FileMode{"file.txt": 0o644},
			wantOut: "mode of 'file.txt' retained as 0644 (rw-r--r--)\n",
		},
		{
			name:    "c reports only the files whose mode changed",
			files:   fsx.MemFS{"a.txt": {Mode: 0o600}, "b.txt": {Mode: 0o644}},
			args:    []string{"-c", "644", "a.txt", "b.txt"},
			want:    map[string]fs.FileMode{"a.txt": 0o644, "b.txt": 0o644},
			wantOut: "mode of 'a.txt' changed from 0600 (rw-------) to 0644 (rw-r--r--)\n",
		},
		{
			name:  "c reports nothing when no mode changes",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"-c", "644", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			name:  "without c or v nothing is reported",
			files: fsx.MemFS{"file.txt": {Mode: 0o600}},
			args:  []string{"644", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			name:    "the last of c and v wins",
			files:   fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:    []string{"-c", "-v", "644", "file.txt"},
			want:    map[string]fs.FileMode{"file.txt": 0o644},
			wantOut: "mode of 'file.txt' retained as 0644 (rw-r--r--)\n",
		},
		{
			name:  "the last of v and c wins",
			files: fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:  []string{"-v", "-c", "644", "file.txt"},
			want:  map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			name:    "the long options work",
			files:   fsx.MemFS{"file.txt": {Mode: 0o600}},
			args:    []string{"--changes", "644", "file.txt"},
			want:    map[string]fs.FileMode{"file.txt": 0o644},
			wantOut: "mode of 'file.txt' changed from 0600 (rw-------) to 0644 (rw-r--r--)\n",
		},
		{
			name:    "v reports the special bits like ls does",
			files:   fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:    []string{"-v", "4755", "file.txt"},
			want:    map[string]fs.FileMode{"file.txt": 0o755 | fs.ModeSetuid},
			wantOut: "mode of 'file.txt' changed from 0644 (rw-r--r--) to 4755 (rwsr-xr-x)\n",
		},
		{
			name:    "v reports the special bits without execute like ls does",
			files:   fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:    []string{"-v", "2644", "file.txt"},
			want:    map[string]fs.FileMode{"file.txt": 0o644 | fs.ModeSetgid},
			wantOut: "mode of 'file.txt' changed from 0644 (rw-r--r--) to 2644 (rw-r-Sr--)\n",
		},
		{
			name:    "v reports the sticky bit like ls does",
			files:   fsx.MemFS{"dir": {Mode: dir(0o755)}},
			args:    []string{"-v", "1755", "dir"},
			want:    map[string]fs.FileMode{"dir": dir(0o755) | fs.ModeSticky},
			wantOut: "mode of 'dir' changed from 0755 (rwxr-xr-x) to 1755 (rwxr-xr-t)\n",
		},
		{
			name:    "v reports the operand as given",
			files:   fsx.MemFS{"file.txt": {Mode: 0o600}},
			args:    []string{"-v", "644", "./file.txt"},
			want:    map[string]fs.FileMode{"file.txt": 0o644},
			wantOut: "mode of './file.txt' changed from 0600 (rw-------) to 0644 (rw-r--r--)\n",
		},
		{
			name:    "v with a symbolic mode reports the resulting mode",
			files:   fsx.MemFS{"file.txt": {Mode: 0o600}},
			args:    []string{"-v", "g+r,o+r", "file.txt"},
			want:    map[string]fs.FileMode{"file.txt": 0o644},
			wantOut: "mode of 'file.txt' changed from 0600 (rw-------) to 0644 (rw-r--r--)\n",
		},
		{
			name:    "v with --reference reports the change",
			files:   fsx.MemFS{"ref": {Mode: 0o640}, "file.txt": {Mode: 0o600}},
			args:    []string{"-v", "--reference=ref", "file.txt"},
			want:    map[string]fs.FileMode{"file.txt": 0o640},
			wantOut: "mode of 'file.txt' changed from 0600 (rw-------) to 0640 (rw-r-----)\n",
		},
	})
}

func TestRunVerboseRecursive(t *testing.T) {
	tree := func() fsx.MemFS {
		return fsx.MemFS{
			"dir":           {Mode: dir(0o755)},
			"dir/a.txt":     {Mode: 0o644},
			"dir/sub":       {Mode: dir(0o755)},
			"dir/sub/b.txt": {Mode: 0o600},
		}
	}

	runTests(t, []testCase{
		{
			name:  "v reports every file of the walk",
			files: tree(),
			args:  []string{"-Rv", "700", "dir"},
			want: map[string]fs.FileMode{
				"dir":           dir(0o700),
				"dir/a.txt":     0o700,
				"dir/sub":       dir(0o700),
				"dir/sub/b.txt": 0o700,
			},
			wantOut: "mode of 'dir' changed from 0755 (rwxr-xr-x) to 0700 (rwx------)\n" +
				"mode of 'dir/a.txt' changed from 0644 (rw-r--r--) to 0700 (rwx------)\n" +
				"mode of 'dir/sub' changed from 0755 (rwxr-xr-x) to 0700 (rwx------)\n" +
				"mode of 'dir/sub/b.txt' changed from 0600 (rw-------) to 0700 (rwx------)\n",
		},
		{
			name:  "c reports only the files whose mode changed",
			files: tree(),
			args:  []string{"-Rc", "a+X", "dir"},
			want: map[string]fs.FileMode{
				"dir":           dir(0o755),
				"dir/a.txt":     0o644,
				"dir/sub":       dir(0o755),
				"dir/sub/b.txt": 0o600,
			},
		},
	})
}

func TestRunRecursiveSymlinks(t *testing.T) {
	runTests(t, []testCase{
		{
			name: "R ignores a symbolic link of the walk",
			files: fsx.MemFS{
				"dir":       {Mode: dir(0o755)},
				"dir/a.txt": {Mode: 0o644},
				"dir/link":  symlink("a.txt"),
			},
			args: []string{"-R", "700", "dir"},
			want: map[string]fs.FileMode{
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
				"dir/link":  fs.ModeSymlink | 0o777,
			},
		},
		{
			name: "R ignores a dangling symbolic link",
			files: fsx.MemFS{
				"dir":       {Mode: dir(0o755)},
				"dir/a.txt": {Mode: 0o644},
				"dir/link":  symlink("missing"),
			},
			args: []string{"-R", "700", "dir"},
			want: map[string]fs.FileMode{
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
				"dir/link":  fs.ModeSymlink | 0o777,
			},
		},
		{
			name: "R does not act on what a link of the walk points at",
			files: fsx.MemFS{
				"dir":      {Mode: dir(0o755)},
				"dir/link": symlink("target"),
				"target":   {Mode: 0o600},
			},
			args: []string{"-R", "700", "dir"},
			want: map[string]fs.FileMode{
				"dir":      dir(0o700),
				"dir/link": fs.ModeSymlink | 0o777,
				"target":   0o600,
			},
		},
		{
			name: "R follows a symbolic link operand to a directory, like -H",
			files: fsx.MemFS{
				"dir":       {Mode: dir(0o755)},
				"dir/a.txt": {Mode: 0o644},
				"dlink":     symlink("dir"),
			},
			args: []string{"-R", "700", "dlink"},
			want: map[string]fs.FileMode{
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
				"dlink":     fs.ModeSymlink | 0o777,
			},
		},
		{
			name: "R does not follow a symbolic link to a directory",
			files: fsx.MemFS{
				"dir":       {Mode: dir(0o755)},
				"dir/a.txt": {Mode: 0o644},
				"dir/sub":   {Mode: dir(0o755)},
				"dir/link":  symlink("sub"),
			},
			args: []string{"-R", "700", "dir"},
			want: map[string]fs.FileMode{
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
				"dir/sub":   dir(0o700),
				"dir/link":  fs.ModeSymlink | 0o777,
			},
		},
		{
			name: "v does not report the links ignored in the walk",
			files: fsx.MemFS{
				"dir":       {Mode: dir(0o755)},
				"dir/a.txt": {Mode: 0o644},
				"dir/link":  symlink("a.txt"),
			},
			args: []string{"-Rv", "700", "dir"},
			want: map[string]fs.FileMode{
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
				"dir/link":  fs.ModeSymlink | 0o777,
			},
			wantOut: "mode of 'dir' changed from 0755 (rwxr-xr-x) to 0700 (rwx------)\n" +
				"mode of 'dir/a.txt' changed from 0644 (rw-r--r--) to 0700 (rwx------)\n",
		},
	})
}

func TestRunSymlinksOsFS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
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
	must(os.Symlink("a.txt", filepath.Join(dir, "sub", "link.txt")))

	// The mode a symbolic link shows is platform-specific, so the links
	// are expected to keep whatever mode they were created with.
	linkMode := func(name string) fs.FileMode {
		fi, err := os.Lstat(filepath.Join(dir, name))
		must(err)
		return fi.Mode()
	}
	link, subLink := linkMode("link.txt"), linkMode("sub/link.txt")

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: stderr,
		FS:     fsx.OsFS(dir),
	}
	if err := New(rt).Run(t.Context(), "-R", "700", "."); err != nil {
		t.Errorf("Run() error = %v, stderr = %q", err, stderr)
	}

	want := map[string]fs.FileMode{
		".":            fs.ModeDir | 0o700,
		"a.txt":        0o700,
		"sub":          fs.ModeDir | 0o700,
		"sub/b.txt":    0o700,
		"link.txt":     link,
		"sub/link.txt": subLink,
	}
	for name, mode := range want {
		fi, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("Lstat(%q) error = %v", name, err)
			continue
		}
		if got := fi.Mode(); got != mode {
			t.Errorf("%s mode = %v, want %v", name, got, mode)
		}
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
		want := "chmod (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
