// Package coreutils contains same types used by the core utilities
// implementation, including a common interface and context struct.
package coreutils

import (
	"context"
	_ "embed"
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

	"charm.land/turtle/coreutils/internal/term"
	"charm.land/turtle/fsx"
)

//go:embed version.txt
var versionText string

// Version is the version of the core utilities, as kept in version.txt.
var Version = strings.TrimSpace(versionText)

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

	// Environ enumerates the environment of the runtime, as "KEY=VALUE"
	// strings. If nil, the environment is treated as empty.
	Environ EnvironFunc

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

	// IsTerminal reports whether the given file descriptor number is open
	// and associated with a terminal. It's used by utilities that test their
	// standard streams, like test. If nil, the descriptors 0, 1 and 2 are
	// checked against Stdin, Stdout and Stderr with a platform-specific
	// terminal check.
	IsTerminal func(fd int) bool

	// ProcessAlive reports whether the process with the given ID exists.
	// It's used by utilities that watch other processes, like tail --pid.
	// If nil, such watching is unsupported.
	ProcessAlive func(pid int) bool

	// Exec runs a utility, waiting for its completion. It's used by
	// utilities that invoke other programs, like xargs, env and find's
	// -exec and -execdir primaries. If nil, such utilities fail instead of
	// running anything.
	Exec ExecFunc

	// ExecUsage reports the cumulative resource usage of the processes
	// executed so far, like getrusage(RUSAGE_CHILDREN). Utilities that
	// time an application, like time, bracket their Exec call with two
	// reports and subtract them to get the usage of one execution. If nil,
	// such usage is unavailable.
	ExecUsage ExecUsageFunc
}

// OsRuntime returns a Runtime wired to the real operating system, using
// workingDir as the working directory. An empty workingDir means the current
// working directory, which is left empty when it can't be determined. A
// relative workingDir is made absolute when possible.
func OsRuntime(workingDir string) *Runtime {
	wd := workingDir
	if wd == "" {
		var err error
		wd, err = os.Getwd()
		if err != nil {
			wd = ""
		}
	} else if abs, err := filepath.Abs(wd); err == nil {
		wd = abs
	}
	rt := &Runtime{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		FS:         fsx.OsFS(""),
		WorkingDir: wd,
		Environ:    os.Environ,
		Clock:      time.Now,
	}
	rt.Exec = func(ctx context.Context, opts ExecOpts) (int, error) {
		return osExec(ctx, rt, opts)
	}
	rt.ProcessAlive = osProcessAlive
	rt.ExecUsage = osExecUsage
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

// Terminal reports whether the given file descriptor number is open and
// associated with a terminal, honoring Runtime.IsTerminal when set.
func (r *Runtime) Terminal(fd int) bool {
	if r.IsTerminal != nil {
		return r.IsTerminal(fd)
	}
	var stream any
	switch fd {
	case 0:
		stream = r.Stdin
	case 1:
		stream = r.Stdout
	case 2:
		stream = r.Stderr
	default:
		return false
	}
	f, ok := stream.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// Now returns the current time, honoring Runtime.Clock when set.
func (r *Runtime) Now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now()
}

// LookupEnv returns the value of an environment variable of the runtime,
// reporting whether it's set. It returns "", false when Environ is nil.
func (r *Runtime) LookupEnv(key string) (string, bool) {
	if r.Environ == nil {
		return "", false
	}
	for _, entry := range r.Environ() {
		if k, v, _ := strings.Cut(entry, "="); k == key {
			return v, true
		}
	}
	return "", false
}

// Getenv returns the value of an environment variable of the runtime, or
// the empty string when it's not set.
func (r *Runtime) Getenv(key string) string {
	v, _ := r.LookupEnv(key)
	return v
}

// EnvList returns the environment of the runtime, as "KEY=VALUE" strings,
// to give to Exec. It returns nil when there's no environment.
func (r *Runtime) EnvList() []string {
	if r.Environ == nil {
		return nil
	}
	return r.Environ()
}

// EnvironFunc is a function signature for the Runtime.Environ function.
type EnvironFunc func() []string

// ClockFunc is a function signature for the Runtime.Clock function.
type ClockFunc func() time.Time

// SleepFunc is a function signature for the Runtime.Sleep function.
type SleepFunc func(ctx context.Context, d time.Duration) error

// ExecOpts describes one execution of a utility.
type ExecOpts struct {
	// Name is the utility to run, resolved in its PATH.
	Name string

	// Args are the arguments to run the utility with.
	Args []string

	// Dir is the directory to run the utility in. An empty one stands for
	// the working directory.
	Dir string

	// Env is the exact environment to run the utility with, as "KEY=VALUE"
	// strings. A nil one means an empty environment, never the process one.
	Env []string

	// Stdin is the standard input of the utility. A nil one is the null
	// device, since the results of the utility reading the same input as
	// its caller are unspecified.
	Stdin io.Reader

	// Stdout is the standard output of the utility. A nil one is the
	// runtime's Stdout.
	Stdout io.Writer

	// Stderr is the standard error of the utility. A nil one is the
	// runtime's Stderr.
	Stderr io.Writer
}

// ExecFunc is a function signature for the Runtime.Exec function. It runs
// the utility described by opts, waiting for its completion and returning
// its exit status. When the utility cannot be found, cannot be invoked, or
// is terminated by a signal, it returns an error along with the status the
// caller should exit with: 127, 126 or 125, respectively.
type ExecFunc func(ctx context.Context, opts ExecOpts) (status int, err error)

func osExec(ctx context.Context, rt *Runtime, opts ExecOpts) (int, error) {
	path, err := lookup(rt, opts.Env, opts.Name)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return 127, err
		}
		return 126, err
	}
	opts.Name = path
	status, err := run(ctx, rt, opts)
	if err != nil && errors.Is(err, syscall.ENOEXEC) && runtime.GOOS != "windows" {
		// An executable file with no executable format runs through the
		// command interpreter, as execvp does.
		opts.Args = append([]string{opts.Name}, opts.Args...)
		opts.Name = "/bin/sh"
		return run(ctx, rt, opts)
	}
	return status, err
}

