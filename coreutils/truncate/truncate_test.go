package truncate

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

type testCase struct {
	name     string
	fsys     fsx.FS
	args     []string
	want     map[string]string
	wantErr  string
	wantExit int
}

// runCase runs a single test case, checking the exit status, the
// diagnostics and the resulting files.
func runCase(t *testing.T, test testCase) {
	t.Helper()

	fsys := test.fsys
	if fsys == nil {
		fsys = fsx.MemFS{}
	}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: new(bytes.Buffer),
		Stderr: stderr,
		FS:     fsys,
	}
	err := New(rt).Run(t.Context(), test.args...)

	code := 0
	if exitErr, ok := errors.AsType[*coreutils.ExitError](err); ok {
		code = exitErr.Code
	} else if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
	checkFiles(t, fsys, test.want)
}

// checkFiles checks that the file system holds exactly the wanted files,
// with the wanted contents.
func checkFiles(t *testing.T, fsys fsx.FS, want map[string]string) {
	t.Helper()

	mem := memOf(fsys)
	if mem == nil {
		return
	}
	var got []string
	for name := range mem {
		got = append(got, name)
	}
	slices.Sort(got)
	var wantNames []string
	for name := range want {
		wantNames = append(wantNames, name)
	}
	slices.Sort(wantNames)
	if strings.Join(got, ",") != strings.Join(wantNames, ",") {
		t.Errorf("Run() files = %v, want %v", got, wantNames)
	}
	for _, name := range wantNames {
		if mem[name].Mode.IsDir() {
			continue
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			t.Errorf("Run() wrote an unreadable %s: %v", name, err)
			continue
		}
		if string(data) != want[name] {
			t.Errorf("Run() %s = %q, want %q", name, data, want[name])
		}
	}
}

// memOf returns the memory file system behind fsys, or nil when it doesn't
// hold one to inspect.
func memOf(fsys fsx.FS) fsx.MemFS {
	switch f := fsys.(type) {
	case fsx.MemFS:
		return f
	case nonSeekableFS:
		return f.MemFS
	case failWriteFS:
		return f.MemFS
	}
	return nil
}

// file returns a memory file system holding one file with the given
// contents.
func file(name, data string) fsx.MemFS {
	return fsx.MemFS{name: {Data: []byte(data), Mode: 0o600}}
}

