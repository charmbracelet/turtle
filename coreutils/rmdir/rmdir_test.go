package rmdir

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

type testCase struct {
	name     string
	files    fsx.MemFS
	args     []string
	present  []string
	absent   []string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: io.Discard,
		Stderr: stderr,
		FS:     test.files,
	}

	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
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

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			name:   "removes an empty directory",
			files:  fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:   []string{"d"},
			absent: []string{"d"},
		},
		{
			name:   "removes every directory in order",
			files:  fsx.MemFS{"one": {Mode: fs.ModeDir}, "two": {Mode: fs.ModeDir}},
			args:   []string{"one", "two"},
			absent: []string{"one", "two"},
		},
		{
			name:   "removes a subdirectory before its parent",
			files:  fsx.MemFS{"a": {Mode: fs.ModeDir}, "a/b": {Mode: fs.ModeDir}},
			args:   []string{"a/b", "a"},
			absent: []string{"a", "a/b"},
		},
		{
			name:     "a parent before the subdirectory is an error",
			files:    fsx.MemFS{"a": {Mode: fs.ModeDir}, "a/b": {Mode: fs.ModeDir}},
			args:     []string{"a", "a/b"},
			present:  []string{"a"},
			absent:   []string{"a/b"},
			wantErr:  "rmdir: a: directory not empty\n",
			wantExit: 1,
		},
		{
			name:     "a missing directory is an error",
			files:    fsx.MemFS{},
			args:     []string{"missing"},
			wantErr:  "rmdir: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a file is an error",
			files:    fsx.MemFS{"file.txt": {Data: []byte("hello")}},
			args:     []string{"file.txt"},
			present:  []string{"file.txt"},
			wantErr:  "rmdir: file.txt: not a directory\n",
			wantExit: 1,
		},
		{
			name:     "a non-empty directory is an error",
			files:    fsx.MemFS{"d/keep.txt": {Data: []byte("keep")}},
			args:     []string{"d"},
			present:  []string{"d/keep.txt"},
			wantErr:  "rmdir: d: directory not empty\n",
			wantExit: 1,
		},
		{
			name:     "keeps removing the other directories after an error",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:     []string{"missing", "d"},
			absent:   []string{"d"},
			wantErr:  "rmdir: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:   "a trailing slash is dropped",
			files:  fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:   []string{"d/"},
			absent: []string{"d"},
		},
		{
			name:   "redundant separators are dropped",
			files:  fsx.MemFS{"a/d": {Mode: fs.ModeDir}},
			args:   []string{"a//d"},
			absent: []string{"a/d"},
		},
		{
			name:   "a directory named like an option after a double dash",
			files:  fsx.MemFS{"-p": {Mode: fs.ModeDir}},
			args:   []string{"--", "-p"},
			absent: []string{"-p"},
		},
		{
			name:   "a directory named with a dash",
			files:  fsx.MemFS{"-": {Mode: fs.ModeDir}},
			args:   []string{"-"},
			absent: []string{"-"},
		},
		{
			name:     "the current directory cannot be removed",
			files:    fsx.MemFS{},
			args:     []string{"."},
			wantErr:  "rmdir: .: invalid argument\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunParents(t *testing.T) {
	tests := []testCase{
		{
			name: "removes every component of the path",
			files: fsx.MemFS{
				"a":     {Mode: fs.ModeDir},
				"a/b":   {Mode: fs.ModeDir},
				"a/b/c": {Mode: fs.ModeDir},
			},
			args:   []string{"-p", "a/b/c"},
			absent: []string{"a", "a/b", "a/b/c"},
		},
		{
			name: "stops at a non-empty directory",
			files: fsx.MemFS{
				"a":          {Mode: fs.ModeDir},
				"a/keep.txt": {Data: []byte("keep")},
				"a/b":        {Mode: fs.ModeDir},
				"a/b/c":      {Mode: fs.ModeDir},
			},
			args:     []string{"-p", "a/b/c"},
			present:  []string{"a", "a/keep.txt"},
			absent:   []string{"a/b", "a/b/c"},
			wantErr:  "rmdir: a: directory not empty\n",
			wantExit: 1,
		},
		{
			name:   "a single component is only the directory itself",
			files:  fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:   []string{"-p", "d"},
			absent: []string{"d"},
		},
		{
			name:     "an intermediate missing directory is an error",
			files:    fsx.MemFS{"a": {Mode: fs.ModeDir}},
			args:     []string{"-p", "a/b"},
			present:  []string{"a"},
			wantErr:  "rmdir: a/b: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "keeps removing the other directories after an error",
			files:    fsx.MemFS{"a": {Mode: fs.ModeDir}, "b": {Mode: fs.ModeDir}, "b/c": {Mode: fs.ModeDir}},
			args:     []string{"-p", "a/b", "b/c"},
			present:  []string{"a"},
			absent:   []string{"b", "b/c"},
			wantErr:  "rmdir: a/b: file does not exist\n",
			wantExit: 1,
		},
		{
			name:   "repeated p options",
			files:  fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:   []string{"-pp", "d"},
			absent: []string{"d"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "missing operand",
			args:     []string{},
			wantErr:  "rmdir: missing operand\n",
			wantExit: 2,
		},
		{
			name:     "invalid option",
			files:    fsx.MemFS{},
			args:     []string{"-x"},
			wantErr:  "rmdir: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "long options are not supported",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:     []string{"--parents", "d"},
			present:  []string{"d"},
			wantErr:  "rmdir: unrecognized option '--parents'\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rt := &coreutils.Runtime{Stderr: io.Discard, FS: fsx.MemFS{}}
	err := New(rt).Run(ctx, "d")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}
