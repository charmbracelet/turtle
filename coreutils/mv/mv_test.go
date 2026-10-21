package mv

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
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
	fsys     fsx.FS
	wd       string
	stdin    string
	args     []string
	want     map[string]string
	wantMode map[string]fs.FileMode
	wantTime map[string]time.Time
	present  []string
	absent   []string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	fsys := test.fsys
	if fsys == nil {
		fsys = test.files
	}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      strings.NewReader(test.stdin),
		Stdout:     io.Discard,
		Stderr:     stderr,
		FS:         fsys,
		WorkingDir: test.wd,
	}

	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
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
		if got, ok := fsx.Atime(fi); ok && !got.Equal(want) {
			t.Errorf("%s access time = %v, want %v", name, got, want)
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

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			name:     "moves a file to a new name",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello"), Mode: 0o604, ModTime: oldTime}},
			args:     []string{"a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "hello"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o604},
			wantTime: map[string]time.Time{"b.txt": oldTime},
			absent:   []string{"a.txt"},
		},
		{
			name:   "overwrites an existing file",
			files:  fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:   []string{"a.txt", "b.txt"},
			want:   map[string]string{"b.txt": "new"},
			absent: []string{"a.txt"},
		},
		{
			name:    "moves a file into an existing directory",
			files:   fsx.MemFS{"a.txt": {Data: []byte("hello")}, "d/keep.txt": {Data: []byte("keep")}},
			args:    []string{"a.txt", "d"},
			want:    map[string]string{"d/a.txt": "hello"},
			present: []string{"d/keep.txt"},
			absent:  []string{"a.txt"},
		},
		{
			name:   "moves multiple files into a directory",
			files:  fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}, "d/keep.txt": {}},
			args:   []string{"a.txt", "b.txt", "d"},
			want:   map[string]string{"d/a.txt": "a", "d/b.txt": "b"},
			absent: []string{"a.txt", "b.txt"},
		},
		{
			name:  "a trailing slash on the target is ignored",
			files: fsx.MemFS{"a.txt": {Data: []byte("hello")}, "d/keep.txt": {}},
			args:  []string{"a.txt", "d/"},
			want:  map[string]string{"d/a.txt": "hello"},
		},
		{
			name:   "a path with a directory component moves to its base name",
			files:  fsx.MemFS{"sub/a.txt": {Data: []byte("hello")}, "d/keep.txt": {}},
			args:   []string{"sub/a.txt", "d"},
			want:   map[string]string{"d/a.txt": "hello"},
			absent: []string{"sub/a.txt"},
		},
		{
			name:     "keeps moving after a failing source",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}, "d/keep.txt": {}},
			args:     []string{"missing.txt", "a.txt", "d"},
			want:     map[string]string{"d/a.txt": "hello"},
			wantErr:  "mv: cannot stat 'missing.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing source is an error",
			files:    fsx.MemFS{},
			args:     []string{"missing.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "mv: cannot stat 'missing.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing destination parent is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt", "nodir/b.txt"},
			present:  []string{"a.txt"},
			absent:   []string{"nodir"},
			wantErr:  "mv: cannot move 'a.txt' to 'nodir/b.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a destination under a file is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}, "f.txt": {Data: []byte("y")}},
			args:     []string{"a.txt", "f.txt/b.txt"},
			present:  []string{"a.txt", "f.txt"},
			wantErr:  "mv: cannot stat 'f.txt/b.txt': not a directory\n",
			wantExit: 1,
		},
		{
			name:   "moves a directory with its contents",
			files:  fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			args:   []string{"d", "e"},
			want:   map[string]string{"e/f.txt": "f", "e/sub/g.txt": "g"},
			absent: []string{"d"},
		},
		{
			name:    "moves a directory into an existing directory",
			files:   fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "e/keep.txt": {Data: []byte("keep")}},
			args:    []string{"d", "e"},
			want:    map[string]string{"e/d/f.txt": "f"},
			present: []string{"e/keep.txt"},
			absent:  []string{"d"},
		},
		{
			name:    "replaces an empty directory with a directory",
			files:   fsx.MemFS{"d1/f.txt": {Data: []byte("f")}, "e/d1": {Mode: fs.ModeDir}},
			args:    []string{"d1", "e"},
			want:    map[string]string{"e/d1/f.txt": "f"},
			absent:  []string{"d1"},
			present: []string{"e"},
		},
		{
			name:     "a directory onto a non-empty directory is an error",
			files:    fsx.MemFS{"d1/f.txt": {Data: []byte("f")}, "e/d1/keep.txt": {Data: []byte("keep")}},
			args:     []string{"d1", "e"},
			present:  []string{"d1/f.txt", "e/d1/keep.txt"},
			wantErr:  "mv: cannot move 'd1' to 'e/d1': directory not empty\n",
			wantExit: 1,
		},
		{
			name:     "a file onto a directory is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}, "d/a.txt": {Mode: fs.ModeDir}},
			args:     []string{"a.txt", "d"},
			present:  []string{"a.txt"},
			wantErr:  "mv: cannot overwrite directory 'd/a.txt' with non-directory 'a.txt'\n",
			wantExit: 1,
		},
		{
			name:     "a directory onto a file is an error",
			files:    fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "file.txt": {Data: []byte("x")}},
			args:     []string{"d", "file.txt"},
			present:  []string{"d/f.txt", "file.txt"},
			wantErr:  "mv: cannot overwrite non-directory 'file.txt' with directory 'd'\n",
			wantExit: 1,
		},
		{
			name:     "a file onto itself is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt", "a.txt"},
			present:  []string{"a.txt"},
			wantErr:  "mv: 'a.txt' and 'a.txt' are the same file\n",
			wantExit: 1,
		},
		{
			name:     "an equivalent path is still the same file",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt", "./a.txt"},
			present:  []string{"a.txt"},
			wantErr:  "mv: 'a.txt' and './a.txt' are the same file\n",
			wantExit: 1,
		},
		{
			name:     "a file into the directory holding it is the same file",
			files:    fsx.MemFS{"d/a.txt": {Data: []byte("x")}},
			args:     []string{"d/a.txt", "d"},
			present:  []string{"d/a.txt"},
			wantErr:  "mv: 'd/a.txt' and 'd/a.txt' are the same file\n",
			wantExit: 1,
		},
		{
			name:     "a file into the working directory is the same file",
			files:    fsx.MemFS{"f.txt": {Data: []byte("x")}},
			args:     []string{"f.txt", "."},
			present:  []string{"f.txt"},
			wantErr:  "mv: 'f.txt' and 'f.txt' are the same file\n",
			wantExit: 1,
		},
		{
			name:     "a directory into itself is an error",
			files:    fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:     []string{"d", "d"},
			present:  []string{"d/f.txt"},
			wantErr:  "mv: cannot move a directory, 'd', into itself, 'd/d'\n",
			wantExit: 1,
		},
		{
			name:     "a directory into its own subdirectory is an error",
			files:    fsx.MemFS{"d/sub/x": {Data: []byte("x")}},
			args:     []string{"d", "d/sub"},
			present:  []string{"d/sub/x"},
			wantErr:  "mv: cannot move a directory, 'd', into itself, 'd/sub/d'\n",
			wantExit: 1,
		},
		{
			name:   "a trailing slash on the source is ignored",
			files:  fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "e/keep.txt": {}},
			args:   []string{"d/", "e"},
			want:   map[string]string{"e/d/f.txt": "f"},
			absent: []string{"d"},
		},
		{
			name:   "a trailing dot on the source names the directory",
			files:  fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:   []string{"d/.", "e"},
			want:   map[string]string{"e/f.txt": "f"},
			absent: []string{"d"},
		},
		{
			name:    "moves an empty file",
			files:   fsx.MemFS{"a.txt": {}},
			args:    []string{"a.txt", "b.txt"},
			want:    map[string]string{"b.txt": ""},
			absent:  []string{"a.txt"},
			present: []string{"b.txt"},
		},
		{
			name:   "moves contents with any bytes",
			files:  fsx.MemFS{"a.txt": {Data: []byte{0, 1, 2, 0xff, '\n'}}},
			args:   []string{"a.txt", "b.txt"},
			want:   map[string]string{"b.txt": "\x00\x01\x02\xff\n"},
			absent: []string{"a.txt"},
		},
		{
			name:   "a dash names a file",
			files:  fsx.MemFS{"-": {Data: []byte("dash")}},
			args:   []string{"-", "b.txt"},
			want:   map[string]string{"b.txt": "dash"},
			absent: []string{"-"},
		},
		{
			name:   "options end at a double dash",
			files:  fsx.MemFS{"-i": {Data: []byte("dash i")}},
			args:   []string{"--", "-i", "b.txt"},
			want:   map[string]string{"b.txt": "dash i"},
			absent: []string{"-i"},
		},
		{
			name:   "relative paths resolve inside the working directory",
			wd:     "base",
			files:  fsx.MemFS{"base/a.txt": {Data: []byte("rel")}},
			args:   []string{"a.txt", "b.txt"},
			want:   map[string]string{"base/b.txt": "rel"},
			absent: []string{"base/a.txt"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDirectoryModeAndTimes(t *testing.T) {
	tests := []testCase{
		{
			name:     "the moved tree keeps its modes and times",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o750, ModTime: oldTime}, "d/f.txt": {Data: []byte("f"), Mode: 0o640, ModTime: oldTime}},
			args:     []string{"d", "e"},
			want:     map[string]string{"e/f.txt": "f"},
			wantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o750, "e/f.txt": 0o640},
			wantTime: map[string]time.Time{"e": oldTime, "e/f.txt": oldTime},
		},
		{
			name:     "nested directories keep their modes and times",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o755}, "d/sub": {Mode: fs.ModeDir | 0o710, ModTime: otherTime}, "d/sub/g.txt": {Data: []byte("g"), ModTime: oldTime}},
			args:     []string{"d", "e"},
			want:     map[string]string{"e/sub/g.txt": "g"},
			wantMode: map[string]fs.FileMode{"e/sub": fs.ModeDir | 0o710},
			wantTime: map[string]time.Time{"e/sub": otherTime, "e/sub/g.txt": oldTime},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunInteractive(t *testing.T) {
	tests := []testCase{
		{
			name:    "prompts and overwrites when affirmative",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:   "y\n",
			args:    []string{"-i", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "new"},
			absent:  []string{"a.txt"},
			wantErr: "mv: overwrite 'b.txt'? ",
		},
		{
			name:    "a capital answer is affirmative too",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:   "Y\n",
			args:    []string{"-i", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "new"},
			absent:  []string{"a.txt"},
			wantErr: "mv: overwrite 'b.txt'? ",
		},
		{
			name:    "declines on a negative answer",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:   "n\n",
			args:    []string{"-i", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "old"},
			present: []string{"a.txt"},
			wantErr: "mv: overwrite 'b.txt'? ",
		},
		{
			name:    "declines on any other answer",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:   "maybe\n",
			args:    []string{"-i", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "old"},
			present: []string{"a.txt"},
			wantErr: "mv: overwrite 'b.txt'? ",
		},
		{
			name:    "declines at the end of the input",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:    []string{"-i", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "old"},
			present: []string{"a.txt"},
			wantErr: "mv: overwrite 'b.txt'? ",
		},
		{
			name:   "doesn't prompt when the destination doesn't exist",
			files:  fsx.MemFS{"a.txt": {Data: []byte("new")}},
			args:   []string{"-i", "a.txt", "b.txt"},
			want:   map[string]string{"b.txt": "new"},
			absent: []string{"a.txt"},
		},
		{
			name:    "prompts for every existing destination",
			files:   fsx.MemFS{"a1": {Data: []byte("1")}, "a2": {Data: []byte("2")}, "d/a1": {Data: []byte("old1")}, "d/a2": {Data: []byte("old2")}},
			stdin:   "y\nn\n",
			args:    []string{"-i", "a1", "a2", "d"},
			want:    map[string]string{"d/a1": "1", "d/a2": "old2"},
			absent:  []string{"a1"},
			present: []string{"a2"},
			wantErr: "mv: overwrite 'd/a1'? mv: overwrite 'd/a2'? ",
		},
		{
			name:     "a directory into itself doesn't prompt",
			files:    fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:     []string{"-i", "d", "d"},
			present:  []string{"d/f.txt"},
			wantErr:  "mv: cannot move a directory, 'd', into itself, 'd/d'\n",
			wantExit: 1,
		},
		{
			name:     "a destination directory doesn't prompt",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}, "d/a.txt": {Mode: fs.ModeDir}},
			args:     []string{"-i", "a.txt", "d"},
			present:  []string{"a.txt"},
			wantErr:  "mv: cannot overwrite directory 'd/a.txt' with non-directory 'a.txt'\n",
			wantExit: 1,
		},
		{
			name:   "f cancels an earlier i",
			files:  fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:   []string{"-if", "a.txt", "b.txt"},
			want:   map[string]string{"b.txt": "new"},
			absent: []string{"a.txt"},
		},
		{
			name:    "i cancels an earlier f",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:   "n\n",
			args:    []string{"-fi", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "old"},
			present: []string{"a.txt"},
			wantErr: "mv: overwrite 'b.txt'? ",
		},
		{
			name:    "the last of the i options wins",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:   "n\n",
			args:    []string{"-i", "-i", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "old"},
			present: []string{"a.txt"},
			wantErr: "mv: overwrite 'b.txt'? ",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunInteractiveWithoutStdin(t *testing.T) {
	files := fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stderr: stderr, FS: files}

	if err := New(rt).Run(t.Context(), "-i", "a.txt", "b.txt"); err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
	if want := "mv: overwrite 'b.txt'? "; stderr.String() != want {
		t.Errorf("Run() stderr = %q, want %q", stderr.String(), want)
	}
	if got, err := fs.ReadFile(files, "b.txt"); err != nil || string(got) != "old" {
		t.Errorf("ReadFile(%q) = %q, %v, want %q", "b.txt", got, err, "old")
	}
	if _, err := fs.Stat(files, "a.txt"); err != nil {
		t.Errorf("Stat(%q) error = %v, want it to still exist", "a.txt", err)
	}
}

