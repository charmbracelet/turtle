package tar

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

var oldTime = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)

// entry is one record of an archive, as parsed back for assertions.
type entry struct {
	name  string
	typ   byte
	link  string
	mode  int64
	body  string
	mtime time.Time
	uid   int
	gid   int
	uname string
	gname string
}

// parseArchive decodes the entries of an archive for inspection.
func parseArchive(t *testing.T, data string) []entry {
	t.Helper()

	tr := tar.NewReader(strings.NewReader(data))
	var entries []entry
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return entries
		}
		if err != nil {
			t.Fatalf("tar.NewReader.Next() error = %v", err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("ReadAll(%s) error = %v", hdr.Name, err)
		}
		entries = append(entries, entry{
			name:  hdr.Name,
			typ:   hdr.Typeflag,
			link:  hdr.Linkname,
			mode:  hdr.Mode,
			body:  string(body),
			mtime: hdr.ModTime,
			uid:   hdr.Uid,
			gid:   hdr.Gid,
		})
	}
}

// compareEntries reports the differences between two entry lists.
func compareEntries(t *testing.T, want, got []entry) {
	t.Helper()

	if len(want) != len(got) {
		t.Fatalf("entries = %v (%d), want %v (%d)", got, len(got), want, len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.name != w.name || g.typ != w.typ || g.link != w.link || g.mode != w.mode ||
			g.body != w.body ||
			(w.uid >= 0 && g.uid != w.uid) || (w.gid >= 0 && g.gid != w.gid) ||
			(!w.mtime.IsZero() && !g.mtime.Equal(w.mtime)) {
			t.Errorf("entry[%d] = %+v, want %+v", i, g, w)
		}
	}
}

// buildArchive encodes entries into an archive for input tests.
func buildArchive(t *testing.T, entries ...entry) string {
	t.Helper()

	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typ,
			Linkname: e.link,
			Mode:     e.mode,
			ModTime:  e.mtime,
			Uid:      e.uid,
			Gid:      e.gid,
			Uname:    e.uname,
			Gname:    e.gname,
		}
		if e.typ == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%s) error = %v", e.name, err)
		}
		if e.body != "" {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("Write(%s) error = %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar.Writer.Close() error = %v", err)
	}
	return buf.String()
}

// environ converts a map to a list of KEY=VALUE strings.
func environ(env map[string]string) []string {
	entries := make([]string, 0, len(env))
	for name, value := range env {
		entries = append(entries, name+"="+value)
	}
	return entries
}

// run executes the utility against files, returning its streams and status.
func run(t *testing.T, files fsx.FS, stdin string, env map[string]string, umask fs.FileMode, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:   strings.NewReader(stdin),
		Stdout:  stdout,
		Stderr:  stderr,
		FS:      files,
		Umask:   umask,
		Environ: func() []string { return environ(env) },
	}
	err := New(rt).Run(t.Context(), args...)
	return stdout, stderr, err
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

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"no mode", nil, "tar: You must specify one of the '-ctrux' options\n"},
		{"no mode with operand", []string{"nf", "-"}, "tar: You must specify one of the '-ctrux' options\n"},
		{"two modes", []string{"-ct", "-"}, "tar: You may not specify more than one '-ctrux' option\n"},
		{"two modes long", []string{"--create", "--list"}, "tar: You may not specify more than one '-ctrux' option\n"},
		{"invalid option", []string{"-cz", "-"}, "tar: invalid option -- 'z'\n"},
		{"unrecognized long option", []string{"-c", "--bogus"}, "tar: unrecognized option '--bogus'\n"},
		{"missing archive", []string{"-c", "-f"}, "tar: option requires an argument -- 'f'\n"},
		{"missing blocking factor", []string{"-c", "--blocking-factor"}, "tar: option '--blocking-factor' requires an argument\n"},
		{"zero blocking factor", []string{"-c", "-b", "0"}, `tar: invalid blocking factor "0"` + "\n"},
		{"bad blocking factor", []string{"-c", "-b", "x"}, `tar: invalid blocking factor "x"` + "\n"},
		{"bad old style letter", []string{"czf", "-", "-"}, "tar: invalid option -- 'z'\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, stderr, err := run(t, nil, "", nil, 0, test.args...)
			if code := exitCode(t, err); code != 2 {
				t.Errorf("Run() exit code = %d, want 2 (error = %v)", code, err)
			}
			if got := stderr.String(); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
		})
	}
}

