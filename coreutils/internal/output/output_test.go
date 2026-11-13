package output

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/fsx"
)

func TestCreate(t *testing.T) {
	tests := []struct {
		name     string
		fsys     fsx.MemFS
		dir      string
		umask    fs.FileMode
		file     string
		data     string
		want     string
		wantPerm fs.FileMode
	}{
		{
			name:     "creates the file with the umask masked permissions",
			fsys:     fsx.MemFS{},
			umask:    0o022,
			file:     "out.txt",
			data:     "hello",
			want:     "hello",
			wantPerm: 0o644,
		},
		{
			name:     "no umask keeps the read and write permissions",
			fsys:     fsx.MemFS{},
			file:     "out.txt",
			data:     "hello",
			want:     "hello",
			wantPerm: 0o666,
		},
		{
			name:     "a full umask drops every permission",
			fsys:     fsx.MemFS{},
			umask:    0o777,
			file:     "out.txt",
			data:     "hello",
			want:     "hello",
			wantPerm: 0,
		},
		{
			name: "truncates an existing file, keeping its permissions",
			fsys: fsx.MemFS{
				"out.txt": {Data: []byte("old contents"), Mode: 0o600},
			},
			file:     "out.txt",
			data:     "new",
			want:     "new",
			wantPerm: 0o600,
		},
		{
			name: "resolves the name from the working directory",
			fsys: fsx.MemFS{
				"dir/.keep": {},
			},
			dir:      "dir",
			file:     "out.txt",
			data:     "hello",
			want:     "hello",
			wantPerm: 0o666,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &coreutils.Runtime{FS: tt.fsys, Umask: tt.umask, WorkingDir: tt.dir}
			w, err := Create(rt, t.Context(), tt.file)
			if err != nil {
				t.Fatalf("Create error = %v", err)
			}
			if _, err := w.Write([]byte(tt.data)); err != nil {
				t.Fatalf("Write error = %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close error = %v", err)
			}
			name := rt.FsName(tt.file)
			if got := string(tt.fsys[name].Data); got != tt.want {
				t.Errorf("file contents = %q, want %q", got, tt.want)
			}
			fi, err := fs.Stat(tt.fsys, name)
			if err != nil {
				t.Fatalf("Stat error = %v", err)
			}
			if fi.Mode().Perm() != tt.wantPerm {
				t.Errorf("permissions = %v, want %v", fi.Mode().Perm(), tt.wantPerm)
			}
		})
	}
}

func TestAppend(t *testing.T) {
	tests := []struct {
		name     string
		fsys     fsx.MemFS
		umask    fs.FileMode
		file     string
		data     string
		want     string
		wantPerm fs.FileMode
	}{
		{
			name:     "creates the file with the umask masked permissions",
			fsys:     fsx.MemFS{},
			umask:    0o077,
			file:     "out.txt",
			data:     "hello",
			want:     "hello",
			wantPerm: 0o600,
		},
		{
			name: "appends to an existing file, keeping its permissions",
			fsys: fsx.MemFS{
				"out.txt": {Data: []byte("old "), Mode: 0o640},
			},
			file:     "out.txt",
			data:     "new",
			want:     "old new",
			wantPerm: 0o640,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &coreutils.Runtime{FS: tt.fsys, Umask: tt.umask}
			w, err := Append(rt, t.Context(), tt.file)
			if err != nil {
				t.Fatalf("Append error = %v", err)
			}
			if _, err := w.Write([]byte(tt.data)); err != nil {
				t.Fatalf("Write error = %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close error = %v", err)
			}
			if got := string(tt.fsys[tt.file].Data); got != tt.want {
				t.Errorf("file contents = %q, want %q", got, tt.want)
			}
			fi, err := fs.Stat(tt.fsys, tt.file)
			if err != nil {
				t.Fatalf("Stat error = %v", err)
			}
			if fi.Mode().Perm() != tt.wantPerm {
				t.Errorf("permissions = %v, want %v", fi.Mode().Perm(), tt.wantPerm)
			}
		})
	}
}

func TestNoFileSystem(t *testing.T) {
	for _, name := range []string{"create", "append"} {
		t.Run(name, func(t *testing.T) {
			open := Create
			if name == "append" {
				open = Append
			}
			_, err := open(&coreutils.Runtime{}, t.Context(), "out.txt")
			if err == nil || err.Error() != "no file system available" {
				t.Errorf("error = %v, want %q", err, "no file system available")
			}
		})
	}
}

func TestContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	fsys := fsx.MemFS{}
	w, err := Create(&coreutils.Runtime{FS: fsys}, ctx, "out.txt")
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	cancel()
	if _, err := w.Write([]byte("hello")); !errors.Is(err, context.Canceled) {
		t.Errorf("Write error = %v, want %v", err, context.Canceled)
	}
	if err := w.Close(); err != nil {
		t.Errorf("Close error = %v", err)
	}
}