// run starts the utility named in opts and waits for its completion, wiring
// its standard streams as the opts zero values require. An empty Dir makes
// it run in the working directory.
func run(ctx context.Context, rt *Runtime, opts ExecOpts) (int, error) {
	cmd := exec.CommandContext(ctx, opts.Name, opts.Args...)
	cmd.Dir = opts.Dir
	if cmd.Dir == "" {
		cmd.Dir = rt.WorkingDir
	}
	cmd.Env = opts.Env
	if cmd.Env == nil {
		// A nil Env would make the process environment be inherited.
		cmd.Env = []string{}
	}
	cmd.Stdout = opts.Stdout
	if cmd.Stdout == nil {
		cmd.Stdout = rt.Stdout
	}
	cmd.Stderr = opts.Stderr
	if cmd.Stderr == nil {
		cmd.Stderr = rt.Stderr
	}
	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	} else if f, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0); err == nil {
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
		// On Windows, a path to a missing utility can fail the extension
		// lookup with ErrNotFound instead of fs.ErrNotExist.
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, exec.ErrNotFound) {
			return 127, err
		}
		return 126, err
	}
	return 0, nil
}

// defaultPath is the search path used when PATH is unset, like the one
// confstr(_CS_PATH) yields on Unix systems.
const defaultPath = "/bin:/usr/bin"

// lookupEnvVar returns the value of the variable named key in a list of
// "KEY=VALUE" strings, reporting whether it's set.
func lookupEnvVar(env []string, key string) (string, bool) {
	for _, entry := range env {
		if k, v, _ := strings.Cut(entry, "="); k == key {
			return v, true
		}
	}
	return "", false
}

// lookup finds the utility in the PATH of the environment it runs with,
// like exec.LookPath does for the process one. The PATH comes from the
// given environment when it's not nil, as env requires for a PATH operand,
// or from the runtime one otherwise. Names holding a path separator are
// returned as they are, so they run relative to the runtime working
// directory, which is also where the relative PATH entries are resolved. An
// empty PATH means the current directory, and an unset one the default
// search path.
func lookup(rt *Runtime, env []string, name string) (string, error) {
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
	if env != nil {
		path, ok = lookupEnvVar(env, "PATH")
	} else {
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
