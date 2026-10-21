package chmod

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
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
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: stderr,
		FS:     test.files,
		Umask:  test.umask,
	}

	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}

	for name, want := range test.want {
		fi, err := fs.Stat(test.files, name)
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
		tests[i].wantErr = "chmod: invalid mode: \"" + tests[i].args[0] + "\"\n"
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
			wantErr:  "chmod: missing operand\n",
			wantExit: 2,
		},
		{
			name:     "a missing file operand is an error",
			files:    fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:     []string{"755"},
			want:     map[string]fs.FileMode{"file.txt": 0o644},
			wantErr:  "chmod: missing file operand\n",
			wantExit: 2,
		},
		{
			name:     "an invalid option is an error",
			files:    fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:     []string{"-x", "755", "file.txt"},
			want:     map[string]fs.FileMode{"file.txt": 0o644},
			wantErr:  "chmod: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option",
			files:    fsx.MemFS{"file.txt": {Mode: 0o644}},
			args:     []string{"--bogus", "755", "file.txt"},
			want:     map[string]fs.FileMode{"file.txt": 0o644},
			wantErr:  "chmod: unrecognized option '--bogus'\n",
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
