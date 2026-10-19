package fsx

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"slices"
	"testing"
	"testing/fstest"
	"time"
)

func TestMemFSOpen(t *testing.T) {
	fsys := MemFS{
		"file.txt":      {Data: []byte("hello")},
		"dir":           {Mode: fs.ModeDir},
		"sub/inner.txt": {Data: []byte("inner")},
	}

	t.Run("reads a file", func(t *testing.T) {
		f, err := fsys.Open("file.txt")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer f.Close()

		got, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if want := "hello"; string(got) != want {
			t.Errorf("ReadAll() = %q, want %q", got, want)
		}
	})

	t.Run("stops reading at the end of the file", func(t *testing.T) {
		f, err := fsys.Open("file.txt")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer f.Close()

		if _, err := io.ReadAll(f); err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if _, err := f.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
			t.Errorf("Read() error = %v, want EOF", err)
		}
	})

	t.Run("stats a file", func(t *testing.T) {
		f, err := fsys.Open("sub/inner.txt")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got, want := fi.Name(), "inner.txt"; got != want {
			t.Errorf("Name() = %q, want %q", got, want)
		}
		if got, want := fi.Size(), int64(5); got != want {
			t.Errorf("Size() = %d, want %d", got, want)
		}
		if fi.IsDir() {
			t.Error("IsDir() = true, want false")
		}
		if _, ok := fi.Sys().(*MemFile); !ok {
			t.Errorf("Sys() = %T, want *MemFile", fi.Sys())
		}
	})

	t.Run("stats an explicit directory", func(t *testing.T) {
		assertDir(t, fsys, "dir")
	})

	t.Run("stats an implicit directory", func(t *testing.T) {
		assertDir(t, fsys, "sub")
	})

	t.Run("stats the root", func(t *testing.T) {
		assertDir(t, fsys, ".")
	})

	t.Run("reading a directory is invalid", func(t *testing.T) {
		f, err := fsys.Open("dir")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer f.Close()

		if _, err := f.Read(make([]byte, 1)); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Read() error = %v, want fs.ErrInvalid", err)
		}
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			path    string
			wantErr error
		}{
			{"missing file", "missing.txt", fs.ErrNotExist},
			{"missing file in a directory", "sub/missing.txt", fs.ErrNotExist},
			{"absolute path", "/file.txt", fs.ErrInvalid},
			{"empty path", "", fs.ErrInvalid},
			{"dot dot", "a/../file.txt", fs.ErrInvalid},
			{"trailing slash", "sub/", fs.ErrInvalid},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if _, err := fsys.Open(test.path); !errors.Is(err, test.wantErr) {
					t.Errorf("Open(%q) error = %v, want %v", test.path, err, test.wantErr)
				}
			})
		}
	})

	t.Run("nil filesystem", func(t *testing.T) {
		var nilFS MemFS
		if _, err := nilFS.Open("file.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Open() error = %v, want fs.ErrNotExist", err)
		}
	})
}

func assertDir(t *testing.T, fsys MemFS, name string) {
	t.Helper()

	f, err := fsys.Open(name)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", name, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("IsDir() = false, want true")
	}
	if !fi.Mode().IsDir() {
		t.Errorf("Mode() = %v, want a directory mode", fi.Mode())
	}
}

