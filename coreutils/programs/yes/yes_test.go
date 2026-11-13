package yes

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
)

// stopWriter records what is written to it and stops a run after a given
// number of writes, either by canceling the context or by failing the write.
type stopWriter struct {
	buf      bytes.Buffer
	ctx      context.Context
	cancel   context.CancelFunc
	left     int
	writeErr error
}

func (w *stopWriter) Write(p []byte) (int, error) {
	if w.left <= 0 {
		w.cancel()
		if w.writeErr != nil {
			return 0, w.writeErr
		}
		return 0, w.ctx.Err()
	}
	w.left--
	w.buf.Write(p)
	if w.left == 0 && w.writeErr == nil {
		w.cancel()
	}
	return len(p), nil
}

// run runs yes with a context which times out if the loop doesn't stop by
// itself, writing at most the given number of writes before stopping it.
func run(t *testing.T, writes int, writeErr error, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	return runCtx(t, ctx, cancel, writes, writeErr, args...)
}

func runCtx(t *testing.T, ctx context.Context, cancel context.CancelFunc, writes int, writeErr error, args ...string) (string, string, error) {
	t.Helper()
	stdout := &stopWriter{ctx: ctx, cancel: cancel, left: writes, writeErr: writeErr}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
	err := New(rt).Run(ctx, args...)
	return stdout.buf.String(), stderr.String(), err
}

func TestRunRepeat(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		writes int
		want   string
	}{
		{
			name:   "no operands repeat y",
			writes: 4,
			want:   "y\n",
		},
		{
			name:   "a single operand",
			args:   []string{"foo"},
			writes: 2,
			want:   "foo\n",
		},
		{
			name:   "operands are separated by single spaces",
			args:   []string{"a", "b", "c"},
			writes: 3,
			want:   "a b c\n",
		},
		{
			name:   "empty operands",
			args:   []string{"", ""},
			writes: 2,
			want:   " \n",
		},
		{
			name:   "a lone dash is an operand",
			args:   []string{"-"},
			writes: 2,
			want:   "-\n",
		},
		{
			name:   "a double dash alone leaves the default operand",
			args:   []string{"--"},
			writes: 2,
			want:   "y\n",
		},
		{
			name:   "operands starting with a dash follow a double dash",
			args:   []string{"--", "--help"},
			writes: 2,
			want:   "--help\n",
		},
		{
			name:   "options after the first operand are operands",
			args:   []string{"foo", "--help"},
			writes: 2,
			want:   "foo --help\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, err := run(t, test.writes, nil, test.args...)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v, want the context error", err)
			}
			if _, ok := errors.AsType[*coreutils.ExitError](err); ok {
				t.Fatalf("Run() error = %v, want the context error as-is", err)
			}
			if stderr != "" {
				t.Errorf("Run() stderr = %q, want empty", stderr)
			}
			if want := strings.Repeat(test.want, test.writes); stdout != want {
				t.Errorf("Run() stdout = %q, want %q", stdout, want)
			}
		})
	}
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("broken pipe")
	stdout, stderr, err := run(t, 2, writeErr)

	if want := "y\ny\n"; stdout != want {
		t.Errorf("Run() stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("Run() stderr = %q, want empty", stderr)
	}
	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok {
		t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
	}
	if exitErr.Code != 1 {
		t.Errorf("Run() exit code = %d, want 1", exitErr.Code)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("Run() error = %v, want it wrapping %v", err, writeErr)
	}
}

func TestRunContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	stdout, stderr, err := runCtx(t, ctx, cancel, 0, nil, "foo")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want %v", err, context.Canceled)
	}
	if _, ok := errors.AsType[*coreutils.ExitError](err); ok {
		t.Fatalf("Run() error = %v, want the context error as-is", err)
	}
	if stdout != "" {
		t.Errorf("Run() stdout = %q, want empty", stdout)
	}
	if stderr != "" {
		t.Errorf("Run() stderr = %q, want empty", stderr)
	}
}

func TestRunOptionWriteError(t *testing.T) {
	writeErr := errors.New("broken pipe")
	tests := []struct {
		name string
		args []string
	}{
		{name: "the help output fails to be written", args: []string{"--help"}},
		{name: "the version output fails to be written", args: []string{"--version"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, err := run(t, 0, writeErr, test.args...)
			if stdout != "" {
				t.Errorf("Run() stdout = %q, want empty", stdout)
			}
			if stderr != "" {
				t.Errorf("Run() stderr = %q, want empty", stderr)
			}
			exitErr, ok := errors.AsType[*coreutils.ExitError](err)
			if !ok {
				t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
			}
			if exitErr.Code != 1 {
				t.Errorf("Run() exit code = %d, want 1", exitErr.Code)
			}
			if !errors.Is(err, writeErr) {
				t.Errorf("Run() error = %v, want it wrapping %v", err, writeErr)
			}
		})
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
		want := "yes (charm.land/turtle/coreutils) " + coreutils.Version + "\nhttps://charm.land/turtle\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}

func TestRunInvalidOptions(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "help has no short form",
			args:    []string{"-h"},
			wantErr: "yes: invalid option -- 'h'\n",
		},
		{
			name:    "version has no short form",
			args:    []string{"-v"},
			wantErr: "yes: invalid option -- 'v'\n",
		},
		{
			name:    "an unknown short option is rejected",
			args:    []string{"-x"},
			wantErr: "yes: invalid option -- 'x'\n",
		},
		{
			name:    "an unknown long option is rejected",
			args:    []string{"--bogus"},
			wantErr: "yes: unrecognized option '--bogus'\n",
		},
		{
			name:    "a long option doesn't take an argument",
			args:    []string{"--help=yes"},
			wantErr: "yes: unrecognized option '--help=yes'\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, err := run(t, 0, nil, test.args...)
			if stdout != "" {
				t.Errorf("Run() stdout = %q, want empty", stdout)
			}
			if stderr != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", stderr, test.wantErr)
			}
			exitErr, ok := errors.AsType[*coreutils.ExitError](err)
			if !ok {
				t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
			}
			if exitErr.Code != 1 {
				t.Errorf("Run() exit code = %d, want 1", exitErr.Code)
			}
		})
	}
}
