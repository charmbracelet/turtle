package fsx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestOsFS(t *testing.T) {
	dir := t.TempDir()
	fsys := OsFS(dir)

	t.Run("creates a file inside the base directory", func(t *testing.T) {
		f, err := fsys.OpenFile("file.txt", os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if _, err := f.Write([]byte("hello")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		if _, err := os.Stat(filepath.Join(dir, "file.txt")); err != nil {
			t.Errorf("Stat() of the created file error = %v", err)
		}
	})

	t.Run("reads a file", func(t *testing.T) {
		got, err := fs.ReadFile(fsys, "file.txt")
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if want := "hello"; string(got) != want {
			t.Errorf("ReadFile() = %q, want %q", got, want)
		}
	})

	t.Run("truncates with O_TRUNC", func(t *testing.T) {
		f, err := fsys.OpenFile("file.txt", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			t.Fatalf("OpenFile() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		fi, err := fs.Stat(fsys, "file.txt")
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got := fi.Size(); got != 0 {
			t.Errorf("Size() = %d, want 0", got)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if _, err := fsys.Open("missing.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Open() error = %v, want fs.ErrNotExist", err)
		}
	})
}

func TestOsFSAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	fsys := OsFS(t.TempDir())

	path := filepath.ToSlash(filepath.Join(dir, "file.txt"))
	f, err := fsys.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "file.txt")); err != nil {
		t.Errorf("Stat() of the created file error = %v", err)
	}
}

func TestOsFSChtimes(t *testing.T) {
	dir := t.TempDir()
	fsys := OsFS(dir)

	// Zero times mean UTIME_OMIT, which the Linux kernel turns into a
	// no-op that doesn't even look at the path, so use real times here.
	atime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	mtime := time.Date(2021, 6, 7, 8, 9, 10, 0, time.UTC)

	if err := fsys.Chtimes("file.txt", atime, mtime); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Chtimes() error = %v, want fs.ErrNotExist", err)
	}

	f, err := fsys.OpenFile("file.txt", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if _, err := f.Write([]byte("hello")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := fsys.Chtimes("file.txt", atime, mtime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	fi, err := fs.Stat(fsys, "file.txt")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := fi.ModTime().Unix(); got != mtime.Unix() {
		t.Errorf("ModTime() = %v, want %v", fi.ModTime(), mtime)
	}
	got, ok := Atime(fi)
	if !ok {
		t.Skip("Atime() is not available on this platform")
	}
	if got.Unix() != atime.Unix() {
		t.Errorf("Atime() = %v, want %v", got, atime)
	}
}

func TestOsFSChmod(t *testing.T) {
	dir := t.TempDir()
	fsys := OsFS(dir)

	if err := fsys.Chmod("file.txt", 0o755); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Chmod() error = %v, want fs.ErrNotExist", err)
	}

	f, err := fsys.OpenFile("file.txt", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Only the writable bit is honored on Windows, where the other permission
	// bits always read back as 0o666.
	want := fs.FileMode(0o755)
	if runtime.GOOS == "windows" {
		want = 0o666
	}

	if err := fsys.Chmod("file.txt", 0o755); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	fi, err := fs.Stat(fsys, "file.txt")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := fi.Mode().Perm(); got != want {
		t.Errorf("Perm() = %o, want %o", got, want)
	}

	if err := fsys.Chmod("file.txt", 0o444); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	fi, err = fs.Stat(fsys, "file.txt")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := fi.Mode().Perm(); got != fs.FileMode(0o444) {
		t.Errorf("Perm() = %o, want 444", got)
	}

	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := fsys.Chmod("sub", 0o700); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	fi, err = fs.Stat(fsys, "sub")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !fi.IsDir() {
		t.Error("IsDir() = false, want true")
	}
}

func TestOsFSChown(t *testing.T) {
	switch runtime.GOOS {
	case "windows", "plan9":
		t.Skip("the file system here doesn't model ownership")
	}

	dir := t.TempDir()
	fsys := OsFS(dir)

	uid, gid := os.Getuid(), os.Getgid()
	chownFS := fsys.(ChownFS)
	if err := chownFS.Chown("file.txt", uid, gid); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Chown() error = %v, want fs.ErrNotExist", err)
	}

	f, err := fsys.OpenFile("file.txt", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Giving the file to its current owner is always allowed, unlike giving it
	// to somebody else.
	if err := chownFS.Chown("file.txt", uid, gid); err != nil && !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Chown() error = %v, want nil or os.ErrPermission", err)
	}
	fi, err := fs.Stat(fsys, "file.txt")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if gotUID, gotGID, ok := Owner(fi); !ok || gotUID != uid || gotGID != gid {
		t.Errorf("Owner() = %d, %d, %v, want %d, %d, true", gotUID, gotGID, ok, uid, gid)
	}

	if err := chownFS.Chown("file.txt", -1, gid); err != nil && !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Chown() error = %v, want nil or os.ErrPermission", err)
	}
	fi, err = fs.Stat(fsys, "file.txt")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if gotUID, _, ok := Owner(fi); !ok || gotUID != uid {
		t.Errorf("Owner() uid = %d, %v, want %d, true", gotUID, ok, uid)
	}
}

func TestOsFSMkdir(t *testing.T) {
	dir := t.TempDir()
	fsys := OsFS(dir)

	if err := fsys.Mkdir("sub", 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	fi, err := fs.Stat(fsys, "sub")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !fi.IsDir() {
		t.Error("IsDir() = false, want true")
	}

	if err := fsys.Mkdir("sub/inner", 0o700); err != nil {
		t.Fatalf("Mkdir() of a nested directory error = %v", err)
	}
	if err := fsys.Mkdir("sub", 0o755); !errors.Is(err, fs.ErrExist) {
		t.Errorf("Mkdir() of an existing directory error = %v, want fs.ErrExist", err)
	}
	if err := fsys.Mkdir("missing/inner", 0o755); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Mkdir() with a missing parent error = %v, want fs.ErrNotExist", err)
	}
}

func TestOsFSRemove(t *testing.T) {
	dir := t.TempDir()
	fsys := OsFS(dir)

	if err := fsys.Remove("missing.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Remove() of a missing file error = %v, want fs.ErrNotExist", err)
	}

	f, err := fsys.OpenFile("file.txt", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := fsys.Remove("file.txt"); err != nil {
		t.Fatalf("Remove() of a file error = %v", err)
	}
	if _, err := fs.Stat(fsys, "file.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat() error = %v, want fs.ErrNotExist", err)
	}

	if err := fsys.Mkdir("sub", 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := fsys.Remove("sub"); err != nil {
		t.Fatalf("Remove() of an empty directory error = %v", err)
	}

	if err := fsys.Mkdir("sub", 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	f, err = fsys.OpenFile("sub/inner.txt", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := fsys.Remove("sub"); err == nil {
		t.Error("Remove() of a non-empty directory error = nil, want an error")
	}
}

func TestOsFSRename(t *testing.T) {
	dir := t.TempDir()
	fsys := OsFS(dir)

	write := func(t *testing.T, name, content string) {
		t.Helper()

		f, err := fsys.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("OpenFile(%q) error = %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}

	if err := fsys.Rename("missing.txt", "new.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Rename() of a missing file error = %v, want fs.ErrNotExist", err)
	}
	if err := fsys.Rename("file.txt", "missing/new.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Rename() to a missing parent error = %v, want fs.ErrNotExist", err)
	}

	t.Run("renames a file inside the base directory", func(t *testing.T) {
		write(t, "old.txt", "hello")

		if err := fsys.Rename("old.txt", "new.txt"); err != nil {
			t.Fatalf("Rename() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "new.txt")); err != nil {
			t.Errorf("Stat() of the new name error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "old.txt")); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat() of the old name error = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("replaces an existing file", func(t *testing.T) {
		write(t, "replace.txt", "old")
		write(t, "replacer.txt", "new")

		if err := fsys.Rename("replacer.txt", "replace.txt"); err != nil {
			t.Fatalf("Rename() error = %v", err)
		}
		got, err := fs.ReadFile(fsys, "replace.txt")
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if want := "new"; string(got) != want {
			t.Errorf("ReadFile() = %q, want %q", got, want)
		}
	})

	t.Run("renames a directory", func(t *testing.T) {
		if err := fsys.Mkdir("dir", 0o755); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		write(t, "dir/inner.txt", "hello")

		if err := fsys.Rename("dir", "renamed"); err != nil {
			t.Fatalf("Rename() error = %v", err)
		}
		got, err := fs.ReadFile(fsys, "renamed/inner.txt")
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if want := "hello"; string(got) != want {
			t.Errorf("ReadFile() = %q, want %q", got, want)
		}
	})
}

func TestOsFSLink(t *testing.T) {
	dir := t.TempDir()
	fsys := OsFS(dir)

	write := func(t *testing.T, name, content string) {
		t.Helper()

		f, err := fsys.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("OpenFile(%q) error = %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}

	if err := fsys.Link("missing.txt", "link.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Link() of a missing file error = %v, want fs.ErrNotExist", err)
	}

	write(t, "file.txt", "hello")

	if err := fsys.Link("file.txt", "link.txt"); err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if err := fsys.Link("file.txt", "link.txt"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("Link() to an existing name error = %v, want fs.ErrExist", err)
	}

	got, err := fs.ReadFile(fsys, "link.txt")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if want := "hello"; string(got) != want {
		t.Errorf("ReadFile() = %q, want %q", got, want)
	}

	if err := fsys.Mkdir("dir", 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := fsys.Link("dir", "dirlink"); err == nil {
		t.Error("Link() of a directory error = nil, want an error")
	}
}

func TestOsFSSymlink(t *testing.T) {
	dir := t.TempDir()
	fsys := OsFS(dir)

	write := func(t *testing.T, name, content string) {
		t.Helper()

		f, err := fsys.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("OpenFile(%q) error = %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}

	symlink := func(t *testing.T, oldName, newName string) {
		t.Helper()

		if err := fsys.Symlink(oldName, newName); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("creating symbolic links needs Developer Mode or admin rights: %v", err)
			}
			t.Fatalf("Symlink(%q, %q) error = %v", oldName, newName, err)
		}
	}

	if _, err := fsys.ReadLink("missing.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadLink() of a missing file error = %v, want fs.ErrNotExist", err)
	}
	if _, err := fsys.Lstat("missing.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat() of a missing file error = %v, want fs.ErrNotExist", err)
	}

	write(t, "file.txt", "hello")
	symlink(t, "file.txt", "sym.txt")

	if err := fsys.Symlink("file.txt", "sym.txt"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("Symlink() to an existing name error = %v, want fs.ErrExist", err)
	}

	got, err := fsys.ReadLink("sym.txt")
	if err != nil {
		t.Fatalf("ReadLink() error = %v", err)
	}
	if want := "file.txt"; got != want {
		t.Errorf("ReadLink() = %q, want %q", got, want)
	}

	fi, err := fsys.Lstat("sym.txt")
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("Lstat() Mode() = %v, want a symbolic link", fi.Mode())
	}

	// Stat follows the link, and reading through it reaches the target.
	fi, err = fs.Stat(fsys, "sym.txt")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		t.Errorf("Stat() Mode() = %v, want the target mode", fi.Mode())
	}
	if got, err := fs.ReadFile(fsys, "sym.txt"); err != nil || string(got) != "hello" {
		t.Errorf("ReadFile() = %q, %v, want %q, nil", got, err, "hello")
	}

	symlink(t, "missing.txt", "dangling.txt")
	if _, err := fs.Stat(fsys, "dangling.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat() of a dangling link error = %v, want fs.ErrNotExist", err)
	}
	if fi, err := fsys.Lstat("dangling.txt"); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("Lstat() of a dangling link = %v, %v, want a symbolic link", fi, err)
	}

	// EINVAL isn't mapped to fs.ErrInvalid by the syscall package, so any
	// error is taken.
	if _, err := fsys.ReadLink("file.txt"); err == nil {
		t.Error("ReadLink() of a regular file error = nil, want an error")
	}
}

func TestOsFSSync(t *testing.T) {
	dir := t.TempDir()
	fsys := OsFS(dir)

	f, err := fsys.OpenFile("file.txt", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if _, err := f.Write([]byte("hello")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	t.Run("an open file syncs", func(t *testing.T) {
		syncer, ok := f.(interface{ Sync() error })
		if !ok {
			t.Fatalf("OpenFile() = %T, want a file with a Sync method", f)
		}
		if err := syncer.Sync(); err != nil {
			t.Errorf("Sync() error = %v", err)
		}
	})

	t.Run("the file system syncs", func(t *testing.T) {
		syncer, ok := fsys.(SyncFS)
		if !ok {
			t.Fatalf("OsFS() = %T, want a SyncFS", fsys)
		}
		if err := syncer.Sync(); err != nil {
			t.Errorf("Sync() error = %v", err)
		}
	})

	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