// crossFS wraps a MemFS whose renames always fail as cross-device moves, so
// mv falls back to duplicating the file and removing the source.
type crossFS struct {
	fsx.MemFS
}

// Rename implements fsx.RenameFS.
func (m *crossFS) Rename(oldPath, newPath string) error {
	return &fs.PathError{Op: "rename", Path: oldPath, Err: exdev}
}

func TestRunCrossDevice(t *testing.T) {
	newCase := func(files fsx.MemFS, test testCase) testCase {
		test.files = files
		test.fsys = &crossFS{MemFS: files}
		return test
	}

	t.Run("moves a file", func(t *testing.T) {
		files := fsx.MemFS{"a.txt": {Data: []byte("hello"), Mode: 0o604, ModTime: oldTime}}
		runCase(t, newCase(files, testCase{
			args:     []string{"a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "hello"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o604},
			wantTime: map[string]time.Time{"b.txt": oldTime},
			absent:   []string{"a.txt"},
		}))
	})

	t.Run("overwrites an existing file", func(t *testing.T) {
		files := fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}}
		runCase(t, newCase(files, testCase{
			args:   []string{"a.txt", "b.txt"},
			want:   map[string]string{"b.txt": "new"},
			absent: []string{"a.txt"},
		}))
	})

	t.Run("moves a directory with its contents", func(t *testing.T) {
		files := fsx.MemFS{"d": {Mode: fs.ModeDir | 0o750, ModTime: oldTime}, "d/f.txt": {Data: []byte("f"), ModTime: oldTime}, "d/sub/g.txt": {Data: []byte("g")}}
		runCase(t, newCase(files, testCase{
			args:     []string{"d", "e"},
			want:     map[string]string{"e/f.txt": "f", "e/sub/g.txt": "g"},
			wantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o750},
			wantTime: map[string]time.Time{"e": oldTime, "e/f.txt": oldTime},
			absent:   []string{"d"},
		}))
	})

	t.Run("moves a directory into an existing directory", func(t *testing.T) {
		files := fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "e/keep.txt": {Data: []byte("keep")}}
		runCase(t, newCase(files, testCase{
			args:    []string{"d", "e"},
			want:    map[string]string{"e/d/f.txt": "f"},
			present: []string{"e/keep.txt"},
			absent:  []string{"d"},
		}))
	})

	t.Run("replaces an empty destination directory", func(t *testing.T) {
		files := fsx.MemFS{"d1/f.txt": {Data: []byte("f")}, "e/d1": {Mode: fs.ModeDir}}
		runCase(t, newCase(files, testCase{
			args:   []string{"d1", "e"},
			want:   map[string]string{"e/d1/f.txt": "f"},
			absent: []string{"d1"},
		}))
	})

	t.Run("a non-empty destination directory is an error", func(t *testing.T) {
		files := fsx.MemFS{"d1/f.txt": {Data: []byte("f")}, "e/d1/keep.txt": {Data: []byte("keep")}}
		runCase(t, newCase(files, testCase{
			args:     []string{"d1", "e"},
			present:  []string{"d1/f.txt", "e/d1/keep.txt"},
			wantErr:  "mv: cannot remove 'e/d1': directory not empty\n",
			wantExit: 1,
		}))
	})

	t.Run("a failing copy keeps the source", func(t *testing.T) {
		files := fsx.MemFS{"a.txt": {Data: []byte("x")}}
		runCase(t, newCase(files, testCase{
			args:     []string{"a.txt", "nodir/b.txt"},
			present:  []string{"a.txt"},
			absent:   []string{"nodir"},
			wantErr:  "mv: cannot create 'nodir/b.txt': file does not exist\n",
			wantExit: 1,
		}))
	})

	t.Run("prompts before copying", func(t *testing.T) {
		files := fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}}
		runCase(t, newCase(files, testCase{
			stdin:   "y\n",
			args:    []string{"-i", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "new"},
			absent:  []string{"a.txt"},
			wantErr: "mv: overwrite 'b.txt'? ",
		}))
	})
}

