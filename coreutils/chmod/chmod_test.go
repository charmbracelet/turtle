package chmod

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

// dir returns the full mode of a directory with the given permission bits.
func dir(mode fs.FileMode) fs.FileMode {
	return fs.ModeDir | mode
}

// testCase is the fields of a testutil.Case plus the file fixtures of the
// tests, and the modes the files must have after the run.
type testCase struct {
	Name     string
	Args     []string
	Want     string
	WantErr  string
	WantExit int
	Umask    fs.FileMode

	Files     fsx.MemFS
	WantModes map[string]fs.FileMode
}

// runCase runs a single case, checking the modes of its files. testutil.Case
// can't replace this one: its mode assertion always stats, while the wanted
// modes of symbolic links are checked with Lstat.
func runCase(t *testing.T, test testCase) {
	t.Helper()

	c := testutil.Case{
		Name:     test.Name,
		Args:     test.Args,
		Want:     test.Want,
		WantErr:  test.WantErr,
		WantExit: test.WantExit,
		Umask:    test.Umask,
		FS:       test.Files,
	}
	wantModes := test.WantModes
	c.Check = func(t *testing.T, rt *coreutils.Runtime) {
		for name, want := range wantModes {
			var fi fs.FileInfo
			var err error
			if want.Type()&fs.ModeSymlink != 0 {
				fi, err = rt.FS.Lstat(name)
			} else {
				fi, err = fs.Stat(rt.FS, name)
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

func TestRunOctal(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:      "sets an absolute mode",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"755", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o755},
		},
		{
			Name:      "sets the mode of every file",
			Files:     fsx.MemFS{"a.txt": {Mode: 0o600}, "b.txt": {Mode: 0o600}},
			Args:      []string{"644", "a.txt", "b.txt"},
			WantModes: map[string]fs.FileMode{"a.txt": 0o644, "b.txt": 0o644},
		},
		{
			Name:      "a leading zero is valid",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"0755", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o755},
		},
		{
			Name:      "a single digit sets the other bits",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o777}},
			Args:      []string{"4", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o004},
		},
		{
			Name:      "clears every bit",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o755 | fs.ModeSetuid}},
			Args:      []string{"0", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0},
		},
		{
			Name:      "sets the set-user-ID bit",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"4755", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o755 | fs.ModeSetuid},
		},
		{
			Name:      "sets the set-group-ID bit",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"2750", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o750 | fs.ModeSetgid},
		},
		{
			Name:      "sets the sticky bit",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"1755", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o755 | fs.ModeSticky},
		},
		{
			Name:      "clears the special bits that are not given",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o755 | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky}},
			Args:      []string{"755", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o755},
		},
		{
			Name:      "changes a directory",
			Files:     fsx.MemFS{"dir": {Mode: dir(0o755)}},
			Args:      []string{"700", "dir"},
			WantModes: map[string]fs.FileMode{"dir": dir(0o700)},
		},
	})
}

func TestRunSymbolic(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:      "sets the bits of one class",
			Files:     fsx.MemFS{"file.txt": {Mode: 0}},
			Args:      []string{"u+x", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o100},
		},
		{
			Name:      "clears the bits of the given classes",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o666}},
			Args:      []string{"go-w", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			Name:      "a means every class",
			Files:     fsx.MemFS{"file.txt": {Mode: 0}},
			Args:      []string{"a+rw", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o666},
		},
		{
			Name:      "a wholist can join classes",
			Files:     fsx.MemFS{"file.txt": {Mode: 0}},
			Args:      []string{"ug+rw", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o660},
		},
		{
			Name:      "a clause can have multiple actions",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o777}},
			Args:      []string{"u+x-w", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o577},
		},
		{
			Name:      "multiple clauses are applied in order",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o777}},
			Args:      []string{"u=rwx,g=rx,o=", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o750},
		},
		{
			Name:      "equals clears the selected classes first",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o777}},
			Args:      []string{"u=rw", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o677},
		},
		{
			Name:      "equals with no perm clears the class",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o777}},
			Args:      []string{"u=", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o077},
		},
		{
			Name:      "a with no perm clears every class",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o777}},
			Args:      []string{"a=", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0},
		},
		{
			Name:      "an action with no perm changes nothing",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"u+", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			Name:      "bits are relative to the mode of each file",
			Files:     fsx.MemFS{"a.txt": {Mode: 0o600}, "b.txt": {Mode: 0o060}},
			Args:      []string{"u+x", "a.txt", "b.txt"},
			WantModes: map[string]fs.FileMode{"a.txt": 0o700, "b.txt": 0o160},
		},
		{
			Name:      "changes a directory",
			Files:     fsx.MemFS{"dir": {Mode: dir(0o775)}},
			Args:      []string{"g-w", "dir"},
			WantModes: map[string]fs.FileMode{"dir": dir(0o755)},
		},
	})
}

