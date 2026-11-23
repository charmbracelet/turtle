package mkdir

import (
	"bytes"
	"io/fs"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

type testCase struct {
	name     string
	files    fsx.MemoryFS
	umask    fs.FileMode
	args     []string
	want     map[string]fs.FileMode
	present  []string
	absent   []string
	wantOut  string
	wantErr  string
	wantExit int
}

// runCase runs one case, checking the modes of the directories it creates
// as permissions, with the directory bit added.
func runCase(t *testing.T, test testCase) {
	t.Helper()

	testutil.Once(t, New, testutil.Case{
		Name:     test.name,
		FS:       test.files,
		Umask:    test.umask,
		Args:     test.args,
		Want:     test.wantOut,
		WantErr:  test.wantErr,
		WantExit: test.wantExit,
		Present:  test.present,
		Absent:   test.absent,
	})

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
}

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			name:  "creates a directory",
			files: fsx.MemoryFS{},
			args:  []string{"d"},
			want:  map[string]fs.FileMode{"d": 0o777},
		},
		{
			name:  "creates every directory in order",
			files: fsx.MemoryFS{},
			args:  []string{"one", "two"},
			want:  map[string]fs.FileMode{"one": 0o777, "two": 0o777},
		},
		{
			name:  "creates a directory inside an existing one",
			files: fsx.MemoryFS{"sub/keep.txt": {Data: []byte("keep")}},
			args:  []string{"sub/d"},
			want:  map[string]fs.FileMode{"sub/d": 0o777},
		},
		{
			name:  "the umask applies to the default mode",
			files: fsx.MemoryFS{},
			umask: 0o022,
			args:  []string{"d"},
			want:  map[string]fs.FileMode{"d": 0o755},
		},
		{
			name:  "a trailing slash is dropped",
			files: fsx.MemoryFS{},
			args:  []string{"d/"},
			want:  map[string]fs.FileMode{"d": 0o777},
		},
		{
			name:  "redundant separators are dropped",
			files: fsx.MemoryFS{"a/keep.txt": {Data: []byte("keep")}},
			args:  []string{"a//d"},
			want:  map[string]fs.FileMode{"a/d": 0o777},
		},
		{
			name:  "a directory named like an option after a double dash",
			files: fsx.MemoryFS{},
			args:  []string{"--", "-p"},
			want:  map[string]fs.FileMode{"-p": 0o777},
		},
		{
			name:  "a directory named with a dash",
			files: fsx.MemoryFS{},
			args:  []string{"-"},
			want:  map[string]fs.FileMode{"-": 0o777},
		},
		{
			name:     "an existing directory is an error",
			files:    fsx.MemoryFS{"d": {Mode: fs.ModeDir}},
			args:     []string{"d"},
			wantErr:  "mkdir: d: file already exists\n",
			wantExit: 1,
		},
		{
			name:     "an existing file is an error",
			files:    fsx.MemoryFS{"file.txt": {Data: []byte("hello")}},
			args:     []string{"file.txt"},
			present:  []string{"file.txt"},
			wantErr:  "mkdir: file.txt: file already exists\n",
			wantExit: 1,
		},
		{
			name:     "a missing parent is an error",
			files:    fsx.MemoryFS{},
			args:     []string{"a/b"},
			absent:   []string{"a"},
			wantErr:  "mkdir: a/b: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a parent that is a file is an error",
			files:    fsx.MemoryFS{"file.txt": {Data: []byte("hello")}},
			args:     []string{"file.txt/d"},
			wantErr:  "mkdir: file.txt/d: not a directory\n",
			wantExit: 1,
		},
		{
			name:     "the root already exists",
			files:    fsx.MemoryFS{},
			args:     []string{"."},
			wantErr:  "mkdir: .: file already exists\n",
			wantExit: 1,
		},
		{
			name:     "keeps creating the other directories after an error",
			files:    fsx.MemoryFS{},
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
			files: fsx.MemoryFS{},
			args:  []string{"-m", "0770", "d"},
			want:  map[string]fs.FileMode{"d": 0o770},
		},
		{
			name:  "takes the mode glued to the option",
			files: fsx.MemoryFS{},
			args:  []string{"-m0770", "d"},
			want:  map[string]fs.FileMode{"d": 0o770},
		},
		{
			name:  "ignores the umask",
			files: fsx.MemoryFS{},
			umask: 0o022,
			args:  []string{"-m", "0770", "d"},
			want:  map[string]fs.FileMode{"d": 0o770},
		},
		{
			name:  "symbolic modes are relative to a=rwx",
			files: fsx.MemoryFS{},
			args:  []string{"-m", "o-rwx", "d"},
			want:  map[string]fs.FileMode{"d": 0o770},
		},
		{
			name:  "symbolic modes assign a single class",
			files: fsx.MemoryFS{},
			args:  []string{"-m", "g=rx", "d"},
			want:  map[string]fs.FileMode{"d": 0o757},
		},
		{
			name:  "the umask applies to symbolic modes without a class",
			files: fsx.MemoryFS{},
			umask: 0o022,
			args:  []string{"-m", "=rwx", "d"},
			want:  map[string]fs.FileMode{"d": 0o755},
		},
		{
			name:  "sets the mode of every directory",
			files: fsx.MemoryFS{},
			args:  []string{"-m", "0700", "one", "two"},
			want:  map[string]fs.FileMode{"one": 0o700, "two": 0o700},
		},
		{
			name:  "the last m wins",
			files: fsx.MemoryFS{},
			args:  []string{"-m", "0777", "-m", "0700", "d"},
			want:  map[string]fs.FileMode{"d": 0o700},
		},
		{
			name:  "sets the special bits",
			files: fsx.MemoryFS{},
			args:  []string{"-m", "g+s", "d"},
			want:  map[string]fs.FileMode{"d": fs.ModeSetgid | 0o777},
		},
		{
			name:     "an invalid mode is an error and creates nothing",
			files:    fsx.MemoryFS{},
			args:     []string{"-m", "garbage", "d"},
			absent:   []string{"d"},
			wantErr:  `mkdir: invalid mode: "garbage"` + "\nTry 'mkdir --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "the option requires an argument",
			files:    fsx.MemoryFS{},
			args:     []string{"-m"},
			wantErr:  "mkdir: option requires an argument -- 'm'\nTry 'mkdir --help' for more information.\n",
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
			files: fsx.MemoryFS{},
			umask: 0o022,
			args:  []string{"-p", "a/b/c"},
			want:  map[string]fs.FileMode{"a": 0o755, "a/b": 0o755, "a/b/c": 0o755},
		},
		{
			name:  "parents get the default mode modified by u+wx",
			files: fsx.MemoryFS{},
			umask: 0o222,
			args:  []string{"-p", "a/b"},
			want:  map[string]fs.FileMode{"a": 0o755, "a/b": 0o555},
		},
		{
			name:    "existing directories are ignored without error",
			files:   fsx.MemoryFS{"a/keep.txt": {Data: []byte("keep")}},
			args:    []string{"-p", "a"},
			present: []string{"a/keep.txt"},
		},
		{
			name:  "creating an already existing directory is idempotent",
			files: fsx.MemoryFS{"a/b": {Mode: fs.ModeDir}},
			args:  []string{"-p", "a/b"},
			want:  map[string]fs.FileMode{"a/b": 0},
		},
		{
			name:  "the root already exists",
			files: fsx.MemoryFS{},
			args:  []string{"-p", "."},
		},
		{
			name:  "an empty operand does nothing",
			files: fsx.MemoryFS{},
			args:  []string{"-p", ""},
		},
		{
			name:  "a trailing slash is dropped",
			files: fsx.MemoryFS{},
			args:  []string{"-p", "a/b/"},
			want:  map[string]fs.FileMode{"a": 0o777, "a/b": 0o777},
		},
		{
			name:  "m applies to the given directory but not to parents",
			files: fsx.MemoryFS{},
			umask: 0o022,
			args:  []string{"-pm", "0700", "a/b"},
			want:  map[string]fs.FileMode{"a": 0o755, "a/b": 0o700},
		},
		{
			name:  "m does not apply to an existing directory",
			files: fsx.MemoryFS{"d": {Mode: fs.ModeDir | 0o755}},
			args:  []string{"-pm", "0700", "d"},
			want:  map[string]fs.FileMode{"d": 0o755},
		},
		{
			name:     "an existing file is an error",
			files:    fsx.MemoryFS{"file.txt": {Data: []byte("hello")}},
			args:     []string{"-p", "file.txt"},
			present:  []string{"file.txt"},
			wantErr:  "mkdir: file.txt: file already exists\n",
			wantExit: 1,
		},
		{
			name:     "an intermediate component that is a file is an error",
			files:    fsx.MemoryFS{"file.txt": {Data: []byte("hello")}},
			args:     []string{"-p", "file.txt/d"},
			wantErr:  "mkdir: file.txt/d: not a directory\n",
			wantExit: 1,
		},
		{
			name:     "keeps creating the other directories after an error",
			files:    fsx.MemoryFS{"file.txt": {Data: []byte("hello")}},
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
			files: fsx.MemoryFS{},
			args:  []string{"--parents", "a/b/c"},
			want:  map[string]fs.FileMode{"a": 0o777, "a/b": 0o777, "a/b/c": 0o777},
		},
		{
			name:  "mode sets the mode",
			files: fsx.MemoryFS{},
			args:  []string{"--mode", "0770", "d"},
			want:  map[string]fs.FileMode{"d": 0o770},
		},
		{
			name:  "mode takes its value glued to it",
			files: fsx.MemoryFS{},
			args:  []string{"--mode=0770", "d"},
			want:  map[string]fs.FileMode{"d": 0o770},
		},
		{
			name:     "mode requires an argument",
			files:    fsx.MemoryFS{},
			args:     []string{"--mode"},
			wantErr:  "mkdir: option '--mode' requires an argument\nTry 'mkdir --help' for more information.\n",
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
			wantErr:  "mkdir: missing operand\nTry 'mkdir --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "invalid option",
			files:    fsx.MemoryFS{},
			args:     []string{"-x"},
			wantErr:  "mkdir: invalid option -- 'x'\nTry 'mkdir --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option",
			files:    fsx.MemoryFS{},
			args:     []string{"--bogus", "d"},
			absent:   []string{"d"},
			wantErr:  "mkdir: unrecognized option '--bogus'\nTry 'mkdir --help' for more information.\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunVerbose(t *testing.T) {
	tests := []testCase{
		{
			name:    "prints a message for each directory created",
			files:   fsx.MemoryFS{},
			args:    []string{"-v", "one", "two"},
			want:    map[string]fs.FileMode{"one": 0o777, "two": 0o777},
			wantOut: "mkdir: created directory 'one'\nmkdir: created directory 'two'\n",
		},
		{
			name:    "prints a message for every parent created",
			files:   fsx.MemoryFS{},
			umask:   0o022,
			args:    []string{"-vp", "a/b/c"},
			want:    map[string]fs.FileMode{"a": 0o755, "a/b": 0o755, "a/b/c": 0o755},
			wantOut: "mkdir: created directory 'a'\nmkdir: created directory 'a/b'\nmkdir: created directory 'a/b/c'\n",
		},
		{
			name:    "prints nothing for existing directories",
			files:   fsx.MemoryFS{"a/b": {Mode: fs.ModeDir}},
			args:    []string{"--verbose", "--parents", "a/b"},
			want:    map[string]fs.FileMode{"a/b": 0},
			wantOut: "",
		},
		{
			name:     "prints a message even when another directory fails",
			files:    fsx.MemoryFS{},
			args:     []string{"-v", "ok", "missing/d"},
			want:     map[string]fs.FileMode{"ok": 0o777},
			wantOut:  "mkdir: created directory 'ok'\n",
			wantErr:  "mkdir: missing/d: file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "the message shows the cleaned path",
			files:   fsx.MemoryFS{"a/keep.txt": {Data: []byte("keep")}},
			args:    []string{"-v", "a//d/"},
			want:    map[string]fs.FileMode{"a/d": 0o777},
			wantOut: "mkdir: created directory 'a/d'\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunVerboseWorkDir(t *testing.T) {
	files := fsx.MemoryFS{"base": {Mode: fs.ModeDir | 0o755}}
	stdout := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: stdout, FS: files, WorkingDir: "base"}

	err := New(rt).Run(t.Context(), "-vp", "a/b")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "mkdir: created directory 'a'\nmkdir: created directory 'a/b'\n"
	if got := stdout.String(); got != want {
		t.Errorf("Run() stdout = %q, want %q", got, want)
	}
	for _, name := range []string{"base/a", "base/a/b"} {
		if _, err := fs.Stat(files, name); err != nil {
			t.Errorf("Stat(%q) error = %v", err, err)
		}
	}
}

func TestRunContext(t *testing.T) {
	testutil.Cancelled(t, New, nil, fsx.MemoryFS{}, "d")
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "mkdir", usage)
}