func TestExpandOldStyle(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"function and archive", []string{"cf", "a.tar", "x"}, []string{"-c", "-f", "a.tar", "x"}},
		{"grouped flags", []string{"cvf", "-", "x"}, []string{"-c", "-v", "-f", "-", "x"}},
		{"blocking factor", []string{"cvfb", "-", "2", "x"}, []string{"-c", "-v", "-f", "-", "-b", "2", "x"}},
		{"all functions", []string{"crtux"}, []string{"-c", "-r", "-t", "-u", "-x"}},
		{"value missing", []string{"cf"}, []string{"-c"}},
		{"bare function", []string{"c"}, []string{"-c"}},
		{"dashed options kept", []string{"-c", "x"}, []string{"-c", "x"}},
		{"operand first", []string{"foo", "x"}, []string{"foo", "x"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := expandOldStyle(test.args)
			if strings.Join(got, " ") != strings.Join(test.want, " ") {
				t.Errorf("expandOldStyle() = %q, want %q", got, test.want)
			}
		})
	}
}

type createCase struct {
	name     string
	files    fsx.MemFS
	stdin    string
	umask    fs.FileMode
	args     []string
	archive  string
	want     []entry
	wantErr  string
	wantExit int
	record   int
}

func runCreate(t *testing.T, test createCase) {
	t.Helper()

	stdout, stderr, err := run(t, test.files, test.stdin, nil, test.umask, test.args...)
	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
	data := stdout.String()
	if test.archive != "" {
		got, err := fs.ReadFile(test.files, test.archive)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", test.archive, err)
		}
		data = string(got)
	}
	if test.record > 0 && len(data)%test.record != 0 {
		t.Errorf("archive length = %d, not a multiple of %d", len(data), test.record)
	}
	compareEntries(t, test.want, parseArchive(t, data))
}

func helloFile() *fsx.MemFile {
	return &fsx.MemFile{Data: []byte("hello\n"), Mode: 0o644, ModTime: oldTime}
}

