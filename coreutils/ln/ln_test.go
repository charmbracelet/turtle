package ln

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"runtime"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

type testCase struct {
	name      string
	files     fsx.MemFS
	wd        string
	args      []string
	want      map[string]string
	wantLinks map[string]string
	wantSame  map[string]string
	present   []string
	absent    []string
	wantErr   string
	wantExit  int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      strings.NewReader(""),
		Stdout:     io.Discard,
		Stderr:     stderr,
		FS:         test.files,
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
	for name, want := range test.wantLinks {
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
	for name, other := range test.wantSame {
		if test.files[name] != test.files[other] {
			t.Errorf("%s and %s are different entries, want the same", name, other)
		}
	}
	for _, name := range test.present {
		if _, err := test.files.Lstat(name); err != nil {
			t.Errorf("Lstat(%q) error = %v, want it to still exist", name, err)
		}
	}
	for _, name := range test.absent {
		if _, err := test.files.Lstat(name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Lstat(%q) error = %v, want fs.ErrNotExist", name, err)
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
			name:     "creates a hard link to a file",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}},
			args:     []string{"a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "hello"},
			wantSame: map[string]string{"b.txt": "a.txt"},
			present:  []string{"a.txt"},
		},
		{
			name:     "creates a hard link into an existing directory",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}, "d/keep.txt": {Data: []byte("keep")}},
			args:     []string{"a.txt", "d"},
			want:     map[string]string{"d/a.txt": "hello"},
			wantSame: map[string]string{"d/a.txt": "a.txt"},
			present:  []string{"d/keep.txt"},
		},
		{
			name:     "links multiple files into a directory",
			files:    fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}, "d/keep.txt": {}},
			args:     []string{"a.txt", "b.txt", "d"},
			want:     map[string]string{"d/a.txt": "a", "d/b.txt": "b"},
			wantSame: map[string]string{"d/a.txt": "a.txt", "d/b.txt": "b.txt"},
		},
		{
			name:     "a path with a directory component links to its base name",
			files:    fsx.MemFS{"sub/a.txt": {Data: []byte("hello")}, "d/keep.txt": {}},
			args:     []string{"sub/a.txt", "d"},
			want:     map[string]string{"d/a.txt": "hello"},
			wantSame: map[string]string{"d/a.txt": "sub/a.txt"},
		},
		{
			name:     "links through a symbolic link to a directory",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}, "real/keep.txt": {}, "dlink": {Data: []byte("real"), Mode: fs.ModeSymlink}},
			args:     []string{"a.txt", "dlink"},
			want:     map[string]string{"dlink/a.txt": "hello"},
			wantSame: map[string]string{"real/a.txt": "a.txt"},
		},
		{
			name:     "keeps going after a failing source",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}, "d/keep.txt": {}},
			args:     []string{"missing.txt", "a.txt", "d"},
			want:     map[string]string{"d/a.txt": "hello"},
			wantErr:  "ln: cannot create link 'd/missing.txt' to 'missing.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing source is an error",
			files:    fsx.MemFS{},
			args:     []string{"missing.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "ln: cannot create link 'b.txt' to 'missing.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a directory source is an error",
			files:    fsx.MemFS{"d/keep.txt": {}},
			args:     []string{"d", "link"},
			absent:   []string{"link"},
			wantErr:  "ln: cannot create link 'link' to 'd': permission denied\n",
			wantExit: 1,
		},
		{
			name:     "an existing destination is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:     []string{"a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "old"},
			wantErr:  "ln: cannot create link 'b.txt': file exists\n",
			wantExit: 1,
		},
		{
			name:    "a dangling symbolic link still exists as a destination",
			files:   fsx.MemFS{"a.txt": {Data: []byte("hello")}, "b.txt": {Data: []byte("missing.txt"), Mode: fs.ModeSymlink}},
			args:    []string{"a.txt", "b.txt"},
			present: []string{"b.txt"},
			wantErr: "ln: cannot create link 'b.txt': file exists\n",
			// The dangling link is left untouched.
			wantLinks: map[string]string{"b.txt": "missing.txt"},
			wantExit:  1,
		},
		{
			name:     "a directory destination is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}, "d/a.txt": {Mode: fs.ModeDir}},
			args:     []string{"a.txt", "d"},
			wantErr:  "ln: cannot create link 'd/a.txt': is a directory\n",
			wantExit: 1,
		},
		{
			name:     "the source and destination are the same file",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}},
			args:     []string{"a.txt", "a.txt"},
			want:     map[string]string{"a.txt": "hello"},
			wantErr:  "ln: 'a.txt' and 'a.txt' are the same file\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSymbolic(t *testing.T) {
	tests := []testCase{
		{
			name:      "creates a symbolic link",
			files:     fsx.MemFS{"a.txt": {Data: []byte("hello")}},
			args:      []string{"-s", "a.txt", "b.txt"},
			want:      map[string]string{"b.txt": "hello"},
			wantLinks: map[string]string{"b.txt": "a.txt"},
			present:   []string{"a.txt"},
		},
		{
			name:      "creates a dangling symbolic link",
			files:     fsx.MemFS{},
			args:      []string{"-s", "missing.txt", "b.txt"},
			wantLinks: map[string]string{"b.txt": "missing.txt"},
		},
		{
			name:      "stores the target as typed",
			files:     fsx.MemFS{"sub/a.txt": {Data: []byte("hello")}},
			args:      []string{"-s", "sub/a.txt", "b.txt"},
			want:      map[string]string{"b.txt": "hello"},
			wantLinks: map[string]string{"b.txt": "sub/a.txt"},
		},
		{
			name:      "creates links into a directory",
			files:     fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}, "d/keep.txt": {}},
			args:      []string{"-s", "a.txt", "b.txt", "d"},
			wantLinks: map[string]string{"d/a.txt": "a.txt", "d/b.txt": "b.txt"},
		},
		{
			name:      "an existing destination is an error",
			files:     fsx.MemFS{"a.txt": {Data: []byte("hello")}, "b.txt": {Data: []byte("old")}},
			args:      []string{"-s", "a.txt", "b.txt"},
			want:      map[string]string{"b.txt": "old"},
			wantErr:   "ln: cannot create symbolic link 'b.txt': file exists\n",
			wantExit:  1,
			wantLinks: nil,
		},
		{
			name:     "a directory destination is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}, "d/a.txt": {Mode: fs.ModeDir}},
			args:     []string{"-s", "a.txt", "d"},
			wantErr:  "ln: cannot create symbolic link 'd/a.txt': is a directory\n",
			wantExit: 1,
		},
		{
			name:    "the source and destination are the same file",
			files:   fsx.MemFS{"a.txt": {Data: []byte("hello")}},
			args:    []string{"-s", "a.txt", "a.txt"},
			want:    map[string]string{"a.txt": "hello"},
			wantErr: "ln: 'a.txt' and 'a.txt' are the same file\n",
			// The file is left untouched.
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunForce(t *testing.T) {
	tests := []testCase{
		{
			name:     "replaces an existing file",
			files:    fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:     []string{"-f", "a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "new"},
			wantSame: map[string]string{"b.txt": "a.txt"},
		},
		{
			name:      "replaces an existing file with a symbolic link",
			files:     fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:      []string{"-sf", "a.txt", "b.txt"},
			want:      map[string]string{"b.txt": "new"},
			wantLinks: map[string]string{"b.txt": "a.txt"},
		},
		{
			name:      "replaces a dangling symbolic link",
			files:     fsx.MemFS{"a.txt": {Data: []byte("hello")}, "b.txt": {Data: []byte("missing.txt"), Mode: fs.ModeSymlink}},
			args:      []string{"-f", "a.txt", "b.txt"},
			want:      map[string]string{"b.txt": "hello"},
			wantSame:  map[string]string{"b.txt": "a.txt"},
			wantLinks: nil,
		},
		{
			name:     "removes the destination even when the link then fails",
			files:    fsx.MemFS{"b.txt": {Data: []byte("old")}},
			args:     []string{"-f", "missing.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "ln: cannot create link 'b.txt' to 'missing.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a directory destination is still an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}, "d/a.txt": {Mode: fs.ModeDir}},
			args:     []string{"-f", "a.txt", "d"},
			wantErr:  "ln: cannot create link 'd/a.txt': is a directory\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLogical(t *testing.T) {
	// sym.txt points at a.txt, so hard linking it either shares the link
	// itself (the default, -P) or the file it refers to (-L).
	linked := func() fsx.MemFS {
		return fsx.MemFS{
			"a.txt":   {Data: []byte("hello")},
			"sym.txt": {Data: []byte("a.txt"), Mode: fs.ModeSymlink},
		}
	}

	tests := []testCase{
		{
			name:     "the default links the symbolic link itself",
			files:    linked(),
			args:     []string{"sym.txt", "hard.txt"},
			want:     map[string]string{"hard.txt": "hello"},
			wantSame: map[string]string{"hard.txt": "sym.txt"},
		},
		{
			name:     "P links the symbolic link itself",
			files:    linked(),
			args:     []string{"-P", "sym.txt", "hard.txt"},
			want:     map[string]string{"hard.txt": "hello"},
			wantSame: map[string]string{"hard.txt": "sym.txt"},
		},
		{
			name:     "L links the file the symbolic link refers to",
			files:    linked(),
			args:     []string{"-L", "sym.txt", "hard.txt"},
			want:     map[string]string{"hard.txt": "hello"},
			wantSame: map[string]string{"hard.txt": "a.txt"},
		},
		{
			name:     "L follows a chain of symbolic links",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}, "sym.txt": {Data: []byte("a.txt"), Mode: fs.ModeSymlink}, "sym2.txt": {Data: []byte("sym.txt"), Mode: fs.ModeSymlink}},
			args:     []string{"-L", "sym2.txt", "hard.txt"},
			wantSame: map[string]string{"hard.txt": "a.txt"},
		},
		{
			name:     "L on a regular file is a plain hard link",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}},
			args:     []string{"-L", "a.txt", "hard.txt"},
			wantSame: map[string]string{"hard.txt": "a.txt"},
		},
		{
			name:     "the last of L and P wins",
			files:    linked(),
			args:     []string{"-LP", "sym.txt", "hard.txt"},
			wantSame: map[string]string{"hard.txt": "sym.txt"},
		},
		{
			name:     "the last of P and L wins",
			files:    linked(),
			args:     []string{"-PL", "sym.txt", "hard.txt"},
			wantSame: map[string]string{"hard.txt": "a.txt"},
		},
		{
			name:      "s makes L ignored",
			files:     fsx.MemFS{},
			args:      []string{"-sL", "missing.txt", "b.txt"},
			wantLinks: map[string]string{"b.txt": "missing.txt"},
		},
		{
			name:     "L on a dangling symbolic link is an error",
			files:    fsx.MemFS{"dangling.txt": {Data: []byte("missing.txt"), Mode: fs.ModeSymlink}},
			args:     []string{"-L", "dangling.txt", "hard.txt"},
			absent:   []string{"hard.txt"},
			wantErr:  "ln: cannot stat 'dangling.txt': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "L reports a loop",
			files:    fsx.MemFS{"loop": {Data: []byte("loop"), Mode: fs.ModeSymlink}},
			args:     []string{"-L", "loop", "hard.txt"},
			absent:   []string{"hard.txt"},
			wantErr:  "ln: cannot stat 'loop': too many levels of symbolic links\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFlags(t *testing.T) {
	tests := []testCase{
		{
			name:      "combined short options",
			files:     fsx.MemFS{"a.txt": {Data: []byte("hello")}, "b.txt": {Data: []byte("old")}},
			args:      []string{"-fs", "a.txt", "b.txt"},
			want:      map[string]string{"b.txt": "hello"},
			wantLinks: map[string]string{"b.txt": "a.txt"},
		},
		{
			name:     "options after an operand are operands",
			files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}},
			args:     []string{"a.txt", "-s"},
			want:     map[string]string{"-s": "hello"},
			wantSame: map[string]string{"-s": "a.txt"},
		},
		{
			name:     "a double dash ends option parsing",
			files:    fsx.MemFS{"-a.txt": {Data: []byte("hello")}},
			args:     []string{"--", "-a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "hello"},
			wantSame: map[string]string{"b.txt": "-a.txt"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLongOptions(t *testing.T) {
	linked := func() fsx.MemFS {
		return fsx.MemFS{
			"a.txt":   {Data: []byte("hello")},
			"sym.txt": {Data: []byte("a.txt"), Mode: fs.ModeSymlink},
		}
	}
	tests := []testCase{
		{
			name:      "symbolic creates a symbolic link",
			files:     fsx.MemFS{"a.txt": {Data: []byte("hello")}},
			args:      []string{"--symbolic", "a.txt", "b.txt"},
			want:      map[string]string{"b.txt": "hello"},
			wantLinks: map[string]string{"b.txt": "a.txt"},
		},
		{
			name:     "force replaces an existing file",
			files:    fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
			args:     []string{"--force", "a.txt", "b.txt"},
			want:     map[string]string{"b.txt": "new"},
			wantSame: map[string]string{"b.txt": "a.txt"},
		},
		{
			name:     "logical links the file the symbolic link refers to",
			files:    linked(),
			args:     []string{"--logical", "sym.txt", "hard.txt"},
			wantSame: map[string]string{"hard.txt": "a.txt"},
		},
		{
			name:     "physical links the symbolic link itself",
			files:    linked(),
			args:     []string{"--physical", "sym.txt", "hard.txt"},
			wantSame: map[string]string{"hard.txt": "sym.txt"},
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
			wantErr:  "ln: missing file operand\n",
			wantExit: 2,
		},
		{
			name:     "missing destination operand",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt"},
			wantErr:  "ln: missing destination file operand after 'a.txt'\n",
			wantExit: 2,
		},
		{
			name:     "invalid option",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"-x", "a.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "ln: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"--bogus", "a.txt", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "ln: unrecognized option '--bogus'\n",
			wantExit: 2,
		},
		{
			name:     "an empty source",
			files:    fsx.MemFS{"a.txt": {}},
			args:     []string{"", "b.txt"},
			absent:   []string{"b.txt"},
			wantErr:  "ln: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "an empty target",
			files:    fsx.MemFS{"a.txt": {}},
			args:     []string{"a.txt", ""},
			present:  []string{"a.txt"},
			wantErr:  "ln: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "a file as target with multiple sources",
			files:    fsx.MemFS{"a.txt": {}, "c.txt": {}, "b.txt": {Data: []byte("old")}},
			args:     []string{"a.txt", "c.txt", "b.txt"},
			present:  []string{"a.txt", "c.txt", "b.txt"},
			wantErr:  "ln: target 'b.txt' is not a directory\n",
			wantExit: 1,
		},
		{
			name:     "a trailing slash requires a directory",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}},
			args:     []string{"a.txt", "missing/"},
			present:  []string{"a.txt"},
			absent:   []string{"missing"},
			wantErr:  "ln: target 'missing/' is not a directory\n",
			wantExit: 1,
		},
		{
			name:     "a trailing slash onto a file is an error",
			files:    fsx.MemFS{"a.txt": {Data: []byte("x")}, "b.txt": {Data: []byte("y")}},
			args:     []string{"a.txt", "b.txt/"},
			present:  []string{"a.txt", "b.txt"},
			wantErr:  "ln: target 'b.txt/' is not a directory\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunWorkingDir(t *testing.T) {
	runCase(t, testCase{
		name:     "resolves operands relative to the working directory",
		files:    fsx.MemFS{"work/a.txt": {Data: []byte("hello")}},
		wd:       "work",
		args:     []string{"a.txt", "b.txt"},
		want:     map[string]string{"work/b.txt": "hello"},
		wantSame: map[string]string{"work/b.txt": "work/a.txt"},
	})
}

func TestRunWindowsSeparators(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only")
	}

	t.Run("backslashes are treated as separators in a source", func(t *testing.T) {
		runCase(t, testCase{
			name:     "hard link",
			files:    fsx.MemFS{"d/f.txt": {Data: []byte("hello")}},
			args:     []string{"d\\f.txt", "hard.txt"},
			want:     map[string]string{"hard.txt": "hello"},
			wantSame: map[string]string{"hard.txt": "d/f.txt"},
		})
	})

	t.Run("a symbolic link target is stored with forward slashes", func(t *testing.T) {
		runCase(t, testCase{
			name:      "symbolic link",
			files:     fsx.MemFS{"d/f.txt": {Data: []byte("hello")}},
			args:      []string{"-s", "d\\f.txt", "sym.txt"},
			wantLinks: map[string]string{"sym.txt": "d/f.txt"},
		})
	})
}

func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rt := &coreutils.Runtime{Stderr: io.Discard, FS: fsx.MemFS{"a.txt": {}}}
	err := New(rt).Run(ctx, "a.txt", "b.txt")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}
