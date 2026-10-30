package cp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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

	folder := test.fsys
	if folder == nil {
		folder = test.files
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      strings.NewReader(test.stdin),
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         folder,
		WorkingDir: test.wd,
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
		if got, ok := fsx.Atime(fi); ok && !got.Equal(want) {
			t.Errorf("%s access time = %v, want %v", name, got, want)
		}
	}
	for _, name := range test.present {
		if _, err := fs.Stat(test.files, name); err != nil {
			t.Errorf("Stat(%q) error = %v, want it to still exist", name, err)
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
			name:    "copies a file to a new name",
			files:   fsx.MemFS{"a.txt": {Data: []byte("hello")}},
			args:    []string{"a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "hello"},
			present: []string{"a.txt"},
		},
		{
			name:  "overwrites an existing file",
			files: fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:  []string{"a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "new"},
		},
		{
			name:    "copies into an existing directory",
			files:   fsx.MemFS{"a.txt": {Data: []byte("hello")}, "d/keep.txt": {Data: []byte("keep")}},
			args:    []string{"a.txt", "d"},
			want:    map[string]string{"d/a.txt": "hello"},
			present: []string{"d/keep.txt"},
		},
		{
			name:  "copies multiple files into a directory",
			files: fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}, "d/keep.txt": {}},
			args:  []string{"a.txt", "b.txt", "d"},
			want:  map[string]string{"d/a.txt": "a", "d/b.txt": "b"},
		},
		{
			name:  "a trailing slash on the target is ignored",
			files: fsx.MemFS{"a.txt": {Data: []byte("hello")}, "d/keep.txt": {}},
			args:  []string{"a.txt", "d/"},
			want:  map[string]string{"d/a.txt": "hello"},
		},
		{
			name:  "a path with a directory component is copied to its base name",
			files: fsx.MemFS{"sub/a.txt": {Data: []byte("hello")}, "d/keep.txt": {}},
			args:  []string{"sub/a.txt", "d"},
			want:  map[string]string{"d/a.txt": "hello"},
		},
		{
			name:     "keeps copying after a failing source",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}, "d/keep.txt": {}},
			args:     []string{"missing.txt", "a.txt", "d"},
			want:     map[string]string{"d/a.txt": "hello"},
			wantErr:  "cp: cannot stat 'missing.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing source is an error",
			files:    fsx.MemFS{},
			args:     []string{"missing.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "cp: cannot stat 'missing.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing destination parent is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt", "nodir/b.txt"},
			absent:   []string{"nodir"},
			wantErr:  "cp: cannot create 'nodir/b.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "the umask applies to new files",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o666}},
			umask:    0o022,
			args:     []string{"a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "x"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o644},
		},
		{
			name:     "the source mode is used without a umask",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o600}},
			args:     []string{"a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o600},
		},
		{
			name:     "the special bits are cleared when creating",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: fs.ModeSetuid | 0o755}},
			args:     []string{"a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o755},
		},
		{
			name:     "overwriting keeps the destination mode",
			files:    fsx.MemFS{"a.txt": {Data: []byte("new"), Mode: 0o777}, "b.txt": {Data: []byte("old"), Mode: 0o600}},
			args:     []string{"a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "new"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o600},
		},
		{
			name:  "copies an empty file",
			files: fsx.MemFS{"a.txt": {}},
			args:  []string{"a.txt", "b.txt"},
			want:  map[string]string{"b.txt": ""},
		},
		{
			name:  "copies contents with any bytes",
			files: fsx.MemFS{"a.txt": {Data: []byte{0, 1, 2, 0xff, '\n'}}},
			args:  []string{"a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "\x00\x01\x02\xff\n"},
		},
		{
			name:  "a dash names a file",
			files: fsx.MemFS{"-": {Data: []byte("dash")}},
			args:  []string{"-", "b.txt"},
			want:  map[string]string{"b.txt": "dash"},
		},
		{
			name:  "options end at a double dash",
			files: fsx.MemFS{"-p": {Data: []byte("dash p")}},
			args:  []string{"--", "-p", "b.txt"},
			want:  map[string]string{"b.txt": "dash p"},
		},
		{
			name:     "glued short flags are accepted",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o640}},
			args:     []string{"-fip", "a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "x"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o640},
		},
		{
			name:     "a file onto itself is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt", "a.txt"},
			wantErr:  "cp: 'a.txt' and 'a.txt' are the same file\n",
			wantExit: 1,
		},
		{
			name:     "an equivalent path is still the same file",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt", "./a.txt"},
			wantErr:  "cp: 'a.txt' and './a.txt' are the same file\n",
			wantExit: 1,
		},
		{
			name:     "a directory source without -R is an error",
			files:    fsx.MemFS{"d/f.txt": {}},
			args:     []string{"d", "e"},
			absent:   []string{"e"},
			wantErr:  "cp: -R not specified; omitting directory 'd'\n",
			wantExit: 1,
		},
		{
			name:     "a directory source into an existing directory without -R is an error",
			files:    fsx.MemFS{"d/f.txt": {}, "e/keep.txt": {}},
			args:     []string{"d", "e"},
			absent:   []string{"e/d"},
			wantErr:  "cp: -R not specified; omitting directory 'd'\n",
			wantExit: 1,
		},
		{
			name:  "a trailing dot on the target is ignored",
			files: fsx.MemFS{"a.txt": {Data: []byte("x")}, "d/keep.txt": {}},
			args:  []string{"a.txt", "d/."},
			want:  map[string]string{"d/a.txt": "x"},
		},
		{
			name:  "relative paths resolve inside the working directory",
			wd:    "base",
			files: fsx.MemFS{"base/a.txt": {Data: []byte("rel")}},
			args:  []string{"a.txt", "b.txt"},
			want:  map[string]string{"base/b.txt": "rel"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRecursive(t *testing.T) {
	tests := []testCase{
		{
			name:  "copies a tree to a new name",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			args:  []string{"-R", "d", "e"},
			want:  map[string]string{"e/f.txt": "f", "e/sub/g.txt": "g"},
		},
		{
			name:    "copies a tree into an existing directory",
			files:   fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "e/keep.txt": {Data: []byte("keep")}},
			args:    []string{"-R", "d", "e"},
			want:    map[string]string{"e/d/f.txt": "f"},
			present: []string{"e/keep.txt"},
		},
		{
			name:  "copies multiple trees",
			files: fsx.MemFS{"d1/f": {Data: []byte("1")}, "d2/g": {Data: []byte("2")}, "e/keep.txt": {}},
			args:  []string{"-R", "d1", "d2", "e"},
			want:  map[string]string{"e/d1/f": "1", "e/d2/g": "2"},
		},
		{
			name:  "copies trees and files together",
			files: fsx.MemFS{"d/f": {}, "g.txt": {Data: []byte("g")}, "e/keep.txt": {}},
			args:  []string{"-R", "d", "g.txt", "e"},
			want:  map[string]string{"e/d/f": "", "e/g.txt": "g"},
		},
		{
			name:  "merges into an existing tree",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "e/d/old.txt": {Data: []byte("old")}},
			args:  []string{"-R", "d", "e"},
			want:  map[string]string{"e/d/f.txt": "f", "e/d/old.txt": "old"},
		},
		{
			name:     "created directories get the source mode",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o750}, "d/f.txt": {}},
			args:     []string{"-R", "d", "e"},
			wantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o750},
		},
		{
			name:     "the umask applies to created directories",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o777}, "d/f.txt": {}},
			umask:    0o027,
			args:     []string{"-R", "d", "e"},
			wantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o750},
		},
		{
			name:     "a restrictive umask still allows the contents to be copied",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o755}, "d/f.txt": {Data: []byte("x")}},
			umask:    0o177,
			args:     []string{"-R", "d", "e"},
			want:     map[string]string{"e/f.txt": "x"},
			wantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o600},
		},
		{
			name:     "files in the tree follow the same umask rules",
			files:    fsx.MemFS{"d/f.txt": {Data: []byte("x"), Mode: 0o666}, "d/sub/g.txt": {Data: []byte("g"), Mode: 0o640}},
			umask:    0o022,
			args:     []string{"-R", "d", "e"},
			wantMode: map[string]fs.FileMode{"e/f.txt": 0o644, "e/sub/g.txt": 0o640},
		},
		{
			name:     "an existing destination directory is left alone",
			files:    fsx.MemFS{"d/f.txt": {Data: []byte("new")}, "e/d": {Mode: fs.ModeDir | 0o700}, "e/d/old.txt": {Data: []byte("old")}},
			args:     []string{"-R", "d", "e"},
			want:     map[string]string{"e/d/f.txt": "new", "e/d/old.txt": "old"},
			wantMode: map[string]fs.FileMode{"e/d": fs.ModeDir | 0o700},
		},
		{
			name:     "copies an empty directory",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o755}},
			args:     []string{"-R", "d", "e"},
			wantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o755},
		},
		{
			name:  "a trailing dot copies the contents",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			args:  []string{"-R", "d/.", "e"},
			want:  map[string]string{"e/f.txt": "f", "e/sub/g.txt": "g"},
		},
		{
			name:    "a trailing dot copies into an existing directory",
			files:   fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "e/keep.txt": {}},
			args:    []string{"-R", "d/.", "e"},
			want:    map[string]string{"e/f.txt": "f"},
			present: []string{"e/keep.txt"},
		},
		{
			name:  "a trailing slash on the source is ignored",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "e/keep.txt": {}},
			args:  []string{"-R", "d/", "e"},
			want:  map[string]string{"e/d/f.txt": "f"},
		},
		{
			name:     "the working directory into its own subdirectory is an error",
			files:    fsx.MemFS{"f.txt": {Data: []byte("f")}},
			args:     []string{"-R", ".", "e"},
			absent:   []string{"e"},
			wantErr:  "cp: cannot copy a directory, '.', into itself, 'e'\n",
			wantExit: 1,
		},
		{
			name:  "a trailing slash on the target creates the directory",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:  []string{"-R", "d", "e/"},
			want:  map[string]string{"e/f.txt": "f"},
		},
		{
			name:  "a file source is copied like without -R",
			files: fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:  []string{"-R", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "new"},
		},
		{
			name:  "the r option copies a tree like -R",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			args:  []string{"-r", "d", "e"},
			want:  map[string]string{"e/f.txt": "f", "e/sub/g.txt": "g"},
		},
		{
			name:     "a directory into itself is an error",
			files:    fsx.MemFS{"d/f.txt": {}},
			args:     []string{"-R", "d", "d"},
			present:  []string{"d/f.txt"},
			wantErr:  "cp: cannot copy a directory, 'd', into itself, 'd/d'\n",
			wantExit: 1,
		},
		{
			name:     "a directory into its own subdirectory is an error",
			files:    fsx.MemFS{"d/sub/x": {}},
			args:     []string{"-R", "d", "d/sub"},
			present:  []string{"d/sub/x"},
			wantErr:  "cp: cannot copy a directory, 'd', into itself, 'd/sub/d'\n",
			wantExit: 1,
		},
		{
			name:     "a directory over a file is an error",
			files:    fsx.MemFS{"d/f.txt": {}, "f.txt": {Data: []byte("x")}},
			args:     []string{"-R", "d", "f.txt"},
			want:     map[string]string{"f.txt": "x"},
			wantErr:  "cp: cannot overwrite non-directory 'f.txt' with directory 'd'\n",
			wantExit: 1,
		},
		{
			name:     "a file over a directory in the tree is an error",
			files:    fsx.MemFS{"d/x": {Data: []byte("x")}, "d/y": {Data: []byte("y")}, "e/d/x/keep.txt": {Data: []byte("keep")}},
			args:     []string{"-R", "d", "e"},
			want:     map[string]string{"e/d/y": "y"},
			present:  []string{"e/d/x/keep.txt"},
			wantErr:  "cp: cannot overwrite directory 'e/d/x' with non-directory 'd/x'\n",
			wantExit: 1,
		},
		{
			name:     "a missing source is an error",
			files:    fsx.MemFS{"e/keep.txt": {}},
			args:     []string{"-R", "missing", "e"},
			wantErr:  "cp: cannot stat 'missing': file does not exist\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunPreserve(t *testing.T) {
	tests := []testCase{
		{
			name:     "preserves the mode",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o604}},
			args:     []string{"-p", "a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o604},
		},
		{
			name:     "the umask doesn't apply with -p",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o640}},
			umask:    0o022,
			args:     []string{"-p", "a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o640},
		},
		{
			name:     "preserves the special bits",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: fs.ModeSetuid | fs.ModeSetgid | 0o755}},
			args:     []string{"-p", "a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": fs.ModeSetuid | fs.ModeSetgid | 0o755},
		},
		{
			name:     "preserves the times",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), ModTime: oldTime}},
			args:     []string{"-p", "a.txt", "b.txt"},
			wantTime: map[string]time.Time{"b.txt": oldTime},
		},
		{
			name:     "preserves mode and times when overwriting",
			files:    fsx.MemFS{"a.txt": {Data: []byte("new"), Mode: 0o642, ModTime: oldTime}, "b.txt": {Data: []byte("old"), Mode: 0o600, ModTime: otherTime}},
			args:     []string{"-p", "a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "new"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o642},
			wantTime: map[string]time.Time{"b.txt": oldTime},
		},
		{
			name:     "preserves the mode and times of created directories",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o751, ModTime: oldTime}, "d/f.txt": {Data: []byte("f"), Mode: 0o644, ModTime: oldTime}},
			args:     []string{"-Rp", "d", "e"},
			want:     map[string]string{"e/f.txt": "f"},
			wantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o751, "e/f.txt": 0o644},
			wantTime: map[string]time.Time{"e": oldTime, "e/f.txt": oldTime},
		},
		{
			name:     "leaves an existing destination directory untouched",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o750, ModTime: oldTime}, "d/f.txt": {Data: []byte("f")}, "e/d": {Mode: fs.ModeDir | 0o700, ModTime: otherTime}},
			args:     []string{"-Rp", "d", "e"},
			want:     map[string]string{"e/d/f.txt": "f"},
			wantMode: map[string]fs.FileMode{"e/d": fs.ModeDir | 0o700},
			wantTime: map[string]time.Time{"e/d": otherTime},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSystemUmask(t *testing.T) {
	// The real file system applies the process umask to new entries, which
	// cp must respect instead of undoing it.
	tests := []struct {
		name     string
		files    fsx.MemFS
		umask    fs.FileMode
		args     []string
		wantMode map[string]fs.FileMode
	}{
		{
			name:     "created files keep the masked mode",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o666}},
			umask:    0o022,
			args:     []string{"a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o644},
		},
		{
			name:     "created directories keep the masked mode",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o777}, "d/f.txt": {}},
			umask:    0o027,
			args:     []string{"-R", "d", "e"},
			wantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o750},
		},
		{
			name:     "a file mode is duplicated exactly with -p",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o666}},
			umask:    0o022,
			args:     []string{"-p", "a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o666},
		},
		{
			name:     "a directory mode is duplicated exactly with -p",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o666}, "d/f.txt": {}},
			umask:    0o022,
			args:     []string{"-Rp", "d", "e"},
			wantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o666},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, testCase{
				files:    test.files,
				fsys:     &umaskFS{MemFS: test.files, umask: test.umask},
				args:     test.args,
				wantMode: test.wantMode,
			})
		})
	}
}