func TestMemFSWrite(t *testing.T) {
	t.Run("creates a file with the given permissions", func(t *testing.T) {
		fsys := MemFS{}

		f, err := fsys.OpenFile("new.txt", os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if n, err := f.Write([]byte("hello")); err != nil || n != 5 {
			t.Errorf("Write() = (%d, %v), want (5, nil)", n, err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		fi, err := fs.Stat(fsys, "new.txt")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got, want := fi.Mode(), fs.FileMode(0o644); got != want {
			t.Errorf("Mode() = %v, want %v", got, want)
		}
	})

	t.Run("keeps the contents when the file already exists", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		f, err := fsys.OpenFile("file.txt", os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		assertFile(t, fsys, "file.txt", "hello")
	})

	t.Run("truncates with O_TRUNC", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		f, err := fsys.OpenFile("file.txt", os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		assertFile(t, fsys, "file.txt", "")
	})

	t.Run("appends with O_APPEND", func(t *testing.T) {
		fsys := MemFS{"log.txt": {Data: []byte("one")}}

		f, err := fsys.OpenFile("log.txt", os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if _, err := f.Write([]byte("two")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		assertFile(t, fsys, "log.txt", "onetwo")
	})

	t.Run("reads what was written", func(t *testing.T) {
		fsys := MemFS{}

		f, err := fsys.OpenFile("file.txt", os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if _, err := f.Write([]byte("hello")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		assertFile(t, fsys, "file.txt", "hello")
	})

	t.Run("updates the modification time on write", func(t *testing.T) {
		old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		fsys := MemFS{"file.txt": {Data: []byte("hello"), ModTime: old}}

		f, err := fsys.OpenFile("file.txt", os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if _, err := f.Write([]byte(" world")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		fi, err := fs.Stat(fsys, "file.txt")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if !fi.ModTime().After(old) {
			t.Errorf("ModTime() = %v, want after %v", fi.ModTime(), old)
		}
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			fsys    MemFS
			path    string
			flag    int
			wantErr error
		}{
			{
				name:    "missing without O_CREATE",
				fsys:    MemFS{},
				path:    "missing.txt",
				flag:    os.O_WRONLY,
				wantErr: fs.ErrNotExist,
			},
			{
				name:    "missing parent directory",
				fsys:    MemFS{},
				path:    "sub/file.txt",
				flag:    os.O_CREATE | os.O_WRONLY,
				wantErr: fs.ErrNotExist,
			},
			{
				name:    "O_EXCL on an existing file",
				fsys:    MemFS{"file.txt": {Data: []byte("hello")}},
				path:    "file.txt",
				flag:    os.O_CREATE | os.O_EXCL | os.O_WRONLY,
				wantErr: fs.ErrExist,
			},
			{
				name:    "creating a file where a directory is",
				fsys:    MemFS{"sub/inner.txt": {Data: []byte("inner")}},
				path:    "sub",
				flag:    os.O_CREATE | os.O_WRONLY,
				wantErr: fs.ErrInvalid,
			},
			{
				name:    "writing to a directory",
				fsys:    MemFS{"dir": {Mode: fs.ModeDir}},
				path:    "dir",
				flag:    os.O_WRONLY,
				wantErr: fs.ErrInvalid,
			},
			{
				name:    "invalid path",
				fsys:    MemFS{},
				path:    "../file.txt",
				flag:    os.O_CREATE | os.O_WRONLY,
				wantErr: fs.ErrInvalid,
			},
			{
				name:    "nil filesystem",
				fsys:    nil,
				path:    "file.txt",
				flag:    os.O_CREATE | os.O_WRONLY,
				wantErr: fs.ErrInvalid,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				_, err := test.fsys.OpenFile(test.path, test.flag, 0o644)
				if !errors.Is(err, test.wantErr) {
					t.Errorf("OpenFile() error = %v, want %v", err, test.wantErr)
				}
			})
		}
	})

	t.Run("creates a file inside an existing directory", func(t *testing.T) {
		fsys := MemFS{"sub/inner.txt": {Data: []byte("inner")}}

		f, err := fsys.OpenFile("sub/new.txt", os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if _, err := f.Write([]byte("new")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		assertFile(t, fsys, "sub/new.txt", "new")
	})
}

func TestMemFSFileHandleErrors(t *testing.T) {
	t.Run("read on a write-only handle", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		f, err := fsys.OpenFile("file.txt", os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		defer f.Close()

		if _, err := f.Read(make([]byte, 1)); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Read() error = %v, want fs.ErrInvalid", err)
		}
	})

	t.Run("write on a read-only handle", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		f, err := fsys.OpenFile("file.txt", os.O_RDONLY, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		defer f.Close()

		if _, err := f.Write([]byte("x")); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Write() error = %v, want fs.ErrInvalid", err)
		}
	})

	t.Run("operations after close", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		f, err := fsys.OpenFile("file.txt", os.O_RDWR, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		if err := f.Close(); !errors.Is(err, fs.ErrClosed) {
			t.Errorf("Close() error = %v, want fs.ErrClosed", err)
		}
		if _, err := f.Read(make([]byte, 1)); !errors.Is(err, fs.ErrClosed) {
			t.Errorf("Read() error = %v, want fs.ErrClosed", err)
		}
		if _, err := f.Write([]byte("x")); !errors.Is(err, fs.ErrClosed) {
			t.Errorf("Write() error = %v, want fs.ErrClosed", err)
		}
		if _, err := f.Stat(); !errors.Is(err, fs.ErrClosed) {
			t.Errorf("Stat() error = %v, want fs.ErrClosed", err)
		}
	})
}

func TestMemFSChtimes(t *testing.T) {
	atime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	mtime := time.Date(2021, 6, 7, 8, 9, 10, 0, time.UTC)

	t.Run("sets both times", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		if err := fsys.Chtimes("file.txt", atime, mtime); err != nil {
			t.Fatalf("Chtimes() error = %v", err)
		}

		fi, err := fs.Stat(fsys, "file.txt")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if !fi.ModTime().Equal(mtime) {
			t.Errorf("ModTime() = %v, want %v", fi.ModTime(), mtime)
		}
		if got, ok := Atime(fi); !ok || !got.Equal(atime) {
			t.Errorf("Atime() = (%v, %v), want (%v, true)", got, ok, atime)
		}
	})

	t.Run("access time defaults to the modification time", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello"), ModTime: mtime}}

		fi, err := fs.Stat(fsys, "file.txt")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got, ok := Atime(fi); !ok || !got.Equal(mtime) {
			t.Errorf("Atime() = (%v, %v), want (%v, true)", got, ok, mtime)
		}
	})

	t.Run("ignores implicit directories", func(t *testing.T) {
		fsys := MemFS{"sub/inner.txt": {Data: []byte("inner")}}

		if err := fsys.Chtimes("sub", atime, mtime); err != nil {
			t.Errorf("Chtimes() error = %v, want nil", err)
		}
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			fsys    MemFS
			path    string
			wantErr error
		}{
			{"missing file", MemFS{}, "missing.txt", fs.ErrNotExist},
			{"invalid path", MemFS{}, "../file.txt", fs.ErrInvalid},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if err := test.fsys.Chtimes(test.path, atime, mtime); !errors.Is(err, test.wantErr) {
					t.Errorf("Chtimes() error = %v, want %v", err, test.wantErr)
				}
			})
		}
	})
}

func TestMemFSReadDir(t *testing.T) {
	fsys := MemFS{
		"file.txt":      {Data: []byte("hello")},
		"dir":           {Mode: fs.ModeDir | 0o700},
		"sub/inner.txt": {Data: []byte("inner")},
		"other/deep/x":  {Data: []byte("x")},
	}

	t.Run("lists the children of the root sorted by name", func(t *testing.T) {
		if got, want := dirNames(t, fsys, "."),
			[]string{"dir", "file.txt", "other", "sub"}; !slices.Equal(got, want) {
			t.Errorf("ReadDir(%q) names = %v, want %v", ".", got, want)
		}
	})

	t.Run("lists the children of a directory", func(t *testing.T) {
		if got, want := dirNames(t, fsys, "sub"), []string{"inner.txt"}; !slices.Equal(got, want) {
			t.Errorf("ReadDir(%q) names = %v, want %v", "sub", got, want)
		}
	})

	t.Run("stats the entries", func(t *testing.T) {
		entries, err := fsys.ReadDir("other")
		if err != nil {
			t.Fatalf("ReadDir(%q) error = %v", "other", err)
		}
		fi, err := entries[0].Info()
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}
		if got, want := fi.Mode(), fs.ModeDir|0o755; got != want {
			t.Errorf("Mode() = %v, want %v", got, want)
		}
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			path    string
			wantErr error
		}{
			{"missing file", "missing.txt", fs.ErrNotExist},
			{"file is not a directory", "file.txt", errNotDir},
			{"absolute path", "/dir", fs.ErrInvalid},
			{"empty path", "", fs.ErrInvalid},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if _, err := fsys.ReadDir(test.path); !errors.Is(err, test.wantErr) {
					t.Errorf("ReadDir(%q) error = %v, want %v", test.path, err, test.wantErr)
				}
			})
		}
	})

	t.Run("walks the whole tree with fs.WalkDir", func(t *testing.T) {
		var got []string
		err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			got = append(got, path)
			return nil
		})
		if err != nil {
			t.Fatalf("WalkDir() error = %v", err)
		}
		want := []string{
			".", "dir", "file.txt", "other", "other/deep", "other/deep/x", "sub", "sub/inner.txt",
		}
		if !slices.Equal(got, want) {
			t.Errorf("WalkDir() paths = %v, want %v", got, want)
		}
	})
}