func TestCreate(t *testing.T) {
	tests := []createCase{
		{
			name:  "file to stdout",
			files: fsx.MemFS{"hello.txt": helloFile()},
			args:  []string{"-cf", "-", "hello.txt"},
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
			},
		},
		{
			name:  "default archive is the standard output",
			files: fsx.MemFS{"hello.txt": helloFile()},
			args:  []string{"-c", "hello.txt"},
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
			},
		},
		{
			name: "directory tree",
			files: fsx.MemFS{
				"sub/a.txt":   {Data: []byte("a\n"), Mode: 0o644, ModTime: oldTime},
				"sub/b.txt":   {Data: []byte("b\n"), Mode: 0o600, ModTime: oldTime},
				"outside.txt": {Data: []byte("no\n"), Mode: 0o644},
			},
			args: []string{"-cf", "-", "sub"},
			want: []entry{
				{name: "sub/", typ: tar.TypeDir, mode: 0o755},
				{name: "sub/a.txt", typ: tar.TypeReg, mode: 0o644, body: "a\n", mtime: oldTime},
				{name: "sub/b.txt", typ: tar.TypeReg, mode: 0o600, body: "b\n", mtime: oldTime},
			},
		},
		{
			name: "trailing slash operand",
			files: fsx.MemFS{
				"sub/a.txt": {Data: []byte("a\n"), Mode: 0o644, ModTime: oldTime},
			},
			args: []string{"-cf", "-", "sub/"},
			want: []entry{
				{name: "sub/", typ: tar.TypeDir, mode: 0o755},
				{name: "sub/a.txt", typ: tar.TypeReg, mode: 0o644, body: "a\n", mtime: oldTime},
			},
		},
		{
			name: "symlink",
			files: fsx.MemFS{
				"link":   {Mode: fs.ModeSymlink | 0o777, Data: []byte("target")},
				"target": {Data: []byte("t\n"), Mode: 0o644},
			},
			args: []string{"-cf", "-", "link"},
			want: []entry{
				{name: "link", typ: tar.TypeSymlink, link: "target", mode: 0o777},
			},
		},
		{
			name: "dereference symlink",
			files: fsx.MemFS{
				"link":      {Mode: fs.ModeSymlink | 0o777, Data: []byte("hello.txt")},
				"hello.txt": helloFile(),
			},
			args: []string{"-cf", "-", "-h", "link"},
			want: []entry{
				{name: "link", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
			},
		},
		{
			name:     "socket ignored",
			files:    fsx.MemFS{"sock": {Mode: fs.ModeSocket}},
			args:     []string{"-cf", "-", "sock"},
			wantErr:  "tar: sock: socket ignored\n",
			wantExit: 1,
		},
		{
			name:  "fifo",
			files: fsx.MemFS{"pipe": {Mode: fs.ModeNamedPipe | 0o644}},
			args:  []string{"-cf", "-", "pipe"},
			want: []entry{
				{name: "pipe", typ: tar.TypeFifo, mode: 0o644},
			},
		},
		{
			name:  "missing operand doesn't stop the rest",
			files: fsx.MemFS{"hello.txt": helloFile()},
			args:  []string{"-cf", "-", "nope", "hello.txt"},
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
			},
			wantErr:  "tar: nope: file does not exist\n",
			wantExit: 1,
		},
		{
			name:  "ignore failed read",
			files: fsx.MemFS{"hello.txt": helloFile()},
			args:  []string{"-cf", "-", "-e", "nope", "hello.txt"},
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
			},
			wantErr: "tar: nope: file does not exist\n",
		},
		{
			name:     "no operands",
			files:    fsx.MemFS{"hello.txt": helloFile()},
			args:     []string{"-cf", "-"},
			wantErr:  "tar: Cowardly refusing to create an empty archive\n",
			wantExit: 2,
		},
		{
			name:  "verbose names on stderr",
			files: fsx.MemFS{"hello.txt": helloFile()},
			args:  []string{"-cvf", "-", "hello.txt"},
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
			},
			wantErr: "tar: hello.txt\n",
		},
		{
			name:    "archive file gets mode from umask",
			files:   fsx.MemFS{"hello.txt": helloFile()},
			umask:   0o022,
			args:    []string{"-cf", "a.tar", "hello.txt"},
			archive: "a.tar",
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
			},
		},
		{
			name:    "declined file is left out",
			files:   fsx.MemFS{"hello.txt": helloFile()},
			args:    []string{"-cf", "-", "-w", "hello.txt"},
			wantErr: "tar: add 'hello.txt'? ",
		},
		{
			name:  "accepted file is archived",
			files: fsx.MemFS{"hello.txt": helloFile()},
			stdin: "y\n",
			args:  []string{"-cf", "-", "-w", "hello.txt"},
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
			},
			wantErr: "tar: add 'hello.txt'? ",
		},
		{
			name:  "blocking factor of two",
			files: fsx.MemFS{"hello.txt": helloFile()},
			args:  []string{"-cf", "-", "-b", "2", "hello.txt"},
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
			},
			record: 1024,
		},
		{
			name:  "blocking factor of one",
			files: fsx.MemFS{"hello.txt": helloFile()},
			args:  []string{"-cf", "-", "-b", "1", "hello.txt"},
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
			},
			record: 512,
		},
		{
			name:  "owner from file system",
			files: fsx.MemFS{"hello.txt": {Data: []byte("hello\n"), Mode: 0o644, ModTime: oldTime, Uid: 1000, Gid: 2000}},
			args:  []string{"-cf", "-", "hello.txt"},
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime, uid: 1000, gid: 2000},
			},
		},
		{
			name:  "setuid bit",
			files: fsx.MemFS{"prog": {Data: []byte(""), Mode: fs.ModeSetuid | 0o755, ModTime: oldTime}},
			args:  []string{"-cf", "-", "prog"},
			want: []entry{
				{name: "prog", typ: tar.TypeReg, mode: 0o4755, mtime: oldTime},
			},
		},
		{
			name:  "default record size",
			files: fsx.MemFS{"hello.txt": helloFile()},
			args:  []string{"-cf", "-", "hello.txt"},
			want: []entry{
				{name: "hello.txt", typ: tar.TypeReg, mode: 0o644, body: "hello\n", mtime: oldTime},
			},
			record: 10240,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCreate(t, test)
		})
	}
}