// umaskFS wraps a MemFS, applying a file mode creation mask to new entries,
// as the real file system does.
type umaskFS struct {
	fsx.MemFS
	umask fs.FileMode
}

// OpenFile implements fsx.WriteFS.
func (m *umaskFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	if flag&os.O_CREATE != 0 {
		perm &^= m.umask
	}
	return m.MemFS.OpenFile(name, flag, perm)
}

// Mkdir implements fsx.MkdirFS.
func (m *umaskFS) Mkdir(name string, perm fs.FileMode) error {
	return m.MemFS.Mkdir(name, perm&^m.umask)
}

func TestRunInteractive(t *testing.T) {
	tests := []testCase{
		{
			name:    "prompts and overwrites when affirmative",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:   "y\n",
			args:    []string{"-i", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "new"},
			wantErr: "cp: overwrite 'b.txt'? ",
		},
		{
			name:    "a capital answer is affirmative too",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:   "Y\n",
			args:    []string{"-i", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "new"},
			wantErr: "cp: overwrite 'b.txt'? ",
		},
		{
			name:    "declines on a negative answer",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:   "n\n",
			args:    []string{"-i", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "old"},
			wantErr: "cp: overwrite 'b.txt'? ",
		},
		{
			name:    "declines on any other answer",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:   "maybe\n",
			args:    []string{"-i", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "old"},
			wantErr: "cp: overwrite 'b.txt'? ",
		},
		{
			name:    "declines at the end of the input",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:    []string{"-i", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "old"},
			wantErr: "cp: overwrite 'b.txt'? ",
		},
		{
			name:  "doesn't prompt when the destination doesn't exist",
			files: fsx.MemFS{"a.txt": {Data: []byte("new")}},
			args:  []string{"-i", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "new"},
		},
		{
			name:    "prompts for every existing destination",
			files:   fsx.MemFS{"a1": {Data: []byte("1")}, "a2": {Data: []byte("2")}, "d/a1": {Data: []byte("old1")}, "d/a2": {Data: []byte("old2")}},
			stdin:   "y\nn\n",
			args:    []string{"-i", "a1", "a2", "d"},
			want:    map[string]string{"d/a1": "1", "d/a2": "old2"},
			wantErr: "cp: overwrite 'd/a1'? cp: overwrite 'd/a2'? ",
		},
		{
			name:    "prompts while copying a hierarchy",
			files:   fsx.MemFS{"d/f.txt": {Data: []byte("new")}, "e/d/f.txt": {Data: []byte("old")}, "e/d/keep.txt": {Data: []byte("keep")}},
			stdin:   "y\n",
			args:    []string{"-Ri", "d", "e"},
			want:    map[string]string{"e/d/f.txt": "new"},
			present: []string{"e/d/keep.txt"},
			wantErr: "cp: overwrite 'e/d/f.txt'? ",
		},
		{
			name:    "doesn't prompt for directories",
			files:   fsx.MemFS{"d/sub/f.txt": {Data: []byte("new")}, "e/d/sub/f.txt": {Data: []byte("old")}},
			stdin:   "n\n",
			args:    []string{"-Ri", "d", "e"},
			want:    map[string]string{"e/d/sub/f.txt": "old"},
			wantErr: "cp: overwrite 'e/d/sub/f.txt'? ",
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
	if want := "cp: overwrite 'b.txt'? "; stderr.String() != want {
		t.Errorf("Run() stderr = %q, want %q", stderr.String(), want)
	}
	if got, err := fs.ReadFile(files, "b.txt"); err != nil || string(got) != "old" {
		t.Errorf("ReadFile(%q) = %q, %v, want %q", "b.txt", got, err, "old")
	}
}

// failOpenFS wraps a MemFS, failing once the attempt to open a given file for
// writing, as a file without write permission would.
type failOpenFS struct {
	fsx.MemFS
	path   string
	failed bool
}

// OpenFile implements fsx.WriteFS.
func (m *failOpenFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	if name == m.path && !m.failed && flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		m.failed = true
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return m.MemFS.OpenFile(name, flag, perm)
}

func TestRunForce(t *testing.T) {
	t.Run("copies normally when the destination can be opened", func(t *testing.T) {
		runCase(t, testCase{
			files: fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:  []string{"-f", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "new"},
		})
	})

	t.Run("a destination that can't be opened is an error", func(t *testing.T) {
		files := fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}}
		runCase(t, testCase{
			files:    files,
			fsys:     &failOpenFS{MemFS: files, path: "b.txt"},
			args:     []string{"a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "old"},
			wantErr:  "cp: cannot create 'b.txt': permission denied\n",
			wantExit: 1,
		})
	})

	t.Run("f removes the destination and tries again", func(t *testing.T) {
		files := fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}}
		runCase(t, testCase{
			files: files,
			fsys:  &failOpenFS{MemFS: files, path: "b.txt"},
			args:  []string{"-f", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "new"},
		})
	})

	t.Run("a destination that can't be removed is still an error", func(t *testing.T) {
		runCase(t, testCase{
			files:    fsx.MemFS{"a.txt": {Data: []byte("new")}},
			args:     []string{"-f", "a.txt", "nodir/b.txt"},
			absent:   []string{"nodir"},
			wantErr:  "cp: cannot create 'nodir/b.txt': file does not exist\n",
			wantExit: 1,
		})
	})
}