func dirNames(t *testing.T, fsys MemFS, name string) []string {
	t.Helper()

	entries, err := fsys.ReadDir(name)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", name, err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}

func TestMemFSChmod(t *testing.T) {
	t.Run("changes the permission bits", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644}}

		if err := fsys.Chmod("file.txt", 0o755); err != nil {
			t.Fatalf("Chmod() error = %v", err)
		}
		fi, err := fs.Stat(fsys, "file.txt")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got, want := fi.Mode(), fs.FileMode(0o755); got != want {
			t.Errorf("Mode() = %v, want %v", got, want)
		}
	})

	t.Run("clears the special bits", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o755 | fs.ModeSetuid | fs.ModeSticky}}

		if err := fsys.Chmod("file.txt", 0o755); err != nil {
			t.Fatalf("Chmod() error = %v", err)
		}
		fi, err := fs.Stat(fsys, "file.txt")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got, want := fi.Mode(), fs.FileMode(0o755); got != want {
			t.Errorf("Mode() = %v, want %v", got, want)
		}
	})

	t.Run("sets the special bits", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o755}}

		if err := fsys.Chmod("file.txt", 0o755|fs.ModeSetgid); err != nil {
			t.Fatalf("Chmod() error = %v", err)
		}
		fi, err := fs.Stat(fsys, "file.txt")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got, want := fi.Mode(), fs.FileMode(0o755)|fs.ModeSetgid; got != want {
			t.Errorf("Mode() = %v, want %v", got, want)
		}
	})

	t.Run("keeps the file type", func(t *testing.T) {
		fsys := MemFS{"dir": {Mode: fs.ModeDir | 0o755}}

		if err := fsys.Chmod("dir", 0o700); err != nil {
			t.Fatalf("Chmod() error = %v", err)
		}
		fi, err := fs.Stat(fsys, "dir")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got, want := fi.Mode(), fs.ModeDir|0o700; got != want {
			t.Errorf("Mode() = %v, want %v", got, want)
		}
	})

	t.Run("makes an implicit directory explicit", func(t *testing.T) {
		fsys := MemFS{"dir/file.txt": {Data: []byte("hello")}}

		if err := fsys.Chmod("dir", 0o700); err != nil {
			t.Fatalf("Chmod() error = %v", err)
		}
		fi, err := fs.Stat(fsys, "dir")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got, want := fi.Mode(), fs.ModeDir|0o700; got != want {
			t.Errorf("Mode() = %v, want %v", got, want)
		}
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			fsys    MemFS
			path    string
			wantErr error
		}{
			{"missing file", MemFS{}, "missing.txt", fs.ErrNotExist},
			{"missing file in a directory", MemFS{}, "dir/missing.txt", fs.ErrNotExist},
			{"absolute path", MemFS{}, "/file.txt", fs.ErrInvalid},
			{"empty path", MemFS{}, "", fs.ErrInvalid},
			{"nil filesystem", nil, "file.txt", fs.ErrInvalid},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if err := test.fsys.Chmod(test.path, 0o755); !errors.Is(err, test.wantErr) {
					t.Errorf("Chmod(%q) error = %v, want %v", test.path, err, test.wantErr)
				}
			})
		}
	})
}

