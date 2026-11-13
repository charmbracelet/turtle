// Package testutil holds the test scaffolding shared by the core utilities
// tests: the table-driven runner for the common Runtime setup and assertions,
// and the reader, writer and file system fakes they need.
package testutil

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/fsx"
)

// Case is one case of a table-driven test: the inputs the Runtime is built
// from, and what Run must produce.
type Case struct {
	Name    string
	Stdin   io.Reader
	FS      fsx.FS
	Dir     string
	Environ []string
	Umask   fs.FileMode
	Args    []string

	// Want and WantErr are the wanted standard output and error, WantExit
	// the wanted exit status.
	Want     string
	WantErr  string
	WantExit int

	// WantFiles, WantMode and WantTime are the wanted contents, mode and
	// modification time of files after the run, Present the files that must
	// still exist and Absent the ones that must not.
	WantFiles map[string]string
	WantMode  map[string]fs.FileMode
	WantTime  map[string]time.Time
	Present   []string
	Absent    []string

	// Check runs extra assertions after the standard ones, with the Runtime
	// the case ran with.
	Check func(*testing.T, *coreutils.Runtime)
}

// Result is what running one case produced.
type Result struct {
	Exit   int
	Err    error
	Stdout string
	Stderr string
}

// Run runs every case as a sub-test of t, asserting each one like Once.
func Run(t *testing.T, new coreutils.NewFunc, tests []Case) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			Once(t, new, test)
		})
	}
}

// Once runs a single case, asserting its exit status, standard output and
// error, and the state of its files, and returning what the run produced.
func Once(t *testing.T, new coreutils.NewFunc, c Case) Result {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := &coreutils.Runtime{
		Stdin:      c.Stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		WorkingDir: c.Dir,
		Umask:      c.Umask,
	}
	if c.FS != nil {
		rt.FS = c.FS
	}
	if c.Environ != nil {
		rt.Environ = func() []string { return c.Environ }
	}

	err := new(rt).Run(t.Context(), c.Args...)
	code := ExitCode(t, err)

	if code != c.WantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, c.WantExit, err)
	}
	if got := stdout.String(); got != c.Want {
		t.Errorf("Run() stdout = %q, want %q", got, c.Want)
	}
	if got := stderr.String(); got != c.WantErr {
		t.Errorf("Run() stderr = %q, want %q", got, c.WantErr)
	}

	assertFiles(t, rt, c)

	if c.Check != nil {
		c.Check(t, rt)
	}

	return Result{Exit: code, Err: err, Stdout: stdout.String(), Stderr: stderr.String()}
}

// assertFiles checks the state of the files of a case after its run.
func assertFiles(t *testing.T, rt *coreutils.Runtime, c Case) {
	t.Helper()

	hasFiles := c.WantFiles != nil || c.WantMode != nil || c.WantTime != nil ||
		c.Present != nil || c.Absent != nil
	if !hasFiles {
		return
	}
	if rt.FS == nil {
		t.Errorf("case checks files but sets no file system")
		return
	}

	for name, want := range c.WantFiles {
		got, err := fs.ReadFile(rt.FS, name)
		if err != nil {
			t.Errorf("ReadFile(%q) error = %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s contents = %q, want %q", name, got, want)
		}
	}
	for name, want := range c.WantMode {
		fi, err := fs.Stat(rt.FS, name)
		if err != nil {
			t.Errorf("Stat(%q) error = %v", name, err)
			continue
		}
		if got := fi.Mode(); got != want {
			t.Errorf("%s mode = %v, want %v", name, got, want)
		}
	}
	for name, want := range c.WantTime {
		fi, err := fs.Stat(rt.FS, name)
		if err != nil {
			t.Errorf("Stat(%q) error = %v", name, err)
			continue
		}
		if !want.IsZero() && !fi.ModTime().Equal(want) {
			t.Errorf("%s mod time = %v, want %v", name, fi.ModTime(), want)
		}
	}
	for _, name := range c.Present {
		if _, err := fs.Stat(rt.FS, name); err != nil {
			t.Errorf("Stat(%q) error = %v, want it to still exist", name, err)
		}
	}
	for _, name := range c.Absent {
		if _, err := fs.Stat(rt.FS, name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat(%q) error = %v, want fs.ErrNotExist", name, err)
		}
	}
}

// ExitCode returns the exit status of a Run call, 0 when it succeeded. It
// fails t when err is neither nil nor a *coreutils.ExitError.
func ExitCode(t *testing.T, err error) int {
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

// Cancelled runs new with a context that is cancelled before Run, checking
// that the context error is returned without any output. When stdin is a
// *CancelReader with a nil Cancel, that reader cancels the context instead,
// on its first read, so the cancellation happens while the input is read.
func Cancelled(t *testing.T, new coreutils.NewFunc, stdin io.Reader, fsys fsx.FS, args ...string) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	if r, ok := stdin.(*CancelReader); ok && r.Cancel == nil {
		r.Cancel = cancel
	} else {
		cancel()
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
	if fsys != nil {
		rt.FS = fsys
	}
	err := new(rt).Run(ctx, args...)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
}

// HelpVersion checks the two common options of a utility: --help must write
// the given usage text, and --version the standard version line, both on the
// standard output and with no error.
func HelpVersion(t *testing.T, new coreutils.NewFunc, prog, usage string) {
	t.Helper()

	t.Run("help", func(t *testing.T) {
		t.Helper()

		stdout := &bytes.Buffer{}
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: &bytes.Buffer{}}
		if err := new(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		t.Helper()

		stdout := &bytes.Buffer{}
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: &bytes.Buffer{}}
		if err := new(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := prog + " (charm.land/turtle/coreutils) " + coreutils.Version + "\nhttps://charm.land/turtle\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