func TestRunSymlinkOptions(t *testing.T) {
	tests := []testCase{
		{
			name:  "H is accepted with -R",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:  []string{"-RH", "d", "e"},
			want:  map[string]string{"e/f.txt": "f"},
		},
		{
			name:  "the last of H, L and P wins",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:  []string{"-RHLP", "d", "e"},
			want:  map[string]string{"e/f.txt": "f"},
		},
		{
			name:  "L is accepted without -R",
			files: fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:  []string{"-L", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "x"},
		},
		{
			name:  "P combines with other options",
			files: fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o640}},
			args:  []string{"-Pf", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "x"},
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
			name:  "force overwrites an existing file",
			files: fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:  []string{"--force", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "new"},
		},
		{
			name:    "interactive prompts and overwrites when affirmative",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:   "y\n",
			args:    []string{"--interactive", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "new"},
			wantErr: "cp: overwrite 'b.txt'? ",
		},
		{
			name:     "preserve preserves the times",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), ModTime: oldTime}},
			args:     []string{"--preserve", "a.txt", "b.txt"},
			wantTime: map[string]time.Time{"b.txt": oldTime},
		},
		{
			name:  "recursive copies a hierarchy",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:  []string{"--recursive", "d", "e"},
			want:  map[string]string{"e/f.txt": "f"},
		},
		{
			name:  "dereference is accepted",
			files: fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:  []string{"--dereference", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "x"},
		},
		{
			name:  "no-dereference is accepted",
			files: fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:  []string{"--no-dereference", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "x"},
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
			wantErr:  "cp: missing file operand\n",
			wantExit: 2,
		},
		{
			name:     "missing destination operand",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt"},
			wantErr:  "cp: missing destination file operand after 'a.txt'\n",
			wantExit: 2,
		},
		{
			name:     "invalid option",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"-x", "a.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "cp: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"--bogus", "a.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "cp: unrecognized option '--bogus'\n",
			wantExit: 2,
		},
		{
			name:     "an empty source",
			files:    fsx.MemFS{"a.txt": {}},
			args:     []string{"", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "cp: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "an empty target",
			files:    fsx.MemFS{"a.txt": {}},
			args:     []string{"a.txt", ""},
			wantErr:  "cp: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "a missing directory as target",
			files:    fsx.MemFS{"a.txt": {}, "c.txt": {}},
			args:     []string{"a.txt", "c.txt", "missing"},
			wantErr:  "cp: target 'missing' is not a directory\n",
			wantExit: 1,
		},
		{
			name:     "a file as target with multiple sources",
			files:    fsx.MemFS{"a.txt": {}, "c.txt": {}, "b.txt": {Data: []byte("old")}},
			args:     []string{"a.txt", "c.txt", "b.txt"},
			want:     map[string]string{"b.txt": "old"},
			wantErr:  "cp: target 'b.txt' is not a directory\n",
			wantExit: 1,
		},
		{
			name:     "a trailing slash requires a directory",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt", "missing/"},
			absent:   []string{"missing"},
			wantErr:  "cp: target 'missing/' is not a directory\n",
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

func TestRunArchive(t *testing.T) {
	tests := []testCase{
		{
			name:     "copies a tree preserving mode and times",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o751, ModTime: oldTime}, "d/f.txt": {Data: []byte("f"), Mode: 0o640, ModTime: oldTime}},
			args:     []string{"-a", "d", "e"},
			want:     map[string]string{"e/f.txt": "f"},
			wantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o751, "e/f.txt": 0o640},
			wantTime: map[string]time.Time{"e": oldTime, "e/f.txt": oldTime},
		},
		{
			name:     "the umask doesn't apply",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir | 0o777}, "d/f.txt": {Data: []byte("x"), Mode: 0o666}},
			umask:    0o022,
			args:     []string{"-a", "d", "e"},
			wantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o777, "e/f.txt": 0o666},
		},
		{
			name:     "a symbolic link operand is copied as a link",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}, "link": {Mode: fs.ModeSymlink, Data: []byte("a.txt")}},
			args:     []string{"-a", "link", "b"},
			wantLink: map[string]string{"b": "a.txt"},
		},
		{
			name:     "the links of a hierarchy are kept",
			files:    fsx.MemFS{"d/a.txt": {Data: []byte("x")}, "d/link": {Mode: fs.ModeSymlink, Data: []byte("a.txt")}},
			args:     []string{"-a", "d", "e"},
			wantLink: map[string]string{"e/link": "a.txt"},
		},
		{
			name:  "the long option works",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}},
			args:  []string{"--archive", "d", "e"},
			want:  map[string]string{"e/f.txt": "f"},
		},
		{
			name:  "a later -L still dereferences",
			files: fsx.MemFS{"a.txt": {Data: []byte("x")}, "link": {Mode: fs.ModeSymlink, Data: []byte("a.txt")}},
			args:  []string{"-aL", "link", "b"},
			want:  map[string]string{"b": "x"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunNoClobber(t *testing.T) {
	tests := []testCase{
		{
			name:  "keeps an existing destination",
			files: fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:  []string{"-n", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "old"},
		},
		{
			name:  "copies when the destination is missing",
			files: fsx.MemFS{"a.txt": {Data: []byte("new")}},
			args:  []string{"-n", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "new"},
		},
		{
			name:  "keeps the files of an existing hierarchy",
			files: fsx.MemFS{"d/a.txt": {Data: []byte("new")}, "e/d/a.txt": {Data: []byte("old")}, "e/d/keep.txt": {Data: []byte("keep")}},
			args:  []string{"-Rn", "d", "e"},
			want:  map[string]string{"e/d/a.txt": "old", "e/d/keep.txt": "keep"},
		},
		{
			name:  "the long option works",
			files: fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:  []string{"--no-clobber", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "old"},
		},
		{
			name:     "a later -i overrides -n",
			files:    fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			stdin:    "n\n",
			args:     []string{"-ni", "a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "old"},
			wantErr:  "cp: overwrite 'b.txt'? ",
			wantExit: 0,
		},
		{
			name:  "a later -n overrides -i",
			files: fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:  []string{"-in", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "old"},
		},
		{
			name:  "-f is ignored with -n",
			files: fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:  []string{"-fn", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "old"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunUpdate(t *testing.T) {
	tests := []testCase{
		{
			name:  "copies when the source is newer",
			files: fsx.MemFS{"a.txt": {Data: []byte("new"), ModTime: oldTime}, "b.txt": {Data: []byte("old"), ModTime: otherTime}},
			args:  []string{"-u", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "new"},
		},
		{
			name:  "skips when the source is older",
			files: fsx.MemFS{"a.txt": {Data: []byte("old"), ModTime: otherTime}, "b.txt": {Data: []byte("new"), ModTime: oldTime}},
			args:  []string{"-u", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "new"},
		},
		{
			name:  "skips when the times are equal",
			files: fsx.MemFS{"a.txt": {Data: []byte("new"), ModTime: oldTime}, "b.txt": {Data: []byte("old"), ModTime: oldTime}},
			args:  []string{"-u", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "old"},
		},
		{
			name:  "copies when the destination is missing",
			files: fsx.MemFS{"a.txt": {Data: []byte("new"), ModTime: oldTime}},
			args:  []string{"-u", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "new"},
		},
		{
			name: "skips only the outdated files of a hierarchy",
			files: fsx.MemFS{
				"d/new.txt":   {Data: []byte("new"), ModTime: oldTime},
				"d/old.txt":   {Data: []byte("old"), ModTime: otherTime},
				"e/d/new.txt": {Data: []byte("kept"), ModTime: otherTime},
				"e/d/old.txt": {Data: []byte("kept"), ModTime: oldTime},
			},
			args: []string{"-Ru", "d", "e"},
			want: map[string]string{"e/d/new.txt": "new", "e/d/old.txt": "kept"},
		},
		{
			name:  "a missing destination directory is still created",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f"), ModTime: oldTime}},
			args:  []string{"-Ru", "d", "e"},
			want:  map[string]string{"e/f.txt": "f"},
		},
		{
			name:  "-n takes precedence over -u",
			files: fsx.MemFS{"a.txt": {Data: []byte("new"), ModTime: oldTime}, "b.txt": {Data: []byte("old"), ModTime: otherTime}},
			args:  []string{"-nu", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "old"},
		},
		{
			name:  "the long option works",
			files: fsx.MemFS{"a.txt": {Data: []byte("new"), ModTime: oldTime}, "b.txt": {Data: []byte("old"), ModTime: otherTime}},
			args:  []string{"--update", "a.txt", "b.txt"},
			want:  map[string]string{"b.txt": "new"},
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
			name:    "names each copied file",
			files:   fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:    []string{"-v", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "x"},
			wantOut: "'a.txt' -> 'b.txt'\n",
		},
		{
			name:    "the long option works",
			files:   fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:    []string{"--verbose", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "x"},
			wantOut: "'a.txt' -> 'b.txt'\n",
		},
		{
			name:  "names each entry of a hierarchy",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			args:  []string{"-Rv", "d", "e"},
			want:  map[string]string{"e/f.txt": "f", "e/sub/g.txt": "g"},
			wantOut: "'d' -> 'e'\n" +
				"'d/f.txt' -> 'e/f.txt'\n" +
				"'d/sub' -> 'e/sub'\n" +
				"'d/sub/g.txt' -> 'e/sub/g.txt'\n",
		},
		{
			name:    "names multiple sources copied into a directory",
			files:   fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}, "d/keep.txt": {}},
			args:    []string{"-v", "a.txt", "b.txt", "d"},
			want:    map[string]string{"d/a.txt": "a", "d/b.txt": "b"},
			wantOut: "'a.txt' -> 'd/a.txt'\n'b.txt' -> 'd/b.txt'\n",
		},
		{
			name:    "a skipped file isn't named",
			files:   fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:    []string{"-nv", "a.txt", "b.txt"},
			want:    map[string]string{"b.txt": "old"},
			wantOut: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunTargetDirectory(t *testing.T) {
	tests := []testCase{
		{
			name:  "copies into the target directory",
			files: fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}, "d/keep.txt": {}},
			args:  []string{"-t", "d", "a.txt", "b.txt"},
			want:  map[string]string{"d/a.txt": "a", "d/b.txt": "b", "d/keep.txt": ""},
		},
		{
			name:  "the long option with an attached value",
			files: fsx.MemFS{"a.txt": {Data: []byte("a")}, "d/keep.txt": {}},
			args:  []string{"--target-directory=d", "a.txt"},
			want:  map[string]string{"d/a.txt": "a"},
		},
		{
			name:  "the long option with the next argument",
			files: fsx.MemFS{"a.txt": {Data: []byte("a")}, "d/keep.txt": {}},
			args:  []string{"--target-directory", "d", "a.txt"},
			want:  map[string]string{"d/a.txt": "a"},
		},
		{
			name:     "the target must be an existing directory",
			files:    fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}},
			args:     []string{"-t", "b.txt", "a.txt"},
			want:     map[string]string{"b.txt": "b"},
			wantErr:  "cp: target 'b.txt' is not a directory\n",
			wantExit: 1,
		},
		{
			name:     "a missing target is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("a")}},
			args:     []string{"-t", "d", "a.txt"},
			absent:   []string{"d"},
			wantErr:  "cp: target 'd' is not a directory\n",
			wantExit: 1,
		},
		{
			name:     "no sources is a usage error",
			files:    fsx.MemFS{"d/keep.txt": {}},
			args:     []string{"-t", "d"},
			wantErr:  "cp: missing file operand\n",
			wantExit: 2,
		},
		{
			name:     "-t and -T conflict",
			files:    fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {}},
			args:     []string{"-t", "d", "-T", "a.txt", "b.txt"},
			want:     map[string]string{"a.txt": "a"},
			wantErr:  "cp: cannot combine --target-directory (-t) and --no-target-directory (-T)\n",
			wantExit: 2,
		},
		{
			name:  "-T copies to an exact name",
			files: fsx.MemFS{"a.txt": {Data: []byte("x")}, "d/keep.txt": {}},
			args:  []string{"-T", "a.txt", "d/b.txt"},
			want:  map[string]string{"d/b.txt": "x", "d/keep.txt": ""},
		},
		{
			name:     "-T keeps a directory destination from being used",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}, "d/keep.txt": {Data: []byte("keep")}},
			args:     []string{"-T", "a.txt", "d"},
			want:     map[string]string{"d/keep.txt": "keep"},
			wantErr:  "cp: cannot overwrite directory 'd' with non-directory 'a.txt'\n",
			wantExit: 1,
		},
		{
			name:     "-T rejects extra operands",
			files:    fsx.MemFS{"a.txt": {}, "b.txt": {}, "c.txt": {}},
			args:     []string{"-T", "a.txt", "b.txt", "c.txt"},
			wantErr:  "cp: extra operand 'c.txt'\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

// link is a MemFS entry holding a symbolic link to target.
func link(target string) *fsx.MemFile {
	return &fsx.MemFile{Mode: fs.ModeSymlink, Data: []byte(target)}
}

func TestRunSymlinks(t *testing.T) {
	tests := []testCase{
		{
			name:  "a link operand is followed without -R",
			files: fsx.MemFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt")},
			args:  []string{"l", "b.txt"},
			want:  map[string]string{"b.txt": "x"},
		},
		{
			name:  "-H follows a link operand",
			files: fsx.MemFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt")},
			args:  []string{"-H", "l", "b.txt"},
			want:  map[string]string{"b.txt": "x"},
		},
		{
			name:  "-L follows a link operand",
			files: fsx.MemFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt")},
			args:  []string{"-L", "l", "b.txt"},
			want:  map[string]string{"b.txt": "x"},
		},
		{
			name:     "-d copies a link operand as a link",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt")},
			args:     []string{"-d", "l", "b"},
			wantLink: map[string]string{"b": "a.txt"},
		},
		{
			name:     "-P copies a link operand as a link",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt")},
			args:     []string{"-P", "l", "b"},
			wantLink: map[string]string{"b": "a.txt"},
		},
		{
			name:     "-d replaces an existing destination",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt"), "b": {Data: []byte("old")}},
			args:     []string{"-d", "l", "b"},
			wantLink: map[string]string{"b": "a.txt"},
		},
		{
			name:     "-d copies a dangling link",
			files:    fsx.MemFS{"l": link("missing")},
			args:     []string{"-d", "l", "b"},
			wantLink: map[string]string{"b": "missing"},
		},
		{
			name:  "-d with -n keeps an existing destination",
			files: fsx.MemFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt"), "b": {Data: []byte("old")}},
			args:  []string{"-dn", "l", "b"},
			want:  map[string]string{"b": "old"},
		},
		{
			name:     "-d with -i prompts",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt"), "b": {Data: []byte("old")}},
			stdin:    "y\n",
			args:     []string{"-di", "l", "b"},
			wantLink: map[string]string{"b": "a.txt"},
			wantErr:  "cp: overwrite 'b'? ",
		},
		{
			name:     "-R keeps the links of a hierarchy",
			files:    fsx.MemFS{"d/a.txt": {Data: []byte("x")}, "d/l": link("a.txt")},
			args:     []string{"-R", "d", "e"},
			wantLink: map[string]string{"e/l": "a.txt"},
		},
		{
			name:     "-r keeps the links of a hierarchy",
			files:    fsx.MemFS{"d/a.txt": {Data: []byte("x")}, "d/l": link("a.txt")},
			args:     []string{"-r", "d", "e"},
			wantLink: map[string]string{"e/l": "a.txt"},
		},
		{
			name:     "-R copies a link operand as a link",
			files:    fsx.MemFS{"d/a.txt": {Data: []byte("x")}, "d/l": link("a.txt"), "e/keep.txt": {}},
			args:     []string{"-R", "d", "e"},
			wantLink: map[string]string{"e/d/l": "a.txt"},
		},
		{
			name:  "-L follows the links of a hierarchy",
			files: fsx.MemFS{"d/a.txt": {Data: []byte("x")}, "d/l": link("a.txt")},
			args:  []string{"-RL", "d", "e"},
			want:  map[string]string{"e/l": "x"},
		},
		{
			name:     "-H dereferences only the command line operands",
			files:    fsx.MemFS{"d/a.txt": {Data: []byte("x")}, "d/l": link("a.txt")},
			args:     []string{"-RH", "d", "e"},
			want:     map[string]string{"e/a.txt": "x"},
			wantLink: map[string]string{"e/l": "a.txt"},
		},
		{
			name:  "-H follows a directory link operand",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "l": link("d")},
			args:  []string{"-RH", "l", "e"},
			want:  map[string]string{"e/f.txt": "f"},
		},
		{
			name:  "-L follows a directory link operand",
			files: fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "l": link("d")},
			args:  []string{"-RL", "l", "e"},
			want:  map[string]string{"e/f.txt": "f"},
		},
		{
			name:     "-d copies a directory link operand as a link",
			files:    fsx.MemFS{"d/f.txt": {Data: []byte("f")}, "l": link("d"), "e/keep.txt": {}},
			args:     []string{"-dR", "l", "e"},
			wantLink: map[string]string{"e/l": "d"},
		},
		{
			name:     "a link to a directory in a tree stays a link",
			files:    fsx.MemFS{"d/sub/f.txt": {Data: []byte("f")}, "d/l": link("sub")},
			args:     []string{"-R", "d", "e"},
			wantLink: map[string]string{"e/l": "sub"},
		},
		{
			name:  "-L follows a link to a directory in a tree",
			files: fsx.MemFS{"d/sub/f.txt": {Data: []byte("f")}, "d/l": link("sub")},
			args:  []string{"-RL", "d", "e"},
			want:  map[string]string{"e/l/f.txt": "f"},
		},
		{
			name:  "a symbolic link destination is followed",
			files: fsx.MemFS{"a.txt": {Data: []byte("x")}, "b.txt": {Data: []byte("old")}, "b": link("b.txt")},
			args:  []string{"a.txt", "b"},
			want:  map[string]string{"b.txt": "x"},
		},
		{
			name:     "a copied link is named with -v",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt")},
			args:     []string{"-dv", "l", "b"},
			wantLink: map[string]string{"b": "a.txt"},
			wantOut:  "'l' -> 'b'\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSymlinksCycles(t *testing.T) {
	t.Run("-L on a link to an ancestor is refused", func(t *testing.T) {
		files := fsx.MemFS{
			"d":       {Mode: fs.ModeDir | 0o755},
			"d/f.txt": {Data: []byte("x")},
			"d/up":    link(".."),
		}
		rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: io.Discard, FS: files}
		err := New(rt).Run(t.Context(), "-RL", "d", "e")
		if err == nil {
			t.Fatal("Run() error = nil, want a failure")
		}
		if _, statErr := fs.Stat(files, "e/f.txt"); statErr != nil {
			t.Errorf("Stat(e/f.txt) error = %v, want the copy to complete", statErr)
		}
	})

	t.Run("a link to a sibling directory is copied twice, without a cycle", func(t *testing.T) {
		files := fsx.MemFS{
			"d":       {Mode: fs.ModeDir | 0o755},
			"d/f.txt": {Data: []byte("x")},
			"d/sub":   {Mode: fs.ModeDir | 0o755},
		}
		rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: io.Discard, FS: files}
		if err := New(rt).Run(t.Context(), "-RL", "d", "e"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if _, statErr := fs.Stat(files, "e/sub"); statErr != nil {
			t.Errorf("Stat(e/sub) error = %v", statErr)
		}
	})
}

func TestRunDanglingDestination(t *testing.T) {
	files := fsx.MemFS{
		"a.txt": {Data: []byte("x")},
		"dlink": link("missing"),
	}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: stderr, FS: files}
	err := New(rt).Run(t.Context(), "a.txt", "dlink")
	if code := exitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if want := "cp: not writing through dangling symlink 'dlink'\n"; stderr.String() != want {
		t.Errorf("Run() stderr = %q, want %q", stderr.String(), want)
	}
	if _, statErr := fs.Stat(files, "missing"); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("Stat(missing) error = %v, want it to stay missing", statErr)
	}
}

func TestRunSymlinksOsFS(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(dir, "l")); err != nil {
		t.Skipf("symbolic links aren't supported here: %v", err)
	}

	tests := []struct {
		name    string
		args    []string
		dst     string
		want    string
		wantOut bool
	}{
		{
			name: "a link operand is followed without -R",
			args: []string{"l", "b"},
			dst:  "b",
			want: "x",
		},
		{
			name:    "-d copies a link operand as a link",
			args:    []string{"-d", "l", "b"},
			dst:     "b",
			wantOut: true,
		},
		{
			name:    "-R keeps a link operand as a link",
			args:    []string{"-R", "l", "b"},
			dst:     "b",
			wantOut: true,
		},
		{
			name: "-L follows a link operand",
			args: []string{"-L", "l", "c"},
			dst:  "c",
			want: "x",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			rt := &coreutils.Runtime{
				Stdout:     stdout,
				Stderr:     io.Discard,
				FS:         fsx.OsFS(dir),
				WorkingDir: dir,
			}
			if err := New(rt).Run(t.Context(), test.args...); err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			dst := filepath.Join(dir, test.dst)
			target, err := os.Readlink(dst)
			if err != nil {
				if !test.wantOut {
					got, err := os.ReadFile(dst)
					if err != nil || string(got) != test.want {
						t.Errorf("ReadFile(%q) = %q, %v, want %q", dst, got, err, test.want)
					}
					return
				}
				t.Fatalf("Readlink(%q) error = %v", dst, err)
			}
			if !test.wantOut {
				t.Fatalf("Readlink(%q) = %q, want a regular file", dst, target)
			}
			if target != "a.txt" {
				t.Errorf("Readlink(%q) = %q, want %q", dst, target, "a.txt")
			}
		})
	}
}

