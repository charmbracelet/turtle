package tar

import (
	"archive/tar"
	"errors"
	"io/fs"
	"runtime"
	"strings"
	"testing"
	"time"

	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// fixture returns an archive holding a file, a directory and a nested file.
func fixture(t *testing.T) string {
	return buildArchive(t,
		entry{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
		entry{name: "sub/", typ: tar.TypeDir, mode: 0o755, mtime: oldTime},
		entry{name: "sub/a.txt", typ: tar.TypeReg, mode: 0o644, body: "a\n", mtime: oldTime},
	)
}

type extractCase struct {
	name     string
	files    fsx.MemFS
	stdin    string
	umask    fs.FileMode
	args     []string
	want     map[string]string
	wantMode map[string]fs.FileMode
	wantTime map[string]time.Time
	wantLink map[string]string
	absent   []string
	check    func(t *testing.T, files fsx.MemFS)
	wantErr  string
	wantExit int
}

func runExtract(t *testing.T, test extractCase) {
	t.Helper()

	if test.files == nil {
		test.files = fsx.MemFS{}
	}
	_, stderr, err := run(t, test.files, test.stdin, nil, test.umask, test.args...)
	if code := testutil.ExitCode(t, err); code != test.wantExit {
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
	}
	for _, name := range test.absent {
		if _, err := fs.Stat(test.files, name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat(%q) error = %v, want fs.ErrNotExist", name, err)
		}
	}
	if test.check != nil {
		test.check(t, test.files)
	}
}

func TestList(t *testing.T) {
	archive := fixture(t)
	base := buildArchive(t, entry{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, mtime: oldTime})
	tests := []struct {
		name     string
		files    fsx.MemFS
		stdin    string
		env      map[string]string
		args     []string
		wantOut  string
		wantErr  string
		wantExit int
	}{
		{
			name:    "names",
			args:    []string{"-tf", "-"},
			stdin:   archive,
			wantOut: "hello.txt\nsub/\nsub/a.txt\n",
		},
		{
			name:    "default archive is standard input",
			args:    []string{"-t"},
			stdin:   archive,
			wantOut: "hello.txt\nsub/\nsub/a.txt\n",
		},
		{
			name:    "TAPE archive",
			files:   fsx.MemFS{"a.tar": {Data: []byte(archive), Mode: 0o644}},
			args:    []string{"-t"},
			env:     map[string]string{"TAPE": "a.tar"},
			wantOut: "hello.txt\nsub/\nsub/a.txt\n",
		},
		{
			name:    "from file",
			files:   fsx.MemFS{"a.tar": {Data: []byte(archive), Mode: 0o644}},
			args:    []string{"-tf", "a.tar"},
			wantOut: "hello.txt\nsub/\nsub/a.txt\n",
		},
		{
			name:    "directory operand matches its children",
			args:    []string{"-tf", "-", "sub"},
			stdin:   archive,
			wantOut: "sub/\nsub/a.txt\n",
		},
		{
			name:  "long format",
			args:  []string{"-t", "-v", "-f", "-"},
			stdin: archive,
			wantOut: "-rw-r--r-- 0/0 6 2001-02-03 04:05 hello.txt\n" +
				"drwxr-xr-x 0/0 0 2001-02-03 04:05 sub/\n" +
				"-rw-r--r-- 0/0 2 2001-02-03 04:05 sub/a.txt\n",
		},
		{
			name:    "long format symlink",
			stdin:   buildArchive(t, entry{name: "link", typ: tar.TypeSymlink, mode: 0o777, link: "target", mtime: oldTime}),
			args:    []string{"-t", "-v", "-f", "-"},
			wantOut: "lrwxrwxrwx 0/0 0 2001-02-03 04:05 link -> target\n",
		},
		{
			name: "long format special entries",
			stdin: buildArchive(t,
				entry{name: "prog", typ: tar.TypeReg, mode: 0o4755, mtime: oldTime, uname: "root", gname: "wheel"},
				entry{name: "noperm", typ: tar.TypeReg, mode: 0o4644, mtime: oldTime, uid: 7, gid: 8},
				entry{name: "stickyd/", typ: tar.TypeDir, mode: 0o1755, mtime: oldTime},
				entry{name: "hard", typ: tar.TypeLink, link: "prog", mode: 0o644, mtime: oldTime},
				entry{name: "pipe", typ: tar.TypeFifo, mode: 0o644, mtime: oldTime},
				entry{name: "chr", typ: tar.TypeChar, mode: 0o600, mtime: oldTime},
				entry{name: "blk", typ: tar.TypeBlock, mode: 0o600, mtime: oldTime},
			),
			args: []string{"-t", "-v", "-f", "-"},
			wantOut: "-rwsr-xr-x root/wheel 0 2001-02-03 04:05 prog\n" +
				"-rwSr--r-- 7/8 0 2001-02-03 04:05 noperm\n" +
				"drwxr-xr-t 0/0 0 2001-02-03 04:05 stickyd/\n" +
				"hrw-r--r-- 0/0 0 2001-02-03 04:05 hard link to prog\n" +
				"prw-r--r-- 0/0 0 2001-02-03 04:05 pipe\n" +
				"crw------- 0/0 0 2001-02-03 04:05 chr\n" +
				"brw------- 0/0 0 2001-02-03 04:05 blk\n",
		},
		{
			name:     "operand not in archive",
			args:     []string{"-tf", "-", "missing"},
			stdin:    archive,
			wantErr:  "tar: missing: Not found in archive\n",
			wantExit: 1,
		},
		{
			name:     "empty archive",
			args:     []string{"-tf", "-"},
			stdin:    strings.Repeat("\x00", 10240),
			wantExit: 0,
		},
		{
			name:     "garbage archive",
			args:     []string{"-tf", "-"},
			stdin:    "not an archive at all",
			wantErr:  "tar: -: This does not look like a tar archive\n",
			wantExit: 1,
		},
		{
			name:     "empty file archive",
			args:     []string{"-tf", "-"},
			wantErr:  "tar: -: This does not look like a tar archive\n",
			wantExit: 1,
		},
		{
			name:     "invalid header",
			args:     []string{"-tf", "-"},
			stdin:    base[:len(base)-1024] + strings.Repeat("x", 512),
			wantOut:  "hello.txt\n",
			wantErr:  "tar: -: This does not look like a tar archive\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, err := run(t, test.files, test.stdin, test.env, 0, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantExit {
				t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
			}
			if got := stdout.String(); got != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", got, test.wantOut)
			}
			if got := stderr.String(); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
		})
	}
}