func TestRunSymbolicWithoutWho(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:      "sets the bits of every class",
			Files:     fsx.MemFS{"file.txt": {Mode: 0}},
			Args:      []string{"+x", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o111},
		},
		{
			Name:      "the umask filters the bits that are set",
			Files:     fsx.MemFS{"file.txt": {Mode: 0}},
			Umask:     0o022,
			Args:      []string{"+w", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o200},
		},
		{
			Name:      "the umask filters the bits that are cleared",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o666}},
			Umask:     0o022,
			Args:      []string{"--", "-w", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o466},
		},
		{
			Name:      "the umask filters the bits that equals sets",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o777}},
			Umask:     0o022,
			Args:      []string{"=rw", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			Name:      "a who is not filtered by the umask",
			Files:     fsx.MemFS{"file.txt": {Mode: 0}},
			Umask:     0o022,
			Args:      []string{"a+rw", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o666},
		},
		{
			Name:      "the special bits are not filtered by the umask",
			Files:     fsx.MemFS{"file.txt": {Mode: 0}},
			Umask:     0o077,
			Args:      []string{"+s", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": fs.ModeSetuid | fs.ModeSetgid},
		},
	})
}

func TestRunSymbolicX(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:      "X sets the execute bits of a directory",
			Files:     fsx.MemFS{"dir": {Mode: dir(0o644)}},
			Args:      []string{"a+X", "dir"},
			WantModes: map[string]fs.FileMode{"dir": dir(0o755)},
		},
		{
			Name:      "X sets the execute bits of a file with an execute bit",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o741}},
			Args:      []string{"a+X", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o751},
		},
		{
			Name:      "X is ignored on a file without execute bits",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"a+X", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			Name:      "X clears the execute bits of a file with an execute bit",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o755}},
			Args:      []string{"a-X", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			Name:      "X is ignored when clearing on a file without execute bits",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"a-X", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
	})
}

func TestRunSymbolicSpecial(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:      "u+s sets the set-user-ID bit",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"u+s", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644 | fs.ModeSetuid},
		},
		{
			Name:      "g+s sets the set-group-ID bit",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"g+s", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644 | fs.ModeSetgid},
		},
		{
			Name:      "o+s changes nothing",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"o+s", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			Name:      "go+s sets only the set-group-ID bit",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"go+s", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644 | fs.ModeSetgid},
		},
		{
			Name:      "s without a who sets both bits",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"+s", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644 | fs.ModeSetuid | fs.ModeSetgid},
		},
		{
			Name:      "u-s clears the set-user-ID bit",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644 | fs.ModeSetuid}},
			Args:      []string{"u-s", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			Name:      "+t sets the sticky bit",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"+t", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644 | fs.ModeSticky},
		},
		{
			Name:      "a-t clears the sticky bit",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644 | fs.ModeSticky}},
			Args:      []string{"a-t", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			Name:      "equals without a who clears the special bits",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o755 | fs.ModeSetuid}},
			Umask:     0o022,
			Args:      []string{"=rw", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			Name:      "equals keeps the special bits of other classes",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o755 | fs.ModeSetuid}},
			Args:      []string{"u=rw", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o655 | fs.ModeSetuid},
		},
	})
}

