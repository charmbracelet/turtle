package unlink

import (
	"bytes"
	"errors"
	"io/fs"
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

	for _, name := range test.present {
		if _, err := fs.Stat(test.files, name); err != nil {
			t.Errorf("Stat(%q) error = %v, want it to still exist", name, err)
		}
	}
	for _, name := range test.absent {
		if _, err := fs.Stat(test.files, name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat(%q) error = %v, want fs.ErrNotExist", name, err)
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
				name:   "removes a file",
				files:  fsx.MemFS{"a.txt": {Data: []byte("hello")}},
				args:   []string{"a.txt"},
				absent: []string{"a.txt"},
			},
			{
				name: "removes a symbolic link itself",
				files: fsx.MemFS{
					"a.txt":  {Data: []byte("hello")},
					"b.link": {Data: []byte("a.txt"), Mode: fs.ModeSymlink},
				},
				args:    []string{"b.link"},
				absent:  []string{"b.link"},
				present: []string{"a.txt"},
			},
			{
				name: "removes a symbolic link to a directory itself",
				files: fsx.MemFS{
					"d/a.txt": {Data: []byte("hello")},
					"link":    {Data: []byte("d"), Mode: fs.ModeSymlink},
				},
				args:    []string{"link"},
				absent:  []string{"link"},
				present: []string{"d/a.txt"},
			},
			{
				name:   "resolves the operand against the working directory",
				files:  fsx.MemFS{"d/a.txt": {Data: []byte("hello")}},
				wd:     "d",
				args:   []string{"a.txt"},
				absent: []string{"d/a.txt"},
			},
			{
				name:   "an operand starting with a dash is a file",
				files:  fsx.MemFS{"-f.txt": {Data: []byte("hello")}},
				args:   []string{"-f.txt"},
				absent: []string{"-f.txt"},
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
				name:     "a directory is not removed",
				files:    fsx.MemFS{"d/a.txt": {Data: []byte("hello")}},
				args:     []string{"d"},
				present:  []string{"d/a.txt"},
				wantErr:  "unlink: d: is a directory\n",
				wantExit: 1,
			},
			{
				name:     "an empty directory is not removed either",
				files:    fsx.MemFS{"d": {Mode: fs.ModeDir}},
				args:     []string{"d"},
				present:  []string{"d"},
				wantErr:  "unlink: d: is a directory\n",
				wantExit: 1,
			},
			{
				name:     "a trailing slash makes the operand a directory reference",
				files:    fsx.MemFS{"a.txt": {Data: []byte("hello")}},
				args:     []string{"a.txt/"},
				present:  []string{"a.txt"},
				wantErr:  "unlink: a.txt/: invalid argument\n",
				wantExit: 1,
			},
			{
				name: "a dot-dot isn't resolved lexically away from a symlink",
				files: fsx.MemFS{
					"a.txt": {Data: []byte("hello")},
					"s":     {Data: []byte("d"), Mode: fs.ModeSymlink},
				},
				args:     []string{"s/../a.txt"},
				present:  []string{"a.txt"},
				wantErr:  "unlink: s/../a.txt: invalid argument\n",
				wantExit: 1,
			},
			{
				name:     "a missing file is an error",
				files:    fsx.MemFS{},
				args:     []string{"missing"},
				wantErr:  "unlink: missing: file does not exist\n",
				wantExit: 1,
			},
			{
				name:     "a missing operand starting with a dash is an error",
				files:    fsx.MemFS{},
				args:     []string{"-f"},
				wantErr:  "unlink: -f: file does not exist\n",
				wantExit: 1,
			},
			{
				name:     "a double dash is an operand",
				files:    fsx.MemFS{},
				args:     []string{"--"},
				wantErr:  "unlink: --: file does not exist\n",
				wantExit: 1,
			},
			{
				name:     "an empty operand is an error",
				files:    fsx.MemFS{},
				args:     []string{""},
				wantErr:  "unlink: invalid zero-length file name\n",
				wantExit: 1,
			},
			{
				name:     "no operand is an error",
				files:    fsx.MemFS{},
				args:     nil,
				wantErr:  "unlink: missing operand\n",
				wantExit: 2,
			},
			{
				name:     "an extra operand is an error",
				files:    fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}},
				args:     []string{"a.txt", "b.txt"},
				present:  []string{"a.txt", "b.txt"},
				wantErr:  "unlink: extra operand 'b.txt'\n",
				wantExit: 2,
			},
			{
				name:     "an extra operand after a double dash is an error",
				files:    fsx.MemFS{"a.txt": {Data: []byte("a")}},
				args:     []string{"--", "a.txt"},
				present:  []string{"a.txt"},
				wantErr:  "unlink: extra operand 'a.txt'\n",
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
		want := "unlink (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
