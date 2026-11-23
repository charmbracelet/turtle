package cp

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

var (
	oldTime   = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	otherTime = time.Date(1999, 12, 31, 23, 59, 58, 0, time.UTC)
)

// testCase is the fields of a testutil.Case plus the parts the file copying
// tests need: the fixtures and their assertions the runner doesn't have (the
// access times and symbolic links), an optional file system to run on instead
// of the fixtures, and the standard input as the plain text to answer the
// interactive prompts with.
type testCase struct {
	Name     string
	Args     []string
	Want     string
	WantErr  string
	WantExit int
	Dir      string
	Umask    fs.FileMode

	FSys      fsx.FS
	Files     fsx.MemoryFS
	Stdin     string
	WantFiles map[string]string
	WantMode  map[string]fs.FileMode
	WantTime  map[string]time.Time
	WantLink  map[string]string
	Present   []string
	Absent    []string
}

// runCase runs a single case through testutil, on the file system the case
// gives, and checks what the runner doesn't: the access times of the files
// and their symbolic links.
func runCase(t *testing.T, test testCase) {
	t.Helper()

	folder := test.FSys
	if folder == nil {
		folder = test.Files
	}

	c := testutil.Case{
		Name:      test.Name,
		Stdin:     strings.NewReader(test.Stdin),
		FS:        folder,
		Dir:       test.Dir,
		Umask:     test.Umask,
		Args:      test.Args,
		Want:      test.Want,
		WantErr:   test.WantErr,
		WantExit:  test.WantExit,
		WantFiles: test.WantFiles,
		WantMode:  test.WantMode,
		WantTime:  test.WantTime,
		Present:   test.Present,
		Absent:    test.Absent,
	}

	files, WantTime, WantLink := test.Files, test.WantTime, test.WantLink
	c.Check = func(t *testing.T, rt *coreutils.Runtime) {
		for name, want := range WantTime {
			fi, err := files.Lstat(name)
			if err != nil {
				t.Errorf("Stat(%q) error = %v", name, err)
				continue
			}
			if got, ok := fsx.Atime(fi); ok && !got.Equal(want) {
				t.Errorf("%s access time = %v, want %v", name, got, want)
			}
		}
		for name, want := range WantLink {
			got, err := files.ReadLink(name)
			if err != nil {
				t.Errorf("ReadLink(%q) error = %v", name, err)
				continue
			}
			if got != want {
				t.Errorf("ReadLink(%q) = %q, want %q", name, got, want)
			}
			fi, err := files.Lstat(name)
			if err != nil {
				t.Errorf("Lstat(%q) error = %v", name, err)
				continue
			}
			if fi.Mode()&fs.ModeSymlink == 0 {
				t.Errorf("Lstat(%q) mode = %v, want a symbolic link", name, fi.Mode())
			}
		}
	}

	testutil.Once(t, New, c)
}

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			Name:      "copies a file to a new name",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("hello")}},
			Args:      []string{"a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "hello"},
			Present:   []string{"a.txt"},
		},
		{
			Name:      "overwrites an existing file",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Args:      []string{"a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
		},
		{
			Name:      "copies into an existing directory",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("hello")}, "d/keep.txt": {Data: []byte("keep")}},
			Args:      []string{"a.txt", "d"},
			WantFiles: map[string]string{"d/a.txt": "hello"},
			Present:   []string{"d/keep.txt"},
		},
		{
			Name:      "copies multiple files into a directory",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}, "d/keep.txt": {}},
			Args:      []string{"a.txt", "b.txt", "d"},
			WantFiles: map[string]string{"d/a.txt": "a", "d/b.txt": "b"},
		},
		{
			Name:      "a trailing slash on the target is ignored",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("hello")}, "d/keep.txt": {}},
			Args:      []string{"a.txt", "d/"},
			WantFiles: map[string]string{"d/a.txt": "hello"},
		},
		{
			Name:      "a path with a directory component is copied to its base name",
			Files:     fsx.MemoryFS{"sub/a.txt": {Data: []byte("hello")}, "d/keep.txt": {}},
			Args:      []string{"sub/a.txt", "d"},
			WantFiles: map[string]string{"d/a.txt": "hello"},
		},
		{
			Name:      "keeps copying after a failing source",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("hello")}, "d/keep.txt": {}},
			Args:      []string{"missing.txt", "a.txt", "d"},
			WantFiles: map[string]string{"d/a.txt": "hello"},
			WantErr:   "cp: cannot stat 'missing.txt': file does not exist\n",
			WantExit:  1,
		},
		{
			Name:     "a missing source is an error",
			Files:    fsx.MemoryFS{},
			Args:     []string{"missing.txt", "b.txt"},
			Absent:   []string{"b.txt"},
			WantErr:  "cp: cannot stat 'missing.txt': file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing destination parent is an error",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:     []string{"a.txt", "nodir/b.txt"},
			Absent:   []string{"nodir"},
			WantErr:  "cp: cannot create 'nodir/b.txt': file does not exist\n",
			WantExit: 1,
		},
		{
			Name:      "the umask applies to new files",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o666}},
			Umask:     0o022,
			Args:      []string{"a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "x"},
			WantMode:  map[string]fs.FileMode{"b.txt": 0o644},
		},
		{
			Name:     "the source mode is used without a umask",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o600}},
			Args:     []string{"a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": 0o600},
		},
		{
			Name:     "the special bits are cleared when creating",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: fs.ModeSetuid | 0o755}},
			Args:     []string{"a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": 0o755},
		},
		{
			Name:      "overwriting keeps the destination mode",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new"), Mode: 0o777}, "b.txt": {Data: []byte("old"), Mode: 0o600}},
			Args:      []string{"a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
			WantMode:  map[string]fs.FileMode{"b.txt": 0o600},
		},
		{
			Name:      "copies an empty file",
			Files:     fsx.MemoryFS{"a.txt": {}},
			Args:      []string{"a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": ""},
		},
		{
			Name:      "copies contents with any bytes",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte{0, 1, 2, 0xff, '\n'}}},
			Args:      []string{"a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "\x00\x01\x02\xff\n"},
		},
		{
			Name:      "a dash names a file",
			Files:     fsx.MemoryFS{"-": {Data: []byte("dash")}},
			Args:      []string{"-", "b.txt"},
			WantFiles: map[string]string{"b.txt": "dash"},
		},
		{
			Name:      "options end at a double dash",
			Files:     fsx.MemoryFS{"-p": {Data: []byte("dash p")}},
			Args:      []string{"--", "-p", "b.txt"},
			WantFiles: map[string]string{"b.txt": "dash p"},
		},
		{
			Name:      "glued short flags are accepted",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o640}},
			Args:      []string{"-fip", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "x"},
			WantMode:  map[string]fs.FileMode{"b.txt": 0o640},
		},
		{
			Name:     "a file onto itself is an error",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:     []string{"a.txt", "a.txt"},
			WantErr:  "cp: 'a.txt' and 'a.txt' are the same file\n",
			WantExit: 1,
		},
		{
			Name:     "an equivalent path is still the same file",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:     []string{"a.txt", "./a.txt"},
			WantErr:  "cp: 'a.txt' and './a.txt' are the same file\n",
			WantExit: 1,
		},
		{
			Name:     "a directory source without -R is an error",
			Files:    fsx.MemoryFS{"d/f.txt": {}},
			Args:     []string{"d", "e"},
			Absent:   []string{"e"},
			WantErr:  "cp: -R not specified; omitting directory 'd'\n",
			WantExit: 1,
		},
		{
			Name:     "a directory source into an existing directory without -R is an error",
			Files:    fsx.MemoryFS{"d/f.txt": {}, "e/keep.txt": {}},
			Args:     []string{"d", "e"},
			Absent:   []string{"e/d"},
			WantErr:  "cp: -R not specified; omitting directory 'd'\n",
			WantExit: 1,
		},
		{
			Name:      "a trailing dot on the target is ignored",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "d/keep.txt": {}},
			Args:      []string{"a.txt", "d/."},
			WantFiles: map[string]string{"d/a.txt": "x"},
		},
		{
			Name:      "relative paths resolve inside the working directory",
			Dir:       "base",
			Files:     fsx.MemoryFS{"base/a.txt": {Data: []byte("rel")}},
			Args:      []string{"a.txt", "b.txt"},
			WantFiles: map[string]string{"base/b.txt": "rel"},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRecursive(t *testing.T) {
	tests := []testCase{
		{
			Name:      "copies a tree to a new name",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			Args:      []string{"-R", "d", "e"},
			WantFiles: map[string]string{"e/f.txt": "f", "e/sub/g.txt": "g"},
		},
		{
			Name:      "copies a tree into an existing directory",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}, "e/keep.txt": {Data: []byte("keep")}},
			Args:      []string{"-R", "d", "e"},
			WantFiles: map[string]string{"e/d/f.txt": "f"},
			Present:   []string{"e/keep.txt"},
		},
		{
			Name:      "copies multiple trees",
			Files:     fsx.MemoryFS{"d1/f": {Data: []byte("1")}, "d2/g": {Data: []byte("2")}, "e/keep.txt": {}},
			Args:      []string{"-R", "d1", "d2", "e"},
			WantFiles: map[string]string{"e/d1/f": "1", "e/d2/g": "2"},
		},
		{
			Name:      "copies trees and files together",
			Files:     fsx.MemoryFS{"d/f": {}, "g.txt": {Data: []byte("g")}, "e/keep.txt": {}},
			Args:      []string{"-R", "d", "g.txt", "e"},
			WantFiles: map[string]string{"e/d/f": "", "e/g.txt": "g"},
		},
		{
			Name:      "merges into an existing tree",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}, "e/d/old.txt": {Data: []byte("old")}},
			Args:      []string{"-R", "d", "e"},
			WantFiles: map[string]string{"e/d/f.txt": "f", "e/d/old.txt": "old"},
		},
		{
			Name:     "created directories get the source mode",
			Files:    fsx.MemoryFS{"d": {Mode: fs.ModeDir | 0o750}, "d/f.txt": {}},
			Args:     []string{"-R", "d", "e"},
			WantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o750},
		},
		{
			Name:     "the umask applies to created directories",
			Files:    fsx.MemoryFS{"d": {Mode: fs.ModeDir | 0o777}, "d/f.txt": {}},
			Umask:    0o027,
			Args:     []string{"-R", "d", "e"},
			WantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o750},
		},
		{
			Name:      "a restrictive umask still allows the contents to be copied",
			Files:     fsx.MemoryFS{"d": {Mode: fs.ModeDir | 0o755}, "d/f.txt": {Data: []byte("x")}},
			Umask:     0o177,
			Args:      []string{"-R", "d", "e"},
			WantFiles: map[string]string{"e/f.txt": "x"},
			WantMode:  map[string]fs.FileMode{"e": fs.ModeDir | 0o600},
		},
		{
			Name:     "files in the tree follow the same umask rules",
			Files:    fsx.MemoryFS{"d/f.txt": {Data: []byte("x"), Mode: 0o666}, "d/sub/g.txt": {Data: []byte("g"), Mode: 0o640}},
			Umask:    0o022,
			Args:     []string{"-R", "d", "e"},
			WantMode: map[string]fs.FileMode{"e/f.txt": 0o644, "e/sub/g.txt": 0o640},
		},
		{
			Name:      "an existing destination directory is left alone",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("new")}, "e/d": {Mode: fs.ModeDir | 0o700}, "e/d/old.txt": {Data: []byte("old")}},
			Args:      []string{"-R", "d", "e"},
			WantFiles: map[string]string{"e/d/f.txt": "new", "e/d/old.txt": "old"},
			WantMode:  map[string]fs.FileMode{"e/d": fs.ModeDir | 0o700},
		},
		{
			Name:     "copies an empty directory",
			Files:    fsx.MemoryFS{"d": {Mode: fs.ModeDir | 0o755}},
			Args:     []string{"-R", "d", "e"},
			WantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o755},
		},
		{
			Name:      "a trailing dot copies the contents",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			Args:      []string{"-R", "d/.", "e"},
			WantFiles: map[string]string{"e/f.txt": "f", "e/sub/g.txt": "g"},
		},
		{
			Name:      "a trailing dot copies into an existing directory",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}, "e/keep.txt": {}},
			Args:      []string{"-R", "d/.", "e"},
			WantFiles: map[string]string{"e/f.txt": "f"},
			Present:   []string{"e/keep.txt"},
		},
		{
			Name:      "a trailing slash on the source is ignored",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}, "e/keep.txt": {}},
			Args:      []string{"-R", "d/", "e"},
			WantFiles: map[string]string{"e/d/f.txt": "f"},
		},
		{
			Name:     "the working directory into its own subdirectory is an error",
			Files:    fsx.MemoryFS{"f.txt": {Data: []byte("f")}},
			Args:     []string{"-R", ".", "e"},
			Absent:   []string{"e"},
			WantErr:  "cp: cannot copy a directory, '.', into itself, 'e'\n",
			WantExit: 1,
		},
		{
			Name:      "a trailing slash on the target creates the directory",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}},
			Args:      []string{"-R", "d", "e/"},
			WantFiles: map[string]string{"e/f.txt": "f"},
		},
		{
			Name:      "a file source is copied like without -R",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Args:      []string{"-R", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
		},
		{
			Name:      "the r option copies a tree like -R",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			Args:      []string{"-r", "d", "e"},
			WantFiles: map[string]string{"e/f.txt": "f", "e/sub/g.txt": "g"},
		},
		{
			Name:     "a directory into itself is an error",
			Files:    fsx.MemoryFS{"d/f.txt": {}},
			Args:     []string{"-R", "d", "d"},
			Present:  []string{"d/f.txt"},
			WantErr:  "cp: cannot copy a directory, 'd', into itself, 'd/d'\n",
			WantExit: 1,
		},
		{
			Name:     "a directory into its own subdirectory is an error",
			Files:    fsx.MemoryFS{"d/sub/x": {}},
			Args:     []string{"-R", "d", "d/sub"},
			Present:  []string{"d/sub/x"},
			WantErr:  "cp: cannot copy a directory, 'd', into itself, 'd/sub/d'\n",
			WantExit: 1,
		},
		{
			Name:      "a directory over a file is an error",
			Files:     fsx.MemoryFS{"d/f.txt": {}, "f.txt": {Data: []byte("x")}},
			Args:      []string{"-R", "d", "f.txt"},
			WantFiles: map[string]string{"f.txt": "x"},
			WantErr:   "cp: cannot overwrite non-directory 'f.txt' with directory 'd'\n",
			WantExit:  1,
		},
		{
			Name:      "a file over a directory in the tree is an error",
			Files:     fsx.MemoryFS{"d/x": {Data: []byte("x")}, "d/y": {Data: []byte("y")}, "e/d/x/keep.txt": {Data: []byte("keep")}},
			Args:      []string{"-R", "d", "e"},
			WantFiles: map[string]string{"e/d/y": "y"},
			Present:   []string{"e/d/x/keep.txt"},
			WantErr:   "cp: cannot overwrite directory 'e/d/x' with non-directory 'd/x'\n",
			WantExit:  1,
		},
		{
			Name:     "a missing source is an error",
			Files:    fsx.MemoryFS{"e/keep.txt": {}},
			Args:     []string{"-R", "missing", "e"},
			WantErr:  "cp: cannot stat 'missing': file does not exist\n",
			WantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunPreserve(t *testing.T) {
	tests := []testCase{
		{
			Name:     "preserves the mode",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o604}},
			Args:     []string{"-p", "a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": 0o604},
		},
		{
			Name:     "the umask doesn't apply with -p",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o640}},
			Umask:    0o022,
			Args:     []string{"-p", "a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": 0o640},
		},
		{
			Name:     "preserves the special bits",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: fs.ModeSetuid | fs.ModeSetgid | 0o755}},
			Args:     []string{"-p", "a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": fs.ModeSetuid | fs.ModeSetgid | 0o755},
		},
		{
			Name:     "preserves the times",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), ModTime: oldTime}},
			Args:     []string{"-p", "a.txt", "b.txt"},
			WantTime: map[string]time.Time{"b.txt": oldTime},
		},
		{
			Name:      "preserves mode and times when overwriting",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new"), Mode: 0o642, ModTime: oldTime}, "b.txt": {Data: []byte("old"), Mode: 0o600, ModTime: otherTime}},
			Args:      []string{"-p", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
			WantMode:  map[string]fs.FileMode{"b.txt": 0o642},
			WantTime:  map[string]time.Time{"b.txt": oldTime},
		},
		{
			Name:      "preserves the mode and times of created directories",
			Files:     fsx.MemoryFS{"d": {Mode: fs.ModeDir | 0o751, ModTime: oldTime}, "d/f.txt": {Data: []byte("f"), Mode: 0o644, ModTime: oldTime}},
			Args:      []string{"-Rp", "d", "e"},
			WantFiles: map[string]string{"e/f.txt": "f"},
			WantMode:  map[string]fs.FileMode{"e": fs.ModeDir | 0o751, "e/f.txt": 0o644},
			WantTime:  map[string]time.Time{"e": oldTime, "e/f.txt": oldTime},
		},
		{
			Name:      "leaves an existing destination directory untouched",
			Files:     fsx.MemoryFS{"d": {Mode: fs.ModeDir | 0o750, ModTime: oldTime}, "d/f.txt": {Data: []byte("f")}, "e/d": {Mode: fs.ModeDir | 0o700, ModTime: otherTime}},
			Args:      []string{"-Rp", "d", "e"},
			WantFiles: map[string]string{"e/d/f.txt": "f"},
			WantMode:  map[string]fs.FileMode{"e/d": fs.ModeDir | 0o700},
			WantTime:  map[string]time.Time{"e/d": otherTime},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSystemUmask(t *testing.T) {
	// The real file system applies the process umask to new entries, which
	// cp must respect instead of undoing it.
	tests := []testCase{
		{
			Name:     "created files keep the masked mode",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o666}},
			Umask:    0o022,
			Args:     []string{"a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": 0o644},
		},
		{
			Name:     "created directories keep the masked mode",
			Files:    fsx.MemoryFS{"d": {Mode: fs.ModeDir | 0o777}, "d/f.txt": {}},
			Umask:    0o027,
			Args:     []string{"-R", "d", "e"},
			WantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o750},
		},
		{
			Name:     "a file mode is duplicated exactly with -p",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o666}},
			Umask:    0o022,
			Args:     []string{"-p", "a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": 0o666},
		},
		{
			Name:     "a directory mode is duplicated exactly with -p",
			Files:    fsx.MemoryFS{"d": {Mode: fs.ModeDir | 0o666}, "d/f.txt": {}},
			Umask:    0o022,
			Args:     []string{"-Rp", "d", "e"},
			WantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o666},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, testCase{
				FSys:     &umaskFS{MemoryFS: test.Files, umask: test.Umask},
				Args:     test.Args,
				WantMode: test.WantMode,
			})
		})
	}
}

