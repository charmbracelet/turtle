package sync

import (
	"bytes"
	"context"
	"errors"
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
				name:  "no operands",
				files: fsx.MemFS{"a.txt": {Data: []byte("a")}},
				args:  nil,
			},
			{
				name:  "file operands",
				files: fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}},
				args:  []string{"a.txt", "b.txt"},
			},
			{
				name:  "resolves the operands against the working directory",
				files: fsx.MemFS{"d/a.txt": {Data: []byte("a")}},
				wd:    "d",
				args:  []string{"a.txt"},
			},
			{
				name:  "a dash operand is a file name",
				files: fsx.MemFS{"-": {Data: []byte("a")}},
				args:  []string{"-"},
			},
			{
				name:  "a file after a double dash",
				files: fsx.MemFS{"-d.txt": {Data: []byte("a")}},
				args:  []string{"--", "-d.txt"},
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
				name:     "a missing file is an error",
				files:    fsx.MemFS{},
				args:     []string{"missing"},
				wantErr:  "sync: error opening 'missing': file does not exist\n",
				wantExit: 1,
			},
			{
				name:     "a good file doesn't hide a missing one",
				files:    fsx.MemFS{"a.txt": {Data: []byte("a")}},
				args:     []string{"a.txt", "missing"},
				wantErr:  "sync: error opening 'missing': file does not exist\n",
				wantExit: 1,
			},
			{
				name:     "a missing file in the working directory is an error",
				files:    fsx.MemFS{"d/a.txt": {Data: []byte("a")}},
				wd:       "d",
				args:     []string{"missing"},
				wantErr:  "sync: error opening 'missing': file does not exist\n",
				wantExit: 1,
			},
			{
				name:     "an invalid option is an error",
				files:    fsx.MemFS{},
				args:     []string{"-x"},
				wantErr:  "sync: invalid option -- 'x'\nTry 'sync --help' for more information.\n",
				wantExit: 2,
			},
			{
				name:     "an unrecognized long option is an error",
				files:    fsx.MemFS{},
				args:     []string{"--nope"},
				wantErr:  "sync: unrecognized option '--nope'\nTry 'sync --help' for more information.\n",
				wantExit: 2,
			},
			{
				name:     "a long option with an argument is an error",
				files:    fsx.MemFS{},
				args:     []string{"--data=1"},
				wantErr:  "sync: option '--data=1' doesn't allow an argument\nTry 'sync --help' for more information.\n",
				wantExit: 2,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("options", func(t *testing.T) {
		tests := []testCase{
			{
				name:  "-d with a file",
				files: fsx.MemFS{"a.txt": {Data: []byte("a")}},
				args:  []string{"-d", "a.txt"},
			},
			{
				name:  "--data with a file",
				files: fsx.MemFS{"a.txt": {Data: []byte("a")}},
				args:  []string{"--data", "a.txt"},
			},
			{
				name:  "-f with a file",
				files: fsx.MemFS{"a.txt": {Data: []byte("a")}},
				args:  []string{"-f", "a.txt"},
			},
			{
				name:  "--file-system with a file",
				files: fsx.MemFS{"a.txt": {Data: []byte("a")}},
				args:  []string{"--file-system", "a.txt"},
			},
			{
				name:  "-f with a missing file falls back to the global sync",
				files: fsx.MemFS{},
				args:  []string{"-f", "missing"},
			},
			{
				name:     "both -d and -f are an error",
				files:    fsx.MemFS{"a.txt": {Data: []byte("a")}},
				args:     []string{"-d", "-f", "a.txt"},
				wantErr:  "sync: cannot specify both --data and --file-system\nTry 'sync --help' for more information.\n",
				wantExit: 1,
			},
			{
				name:     "both --data and --file-system are an error",
				files:    fsx.MemFS{},
				args:     []string{"--data", "--file-system"},
				wantErr:  "sync: cannot specify both --data and --file-system\nTry 'sync --help' for more information.\n",
				wantExit: 1,
			},
			{
				name:     "--data without operands is an error",
				files:    fsx.MemFS{},
				args:     []string{"--data"},
				wantErr:  "sync: --data needs at least one argument\nTry 'sync --help' for more information.\n",
				wantExit: 1,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})
}

// countFS counts how many times the whole file system was asked to sync.
type countFS struct {
	fsx.MemFS
	syncs int
}

// Sync implements fsx.SyncFS.
func (c *countFS) Sync() error {
	c.syncs++
	return c.MemFS.Sync()
}

// synclessFS hides the SyncFS of the file system it holds.
type synclessFS struct {
	fsx.FS
}

func TestRunSyncs(t *testing.T) {
	t.Run("no operands sync the whole file system", func(t *testing.T) {
		fsys := &countFS{MemFS: fsx.MemFS{"a.txt": {Data: []byte("a")}}}
		rt := &coreutils.Runtime{Stderr: new(bytes.Buffer), FS: fsys}
		if err := New(rt).Run(t.Context()); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if fsys.syncs != 1 {
			t.Errorf("Run() synced %d times, want 1", fsys.syncs)
		}
	})

	t.Run("file operands don't sync the whole file system", func(t *testing.T) {
		fsys := &countFS{MemFS: fsx.MemFS{"a.txt": {Data: []byte("a")}}}
		rt := &coreutils.Runtime{Stderr: new(bytes.Buffer), FS: fsys}
		if err := New(rt).Run(t.Context(), "a.txt"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if fsys.syncs != 0 {
			t.Errorf("Run() synced %d times, want 0", fsys.syncs)
		}
	})

	t.Run("-f with operands falls back to the whole file system", func(t *testing.T) {
		fsys := &countFS{MemFS: fsx.MemFS{}}
		rt := &coreutils.Runtime{Stderr: new(bytes.Buffer), FS: fsys}
		if err := New(rt).Run(t.Context(), "-f", "missing"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if fsys.syncs != 1 {
			t.Errorf("Run() synced %d times, want 1", fsys.syncs)
		}
	})

	t.Run("a file system without SyncFS is a no-op", func(t *testing.T) {
		fsys := synclessFS{fsx.MemFS{"a.txt": {Data: []byte("a")}}}
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stderr: stderr, FS: fsys}
		if err := New(rt).Run(t.Context()); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("Run() stderr = %q, want none", got)
		}
	})
}

func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rt := &coreutils.Runtime{Stderr: new(bytes.Buffer), FS: fsx.MemFS{}}
	if err := New(rt).Run(ctx, "a.txt"); !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
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
		want := "sync (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