func TestRunSymbolicCopy(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:      "g=u copies the user permissions",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o741}},
			Args:      []string{"g=u", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o771},
		},
		{
			Name:      "o=u copies the user permissions",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o750}},
			Args:      []string{"o=u", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o757},
		},
		{
			Name:      "a permcopy with no who applies to every class",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o640}},
			Args:      []string{"=u", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o666},
		},
		{
			Name:      "actions are applied in order, like o=u-g from the spec",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o750}},
			Args:      []string{"o=u-g", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o752},
		},
		{
			Name:      "uo=g matches the spec example",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o641}},
			Args:      []string{"uo=g", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o444},
		},
		{
			Name:      "the permissions are copied from each file",
			Files:     fsx.MemFS{"a.txt": {Mode: 0o600}, "b.txt": {Mode: 0o060}},
			Args:      []string{"g=u", "a.txt", "b.txt"},
			WantModes: map[string]fs.FileMode{"a.txt": 0o660, "b.txt": 0},
		},
	})
}

func TestRunInvalidModes(t *testing.T) {
	tests := []testCase{
		{Name: "an invalid octal digit", Args: []string{"888", "file.txt"}},
		{Name: "too many digits", Args: []string{"12345", "file.txt"}},
		{Name: "octal and symbolic mixed", Args: []string{"77+x", "file.txt"}},
		{Name: "an unknown perm letter", Args: []string{"q+x", "file.txt"}},
		{Name: "a who without actions", Args: []string{"u", "file.txt"}},
		{Name: "an empty clause", Args: []string{"u+x,,g+w", "file.txt"}},
		{Name: "a permcopy followed by perms", Args: []string{"o=ug", "file.txt"}},
		{Name: "a missing op", Args: []string{"ur", "file.txt"}},
		{Name: "a perm without an op", Args: []string{"rw", "file.txt"}},
		{Name: "an empty mode", Args: []string{"", "file.txt"}},
	}
	for i := range tests {
		tests[i].Files = fsx.MemFS{"file.txt": {Mode: 0o644}}
		tests[i].WantModes = map[string]fs.FileMode{"file.txt": 0o644}
		tests[i].WantErr = "chmod: invalid mode: \"" + tests[i].Args[0] + "\"\nTry 'chmod --help' for more information.\n"
		tests[i].WantExit = 1
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
			Name:  "R changes every file below the directory",
			Files: tree(),
			Args:  []string{"-R", "700", "dir"},
			WantModes: map[string]fs.FileMode{
				"dir":           dir(0o700),
				"dir/a.txt":     0o700,
				"dir/sub":       dir(0o700),
				"dir/sub/b.txt": 0o700,
			},
		},
		{
			Name:      "R on a plain file works like without R",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"-R", "600", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o600},
		},
		{
			Name:  "R with a symbolic mode",
			Files: tree(),
			Args:  []string{"-R", "go-w", "dir"},
			WantModes: map[string]fs.FileMode{
				"dir":           dir(0o755),
				"dir/a.txt":     0o644,
				"dir/sub":       dir(0o755),
				"dir/sub/b.txt": 0o600,
			},
		},
		{
			Name:  "R with X only touches files that have an execute bit",
			Files: tree(),
			Args:  []string{"-R", "a+X", "dir"},
			WantModes: map[string]fs.FileMode{
				"dir":           dir(0o755),
				"dir/a.txt":     0o644,
				"dir/sub":       dir(0o755),
				"dir/sub/b.txt": 0o600,
			},
		},
		{
			Name:  "R with X adds the execute bits to directories",
			Files: fsx.MemFS{"dir": {Mode: dir(0o644)}, "dir/a.txt": {Mode: 0o744}},
			Args:  []string{"-R", "a+X", "dir"},
			WantModes: map[string]fs.FileMode{
				"dir":       dir(0o755),
				"dir/a.txt": 0o755,
			},
		},
		{
			Name:      "R keeps going after a missing file",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"-R", "700", "missing", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o700},
			WantErr:   "chmod: missing: file does not exist\n",
			WantExit:  1,
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
			Name:  "recursive changes every file below the directory",
			Files: fsx.MemFS{"dir": {Mode: dir(0o755)}, "dir/a.txt": {Mode: 0o644}},
			Args:  []string{"--recursive", "700", "dir"},
			WantModes: map[string]fs.FileMode{
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
			},
		},
	})
}

