package printenv

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"charm.land/snail/coreutils"
)

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

// runPrintenv runs printenv with the given environment and arguments,
// returning the standard output and the error.
func runPrintenv(t *testing.T, environ []string, args ...string) (string, string, error) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout:  stdout,
		Stderr:  stderr,
		Environ: func() []string { return environ },
	}
	err := New(rt).Run(t.Context(), args...)
	return stdout.String(), stderr.String(), err
}

// runPrintenvWriter runs printenv writing to w instead of a buffer.
func runPrintenvWriter(t *testing.T, w io.Writer, environ []string, args ...string) error {
	t.Helper()

	rt := &coreutils.Runtime{
		Stdout:  w,
		Stderr:  new(bytes.Buffer),
		Environ: func() []string { return environ },
	}
	return New(rt).Run(t.Context(), args...)
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
	environ := []string{"HOME=/home/dwc", "PATH=/bin:/usr/bin", "EMPTY=", "SHELL=/bin/bash"}

	t.Run("no operands", func(t *testing.T) {
		tests := []struct {
			name    string
			environ []string
			want    string
		}{
			{
				name:    "every entry is written in order",
				environ: environ,
				want:    "HOME=/home/dwc\nPATH=/bin:/usr/bin\nEMPTY=\nSHELL=/bin/bash\n",
			},
			{
				name:    "an empty environment writes nothing",
				environ: nil,
				want:    "",
			},
			{
				name:    "an environment with one entry",
				environ: []string{"FOO=1"},
				want:    "FOO=1\n",
			},
			{
				name:    "a value with newlines is written as is",
				environ: []string{"MULTI=a\nb"},
				want:    "MULTI=a\nb\n",
			},
			{
				name:    "entries with multi-byte characters",
				environ: []string{"MSG=ção"},
				want:    "MSG=ção\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, stderr, err := runPrintenv(t, test.environ)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
				if stderr != "" {
					t.Errorf("Run() stderr = %q, want empty", stderr)
				}
			})
		}
	})

	t.Run("values", func(t *testing.T) {
		tests := []struct {
			name    string
			env     []string
			args    []string
			want    string
			wantErr int
		}{
			{
				name:    "a single variable prints only its value",
				args:    []string{"HOME"},
				want:    "/home/dwc\n",
				wantErr: 0,
			},
			{
				name:    "multiple variables print one value per line",
				args:    []string{"HOME", "SHELL"},
				want:    "/home/dwc\n/bin/bash\n",
				wantErr: 0,
			},
			{
				name:    "a variable set to an empty value is found",
				args:    []string{"EMPTY"},
				want:    "\n",
				wantErr: 0,
			},
			{
				name:    "a missing variable prints nothing and exits 1",
				args:    []string{"MISSING"},
				want:    "",
				wantErr: 1,
			},
			{
				name:    "later operands are still processed after a missing one",
				args:    []string{"MISSING", "HOME"},
				want:    "/home/dwc\n",
				wantErr: 1,
			},
			{
				name:    "a missing variable between found ones",
				args:    []string{"HOME", "MISSING", "SHELL"},
				want:    "/home/dwc\n/bin/bash\n",
				wantErr: 1,
			},
			{
				name:    "the same variable may repeat",
				args:    []string{"HOME", "HOME"},
				want:    "/home/dwc\n/home/dwc\n",
				wantErr: 0,
			},
			{
				name:    "a value with an embedded newline is written as is",
				env:     []string{"MULTI=a\nb"},
				args:    []string{"MULTI"},
				want:    "a\nb\n",
				wantErr: 0,
			},
			{
				name:    "an empty name is a missing variable",
				args:    []string{""},
				want:    "",
				wantErr: 1,
			},
			{
				name:    "a lone dash is an operand, not an option",
				args:    []string{"-"},
				want:    "",
				wantErr: 1,
			},
			{
				name:    "an option-looking name after a variable is an operand",
				args:    []string{"HOME", "-0"},
				want:    "/home/dwc\n",
				wantErr: 1,
			},
			{
				name:    "a name with no environment at all is missing",
				env:     []string{},
				args:    []string{"HOME"},
				want:    "",
				wantErr: 1,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				env := environ
				if test.env != nil {
					env = test.env
				}
				got, stderr, err := runPrintenv(t, env, test.args...)
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
				if code := exitCode(t, err); code != test.wantErr {
					t.Errorf("Run() exit status = %d, want %d", code, test.wantErr)
				}
				if stderr != "" {
					t.Errorf("Run() stderr = %q, want empty", stderr)
				}
			})
		}
	})

	t.Run("zero", func(t *testing.T) {
		tests := []struct {
			name    string
			env     []string
			args    []string
			want    string
			wantErr int
		}{
			{
				name:    "-0 separates values with NUL",
				args:    []string{"-0", "HOME", "SHELL"},
				want:    "/home/dwc\x00/bin/bash\x00",
				wantErr: 0,
			},
			{
				name:    "--null separates values with NUL",
				args:    []string{"--null", "HOME", "SHELL"},
				want:    "/home/dwc\x00/bin/bash\x00",
				wantErr: 0,
			},
			{
				name:    "-0 with a missing variable still exits 1",
				args:    []string{"-0", "MISSING", "HOME"},
				want:    "/home/dwc\x00",
				wantErr: 1,
			},
			{
				name:    "-0 without operands separates entries with NUL",
				args:    []string{"-0"},
				want:    "HOME=/home/dwc\x00PATH=/bin:/usr/bin\x00EMPTY=\x00SHELL=/bin/bash\x00",
				wantErr: 0,
			},
			{
				name:    "-0 and --null together",
				args:    []string{"-0", "--null", "HOME"},
				want:    "/home/dwc\x00",
				wantErr: 0,
			},
			{
				name:    "a value with an embedded newline keeps NUL as separator",
				env:     []string{"MULTI=a\nb"},
				args:    []string{"-0", "MULTI"},
				want:    "a\nb\x00",
				wantErr: 0,
			},
			{
				name:    "-- ends the options",
				args:    []string{"-0", "--", "HOME"},
				want:    "/home/dwc\x00",
				wantErr: 0,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				env := environ
				if test.env != nil {
					env = test.env
				}
				got, stderr, err := runPrintenv(t, env, test.args...)
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
				if code := exitCode(t, err); code != test.wantErr {
					t.Errorf("Run() exit status = %d, want %d", code, test.wantErr)
				}
				if stderr != "" {
					t.Errorf("Run() stderr = %q, want empty", stderr)
				}
			})
		}
	})

	t.Run("usage errors", func(t *testing.T) {
		tests := []struct {
			name    string
			args    []string
			wantErr int
		}{
			{
				name:    "an unrecognized short option",
				args:    []string{"-x"},
				wantErr: 1,
			},
			{
				name:    "an unrecognized long option",
				args:    []string{"--bogus"},
				wantErr: 1,
			},
			{
				name:    "a long option taking an argument is rejected",
				args:    []string{"--null=x"},
				wantErr: 1,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, stderr, err := runPrintenv(t, environ, test.args...)
				if got != "" {
					t.Errorf("Run() stdout = %q, want empty", got)
				}
				if code := exitCode(t, err); code != test.wantErr {
					t.Errorf("Run() exit status = %d, want %d", code, test.wantErr)
				}
				if stderr == "" {
					t.Errorf("Run() stderr = %q, want a diagnostic", stderr)
				}
			})
		}
	})
}

