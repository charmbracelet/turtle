package link

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

type testCase struct {
	name     string
	files    fsx.MemoryFS
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

	testutil.Once(t, New, testutil.Case{
		Name:      test.name,
		FS:        test.files,
		Dir:       test.wd,
		Args:      test.args,
		WantFiles: test.want,
		Present:   test.present,
		Absent:    test.absent,
		WantErr:   test.wantErr,
		WantExit:  test.wantExit,
	})

	for name, other := range test.wantSame {
		if test.files[name] != test.files[other] {
			t.Errorf("%s and %s are different entries, want the same", name, other)
		}
	}
}

func TestRun(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := []testCase{
			{
				name:     "creates a hard link to a file",
				files:    fsx.MemoryFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"a.txt", "b.txt"},
				want:     map[string]string{"b.txt": "hello"},
				wantSame: map[string]string{"b.txt": "a.txt"},
				present:  []string{"a.txt"},
			},
			{
				name:     "resolves the operands against the working directory",
				files:    fsx.MemoryFS{"d/a.txt": {Data: []byte("hello")}},
				wd:       "d",
				args:     []string{"a.txt", "b.txt"},
				want:     map[string]string{"d/b.txt": "hello"},
				wantSame: map[string]string{"d/b.txt": "d/a.txt"},
				present:  []string{"d/a.txt"},
			},
			{
				name:     "an operand starting with a dash is a file",
				files:    fsx.MemoryFS{"-f.txt": {Data: []byte("hello")}},
				args:     []string{"-f.txt", "b.txt"},
				want:     map[string]string{"b.txt": "hello"},
				wantSame: map[string]string{"b.txt": "-f.txt"},
				present:  []string{"-f.txt"},
			},
			{
				name:     "a double dash is an operand",
				files:    fsx.MemoryFS{"--": {Data: []byte("hello")}},
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
				files:    fsx.MemoryFS{"a.txt": {Data: []byte("new")}, "b.txt": {Data: []byte("old")}},
				args:     []string{"a.txt", "b.txt"},
				want:     map[string]string{"b.txt": "old"},
				present:  []string{"a.txt"},
				wantErr:  "link: cannot create link 'b.txt' to 'a.txt': file already exists\n",
				wantExit: 1,
			},
			{
				name:     "a directory source is an error",
				files:    fsx.MemoryFS{"d/keep.txt": {}},
				args:     []string{"d", "link"},
				present:  []string{"d/keep.txt"},
				absent:   []string{"link"},
				wantErr:  "link: cannot create link 'link' to 'd': permission denied\n",
				wantExit: 1,
			},
			{
				name:     "a missing source is an error",
				files:    fsx.MemoryFS{},
				args:     []string{"missing.txt", "b.txt"},
				absent:   []string{"b.txt"},
				wantErr:  "link: cannot create link 'b.txt' to 'missing.txt': file does not exist\n",
				wantExit: 1,
			},
			{
				name:     "a missing parent of the destination is an error",
				files:    fsx.MemoryFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"a.txt", "d/b.txt"},
				present:  []string{"a.txt"},
				wantErr:  "link: cannot create link 'd/b.txt' to 'a.txt': file does not exist\n",
				wantExit: 1,
			},
			{
				name:     "a trailing slash on the source is rejected",
				files:    fsx.MemoryFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"a.txt/", "b.txt"},
				present:  []string{"a.txt"},
				absent:   []string{"b.txt"},
				wantErr:  "link: cannot create link 'b.txt' to 'a.txt/': invalid argument\n",
				wantExit: 1,
			},
			{
				name:     "a trailing slash on the destination is rejected",
				files:    fsx.MemoryFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"a.txt", "b.txt/"},
				present:  []string{"a.txt"},
				absent:   []string{"b.txt"},
				wantErr:  "link: cannot create link 'b.txt/' to 'a.txt': invalid argument\n",
				wantExit: 1,
			},
			{
				name: "a dot-dot isn't resolved lexically away from a symlink",
				files: fsx.MemoryFS{
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
				files:    fsx.MemoryFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"", "b.txt"},
				present:  []string{"a.txt"},
				absent:   []string{"b.txt"},
				wantErr:  "link: invalid zero-length file name\n",
				wantExit: 1,
			},
			{
				name:     "an empty destination operand is an error",
				files:    fsx.MemoryFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"a.txt", ""},
				present:  []string{"a.txt"},
				wantErr:  "link: invalid zero-length file name\n",
				wantExit: 1,
			},
			{
				name:     "no operand is an error",
				files:    fsx.MemoryFS{},
				args:     nil,
				wantErr:  "link: missing file operand\nTry 'link --help' for more information.\n",
				wantExit: 2,
			},
			{
				name:     "a single operand is an error",
				files:    fsx.MemoryFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"a.txt"},
				present:  []string{"a.txt"},
				wantErr:  "link: missing destination file operand after 'a.txt'\nTry 'link --help' for more information.\n",
				wantExit: 2,
			},
			{
				name:     "extra operands are an error",
				files:    fsx.MemoryFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}},
				args:     []string{"a.txt", "b.txt", "c.txt"},
				present:  []string{"a.txt", "b.txt"},
				absent:   []string{"c.txt"},
				wantErr:  "link: extra operand 'c.txt'\nTry 'link --help' for more information.\n",
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
	files := fsx.MemoryFS{"a.txt": {Data: []byte("hello")}}
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
	testutil.HelpVersion(t, New, "link", usage)
}