// umaskFS wraps a MemoryFS, applying a file mode creation mask to new entries,
// as the real file system does.
type umaskFS struct {
	fsx.MemoryFS
	umask fs.FileMode
}

// OpenFile implements fsx.WriteFS.
func (m *umaskFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	if flag&os.O_CREATE != 0 {
		perm &^= m.umask
	}
	return m.MemoryFS.OpenFile(name, flag, perm)
}

// Mkdir implements fsx.MkdirFS.
func (m *umaskFS) Mkdir(name string, perm fs.FileMode) error {
	return m.MemoryFS.Mkdir(name, perm&^m.umask)
}

func TestRunInteractive(t *testing.T) {
	tests := []testCase{
		{
			Name:      "prompts and overwrites when affirmative",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Stdin:     "y\n",
			Args:      []string{"-i", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
			WantErr:   "cp: overwrite 'b.txt'? ",
		},
		{
			Name:      "a capital answer is affirmative too",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Stdin:     "Y\n",
			Args:      []string{"-i", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
			WantErr:   "cp: overwrite 'b.txt'? ",
		},
		{
			Name:      "declines on a negative answer",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Stdin:     "n\n",
			Args:      []string{"-i", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
			WantErr:   "cp: overwrite 'b.txt'? ",
		},
		{
			Name:      "declines on any other answer",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Stdin:     "maybe\n",
			Args:      []string{"-i", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
			WantErr:   "cp: overwrite 'b.txt'? ",
		},
		{
			Name:      "declines at the end of the input",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Args:      []string{"-i", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
			WantErr:   "cp: overwrite 'b.txt'? ",
		},
		{
			Name:      "doesn't prompt when the destination doesn't exist",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}},
			Args:      []string{"-i", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
		},
		{
			Name:      "prompts for every existing destination",
			Files:     fsx.MemoryFS{"a1": {Data: []byte("1")}, "a2": {Data: []byte("2")}, "d/a1": {Data: []byte("old1")}, "d/a2": {Data: []byte("old2")}},
			Stdin:     "y\nn\n",
			Args:      []string{"-i", "a1", "a2", "d"},
			WantFiles: map[string]string{"d/a1": "1", "d/a2": "old2"},
			WantErr:   "cp: overwrite 'd/a1'? cp: overwrite 'd/a2'? ",
		},
		{
			Name:      "prompts while copying a hierarchy",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("new")}, "e/d/f.txt": {Data: []byte("old")}, "e/d/keep.txt": {Data: []byte("keep")}},
			Stdin:     "y\n",
			Args:      []string{"-Ri", "d", "e"},
			WantFiles: map[string]string{"e/d/f.txt": "new"},
			Present:   []string{"e/d/keep.txt"},
			WantErr:   "cp: overwrite 'e/d/f.txt'? ",
		},
		{
			Name:      "doesn't prompt for directories",
			Files:     fsx.MemoryFS{"d/sub/f.txt": {Data: []byte("new")}, "e/d/sub/f.txt": {Data: []byte("old")}},
			Stdin:     "n\n",
			Args:      []string{"-Ri", "d", "e"},
			WantFiles: map[string]string{"e/d/sub/f.txt": "old"},
			WantErr:   "cp: overwrite 'e/d/sub/f.txt'? ",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunInteractiveWithoutStdin(t *testing.T) {
	files := fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}}
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

// failOpenFS wraps a MemoryFS, failing once the attempt to open a given file for
// writing, as a file without write permission would.
type failOpenFS struct {
	fsx.MemoryFS
	path   string
	failed bool
}

// OpenFile implements fsx.WriteFS.
func (m *failOpenFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	if name == m.path && !m.failed && flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		m.failed = true
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return m.MemoryFS.OpenFile(name, flag, perm)
}

func TestRunForce(t *testing.T) {
	t.Run("copies normally when the destination can be opened", func(t *testing.T) {
		runCase(t, testCase{
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Args:      []string{"-f", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
		})
	})

	t.Run("a destination that can't be opened is an error", func(t *testing.T) {
		files := fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}}
		runCase(t, testCase{
			Files:     files,
			FSys:      &failOpenFS{MemoryFS: files, path: "b.txt"},
			Args:      []string{"a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
			WantErr:   "cp: cannot create 'b.txt': permission denied\n",
			WantExit:  1,
		})
	})

	t.Run("f removes the destination and tries again", func(t *testing.T) {
		files := fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}}
		runCase(t, testCase{
			Files:     files,
			FSys:      &failOpenFS{MemoryFS: files, path: "b.txt"},
			Args:      []string{"-f", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
		})
	})

	t.Run("a destination that can't be removed is still an error", func(t *testing.T) {
		runCase(t, testCase{
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("new")}},
			Args:     []string{"-f", "a.txt", "nodir/b.txt"},
			Absent:   []string{"nodir"},
			WantErr:  "cp: cannot create 'nodir/b.txt': file does not exist\n",
			WantExit: 1,
		})
	})
}

func TestRunSymlinkOptions(t *testing.T) {
	tests := []testCase{
		{
			Name:      "H is accepted with -R",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}},
			Args:      []string{"-RH", "d", "e"},
			WantFiles: map[string]string{"e/f.txt": "f"},
		},
		{
			Name:      "the last of H, L and P wins",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}},
			Args:      []string{"-RHLP", "d", "e"},
			WantFiles: map[string]string{"e/f.txt": "f"},
		},
		{
			Name:      "L is accepted without -R",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:      []string{"-L", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "x"},
		},
		{
			Name:      "P combines with other options",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o640}},
			Args:      []string{"-Pf", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "x"},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLongOptions(t *testing.T) {
	tests := []testCase{
		{
			Name:      "force overwrites an existing file",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Args:      []string{"--force", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
		},
		{
			Name:      "interactive prompts and overwrites when affirmative",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Stdin:     "y\n",
			Args:      []string{"--interactive", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
			WantErr:   "cp: overwrite 'b.txt'? ",
		},
		{
			Name:     "preserve preserves the times",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), ModTime: oldTime}},
			Args:     []string{"--preserve", "a.txt", "b.txt"},
			WantTime: map[string]time.Time{"b.txt": oldTime},
		},
		{
			Name:      "recursive copies a hierarchy",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}},
			Args:      []string{"--recursive", "d", "e"},
			WantFiles: map[string]string{"e/f.txt": "f"},
		},
		{
			Name:      "dereference is accepted",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:      []string{"--dereference", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "x"},
		},
		{
			Name:      "no-dereference is accepted",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:      []string{"--no-dereference", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "x"},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	tests := []testCase{
		{
			Name:     "missing file operand",
			Args:     []string{},
			WantErr:  "cp: missing file operand\nTry 'cp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing destination operand",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:     []string{"a.txt"},
			WantErr:  "cp: missing destination file operand after 'a.txt'\nTry 'cp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:     []string{"-x", "a.txt", "b.txt"},
			Absent:   []string{"b.txt"},
			WantErr:  "cp: invalid option -- 'x'\nTry 'cp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unrecognized long option",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:     []string{"--bogus", "a.txt", "b.txt"},
			Absent:   []string{"b.txt"},
			WantErr:  "cp: unrecognized option '--bogus'\nTry 'cp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty source",
			Files:    fsx.MemoryFS{"a.txt": {}},
			Args:     []string{"", "b.txt"},
			Absent:   []string{"b.txt"},
			WantErr:  "cp: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "an empty target",
			Files:    fsx.MemoryFS{"a.txt": {}},
			Args:     []string{"a.txt", ""},
			WantErr:  "cp: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "a missing directory as target",
			Files:    fsx.MemoryFS{"a.txt": {}, "c.txt": {}},
			Args:     []string{"a.txt", "c.txt", "missing"},
			WantErr:  "cp: target 'missing' is not a directory\n",
			WantExit: 1,
		},
		{
			Name:      "a file as target with multiple sources",
			Files:     fsx.MemoryFS{"a.txt": {}, "c.txt": {}, "b.txt": {Data: []byte("old")}},
			Args:      []string{"a.txt", "c.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
			WantErr:   "cp: target 'b.txt' is not a directory\n",
			WantExit:  1,
		},
		{
			Name:     "a trailing slash requires a directory",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:     []string{"a.txt", "missing/"},
			Absent:   []string{"missing"},
			WantErr:  "cp: target 'missing/' is not a directory\n",
			WantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunContext(t *testing.T) {
	testutil.Cancelled(t, New, nil, fsx.MemoryFS{}, "a.txt", "b.txt")
}

func TestRunArchive(t *testing.T) {
	tests := []testCase{
		{
			Name:      "copies a tree preserving mode and times",
			Files:     fsx.MemoryFS{"d": {Mode: fs.ModeDir | 0o751, ModTime: oldTime}, "d/f.txt": {Data: []byte("f"), Mode: 0o640, ModTime: oldTime}},
			Args:      []string{"-a", "d", "e"},
			WantFiles: map[string]string{"e/f.txt": "f"},
			WantMode:  map[string]fs.FileMode{"e": fs.ModeDir | 0o751, "e/f.txt": 0o640},
			WantTime:  map[string]time.Time{"e": oldTime, "e/f.txt": oldTime},
		},
		{
			Name:     "the umask doesn't apply",
			Files:    fsx.MemoryFS{"d": {Mode: fs.ModeDir | 0o777}, "d/f.txt": {Data: []byte("x"), Mode: 0o666}},
			Umask:    0o022,
			Args:     []string{"-a", "d", "e"},
			WantMode: map[string]fs.FileMode{"e": fs.ModeDir | 0o777, "e/f.txt": 0o666},
		},
		{
			Name:     "a symbolic link operand is copied as a link",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "link": {Mode: fs.ModeSymlink, Data: []byte("a.txt")}},
			Args:     []string{"-a", "link", "b"},
			WantLink: map[string]string{"b": "a.txt"},
		},
		{
			Name:     "the links of a hierarchy are kept",
			Files:    fsx.MemoryFS{"d/a.txt": {Data: []byte("x")}, "d/link": {Mode: fs.ModeSymlink, Data: []byte("a.txt")}},
			Args:     []string{"-a", "d", "e"},
			WantLink: map[string]string{"e/link": "a.txt"},
		},
		{
			Name:      "the long option works",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}},
			Args:      []string{"--archive", "d", "e"},
			WantFiles: map[string]string{"e/f.txt": "f"},
		},
		{
			Name:      "a later -L still dereferences",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "link": {Mode: fs.ModeSymlink, Data: []byte("a.txt")}},
			Args:      []string{"-aL", "link", "b"},
			WantFiles: map[string]string{"b": "x"},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunNoClobber(t *testing.T) {
	tests := []testCase{
		{
			Name:      "keeps an existing destination",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Args:      []string{"-n", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
		},
		{
			Name:      "copies when the destination is missing",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}},
			Args:      []string{"-n", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
		},
		{
			Name:      "keeps the files of an existing hierarchy",
			Files:     fsx.MemoryFS{"d/a.txt": {Data: []byte("new")}, "e/d/a.txt": {Data: []byte("old")}, "e/d/keep.txt": {Data: []byte("keep")}},
			Args:      []string{"-Rn", "d", "e"},
			WantFiles: map[string]string{"e/d/a.txt": "old", "e/d/keep.txt": "keep"},
		},
		{
			Name:      "the long option works",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Args:      []string{"--no-clobber", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
		},
		{
			Name:      "a later -i overrides -n",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Stdin:     "n\n",
			Args:      []string{"-ni", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
			WantErr:   "cp: overwrite 'b.txt'? ",
			WantExit:  0,
		},
		{
			Name:      "a later -n overrides -i",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Args:      []string{"-in", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
		},
		{
			Name:      "-f is ignored with -n",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Args:      []string{"-fn", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunUpdate(t *testing.T) {
	tests := []testCase{
		{
			Name:      "copies when the source is newer",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new"), ModTime: oldTime}, "b.txt": {Data: []byte("old"), ModTime: otherTime}},
			Args:      []string{"-u", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
		},
		{
			Name:      "skips when the source is older",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("old"), ModTime: otherTime}, "b.txt": {Data: []byte("new"), ModTime: oldTime}},
			Args:      []string{"-u", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
		},
		{
			Name:      "skips when the times are equal",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new"), ModTime: oldTime}, "b.txt": {Data: []byte("old"), ModTime: oldTime}},
			Args:      []string{"-u", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
		},
		{
			Name:      "copies when the destination is missing",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new"), ModTime: oldTime}},
			Args:      []string{"-u", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
		},
		{
			Name: "skips only the outdated files of a hierarchy",
			Files: fsx.MemoryFS{
				"d/new.txt":   {Data: []byte("new"), ModTime: oldTime},
				"d/old.txt":   {Data: []byte("old"), ModTime: otherTime},
				"e/d/new.txt": {Data: []byte("kept"), ModTime: otherTime},
				"e/d/old.txt": {Data: []byte("kept"), ModTime: oldTime},
			},
			Args:      []string{"-Ru", "d", "e"},
			WantFiles: map[string]string{"e/d/new.txt": "new", "e/d/old.txt": "kept"},
		},
		{
			Name:      "a missing destination directory is still created",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f"), ModTime: oldTime}},
			Args:      []string{"-Ru", "d", "e"},
			WantFiles: map[string]string{"e/f.txt": "f"},
		},
		{
			Name:      "-n takes precedence over -u",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new"), ModTime: oldTime}, "b.txt": {Data: []byte("old"), ModTime: otherTime}},
			Args:      []string{"-nu", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
		},
		{
			Name:      "the long option works",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new"), ModTime: oldTime}, "b.txt": {Data: []byte("old"), ModTime: otherTime}},
			Args:      []string{"--update", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "new"},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunVerbose(t *testing.T) {
	tests := []testCase{
		{
			Name:      "names each copied file",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:      []string{"-v", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "x"},
			Want:      "'a.txt' -> 'b.txt'\n",
		},
		{
			Name:      "the long option works",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:      []string{"--verbose", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "x"},
			Want:      "'a.txt' -> 'b.txt'\n",
		},
		{
			Name:      "names each entry of a hierarchy",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}, "d/sub/g.txt": {Data: []byte("g")}},
			Args:      []string{"-Rv", "d", "e"},
			WantFiles: map[string]string{"e/f.txt": "f", "e/sub/g.txt": "g"},
			Want: "'d' -> 'e'\n" +
				"'d/f.txt' -> 'e/f.txt'\n" +
				"'d/sub' -> 'e/sub'\n" +
				"'d/sub/g.txt' -> 'e/sub/g.txt'\n",
		},
		{
			Name:      "names multiple sources copied into a directory",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}, "d/keep.txt": {}},
			Args:      []string{"-v", "a.txt", "b.txt", "d"},
			WantFiles: map[string]string{"d/a.txt": "a", "d/b.txt": "b"},
			Want:      "'a.txt' -> 'd/a.txt'\n'b.txt' -> 'd/b.txt'\n",
		},
		{
			Name:      "a skipped file isn't named",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			Args:      []string{"-nv", "a.txt", "b.txt"},
			WantFiles: map[string]string{"b.txt": "old"},
			Want:      "",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunTargetDirectory(t *testing.T) {
	tests := []testCase{
		{
			Name:      "copies into the target directory",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}, "d/keep.txt": {}},
			Args:      []string{"-t", "d", "a.txt", "b.txt"},
			WantFiles: map[string]string{"d/a.txt": "a", "d/b.txt": "b", "d/keep.txt": ""},
		},
		{
			Name:      "the long option with an attached value",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("a")}, "d/keep.txt": {}},
			Args:      []string{"--target-directory=d", "a.txt"},
			WantFiles: map[string]string{"d/a.txt": "a"},
		},
		{
			Name:      "the long option with the next argument",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("a")}, "d/keep.txt": {}},
			Args:      []string{"--target-directory", "d", "a.txt"},
			WantFiles: map[string]string{"d/a.txt": "a"},
		},
		{
			Name:      "the target must be an existing directory",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}},
			Args:      []string{"-t", "b.txt", "a.txt"},
			WantFiles: map[string]string{"b.txt": "b"},
			WantErr:   "cp: target 'b.txt' is not a directory\n",
			WantExit:  1,
		},
		{
			Name:     "a missing target is an error",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("a")}},
			Args:     []string{"-t", "d", "a.txt"},
			Absent:   []string{"d"},
			WantErr:  "cp: target 'd' is not a directory\n",
			WantExit: 1,
		},
		{
			Name:     "no sources is a usage error",
			Files:    fsx.MemoryFS{"d/keep.txt": {}},
			Args:     []string{"-t", "d"},
			WantErr:  "cp: missing file operand\nTry 'cp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:      "-t and -T conflict",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("a")}, "b.txt": {}},
			Args:      []string{"-t", "d", "-T", "a.txt", "b.txt"},
			WantFiles: map[string]string{"a.txt": "a"},
			WantErr:   "cp: cannot combine --target-directory (-t) and --no-target-directory (-T)\nTry 'cp --help' for more information.\n",
			WantExit:  2,
		},
		{
			Name:      "-T copies to an exact name",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "d/keep.txt": {}},
			Args:      []string{"-T", "a.txt", "d/b.txt"},
			WantFiles: map[string]string{"d/b.txt": "x", "d/keep.txt": ""},
		},
		{
			Name:      "-T keeps a directory destination from being used",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "d/keep.txt": {Data: []byte("keep")}},
			Args:      []string{"-T", "a.txt", "d"},
			WantFiles: map[string]string{"d/keep.txt": "keep"},
			WantErr:   "cp: cannot overwrite directory 'd' with non-directory 'a.txt'\n",
			WantExit:  1,
		},
		{
			Name:     "-T rejects extra operands",
			Files:    fsx.MemoryFS{"a.txt": {}, "b.txt": {}, "c.txt": {}},
			Args:     []string{"-T", "a.txt", "b.txt", "c.txt"},
			WantErr:  "cp: extra operand 'c.txt'\nTry 'cp --help' for more information.\n",
			WantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

// link is a MemoryFS entry holding a symbolic link to target.
func link(target string) *fsx.MemoryFile {
	return &fsx.MemoryFile{Mode: fs.ModeSymlink, Data: []byte(target)}
}

func TestRunSymlinks(t *testing.T) {
	tests := []testCase{
		{
			Name:      "a link operand is followed without -R",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt")},
			Args:      []string{"l", "b.txt"},
			WantFiles: map[string]string{"b.txt": "x"},
		},
		{
			Name:      "-H follows a link operand",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt")},
			Args:      []string{"-H", "l", "b.txt"},
			WantFiles: map[string]string{"b.txt": "x"},
		},
		{
			Name:      "-L follows a link operand",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt")},
			Args:      []string{"-L", "l", "b.txt"},
			WantFiles: map[string]string{"b.txt": "x"},
		},
		{
			Name:     "-d copies a link operand as a link",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt")},
			Args:     []string{"-d", "l", "b"},
			WantLink: map[string]string{"b": "a.txt"},
		},
		{
			Name:     "-P copies a link operand as a link",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt")},
			Args:     []string{"-P", "l", "b"},
			WantLink: map[string]string{"b": "a.txt"},
		},
		{
			Name:     "-d replaces an existing destination",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt"), "b": {Data: []byte("old")}},
			Args:     []string{"-d", "l", "b"},
			WantLink: map[string]string{"b": "a.txt"},
		},
		{
			Name:     "-d copies a dangling link",
			Files:    fsx.MemoryFS{"l": link("missing")},
			Args:     []string{"-d", "l", "b"},
			WantLink: map[string]string{"b": "missing"},
		},
		{
			Name:      "-d with -n keeps an existing destination",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt"), "b": {Data: []byte("old")}},
			Args:      []string{"-dn", "l", "b"},
			WantFiles: map[string]string{"b": "old"},
		},
		{
			Name:     "-d with -i prompts",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt"), "b": {Data: []byte("old")}},
			Stdin:    "y\n",
			Args:     []string{"-di", "l", "b"},
			WantLink: map[string]string{"b": "a.txt"},
			WantErr:  "cp: overwrite 'b'? ",
		},
		{
			Name:     "-R keeps the links of a hierarchy",
			Files:    fsx.MemoryFS{"d/a.txt": {Data: []byte("x")}, "d/l": link("a.txt")},
			Args:     []string{"-R", "d", "e"},
			WantLink: map[string]string{"e/l": "a.txt"},
		},
		{
			Name:     "-r keeps the links of a hierarchy",
			Files:    fsx.MemoryFS{"d/a.txt": {Data: []byte("x")}, "d/l": link("a.txt")},
			Args:     []string{"-r", "d", "e"},
			WantLink: map[string]string{"e/l": "a.txt"},
		},
		{
			Name:     "-R copies a link operand as a link",
			Files:    fsx.MemoryFS{"d/a.txt": {Data: []byte("x")}, "d/l": link("a.txt"), "e/keep.txt": {}},
			Args:     []string{"-R", "d", "e"},
			WantLink: map[string]string{"e/d/l": "a.txt"},
		},
		{
			Name:      "-L follows the links of a hierarchy",
			Files:     fsx.MemoryFS{"d/a.txt": {Data: []byte("x")}, "d/l": link("a.txt")},
			Args:      []string{"-RL", "d", "e"},
			WantFiles: map[string]string{"e/l": "x"},
		},
		{
			Name:      "-H dereferences only the command line operands",
			Files:     fsx.MemoryFS{"d/a.txt": {Data: []byte("x")}, "d/l": link("a.txt")},
			Args:      []string{"-RH", "d", "e"},
			WantFiles: map[string]string{"e/a.txt": "x"},
			WantLink:  map[string]string{"e/l": "a.txt"},
		},
		{
			Name:      "-H follows a directory link operand",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}, "l": link("d")},
			Args:      []string{"-RH", "l", "e"},
			WantFiles: map[string]string{"e/f.txt": "f"},
		},
		{
			Name:      "-L follows a directory link operand",
			Files:     fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}, "l": link("d")},
			Args:      []string{"-RL", "l", "e"},
			WantFiles: map[string]string{"e/f.txt": "f"},
		},
		{
			Name:     "-d copies a directory link operand as a link",
			Files:    fsx.MemoryFS{"d/f.txt": {Data: []byte("f")}, "l": link("d"), "e/keep.txt": {}},
			Args:     []string{"-dR", "l", "e"},
			WantLink: map[string]string{"e/l": "d"},
		},
		{
			Name:     "a link to a directory in a tree stays a link",
			Files:    fsx.MemoryFS{"d/sub/f.txt": {Data: []byte("f")}, "d/l": link("sub")},
			Args:     []string{"-R", "d", "e"},
			WantLink: map[string]string{"e/l": "sub"},
		},
		{
			Name:      "-L follows a link to a directory in a tree",
			Files:     fsx.MemoryFS{"d/sub/f.txt": {Data: []byte("f")}, "d/l": link("sub")},
			Args:      []string{"-RL", "d", "e"},
			WantFiles: map[string]string{"e/l/f.txt": "f"},
		},
		{
			Name:      "a symbolic link destination is followed",
			Files:     fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "b.txt": {Data: []byte("old")}, "b": link("b.txt")},
			Args:      []string{"a.txt", "b"},
			WantFiles: map[string]string{"b.txt": "x"},
		},
		{
			Name:     "a copied link is named with -v",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}, "l": link("a.txt")},
			Args:     []string{"-dv", "l", "b"},
			WantLink: map[string]string{"b": "a.txt"},
			Want:     "'l' -> 'b'\n",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSymlinksCycles(t *testing.T) {
	t.Run("-L on a link to an ancestor is refused", func(t *testing.T) {
		files := fsx.MemoryFS{
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
		files := fsx.MemoryFS{
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
	files := fsx.MemoryFS{
		"a.txt": {Data: []byte("x")},
		"dlink": link("missing"),
	}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: io.Discard, Stderr: stderr, FS: files}
	err := New(rt).Run(t.Context(), "a.txt", "dlink")
	if code := testutil.ExitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if want := "cp: not writing through dangling symlink 'dlink'\n"; stderr.String() != want {
		t.Errorf("Run() stderr = %q, want %q", stderr.String(), want)
	}
	if _, statErr := fs.Stat(files, "missing"); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("Stat(missing) error = %v, want it to stay missing", statErr)
	}
}

func TestRunSymlinksSystemFS(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(dir, "l")); err != nil {
		t.Skipf("symbolic links aren't supported here: %v", err)
	}

	tests := []struct {
		Name    string
		Args    []string
		dst     string
		want    string
		wantOut bool
	}{
		{
			Name: "a link operand is followed without -R",
			Args: []string{"l", "b"},
			dst:  "b",
			want: "x",
		},
		{
			Name:    "-d copies a link operand as a link",
			Args:    []string{"-d", "l", "b"},
			dst:     "b",
			wantOut: true,
		},
		{
			Name:    "-R keeps a link operand as a link",
			Args:    []string{"-R", "l", "b"},
			dst:     "b",
			wantOut: true,
		},
		{
			Name: "-L follows a link operand",
			Args: []string{"-L", "l", "c"},
			dst:  "c",
			want: "x",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			rt := &coreutils.Runtime{
				Stdout:     stdout,
				Stderr:     io.Discard,
				FS:         fsx.SystemFS(dir),
				WorkingDir: dir,
			}
			if err := New(rt).Run(t.Context(), test.Args...); err != nil {
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
			Name:     "--preserve keeps the mode and times",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o640, ModTime: oldTime}},
			Args:     []string{"--preserve", "a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": 0o640},
			WantTime: map[string]time.Time{"b.txt": oldTime},
		},
		{
			Name:     "--preserve=mode keeps only the mode",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o666, ModTime: oldTime}},
			Umask:    0o022,
			Args:     []string{"--preserve=mode", "a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": 0o666},
		},
		{
			Name:     "--preserve=timestamps keeps only the times",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o666, ModTime: oldTime}},
			Umask:    0o022,
			Args:     []string{"--preserve=timestamps", "a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": 0o644},
			WantTime: map[string]time.Time{"b.txt": oldTime},
		},
		{
			Name:     "attributes can be combined",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o640, ModTime: oldTime}},
			Args:     []string{"--preserve=mode,timestamps", "a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": 0o640},
			WantTime: map[string]time.Time{"b.txt": oldTime},
		},
		{
			Name:     "--no-preserve drops the mode and times",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o666, ModTime: oldTime}},
			Umask:    0o022,
			Args:     []string{"-p", "--no-preserve=mode,timestamps", "a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": 0o644},
		},
		{
			Name:     "an unknown attribute is an error",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:     []string{"--preserve=mode,bogus", "a.txt", "b.txt"},
			Absent:   []string{"b.txt"},
			WantErr:  "cp: invalid attribute list: 'mode,bogus'\nTry 'cp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown attribute of --no-preserve is an error",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x")}},
			Args:     []string{"--no-preserve=bogus", "a.txt", "b.txt"},
			Absent:   []string{"b.txt"},
			WantErr:  "cp: invalid attribute list: 'bogus'\nTry 'cp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "--no-preserve takes the next argument as its list",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o666, ModTime: oldTime}},
			Umask:    0o022,
			Args:     []string{"-p", "--no-preserve", "a.txt", "b.txt"},
			WantErr:  "cp: invalid attribute list: 'a.txt'\nTry 'cp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "--no-preserve= with an empty list is ambiguous",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o666, ModTime: oldTime}},
			Umask:    0o022,
			Args:     []string{"-p", "--no-preserve=", "a.txt", "b.txt"},
			WantErr:  "cp: ambiguous argument '' for '--no-preserve'\nTry 'cp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "--no-preserve=mode clears only the mode",
			Files:    fsx.MemoryFS{"a.txt": {Data: []byte("x"), Mode: 0o666, ModTime: oldTime}},
			Umask:    0o022,
			Args:     []string{"-p", "--no-preserve=mode", "a.txt", "b.txt"},
			WantMode: map[string]fs.FileMode{"b.txt": 0o644},
			WantTime: map[string]time.Time{"b.txt": oldTime},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunPreserveOwnership(t *testing.T) {
	files := fsx.MemoryFS{"a.txt": {Data: []byte("x"), Uid: 1000, Gid: 2000}}
	runCase(t, testCase{
		Files: files,
		Args:  []string{"--preserve=all", "a.txt", "b.txt"},
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

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "cp", usage)
}