func TestMemFSChown(t *testing.T) {
	t.Run("changes the uid and gid", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello"), Uid: 1, Gid: 2}}

		if err := fsys.Chown("file.txt", 1000, 2000); err != nil {
			t.Fatalf("Chown() error = %v", err)
		}
		fi, err := fs.Stat(fsys, "file.txt")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		uid, gid, ok := Owner(fi)
		if !ok {
			t.Fatal("Owner() ok = false, want true")
		}
		if uid != 1000 || gid != 2000 {
			t.Errorf("Owner() = %d, %d, want 1000, 2000", uid, gid)
		}
	})

	t.Run("keeps an ID given as -1", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello"), Uid: 7, Gid: 8}}

		if err := fsys.Chown("file.txt", -1, 9); err != nil {
			t.Fatalf("Chown() error = %v", err)
		}
		if file := fsys["file.txt"]; file.Uid != 7 || file.Gid != 9 {
			t.Errorf("Uid, Gid = %d, %d, want 7, 9", file.Uid, file.Gid)
		}
	})

	t.Run("ignores implicit directories", func(t *testing.T) {
		fsys := MemFS{"dir/file.txt": {Data: []byte("hello")}}

		if err := fsys.Chown("dir", 1000, 1000); err != nil {
			t.Errorf("Chown() error = %v, want nil", err)
		}
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			fsys    MemFS
			path    string
			wantErr error
		}{
			{"missing file", MemFS{}, "missing.txt", fs.ErrNotExist},
			{"missing file in a directory", MemFS{}, "dir/missing.txt", fs.ErrNotExist},
			{"absolute path", MemFS{}, "/file.txt", fs.ErrInvalid},
			{"empty path", MemFS{}, "", fs.ErrInvalid},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if err := test.fsys.Chown(test.path, 0, 0); !errors.Is(err, test.wantErr) {
					t.Errorf("Chown(%q) error = %v, want %v", test.path, err, test.wantErr)
				}
			})
		}
	})
}

func TestOwner(t *testing.T) {
	t.Run("reports nothing without a file info", func(t *testing.T) {
		if _, _, ok := Owner(nil); ok {
			t.Error("Owner(nil) ok = true, want false")
		}
	})

	t.Run("reports nothing for a foreign file info", func(t *testing.T) {
		fi, err := fs.Stat(fstest.MapFS{"file.txt": {Data: []byte("hello")}}, "file.txt")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if _, _, ok := Owner(fi); ok {
			t.Error("Owner() ok = true, want false")
		}
	})
}

func TestMemFSMkdir(t *testing.T) {
	t.Run("creates a directory with the given permissions", func(t *testing.T) {
		fsys := MemFS{}

		if err := fsys.Mkdir("dir", 0o755); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}

		fi, err := fs.Stat(fsys, "dir")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got, want := fi.Mode(), fs.ModeDir|0o755; got != want {
			t.Errorf("Mode() = %v, want %v", got, want)
		}
	})

	t.Run("drops the special bits from the permissions", func(t *testing.T) {
		fsys := MemFS{}

		if err := fsys.Mkdir("dir", 0o755|fs.ModeSetgid); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}

		fi, err := fs.Stat(fsys, "dir")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got, want := fi.Mode(), fs.ModeDir|0o755; got != want {
			t.Errorf("Mode() = %v, want %v", got, want)
		}
	})

	t.Run("creates a directory inside an implicit one", func(t *testing.T) {
		fsys := MemFS{"sub/inner.txt": {Data: []byte("inner")}}

		if err := fsys.Mkdir("sub/dir", 0o700); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		assertDir(t, fsys, "sub/dir")
	})

	t.Run("creates a directory inside an explicit one", func(t *testing.T) {
		fsys := MemFS{"dir": {Mode: fs.ModeDir | 0o755}}

		if err := fsys.Mkdir("dir/new", 0o700); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		assertDir(t, fsys, "dir/new")
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			fsys    MemFS
			path    string
			wantErr error
		}{
			{"existing file", MemFS{"file.txt": {Data: []byte("hello")}}, "file.txt", fs.ErrExist},
			{"existing directory", MemFS{"dir": {Mode: fs.ModeDir}}, "dir", fs.ErrExist},
			{"existing implicit directory", MemFS{"sub/inner.txt": {Data: []byte("inner")}}, "sub", fs.ErrExist},
			{"root", MemFS{}, ".", fs.ErrExist},
			{"missing parent", MemFS{}, "sub/dir", fs.ErrNotExist},
			{"parent is a file", MemFS{"file.txt": {Data: []byte("hello")}}, "file.txt/sub", errNotDir},
			{"invalid path", MemFS{}, "../dir", fs.ErrInvalid},
			{"absolute path", MemFS{}, "/dir", fs.ErrInvalid},
			{"empty path", MemFS{}, "", fs.ErrInvalid},
			{"nil filesystem", nil, "dir", fs.ErrInvalid},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if err := test.fsys.Mkdir(test.path, 0o755); !errors.Is(err, test.wantErr) {
					t.Errorf("Mkdir(%q) error = %v, want %v", test.path, err, test.wantErr)
				}
			})
		}
	})
}