func TestRun(t *testing.T) {
	tests := []testCase{
		// Absolute sizes.
		{
			name:     "an absolute size shrinks a file",
			fsys:     file("f", "hello world"),
			args:     []string{"-s", "5", "f"},
			want:     map[string]string{"f": "hello"},
			wantExit: 0,
		},
		{
			name:     "an absolute size grows a file",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "5", "f"},
			want:     map[string]string{"f": "abc\x00\x00"},
			wantExit: 0,
		},
		{
			name:     "an absolute size of zero empties a file",
			fsys:     file("f", "hello"),
			args:     []string{"-s", "0", "f"},
			want:     map[string]string{"f": ""},
			wantExit: 0,
		},
		{
			name:     "an absolute size keeps a file of the same size",
			fsys:     file("f", "hello"),
			args:     []string{"-s", "5", "f"},
			want:     map[string]string{"f": "hello"},
			wantExit: 0,
		},
		{
			name:     "an absolute size of a missing file creates it",
			args:     []string{"-s", "3", "f"},
			want:     map[string]string{"f": "\x00\x00\x00"},
			wantExit: 0,
		},
		{
			name:     "an absolute size of zero creates an empty file",
			args:     []string{"-s", "0", "f"},
			want:     map[string]string{"f": ""},
			wantExit: 0,
		},

		// Relative sizes.
		{
			name:     "a plus size extends a file",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "+3", "f"},
			want:     map[string]string{"f": "abc\x00\x00\x00"},
			wantExit: 0,
		},
		{
			name:     "a plus size of zero keeps a file",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "+0", "f"},
			want:     map[string]string{"f": "abc"},
			wantExit: 0,
		},
		{
			name:     "a minus size reduces a file",
			fsys:     file("f", "abcdef"),
			args:     []string{"-s", "-2", "f"},
			want:     map[string]string{"f": "abcd"},
			wantExit: 0,
		},
		{
			name:     "a minus size past the file empties it",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "-10", "f"},
			want:     map[string]string{"f": ""},
			wantExit: 0,
		},
		{
			name:     "a plus size creates a missing file",
			args:     []string{"-s", "+2", "f"},
			want:     map[string]string{"f": "\x00\x00"},
			wantExit: 0,
		},
		{
			name:     "a minus size creates an empty missing file",
			args:     []string{"-s", "-2", "f"},
			want:     map[string]string{"f": ""},
			wantExit: 0,
		},
		{
			name:     "a minus size skips a missing file with -c",
			args:     []string{"-c", "-s", "-2", "f"},
			want:     map[string]string{},
			wantExit: 0,
		},
		{
			name:     "a plus size skips a missing file with -c",
			args:     []string{"-c", "-s", "+2", "f"},
			want:     map[string]string{},
			wantExit: 0,
		},

		// At-most and at-least sizes.
		{
			name:     "an at most size keeps a smaller file",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "<10", "f"},
			want:     map[string]string{"f": "abc"},
			wantExit: 0,
		},
		{
			name:     "an at most size shrinks a bigger file",
			fsys:     file("f", "abcdef"),
			args:     []string{"-s", "<4", "f"},
			want:     map[string]string{"f": "abcd"},
			wantExit: 0,
		},
		{
			name:     "an at least size keeps a bigger file",
			fsys:     file("f", "abcdef"),
			args:     []string{"-s", ">3", "f"},
			want:     map[string]string{"f": "abcdef"},
			wantExit: 0,
		},
		{
			name:     "an at least size grows a smaller file",
			fsys:     file("f", "abc"),
			args:     []string{"-s", ">5", "f"},
			want:     map[string]string{"f": "abc\x00\x00"},
			wantExit: 0,
		},

		// Rounded sizes.
		{
			name:     "a round down size keeps an exact multiple",
			fsys:     file("f", strings.Repeat("a", 10)),
			args:     []string{"-s", "/5", "f"},
			want:     map[string]string{"f": strings.Repeat("a", 10)},
			wantExit: 0,
		},
		{
			name:     "a round down size truncates a file",
			fsys:     file("f", strings.Repeat("a", 12)),
			args:     []string{"-s", "/5", "f"},
			want:     map[string]string{"f": strings.Repeat("a", 10)},
			wantExit: 0,
		},
		{
			name:     "a round up size grows a file",
			fsys:     file("f", strings.Repeat("a", 12)),
			args:     []string{"-s", "%5", "f"},
			want:     map[string]string{"f": strings.Repeat("a", 12) + "\x00\x00\x00"},
			wantExit: 0,
		},
		{
			name:     "a round up size keeps an exact multiple",
			fsys:     file("f", strings.Repeat("a", 10)),
			args:     []string{"-s", "%5", "f"},
			want:     map[string]string{"f": strings.Repeat("a", 10)},
			wantExit: 0,
		},
		{
			name:     "a round down size creates a missing file",
			args:     []string{"-s", "/5", "f"},
			want:     map[string]string{"f": ""},
			wantExit: 0,
		},
		{
			name:     "a round up size creates an empty missing file",
			args:     []string{"-s", "%5", "f"},
			want:     map[string]string{"f": ""},
			wantExit: 0,
		},
		{
			name:     "a round down size of zero fails",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "/0", "f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: division by zero\n",
			wantExit: 2,
		},
		{
			name:     "a round up size of zero fails",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "%0", "f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: division by zero\n",
			wantExit: 2,
		},

		// Size suffixes.
		{
			name:     "a K suffix is a kibibyte",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "1K", "f"},
			want:     map[string]string{"f": "abc" + strings.Repeat("\x00", 1024-3)},
			wantExit: 0,
		},
		{
			name:     "a KB suffix is a kilobyte",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "1KB", "f"},
			want:     map[string]string{"f": "abc" + strings.Repeat("\x00", 1000-3)},
			wantExit: 0,
		},
		{
			name:     "a KiB suffix is a kibibyte",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "1KiB", "f"},
			want:     map[string]string{"f": "abc" + strings.Repeat("\x00", 1024-3)},
			wantExit: 0,
		},
		{
			name:     "a lowercase suffix is accepted",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "2k", "f"},
			want:     map[string]string{"f": "abc" + strings.Repeat("\x00", 2048-3)},
			wantExit: 0,
		},
		{
			name:     "an M suffix is a mebibyte",
			fsys:     file("f", ""),
			args:     []string{"-s", "1M", "f"},
			want:     map[string]string{"f": strings.Repeat("\x00", 1024*1024)},
			wantExit: 0,
		},
		{
			name:     "an MB suffix is a megabyte",
			fsys:     file("f", ""),
			args:     []string{"-s", "1MB", "f"},
			want:     map[string]string{"f": strings.Repeat("\x00", 1000*1000)},
			wantExit: 0,
		},
		{
			name:     "a G suffix extends with a plus size",
			fsys:     file("f", ""),
			args:     []string{"-s", "+1G", "f"},
			want:     map[string]string{"f": strings.Repeat("\x00", 1024*1024*1024)},
			wantExit: 0,
		},
		{
			name:     "an unknown suffix is invalid",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "1X", "f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: invalid number: '1X'\n",
			wantExit: 2,
		},

		// The -r reference file.
		{
			name: "a reference size grows a file",
			fsys: fsx.MemFS{
				"f":   {Data: []byte("abc")},
				"ref": {Data: []byte(strings.Repeat("r", 10))},
			},
			args:     []string{"-r", "ref", "f"},
			want:     map[string]string{"f": "abc" + strings.Repeat("\x00", 7), "ref": strings.Repeat("r", 10)},
			wantExit: 0,
		},
		{
			name: "a reference size shrinks a file",
			fsys: fsx.MemFS{
				"f":   {Data: []byte("abcdef")},
				"ref": {Data: []byte("rr")},
			},
			args:     []string{"-r", "ref", "f"},
			want:     map[string]string{"f": "ab", "ref": "rr"},
			wantExit: 0,
		},
		{
			name: "a reference size applies to every file",
			fsys: fsx.MemFS{
				"a":   {Data: []byte("aaa")},
				"b":   {Data: []byte(strings.Repeat("b", 8))},
				"ref": {Data: []byte(strings.Repeat("r", 5))},
			},
			args: []string{"-r", "ref", "a", "b"},
			want: map[string]string{
				"a":   "aaa\x00\x00",
				"b":   "bbbbb",
				"ref": strings.Repeat("r", 5),
			},
			wantExit: 0,
		},
		{
			name: "a relative size is relative to the reference size",
			fsys: fsx.MemFS{
				"f":   {Data: []byte(strings.Repeat("a", 100))},
				"ref": {Data: []byte(strings.Repeat("r", 4))},
			},
			args:     []string{"-r", "ref", "-s", "+2", "f"},
			want:     map[string]string{"f": strings.Repeat("a", 6), "ref": strings.Repeat("r", 4)},
			wantExit: 0,
		},
		{
			name:     "a missing reference fails",
			fsys:     file("f", "abc"),
			args:     []string{"-r", "ref", "f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: cannot stat ref: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "an empty reference is a name, not an omitted option",
			fsys:     file("f", "abc"),
			args:     []string{"-r", "", "f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: cannot stat : invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "an absolute size fails with a reference",
			fsys:     file("f", "abc"),
			args:     []string{"-r", "f", "-s", "10", "f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: you must specify a relative --size with --reference\n",
			wantExit: 2,
		},

		// Missing files.
		{
			name:     "a missing file with -c is skipped",
			args:     []string{"-c", "-s", "5", "f"},
			want:     map[string]string{},
			wantExit: 0,
		},
		{
			name:     "a missing file in a missing directory fails",
			args:     []string{"-s", "5", "no/dir/f"},
			want:     map[string]string{},
			wantErr:  "truncate: cannot open no/dir/f for writing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing file with -c in a missing directory is skipped",
			args:     []string{"-c", "-s", "5", "no/dir/f"},
			want:     map[string]string{},
			wantExit: 0,
		},
		{
			name:     "an existing file is truncated with -c",
			fsys:     file("f", "hello world"),
			args:     []string{"-c", "-s", "5", "f"},
			want:     map[string]string{"f": "hello"},
			wantExit: 0,
		},

		// Usage errors.
		{
			name:     "no size or reference fails",
			fsys:     file("f", "abc"),
			args:     []string{"f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: you must specify either --size or --reference\n",
			wantExit: 2,
		},
		{
			name:     "no file operand fails",
			args:     []string{"-s", "5"},
			want:     map[string]string{},
			wantErr:  "truncate: missing file operand\n",
			wantExit: 2,
		},
		{
			name:     "no options at all fails",
			args:     nil,
			want:     map[string]string{},
			wantErr:  "truncate: missing file operand\n",
			wantExit: 2,
		},
		{
			name:     "an invalid number fails",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "abc", "f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: invalid number: 'abc'\n",
			wantExit: 2,
		},
		{
			name:     "an empty size fails",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "", "f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: invalid number: ''\n",
			wantExit: 2,
		},
		{
			name:     "a trailing size character fails",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "3x", "f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: invalid number: '3x'\n",
			wantExit: 2,
		},
		{
			name:     "combined relative modifiers fail",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "/+3", "f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: multiple relative modifiers specified\n",
			wantExit: 2,
		},
		{
			name:     "an invalid option fails",
			fsys:     file("f", "abc"),
			args:     []string{"-x", "-s", "3", "f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "a missing option-argument fails",
			fsys:     file("f", "abc"),
			args:     []string{"-s"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: option requires an argument -- 's'\n",
			wantExit: 2,
		},
		{
			name:     "an overflowing size fails",
			fsys:     file("f", "abc"),
			args:     []string{"-s", "999999999999999999999999999999", "f"},
			want:     map[string]string{"f": "abc"},
			wantErr:  "truncate: invalid number: '999999999999999999999999999999'\n",
			wantExit: 2,
		},

		// Option forms.
		{
			name:     "an attached option-argument works",
			fsys:     file("f", "hello world"),
			args:     []string{"-s5", "f"},
			want:     map[string]string{"f": "hello"},
			wantExit: 0,
		},
		{
			name:     "a grouped option works",
			fsys:     file("f", "hello world"),
			args:     []string{"-cs", "5", "f"},
			want:     map[string]string{"f": "hello"},
			wantExit: 0,
		},
		{
			name:     "long options work",
			fsys:     file("f", "hello world"),
			args:     []string{"--no-create", "--size=5", "f"},
			want:     map[string]string{"f": "hello"},
			wantExit: 0,
		},
		{
			name:     "a long option in the next argument works",
			fsys:     file("f", "hello world"),
			args:     []string{"--size", "5", "f"},
			want:     map[string]string{"f": "hello"},
			wantExit: 0,
		},
		{
			name:     "operands after -- are files",
			fsys:     file("f", "hello world"),
			args:     []string{"-s", "5", "--", "f"},
			want:     map[string]string{"f": "hello"},
			wantExit: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunMultipleFiles(t *testing.T) {
	t.Run("all files are truncated", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a": {Data: []byte("aaaa")},
			"b": {Data: []byte("bbbbbb")},
		}
		runCase(t, testCase{
			fsys:     fsys,
			args:     []string{"-s", "2", "a", "b"},
			want:     map[string]string{"a": "aa", "b": "bb"},
			wantExit: 0,
		})
	})

	t.Run("a failing file doesn't keep the others from being truncated", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a":   {Data: []byte("aaaa")},
			"b":   {Data: []byte("bbbbbb")},
			"dir": {Mode: fs.ModeDir | 0o755},
		}
		runCase(t, testCase{
			fsys:     fsys,
			args:     []string{"-s", "2", "a", "dir", "b"},
			want:     map[string]string{"a": "aa", "b": "bb", "dir": ""},
			wantErr:  "truncate: cannot open dir for writing: invalid argument\n",
			wantExit: 1,
		})
	})

	t.Run("the diagnostics of every failing file are reported", func(t *testing.T) {
		fsys := fsx.MemFS{
			"dir": {Mode: fs.ModeDir | 0o755},
		}
		runCase(t, testCase{
			fsys:     fsys,
			args:     []string{"-s", "2", "dir", "dir"},
			want:     map[string]string{"dir": ""},
			wantErr:  strings.Repeat("truncate: cannot open dir for writing: invalid argument\n", 2),
			wantExit: 1,
		})
	})
}

func TestRunContext(t *testing.T) {
	fsys := file("f", "hello")
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: new(bytes.Buffer),
		Stderr: stderr,
		FS:     fsys,
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := New(rt).Run(ctx, "-s", "3", "f")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
	if data, err := fs.ReadFile(fsys, "f"); err != nil || string(data) != "hello" {
		t.Errorf("Run() changed f to %q (%v)", data, err)
	}
}

// failWriteFS wraps a MemFS so that writing to opened files fails.
type failWriteFS struct {
	fsx.MemFS
	err error
}

// OpenFile implements fsx.WriteFS.
func (m failWriteFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	f, err := m.MemFS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return failWriteFile{File: f, err: m.err}, nil
}

type failWriteFile struct {
	fsx.File
	err error
}

// Write implements io.Writer.
func (f failWriteFile) Write([]byte) (int, error) {
	return 0, f.err
}

// nonSeekableFS wraps a MemFS so that its files can't be sought.
type nonSeekableFS struct {
	fsx.MemFS
}

// plainFile hides the seek support of the file it wraps.
type plainFile struct {
	fsx.File
}

// OpenFile implements fsx.WriteFS.
func (m nonSeekableFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	f, err := m.MemFS.OpenFile(name, flag, perm)
	return plainFile{f}, err
}

func TestRunWriteFailure(t *testing.T) {
	t.Run("a failed write is reported", func(t *testing.T) {
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdout: new(bytes.Buffer),
			Stderr: stderr,
			FS:     failWriteFS{MemFS: file("f", "hello"), err: errors.New("boom")},
		}
		err := New(rt).Run(t.Context(), "-s", "10", "f")
		code := 0
		if exitErr, ok := errors.AsType[*coreutils.ExitError](err); ok {
			code = exitErr.Code
		} else if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
		if want := "truncate: failed to truncate f at 10 bytes: boom\n"; stderr.String() != want {
			t.Errorf("Run() stderr = %q, want %q", stderr.String(), want)
		}
	})
}

func TestRunAppendOnly(t *testing.T) {
	// On a file system whose files can't be sought, growing is done by
	// appending zeroes instead.
	t.Run("a file grows through appended zeroes", func(t *testing.T) {
		fsys := nonSeekableFS{MemFS: file("f", "abc")}
		runCase(t, testCase{
			fsys:     fsys,
			args:     []string{"-s", "5", "f"},
			want:     map[string]string{"f": "abc\x00\x00"},
			wantExit: 0,
		})
	})

	t.Run("a missing file grows through appended zeroes", func(t *testing.T) {
		fsys := nonSeekableFS{MemFS: fsx.MemFS{}}
		runCase(t, testCase{
			fsys:     fsys,
			args:     []string{"-s", "3", "f"},
			want:     map[string]string{"f": "\x00\x00\x00"},
			wantExit: 0,
		})
	})
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		base     int64
		want     int64
		wantBase int64
	}{
		{name: "a plain number", value: "5", base: 10, want: 5},
		{name: "a zero", value: "0", base: 10, want: 0},
		{name: "a plus prefix", value: "+5", base: 10, want: 15},
		{name: "a minus prefix", value: "-5", base: 10, want: 5},
		{name: "a minus prefix past the base", value: "-50", base: 10, want: 0},
		{name: "an at most prefix", value: "<4", base: 10, want: 4},
		{name: "an at least prefix", value: ">4", base: 2, want: 4},
		{name: "a round down prefix", value: "/3", base: 10, want: 9},
		{name: "a round up prefix", value: "%3", base: 10, want: 12},
		{name: "a round up prefix of an exact multiple", value: "%3", base: 9, want: 9},
		{name: "a K suffix", value: "2K", want: 2048},
		{name: "a KB suffix", value: "2KB", want: 2000},
		{name: "a KiB suffix", value: "2KiB", want: 2048},
		{name: "a M suffix", value: "1M", want: 1024 * 1024},
		{name: "a G suffix", value: "1G", want: 1024 * 1024 * 1024},
		{name: "a T suffix", value: "1T", want: 1024 * 1024 * 1024 * 1024},
		{name: "a lowercase suffix", value: "1k", want: 1024},
		{name: "a suffixed plus size", value: "+2K", base: 1, want: 2049},
		{name: "a suffixed round up size", value: "%1K", base: 1025, want: 2048},
		{name: "leading spaces", value: " +5", base: 10, want: 15},
		{name: "spaces before the number", value: "< 5", base: 10, want: 5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec, err := parseSize(test.value)
			if err != nil {
				t.Fatalf("parseSize(%q) error = %v", test.value, err)
			}
			wantBase := test.base
			if got := spec.apply(wantBase); got != test.want {
				t.Errorf("parseSize(%q).apply(%d) = %d, want %d", test.value, wantBase, got, test.want)
			}
		})
	}
}

func TestParseSizeError(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{name: "an empty size", value: "", wantErr: "invalid number: ''"},
		{name: "a non number", value: "abc", wantErr: "invalid number: 'abc'"},
		{name: "a trailing character", value: "5x", wantErr: "invalid number: '5x'"},
		{name: "a bare sign", value: "+", wantErr: "invalid number: '+'"},
		{name: "a huge size", value: "999999999999999999999999999999", wantErr: "invalid number: '999999999999999999999999999999'"},
		{name: "an overflowing suffix", value: "1Q", wantErr: "invalid number: '1Q'"},
		{name: "combined modifiers", value: "/+5", wantErr: "multiple relative modifiers specified"},
		{name: "combined modifiers again", value: "%+5", wantErr: "multiple relative modifiers specified"},
		{name: "a division by zero", value: "/0", wantErr: "division by zero"},
		{name: "a division by zero again", value: "%0", wantErr: "division by zero"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSize(test.value)
			if err == nil {
				t.Fatalf("parseSize(%q) error = nil, want %q", test.value, test.wantErr)
			}
			if err.Error() != test.wantErr {
				t.Errorf("parseSize(%q) error = %q, want %q", test.value, err, test.wantErr)
			}
		})
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "truncate (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
