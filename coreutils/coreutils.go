// Package coreutils contains same types used by the core utilities
// implementation, including a common interface and context struct.
package coreutils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"charm.land/snail/fsx"
)

// NewFunc is a function signature that every core utility package should follow
// for their `package.New` funcs.
type NewFunc func(*Runtime) Program

// Runtime is a context struct containing core stuff that a core utility need
// to work.
type Runtime struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	FS         fsx.FS
	WorkingDir string
	LookupEnv  LookupEnvFunc

	// Umask is the file mode creation mask. It's used by utilities like chmod
	// when a symbolic mode doesn't specify who. The zero value means no
	// masking.
	Umask fs.FileMode

	// Clock returns the current time. It's used by utilities that need the
	// current time, like date, so tests can replace it. If nil, real time is
	// used.
	Clock ClockFunc

	// Sleep blocks for a given duration or until the context is done. It's
	// used by utilities that wait for more input, like tail -f, so that tests
	// can replace the waiting instead of spending real time. If nil, a real
	// sleep is used.
	Sleep SleepFunc

	// Exec runs a utility, waiting for its completion. It's used by
	// utilities that invoke other programs, like xargs. If nil, such
	// utilities fail instead of running anything.
	Exec ExecFunc
}

// OsRuntime returns a Runtime wired to the real operating system.
//
// WorkingDir is left empty when the current directory can't be determined, so
// relative paths still resolve through the process.
func OsRuntime() *Runtime {
	wd, err := os.Getwd()
	if err != nil {
		wd = ""
	}
	rt := &Runtime{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		FS:         fsx.OsFS(""),
		WorkingDir: wd,
		LookupEnv:  os.LookupEnv,
		Clock:      time.Now,
	}
	rt.Exec = func(ctx context.Context, name string, args []string) (int, error) {
		return osExec(ctx, rt, name, args)
	}
	return rt
}

// Wait blocks for d or until ctx is done, returning the context error when
// ctx finishes first. Utilities that wait for more input, like tail -f, must
// use it instead of time.Sleep, honoring Runtime.Sleep when it's set.
func (r *Runtime) Wait(ctx context.Context, d time.Duration) error {
	if r.Sleep != nil {
		return r.Sleep(ctx, d)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// ResolvePath resolves a path relative to the working directory.
// If path is absolute, it's returned untouched.
func (r *Runtime) ResolvePath(path string) string {
	switch {
	case r.WorkingDir == "":
		return path
	case filepath.IsAbs(path):
		return path
	default:
		return filepath.Join(r.WorkingDir, path)
	}
}

// Now returns the current time, honoring Runtime.Clock when set.
func (r *Runtime) Now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now()
}

// Getenv is a helper to retrieve an environment variable value without the
// extra bool return. An empty string is returned when no lookup function is
// set.
func (r *Runtime) Getenv(key string) string {
	if r.LookupEnv == nil {
		return ""
	}
	v, _ := r.LookupEnv(key)
	return v
}

// LookupEnvFunc is a function signature for the LookupEnv function.
type LookupEnvFunc func(string) (string, bool)

// ClockFunc is a function signature for the Runtime.Clock function.
type ClockFunc func() time.Time

// SleepFunc is a function signature for the Runtime.Sleep function.
type SleepFunc func(ctx context.Context, d time.Duration) error

// ExecFunc is a function signature for the Runtime.Exec function. It runs
// the named utility with the given arguments and waits for its completion,
// returning its exit status. When the utility cannot be found, cannot be
// invoked, or is terminated by a signal, it returns an error along with the
// status the caller should exit with: 127, 126 or 125, respectively.
type ExecFunc func(ctx context.Context, name string, args []string) (status int, err error)

// osExec runs a utility, wiring it to the runtime outputs and to the null
// device as its standard input, since the results of the utility reading
// the same input as xargs are unspecified.
func osExec(ctx context.Context, rt *Runtime, name string, args []string) (int, error) {
	path, err := lookup(rt, name)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return 127, err
		}
		return 126, err
	}
	status, err := run(ctx, rt, path, args)
	if err != nil && errors.Is(err, syscall.ENOEXEC) && runtime.GOOS != "windows" {
		// An executable file with no executable format runs through the
		// command interpreter, as execvp does.
		return run(ctx, rt, "/bin/sh", append([]string{path}, args...))
	}
	return status, err
}

// run starts the utility and waits for its completion, wiring it to the
// runtime outputs and to the null device as its standard input.
func run(ctx context.Context, rt *Runtime, path string, args []string) (int, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = rt.WorkingDir
	cmd.Stdout = rt.Stdout
	cmd.Stderr = rt.Stderr
	if f, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0); err == nil {
		cmd.Stdin = f
		defer f.Close()
	}
	if err := cmd.Run(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			if code := exitErr.ExitCode(); code >= 0 {
				return code, nil
			}
			return 125, exitErr
		}
		if errors.Is(err, fs.ErrNotExist) {
			return 127, err
		}
		return 126, err
	}
	return 0, nil
}

// defaultPath is the search path used when PATH is unset, like the one
// confstr(_CS_PATH) yields on Unix systems.
const defaultPath = "/bin:/usr/bin"

// lookup finds the utility in the PATH of the runtime, like exec.LookPath
// does for the process one. Names holding a path separator are returned as
// they are, so they run relative to the runtime working directory, which is
// also where the relative PATH entries are resolved. An empty PATH means
// the current directory, and an unset one the default search path.
func lookup(rt *Runtime, name string) (string, error) {
	if filepath.IsAbs(name) || strings.ContainsRune(name, '/') ||
		runtime.GOOS == "windows" && strings.ContainsRune(name, '\\') {
		return name, nil
	}
	// The working directory is made absolute, so the found candidates keep
	// working when the utility runs with it set.
	wd, err := filepath.Abs(rt.WorkingDir)
	if err != nil {
		wd = rt.WorkingDir
	}
	path, ok := "", false
	if rt.LookupEnv != nil {
		path, ok = rt.LookupEnv("PATH")
	}
	if !ok {
		path = defaultPath
	}
	dirs := filepath.SplitList(path)
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	var accessErr error
	for _, dir := range dirs {
		if dir == "" {
			dir = "."
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(wd, dir)
		}
		path := filepath.Join(dir, name)
		if runtime.GOOS == "windows" {
			for _, p := range []string{path + ".exe", path} {
				if info, err := os.Stat(p); err == nil && !info.IsDir() {
					return p, nil
				}
			}
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				accessErr = &exec.Error{Name: path, Err: err}
			}
			continue
		}
		if info.IsDir() {
			continue
		}
		if info.Mode().Perm()&0o0111 == 0 {
			accessErr = &exec.Error{Name: path, Err: fs.ErrPermission}
			continue
		}
		return path, nil
	}
	if accessErr != nil {
		return "", accessErr
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}

// Program is an interface that every core utility should implement.
type Program interface {
	Run(ctx context.Context, args ...string) error
}

// ExitError is a custom error type that should ideally be used for every error
// returned by core utilities execution.
type ExitError struct {
	Code int
	Err  error
}

// Error implements the error interface.
func (e *ExitError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("exit status %d: %v", e.Code, e.Err)
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

// Unwrap implements the interface to make ExitError unwrapable.
func (e *ExitError) Unwrap() error {
	return e.Err
}