func TestCreateAbsolute(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(dir+"/d", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/d/f.txt", []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := run(t, fsx.OsFS(""), "", nil, 0, "-cf", "-", dir+"/d/f.txt")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := stderr.String(); got != "tar: Removing leading '/' from member names\n" {
		t.Errorf("stderr = %q, want the strip note", got)
	}

	// The stored name drops what made the operand absolute: the leading
	// slashes, and the drive letter on Windows, where files also report a
	// mode of 0666.
	want := strings.TrimLeft(filepath.ToSlash(dir+"/d/f.txt"), "/")
	if vol := filepath.VolumeName(want); vol != "" {
		want = strings.TrimLeft(want[len(vol):], "/")
	}
	mode := int64(0o644)
	if runtime.GOOS == "windows" {
		mode = 0o666
	}
	compareEntries(t, []entry{
		{name: want, typ: tar.TypeReg, mode: mode, body: "x\n", uid: -1, gid: -1},
	}, parseArchive(t, stdout.String()))
}

func TestWriteError(t *testing.T) {
	rt := &coreutils.Runtime{
		Stdout: errorWriter{},
		Stderr: new(bytes.Buffer),
		FS:     fsx.MemFS{"hello.txt": helloFile()},
	}
	err := New(rt).Run(t.Context(), "-cf", "-", "hello.txt")
	if code := exitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1", code)
	}
}

type errorWriter struct{}

func (errorWriter) Write(p []byte) (int, error) { return 0, errors.New("boom") }

func TestContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	files := fsx.MemFS{"hello.txt": helloFile()}
	rt := &coreutils.Runtime{
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
		FS:     files,
	}
	if err := New(rt).Run(ctx, "-cf", "-", "hello.txt"); !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
	if err := New(rt).Run(ctx, "-xf", "-"); !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestNoFileSystem(t *testing.T) {
	_, stderr, err := run(t, nil, "", nil, 0, "-cf", "a.tar", "hello.txt")
	if code := exitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1", code)
	}
	if got := stderr.String(); got != "tar: a.tar: no file system available\n" {
		t.Errorf("stderr = %q, want %q", got, "tar: a.tar: no file system available\n")
	}
}

func TestParseSize(t *testing.T) {
	bin := make([]byte, 12)
	bin[0] = 0x80
	bin[11] = 0xff
	tests := []struct {
		name  string
		field []byte
		want  int64
		err   bool
	}{
		{"octal", []byte("12\x00"), 10, false},
		{"octal with spaces", []byte("  12  "), 10, false},
		{"empty", make([]byte, 12), 0, false},
		{"base 256", bin, 255, false},
		{"invalid", []byte("9"), 0, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseSize(test.field)
			if (err != nil) != test.err {
				t.Fatalf("parseSize() error = %v, want err %v", err, test.err)
			}
			if got != test.want {
				t.Errorf("parseSize() = %d, want %d", got, test.want)
			}
		})
	}
}
