package time

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"charm.land/snail/coreutils"
)

// fakeClock replies with the scripted times in order, recording the calls.
type fakeClock struct {
	times []time.Time
	calls int
}

func (c *fakeClock) now() time.Time {
	c.calls++
	if len(c.times) == 0 {
		return time.Time{}
	}
	if c.calls > len(c.times) {
		return c.times[len(c.times)-1]
	}
	return c.times[c.calls-1]
}

// execRecord records an invocation of the stub Exec, replying with a reply
// function when given, or with the status and error held.
type execRecord struct {
	called bool
	dir    string
	name   string
	args   []string
	env    []string
	status int
	err    error
	reply  func(ctx context.Context) (int, error)
}

func (r *execRecord) exec(ctx context.Context, dir, name string, args, env []string) (int, error) {
	r.called, r.dir, r.name, r.args, r.env = true, dir, name, args, env
	if r.reply != nil {
		return r.reply(ctx)
	}
	return r.status, r.err
}

// newRuntime builds a runtime with the given clock and execution record. The
// environment is set only when environ is not nil, so the runs with no
// environment are covered too.
func newRuntime(clock *fakeClock, rec *execRecord, environ []string) *coreutils.Runtime {
	rt := &coreutils.Runtime{
		Stdin:  bytes.NewReader(nil),
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
		Clock:  clock.now,
	}
	if environ != nil {
		rt.Environ = func() []string { return environ }
	}
	if rec != nil {
		rt.Exec = rec.exec
	}
	return rt
}

func stdout(t *testing.T, rt *coreutils.Runtime) string {
	t.Helper()
	return rt.Stdout.(*bytes.Buffer).String()
}

func stderr(t *testing.T, rt *coreutils.Runtime) string {
	t.Helper()
	return rt.Stderr.(*bytes.Buffer).String()
}

func TestRun(t *testing.T) {
	// The base times: a run which starts at noon and lasts the duration the
	// test asks for.
	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		args     []string
		duration time.Duration
		status   int
		want     string
		wantCode int // -1 stands for a nil error, status 0
	}{
		{
			name:     "default format",
			args:     []string{"true"},
			duration: 1500 * time.Millisecond,
			want:     "real 1.50\nuser 0.00\nsys 0.00\n",
			wantCode: -1,
		},
		{
			name:     "-p format",
			args:     []string{"-p", "true"},
			duration: 1500 * time.Millisecond,
			want:     "real 1.50\nuser 0.00\nsys 0.00\n",
			wantCode: -1,
		},
		{
			name:     "-p grouped twice",
			args:     []string{"-pp", "true"},
			duration: time.Second,
			want:     "real 1.00\nuser 0.00\nsys 0.00\n",
			wantCode: -1,
		},
		{
			name:     "zero duration",
			args:     []string{"true"},
			duration: 0,
			want:     "real 0.00\nuser 0.00\nsys 0.00\n",
			wantCode: -1,
		},
		{
			name:     "subsecond fractions are kept",
			args:     []string{"true"},
			duration: 250 * time.Millisecond,
			want:     "real 0.25\nuser 0.00\nsys 0.00\n",
			wantCode: -1,
		},
		{
			name:     "long durations are seconds",
			args:     []string{"true"},
			duration: 61*time.Second + 250*time.Millisecond,
			want:     "real 61.25\nuser 0.00\nsys 0.00\n",
			wantCode: -1,
		},
		{
			name:     "the milliseconds are rounded",
			args:     []string{"true"},
			duration: 999 * time.Millisecond,
			want:     "real 1.00\nuser 0.00\nsys 0.00\n",
			wantCode: -1,
		},
		{
			name:     "nonzero exit status is passed along",
			args:     []string{"false"},
			duration: time.Second,
			status:   3,
			want:     "real 1.00\nuser 0.00\nsys 0.00\n",
			wantCode: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := &fakeClock{times: []time.Time{start, start.Add(test.duration)}}
			rec := &execRecord{status: test.status}
			rt := newRuntime(clock, rec, []string{"PATH=/bin"})

			err := New(rt).Run(t.Context(), test.args...)

			if test.wantCode < 0 {
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
			} else {
				var exitErr *coreutils.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
				}
				if exitErr.Code != test.wantCode {
					t.Errorf("Run() exit status = %d, want %d", exitErr.Code, test.wantCode)
				}
			}
			if got := stderr(t, rt); got != test.want {
				t.Errorf("Run() stderr = %q, want %q", got, test.want)
			}
			if got := stdout(t, rt); got != "" {
				t.Errorf("Run() stdout = %q, want empty", got)
			}
			if clock.calls != 2 {
				t.Errorf("clock called %d times, want 2", clock.calls)
			}
		})
	}
}

