package sync

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

func TestRun(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := []testutil.Case{
			{
				Name: "no operands",
				FS:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
				Args: nil,
			},
			{
				Name: "file operands",
				FS:   fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}},
				Args: []string{"a.txt", "b.txt"},
			},
			{
				Name: "resolves the operands against the working directory",
				FS:   fsx.MemFS{"d/a.txt": {Data: []byte("a")}},
				Dir:  "d",
				Args: []string{"a.txt"},
			},
			{
				Name: "a dash operand is a file name",
				FS:   fsx.MemFS{"-": {Data: []byte("a")}},
				Args: []string{"-"},
			},
			{
				Name: "a file after a double dash",
				FS:   fsx.MemFS{"-d.txt": {Data: []byte("a")}},
				Args: []string{"--", "-d.txt"},
			},
		}
		testutil.Run(t, New, tests)
	})

	t.Run("failures", func(t *testing.T) {
		tests := []testutil.Case{
			{
				Name:     "a missing file is an error",
				FS:       fsx.MemFS{},
				Args:     []string{"missing"},
				WantErr:  "sync: error opening 'missing': file does not exist\n",
				WantExit: 1,
			},
			{
				Name:     "a good file doesn't hide a missing one",
				FS:       fsx.MemFS{"a.txt": {Data: []byte("a")}},
				Args:     []string{"a.txt", "missing"},
				WantErr:  "sync: error opening 'missing': file does not exist\n",
				WantExit: 1,
			},
			{
				Name:     "a missing file in the working directory is an error",
				FS:       fsx.MemFS{"d/a.txt": {Data: []byte("a")}},
				Dir:      "d",
				Args:     []string{"missing"},
				WantErr:  "sync: error opening 'missing': file does not exist\n",
				WantExit: 1,
			},
			{
				Name:     "an invalid option is an error",
				FS:       fsx.MemFS{},
				Args:     []string{"-x"},
				WantErr:  "sync: invalid option -- 'x'\nTry 'sync --help' for more information.\n",
				WantExit: 2,
			},
			{
				Name:     "an unrecognized long option is an error",
				FS:       fsx.MemFS{},
				Args:     []string{"--nope"},
				WantErr:  "sync: unrecognized option '--nope'\nTry 'sync --help' for more information.\n",
				WantExit: 2,
			},
			{
				Name:     "a long option with an argument is an error",
				FS:       fsx.MemFS{},
				Args:     []string{"--data=1"},
				WantErr:  "sync: option '--data=1' doesn't allow an argument\nTry 'sync --help' for more information.\n",
				WantExit: 2,
			},
		}
		testutil.Run(t, New, tests)
	})

	t.Run("options", func(t *testing.T) {
		tests := []testutil.Case{
			{
				Name: "-d with a file",
				FS:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
				Args: []string{"-d", "a.txt"},
			},
			{
				Name: "--data with a file",
				FS:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
				Args: []string{"--data", "a.txt"},
			},
			{
				Name: "-f with a file",
				FS:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
				Args: []string{"-f", "a.txt"},
			},
			{
				Name: "--file-system with a file",
				FS:   fsx.MemFS{"a.txt": {Data: []byte("a")}},
				Args: []string{"--file-system", "a.txt"},
			},
			{
				Name: "-f with a missing file falls back to the global sync",
				FS:   fsx.MemFS{},
				Args: []string{"-f", "missing"},
			},
			{
				Name:     "both -d and -f are an error",
				FS:       fsx.MemFS{"a.txt": {Data: []byte("a")}},
				Args:     []string{"-d", "-f", "a.txt"},
				WantErr:  "sync: cannot specify both --data and --file-system\nTry 'sync --help' for more information.\n",
				WantExit: 1,
			},
			{
				Name:     "both --data and --file-system are an error",
				FS:       fsx.MemFS{},
				Args:     []string{"--data", "--file-system"},
				WantErr:  "sync: cannot specify both --data and --file-system\nTry 'sync --help' for more information.\n",
				WantExit: 1,
			},
			{
				Name:     "--data without operands is an error",
				FS:       fsx.MemFS{},
				Args:     []string{"--data"},
				WantErr:  "sync: --data needs at least one argument\nTry 'sync --help' for more information.\n",
				WantExit: 1,
			},
		}
		testutil.Run(t, New, tests)
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
	testutil.HelpVersion(t, New, "sync", usage)
}
