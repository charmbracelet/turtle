// Package coreutils contains same types used by the core utilities
// implementation, including a common interface and context struct.
package coreutils

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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

	// Sleep blocks for a given duration or until the context is done. It's
	// used by utilities that wait for more input, like tail -f, so that tests
	// can replace the waiting instead of spending real time. If nil, a real
	// sleep is used.
	Sleep SleepFunc
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
	return &Runtime{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		FS:         fsx.OsFS(""),
		WorkingDir: wd,
		LookupEnv:  os.LookupEnv,
	}
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

// Getenv is a helper to retrieve an environment variable value without the
// extra bool return.
func (r *Runtime) Getenv(key string) string {
	v, _ := r.LookupEnv(key)
	return v
}

// LookupEnvFunc is a function signature for the LookupEnv function.
type LookupEnvFunc func(string) (string, bool)

// SleepFunc is a function signature for the Runtime.Sleep function.
type SleepFunc func(ctx context.Context, d time.Duration) error

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