func TestRunInvocation(t *testing.T) {
	t.Run("the utility and its arguments are passed", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(&fakeClock{}, rec, nil)

		if err := New(rt).Run(t.Context(), "echo", "-n", "a", "b"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if rec.name != "echo" {
			t.Errorf("executed %q, want echo", rec.name)
		}
		if got, want := rec.args, []string{"-n", "a", "b"}; !slices.Equal(got, want) {
			t.Errorf("executed with args %v, want %v", got, want)
		}
	})

	t.Run("the -p after the utility is an argument", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(&fakeClock{}, rec, nil)

		if err := New(rt).Run(t.Context(), "true", "-p"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if rec.name != "true" {
			t.Errorf("executed %q, want true", rec.name)
		}
		if got, want := rec.args, []string{"-p"}; !slices.Equal(got, want) {
			t.Errorf("executed with args %v, want %v", got, want)
		}
	})

	t.Run("double dash makes an option-like utility be invoked", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(&fakeClock{}, rec, nil)

		if err := New(rt).Run(t.Context(), "--", "-p"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if rec.name != "-p" {
			t.Errorf("executed %q, want -p", rec.name)
		}
		if len(rec.args) != 0 {
			t.Errorf("executed with args %v, want none", rec.args)
		}
		if got, want := stderr(t, rt), "real 0.00\nuser 0.00\nsys 0.00\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the arguments of an option-like utility are passed", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(&fakeClock{}, rec, nil)

		if err := New(rt).Run(t.Context(), "--", "./-p-util", "-p", "arg"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if rec.name != "./-p-util" {
			t.Errorf("executed %q, want ./-p-util", rec.name)
		}
		if got, want := rec.args, []string{"-p", "arg"}; !slices.Equal(got, want) {
			t.Errorf("executed with args %v, want %v", got, want)
		}
	})

	t.Run("the utility runs in the working directory", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(&fakeClock{}, rec, nil)
		rt.WorkingDir = "/working/dir"

		if err := New(rt).Run(t.Context(), "true"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if rec.dir != "" {
			t.Errorf("executed in dir %q, want the empty one of the working directory", rec.dir)
		}
	})

	t.Run("the environment of the runtime is passed", func(t *testing.T) {
		rec := &execRecord{}
		environ := []string{"PATH=/bin", "FOO=bar"}
		rt := newRuntime(&fakeClock{}, rec, environ)

		if err := New(rt).Run(t.Context(), "true"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := rec.env, environ; !slices.Equal(got, want) {
			t.Errorf("executed with env %v, want %v", got, want)
		}
	})

	t.Run("no environment is passed when the runtime has none", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(&fakeClock{}, rec, nil)

		if err := New(rt).Run(t.Context(), "true"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if rec.env != nil {
			t.Errorf("executed with env %v, want nil", rec.env)
		}
	})
}

// notFoundErr and notInvokedErr stand for the errors the real execution
// reports for a utility which cannot be found or invoked.
var (
	notFoundErr   = errors.New(`exec: "missing": executable file not found in $PATH`)
	notInvokedErr = errors.New(`fork/exec /bin/missing: permission denied`)
	terminatedErr = errors.New("signal: segmentation fault")
)

func TestRunFailedExecution(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		err      error
		wantCode int
		wantErr  string
	}{
		{
			name:     "utility not found exits with 127",
			status:   127,
			err:      notFoundErr,
			wantCode: 127,
			wantErr:  "time: " + notFoundErr.Error() + "\n",
		},
		{
			name:     "utility not invocable exits with 126",
			status:   126,
			err:      notInvokedErr,
			wantCode: 126,
			wantErr:  "time: " + notInvokedErr.Error() + "\n",
		},
		{
			name:     "terminated by a signal exits with 125",
			status:   125,
			err:      terminatedErr,
			wantCode: 125,
			wantErr:  "time: " + terminatedErr.Error() + "\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := &fakeClock{times: []time.Time{{}, {}}}
			rec := &execRecord{status: test.status, err: test.err}
			rt := newRuntime(clock, rec, nil)

			err := New(rt).Run(t.Context(), "missing")

			var exitErr *coreutils.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
			}
			if exitErr.Code != test.wantCode {
				t.Errorf("Run() exit status = %d, want %d", exitErr.Code, test.wantCode)
			}
			if !errors.Is(err, test.err) {
				t.Errorf("Run() error = %v, want it to wrap %v", err, test.err)
			}
			if got := stderr(t, rt); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
			if got := stdout(t, rt); got != "" {
				t.Errorf("Run() stdout = %q, want empty", got)
			}
			if clock.calls != 2 {
				t.Errorf("clock called %d times, want 2", clock.calls)
			}
		})
	}
}

func TestUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing utility operand",
			wantErr: "time: missing operand\nTry 'time --help' for more information.\n",
		},
		{
			name:    "invalid option",
			args:    []string{"-x", "true"},
			wantErr: "time: invalid option -- 'x'\nTry 'time --help' for more information.\n",
		},
		{
			name:    "unrecognized long option",
			args:    []string{"--posix", "true"},
			wantErr: "time: unrecognized option '--posix'\nTry 'time --help' for more information.\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := &execRecord{}
			rt := newRuntime(&fakeClock{}, rec, nil)

			err := New(rt).Run(t.Context(), test.args...)

			var exitErr *coreutils.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
			}
			if exitErr.Code != 1 {
				t.Errorf("Run() exit status = %d, want 1", exitErr.Code)
			}
			if got := stderr(t, rt); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
			if rec.called {
				t.Errorf("executed %q, want no execution at all", rec.name)
			}
		})
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		rt := newRuntime(&fakeClock{}, nil, nil)

		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout(t, rt), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if got := stderr(t, rt); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
	})

	t.Run("help after -p", func(t *testing.T) {
		rt := newRuntime(&fakeClock{}, nil, nil)

		if err := New(rt).Run(t.Context(), "-p", "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout(t, rt), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		rt := newRuntime(&fakeClock{}, nil, nil)

		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "time (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout(t, rt); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}

func TestContextCancellation(t *testing.T) {
	t.Run("canceled context runs nothing", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		rec := &execRecord{}
		rt := newRuntime(&fakeClock{}, rec, nil)

		if err := New(rt).Run(ctx, "true"); err != context.Canceled {
			t.Fatalf("Run() error = %v, want the context error itself", err)
		}
		if rec.called {
			t.Errorf("executed %q, want no execution at all", rec.name)
		}
		if got := stderr(t, rt); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
	})

	t.Run("a canceled context error of the execution is returned as-is", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		rec := &execRecord{reply: func(ctx context.Context) (int, error) {
			return 125, errors.New("signal: killed")
		}}
		rt := newRuntime(&fakeClock{}, rec, nil)

		if err := New(rt).Run(ctx, "true"); err != context.Canceled {
			t.Fatalf("Run() error = %v, want the context error itself", err)
		}
		if got := stderr(t, rt); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
	})

	t.Run("an execution error which is not of the context is reported", func(t *testing.T) {
		rec := &execRecord{status: 127, err: notFoundErr}
		rt := newRuntime(&fakeClock{}, rec, nil)

		err := New(rt).Run(t.Context(), "missing")
		var exitErr *coreutils.ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 127 {
			t.Fatalf("Run() error = %v, want a *coreutils.ExitError with status 127", err)
		}
	})
}

func TestNoExec(t *testing.T) {
	rt := newRuntime(&fakeClock{}, nil, nil)

	err := New(rt).Run(t.Context(), "true")

	var exitErr *coreutils.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
	}
	if exitErr.Code != 125 {
		t.Errorf("Run() exit status = %d, want 125", exitErr.Code)
	}
	if got, want := stderr(t, rt), "time: no command execution available\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
	if got := stdout(t, rt); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
}
