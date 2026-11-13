package fsx

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRootFSFiles(t *testing.T) {
	dir, fsys := newTestRootFS(t)
	writeTestRootFile(t, fsys, "file.txt", "hello")

	t.Run("reads and stats", func(t *testing.T) {
		data, err := fs.ReadFile(fsys, "file.txt")
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if got, want := string(data), "hello"; got != want {
			t.Errorf("ReadFile() = %q, want %q", got, want)
		}
		if _, err := os.Stat(filepath.Join(dir, "file.txt")); err != nil {
			t.Errorf("Stat() of the created file error = %v", err)
		}
		fi, err := fs.Stat(fsys, "file.txt")
		if err != nil {
			t.Fatalf("fs.Stat() error = %v", err)
		}
		if got, want := fi.Name(), "file.txt"; got != want {
			t.Errorf("Name() = %q, want %q", got, want)
		}
	})

	t.Run("lists directories", func(t *testing.T) {
		entries, err := fs.ReadDir(fsys, ".")
		if err != nil {
			t.Fatalf("ReadDir() error = %v", err)
		}
		if len(entries) != 1 || entries[0].Name() != "file.txt" {
			t.Errorf("ReadDir() = %v, want only file.txt", entries)
		}
	})

	t.Run("preserves file capabilities", func(t *testing.T) {
		f, err := fsys.Open("file.txt")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		if _, ok := f.(SeekFile); !ok {
			t.Errorf("Open() = %T, want a SeekFile", f)
		}
		if _, ok := f.(interface{ Sync() error }); !ok {
			t.Errorf("Open() = %T, want a file with Sync", f)
		}
		if err := f.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	t.Run("supports changing ownership", func(t *testing.T) {
		if _, ok := fsys.(ChownFS); !ok {
			t.Errorf("RootFS() = %T, want a ChownFS", fsys)
		}
	})
}

func TestRootFSSymlinks(t *testing.T) {
	dir, fsys := newTestRootFS(t)
	writeTestRootFile(t, fsys, "file.txt", "hello")

	t.Run("reads and follows relative targets", func(t *testing.T) {
		makeTestRootSymlink(t, fsys, "file.txt", "link.txt")
		target, err := fsys.ReadLink("link.txt")
		if err != nil {
			t.Fatalf("ReadLink() error = %v", err)
		}
		if got, want := target, "file.txt"; got != want {
			t.Errorf("ReadLink() = %q, want %q", got, want)
		}
		fi, err := fsys.Lstat("link.txt")
		if err != nil {
			t.Fatalf("Lstat() error = %v", err)
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("Lstat() Mode() = %v, want a symbolic link", fi.Mode())
		}
		data, err := fs.ReadFile(fsys, "link.txt")
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if got, want := string(data), "hello"; got != want {
			t.Errorf("ReadFile() = %q, want %q", got, want)
		}
	})

	t.Run("rejects absolute targets when following", func(t *testing.T) {
		target := filepath.Join(dir, "file.txt")
		makeTestRootSymlink(t, fsys, target, "absolute.txt")
		if _, err := fs.Stat(fsys, "absolute.txt"); err == nil {
			t.Error("Stat() through an absolute symbolic link error = nil, want an error")
		}
		got, err := fsys.ReadLink("absolute.txt")
		if err != nil {
			t.Fatalf("ReadLink() error = %v", err)
		}
		if got != target {
			t.Errorf("ReadLink() = %q, want %q", got, target)
		}
	})
}

func TestRootFSConfinement(t *testing.T) {
	dir, fsys := newTestRootFS(t)
	writeTestRootFile(t, fsys, "file.txt", "hello")

	tests := []struct {
		name string
		path string
	}{
		{name: "absolute path", path: filepath.Join(dir, "file.txt")},
		{name: "parent path", path: "../file.txt"},
		{name: "dot path", path: "./file.txt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f, err := fsys.Open(test.path)
			if err == nil {
				_ = f.Close()
				t.Errorf("Open(%q) error = nil, want an error", test.path)
			}
		})
	}
}

func newTestRootFS(t *testing.T) (string, FS) {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("Root.Close() error = %v", err)
		}
	})
	return dir, RootFS(root)
}

func writeTestRootFile(t *testing.T, fsys FS, name, data string) {
	t.Helper()
	f, err := fsys.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(%q) error = %v", name, err)
	}
	if _, err := f.Write([]byte(data)); err != nil {
		_ = f.Close()
		t.Fatalf("Write(%q) error = %v", name, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close(%q) error = %v", name, err)
	}
}

func makeTestRootSymlink(t *testing.T, fsys FS, oldName, newName string) {
	t.Helper()
	if err := fsys.Symlink(oldName, newName); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("creating symbolic links needs Developer Mode or admin rights: %v", err)
		}
		t.Fatalf("Symlink(%q, %q) error = %v", oldName, newName, err)
	}
}