func TestMemFSRemove(t *testing.T) {
	t.Run("removes a file", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		if err := fsys.Remove("file.txt"); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}
		if _, err := fs.Stat(fsys, "file.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat() error = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("removes an empty directory", func(t *testing.T) {
		fsys := MemFS{"dir": {Mode: fs.ModeDir}}

		if err := fsys.Remove("dir"); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}
		if _, err := fs.Stat(fsys, "dir"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat() error = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("removes a symbolic link instead of its target", func(t *testing.T) {
		fsys := MemFS{
			"file.txt": {Data: []byte("hello")},
			"sym.txt":  {Data: []byte("file.txt"), Mode: fs.ModeSymlink},
		}

		if err := fsys.Remove("sym.txt"); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}
		if _, err := fsys.Lstat("sym.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Lstat() error = %v, want fs.ErrNotExist", err)
		}
		assertFile(t, fsys, "file.txt", "hello")
	})

	t.Run("removes through a symbolic link to a directory", func(t *testing.T) {
		fsys := MemFS{
			"dir/file.txt": {Data: []byte("hello")},
			"link":         {Data: []byte("dir"), Mode: fs.ModeSymlink},
		}

		if err := fsys.Remove("link/file.txt"); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}
		if _, err := fs.Stat(fsys, "dir/file.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat() error = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			fsys    MemFS
			path    string
			wantErr error
		}{
			{"missing file", MemFS{}, "missing.txt", fs.ErrNotExist},
			{"non-empty implicit directory", MemFS{"dir/inner.txt": {Data: []byte("inner")}}, "dir", ErrNotEmpty},
			{
				"non-empty explicit directory",
				MemFS{"dir": {Mode: fs.ModeDir}, "dir/inner.txt": {Data: []byte("inner")}},
				"dir",
				ErrNotEmpty,
			},
			{"root", MemFS{}, ".", fs.ErrInvalid},
			{"invalid path", MemFS{}, "../dir", fs.ErrInvalid},
			{"nil filesystem", nil, "file.txt", fs.ErrNotExist},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if err := test.fsys.Remove(test.path); !errors.Is(err, test.wantErr) {
					t.Errorf("Remove(%q) error = %v, want %v", test.path, err, test.wantErr)
				}
			})
		}
	})
}

func TestMemFSRename(t *testing.T) {
	oldTime := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)

	t.Run("renames a file", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o604, ModTime: oldTime}}

		if err := fsys.Rename("file.txt", "renamed.txt"); err != nil {
			t.Fatalf("Rename() error = %v", err)
		}
		assertFile(t, fsys, "renamed.txt", "hello")
		if _, err := fs.Stat(fsys, "file.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat() of the old name error = %v, want fs.ErrNotExist", err)
		}
		fi, err := fs.Stat(fsys, "renamed.txt")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got := fi.Mode(); got != 0o604 {
			t.Errorf("Mode() = %v, want 0o604", got)
		}
		if got := fi.ModTime(); !got.Equal(oldTime) {
			t.Errorf("ModTime() = %v, want %v", got, oldTime)
		}
	})

	t.Run("moves a file between directories", func(t *testing.T) {
		fsys := MemFS{"dir/file.txt": {Data: []byte("hello")}, "other/keep.txt": {}}

		if err := fsys.Rename("dir/file.txt", "other/file.txt"); err != nil {
			t.Fatalf("Rename() error = %v", err)
		}
		assertFile(t, fsys, "other/file.txt", "hello")
		assertFile(t, fsys, "other/keep.txt", "")
	})

	t.Run("renames a directory with its contents", func(t *testing.T) {
		fsys := MemFS{
			"dir":          {Mode: fs.ModeDir | 0o755},
			"dir/file.txt": {Data: []byte("hello")},
			"dir/sub":      {Mode: fs.ModeDir | 0o700},
			"dir/sub/deep": {Data: []byte("deep")},
		}

		if err := fsys.Rename("dir", "renamed"); err != nil {
			t.Fatalf("Rename() error = %v", err)
		}
		assertFile(t, fsys, "renamed/file.txt", "hello")
		assertFile(t, fsys, "renamed/sub/deep", "deep")
		for _, name := range []string{"dir", "dir/file.txt", "dir/sub", "dir/sub/deep"} {
			if _, err := fs.Stat(fsys, name); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("Stat(%q) error = %v, want fs.ErrNotExist", name, err)
			}
		}
	})

	t.Run("renames an implicit directory", func(t *testing.T) {
		fsys := MemFS{"dir/file.txt": {Data: []byte("hello")}}

		if err := fsys.Rename("dir", "renamed"); err != nil {
			t.Fatalf("Rename() error = %v", err)
		}
		assertFile(t, fsys, "renamed/file.txt", "hello")
	})

	t.Run("replaces an existing file", func(t *testing.T) {
		fsys := MemFS{"old.txt": {Data: []byte("new")}, "new.txt": {Data: []byte("old")}}

		if err := fsys.Rename("old.txt", "new.txt"); err != nil {
			t.Fatalf("Rename() error = %v", err)
		}
		assertFile(t, fsys, "new.txt", "new")
		if _, err := fs.Stat(fsys, "old.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat() of the old name error = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("replaces an empty directory", func(t *testing.T) {
		fsys := MemFS{"dir/file.txt": {Data: []byte("hello")}, "empty": {Mode: fs.ModeDir}}

		if err := fsys.Rename("dir", "empty"); err != nil {
			t.Fatalf("Rename() error = %v", err)
		}
		assertFile(t, fsys, "empty/file.txt", "hello")
	})

	t.Run("the same name is a no-op", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		if err := fsys.Rename("file.txt", "file.txt"); err != nil {
			t.Fatalf("Rename() error = %v", err)
		}
		assertFile(t, fsys, "file.txt", "hello")
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			fsys    MemFS
			oldName string
			newName string
			wantErr error
		}{
			{"missing file", MemFS{}, "missing.txt", "new.txt", fs.ErrNotExist},
			{"nil filesystem", nil, "file.txt", "new.txt", fs.ErrNotExist},
			{"root", MemFS{"file.txt": {}}, ".", "new.txt", fs.ErrInvalid},
			{"new name is the root", MemFS{"file.txt": {}}, "file.txt", ".", fs.ErrInvalid},
			{"invalid old path", MemFS{}, "../old", "new.txt", fs.ErrInvalid},
			{"invalid new path", MemFS{"file.txt": {}}, "file.txt", "../new", fs.ErrInvalid},
			{
				"missing parent of the new name",
				MemFS{"file.txt": {}},
				"file.txt",
				"missing/new.txt",
				fs.ErrNotExist,
			},
			{
				"parent of the new name is a file",
				MemFS{"file.txt": {}, "blocker": {Data: []byte("x")}},
				"file.txt",
				"blocker/new.txt",
				errNotDir,
			},
			{
				"file onto a directory",
				MemFS{"file.txt": {Data: []byte("x")}, "dir/keep.txt": {}},
				"file.txt",
				"dir",
				errIsDir,
			},
			{
				"directory onto a file",
				MemFS{"dir/file.txt": {}, "file.txt": {Data: []byte("x")}},
				"dir",
				"file.txt",
				errNotDir,
			},
			{
				"directory onto a non-empty directory",
				MemFS{"dir1/file.txt": {}, "dir2/keep.txt": {}},
				"dir1",
				"dir2",
				ErrNotEmpty,
			},
			{
				"directory into itself",
				MemFS{"dir/sub": {Mode: fs.ModeDir}},
				"dir",
				"dir/sub",
				errInSelf,
			},
			{
				"directory into a deeper subdirectory of itself",
				MemFS{"dir/sub": {Mode: fs.ModeDir}},
				"dir",
				"dir/sub/name",
				errInSelf,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				err := test.fsys.Rename(test.oldName, test.newName)
				if !errors.Is(err, test.wantErr) {
					t.Errorf("Rename(%q, %q) error = %v, want %v", test.oldName, test.newName, err, test.wantErr)
				}
			})
		}
	})
}