func TestRunWriteError(t *testing.T) {
	environ := []string{"HOME=/home/dwc", "PATH=/bin:/usr/bin"}
	writeErr := errors.New("write error")

	t.Run("no operands", func(t *testing.T) {
		err := runPrintenvWriter(t, errorWriter{err: writeErr}, environ)
		if code := exitCode(t, err); code != 2 {
			t.Errorf("Run() exit status = %d, want 2", code)
		}
		if !errors.Is(err, writeErr) {
			t.Errorf("Run() error = %v, want it to wrap %v", err, writeErr)
		}
	})

	t.Run("values", func(t *testing.T) {
		err := runPrintenvWriter(t, errorWriter{err: writeErr}, environ, "HOME")
		if code := exitCode(t, err); code != 2 {
			t.Errorf("Run() exit status = %d, want 2", code)
		}
		if !errors.Is(err, writeErr) {
			t.Errorf("Run() error = %v, want it to wrap %v", err, writeErr)
		}
	})

	t.Run("a missing standard output", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stderr:  new(bytes.Buffer),
			Environ: func() []string { return environ },
			Stdout:  nil,
		}
		err := New(rt).Run(t.Context())
		if code := exitCode(t, err); code != 2 {
			t.Errorf("Run() exit status = %d, want 2", code)
		}
	})
}

func TestRunContext(t *testing.T) {
	environ := []string{"HOME=/home/dwc", "PATH=/bin:/usr/bin"}

	t.Run("canceled before values", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		rt := &coreutils.Runtime{
			Stdout:  new(bytes.Buffer),
			Stderr:  new(bytes.Buffer),
			Environ: func() []string { return environ },
		}
		err := New(rt).Run(ctx, "HOME")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want %v", err, context.Canceled)
		}
	})

	t.Run("canceled before entries", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		rt := &coreutils.Runtime{
			Stdout:  new(bytes.Buffer),
			Stderr:  new(bytes.Buffer),
			Environ: func() []string { return environ },
		}
		err := New(rt).Run(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want %v", err, context.Canceled)
		}
	})

	t.Run("canceled between values stops the run", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		stdout := new(cancelWriter{cancel: cancel})
		rt := &coreutils.Runtime{
			Stdout:  stdout,
			Stderr:  new(bytes.Buffer),
			Environ: func() []string { return environ },
		}
		err := New(rt).Run(ctx, "HOME", "PATH")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want %v", err, context.Canceled)
		}
		if got := stdout.String(); got != "/home/dwc\n" {
			t.Errorf("Run() stdout = %q, want only the first value", got)
		}
	})
}

// cancelWriter cancels a context after its first write.
type cancelWriter struct {
	cancel context.CancelFunc
	buf    bytes.Buffer
}

func (w *cancelWriter) Write(b []byte) (int, error) {
	n, err := w.buf.Write(b)
	w.cancel()
	return n, err
}

func (w *cancelWriter) String() string {
	return w.buf.String()
}

func TestRunUsageError(t *testing.T) {
	stdout, stderr, err := runPrintenv(t, nil, "-x")
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if got := stdout; got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
	want := "printenv: invalid option -- 'x'\nTry 'printenv --help' for more information.\n"
	if got := stderr; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout, stderr, err := runPrintenv(t, nil, "--help")
		if err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout, usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if got := stderr; got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout, stderr, err := runPrintenv(t, nil, "--version")
		if err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "printenv (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if got := stderr; got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
	})
}