func TestRunGeneral(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:      "a missing operand is an error",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      nil,
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
			WantErr:   "chmod: missing operand\nTry 'chmod --help' for more information.\n",
			WantExit:  2,
		},
		{
			Name:      "a missing file operand is an error",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"755"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
			WantErr:   "chmod: missing file operand\nTry 'chmod --help' for more information.\n",
			WantExit:  2,
		},
		{
			Name:      "an invalid option is an error",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"-x", "755", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
			WantErr:   "chmod: invalid option -- 'x'\nTry 'chmod --help' for more information.\n",
			WantExit:  2,
		},
		{
			Name:      "an unrecognized long option",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"--bogus", "755", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
			WantErr:   "chmod: unrecognized option '--bogus'\nTry 'chmod --help' for more information.\n",
			WantExit:  2,
		},
		{
			Name:      "a mode starting with a dash can follow a double dash",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o666}},
			Args:      []string{"--", "-w", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o444},
		},
		{
			Name:      "a file named like an option can follow a double dash",
			Files:     fsx.MemFS{"-w": {Mode: 0o644}},
			Args:      []string{"--", "600", "-w"},
			WantModes: map[string]fs.FileMode{"-w": 0o600},
		},
		{
			Name:      "a missing file is an error, but the others are changed",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"755", "missing", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o755},
			WantErr:   "chmod: missing: file does not exist\n",
			WantExit:  1,
		},
	})
}

func TestRunReference(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:      "uses the mode of the reference file",
			Files:     fsx.MemFS{"ref": {Mode: 0o640}, "file.txt": {Mode: 0o600}},
			Args:      []string{"--reference=ref", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o640},
		},
		{
			Name:      "the reference can be a separate argument",
			Files:     fsx.MemFS{"ref": {Mode: 0o640}, "file.txt": {Mode: 0o600}},
			Args:      []string{"--reference", "ref", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o640},
		},
		{
			Name:      "the mode applies to every file",
			Files:     fsx.MemFS{"ref": {Mode: 0o600}, "a.txt": {Mode: 0o644}, "b.txt": {Mode: 0o755}},
			Args:      []string{"--reference=ref", "a.txt", "b.txt"},
			WantModes: map[string]fs.FileMode{"a.txt": 0o600, "b.txt": 0o600},
		},
		{
			Name:      "the special bits are copied",
			Files:     fsx.MemFS{"ref": {Mode: 0o755 | fs.ModeSetuid | fs.ModeSticky}, "file.txt": {Mode: 0o644}},
			Args:      []string{"--reference=ref", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o755 | fs.ModeSetuid | fs.ModeSticky},
		},
		{
			Name:      "a reference to a symbolic link is dereferenced",
			Files:     fsx.MemFS{"ref": {Mode: 0o640}, "link": symlink("ref"), "file.txt": {Mode: 0o600}},
			Args:      []string{"--reference=link", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o640},
		},
		{
			Name:  "a reference can be combined with R",
			Files: fsx.MemFS{"ref": {Mode: 0o700}, "dir": {Mode: dir(0o755)}, "dir/a.txt": {Mode: 0o644}},
			Args:  []string{"-R", "--reference=ref", "dir"},
			WantModes: map[string]fs.FileMode{
				"ref":       0o700,
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
			},
		},
		{
			// With --reference, every operand is a file: a mode-looking
			// name is just a missing file.
			Name:      "a mode-looking operand is a file with --reference",
			Files:     fsx.MemFS{"ref": {Mode: 0o640}, "file.txt": {Mode: 0o644}},
			Args:      []string{"--reference=ref", "644", "file.txt"},
			WantModes: map[string]fs.FileMode{"ref": 0o640, "file.txt": 0o640},
			WantErr:   "chmod: 644: file does not exist\n",
			WantExit:  1,
		},
		{
			Name:      "a missing reference file is an error",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"--reference=missing", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
			WantErr:   "chmod: failed to get attributes of missing: file does not exist\n",
			WantExit:  1,
		},
		{
			Name:      "no operands is a usage error",
			Files:     fsx.MemFS{"ref": {Mode: 0o640}},
			Args:      []string{"--reference=ref"},
			WantModes: map[string]fs.FileMode{"ref": 0o640},
			WantErr:   "chmod: missing operand\nTry 'chmod --help' for more information.\n",
			WantExit:  2,
		},
		{
			Name:      "a lone operand is a file, not a mode",
			Files:     fsx.MemFS{"ref": {Mode: 0o640}, "u+x": {Mode: 0o600}},
			Args:      []string{"--reference=ref", "u+x"},
			WantModes: map[string]fs.FileMode{"u+x": 0o640},
		},
		{
			Name:      "--reference works with the mode-like file x+y", // sanity: non-mode operand
			Files:     fsx.MemFS{"ref": {Mode: 0o640}, "x+y": {Mode: 0o600}},
			Args:      []string{"--reference=ref", "x+y"},
			WantModes: map[string]fs.FileMode{"x+y": 0o640},
		},
	})
}