func TestMemFSSeek(t *testing.T) {
	openSeekable := func(t *testing.T, data string) SeekFile {
		t.Helper()

		f, err := MemFS{"file.txt": {Data: []byte(data)}}.Open("file.txt")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		t.Cleanup(func() { f.Close() })

		s, ok := f.(SeekFile)
		if !ok {
			t.Fatalf("Open() = %T, want a SeekFile", f)
		}
		return s
	}

	t.Run("reads from the new offset", func(t *testing.T) {
		f := openSeekable(t, "hello")

		if got, err := f.Seek(2, io.SeekStart); err != nil || got != 2 {
			t.Fatalf("Seek() = (%d, %v), want (2, nil)", got, err)
		}
		got, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if want := "llo"; string(got) != want {
			t.Errorf("ReadAll() = %q, want %q", got, want)
		}
	})

	t.Run("relative seeks", func(t *testing.T) {
		f := openSeekable(t, "hello")

		tests := []struct {
			name   string
			offset int64
			whence int
			want   int64
		}{
			{"from the start", 1, io.SeekStart, 1},
			{"forward from the current offset", 3, io.SeekCurrent, 4},
			{"back from the current offset", -2, io.SeekCurrent, 2},
			{"before the end", -3, io.SeekEnd, 2},
			{"at the end", 0, io.SeekEnd, 5},
			{"past the end", 2, io.SeekEnd, 7},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := f.Seek(test.offset, test.whence)
				if err != nil || got != test.want {
					t.Fatalf("Seek() = (%d, %v), want (%d, nil)", got, err, test.want)
				}
			})
		}
	})

	t.Run("seeking past the end reads EOF", func(t *testing.T) {
		f := openSeekable(t, "hello")

		if _, err := f.Seek(1, io.SeekEnd); err != nil {
			t.Fatalf("Seek() error = %v", err)
		}
		got, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("ReadAll() = %q, want no bytes", got)
		}
	})

	t.Run("errors", func(t *testing.T) {
		t.Run("before the start", func(t *testing.T) {
			f := openSeekable(t, "hello")
			if _, err := f.Seek(-1, io.SeekStart); !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("Seek() error = %v, want fs.ErrInvalid", err)
			}
		})

		t.Run("invalid whence", func(t *testing.T) {
			f := openSeekable(t, "hello")
			if _, err := f.Seek(0, 3); !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("Seek() error = %v, want fs.ErrInvalid", err)
			}
		})

		t.Run("closed file", func(t *testing.T) {
			f := openSeekable(t, "hello")
			if err := f.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if _, err := f.Seek(0, io.SeekStart); !errors.Is(err, fs.ErrClosed) {
				t.Errorf("Seek() error = %v, want fs.ErrClosed", err)
			}
		})
	})

	t.Run("writes from the new offset", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}
		f, err := fsys.OpenFile("file.txt", os.O_RDWR, 0)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		defer f.Close()

		s, ok := f.(SeekFile)
		if !ok {
			t.Fatalf("OpenFile() = %T, want a SeekFile", f)
		}
		if _, err := s.Seek(1, io.SeekStart); err != nil {
			t.Fatalf("Seek() error = %v", err)
		}
		if _, err := f.Write([]byte("EY")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		assertFile(t, fsys, "file.txt", "hEYlo")
	})
}

func TestAtimeNilFileInfo(t *testing.T) {
	if _, ok := Atime(nil); ok {
		t.Error("Atime(nil) = true, want false")
	}
}

