package link

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

type testCase struct {
	name     string
	files    fsx.MemFS
	wd       string
	args     []string
	want     map[string]string
	wantSame map[string]string
	present  []string
	absent   []string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
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
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want none", got)
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

func TestRun(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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
				name:     "resolves the operands against the working directory",
				files:    fsx.MemFS{"d/a.txt": {Data: []byte("hello")}},
				wd:       "d",
				args:     []string{"a.txt", "b.txt"},
				want:     map[string]string{"d/b.txt": "hello"},
				wantSame: map[string]string{"d/b.txt": "d/a.txt"},
				present:  []string{"d/a.txt"},
			},
			{
				name:     "an operand starting with a dash is a file",
				files:    fsx.MemFS{"-f.txt": {Data: []byte("hello")}},
				args:     []string{"-f.txt", "b.txt"},
				want:     map[string]string{"b.txt": "hello"},
				wantSame: map[string]string{"b.txt": "-f.txt"},
				present:  []string{"-f.txt"},
			},
			{
				name:     "a double dash is an operand",
				files:    fsx.MemFS{"--": {Data: []byte("hello")}},
				args:     []string{"--", "b.txt"},
				want:     map[string]string{"b.txt": "hello"},
				wantSame: map[string]string{"b.txt": "--"},
				present:  []string{"--"},
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("failures", func(t *testing.T) {
		tests := []testCase{
			{
				name:     "an existing destination is an error",
				files:    fsx.MemFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
				args:     []string{"a.txt", "b.txt"},
				want:     map[string]string{"b.txt": "old"},
				present:  []string{"a.txt"},
				wantErr:  "link: cannot create link 'b.txt' to 'a.txt': file already exists\n",
				wantExit: 1,
			},
			{
				name:     "a directory source is an error",
				files:    fsx.MemFS{"d/keep.txt": {}},
				args:     []string{"d", "link"},
				present:  []string{"d/keep.txt"},
				absent:   []string{"link"},
				wantErr:  "link: cannot create link 'link' to 'd': permission denied\n",
				wantExit: 1,
			},
			{
				name:     "a missing source is an error",
				files:    fsx.MemFS{},
				args:     []string{"missing.txt", "b.txt"},
				absent:   []string{"b.txt"},
				wantErr:  "link: cannot create link 'b.txt' to 'missing.txt': file does not exist\n",
				wantExit: 1,
			},
			{
				name:     "a missing parent of the destination is an error",
				files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"a.txt", "d/b.txt"},
				present:  []string{"a.txt"},
				wantErr:  "link: cannot create link 'd/b.txt' to 'a.txt': file does not exist\n",
				wantExit: 1,
			},
			{
				name:     "a trailing slash on the source is rejected",
				files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"a.txt/", "b.txt"},
				present:  []string{"a.txt"},
				absent:   []string{"b.txt"},
				wantErr:  "link: cannot create link 'b.txt' to 'a.txt/': invalid argument\n",
				wantExit: 1,
			},
			{
				name:     "a trailing slash on the destination is rejected",
				files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"a.txt", "b.txt/"},
				present:  []string{"a.txt"},
				absent:   []string{"b.txt"},
				wantErr:  "link: cannot create link 'b.txt/' to 'a.txt': invalid argument\n",
				wantExit: 1,
			},
			{
				name: "a dot-dot isn't resolved lexically away from a symlink",
				files: fsx.MemFS{
					"a.txt": {Data: []byte("hello")},
					"s":     {Data: []byte("d"), Mode: fs.ModeSymlink},
				},
				args:     []string{"s/../a.txt", "b.txt"},
				present:  []string{"a.txt"},
				absent:   []string{"b.txt"},
				wantErr:  "link: cannot create link 'b.txt' to 's/../a.txt': invalid argument\n",
				wantExit: 1,
			},
			{
				name:     "an empty source operand is an error",
				files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"", "b.txt"},
				present:  []string{"a.txt"},
				absent:   []string{"b.txt"},
				wantErr:  "link: invalid zero-length file name\n",
				wantExit: 1,
			},
			{
				name:     "an empty destination operand is an error",
				files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"a.txt", ""},
				present:  []string{"a.txt"},
				wantErr:  "link: invalid zero-length file name\n",
				wantExit: 1,
			},
			{
				name:     "no operand is an error",
				files:    fsx.MemFS{},
				args:     nil,
				wantErr:  "link: missing file operand\n",
				wantExit: 2,
			},
			{
				name:     "a single operand is an error",
				files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"a.txt"},
				present:  []string{"a.txt"},
				wantErr:  "link: missing destination file operand after 'a.txt'\n",
				wantExit: 2,
			},
			{
				name:     "extra operands are an error",
				files:    fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}},
				args:     []string{"a.txt", "b.txt", "c.txt"},
				present:  []string{"a.txt", "b.txt"},
				absent:   []string{"c.txt"},
				wantErr:  "link: extra operand 'c.txt'\n",
				wantExit: 2,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})
}

func TestLinkSharesData(t *testing.T) {
	files := fsx.MemFS{"a.txt": {Data: []byte("hello")}}
	rt := &coreutils.Runtime{
		Stdin:      strings.NewReader(""),
		Stdout:     io.Discard,
		Stderr:     new(bytes.Buffer),
		FS:         files,
		WorkingDir: "",
	}

	if err := New(rt).Run(t.Context(), "a.txt", "b.txt"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	f, err := files.OpenFile("b.txt", os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		t.Fatalf("OpenFile(%q) error = %v", "b.txt", err)
	}
	if _, err := f.Write([]byte("world")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	got, err := fs.ReadFile(files, "a.txt")
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", "a.txt", err)
	}
	if string(got) != "world" {
		t.Errorf("ReadFile(%q) = %q, want %q", "a.txt", got, "world")
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader(""),
			Stdout: stdout,
			Stderr: new(bytes.Buffer),
		}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader(""),
			Stdout: stdout,
			Stderr: new(bytes.Buffer),
		}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "link (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
