package tar

import (
	"archive/tar"
	"io/fs"
	"testing"
	"time"

	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

var newTime = oldTime.Add(time.Hour)

func TestAppend(t *testing.T) {
	tests := []struct {
		name     string
		files    fsx.MemoryFS
		args     []string
		archive  string
		want     []entry
		absent   []string
		wantErr  string
		wantExit int
		record   int
	}{
		{
			name: "append to an archive",
			files: fsx.MemoryFS{
				"a.tar": {Data: []byte(buildArchive(t, entry{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime})), Mode: 0o644},
				"f.txt": {Data: []byte("f\n"), Mode: 0o644, ModTime: newTime},
			},
			args:    []string{"-rf", "a.tar", "f.txt"},
			archive: "a.tar",
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
				{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "f\n", mtime: newTime},
			},
			absent: []string{"a.tar.tmp"},
			record: 10240,
		},
		{
			name: "verbose append",
			files: fsx.MemoryFS{
				"a.tar": {Data: []byte(buildArchive(t)), Mode: 0o644},
				"f.txt": {Data: []byte("f\n"), Mode: 0o644, ModTime: newTime},
			},
			args:    []string{"-rvf", "a.tar", "f.txt"},
			archive: "a.tar",
			want:    []entry{{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "f\n", mtime: newTime}},
			wantErr: "tar: f.txt\n",
		},
		{
			name:     "archive on a standard stream",
			files:    fsx.MemoryFS{"f.txt": {Data: []byte("f\n"), Mode: 0o644}},
			args:     []string{"-rf", "-", "f.txt"},
			wantErr:  "tar: cannot update an archive on a standard stream\n",
			wantExit: 1,
		},
		{
			name:     "missing archive",
			files:    fsx.MemoryFS{"f.txt": {Data: []byte("f\n"), Mode: 0o644}},
			args:     []string{"-rf", "a.tar", "f.txt"},
			wantErr:  "tar: a.tar: file does not exist\n",
			wantExit: 1,
		},
		{
			name: "no operands keeps the archive",
			files: fsx.MemoryFS{
				"a.tar": {Data: []byte(buildArchive(t, entry{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime})), Mode: 0o644},
			},
			args:    []string{"-rf", "a.tar"},
			archive: "a.tar",
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
			},
		},
		{
			name: "append to a corrupt archive",
			files: fsx.MemoryFS{
				"a.tar": {Data: []byte("garbage"), Mode: 0o644},
				"f.txt": {Data: []byte("f\n"), Mode: 0o644},
			},
			args:     []string{"-rf", "a.tar", "f.txt"},
			wantErr:  "tar: a.tar: unexpected EOF\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, stderr, err := run(t, test.files, "", nil, 0, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantExit {
				t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
			}
			if got := stderr.String(); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
			if test.archive == "" {
				return
			}
			data, err := fs.ReadFile(test.files, test.archive)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", test.archive, err)
			}
			if test.record > 0 && len(data)%test.record != 0 {
				t.Errorf("archive length = %d, not a multiple of %d", len(data), test.record)
			}
			compareEntries(t, test.want, parseArchive(t, string(data)))
			for _, name := range test.absent {
				if _, err := fs.Stat(test.files, name); err == nil {
					t.Errorf("Stat(%q) error = nil, want it removed", name)
				}
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	base := buildArchive(t,
		entry{name: "sub/", typ: tar.TypeDir, mode: 0o755, mtime: oldTime},
		entry{name: "sub/f.txt", typ: tar.TypeReg, mode: 0o644, body: "s\n", mtime: oldTime},
		entry{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "old\n", mtime: oldTime},
	)
	tests := []struct {
		name     string
		files    fsx.MemoryFS
		args     []string
		want     []entry
		wantErr  string
		wantExit int
	}{
		{
			name: "older file is skipped",
			files: fsx.MemoryFS{
				"a.tar": {Data: []byte(base), Mode: 0o644},
				"f.txt": {Data: []byte("old\n"), Mode: 0o644, ModTime: oldTime},
			},
			args: []string{"-uf", "a.tar", "f.txt"},
			want: []entry{
				{name: "sub/", typ: tar.TypeDir, mode: 0o755, mtime: oldTime},
				{name: "sub/f.txt", typ: tar.TypeReg, mode: 0o644, body: "s\n", mtime: oldTime},
				{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "old\n", mtime: oldTime},
			},
		},
		{
			name: "newer file is appended",
			files: fsx.MemoryFS{
				"a.tar": {Data: []byte(base), Mode: 0o644},
				"f.txt": {Data: []byte("new\n"), Mode: 0o644, ModTime: newTime},
			},
			args: []string{"-uf", "a.tar", "f.txt"},
			want: []entry{
				{name: "sub/", typ: tar.TypeDir, mode: 0o755, mtime: oldTime},
				{name: "sub/f.txt", typ: tar.TypeReg, mode: 0o644, body: "s\n", mtime: oldTime},
				{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "old\n", mtime: oldTime},
				{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "new\n", mtime: newTime},
			},
		},
		{
			name: "absent file is appended",
			files: fsx.MemoryFS{
				"a.tar": {Data: []byte(base), Mode: 0o644},
				"g.txt": {Data: []byte("g\n"), Mode: 0o644, ModTime: oldTime},
			},
			args: []string{"-uf", "a.tar", "g.txt"},
			want: []entry{
				{name: "sub/", typ: tar.TypeDir, mode: 0o755, mtime: oldTime},
				{name: "sub/f.txt", typ: tar.TypeReg, mode: 0o644, body: "s\n", mtime: oldTime},
				{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "old\n", mtime: oldTime},
				{name: "g.txt", typ: tar.TypeReg, mode: 0o644, body: "g\n", mtime: oldTime},
			},
		},
		{
			name: "children of an unchanged directory are still considered",
			files: fsx.MemoryFS{
				"a.tar":     {Data: []byte(base), Mode: 0o644},
				"sub/f.txt": {Data: []byte("s\n"), Mode: 0o644, ModTime: oldTime},
				"sub/g.txt": {Data: []byte("g\n"), Mode: 0o644, ModTime: oldTime},
			},
			args: []string{"-uf", "a.tar", "sub"},
			want: []entry{
				{name: "sub/", typ: tar.TypeDir, mode: 0o755, mtime: oldTime},
				{name: "sub/f.txt", typ: tar.TypeReg, mode: 0o644, body: "s\n", mtime: oldTime},
				{name: "f.txt", typ: tar.TypeReg, mode: 0o644, body: "old\n", mtime: oldTime},
				{name: "sub/g.txt", typ: tar.TypeReg, mode: 0o644, body: "g\n", mtime: oldTime},
			},
		},
		{
			name: "verbose lists only the appended files",
			files: fsx.MemoryFS{
				"a.tar":     {Data: []byte(base), Mode: 0o644},
				"f.txt":     {Data: []byte("new\n"), Mode: 0o644, ModTime: newTime},
				"sub/f.txt": {Data: []byte("s\n"), Mode: 0o644, ModTime: oldTime},
			},
			args:    []string{"-uvf", "a.tar", "f.txt", "sub/f.txt"},
			wantErr: "tar: f.txt\n",
		},
		{
			name: "missing archive",
			files: fsx.MemoryFS{
				"f.txt": {Data: []byte("f\n"), Mode: 0o644},
			},
			args:     []string{"-uf", "a.tar", "f.txt"},
			wantErr:  "tar: a.tar: file does not exist\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, stderr, err := run(t, test.files, "", nil, 0, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantExit {
				t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
			}
			if got := stderr.String(); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
			if test.want == nil {
				return
			}
			data, err := fs.ReadFile(test.files, "a.tar")
			if err != nil {
				t.Fatalf("ReadFile(a.tar) error = %v", err)
			}
			compareEntries(t, test.want, parseArchive(t, string(data)))
		})
	}
}