func TestMemFSLink(t *testing.T) {
	t.Run("creates a second name for the same file", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		if err := fsys.Link("file.txt", "link.txt"); err != nil {
			t.Fatalf("Link() error = %v", err)
		}
		assertFile(t, fsys, "link.txt", "hello")

		f, err := fsys.OpenFile("link.txt", os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if _, err := f.Write([]byte("bye")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		assertFile(t, fsys, "file.txt", "bye")
	})

	t.Run("removing one name keeps the other", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		if err := fsys.Link("file.txt", "link.txt"); err != nil {
			t.Fatalf("Link() error = %v", err)
		}
		if err := fsys.Remove("file.txt"); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}
		assertFile(t, fsys, "link.txt", "hello")
	})

	t.Run("links a symbolic link as-is", func(t *testing.T) {
		fsys := MemFS{
			"file.txt": {Data: []byte("hello")},
			"sym.txt":  {Data: []byte("file.txt"), Mode: fs.ModeSymlink},
		}

		if err := fsys.Link("sym.txt", "hard.txt"); err != nil {
			t.Fatalf("Link() error = %v", err)
		}
		fi, err := fsys.Lstat("hard.txt")
		if err != nil {
			t.Fatalf("Lstat() error = %v", err)
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("Mode() = %v, want a symbolic link", fi.Mode())
		}
		assertFile(t, fsys, "hard.txt", "hello")
	})

	t.Run("links through a symbolic link to a directory", func(t *testing.T) {
		fsys := MemFS{
			"dir/file.txt": {Data: []byte("hello")},
			"link":         {Data: []byte("dir"), Mode: fs.ModeSymlink},
		}

		if err := fsys.Link("dir/file.txt", "link/other.txt"); err != nil {
			t.Fatalf("Link() error = %v", err)
		}
		assertFile(t, fsys, "link/other.txt", "hello")
		assertFile(t, fsys, "dir/other.txt", "hello")
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			fsys    MemFS
			oldName string
			newName string
			wantErr error
		}{
			{"missing source", MemFS{}, "missing.txt", "link.txt", fs.ErrNotExist},
			{"nil filesystem", nil, "file.txt", "link.txt", fs.ErrNotExist},
			{"source is a directory", MemFS{"dir/file.txt": {}}, "dir", "link.txt", fs.ErrPermission},
			{"existing destination", MemFS{"file.txt": {}, "other.txt": {}}, "file.txt", "other.txt", fs.ErrExist},
			{"destination is a directory", MemFS{"file.txt": {}, "dir/keep.txt": {}}, "file.txt", "dir", fs.ErrExist},
			{"missing parent of the destination", MemFS{"file.txt": {}}, "file.txt", "missing/link.txt", fs.ErrNotExist},
			{"parent of the destination is a file", MemFS{"file.txt": {}, "blocker": {}}, "file.txt", "blocker/link.txt", errNotDir},
			{"source is the root", MemFS{"file.txt": {}}, ".", "link.txt", fs.ErrInvalid},
			{"destination is the root", MemFS{"file.txt": {}}, "file.txt", ".", fs.ErrInvalid},
			{"invalid source path", MemFS{}, "../file.txt", "link.txt", fs.ErrInvalid},
			{"invalid destination path", MemFS{"file.txt": {}}, "file.txt", "../link.txt", fs.ErrInvalid},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				err := test.fsys.Link(test.oldName, test.newName)
				if !errors.Is(err, test.wantErr) {
					t.Errorf("Link(%q, %q) error = %v, want %v", test.oldName, test.newName, err, test.wantErr)
				}
			})
		}
	})
}

