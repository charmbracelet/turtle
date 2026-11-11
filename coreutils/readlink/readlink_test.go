package readlink

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
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

type testCase struct {
	name     string
	files    fsx.MemFS
	wd       string
	args     []string
	environ  []string
	wantOut  string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         test.files,
		WorkingDir: test.wd,
	}
	if test.environ != nil {
		rt.Environ = func() []string { return test.environ }
	}

	err := New(rt).Run(t.Context(), test.args...)

	if got := stdout.String(); got != test.wantOut {
		t.Errorf("Run() stdout = %q, want %q", got, test.wantOut)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
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

func TestRunLinkTargets(t *testing.T) {
	tests := []testCase{
		{
			name:    "prints the target of a symbolic link",
			files:   tree(),
			wd:      "work",
			args:    []string{"link"},
			wantOut: "a.txt\n",
		},
		{
			name:    "a target is printed as stored, without being resolved",
			files:   tree(),
			wd:      "work",
			args:    []string{"chain"},
			wantOut: "link\n",
		},
		{
			name:    "a dangling link still prints its target",
			files:   tree(),
			wd:      "work",
			args:    []string{"dangling"},
			wantOut: "missing.txt\n",
		},
		{
			name:    "a link to a directory prints its target",
			files:   tree(),
			wd:      "work",
			args:    []string{"dirlink"},
			wantOut: "d\n",
		},
		{
			name:    "each operand is printed on its own line",
			files:   tree(),
			wd:      "work",
			args:    []string{"link", "dangling"},
			wantOut: "a.txt\nmissing.txt\n",
		},
		{
			name:    "the working directory is a prefix of the operand",
			files:   tree(),
			wd:      "work",
			args:    []string{"dirlink/../link"},
			wantOut: "a.txt\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunNotALink(t *testing.T) {
	tests := []testCase{
		{
			name:     "a regular file prints nothing and fails quietly",
			files:    tree(),
			wd:       "work",
			args:     []string{"a.txt"},
			wantExit: 1,
		},
		{
			name:     "a directory prints nothing and fails quietly",
			files:    tree(),
			wd:       "work",
			args:     []string{"d"},
			wantExit: 1,
		},
		{
			name:     "a missing file fails quietly",
			files:    tree(),
			wd:       "work",
			args:     []string{"missing"},
			wantExit: 1,
		},
		{
			name:     "-v reports a file which isn't a link",
			files:    tree(),
			wd:       "work",
			args:     []string{"-v", "a.txt"},
			wantErr:  "readlink: a.txt: not a symbolic link\n",
			wantExit: 1,
		},
		{
			name:     "-v reports a missing file",
			files:    tree(),
			wd:       "work",
			args:     []string{"-v", "missing"},
			wantErr:  "readlink: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a failing operand doesn't stop the others",
			files:    tree(),
			wd:       "work",
			args:     []string{"a.txt", "link"},
			wantOut:  "a.txt\n",
			wantExit: 1,
		},
		{
			name:     "-q suppresses the error messages",
			files:    tree(),
			wd:       "work",
			args:     []string{"-q", "missing"},
			wantExit: 1,
		},
		{
			name:     "-s is --silent",
			files:    tree(),
			wd:       "work",
			args:     []string{"-s", "a.txt"},
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunPOSIXLYCorrect(t *testing.T) {
	env := []string{"POSIXLY_CORRECT=1"}
	tests := []testCase{
		{
			name:     "errors are reported by default",
			files:    tree(),
			wd:       "work",
			args:     []string{"missing"},
			environ:  env,
			wantErr:  "readlink: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "an empty POSIXLY_CORRECT still counts as set",
			files:    tree(),
			wd:       "work",
			args:     []string{"missing"},
			environ:  []string{"POSIXLY_CORRECT="},
			wantErr:  "readlink: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "-q still suppresses them",
			files:    tree(),
			wd:       "work",
			args:     []string{"-q", "missing"},
			environ:  env,
			wantExit: 1,
		},
		{
			name:     "-s still suppresses them",
			files:    tree(),
			wd:       "work",
			args:     []string{"-s", "a.txt"},
			environ:  env,
			wantExit: 1,
		},
		{
			name:     "-v keeps the messages on with POSIXLY_CORRECT",
			files:    tree(),
			wd:       "work",
			args:     []string{"-v", "missing"},
			environ:  env,
			wantErr:  "readlink: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "successful operands print their target",
			files:   tree(),
			wd:      "work",
			args:    []string{"link"},
			environ: env,
			wantOut: "a.txt\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunCanonicalize(t *testing.T) {
	tests := []testCase{
		{
			name:    "-f resolves a regular file",
			files:   tree(),
			wd:      "work",
			args:    []string{"-f", "a.txt"},
			wantOut: "work/a.txt\n",
		},
		{
			name:    "-f resolves a link to its target",
			files:   tree(),
			wd:      "work",
			args:    []string{"-f", "link"},
			wantOut: "work/a.txt\n",
		},
		{
			name:    "-f resolves the whole chain of links",
			files:   tree(),
			wd:      "work",
			args:    []string{"-f", "chain"},
			wantOut: "work/a.txt\n",
		},
		{
			name:    "-f keeps a dangling last component",
			files:   tree(),
			wd:      "work",
			args:    []string{"-f", "dangling"},
			wantOut: "work/missing.txt\n",
		},
		{
			name:     "-f fails quietly when a component before the last is missing",
			files:    tree(),
			wd:       "work",
			args:     []string{"-f", "missing/x"},
			wantExit: 1,
		},
		{
			name:     "-v reports the errors of canonicalize mode",
			files:    tree(),
			wd:       "work",
			args:     []string{"-v", "-f", "missing/x"},
			wantErr:  "readlink: missing/x: file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "-e resolves existing names",
			files:   tree(),
			wd:      "work",
			args:    []string{"-e", "link"},
			wantOut: "work/a.txt\n",
		},
		{
			name:     "-e fails on a missing last component",
			files:    tree(),
			wd:       "work",
			args:     []string{"-e", "missing"},
			wantExit: 1,
		},
		{
			name:     "-e fails on a dangling link",
			files:    tree(),
			wd:       "work",
			args:     []string{"-e", "dangling"},
			wantExit: 1,
		},
		{
			name:    "-m treats missing components as directories",
			files:   tree(),
			wd:      "work",
			args:    []string{"-m", "none/x"},
			wantOut: "work/none/x\n",
		},
		{
			name:     "-f fails on a symbolic link loop",
			files:    tree(),
			wd:       "work",
			args:     []string{"-f", "loop"},
			wantExit: 1,
		},
		{
			name:    "the last mode option wins",
			files:   tree(),
			wd:      "work",
			args:    []string{"-f", "-e", "a.txt"},
			wantOut: "work/a.txt\n",
		},
		{
			name:    "-f ignores a trailing slash",
			files:   tree(),
			wd:      "work",
			args:    []string{"-f", "d/"},
			wantOut: "work/d\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDelimiters(t *testing.T) {
	tests := []testCase{
		{
			name:    "-n prints no delimiter for a single file",
			files:   tree(),
			wd:      "work",
			args:    []string{"-n", "link"},
			wantOut: "a.txt",
		},
		{
			name:    "-n prints no delimiter for several files",
			files:   tree(),
			wd:      "work",
			args:    []string{"-n", "link", "dangling"},
			wantOut: "a.txtmissing.txt",
			wantErr: "readlink: warning: -n specified with more than one operand; not printing delimiters\n",
		},
		{
			name:    "-n keeps -z from printing a delimiter",
			files:   tree(),
			wd:      "work",
			args:    []string{"-nz", "link"},
			wantOut: "a.txt",
		},
		{
			name:    "-z ends each line with NUL",
			files:   tree(),
			wd:      "work",
			args:    []string{"-z", "link", "dangling"},
			wantOut: "a.txt\x00missing.txt\x00",
		},
		{
			name:    "short options can be grouped",
			files:   tree(),
			wd:      "work",
			args:    []string{"-qzn", "link"},
			wantOut: "a.txt",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunUsageErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "missing operands",
			files:    tree(),
			wd:       "work",
			wantErr:  "readlink: missing operand\nTry 'readlink --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid short option",
			files:    tree(),
			wd:       "work",
			args:     []string{"-x", "link"},
			wantErr:  "readlink: invalid option -- 'x'\nTry 'readlink --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an unknown long option",
			files:    tree(),
			wd:       "work",
			args:     []string{"--bogus", "link"},
			wantErr:  "readlink: unrecognized option '--bogus'\nTry 'readlink --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an empty operand",
			files:    tree(),
			wd:       "work",
			args:     []string{""},
			wantErr:  "readlink: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:  "the options end at the first operand",
			files: tree(),
			wd:    "work",
			args:  []string{"link", "-n"},
			// "-n" is an operand, and it names no link; it fails quietly.
			wantOut:  "a.txt\n",
			wantExit: 1,
		},
		{
			name:    "-- ends the options",
			files:   tree(),
			wd:      "work",
			args:    []string{"--", "link"},
			wantOut: "a.txt\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestCommonOptions(t *testing.T) {
	t.Run("--help prints the usage", func(t *testing.T) {
		runCase(t, testCase{
			files:   tree(),
			wd:      "work",
			args:    []string{"--help"},
			wantOut: usage,
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
