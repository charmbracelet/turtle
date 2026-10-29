package test

import (
	"bytes"
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

var (
	oldTime = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
)

// testFS describes the files most of the tests evaluate primaries against.
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"file":     {Data: []byte("contents"), Mode: 0o644},
		"empty":    {Mode: 0o644},
		"dir":      {Mode: fs.ModeDir | 0o755},
		"noread":   {Mode: 0o200},
		"nowrite":  {Mode: 0o444},
		"noexec":   {Mode: 0o644},
		"exec":     {Mode: 0o755},
		"setuid":   {Mode: fs.ModeSetuid | 0o755},
		"setgid":   {Mode: fs.ModeSetgid | 0o755},
		"block":    {Mode: fs.ModeDevice | 0o644},
		"chardev":  {Mode: fs.ModeDevice | fs.ModeCharDevice | 0o644},
		"fifo":     {Mode: fs.ModeNamedPipe | 0o644},
		"socket":   {Mode: fs.ModeSocket | 0o644},
		"symlink":  {Data: []byte("file"), Mode: fs.ModeSymlink},
		"dirlink":  {Data: []byte("dir"), Mode: fs.ModeSymlink},
		"dangling": {Data: []byte("missing"), Mode: fs.ModeSymlink},
		"older":    {Data: []byte("x"), Mode: 0o644, ModTime: oldTime},
		"newer":    {Data: []byte("x"), Mode: 0o644, ModTime: newTime},
	}
}

type testCase struct {
	name       string
	files      fsx.MemFS
	wd         string
	setup      func(t *testing.T, fsys fsx.MemFS)
	args       []string
	want       bool
	wantErr    string
	stdout     io.Writer
	isTerminal func(fd int) bool
}

func runTest(t *testing.T, test testCase) {
	t.Helper()

	files := test.files
	if files == nil {
		files = testFS()
	}
	if test.setup != nil {
		test.setup(t, files)
	}

	stderr := new(bytes.Buffer)
	stdout := io.Discard
	if test.stdout != nil {
		stdout = test.stdout
	}
	rt := &coreutils.Runtime{
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         files,
		WorkingDir: test.wd,
		IsTerminal: test.isTerminal,
	}

	err := New(rt).Run(t.Context(), test.args...)

	wantExit := 0
	wantStderr := ""
	switch {
	case test.wantErr != "":
		wantExit = 2
		wantStderr = "test: " + test.wantErr + "\n"
	case !test.want:
		wantExit = 1
	}
	if code := exitCode(t, err); code != wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, wantExit, err)
	}
	if got := stderr.String(); got != wantStderr {
		t.Errorf("Run() stderr = %q, want %q", got, wantStderr)
	}
}

func exitCode(t *testing.T, err error) int {
	t.Helper()

	if err == nil {
		return 0
	}
	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok {
		t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
	}
	return exitErr.Code
}

// link returns a setup that makes newName a link to oldName.
func link(oldName, newName string) func(t *testing.T, fsys fsx.MemFS) {
	return func(t *testing.T, fsys fsx.MemFS) {
		t.Helper()
		if err := fsys.Link(oldName, newName); err != nil {
			t.Fatalf("Link(%q, %q) error = %v", oldName, newName, err)
		}
	}
}

