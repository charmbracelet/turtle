package mkdir

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
	umask    fs.FileMode
	args     []string
	want     map[string]fs.FileMode
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
		Umask:  test.umask,
	}

	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}

	for name, wantMode := range test.want {
		fi, err := fs.Stat(test.files, name)
		if err != nil {
			t.Errorf("Stat(%q) error = %v", name, err)
			continue
		}
		if got, want := fi.Mode(), fs.ModeDir|wantMode; got != want {
			t.Errorf("%s mode = %v, want %v", name, got, want)
		}
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
			name:  "creates a directory",
			files: fsx.MemFS{},
			args:  []string{"d"},
			want:  map[string]fs.FileMode{"d": 0o777},
		},
		{
			name:  "creates every directory in order",
			files: fsx.MemFS{},
			args:  []string{"one", "two"},
			want:  map[string]fs.FileMode{"one": 0o777, "two": 0o777},
		},
		{
			name:  "creates a directory inside an existing one",
			files: fsx.MemFS{"sub/keep.txt": {Data: []byte("keep")}},
			args:  []string{"sub/d"},
			want:  map[string]fs.FileMode{"sub/d": 0o777},
		},
		{
			name:  "the umask applies to the default mode",
			files: fsx.MemFS{},
			umask: 0o022,
			args:  []string{"d"},
			want:  map[string]fs.FileMode{"d": 0o755},
		},
		{
			name:  "a trailing slash is dropped",
			files: fsx.MemFS{},
			args:  []string{"d/"},
			want:  map[string]fs.FileMode{"d": 0o777},
		},
		{
			name:  "redundant separators are dropped",
			files: fsx.MemFS{"a/keep.txt": {Data: []byte("keep")}},
			args:  []string{"a//d"},
			want:  map[string]fs.FileMode{"a/d": 0o777},
		},
		{
			name:  "a directory named like an option after a double dash",
			files: fsx.MemFS{},
			args:  []string{"--", "-p"},
			want:  map[string]fs.FileMode{"-p": 0o777},
		},
		{
			name:  "a directory named with a dash",
			files: fsx.MemFS{},
			args:  []string{"-"},
			want:  map[string]fs.FileMode{"-": 0o777},
		},
		{
			name:     "an existing directory is an error",
			files:    fsx.MemFS{"d": {Mode: fs.ModeDir}},
			args:     []string{"d"},
			wantErr:  "mkdir: d: file already exists\n",
			wantExit: 1,
		},
		{
			name:     "an existing file is an error",
			files:    fsx.MemFS{"file.txt": {Data: []byte("hello")}},
			args:     []string{"file.txt"},
			present:  []string{"file.txt"},
			wantErr:  "mkdir: file.txt: file already exists\n",
			wantExit: 1,
		},
		{
			name:     "a missing parent is an error",
			files:    fsx.MemFS{},
			args:     []string{"a/b"},
			absent:   []string{"a"},
			wantErr:  "mkdir: a/b: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a parent that is a file is an error",
			files:    fsx.MemFS{"file.txt": {Data: []byte("hello")}},
			args:     []string{"file.txt/d"},
			wantErr:  "mkdir: file.txt/d: not a directory\n",
			wantExit: 1,
		},
		{
			name:     "the root already exists",
			files:    fsx.MemFS{},
			args:     []string{"."},
			wantErr:  "mkdir: .: file already exists\n",
			wantExit: 1,
		},
		{
			name:     "keeps creating the other directories after an error",
			files:    fsx.MemFS{},
			args:     []string{"missing/d", "ok"},
			want:     map[string]fs.FileMode{"ok": 0o777},
			wantErr:  "mkdir: missing/d: file does not exist\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunMode(t *testing.T) {
	tests := []testCase{
		{
			name:  "sets the octal mode",
			files: fsx.MemFS{},
			args:  []string{"-m", "0770", "d"},
			want:  map[string]fs.FileMode{"d": 0o770},
		},
		{
			name:  "takes the mode glued to the option",
			files: fsx.MemFS{},
			args:  []string{"-m0770", "d"},
			want:  map[string]fs.FileMode{"d": 0o770},
		},
		{
			name:  "ignores the umask",
			files: fsx.MemFS{},
			umask: 0o022,
			args:  []string{"-m", "0770", "d"},
			want:  map[string]fs.FileMode{"d": 0o770},
		},
		{
			name:  "symbolic modes are relative to a=rwx",
			files: fsx.MemFS{},
			args:  []string{"-m", "o-rwx", "d"},
			want:  map[string]fs.FileMode{"d": 0o770},
		},
		{
			name:  "symbolic modes assign a single class",
			files: fsx.MemFS{},
			args:  []string{"-m", "g=rx", "d"},
			want:  map[string]fs.FileMode{"d": 0o757},
		},
		{
			name:  "the umask applies to symbolic modes without a class",
			files: fsx.MemFS{},
			umask: 0o022,
			args:  []string{"-m", "=rwx", "d"},
			want:  map[string]fs.FileMode{"d": 0o755},
		},
		{
			name:  "sets the mode of every directory",
			files: fsx.MemFS{},
			args:  []string{"-m", "0700", "one", "two"},
			want:  map[string]fs.FileMode{"one": 0o700, "two": 0o700},
		},
		{
			name:  "the last m wins",
			files: fsx.MemFS{},
			args:  []string{"-m", "0777", "-m", "0700", "d"},
			want:  map[string]fs.FileMode{"d": 0o700},
		},
		{
			name:  "sets the special bits",
			files: fsx.MemFS{},
			args:  []string{"-m", "g+s", "d"},
			want:  map[string]fs.FileMode{"d": fs.ModeSetgid | 0o777},
		},
		{
			name:     "an invalid mode is an error and creates nothing",
			files:    fsx.MemFS{},
			args:     []string{"-m", "garbage", "d"},
			absent:   []string{"d"},
			wantErr:  `mkdir: invalid mode: "garbage"` + "\n",
			wantExit: 1,
		},
		{
			name:     "the option requires an argument",
			files:    fsx.MemFS{},
			args:     []string{"-m"},
			wantErr:  "mkdir: option requires an argument -- 'm'\n",
			wantExit: 2,
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
			name:  "creates missing parents",
			files: fsx.MemFS{},
			umask: 0o022,
			args:  []string{"-p", "a/b/c"},
			want:  map[string]fs.FileMode{"a": 0o755, "a/b": 0o755, "a/b/c": 0o755},
		},
		{
			name:  "parents get the default mode modified by u+wx",
			files: fsx.MemFS{},
			umask: 0o222,
			args:  []string{"-p", "a/b"},
			want:  map[string]fs.FileMode{"a": 0o755, "a/b": 0o555},
		},
		{
			name:    "existing directories are ignored without error",
			files:   fsx.MemFS{"a/keep.txt": {Data: []byte("keep")}},
			args:    []string{"-p", "a"},
			present: []string{"a/keep.txt"},
		},
		{
			name:  "creating an already existing directory is idempotent",
			files: fsx.MemFS{"a/b": {Mode: fs.ModeDir}},
			args:  []string{"-p", "a/b"},
			want:  map[string]fs.FileMode{"a/b": 0},
		},
		{
			name:  "the root already exists",
			files: fsx.MemFS{},
			args:  []string{"-p", "."},
		},
		{
			name:  "an empty operand does nothing",
			files: fsx.MemFS{},
			args:  []string{"-p", ""},
		},
		{
			name:  "a trailing slash is dropped",
			files: fsx.MemFS{},
			args:  []string{"-p", "a/b/"},
			want:  map[string]fs.FileMode{"a": 0o777, "a/b": 0o777},
		},
		{
			name:  "m applies to the given directory but not to parents",
			files: fsx.MemFS{},
			umask: 0o022,
			args:  []string{"-pm", "0700", "a/b"},
			want:  map[string]fs.FileMode{"a": 0o755, "a/b": 0o700},
		},
		{
			name:  "m does not apply to an existing directory",
			files: fsx.MemFS{"d": {Mode: fs.ModeDir | 0o755}},
			args:  []string{"-pm", "0700", "d"},
			want:  map[string]fs.FileMode{"d": 0o755},
		},
		{
			name:     "an existing file is an error",
			files:    fsx.MemFS{"file.txt": {Data: []byte("hello")}},
			args:     []string{"-p", "file.txt"},
			present:  []string{"file.txt"},
			wantErr:  "mkdir: file.txt: file already exists\n",
			wantExit: 1,
		},
		{
			name:     "an intermediate component that is a file is an error",
			files:    fsx.MemFS{"file.txt": {Data: []byte("hello")}},
			args:     []string{"-p", "file.txt/d"},
			wantErr:  "mkdir: file.txt/d: not a directory\n",
			wantExit: 1,
		},
		{
			name:     "keeps creating the other directories after an error",
			files:    fsx.MemFS{"file.txt": {Data: []byte("hello")}},
			args:     []string{"-p", "file.txt/d", "ok"},
			want:     map[string]fs.FileMode{"ok": 0o777},
			wantErr:  "mkdir: file.txt/d: not a directory\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLongOptions(t *testing.T) {
	tests := []testCase{
		{
			name:  "parents creates missing parents",
			files: fsx.MemFS{},
			args:  []string{"--parents", "a/b/c"},
			want:  map[string]fs.FileMode{"a": 0o777, "a/b": 0o777, "a/b/c": 0o777},
		},
		{
			name:  "mode sets the mode",
			files: fsx.MemFS{},
			args:  []string{"--mode", "0770", "d"},
			want:  map[string]fs.FileMode{"d": 0o770},
		},
		{
			name:  "mode takes its value glued to it",
			files: fsx.MemFS{},
			args:  []string{"--mode=0770", "d"},
			want:  map[string]fs.FileMode{"d": 0o770},
		},
		{
			name:     "mode requires an argument",
			files:    fsx.MemFS{},
			args:     []string{"--mode"},
			wantErr:  "mkdir: option '--mode' requires an argument\n",
			wantExit: 2,
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
			wantErr:  "mkdir: missing operand\n",
			wantExit: 2,
		},
		{
			name:     "invalid option",
			files:    fsx.MemFS{},
			args:     []string{"-x"},
			wantErr:  "mkdir: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option",
			files:    fsx.MemFS{},
			args:     []string{"--bogus", "d"},
			absent:   []string{"d"},
			wantErr:  "mkdir: unrecognized option '--bogus'\n",
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
