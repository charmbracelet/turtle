package readlink

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

// tree returns a working directory with files, directories and symbolic
// links the tests read.
func tree() fsx.MemFS {
	return fsx.MemFS{
		"work/a.txt":      {Data: []byte("hello")},
		"work/d/keep.txt": {},
		"work/x/b.txt":    {Data: []byte("b")},
		"work/link":       {Data: []byte("a.txt"), Mode: fs.ModeSymlink},
		"work/chain":      {Data: []byte("link"), Mode: fs.ModeSymlink},
		"work/dirlink":    {Data: []byte("d"), Mode: fs.ModeSymlink},
		"work/dangling":   {Data: []byte("missing.txt"), Mode: fs.ModeSymlink},
		"work/loop":       {Data: []byte("loop"), Mode: fs.ModeSymlink},
	}
}

func TestRunLinkTargets(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "prints the target of a symbolic link",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"link"},
			Want: "a.txt\n",
		},
		{
			Name: "a target is printed as stored, without being resolved",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"chain"},
			Want: "link\n",
		},
		{
			Name: "a dangling link still prints its target",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"dangling"},
			Want: "missing.txt\n",
		},
		{
			Name: "a link to a directory prints its target",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"dirlink"},
			Want: "d\n",
		},
		{
			Name: "each operand is printed on its own line",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"link", "dangling"},
			Want: "a.txt\nmissing.txt\n",
		},
		{
			Name: "the working directory is a prefix of the operand",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"dirlink/../link"},
			Want: "a.txt\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunNotALink(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "a regular file prints nothing and fails quietly",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"a.txt"},
			WantExit: 1,
		},
		{
			Name:     "a directory prints nothing and fails quietly",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"d"},
			WantExit: 1,
		},
		{
			Name:     "a missing file fails quietly",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"missing"},
			WantExit: 1,
		},
		{
			Name:     "-v reports a file which isn't a link",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-v", "a.txt"},
			WantErr:  "readlink: a.txt: not a symbolic link\n",
			WantExit: 1,
		},
		{
			Name:     "-v reports a missing file",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-v", "missing"},
			WantErr:  "readlink: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a failing operand doesn't stop the others",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"a.txt", "link"},
			Want:     "a.txt\n",
			WantExit: 1,
		},
		{
			Name:     "-q suppresses the error messages",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-q", "missing"},
			WantExit: 1,
		},
		{
			Name:     "-s is --silent",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-s", "a.txt"},
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunPOSIXLYCorrect(t *testing.T) {
	env := []string{"POSIXLY_CORRECT=1"}
	tests := []testutil.Case{
		{
			Name:     "errors are reported by default",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"missing"},
			Environ:  env,
			WantErr:  "readlink: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "an empty POSIXLY_CORRECT still counts as set",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"missing"},
			Environ:  []string{"POSIXLY_CORRECT="},
			WantErr:  "readlink: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "-q still suppresses them",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-q", "missing"},
			Environ:  env,
			WantExit: 1,
		},
		{
			Name:     "-s still suppresses them",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-s", "a.txt"},
			Environ:  env,
			WantExit: 1,
		},
		{
			Name:     "-v keeps the messages on with POSIXLY_CORRECT",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-v", "missing"},
			Environ:  env,
			WantErr:  "readlink: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:    "successful operands print their target",
			FS:      tree(),
			Dir:     "work",
			Args:    []string{"link"},
			Environ: env,
			Want:    "a.txt\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunCanonicalize(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-f resolves a regular file",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-f", "a.txt"},
			Want: "work/a.txt\n",
		},
		{
			Name: "-f resolves a link to its target",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-f", "link"},
			Want: "work/a.txt\n",
		},
		{
			Name: "-f resolves the whole chain of links",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-f", "chain"},
			Want: "work/a.txt\n",
		},
		{
			Name: "-f keeps a dangling last component",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-f", "dangling"},
			Want: "work/missing.txt\n",
		},
		{
			Name:     "-f fails quietly when a component before the last is missing",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-f", "missing/x"},
			WantExit: 1,
		},
		{
			Name:     "-v reports the errors of canonicalize mode",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-v", "-f", "missing/x"},
			WantErr:  "readlink: missing/x: file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "-e resolves existing names",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-e", "link"},
			Want: "work/a.txt\n",
		},
		{
			Name:     "-e fails on a missing last component",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-e", "missing"},
			WantExit: 1,
		},
		{
			Name:     "-e fails on a dangling link",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-e", "dangling"},
			WantExit: 1,
		},
		{
			Name: "-m treats missing components as directories",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-m", "none/x"},
			Want: "work/none/x\n",
		},
		{
			Name:     "-f fails on a symbolic link loop",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-f", "loop"},
			WantExit: 1,
		},
		{
			Name: "the last mode option wins",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-f", "-e", "a.txt"},
			Want: "work/a.txt\n",
		},
		{
			Name: "-f ignores a trailing slash",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-f", "d/"},
			Want: "work/d\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunDelimiters(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-n prints no delimiter for a single file",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-n", "link"},
			Want: "a.txt",
		},
		{
			Name:    "-n prints no delimiter for several files",
			FS:      tree(),
			Dir:     "work",
			Args:    []string{"-n", "link", "dangling"},
			Want:    "a.txtmissing.txt",
			WantErr: "readlink: warning: -n specified with more than one operand; not printing delimiters\n",
		},
		{
			Name: "-n keeps -z from printing a delimiter",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-nz", "link"},
			Want: "a.txt",
		},
		{
			Name: "-z ends each line with NUL",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-z", "link", "dangling"},
			Want: "a.txt\x00missing.txt\x00",
		},
		{
			Name: "short options can be grouped",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"-qzn", "link"},
			Want: "a.txt",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunUsageErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "missing operands",
			FS:       tree(),
			Dir:      "work",
			WantErr:  "readlink: missing operand\nTry 'readlink --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid short option",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"-x", "link"},
			WantErr:  "readlink: invalid option -- 'x'\nTry 'readlink --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an unknown long option",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{"--bogus", "link"},
			WantErr:  "readlink: unrecognized option '--bogus'\nTry 'readlink --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an empty operand",
			FS:       tree(),
			Dir:      "work",
			Args:     []string{""},
			WantErr:  "readlink: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name: "the options end at the first operand",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"link", "-n"},
			// "-n" is an operand, and it names no link; it fails quietly.
			Want:     "a.txt\n",
			WantExit: 1,
		},
		{
			Name: "-- ends the options",
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--", "link"},
			Want: "a.txt\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestCommonOptions(t *testing.T) {
	t.Run("--help prints the usage", func(t *testing.T) {
		testutil.Once(t, New, testutil.Case{
			FS:   tree(),
			Dir:  "work",
			Args: []string{"--help"},
			Want: usage,
		})
	})
	t.Run("--version prints the program name", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer), FS: tree()}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if out := stdout.String(); !strings.Contains(out, "readlink (charm.land/snail/coreutils)") {
			t.Errorf("Run() stdout = %q, want the version line", out)
		}
	})
}

// cancelFS cancels its context on the first lookup, so the resolution of
// the operands runs into a canceled context.
type cancelFS struct {
	fsx.MemFS
	cancel context.CancelFunc
}

func (f cancelFS) Lstat(name string) (fs.FileInfo, error) {
	f.cancel()
	return f.MemFS.Lstat(name)
}

func TestRunContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         cancelFS{MemFS: tree(), cancel: cancel},
		WorkingDir: "work",
	}

	// The first lookup cancels the context; the error of the resolution
	// must come out as the context error, not as a failing operand.
	err := New(rt).Run(ctx, "-f", "d/x/missing")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want no diagnostic", got)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want no output", got)
	}
}
