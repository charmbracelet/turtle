package install

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"runtime"
	"strings"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

var (
	oldTime   = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	otherTime = time.Date(1999, 12, 31, 23, 59, 58, 0, time.UTC)
)

type testCase struct {
	name     string
	files    fsx.MemFS
	wd       string
	environ  []string
	umask    fs.FileMode
	args     []string
	want     map[string]string
	wantMode map[string]fs.FileMode
	wantTime map[string]time.Time
	wantLink map[string]string
	wantOut  string
	present  []string
	absent   []string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         test.files,
		WorkingDir: test.wd,
		Environ:    func() []string { return test.environ },
		Umask:      test.umask,
	}

	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
	if got := stdout.String(); got != test.wantOut {
		t.Errorf("Run() stdout = %q, want %q", got, test.wantOut)
	}

	for name, want := range test.want {
		got, err := fs.ReadFile(test.files, name)
		if err != nil {
			t.Errorf("ReadFile(%q) error = %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("ReadFile(%q) = %q, want %q", name, got, want)
		}
	}
	for name, want := range test.wantMode {
		fi, err := fs.Stat(test.files, name)
		if err != nil {
			t.Errorf("Stat(%q) error = %v", name, err)
			continue
		}
		if got := fi.Mode(); got != want {
			t.Errorf("%s mode = %v, want %v", name, got, want)
		}
	}
	for name, want := range test.wantTime {
		fi, err := fs.Stat(test.files, name)
		if err != nil {
			t.Errorf("Stat(%q) error = %v", name, err)
			continue
		}
		if got := fi.ModTime(); !got.Equal(want) {
			t.Errorf("%s modified time = %v, want %v", name, got, want)
		}
	}
	for name, want := range test.wantLink {
		got, err := test.files.ReadLink(name)
		if err != nil {
			t.Errorf("ReadLink(%q) error = %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("ReadLink(%q) = %q, want %q", name, got, want)
		}
		fi, err := test.files.Lstat(name)
		if err != nil {
			t.Errorf("Lstat(%q) error = %v", name, err)
			continue
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("Lstat(%q) mode = %v, want a symbolic link", name, fi.Mode())
		}
	}
	for _, name := range test.present {
		if _, err := fs.Stat(test.files, name); err != nil {
			t.Errorf("Stat(%q) error = %v, want it to still exist", name, err)
		}
	}
	for _, name := range test.absent {
		if _, err := fs.Stat(test.files, name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat(%q) error = %v, want fs.ErrNotExist", name, err)
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

// link is a MemFS entry holding a symbolic link to target.
func link(target string) *fsx.MemFile {
	return &fsx.MemFile{Mode: fs.ModeSymlink, Data: []byte(target)}
}

func TestRunCopy(t *testing.T) {
	tests := []testCase{
		{
			name:     "two operands copy a file",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "x"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o755},
		},
		{
			name:     "multiple sources copy into a directory",
			files:    fsx.MemFS{"a": {Data: []byte("1")}, "b": {Data: []byte("2")}, "d": {Mode: fs.ModeDir | 0o755}},
			args:     []string{"a", "b", "d"},
			want:     map[string]string{"d/a": "1", "d/b": "2"},
			wantMode: map[string]fs.FileMode{"d/a": 0o755, "d/b": 0o755},
		},
		{
			name:  "-t copies into a directory",
			files: fsx.MemFS{"a": {Data: []byte("1")}, "d": {Mode: fs.ModeDir | 0o755}},
			args:  []string{"-t", "d", "a"},
			want:  map[string]string{"d/a": "1"},
		},
		{
			name:     "-T refuses a directory destination",
			files:    fsx.MemFS{"a": {Data: []byte("1")}, "d": {Mode: fs.ModeDir | 0o755}},
			args:     []string{"-T", "a", "d"},
			wantErr:  "install: cannot overwrite directory 'd' with non-directory 'a'\n",
			wantExit: 1,
		},
		{
			name:  "an existing destination is replaced",
			files: fsx.MemFS{"a": {Data: []byte("new"), Mode: 0o600}, "b": {Data: []byte("old"), Mode: 0o600}},
			args:  []string{"a", "b"},
			want:  map[string]string{"b": "new"},
			wantMode: map[string]fs.FileMode{
				"b": 0o755,
			},
		},
		{
			name:     "the source mode is not used",
			files:    fsx.MemFS{"a": {Data: []byte("x"), Mode: 0o600}},
			args:     []string{"a", "b"},
			wantMode: map[string]fs.FileMode{"b": 0o755},
		},
		{
			name:     "the umask never masks the mode",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			umask:    0o077,
			args:     []string{"a", "b"},
			want:     map[string]string{"b": "x"},
			wantMode: map[string]fs.FileMode{"b": 0o755},
		},
		{
			name:     "a missing source is an error",
			files:    fsx.MemFS{},
			args:     []string{"missing.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "install: cannot stat 'missing.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing destination parent is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt", "nodir/b.txt"},
			absent:   []string{"nodir"},
			wantErr:  "install: cannot create 'nodir/b.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a directory source is omitted",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o755}},
			args:     []string{"d", "b"},
			absent:   []string{"b"},
			wantErr:  "install: omitting directory 'd'\n",
			wantExit: 1,
		},
		{
			name:     "a file is not copied onto itself",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			args:     []string{"a", "a"},
			wantErr:  "install: 'a' and 'a' are the same file\n",
			wantExit: 1,
		},
		{
			name:     "a file is not copied onto itself through a link",
			files:    fsx.MemFS{"a": {Data: []byte("x")}, "l": link("a")},
			args:     []string{"a", "l"},
			wantErr:  "install: 'a' and 'l' are the same file\n",
			wantExit: 1,
		},
		{
			name:     "a directory destination is not overwritten",
			files:    fsx.MemFS{"a": {Data: []byte("x")}, "d": {Mode: fs.ModeDir | 0o755}},
			args:     []string{"-T", "a", "d"},
			wantErr:  "install: cannot overwrite directory 'd' with non-directory 'a'\n",
			wantExit: 1,
		},
		{
			name:     "no operand is a usage error",
			files:    fsx.MemFS{},
			args:     nil,
			wantErr:  "install: missing file operand\nTry 'install --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "one operand misses the destination",
			files:    fsx.MemFS{"a": {}},
			args:     []string{"a"},
			wantErr:  "install: missing destination file operand after 'a'\nTry 'install --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an extra operand is a usage error with -T",
			files:    fsx.MemFS{"a": {}, "b": {}, "c": {}},
			args:     []string{"-T", "a", "b", "c"},
			wantErr:  "install: extra operand 'c'\nTry 'install --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "several sources need a directory target",
			files:    fsx.MemFS{"a": {Data: []byte("1")}, "b": {Data: []byte("2")}, "c": {Data: []byte("3")}},
			args:     []string{"a", "b", "c"},
			wantErr:  "install: target 'c' is not a directory\n",
			wantExit: 1,
		},
		{
			name:     "-t needs a directory",
			files:    fsx.MemFS{"a": {Data: []byte("1")}},
			args:     []string{"-t", "nodir", "a"},
			wantErr:  "install: failed to access 'nodir': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "-t refuses a file target",
			files:    fsx.MemFS{"a": {Data: []byte("1")}, "f": {Data: []byte("2")}},
			args:     []string{"-t", "f", "a"},
			wantErr:  "install: failed to access 'f': not a directory\n",
			wantExit: 1,
		},
		{
			name:     "-t can't be combined with -T",
			files:    fsx.MemFS{},
			args:     []string{"-t", "d", "-T"},
			wantErr:  "install: cannot combine --target-directory (-t) and --no-target-directory (-T)\nTry 'install --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a target directory can't be given twice",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o755}},
			args:     []string{"-t", "d", "-t", "d"},
			wantErr:  "install: multiple target directories specified\nTry 'install --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "-d can't take a target directory",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o755}},
			args:     []string{"-d", "-t", "d"},
			wantErr:  "install: target directory not allowed when installing a directory\nTry 'install --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a zero-length operand is an error",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			args:     []string{"a", ""},
			wantErr:  "install: invalid zero-length file name\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunMode(t *testing.T) {
	tests := []testCase{
		{
			name:     "an octal mode",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			args:     []string{"-m", "644", "a", "b"},
			want:     map[string]string{"b": "x"},
			wantMode: map[string]fs.FileMode{"b": 0o644},
		},
		{
			name:     "an attached octal mode",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			args:     []string{"-m644", "a", "b"},
			wantMode: map[string]fs.FileMode{"b": 0o644},
		},
		{
			name:     "the special bits are set",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			args:     []string{"-m", "4755", "a", "b"},
			wantMode: map[string]fs.FileMode{"b": fs.ModeSetuid | 0o755},
		},
		{
			name:     "a symbolic mode starts from no access",
			files:    fsx.MemFS{"a": {Data: []byte("x"), Mode: 0o666}},
			args:     []string{"-m", "u=rw,go=r", "a", "b"},
			wantMode: map[string]fs.FileMode{"b": 0o644},
		},
		{
			name:     "a symbolic mode sets the mode of directories",
			files:    fsx.MemFS{},
			args:     []string{"-d", "-m", "u=rwx,go=", "d"},
			wantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o700},
		},
		{
			name:     "an invalid mode is an error",
			files:    fsx.MemFS{"a": {}},
			args:     []string{"-m", "bogus", "a", "b"},
			wantErr:  "install: invalid mode: \"bogus\"\n",
			wantExit: 1,
		},
		{
			name:     "a missing mode operand is a usage error",
			files:    fsx.MemFS{"a": {}},
			args:     []string{"-m"},
			wantErr:  "install: option requires an argument -- 'm'\nTry 'install --help' for more information.\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunTimestamps(t *testing.T) {
	tests := []testCase{
		{
			name:     "-p preserves the times of the source",
			files:    fsx.MemFS{"a": {Data: []byte("x"), ModTime: oldTime, Mode: 0o644}},
			args:     []string{"-p", "a", "b"},
			wantTime: map[string]time.Time{"b": oldTime},
		},
		{
			name:  "the times are those of the installation without -p",
			files: fsx.MemFS{"a": {Data: []byte("x"), ModTime: oldTime, Mode: 0o644}},
			args:  []string{"a", "b"},
			want:  map[string]string{"b": "x"},
		},
		{
			name:     "-p applies to an overwritten destination",
			files:    fsx.MemFS{"a": {Data: []byte("x"), ModTime: oldTime, Mode: 0o644}, "b": {Data: []byte("y"), ModTime: otherTime}},
			args:     []string{"-p", "a", "b"},
			want:     map[string]string{"b": "x"},
			wantTime: map[string]time.Time{"b": oldTime},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDirectory(t *testing.T) {
	tests := []testCase{
		{
			name:     "-d creates one directory",
			files:    fsx.MemFS{},
			args:     []string{"-d", "d"},
			wantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o755},
		},
		{
			name:     "-d creates several directories",
			files:    fsx.MemFS{},
			args:     []string{"-d", "a", "b"},
			wantMode: map[string]fs.FileMode{"a": fs.ModeDir | 0o755, "b": fs.ModeDir | 0o755},
		},
		{
			name:  "-d creates the missing parents",
			files: fsx.MemFS{"a": {Mode: fs.ModeDir | 0o700}},
			args:  []string{"-d", "a/b/c"},
			wantMode: map[string]fs.FileMode{
				"a":   fs.ModeDir | 0o700,
				"a/b": fs.ModeDir | 0o755,
			},
			present: []string{"a/b/c"},
		},
		{
			name:  "-d parents get 755 regardless of the umask",
			files: fsx.MemFS{},
			umask: 0o077,
			args:  []string{"-d", "a/b"},
			wantMode: map[string]fs.FileMode{
				"a":   fs.ModeDir | 0o755,
				"a/b": fs.ModeDir | 0o755,
			},
		},
		{
			name:  "-d parents get 755 regardless of -m",
			files: fsx.MemFS{},
			args:  []string{"-d", "-m", "700", "a/b"},
			wantMode: map[string]fs.FileMode{
				"a":   fs.ModeDir | 0o755,
				"a/b": fs.ModeDir | 0o700,
			},
		},
		{
			name:     "-d resets the mode of an existing directory",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o700}},
			args:     []string{"-d", "d"},
			wantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o755},
		},
		{
			name:     "-d sets the mode of an existing directory with -m",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o755}},
			args:     []string{"-d", "-m", "700", "d"},
			wantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o700},
		},
		{
			name:     "-d announces every directory it creates",
			files:    fsx.MemFS{},
			args:     []string{"-d", "-v", "a/b"},
			wantOut:  "install: creating directory 'a'\ninstall: creating directory 'a/b'\n",
			wantMode: map[string]fs.FileMode{"a": fs.ModeDir | 0o755, "a/b": fs.ModeDir | 0o755},
		},
		{
			name:     "-d keeps quiet about existing directories",
			files:    fsx.MemFS{"a": {Mode: fs.ModeDir | 0o755}},
			args:     []string{"-d", "-v", "a/b"},
			wantOut:  "install: creating directory 'a/b'\n",
			wantMode: map[string]fs.FileMode{"a/b": fs.ModeDir | 0o755},
		},
		{
			name:     "-d on an existing file is an error",
			files:    fsx.MemFS{"f": {Data: []byte("x")}},
			args:     []string{"-d", "f"},
			wantErr:  "install: cannot create directory 'f': file already exists\n",
			wantExit: 1,
		},
		{
			name:     "-d above an existing file is an error",
			files:    fsx.MemFS{"f": {Data: []byte("x")}},
			args:     []string{"-d", "f/d"},
			wantErr:  "install: cannot create directory 'f': not a directory\n",
			wantExit: 1,
		},
		{
			name:     "-d without an operand is a usage error",
			files:    fsx.MemFS{},
			args:     []string{"-d"},
			wantErr:  "install: missing operand\nTry 'install --help' for more information.\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunCreateParents(t *testing.T) {
	tests := []testCase{
		{
			name:     "-D creates the parents of the destination",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			args:     []string{"-D", "a", "d/b"},
			want:     map[string]string{"d/b": "x"},
			wantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o755, "d/b": 0o755},
		},
		{
			name:     "-D parents get 755 regardless of the umask",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			umask:    0o077,
			args:     []string{"-D", "a", "d/e/b"},
			wantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o755, "d/e": fs.ModeDir | 0o755},
		},
		{
			name:    "-D announces the directories it creates",
			files:   fsx.MemFS{"a": {Data: []byte("x")}},
			args:    []string{"-Dv", "a", "d/b"},
			wantOut: "install: creating directory 'd'\n'a' -> 'd/b'\n",
			want:    map[string]string{"d/b": "x"},
		},
		{
			name:  "-D leaves an existing destination alone",
			files: fsx.MemFS{"a": {Data: []byte("x")}, "b": {Data: []byte("y")}},
			args:  []string{"-D", "a", "b"},
			want:  map[string]string{"b": "x"},
		},
		{
			name:     "-D above an existing file is an error",
			files:    fsx.MemFS{"a": {Data: []byte("x")}, "f": {Data: []byte("y")}},
			args:     []string{"-D", "a", "f/b"},
			wantErr:  "install: cannot create directory 'f': not a directory\n",
			wantExit: 1,
		},
		{
			name:     "-D -t creates the target directory",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			args:     []string{"-D", "-t", "d/e", "a"},
			want:     map[string]string{"d/e/a": "x"},
			wantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o755, "d/e": fs.ModeDir | 0o755},
		},
		{
			name:     "-D -t refuses a file target",
			files:    fsx.MemFS{"a": {Data: []byte("x")}, "d": {Mode: fs.ModeDir | 0o755}, "d/f": {Data: []byte("y")}},
			args:     []string{"-D", "-t", "d/f", "a"},
			wantErr:  "install: failed to access 'd/f': not a directory\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunBackup(t *testing.T) {
	tests := []testCase{
		{
			name:  "-b makes a simple backup by default",
			files: fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			args:  []string{"-b", "a", "b"},
			want:  map[string]string{"b": "new", "b~": "old"},
		},
		{
			name:  "-S overrides the suffix",
			files: fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			args:  []string{"-b", "-S", ".bak", "a", "b"},
			want:  map[string]string{"b": "new", "b.bak": "old"},
		},
		{
			name:    "SIMPLE_BACKUP_SUFFIX sets the suffix",
			files:   fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			environ: []string{"SIMPLE_BACKUP_SUFFIX=.old"},
			args:    []string{"-b", "a", "b"},
			want:    map[string]string{"b": "new", "b.old": "old"},
		},
		{
			name:  "--backup=numbered numbers the backups",
			files: fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			args:  []string{"--backup=numbered", "a", "b"},
			want:  map[string]string{"b": "new", "b.~1~": "old"},
		},
		{
			name:  "numbered backups continue after the highest",
			files: fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}, "b.~2~": {Data: []byte("older")}},
			args:  []string{"--backup=numbered", "a", "b"},
			want:  map[string]string{"b": "new", "b.~3~": "old"},
		},
		{
			name:  "--backup=existing follows numbered ones",
			files: fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}, "b.~1~": {Data: []byte("older")}},
			args:  []string{"--backup=existing", "a", "b"},
			want:  map[string]string{"b": "new", "b.~2~": "old"},
		},
		{
			name:    "VERSION_CONTROL sets the method",
			files:   fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			environ: []string{"VERSION_CONTROL=numbered"},
			args:    []string{"-b", "a", "b"},
			want:    map[string]string{"b": "new", "b.~1~": "old"},
		},
		{
			name:    "an invalid VERSION_CONTROL is ignored with a warning",
			files:   fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			environ: []string{"VERSION_CONTROL=bogus"},
			args:    []string{"-b", "a", "b"},
			want:    map[string]string{"b": "new", "b~": "old"},
			wantErr: "install: ignoring invalid value of environment variable VERSION_CONTROL\n",
		},
		{
			name:    "a set but empty SIMPLE_BACKUP_SUFFIX is honored",
			files:   fsx.MemFS{"a": {Data: []byte("n")}, "b": {Data: []byte("old")}},
			environ: []string{"SIMPLE_BACKUP_SUFFIX="},
			args:    []string{"-b", "a", "b"},
			want:    map[string]string{"b": "n"},
		},
		{
			name:     "an invalid backup method is a usage error",
			files:    fsx.MemFS{"a": {}, "b": {}},
			args:     []string{"--backup=bogus", "a", "b"},
			wantErr:  "install: invalid backup method 'bogus'\nTry 'install --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "-b backups a directory destination with -T",
			files:    fsx.MemFS{"a": {Data: []byte("x")}, "d": {Mode: fs.ModeDir | 0o700}},
			args:     []string{"-bT", "a", "d"},
			want:     map[string]string{"d": "x"},
			wantMode: map[string]fs.FileMode{"d~": fs.ModeDir | 0o700},
		},
		{
			name:    "verbose tells about the backup",
			files:   fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			args:    []string{"-bv", "a", "b"},
			wantOut: "'a' -> 'b' (backup: 'b~')\n",
			want:    map[string]string{"b": "new", "b~": "old"},
		},
		{
			name:     "a backup which would destroy the source is refused",
			files:    fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}, "b~": {Data: []byte("new")}},
			args:     []string{"-b", "b~", "b"},
			wantErr:  "install: backing up 'b' might destroy source; 'b~' not copied\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunCompare(t *testing.T) {
	tests := []testCase{
		{
			name:     "-C skips an identical destination",
			files:    fsx.MemFS{"a": {Data: []byte("x"), Mode: 0o755}, "b": {Data: []byte("x"), Mode: 0o755, ModTime: otherTime}},
			args:     []string{"-C", "a", "b"},
			want:     map[string]string{"b": "x"},
			wantTime: map[string]time.Time{"b": otherTime},
		},
		{
			name:     "-C installs a different content",
			files:    fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old"), Mode: 0o600}},
			args:     []string{"-C", "a", "b"},
			want:     map[string]string{"b": "new"},
			wantMode: map[string]fs.FileMode{"b": 0o755},
		},
		{
			name:     "-C installs a different mode",
			files:    fsx.MemFS{"a": {Data: []byte("x")}, "b": {Data: []byte("x"), Mode: 0o600}},
			args:     []string{"-C", "a", "b"},
			wantMode: map[string]fs.FileMode{"b": 0o755},
		},
		{
			name:  "-C installs a longer content",
			files: fsx.MemFS{"a": {Data: []byte("longer")}, "b": {Data: []byte("x"), Mode: 0o755}},
			args:  []string{"-C", "a", "b"},
			want:  map[string]string{"b": "longer"},
		},
		{
			name:  "-C installs a shorter content",
			files: fsx.MemFS{"a": {Data: []byte("x")}, "b": {Data: []byte("longer"), Mode: 0o755}},
			args:  []string{"-C", "a", "b"},
			want:  map[string]string{"b": "x"},
		},
		{
			name:     "-C installs a missing destination",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			args:     []string{"-C", "a", "b"},
			want:     map[string]string{"b": "x"},
			wantMode: map[string]fs.FileMode{"b": 0o755},
		},
		{
			name:     "-C replaces a destination which is a symbolic link",
			files:    fsx.MemFS{"a": {Data: []byte("x")}, "c": {Data: []byte("y")}, "l": link("c")},
			args:     []string{"-C", "a", "l"},
			want:     map[string]string{"l": "x", "c": "y"},
			wantMode: map[string]fs.FileMode{"l": 0o755},
		},
		{
			name:     "-C with -p still sets the timestamps",
			files:    fsx.MemFS{"a": {Data: []byte("x"), ModTime: oldTime, Mode: 0o755}, "b": {Data: []byte("x"), Mode: 0o755, ModTime: otherTime}},
			args:     []string{"-Cp", "a", "b"},
			wantTime: map[string]time.Time{"b": oldTime},
		},
		{
			name:     "-C with a special mode warns and always installs",
			files:    fsx.MemFS{"a": {Data: []byte("x")}, "b": {Data: []byte("x"), Mode: fs.ModeSetuid | 0o755}},
			args:     []string{"-C", "-m", "4755", "a", "b"},
			want:     map[string]string{"b": "x"},
			wantMode: map[string]fs.FileMode{"b": fs.ModeSetuid | 0o755},
			wantErr:  "install: the --compare (-C) option is ignored when you specify a mode with non-permission bits\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOwnership(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ownership is not set on windows")
	}
	tests := []testCase{
		{
			name:     "-o and -g set the ownership",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			args:     []string{"-o", "100", "-g", "200", "a", "b"},
			want:     map[string]string{"b": "x"},
			wantMode: map[string]fs.FileMode{"b": 0o755},
		},
		{
			name:     "-g sets the group of a directory",
			files:    fsx.MemFS{},
			args:     []string{"-d", "-g", "200", "d"},
			wantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o755},
		},
		{
			name:     "an invalid owner is an error",
			files:    fsx.MemFS{"a": {}},
			args:     []string{"-o", "bogus", "a", "b"},
			wantErr:  "install: invalid user 'bogus'\n",
			wantExit: 1,
		},
		{
			name:     "an invalid group is an error",
			files:    fsx.MemFS{"a": {}},
			args:     []string{"-g", "bogus", "a", "b"},
			wantErr:  "install: invalid group 'bogus'\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
			if test.wantExit != 0 {
				return
			}
			dst := strings.TrimSuffix(test.args[len(test.args)-1], "/")
			var wantUid, wantGid int
			for i, arg := range test.args {
				switch arg {
				case "-o":
					wantUid, _ = parseOwner(test.args[i+1])
				case "-g":
					wantGid, _ = parseGroup(test.args[i+1])
				}
			}
			fi, err := fs.Stat(test.files, dst)
			if err != nil {
				t.Fatalf("Stat(%q) error = %v", dst, err)
			}
			uid, gid, ok := fsx.Owner(fi)
			if !ok {
				t.Fatalf("Owner(%q) not available", dst)
			}
			if uid != wantUid || gid != wantGid {
				t.Errorf("%s owner = %d:%d, want %d:%d", dst, uid, gid, wantUid, wantGid)
			}
		})
	}
}

