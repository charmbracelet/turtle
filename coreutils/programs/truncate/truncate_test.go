package truncate

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

type testCase struct {
	Name     string
	FS       fsx.FS
	Args     []string
	WantErr  string
	WantExit int
	want     map[string]string
}

// runCase runs a single test case like testutil.Once, with an empty file
// system when the case sets none, checking the resulting files like the
// table asks.
func runCase(t *testing.T, test testCase) {
	t.Helper()

	if test.FS == nil {
		test.FS = fsx.MemoryFS{}
	}
	testutil.Once(t, New, testutil.Case{
		Name:     test.Name,
		FS:       test.FS,
		Args:     test.Args,
		WantErr:  test.WantErr,
		WantExit: test.WantExit,
		Check: func(t *testing.T, rt *coreutils.Runtime) {
			checkFiles(t, rt.FS, test.want)
		},
	})
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
func memOf(fsys fsx.FS) fsx.MemoryFS {
	switch f := fsys.(type) {
	case fsx.MemoryFS:
		return f
	case nonSeekableFS:
		return f.MemoryFS
	case testutil.FailWriteFS:
		return memOf(f.FS)
	}
	return nil
}

// file returns a memory file system holding one file with the given
// contents.
func file(name, data string) fsx.MemoryFS {
	return fsx.MemoryFS{name: {Data: []byte(data), Mode: 0o600}}
}

func TestRun(t *testing.T) {
	tests := []testCase{
		// Absolute sizes.
		{
			Name:     "an absolute size shrinks a file",
			FS:       file("f", "hello world"),
			Args:     []string{"-s", "5", "f"},
			want:     map[string]string{"f": "hello"},
			WantExit: 0,
		},
		{
			Name:     "an absolute size grows a file",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "5", "f"},
			want:     map[string]string{"f": "abc\x00\x00"},
			WantExit: 0,
		},
		{
			Name:     "an absolute size of zero empties a file",
			FS:       file("f", "hello"),
			Args:     []string{"-s", "0", "f"},
			want:     map[string]string{"f": ""},
			WantExit: 0,
		},
		{
			Name:     "an absolute size keeps a file of the same size",
			FS:       file("f", "hello"),
			Args:     []string{"-s", "5", "f"},
			want:     map[string]string{"f": "hello"},
			WantExit: 0,
		},
		{
			Name:     "an absolute size of a missing file creates it",
			Args:     []string{"-s", "3", "f"},
			want:     map[string]string{"f": "\x00\x00\x00"},
			WantExit: 0,
		},
		{
			Name:     "an absolute size of zero creates an empty file",
			Args:     []string{"-s", "0", "f"},
			want:     map[string]string{"f": ""},
			WantExit: 0,
		},

		// Relative sizes.
		{
			Name:     "a plus size extends a file",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "+3", "f"},
			want:     map[string]string{"f": "abc\x00\x00\x00"},
			WantExit: 0,
		},
		{
			Name:     "a plus size of zero keeps a file",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "+0", "f"},
			want:     map[string]string{"f": "abc"},
			WantExit: 0,
		},
		{
			Name:     "a minus size reduces a file",
			FS:       file("f", "abcdef"),
			Args:     []string{"-s", "-2", "f"},
			want:     map[string]string{"f": "abcd"},
			WantExit: 0,
		},
		{
			Name:     "a minus size past the file empties it",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "-10", "f"},
			want:     map[string]string{"f": ""},
			WantExit: 0,
		},
		{
			Name:     "a plus size creates a missing file",
			Args:     []string{"-s", "+2", "f"},
			want:     map[string]string{"f": "\x00\x00"},
			WantExit: 0,
		},
		{
			Name:     "a minus size creates an empty missing file",
			Args:     []string{"-s", "-2", "f"},
			want:     map[string]string{"f": ""},
			WantExit: 0,
		},
		{
			Name:     "a minus size skips a missing file with -c",
			Args:     []string{"-c", "-s", "-2", "f"},
			want:     map[string]string{},
			WantExit: 0,
		},
		{
			Name:     "a plus size skips a missing file with -c",
			Args:     []string{"-c", "-s", "+2", "f"},
			want:     map[string]string{},
			WantExit: 0,
		},

		// At-most and at-least sizes.
		{
			Name:     "an at most size keeps a smaller file",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "<10", "f"},
			want:     map[string]string{"f": "abc"},
			WantExit: 0,
		},
		{
			Name:     "an at most size shrinks a bigger file",
			FS:       file("f", "abcdef"),
			Args:     []string{"-s", "<4", "f"},
			want:     map[string]string{"f": "abcd"},
			WantExit: 0,
		},
		{
			Name:     "an at least size keeps a bigger file",
			FS:       file("f", "abcdef"),
			Args:     []string{"-s", ">3", "f"},
			want:     map[string]string{"f": "abcdef"},
			WantExit: 0,
		},
		{
			Name:     "an at least size grows a smaller file",
			FS:       file("f", "abc"),
			Args:     []string{"-s", ">5", "f"},
			want:     map[string]string{"f": "abc\x00\x00"},
			WantExit: 0,
		},

		// Rounded sizes.
		{
			Name:     "a round down size keeps an exact multiple",
			FS:       file("f", strings.Repeat("a", 10)),
			Args:     []string{"-s", "/5", "f"},
			want:     map[string]string{"f": strings.Repeat("a", 10)},
			WantExit: 0,
		},
		{
			Name:     "a round down size truncates a file",
			FS:       file("f", strings.Repeat("a", 12)),
			Args:     []string{"-s", "/5", "f"},
			want:     map[string]string{"f": strings.Repeat("a", 10)},
			WantExit: 0,
		},
		{
			Name:     "a round up size grows a file",
			FS:       file("f", strings.Repeat("a", 12)),
			Args:     []string{"-s", "%5", "f"},
			want:     map[string]string{"f": strings.Repeat("a", 12) + "\x00\x00\x00"},
			WantExit: 0,
		},
		{
			Name:     "a round up size keeps an exact multiple",
			FS:       file("f", strings.Repeat("a", 10)),
			Args:     []string{"-s", "%5", "f"},
			want:     map[string]string{"f": strings.Repeat("a", 10)},
			WantExit: 0,
		},
		{
			Name:     "a round down size creates a missing file",
			Args:     []string{"-s", "/5", "f"},
			want:     map[string]string{"f": ""},
			WantExit: 0,
		},
		{
			Name:     "a round up size creates an empty missing file",
			Args:     []string{"-s", "%5", "f"},
			want:     map[string]string{"f": ""},
			WantExit: 0,
		},
		{
			Name:     "a round down size of zero fails",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "/0", "f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: division by zero\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a round up size of zero fails",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "%0", "f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: division by zero\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},

		// Size suffixes.
		{
			Name:     "a K suffix is a kibibyte",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "1K", "f"},
			want:     map[string]string{"f": "abc" + strings.Repeat("\x00", 1024-3)},
			WantExit: 0,
		},
		{
			Name:     "a KB suffix is a kilobyte",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "1KB", "f"},
			want:     map[string]string{"f": "abc" + strings.Repeat("\x00", 1000-3)},
			WantExit: 0,
		},
		{
			Name:     "a KiB suffix is a kibibyte",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "1KiB", "f"},
			want:     map[string]string{"f": "abc" + strings.Repeat("\x00", 1024-3)},
			WantExit: 0,
		},
		{
			Name:     "a lowercase suffix is accepted",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "2k", "f"},
			want:     map[string]string{"f": "abc" + strings.Repeat("\x00", 2048-3)},
			WantExit: 0,
		},
		{
			Name:     "an M suffix is a mebibyte",
			FS:       file("f", ""),
			Args:     []string{"-s", "1M", "f"},
			want:     map[string]string{"f": strings.Repeat("\x00", 1024*1024)},
			WantExit: 0,
		},
		{
			Name:     "an MB suffix is a megabyte",
			FS:       file("f", ""),
			Args:     []string{"-s", "1MB", "f"},
			want:     map[string]string{"f": strings.Repeat("\x00", 1000*1000)},
			WantExit: 0,
		},
		{
			Name:     "a G suffix extends with a plus size",
			FS:       file("f", ""),
			Args:     []string{"-s", "+1G", "f"},
			want:     map[string]string{"f": strings.Repeat("\x00", 1024*1024*1024)},
			WantExit: 0,
		},
		{
			Name:     "an unknown suffix is invalid",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "1X", "f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: invalid number: '1X'\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},

		// The -r reference file.
		{
			Name: "a reference size grows a file",
			FS: fsx.MemoryFS{
				"f":   {Data: []byte("abc")},
				"ref": {Data: []byte(strings.Repeat("r", 10))},
			},
			Args:     []string{"-r", "ref", "f"},
			want:     map[string]string{"f": "abc" + strings.Repeat("\x00", 7), "ref": strings.Repeat("r", 10)},
			WantExit: 0,
		},
		{
			Name: "a reference size shrinks a file",
			FS: fsx.MemoryFS{
				"f":   {Data: []byte("abcdef")},
				"ref": {Data: []byte("rr")},
			},
			Args:     []string{"-r", "ref", "f"},
			want:     map[string]string{"f": "ab", "ref": "rr"},
			WantExit: 0,
		},
		{
			Name: "a reference size applies to every file",
			FS: fsx.MemoryFS{
				"a":   {Data: []byte("aaa")},
				"b":   {Data: []byte(strings.Repeat("b", 8))},
				"ref": {Data: []byte(strings.Repeat("r", 5))},
			},
			Args: []string{"-r", "ref", "a", "b"},
			want: map[string]string{
				"a":   "aaa\x00\x00",
				"b":   "bbbbb",
				"ref": strings.Repeat("r", 5),
			},
			WantExit: 0,
		},
		{
			Name: "a relative size is relative to the reference size",
			FS: fsx.MemoryFS{
				"f":   {Data: []byte(strings.Repeat("a", 100))},
				"ref": {Data: []byte(strings.Repeat("r", 4))},
			},
			Args:     []string{"-r", "ref", "-s", "+2", "f"},
			want:     map[string]string{"f": strings.Repeat("a", 6), "ref": strings.Repeat("r", 4)},
			WantExit: 0,
		},
		{
			Name:     "a missing reference fails",
			FS:       file("f", "abc"),
			Args:     []string{"-r", "ref", "f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: cannot stat ref: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "an empty reference is a name, not an omitted option",
			FS:       file("f", "abc"),
			Args:     []string{"-r", "", "f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: cannot stat : invalid argument\n",
			WantExit: 1,
		},
		{
			Name:     "an absolute size fails with a reference",
			FS:       file("f", "abc"),
			Args:     []string{"-r", "f", "-s", "10", "f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: you must specify a relative --size with --reference\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},

		// Missing files.
		{
			Name:     "a missing file with -c is skipped",
			Args:     []string{"-c", "-s", "5", "f"},
			want:     map[string]string{},
			WantExit: 0,
		},
		{
			Name:     "a missing file in a missing directory fails",
			Args:     []string{"-s", "5", "no/dir/f"},
			want:     map[string]string{},
			WantErr:  "truncate: cannot open no/dir/f for writing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file with -c in a missing directory is skipped",
			Args:     []string{"-c", "-s", "5", "no/dir/f"},
			want:     map[string]string{},
			WantExit: 0,
		},
		{
			Name:     "an existing file is truncated with -c",
			FS:       file("f", "hello world"),
			Args:     []string{"-c", "-s", "5", "f"},
			want:     map[string]string{"f": "hello"},
			WantExit: 0,
		},

		// Usage errors.
		{
			Name:     "no size or reference fails",
			FS:       file("f", "abc"),
			Args:     []string{"f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: you must specify either --size or --reference\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "no file operand fails",
			Args:     []string{"-s", "5"},
			want:     map[string]string{},
			WantErr:  "truncate: missing file operand\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "no options at all fails",
			Args:     nil,
			want:     map[string]string{},
			WantErr:  "truncate: missing file operand\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid number fails",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "abc", "f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: invalid number: 'abc'\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty size fails",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "", "f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: invalid number: ''\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a trailing size character fails",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "3x", "f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: invalid number: '3x'\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "combined relative modifiers fail",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "/+3", "f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: multiple relative modifiers specified\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid option fails",
			FS:       file("f", "abc"),
			Args:     []string{"-x", "-s", "3", "f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: invalid option -- 'x'\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a missing option-argument fails",
			FS:       file("f", "abc"),
			Args:     []string{"-s"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: option requires an argument -- 's'\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an overflowing size fails",
			FS:       file("f", "abc"),
			Args:     []string{"-s", "999999999999999999999999999999", "f"},
			want:     map[string]string{"f": "abc"},
			WantErr:  "truncate: invalid number: '999999999999999999999999999999'\nTry 'truncate --help' for more information.\n",
			WantExit: 2,
		},

		// Option forms.
		{
			Name:     "an attached option-argument works",
			FS:       file("f", "hello world"),
			Args:     []string{"-s5", "f"},
			want:     map[string]string{"f": "hello"},
			WantExit: 0,
		},
		{
			Name:     "a grouped option works",
			FS:       file("f", "hello world"),
			Args:     []string{"-cs", "5", "f"},
			want:     map[string]string{"f": "hello"},
			WantExit: 0,
		},
		{
			Name:     "long options work",
			FS:       file("f", "hello world"),
			Args:     []string{"--no-create", "--size=5", "f"},
			want:     map[string]string{"f": "hello"},
			WantExit: 0,
		},
		{
			Name:     "a long option in the next argument works",
			FS:       file("f", "hello world"),
			Args:     []string{"--size", "5", "f"},
			want:     map[string]string{"f": "hello"},
			WantExit: 0,
		},
		{
			Name:     "operands after -- are files",
			FS:       file("f", "hello world"),
			Args:     []string{"-s", "5", "--", "f"},
			want:     map[string]string{"f": "hello"},
			WantExit: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunMultipleFiles(t *testing.T) {
	t.Run("all files are truncated", func(t *testing.T) {
		fsys := fsx.MemoryFS{
			"a": {Data: []byte("aaaa")},
			"b": {Data: []byte("bbbbbb")},
		}
		runCase(t, testCase{
			FS:       fsys,
			Args:     []string{"-s", "2", "a", "b"},
			want:     map[string]string{"a": "aa", "b": "bb"},
			WantExit: 0,
		})
	})

	t.Run("a failing file doesn't keep the others from being truncated", func(t *testing.T) {
		fsys := fsx.MemoryFS{
			"a":   {Data: []byte("aaaa")},
			"b":   {Data: []byte("bbbbbb")},
			"dir": {Mode: fs.ModeDir | 0o755},
		}
		runCase(t, testCase{
			FS:       fsys,
			Args:     []string{"-s", "2", "a", "dir", "b"},
			want:     map[string]string{"a": "aa", "b": "bb", "dir": ""},
			WantErr:  "truncate: cannot open dir for writing: invalid argument\n",
			WantExit: 1,
		})
	})

	t.Run("the diagnostics of every failing file are reported", func(t *testing.T) {
		fsys := fsx.MemoryFS{
			"dir": {Mode: fs.ModeDir | 0o755},
		}
		runCase(t, testCase{
			FS:       fsys,
			Args:     []string{"-s", "2", "dir", "dir"},
			want:     map[string]string{"dir": ""},
			WantErr:  strings.Repeat("truncate: cannot open dir for writing: invalid argument\n", 2),
			WantExit: 1,
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

// nonSeekableFS wraps a MemoryFS so that its files can't be sought.
type nonSeekableFS struct {
	fsx.MemoryFS
}

// plainFile hides the seek support of the file it wraps.
type plainFile struct {
	fsx.File
}

// OpenFile implements fsx.WriteFS.
func (m nonSeekableFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	f, err := m.MemoryFS.OpenFile(name, flag, perm)
	return plainFile{f}, err
}

func TestRunWriteFailure(t *testing.T) {
	t.Run("a failed write is reported", func(t *testing.T) {
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdout: new(bytes.Buffer),
			Stderr: stderr,
			FS:     testutil.FailWriteFS{FS: file("f", "hello"), Err: errors.New("boom")},
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
		fsys := nonSeekableFS{MemoryFS: file("f", "abc")}
		runCase(t, testCase{
			FS:       fsys,
			Args:     []string{"-s", "5", "f"},
			want:     map[string]string{"f": "abc\x00\x00"},
			WantExit: 0,
		})
	})

	t.Run("a missing file grows through appended zeroes", func(t *testing.T) {
		fsys := nonSeekableFS{MemoryFS: fsx.MemoryFS{}}
		runCase(t, testCase{
			FS:       fsys,
			Args:     []string{"-s", "3", "f"},
			want:     map[string]string{"f": "\x00\x00\x00"},
			WantExit: 0,
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
		{name: "a boot suffix", value: "5b", wantErr: "invalid number: '5b'"},
		{name: "a character suffix", value: "5c", wantErr: "invalid number: '5c'"},
		{name: "a word suffix", value: "5w", wantErr: "invalid number: '5w'"},
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
	testutil.HelpVersion(t, New, "truncate", usage)
}