func TestRunPreserveList(t *testing.T) {
	tests := []testCase{
		{
			name:     "--preserve keeps the mode and times",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o640, ModTime: oldTime}},
			args:     []string{"--preserve", "a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o640},
			wantTime: map[string]time.Time{"b.txt": oldTime},
		},
		{
			name:     "--preserve=mode keeps only the mode",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o666, ModTime: oldTime}},
			umask:    0o022,
			args:     []string{"--preserve=mode", "a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o666},
		},
		{
			name:     "--preserve=timestamps keeps only the times",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o666, ModTime: oldTime}},
			umask:    0o022,
			args:     []string{"--preserve=timestamps", "a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o644},
			wantTime: map[string]time.Time{"b.txt": oldTime},
		},
		{
			name:     "attributes can be combined",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o640, ModTime: oldTime}},
			args:     []string{"--preserve=mode,timestamps", "a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o640},
			wantTime: map[string]time.Time{"b.txt": oldTime},
		},
		{
			name:     "--no-preserve drops the mode and times",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o666, ModTime: oldTime}},
			umask:    0o022,
			args:     []string{"-p", "--no-preserve=mode,timestamps", "a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o644},
		},
		{
			name:     "an unknown attribute is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"--preserve=mode,bogus", "a.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "cp: invalid attribute list: 'mode,bogus'\n",
			wantExit: 2,
		},
		{
			name:     "an unknown attribute of --no-preserve is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"--no-preserve=bogus", "a.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "cp: invalid attribute list: 'bogus'\n",
			wantExit: 2,
		},
		{
			name:     "--no-preserve takes the next argument as its list",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o666, ModTime: oldTime}},
			umask:    0o022,
			args:     []string{"-p", "--no-preserve", "a.txt", "b.txt"},
			wantErr:  "cp: invalid attribute list: 'a.txt'\n",
			wantExit: 2,
		},
		{
			name:     "--no-preserve= with an empty list is ambiguous",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o666, ModTime: oldTime}},
			umask:    0o022,
			args:     []string{"-p", "--no-preserve=", "a.txt", "b.txt"},
			wantErr:  "cp: ambiguous argument '' for '--no-preserve'\n",
			wantExit: 2,
		},
		{
			name:     "--no-preserve=mode clears only the mode",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x"), Mode: 0o666, ModTime: oldTime}},
			umask:    0o022,
			args:     []string{"-p", "--no-preserve=mode", "a.txt", "b.txt"},
			wantMode: map[string]fs.FileMode{"b.txt": 0o644},
			wantTime: map[string]time.Time{"b.txt": oldTime},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunPreserveOwnership(t *testing.T) {
	files := fsx.MemFS{"a.txt": {Data: []byte("x"), Uid: 1000, Gid: 2000}}
	runCase(t, testCase{
		files: files,
		args:  []string{"--preserve=all", "a.txt", "b.txt"},
	})

	fi, err := fs.Stat(files, "b.txt")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	uid, gid, ok := fsx.Owner(fi)
	if !ok || uid != 1000 || gid != 2000 {
		t.Errorf("owner of b.txt = %d, %d, %v, want 1000, 2000, true", uid, gid, ok)
	}
}