func TestMemFSSymlink(t *testing.T) {
	t.Run("creates a link to an existing file", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		if err := fsys.Symlink("file.txt", "sym.txt"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		fi, err := fsys.Lstat("sym.txt")
		if err != nil {
			t.Fatalf("Lstat() error = %v", err)
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("Mode() = %v, want a symbolic link", fi.Mode())
		}
		got, err := fsys.ReadLink("sym.txt")
		if err != nil || got != "file.txt" {
			t.Errorf("ReadLink() = %q, %v, want %q, nil", got, err, "file.txt")
		}
		assertFile(t, fsys, "sym.txt", "hello")
	})

	t.Run("creates a dangling link", func(t *testing.T) {
		fsys := MemFS{}

		if err := fsys.Symlink("missing.txt", "sym.txt"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		if _, err := fs.Stat(fsys, "sym.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat() error = %v, want fs.ErrNotExist", err)
		}
		if _, err := fsys.Lstat("sym.txt"); err != nil {
			t.Errorf("Lstat() error = %v, want it to exist", err)
		}
	})

	t.Run("follows a chain of links", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		if err := fsys.Symlink("file.txt", "a.txt"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		if err := fsys.Symlink("a.txt", "b.txt"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		assertFile(t, fsys, "b.txt", "hello")
	})

	t.Run("follows a link in the middle of a path", func(t *testing.T) {
		fsys := MemFS{"dir/file.txt": {Data: []byte("hello")}}

		if err := fsys.Symlink("dir", "link"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		assertFile(t, fsys, "link/file.txt", "hello")

		fi, err := fs.Stat(fsys, "link")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if !fi.IsDir() {
			t.Error("Stat() of the link IsDir() = false, want true")
		}
		if _, err := fs.ReadDir(fsys, "link"); err != nil {
			t.Errorf("ReadDir() error = %v", err)
		}
	})

	t.Run("resolves a relative target from the link directory", func(t *testing.T) {
		fsys := MemFS{"dir/file.txt": {Data: []byte("hello")}}

		if err := fsys.Symlink("file.txt", "dir/sym.txt"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		assertFile(t, fsys, "dir/sym.txt", "hello")
	})

	t.Run("creates a link through a symbolic link to a directory", func(t *testing.T) {
		fsys := MemFS{
			"dir/keep.txt": {Data: []byte("keep")},
			"link":         {Data: []byte("dir"), Mode: fs.ModeSymlink},
		}

		if err := fsys.Symlink("file.txt", "link/sym.txt"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		got, err := fsys.ReadLink("link/sym.txt")
		if err != nil || got != "file.txt" {
			t.Errorf("ReadLink() = %q, %v, want %q, nil", got, err, "file.txt")
		}
		got, err = fsys.ReadLink("dir/sym.txt")
		if err != nil || got != "file.txt" {
			t.Errorf("ReadLink() = %q, %v, want %q, nil", got, err, "file.txt")
		}
	})

	t.Run("writes through a link to its target", func(t *testing.T) {
		fsys := MemFS{"file.txt": {Data: []byte("hello")}}

		if err := fsys.Symlink("file.txt", "sym.txt"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		f, err := fsys.OpenFile("sym.txt", os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if _, err := f.Write([]byte("bye")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		assertFile(t, fsys, "file.txt", "bye")
	})

	t.Run("creates the target of a dangling link", func(t *testing.T) {
		fsys := MemFS{}

		if err := fsys.Symlink("target.txt", "sym.txt"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		f, err := fsys.OpenFile("sym.txt", os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if _, err := f.Write([]byte("hello")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		assertFile(t, fsys, "target.txt", "hello")
	})

	t.Run("reports a loop", func(t *testing.T) {
		fsys := MemFS{}

		if err := fsys.Symlink("b.txt", "a.txt"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		if err := fsys.Symlink("a.txt", "b.txt"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		if _, err := fs.Stat(fsys, "a.txt"); !errors.Is(err, errLoop) {
			t.Errorf("Stat() error = %v, want %v", err, errLoop)
		}
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			fsys    MemFS
			newName string
			wantErr error
		}{
			{"nil filesystem", nil, "sym.txt", fs.ErrInvalid},
			{"existing destination", MemFS{"file.txt": {}}, "file.txt", fs.ErrExist},
			{"destination is a directory", MemFS{"dir/keep.txt": {}}, "dir", fs.ErrExist},
			{"missing parent of the destination", MemFS{}, "missing/sym.txt", fs.ErrNotExist},
			{"parent of the destination is a file", MemFS{"blocker": {}}, "blocker/sym.txt", errNotDir},
			{"destination is the root", MemFS{}, ".", fs.ErrInvalid},
			{"invalid destination path", MemFS{}, "../sym.txt", fs.ErrInvalid},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				err := test.fsys.Symlink("file.txt", test.newName)
				if !errors.Is(err, test.wantErr) {
					t.Errorf("Symlink(%q) error = %v, want %v", test.newName, err, test.wantErr)
				}
			})
		}
	})
}

func TestMemFSReadLink(t *testing.T) {
	tests := []struct {
		name    string
		fsys    MemFS
		path    string
		want    string
		wantErr error
	}{
		{"link", MemFS{"sym.txt": {Data: []byte("file.txt"), Mode: fs.ModeSymlink}}, "sym.txt", "file.txt", nil},
		{"not a link", MemFS{"file.txt": {}}, "file.txt", "", fs.ErrInvalid},
		{"directory", MemFS{"dir/file.txt": {}}, "dir", "", fs.ErrInvalid},
		{"missing", MemFS{}, "missing.txt", "", fs.ErrNotExist},
		{"invalid path", MemFS{}, "/file.txt", "", fs.ErrInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.fsys.ReadLink(test.path)
			if !errors.Is(err, test.wantErr) {
				t.Errorf("ReadLink(%q) error = %v, want %v", test.path, err, test.wantErr)
			}
			if got != test.want {
				t.Errorf("ReadLink(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

func TestMemFSLstat(t *testing.T) {
	fsys := MemFS{
		"file.txt":      {Data: []byte("hello"), Mode: 0o604},
		"sym.txt":       {Data: []byte("file.txt"), Mode: fs.ModeSymlink},
		"dangling.txt":  {Data: []byte("missing.txt"), Mode: fs.ModeSymlink},
		"dir/inner.txt": {Data: []byte("inner")},
	}

	t.Run("describes a symbolic link without following it", func(t *testing.T) {
		fi, err := fsys.Lstat("sym.txt")
		if err != nil {
			t.Fatalf("Lstat() error = %v", err)
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("Mode() = %v, want a symbolic link", fi.Mode())
		}
	})

	t.Run("describes a dangling link", func(t *testing.T) {
		fi, err := fsys.Lstat("dangling.txt")
		if err != nil {
			t.Fatalf("Lstat() error = %v", err)
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("Mode() = %v, want a symbolic link", fi.Mode())
		}
	})

	t.Run("describes a file", func(t *testing.T) {
		fi, err := fsys.Lstat("file.txt")
		if err != nil {
			t.Fatalf("Lstat() error = %v", err)
		}
		if got := fi.Mode(); got != 0o604 {
			t.Errorf("Mode() = %v, want 0o604", got)
		}
	})

	t.Run("describes a directory", func(t *testing.T) {
		fi, err := fsys.Lstat("dir")
		if err != nil {
			t.Fatalf("Lstat() error = %v", err)
		}
		if !fi.IsDir() {
			t.Error("IsDir() = false, want true")
		}
	})

	t.Run("missing", func(t *testing.T) {
		if _, err := fsys.Lstat("missing.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Lstat() error = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		if _, err := fsys.Lstat("/file.txt"); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Lstat() error = %v, want fs.ErrInvalid", err)
		}
	})
}

func assertFile(t *testing.T, fsys MemFS, name, want string) {
	t.Helper()

	got, err := fs.ReadFile(fsys, name)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", name, err)
	}
	if string(got) != want {
		t.Errorf("ReadFile(%q) = %q, want %q", name, got, want)
	}
}