func TestOldStyle(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		files := fsx.MemoryFS{"hello.txt": helloFile()}
		stdout, _, err := run(t, files, "", nil, 0, "cf", "-", "hello.txt")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		compareEntries(t, []entry{
			{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
		}, parseArchive(t, stdout.String()))
	})
	t.Run("create with blocking factor", func(t *testing.T) {
		files := fsx.MemoryFS{"hello.txt": helloFile()}
		stdout, stderr, err := run(t, files, "", nil, 0, "cvfb", "-", "2", "hello.txt")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got := stderr.String(); got != "tar: hello.txt\n" {
			t.Errorf("stderr = %q, want %q", got, "tar: hello.txt\n")
		}
		if got := stdout.Len(); got%1024 != 0 {
			t.Errorf("archive length = %d, not a multiple of 1024", got)
		}
	})
	t.Run("extract", func(t *testing.T) {
		files := fsx.MemoryFS{}
		_, _, err := run(t, files, fixture(t), nil, 0, "xf")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		got, err := fs.ReadFile(files, "hello.txt")
		if err != nil {
			t.Fatalf("ReadFile(hello.txt) error = %v", err)
		}
		if string(got) != "hello\n" {
			t.Errorf("ReadFile(hello.txt) = %q, want %q", got, "hello\n")
		}
	})
	t.Run("list", func(t *testing.T) {
		files := fsx.MemoryFS{"a.tar": {Data: []byte(fixture(t)), Mode: 0o644}}
		stdout, _, err := run(t, files, "", nil, 0, "tf", "a.tar")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got := stdout.String(); got != "hello.txt\nsub/\nsub/a.txt\n" {
			t.Errorf("stdout = %q, want the entry names", got)
		}
	})
}

func TestLongOptions(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		files := fsx.MemoryFS{"hello.txt": helloFile()}
		stdout, stderr, err := run(t, files, "", nil, 0, "--create", "--file=-", "--verbose", "--blocking-factor=2", "hello.txt")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got := stderr.String(); got != "tar: hello.txt\n" {
			t.Errorf("stderr = %q, want %q", got, "tar: hello.txt\n")
		}
		if got := stdout.Len(); got%1024 != 0 {
			t.Errorf("archive length = %d, not a multiple of 1024", got)
		}
	})
	t.Run("extract", func(t *testing.T) {
		files := fsx.MemoryFS{}
		_, _, err := run(t, files, fixture(t), nil, 0, "--extract", "--file=-", "--touch")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		fi, err := fs.Stat(files, "hello.txt")
		if err != nil {
			t.Fatalf("Stat(hello.txt) error = %v", err)
		}
		if fi.ModTime().Equal(oldTime) {
			t.Errorf("hello.txt modified time = %v, want the extraction time with --touch", fi.ModTime())
		}
	})
	t.Run("list", func(t *testing.T) {
		stdout, _, err := run(t, fsx.MemoryFS{}, fixture(t), nil, 0, "--list", "--file=-")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got := stdout.String(); got != "hello.txt\nsub/\nsub/a.txt\n" {
			t.Errorf("stdout = %q, want the entry names", got)
		}
	})
}