func TestRunSymlinks(t *testing.T) {
	tests := []testCase{
		{
			name:     "a symbolic link source is followed",
			files:    fsx.MemFS{"a": {Data: []byte("x")}, "l": link("a")},
			args:     []string{"l", "b"},
			want:     map[string]string{"b": "x"},
			wantMode: map[string]fs.FileMode{"b": 0o755},
		},
		{
			name:     "a dangling symbolic link source is an error",
			files:    fsx.MemFS{"l": link("missing")},
			args:     []string{"l", "b"},
			wantErr:  "install: cannot stat 'l': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a symbolic link destination is replaced",
			files:    fsx.MemFS{"a": {Data: []byte("x")}, "b": {Data: []byte("y")}, "l": link("b")},
			args:     []string{"a", "l"},
			want:     map[string]string{"l": "x", "b": "y"},
			wantMode: map[string]fs.FileMode{"l": 0o755},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunVerbose(t *testing.T) {
	tests := []testCase{
		{
			name:    "verbose tells about each file",
			files:   fsx.MemFS{"a": {Data: []byte("1")}, "b": {Data: []byte("2")}, "d": {Mode: fs.ModeDir | 0o755}},
			args:    []string{"-v", "a", "b", "d"},
			wantOut: "'a' -> 'd/a'\n'b' -> 'd/b'\n",
			want:    map[string]string{"d/a": "1", "d/b": "2"},
		},
		{
			name:    "verbose tells about a replaced destination",
			files:   fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			args:    []string{"-v", "a", "b"},
			wantOut: "removed 'b'\n'a' -> 'b'\n",
			want:    map[string]string{"b": "new"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLongOptions(t *testing.T) {
	tests := []testCase{
		{
			name:     "--directory",
			files:    fsx.MemFS{},
			args:     []string{"--directory", "d"},
			wantMode: map[string]fs.FileMode{"d": fs.ModeDir | 0o755},
		},
		{
			name:     "--mode with an attached value",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			args:     []string{"--mode=600", "a", "b"},
			wantMode: map[string]fs.FileMode{"b": 0o600},
		},
		{
			name:     "--mode with the next argument",
			files:    fsx.MemFS{"a": {Data: []byte("x")}},
			args:     []string{"--mode", "600", "a", "b"},
			wantMode: map[string]fs.FileMode{"b": 0o600},
		},
		{
			name:  "--target-directory",
			files: fsx.MemFS{"a": {Data: []byte("x")}, "d": {Mode: fs.ModeDir | 0o755}},
			args:  []string{"--target-directory=d", "a"},
			want:  map[string]string{"d/a": "x"},
		},
		{
			name:  "--no-target-directory",
			files: fsx.MemFS{"a": {Data: []byte("x")}, "d": {Mode: fs.ModeDir | 0o755}},
			args:  []string{"--no-target-directory", "a", "f"},
			want:  map[string]string{"f": "x"},
		},
		{
			name:     "--compare",
			files:    fsx.MemFS{"a": {Data: []byte("x")}, "b": {Data: []byte("x"), Mode: 0o755}},
			args:     []string{"--compare", "a", "b"},
			wantMode: map[string]fs.FileMode{"b": 0o755},
		},
		{
			name:     "--preserve-timestamps",
			files:    fsx.MemFS{"a": {Data: []byte("x"), ModTime: oldTime, Mode: 0o644}},
			args:     []string{"--preserve-timestamps", "a", "b"},
			wantTime: map[string]time.Time{"b": oldTime},
		},
		{
			name:  "--suffix",
			files: fsx.MemFS{"a": {Data: []byte("new")}, "b": {Data: []byte("old")}},
			args:  []string{"--backup", "--suffix=.bak", "a", "b"},
			want:  map[string]string{"b": "new", "b.bak": "old"},
		},
		{
			name:     "an unknown long option is a usage error",
			files:    fsx.MemFS{},
			args:     []string{"--bogus"},
			wantErr:  "install: unrecognized option '--bogus'\nTry 'install --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an unknown short option is a usage error",
			files:    fsx.MemFS{},
			args:     []string{"-x"},
			wantErr:  "install: invalid option -- 'x'\nTry 'install --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an argument to a flag is a usage error",
			files:    fsx.MemFS{},
			args:     []string{"--verbose=yes"},
			wantErr:  "install: option '--verbose=yes' doesn't allow an argument\nTry 'install --help' for more information.\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunIgnoredOption(t *testing.T) {
	runCase(t, testCase{
		name:     "-c is ignored",
		files:    fsx.MemFS{"a": {Data: []byte("x")}},
		args:     []string{"-c", "a", "b"},
		want:     map[string]string{"b": "x"},
		wantMode: map[string]fs.FileMode{"b": 0o755},
	})
}

func TestRunOptionsAfterOperands(t *testing.T) {
	runCase(t, testCase{
		name:    "an option after an operand is an operand",
		files:   fsx.MemFS{"a": {Data: []byte("x")}, "-v": {Data: []byte("y")}, "d": {Mode: fs.ModeDir | 0o755}},
		args:    []string{"a", "-v", "d"},
		want:    map[string]string{"d/a": "x", "d/-v": "y"},
		wantOut: "",
	})
}

func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rt := &coreutils.Runtime{Stderr: io.Discard, FS: fsx.MemFS{"a": {Data: []byte("x")}}}
	err := New(rt).Run(ctx, "a", "b")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestCompareContext(t *testing.T) {
	tests := []struct {
		name  string
		files fsx.MemFS
	}{
		{
			name:  "empty files",
			files: fsx.MemFS{"a": {Mode: 0o755}, "b": {Mode: 0o755}},
		},
		{
			name:  "files with content",
			files: fsx.MemFS{"a": {Data: []byte("x"), Mode: 0o755}, "b": {Data: []byte("x"), Mode: 0o755}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			p := &Prog{Runtime: &coreutils.Runtime{FS: test.files}}
			fi, err := fs.Stat(test.files, "a")
			if err != nil {
				t.Fatal(err)
			}
			dfi, err := fs.Stat(test.files, "b")
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.upToDate(ctx, pair{src: "a", dst: "b"}, fi, dfi, settings{fileMode: 0o755})
			if !errors.Is(err, context.Canceled) {
				t.Errorf("upToDate() error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestCompareContextFinalRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	files := cancelFS{FS: fsx.MemFS{"a": {Data: []byte("x"), Mode: 0o755}, "b": {Data: []byte("x"), Mode: 0o755}}, cancel: cancel}

	p := &Prog{Runtime: &coreutils.Runtime{FS: files}}
	fi, err := fs.Stat(files.FS, "a")
	if err != nil {
		t.Fatal(err)
	}
	dfi, err := fs.Stat(files.FS, "b")
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.upToDate(ctx, pair{src: "a", dst: "b"}, fi, dfi, settings{fileMode: 0o755})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("upToDate() error = %v, want context.Canceled", err)
	}
}

// cancelFS cancels its context when the second read of the destination file
// happens, the one which checks for extra bytes after the compared ones.
type cancelFS struct {
	fsx.FS
	cancel context.CancelFunc
}

func (f cancelFS) Open(name string) (fs.File, error) {
	file, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	if name != "b" {
		return file, nil
	}
	return &cancelFile{cancel: f.cancel, File: file}, nil
}

// cancelFile cancels the context of the enclosing test on its second read.
type cancelFile struct {
	fs.File
	cancel context.CancelFunc
	reads  int
}

func (f *cancelFile) Read(p []byte) (int, error) {
	f.reads++
	if f.reads == 2 {
		f.cancel()
	}
	return f.File.Read(p)
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
		want := "install (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