func TestRunArgumentCounts(t *testing.T) {
	tests := []testCase{
		{name: "no arguments is false", args: nil, want: false},
		{name: "a non-empty string is true", args: []string{"foo"}, want: true},
		{name: "an empty string is false", args: []string{""}, want: false},
		{name: "a lone operator is a non-empty string", args: []string{"!"}, want: true},
		{name: "a right bracket is a non-empty string", args: []string{"]"}, want: true},
		{name: "negation of a non-empty string", args: []string{"!", "foo"}, want: false},
		{name: "negation of an empty string", args: []string{"!", ""}, want: true},
		{name: "negation of a right bracket is false", args: []string{"!", "]"}, want: false},
		{
			name:    "two operands without an operator",
			args:    []string{"foo", "bar"},
			wantErr: `unexpected argument "bar"`,
		},
		{name: "binary primary between two operands", args: []string{"foo", "=", "foo"}, want: true},
		{name: "binary primary with the == synonym", args: []string{"foo", "==", "foo"}, want: true},
		{name: "negation of a three-argument == test", args: []string{"!", "foo", "==", "bar"}, want: true},
		{name: "negation of a two-argument test", args: []string{"!", "-z", ""}, want: false},
		{name: "double negation", args: []string{"!", "!", "foo"}, want: true},
		{name: "parenthesized single argument", args: []string{"(", "foo", ")"}, want: true},
		{name: "parenthesized empty string", args: []string{"(", "", ")"}, want: false},
		{name: "negation of a parenthesized argument", args: []string{"!", "(", "foo", ")"}, want: false},
		{name: "negation of a three-argument test", args: []string{"!", "foo", "=", "bar"}, want: true},
		{name: "obsolescent parenthesized unary primary", args: []string{"(", "-f", "file", ")"}, want: true},
		{name: "obsolescent parenthesized unary, missing file", args: []string{"(", "-f", "missing", ")"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunStringPrimaries(t *testing.T) {
	tests := []testCase{
		{name: "n on a non-empty string", args: []string{"-n", "foo"}, want: true},
		{name: "n on an empty string", args: []string{"-n", ""}, want: false},
		{name: "z on an empty string", args: []string{"-z", ""}, want: true},
		{name: "z on a non-empty string", args: []string{"-z", "foo"}, want: false},
		{name: "equal strings", args: []string{"foo", "=", "foo"}, want: true},
		{name: "different strings", args: []string{"foo", "=", "bar"}, want: false},
		{name: "equal strings with the == synonym", args: []string{"foo", "==", "foo"}, want: true},
		{name: "different strings with the == synonym", args: []string{"foo", "==", "bar"}, want: false},
		{name: "empty strings with the == synonym", args: []string{"", "==", ""}, want: true},
		{name: "non-equal strings", args: []string{"foo", "!=", "bar"}, want: true},
		{name: "non-equal equal strings", args: []string{"foo", "!=", "foo"}, want: false},
		{name: "collates after", args: []string{"b", ">", "a"}, want: true},
		{name: "does not collate after", args: []string{"a", ">", "b"}, want: false},
		{name: "collates before", args: []string{"a", "<", "b"}, want: true},
		{name: "does not collate before", args: []string{"b", "<", "a"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunNumericPrimaries(t *testing.T) {
	tests := []testCase{
		{name: "eq", args: []string{"1", "-eq", "1"}, want: true},
		{name: "eq false", args: []string{"1", "-eq", "2"}, want: false},
		{name: "ne", args: []string{"1", "-ne", "2"}, want: true},
		{name: "ne false", args: []string{"1", "-ne", "1"}, want: false},
		{name: "gt", args: []string{"3", "-gt", "2"}, want: true},
		{name: "gt false", args: []string{"2", "-gt", "2"}, want: false},
		{name: "ge", args: []string{"2", "-ge", "2"}, want: true},
		{name: "ge false", args: []string{"1", "-ge", "2"}, want: false},
		{name: "lt", args: []string{"1", "-lt", "2"}, want: true},
		{name: "lt false", args: []string{"2", "-lt", "2"}, want: false},
		{name: "le", args: []string{"2", "-le", "2"}, want: true},
		{name: "le false", args: []string{"3", "-le", "2"}, want: false},
		{name: "negative integers", args: []string{"-1", "-gt", "-2"}, want: true},
		{
			name:    "a leading blank",
			args:    []string{" 1", "-eq", "1"},
			wantErr: `integer expression expected before "-eq"`,
		},
		{
			name:    "a trailing blank",
			args:    []string{"1 ", "-eq", "1"},
			wantErr: `integer expression expected before "-eq"`,
		},
		{
			name:    "a leading plus sign",
			args:    []string{"+5", "-eq", "5"},
			wantErr: `integer expression expected before "-eq"`,
		},
		{
			name:    "a non-integer operand",
			args:    []string{"1", "-eq", "x"},
			wantErr: `integer expression expected before "-eq"`,
		},
		{
			name:    "a hexadecimal operand",
			args:    []string{"0x10", "-eq", "1"},
			wantErr: `integer expression expected before "-eq"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunFilePrimaries(t *testing.T) {
	tests := []testCase{
		{name: "b on a block special file", args: []string{"-b", "block"}, want: true},
		{name: "b on a regular file", args: []string{"-b", "file"}, want: false},
		{name: "c on a character special file", args: []string{"-c", "chardev"}, want: true},
		{name: "c on a block special file", args: []string{"-c", "block"}, want: false},
		{name: "d on a directory", args: []string{"-d", "dir"}, want: true},
		{name: "d on a regular file", args: []string{"-d", "file"}, want: false},
		{name: "e on an existing file", args: []string{"-e", "file"}, want: true},
		{name: "e on a missing file", args: []string{"-e", "missing"}, want: false},
		{name: "f on a regular file", args: []string{"-f", "file"}, want: true},
		{name: "f on a directory", args: []string{"-f", "dir"}, want: false},
		{name: "g on a set-group-ID file", args: []string{"-g", "setgid"}, want: true},
		{name: "g on a regular file", args: []string{"-g", "file"}, want: false},
		{name: "p on a FIFO", args: []string{"-p", "fifo"}, want: true},
		{name: "p on a regular file", args: []string{"-p", "file"}, want: false},
		{name: "S on a socket", args: []string{"-S", "socket"}, want: true},
		{name: "S on a regular file", args: []string{"-S", "file"}, want: false},
		{name: "r on a readable file", args: []string{"-r", "file"}, want: true},
		{name: "r on a non-readable file", args: []string{"-r", "noread"}, want: false},
		{name: "r on a missing file", args: []string{"-r", "missing"}, want: false},
		{name: "s on a non-empty file", args: []string{"-s", "file"}, want: true},
		{name: "s on an empty file", args: []string{"-s", "empty"}, want: false},
		{name: "s on a missing file", args: []string{"-s", "missing"}, want: false},
		{name: "t on standard input", args: []string{"-t", "0"}, want: false},
		{name: "t on standard output", args: []string{"-t", "1"}, want: false},
		{name: "t on a non-number", args: []string{"-t", "x"}, want: false},
		{name: "u on a set-user-ID file", args: []string{"-u", "setuid"}, want: true},
		{name: "u on a regular file", args: []string{"-u", "file"}, want: false},
		{name: "w on a writable file", args: []string{"-w", "file"}, want: true},
		{name: "w on a non-writable file", args: []string{"-w", "nowrite"}, want: false},
		{name: "x on an executable file", args: []string{"-x", "exec"}, want: true},
		{name: "x on a non-executable file", args: []string{"-x", "noexec"}, want: false},
		{name: "x on a searchable directory", args: []string{"-x", "dir"}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunTerminal(t *testing.T) {
	t.Run("honors the IsTerminal function", func(t *testing.T) {
		stub := func(fd int) bool { return fd == 1 }
		tests := []testCase{
			{name: "a terminal descriptor", args: []string{"-t", "1"}, want: true, isTerminal: stub},
			{name: "a non-terminal descriptor", args: []string{"-t", "0"}, want: false, isTerminal: stub},
			{name: "a descriptor without a stream", args: []string{"-t", "3"}, want: false, isTerminal: stub},
			{name: "a non-number", args: []string{"-t", "x"}, want: false, isTerminal: func(int) bool { return true }},
			{name: "a negative descriptor", args: []string{"-t", "-1"}, want: false, isTerminal: func(int) bool { return true }},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runTest(t, test)
			})
		}
	})

	t.Run("checks the standard streams by default", func(t *testing.T) {
		t.Run("a character device file is not a terminal", func(t *testing.T) {
			f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { f.Close() })
			runTest(t, testCase{name: "-t 1", args: []string{"-t", "1"}, want: false, stdout: f})
		})

		t.Run("a regular file is not a terminal", func(t *testing.T) {
			f, err := os.Create(filepath.Join(t.TempDir(), "out"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { f.Close() })
			runTest(t, testCase{name: "-t 1", args: []string{"-t", "1"}, want: false, stdout: f})
		})

		t.Run("the process standard output is not a terminal", func(t *testing.T) {
			// The go tool pipes the standard output of the test binary.
			runTest(t, testCase{name: "-t 1", args: []string{"-t", "1"}, want: false, stdout: os.Stdout})
		})

		t.Run("a stream that is not a file", func(t *testing.T) {
			runTest(t, testCase{name: "-t 1", args: []string{"-t", "1"}, want: false, stdout: new(bytes.Buffer)})
		})

		t.Run("a real terminal when the process has one", func(t *testing.T) {
			f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
			if err != nil {
				t.Skipf("opening /dev/tty: %v", err)
			}
			t.Cleanup(func() { f.Close() })
			runTest(t, testCase{name: "-t 1", args: []string{"-t", "1"}, want: true, stdout: f})
		})
	})
}

func TestRunSymbolicLinks(t *testing.T) {
	tests := []testCase{
		{name: "e follows a symbolic link", args: []string{"-e", "symlink"}, want: true},
		{name: "e on a dangling symbolic link", args: []string{"-e", "dangling"}, want: false},
		{name: "f follows a symbolic link to a regular file", args: []string{"-f", "symlink"}, want: true},
		{name: "f on a dangling symbolic link", args: []string{"-f", "dangling"}, want: false},
		{name: "d follows a symbolic link to a directory", args: []string{"-d", "dirlink"}, want: true},
		{name: "h on a symbolic link", args: []string{"-h", "symlink"}, want: true},
		{name: "h on a dangling symbolic link", args: []string{"-h", "dangling"}, want: true},
		{name: "h on a regular file", args: []string{"-h", "file"}, want: false},
		{name: "L on a symbolic link", args: []string{"-L", "symlink"}, want: true},
		{name: "L on a dangling symbolic link", args: []string{"-L", "dangling"}, want: true},
		{name: "L on a regular file", args: []string{"-L", "file"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunFileComparisons(t *testing.T) {
	tests := []testCase{
		{name: "nt on a newer file", args: []string{"newer", "-nt", "older"}, want: true},
		{name: "nt on an older file", args: []string{"older", "-nt", "newer"}, want: false},
		{name: "nt with equal times", args: []string{"file", "-nt", "file"}, want: false},
		{name: "nt against a missing file", args: []string{"file", "-nt", "missing"}, want: true},
		{name: "nt on a missing file", args: []string{"missing", "-nt", "file"}, want: false},
		{name: "ot on an older file", args: []string{"older", "-ot", "newer"}, want: true},
		{name: "ot on a newer file", args: []string{"newer", "-ot", "older"}, want: false},
		{name: "ot against a missing file", args: []string{"file", "-ot", "missing"}, want: false},
		{name: "ot on a missing file", args: []string{"missing", "-ot", "file"}, want: true},
		{name: "ef on the same file", args: []string{"file", "-ef", "file"}, want: true},
		{name: "ef on two links to the same file", args: []string{"file", "-ef", "hard"}, want: true, setup: link("file", "hard")},
		{name: "ef follows symbolic links", args: []string{"symlink", "-ef", "file"}, want: true},
		{name: "ef on two different files", args: []string{"file", "-ef", "empty"}, want: false},
		{name: "ef against a missing file", args: []string{"file", "-ef", "missing"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOperators(t *testing.T) {
	tests := []testCase{
		{name: "negation", args: []string{"!", "-f", "file"}, want: false},
		{name: "negation of a false primary", args: []string{"!", "-f", "missing"}, want: true},
		{name: "and", args: []string{"-f", "file", "-a", "-f", "empty"}, want: true},
		{name: "and with a false term", args: []string{"-f", "file", "-a", "-f", "missing"}, want: false},
		{name: "or", args: []string{"-f", "missing", "-o", "-d", "dir"}, want: true},
		{name: "or with two false terms", args: []string{"-f", "missing", "-o", "-f", "missing"}, want: false},
		{name: "and binds tighter than or", args: []string{"foo", "-o", "bar", "-a", ""}, want: true},
		{name: "negation binds tighter than and", args: []string{"!", "", "-a", "foo"}, want: true},
		{name: "negation of a binary primary", args: []string{"!", "1", "-gt", "2"}, want: true},
		{name: "grouping", args: []string{"(", "-f", "file", "-a", "-d", "dir", ")"}, want: true},
		{name: "grouping changes the result", args: []string{"(", "foo", "-o", "bar", ")", "-a", ""}, want: false},
		{name: "without grouping the result differs", args: []string{"foo", "-o", "bar", "-a", ""}, want: true},
		{name: "negation of a group", args: []string{"!", "(", "-f", "file", "-a", "-d", "dir", ")"}, want: false},
		{name: "nested groups", args: []string{"!", "(", "(", "", ")", "-o", "", ")"}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunSyntaxErrors(t *testing.T) {
	tests := []testCase{
		{
			name:    "three operands without an operator",
			args:    []string{"foo", "bar", "baz"},
			wantErr: `unexpected argument "bar"`,
		},
		{
			name:    "an operator without its right operand",
			args:    []string{"foo", "="},
			wantErr: `unexpected argument "="`,
		},
		{
			name:    "an extra operand after a binary primary",
			args:    []string{"foo", "=", "bar", "baz"},
			wantErr: `unexpected argument "baz"`,
		},
		{
			name:    "an integer operator as a string",
			args:    []string{"-eq", "1"},
			wantErr: `unexpected argument "1"`,
		},
		{
			name:    "missing closing parenthesis",
			args:    []string{"(", "foo"},
			wantErr: `')' expected`,
		},
		{
			name:    "empty parentheses",
			args:    []string{"(", ")"},
			wantErr: `')' expected`,
		},
		{
			name:    "an operator without a following term",
			args:    []string{"foo", "-a"},
			wantErr: "argument expected",
		},
		{
			name:    "a dangling or operator",
			args:    []string{"foo", "-o"},
			wantErr: "argument expected",
		},
		{
			name:    "a closing parenthesis without an opening one",
			args:    []string{"foo", ")"},
			wantErr: `unexpected argument ")"`,
		},
		{
			name:    "negation at the end of a group",
			args:    []string{"(", "!"},
			wantErr: "argument expected",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunWorkingDir(t *testing.T) {
	tests := []testCase{
		{
			name:  "resolves operands relative to the working directory",
			files: fsx.MemFS{"work/file": {Data: []byte("x"), Mode: 0o644}},
			wd:    "work",
			args:  []string{"-f", "file"},
			want:  true,
		},
		{
			name:  "a missing file below the working directory",
			files: fsx.MemFS{"work/file": {Data: []byte("x"), Mode: 0o644}},
			wd:    "work",
			args:  []string{"-f", "nope"},
			want:  false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunWindowsSeparators(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only")
	}

	runTest(t, testCase{
		name:  "backslashes are treated as separators",
		files: fsx.MemFS{"d/file": {Data: []byte("x"), Mode: 0o644}},
		args:  []string{"-f", `d\file`},
		want:  true,
	})
}

func TestRunNoFileSystem(t *testing.T) {
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stderr: stderr}

	err := New(rt).Run(t.Context(), "-f", "file")

	if code := exitCode(t, err); code != 2 {
		t.Errorf("Run() exit code = %d, want 2 (error = %v)", code, err)
	}
	if got, want := stderr.String(), "test: no file system available\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}