func TestRunLongOptions(t *testing.T) {
	tests := []testCase{
		{
			name:    "interactive prompts and overwrites when affirmative",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:   "y\n",
			args:    []string{"--interactive", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "new"},
			absent:  []string{"a.txt"},
			wantErr: "mv: overwrite 'b.txt'? ",
		},
		{
			name:    "force cancels an earlier interactive",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:    []string{"--interactive", "--force", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "new"},
			absent:  []string{"a.txt"},
			wantErr: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "missing file operand",
			args:     []string{},
			wantErr:  "mv: missing file operand\n",
			wantExit: 2,
		},
		{
			name:     "missing destination operand",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt"},
			wantErr:  "mv: missing destination file operand after 'a.txt'\n",
			wantExit: 2,
		},
		{
			name:     "invalid option",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"-x", "a.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "mv: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "the R option is not supported",
			files:    fsx.MemFS{"d/f.txt": {}},
			args:     []string{"-R", "d", "e"},
			absent:   []string{"e"},
			wantErr:  "mv: invalid option -- 'R'\n",
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"--bogus", "a.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "mv: unrecognized option '--bogus'\n",
			wantExit: 2,
		},
		{
			name:     "an empty source",
			files:    fsx.MemFS{"a.txt": {}},
			args:     []string{"", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "mv: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "an empty target",
			files:    fsx.MemFS{"a.txt": {}},
			args:     []string{"a.txt", ""},
			present:  []string{"a.txt"},
			wantErr:  "mv: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "a missing directory as target",
			files:    fsx.MemFS{"a.txt": {}, "c.txt": {}},
			args:     []string{"a.txt", "c.txt", "missing"},
			present:  []string{"a.txt", "c.txt"},
			wantErr:  "mv: target 'missing' is not a directory\n",
			wantExit: 1,
		},
		{
			name:     "a file as target with multiple sources",
			files:    fsx.MemFS{"a.txt": {}, "c.txt": {}, "b.txt": {Data: []byte("old")}},
			args:     []string{"a.txt", "c.txt", "b.txt"},
			present:  []string{"a.txt", "c.txt", "b.txt"},
			wantErr:  "mv: target 'b.txt' is not a directory\n",
			wantExit: 1,
		},
		{
			name:     "a trailing slash requires a directory",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt", "missing/"},
			present:  []string{"a.txt"},
			absent:   []string{"missing"},
			wantErr:  "mv: target 'missing/' is not a directory\n",
			wantExit: 1,
		},
		{
			name:     "a trailing slash onto a file is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}, "b.txt": {Data: []byte("y")}},
			args:     []string{"a.txt", "b.txt/"},
			present:  []string{"a.txt", "b.txt"},
			wantErr:  "mv: target 'b.txt/' is not a directory\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rt := &coreutils.Runtime{Stderr: io.Discard, FS: fsx.MemFS{}}
	err := New(rt).Run(ctx, "a.txt", "b.txt")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}