func TestRunVerbose(t *testing.T) {
	runTests(t, []testCase{
		{
			Name:      "v reports a change",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o600}},
			Args:      []string{"-v", "644", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
			Want:      "mode of 'file.txt' changed from 0600 (rw-------) to 0644 (rw-r--r--)\n",
		},
		{
			Name:      "v reports a retained mode",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"-v", "644", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
			Want:      "mode of 'file.txt' retained as 0644 (rw-r--r--)\n",
		},
		{
			Name:      "c reports only the files whose mode changed",
			Files:     fsx.MemFS{"a.txt": {Mode: 0o600}, "b.txt": {Mode: 0o644}},
			Args:      []string{"-c", "644", "a.txt", "b.txt"},
			WantModes: map[string]fs.FileMode{"a.txt": 0o644, "b.txt": 0o644},
			Want:      "mode of 'a.txt' changed from 0600 (rw-------) to 0644 (rw-r--r--)\n",
		},
		{
			Name:      "c reports nothing when no mode changes",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"-c", "644", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			Name:      "without c or v nothing is reported",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o600}},
			Args:      []string{"644", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			Name:      "the last of c and v wins",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"-c", "-v", "644", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
			Want:      "mode of 'file.txt' retained as 0644 (rw-r--r--)\n",
		},
		{
			Name:      "the last of v and c wins",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"-v", "-c", "644", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
		},
		{
			Name:      "the long options work",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o600}},
			Args:      []string{"--changes", "644", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
			Want:      "mode of 'file.txt' changed from 0600 (rw-------) to 0644 (rw-r--r--)\n",
		},
		{
			Name:      "v reports the special bits like ls does",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"-v", "4755", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o755 | fs.ModeSetuid},
			Want:      "mode of 'file.txt' changed from 0644 (rw-r--r--) to 4755 (rwsr-xr-x)\n",
		},
		{
			Name:      "v reports the special bits without execute like ls does",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o644}},
			Args:      []string{"-v", "2644", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644 | fs.ModeSetgid},
			Want:      "mode of 'file.txt' changed from 0644 (rw-r--r--) to 2644 (rw-r-Sr--)\n",
		},
		{
			Name:      "v reports the sticky bit like ls does",
			Files:     fsx.MemFS{"dir": {Mode: dir(0o755)}},
			Args:      []string{"-v", "1755", "dir"},
			WantModes: map[string]fs.FileMode{"dir": dir(0o755) | fs.ModeSticky},
			Want:      "mode of 'dir' changed from 0755 (rwxr-xr-x) to 1755 (rwxr-xr-t)\n",
		},
		{
			Name:      "v reports the operand as given",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o600}},
			Args:      []string{"-v", "644", "./file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
			Want:      "mode of './file.txt' changed from 0600 (rw-------) to 0644 (rw-r--r--)\n",
		},
		{
			Name:      "v with a symbolic mode reports the resulting mode",
			Files:     fsx.MemFS{"file.txt": {Mode: 0o600}},
			Args:      []string{"-v", "g+r,o+r", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o644},
			Want:      "mode of 'file.txt' changed from 0600 (rw-------) to 0644 (rw-r--r--)\n",
		},
		{
			Name:      "v with --reference reports the change",
			Files:     fsx.MemFS{"ref": {Mode: 0o640}, "file.txt": {Mode: 0o600}},
			Args:      []string{"-v", "--reference=ref", "file.txt"},
			WantModes: map[string]fs.FileMode{"file.txt": 0o640},
			Want:      "mode of 'file.txt' changed from 0600 (rw-------) to 0640 (rw-r-----)\n",
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
			Name:  "v reports every file of the walk",
			Files: tree(),
			Args:  []string{"-Rv", "700", "dir"},
			WantModes: map[string]fs.FileMode{
				"dir":           dir(0o700),
				"dir/a.txt":     0o700,
				"dir/sub":       dir(0o700),
				"dir/sub/b.txt": 0o700,
			},
			Want: "mode of 'dir' changed from 0755 (rwxr-xr-x) to 0700 (rwx------)\n" +
				"mode of 'dir/a.txt' changed from 0644 (rw-r--r--) to 0700 (rwx------)\n" +
				"mode of 'dir/sub' changed from 0755 (rwxr-xr-x) to 0700 (rwx------)\n" +
				"mode of 'dir/sub/b.txt' changed from 0600 (rw-------) to 0700 (rwx------)\n",
		},
		{
			Name:  "c reports only the files whose mode changed",
			Files: tree(),
			Args:  []string{"-Rc", "a+X", "dir"},
			WantModes: map[string]fs.FileMode{
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
			Name: "R ignores a symbolic link of the walk",
			Files: fsx.MemFS{
				"dir":       {Mode: dir(0o755)},
				"dir/a.txt": {Mode: 0o644},
				"dir/link":  symlink("a.txt"),
			},
			Args: []string{"-R", "700", "dir"},
			WantModes: map[string]fs.FileMode{
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
				"dir/link":  fs.ModeSymlink | 0o777,
			},
		},
		{
			Name: "R ignores a dangling symbolic link",
			Files: fsx.MemFS{
				"dir":       {Mode: dir(0o755)},
				"dir/a.txt": {Mode: 0o644},
				"dir/link":  symlink("missing"),
			},
			Args: []string{"-R", "700", "dir"},
			WantModes: map[string]fs.FileMode{
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
				"dir/link":  fs.ModeSymlink | 0o777,
			},
		},
		{
			Name: "R does not act on what a link of the walk points at",
			Files: fsx.MemFS{
				"dir":      {Mode: dir(0o755)},
				"dir/link": symlink("target"),
				"target":   {Mode: 0o600},
			},
			Args: []string{"-R", "700", "dir"},
			WantModes: map[string]fs.FileMode{
				"dir":      dir(0o700),
				"dir/link": fs.ModeSymlink | 0o777,
				"target":   0o600,
			},
		},
		{
			Name: "R follows a symbolic link operand to a directory, like -H",
			Files: fsx.MemFS{
				"dir":       {Mode: dir(0o755)},
				"dir/a.txt": {Mode: 0o644},
				"dlink":     symlink("dir"),
			},
			Args: []string{"-R", "700", "dlink"},
			WantModes: map[string]fs.FileMode{
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
				"dlink":     fs.ModeSymlink | 0o777,
			},
		},
		{
			Name: "R does not follow a symbolic link to a directory",
			Files: fsx.MemFS{
				"dir":       {Mode: dir(0o755)},
				"dir/a.txt": {Mode: 0o644},
				"dir/sub":   {Mode: dir(0o755)},
				"dir/link":  symlink("sub"),
			},
			Args: []string{"-R", "700", "dir"},
			WantModes: map[string]fs.FileMode{
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
				"dir/sub":   dir(0o700),
				"dir/link":  fs.ModeSymlink | 0o777,
			},
		},
		{
			Name: "v does not report the links ignored in the walk",
			Files: fsx.MemFS{
				"dir":       {Mode: dir(0o755)},
				"dir/a.txt": {Mode: 0o644},
				"dir/link":  symlink("a.txt"),
			},
			Args: []string{"-Rv", "700", "dir"},
			WantModes: map[string]fs.FileMode{
				"dir":       dir(0o700),
				"dir/a.txt": 0o700,
				"dir/link":  fs.ModeSymlink | 0o777,
			},
			Want: "mode of 'dir' changed from 0755 (rwxr-xr-x) to 0700 (rwx------)\n" +
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
	testutil.HelpVersion(t, New, "chmod", usage)
}