func TestExtract(t *testing.T) {
	tests := []extractCase{
		{
			name:  "files and directories",
			stdin: fixture(t),
			args:  []string{"-xf", "-"},
			want: map[string]string{
				"hello.txt": "hello\n",
				"sub/a.txt": "a\n",
			},
			wantMode: map[string]fs.FileMode{
				"hello.txt": 0o644,
				"sub":       fs.ModeDir | 0o755,
				"sub/a.txt": 0o644,
			},
			wantTime: map[string]time.Time{
				"hello.txt": oldTime,
				"sub":       oldTime,
				"sub/a.txt": oldTime,
			},
		},
		{
			name:  "umask masks modes",
			stdin: buildArchive(t, entry{name: "f.txt", typ: tar.TypeReg, mode: 0o777, body: "x"}),
			umask: 0o022,
			args:  []string{"-xf", "-"},
			wantMode: map[string]fs.FileMode{
				"f.txt": 0o755,
			},
		},
		{
			name:  "preserve modes and setuid",
			stdin: buildArchive(t, entry{name: "prog", typ: tar.TypeReg, mode: 0o4755, body: "x"}),
			umask: 0o022,
			args:  []string{"-xf", "-", "-p"},
			wantMode: map[string]fs.FileMode{
				"prog": fs.ModeSetuid | 0o755,
			},
		},
		{
			name:  "umask masks setuid without -p",
			stdin: buildArchive(t, entry{name: "prog", typ: tar.TypeReg, mode: 0o4755, body: "x"}),
			umask: 0o022,
			args:  []string{"-xf", "-"},
			wantMode: map[string]fs.FileMode{
				"prog": 0o755,
			},
		},
		{
			name:  "touch keeps current time",
			stdin: buildArchive(t, entry{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "x", mtime: oldTime}),
			args:  []string{"-xf", "-", "-m"},
			check: func(t *testing.T, files fsx.MemFS) {
				fi, err := fs.Stat(files, "f.txt")
				if err != nil {
					t.Fatalf("Stat(f.txt) error = %v", err)
				}
				if fi.ModTime().Equal(oldTime) {
					t.Errorf("f.txt modified time = %v, want the extraction time", fi.ModTime())
				}
			},
		},
		{
			name: "symlink",
			stdin: buildArchive(t,
				entry{name: "target", typ: tar.TypeReg, mode: 0o644, body: "t\n", mtime: oldTime},
				entry{name: "link", typ: tar.TypeSymlink, link: "target", mode: 0o777, mtime: oldTime},
			),
			args: []string{"-xf", "-"},
			want: map[string]string{
				"target": "t\n",
				"link":   "t\n",
			},
			wantLink: map[string]string{"link": "target"},
		},
		{
			name: "hard link",
			stdin: buildArchive(t,
				entry{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "X", mtime: oldTime},
				entry{name: "link", typ: tar.TypeLink, link: "f.txt", mode: 0o644, mtime: oldTime},
			),
			args: []string{"-xf", "-"},
			want: map[string]string{
				"f.txt": "X",
				"link":  "X",
			},
		},
		{
			name:     "symlink replaces a file",
			files:    fsx.MemFS{"link": {Data: []byte("old"), Mode: 0o644}},
			stdin:    buildArchive(t, entry{name: "link", typ: tar.TypeSymlink, link: "target", mode: 0o777}),
			args:     []string{"-xf", "-"},
			wantLink: map[string]string{"link": "target"},
		},
		{
			name: "hard link before its target",
			stdin: buildArchive(t,
				entry{name: "link", typ: tar.TypeLink, link: "f.txt", mode: 0o644, mtime: oldTime},
				entry{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "X", mtime: oldTime},
			),
			args: []string{"-xf", "-"},
			want: map[string]string{
				"f.txt": "X",
				"link":  "X",
			},
		},
		{
			name: "hard link to a missing target",
			stdin: buildArchive(t,
				entry{name: "link", typ: tar.TypeLink, link: "f.txt", mode: 0o644, mtime: oldTime},
			),
			args:     []string{"-xf", "-"},
			absent:   []string{"link"},
			wantErr:  "tar: link: file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "leading slash stripped",
			stdin:   buildArchive(t, entry{name: "/sub/f.txt", typ: tar.TypeReg, mode: 0o644, body: "x", mtime: oldTime}),
			args:    []string{"-xf", "-"},
			want:    map[string]string{"sub/f.txt": "x"},
			wantErr: "tar: Removing leading '/' from member names\n",
		},
		{
			name:     "parent directory escape refused",
			stdin:    buildArchive(t, entry{name: "sub/../evil.txt", typ: tar.TypeReg, mode: 0o644, body: "x"}),
			args:     []string{"-xf", "-"},
			absent:   []string{"evil.txt"},
			wantErr:  "tar: sub/../evil.txt: Member name contains '..'\n",
			wantExit: 1,
		},
		{
			name:     "absolute escape refused",
			stdin:    buildArchive(t, entry{name: "../evil.txt", typ: tar.TypeReg, mode: 0o644, body: "x"}),
			args:     []string{"-xf", "-"},
			absent:   []string{"evil.txt"},
			wantErr:  "tar: ../evil.txt: Member name contains '..'\n",
			wantExit: 1,
		},
		{
			name: "symbolic link in the path is refused",
			files: fsx.MemFS{
				"evil/old.txt": {Data: []byte("old"), Mode: 0o644},
				"link":         {Mode: fs.ModeSymlink | 0o777, Data: []byte("evil")},
			},
			stdin:    buildArchive(t, entry{name: "link/pwned.txt", typ: tar.TypeReg, mode: 0o644, body: "x"}),
			args:     []string{"-xf", "-"},
			absent:   []string{"link/pwned.txt", "evil/pwned.txt"},
			want:     map[string]string{"evil/old.txt": "old"},
			wantErr:  "tar: link/pwned.txt: refusing to extract through symbolic link 'link'\n",
			wantExit: 1,
		},
		{
			name: "symbolic link higher in the path is refused",
			files: fsx.MemFS{
				"evil/old.txt": {Data: []byte("old"), Mode: 0o644},
				"a/link":       {Mode: fs.ModeSymlink | 0o777, Data: []byte("../evil")},
			},
			stdin:    buildArchive(t, entry{name: "a/link/deep/x.txt", typ: tar.TypeReg, mode: 0o644, body: "x"}),
			args:     []string{"-xf", "-"},
			absent:   []string{"evil/deep/x.txt"},
			wantErr:  "tar: a/link/deep/x.txt: refusing to extract through symbolic link 'a/link'\n",
			wantExit: 1,
		},
		{
			name: "regular file replaces a symbolic link",
			files: fsx.MemFS{
				"target": {Data: []byte("OLD"), Mode: 0o644},
				"f.txt":  {Mode: fs.ModeSymlink | 0o777, Data: []byte("target")},
			},
			stdin: buildArchive(t, entry{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "new"}),
			args:  []string{"-xf", "-"},
			want:  map[string]string{"f.txt": "new", "target": "OLD"},
			check: func(t *testing.T, files fsx.MemFS) {
				if _, err := files.ReadLink("f.txt"); err == nil {
					t.Errorf("ReadLink(f.txt) succeeded, want f.txt to be a regular file")
				}
			},
		},
		{
			name: "directory over a symbolic link is refused",
			files: fsx.MemFS{
				"evil/old.txt": {Data: []byte("old"), Mode: 0o644},
				"link":         {Mode: fs.ModeSymlink | 0o777, Data: []byte("evil")},
			},
			stdin:    buildArchive(t, entry{name: "link/", typ: tar.TypeDir, mode: 0o755}),
			args:     []string{"-xf", "-"},
			wantErr:  "tar: link/: not a directory\n",
			wantExit: 1,
		},
		{
			name:     "file over directory",
			files:    fsx.MemFS{"x/in.txt": {Data: []byte("i"), Mode: 0o644}},
			stdin:    buildArchive(t, entry{name: "x", typ: tar.TypeReg, mode: 0o644, body: "x"}),
			args:     []string{"-xf", "-"},
			wantErr:  "tar: x: invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "directory over file",
			files:    fsx.MemFS{"x": {Data: []byte("f"), Mode: 0o644}},
			stdin:    buildArchive(t, entry{name: "x/", typ: tar.TypeDir, mode: 0o755}),
			args:     []string{"-xf", "-"},
			wantErr:  "tar: x/: not a directory\n",
			wantExit: 1,
		},
		{
			name:     "special file",
			stdin:    buildArchive(t, entry{name: "pipe", typ: tar.TypeFifo, mode: 0o644}),
			args:     []string{"-xf", "-"},
			absent:   []string{"pipe"},
			wantErr:  "tar: pipe: cannot create special files\n",
			wantExit: 1,
		},
		{
			name:     "ignore errors doesn't apply to extraction",
			stdin:    buildArchive(t, entry{name: "pipe", typ: tar.TypeFifo, mode: 0o644}),
			args:     []string{"-xf", "-", "-e"},
			absent:   []string{"pipe"},
			wantErr:  "tar: pipe: cannot create special files\n",
			wantExit: 1,
		},
		{
			name:  "overwrite truncates",
			files: fsx.MemFS{"f.txt": {Data: []byte("old longer content"), Mode: 0o644}},
			stdin: buildArchive(t, entry{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "new"}),
			args:  []string{"-xf", "-"},
			want:  map[string]string{"f.txt": "new"},
		},
		{
			name: "later entry wins",
			stdin: buildArchive(t,
				entry{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "first"},
				entry{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "second"},
			),
			args: []string{"-xf", "-"},
			want: map[string]string{"f.txt": "second"},
		},
		{
			name:   "filter by operand",
			stdin:  fixture(t),
			args:   []string{"-xf", "-", "sub"},
			want:   map[string]string{"sub/a.txt": "a\n"},
			absent: []string{"hello.txt"},
		},
		{
			name:     "operand not in archive",
			stdin:    fixture(t),
			args:     []string{"-xf", "-", "sub2"},
			wantErr:  "tar: sub2: Not found in archive\n",
			wantExit: 1,
		},
		{
			name:    "declined file is left out",
			stdin:   fixture(t),
			args:    []string{"-xf", "-", "-w"},
			absent:  []string{"hello.txt", "sub/a.txt"},
			wantErr: "tar: extract 'hello.txt'? tar: extract 'sub/'? tar: extract 'sub/a.txt'? ",
		},
		{
			name:  "accepted file is extracted",
			files: fsx.MemFS{"a.tar": {Data: []byte(fixture(t)), Mode: 0o644}},
			stdin: "y\nn\ny\n",
			args:  []string{"-xf", "a.tar", "-w"},
			want: map[string]string{
				"hello.txt": "hello\n",
				"sub/a.txt": "a\n",
			},
			wantErr: "tar: extract 'hello.txt'? tar: extract 'sub/'? tar: extract 'sub/a.txt'? ",
		},
		{
			name:    "verbose names on stderr",
			stdin:   fixture(t),
			args:    []string{"-xvf", "-"},
			wantErr: "tar: hello.txt\ntar: sub/\ntar: sub/a.txt\n",
			want: map[string]string{
				"hello.txt": "hello\n",
				"sub/a.txt": "a\n",
			},
		},
		{
			name:  "read-only file mode",
			stdin: buildArchive(t, entry{name: "f.txt", typ: tar.TypeReg, mode: 0o444, body: "x"}),
			args:  []string{"-xf", "-"},
			want:  map[string]string{"f.txt": "x"},
			wantMode: map[string]fs.FileMode{
				"f.txt": 0o444,
			},
		},
		{
			name:  "implicit parents",
			stdin: buildArchive(t, entry{name: "deep/nest/f.txt", typ: tar.TypeReg, mode: 0o644, body: "x"}),
			args:  []string{"-xf", "-"},
			want:  map[string]string{"deep/nest/f.txt": "x"},
		},
		{
			name:  "empty archive",
			stdin: strings.Repeat("\x00", 10240),
			args:  []string{"-xf", "-"},
		},
		{
			name:     "garbage archive",
			stdin:    "not an archive at all",
			args:     []string{"-xf", "-"},
			wantErr:  "tar: -: This does not look like a tar archive\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runExtract(t, test)
		})
	}
}

func TestExtractWindowsSeparators(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("backslash is a separator only on Windows")
	}
	tests := []extractCase{
		{
			name:     "backslash parent escape refused",
			stdin:    buildArchive(t, entry{name: `..\evil.txt`, typ: tar.TypeReg, mode: 0o644, body: "x"}),
			args:     []string{"-xf", "-"},
			wantErr:  `tar: ..\evil.txt: Member name contains '..'` + "\n",
			wantExit: 1,
		},
		{
			name:     "backslash nested escape refused",
			stdin:    buildArchive(t, entry{name: `sub\..\evil.txt`, typ: tar.TypeReg, mode: 0o644, body: "x"}),
			args:     []string{"-xf", "-"},
			wantErr:  `tar: sub\..\evil.txt: Member name contains '..'` + "\n",
			wantExit: 1,
		},
		{
			name:     "backslash drive escape refused",
			stdin:    buildArchive(t, entry{name: `C:\evil.txt`, typ: tar.TypeReg, mode: 0o644, body: "x"}),
			args:     []string{"-xf", "-"},
			want:     map[string]string{"evil.txt": "x"},
			wantErr:  "tar: Removing leading '/' from member names\n",
			wantExit: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runExtract(t, test)
		})
	}
}
