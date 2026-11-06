package time

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
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

func (r *execRecord) exec(ctx context.Context, opts coreutils.ExecOpts) (int, error) {
	r.called, r.dir, r.name, r.args, r.env = true, opts.Dir, opts.Name, opts.Args, opts.Env
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

// usageStub reports a zero snapshot on the first call and the given usage on
// the later ones, so the delta of the two reports bracketing an execution is
// the usage itself. The number of calls is returned, for the tests checking
// the bracketing.
func usageStub(after coreutils.ExecUsage) (coreutils.ExecUsageFunc, *int) {
	calls := 0
	return func() (coreutils.ExecUsage, bool) {
		calls++
		if calls == 1 {
			return coreutils.ExecUsage{}, true
		}
		return after, true
	}, &calls
}

// failedUsage reports that the usage counters are unavailable, as a runtime
// without them does.
func failedUsage() (coreutils.ExecUsage, bool) {
	return coreutils.ExecUsage{}, false
}

func stdout(t *testing.T, rt *coreutils.Runtime) string {
	t.Helper()
	return rt.Stdout.(*bytes.Buffer).String()
}

func stderr(t *testing.T, rt *coreutils.Runtime) string {
	t.Helper()
	return rt.Stderr.(*bytes.Buffer).String()
}

// exitCode reports the exit status an error stands for.
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
			name:     "the milliseconds are truncated",
			args:     []string{"true"},
			duration: 999 * time.Millisecond,
			want:     "real 0.99\nuser 0.00\nsys 0.00\n",
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
		{
			name:    "missing format argument",
			args:    []string{"-f"},
			wantErr: "time: option requires an argument -- 'f'\nTry 'time --help' for more information.\n",
		},
		{
			name:    "missing output argument",
			args:    []string{"--output"},
			wantErr: "time: option '--output' requires an argument\nTry 'time --help' for more information.\n",
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

// The usage the format tests report: every counter distinct, so the
// specifiers can be told apart.
var formatUsage = coreutils.ExecUsage{
	UserTime:            1200 * time.Millisecond,
	SysTime:             300 * time.Millisecond,
	MaxRSS:              4096 * 1024,
	MinorFaults:         5,
	MajorFaults:         2,
	VoluntarySwitches:   11,
	InvoluntarySwitches: 7,
	FSInputs:            3,
	FSOutputs:           1,
	SocketsReceived:     4,
	SocketsSent:         6,
	Swaps:               9,
	Signals:             13,
}

// runFormat runs time with the given arguments, with the format usage
// reported by the hook, an execution of true with the given arguments, a run
// of two seconds, and the given exit status.
func runFormat(t *testing.T, status int, args ...string) *coreutils.Runtime {
	t.Helper()
	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{times: []time.Time{start, start.Add(2 * time.Second)}}
	rec := &execRecord{status: status}
	rt := newRuntime(clock, rec, nil)
	rt.ExecUsage, _ = usageStub(formatUsage)
	if err := New(rt).Run(t.Context(), args...); err != nil {
		if _, ok := errors.AsType[*coreutils.ExitError](err); !ok {
			t.Fatalf("Run() error = %v", err)
		}
	}
	return rt
}

func TestFormatSpecifiers(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   string
	}{
		{"command line", "%C", "true one two\n"},
		{"average unshared data size", "%D", "0\n"},
		{"elapsed clock", "%E", "0:02.00\n"},
		{"elapsed seconds", "%e", "2.00\n"},
		{"major page faults", "%F", "2\n"},
		{"file system inputs", "%I", "3\n"},
		{"average total memory use", "%K", "0\n"},
		{"maximum resident set size", "%M", "4096\n"},
		{"file system outputs", "%O", "1\n"},
		{"percent of CPU", "%P", "75%\n"},
		{"minor page faults", "%R", "5\n"},
		{"system time", "%S", "0.30\n"},
		{"user time", "%U", "1.20\n"},
		{"swaps", "%W", "9\n"},
		{"average shared text size", "%X", "0\n"},
		{"page size", "%Z", fmt.Sprint(os.Getpagesize()) + "\n"},
		{"involuntary context switches", "%c", "7\n"},
		{"voluntary context switches", "%w", "11\n"},
		{"socket messages received", "%r", "4\n"},
		{"socket messages sent", "%s", "6\n"},
		{"signals delivered", "%k", "13\n"},
		{"exit status", "%x", "0\n"},
		{"termination type", "%Tt", "normal\n"},
		{"exit code of a normal exit", "%Tx", "0\n"},
		{"signal number of a normal exit", "%Tn", "\n"},
		{"signal name of a normal exit", "%Ts", "\n"},
		{"ok of a clean exit", "%To", "ok\n"},
		{"literal percent", "100%%", "100%\n"},
		{"tab escape", "a\\tb", "a\tb\n"},
		{"newline escape", "a\\nb", "a\nb\n"},
		{"backslash escape", "a\\\\b", "a\\b\n"},
		{"invalid specifier", "%Q", "?Q\n"},
		{"trailing percent", "x%", "x?\n"},
		{"invalid escape", "a\\qb", "a?\\qb\n"},
		{"trailing backslash", "a\\", "a?\\\n"},
		{"unknown termination kind", "%T?", "T?=unknown\n"},
		{"missing termination letter", "%T", "T=missing letter\n"},
		{"plain text is kept", "no specifiers", "no specifiers\n"},
		{"several specifiers", "%U user, %S sys", "1.20 user, 0.30 sys\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rt := runFormat(t, 0, "-f", test.format, "true", "one", "two")
			if got := stderr(t, rt); got != test.want {
				t.Errorf("Run() stderr = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFormatElapsedAndPercent(t *testing.T) {
	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		duration time.Duration
		cpu      time.Duration
		wantE    string
		wante    string
		wantP    string
	}{
		{
			name:  "no time at all",
			wantE: "0:00.00", wante: "0.00", wantP: "?%",
		},
		{
			name:     "under a minute",
			duration: 6500 * time.Millisecond,
			wantE:    "0:06.50", wante: "6.50", wantP: "0%",
		},
		{
			name:     "over a minute",
			duration: (65*time.Second + 500*time.Millisecond),
			wantE:    "1:05.50", wante: "65.50", wantP: "0%",
		},
		{
			name:     "over an hour drops the fraction",
			duration: (1*time.Hour + time.Minute + time.Second + 500*time.Millisecond),
			wantE:    "1:01:01", wante: "3661.50", wantP: "0%",
		},
		{
			name:     "the CPU got more than the wall clock",
			duration: 500 * time.Millisecond,
			cpu:      2 * time.Second,
			wantE:    "0:00.50", wante: "0.50", wantP: "400%",
		},
		{
			name:     "the fractional percent is rounded down",
			duration: 3 * time.Second,
			cpu:      time.Second,
			wantE:    "0:03.00", wante: "3.00", wantP: "33%",
		},
		{
			name:     "just under twice the wall clock",
			duration: time.Millisecond + 1000*time.Nanosecond,
			cpu:      time.Millisecond + 999*time.Microsecond,
			wantE:    "0:00.00", wante: "0.00", wantP: "199%",
		},
		{
			name:     "the cpu equals the wall clock",
			duration: time.Second,
			cpu:      time.Second,
			wantE:    "0:01.00", wante: "1.00", wantP: "100%",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := &fakeClock{times: []time.Time{start, start.Add(test.duration)}}
			rec := &execRecord{}
			rt := newRuntime(clock, rec, nil)
			rt.ExecUsage, _ = usageStub(coreutils.ExecUsage{UserTime: test.cpu})

			if err := New(rt).Run(t.Context(), "-f", "%E|%e|%P", "true"); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			want := test.wantE + "|" + test.wante + "|" + test.wantP + "\n"
			if got := stderr(t, rt); got != want {
				t.Errorf("Run() stderr = %q, want %q", got, want)
			}
		})
	}
}

// TestPercentExtremes runs the percent of CPU on the borderline durations,
// where the naive arithmetic of the ratio would overflow.
func TestPercentExtremes(t *testing.T) {
	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	run := func(t *testing.T, elapsed time.Duration, usage coreutils.ExecUsage) string {
		t.Helper()
		clock := &fakeClock{times: []time.Time{start, start.Add(elapsed)}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, nil)
		rt.ExecUsage, _ = usageStub(usage)
		if err := New(rt).Run(t.Context(), "-f", "%P", "true"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		return stderr(t, rt)
	}

	t.Run("nine years of CPU in ten years of wall clock", func(t *testing.T) {
		elapsed := 10 * 365 * 24 * time.Hour
		usage := coreutils.ExecUsage{UserTime: 9 * 365 * 24 * time.Hour}
		if got, want := run(t, elapsed, usage), "90%\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the maximal durations are equal", func(t *testing.T) {
		usage := coreutils.ExecUsage{UserTime: time.Duration(math.MaxInt64)}
		if got, want := run(t, time.Duration(math.MaxInt64), usage), "100%\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the user and system times saturate their sum", func(t *testing.T) {
		// The sum of the two times overflows, saturating at the largest
		// duration, and the percent stays sane against two centuries of
		// wall clock.
		elapsed := 200 * 365 * 24 * time.Hour
		usage := coreutils.ExecUsage{
			UserTime: time.Duration(math.MaxInt64),
			SysTime:  5 * time.Hour,
		}
		if got, want := run(t, elapsed, usage), "146%\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the percent saturates instead of overflowing", func(t *testing.T) {
		// A nanosecond of wall clock cannot tell the percent of the
		// maximal CPU time; the quotient itself would overflow.
		usage := coreutils.ExecUsage{UserTime: time.Duration(math.MaxInt64)}
		if got, want := run(t, time.Nanosecond, usage),
			"18446744073709551615%\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})
}

func TestDefaultFormat(t *testing.T) {
	t.Run("the compact format of GNU time", func(t *testing.T) {
		rt := runFormat(t, 0, "true")
		want := "1.20user 0.30system 0:02.00elapsed 75%CPU " +
			"(0avgtext+0avgdata 4096maxresident)k\n" +
			"3inputs+1outputs (2major+5minor)pagefaults 9swaps\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the usage is taken from the delta of the two reports", func(t *testing.T) {
		start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		clock := &fakeClock{times: []time.Time{start, start.Add(time.Second)}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, nil)
		hook, calls := usageStub(coreutils.ExecUsage{UserTime: time.Second, SysTime: time.Second})
		rt.ExecUsage = hook

		if err := New(rt).Run(t.Context(), "true"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if *calls != 2 {
			t.Errorf("usage reported %d times, want 2", *calls)
		}
		want := "1.00user 1.00system 0:01.00elapsed 200%CPU " +
			"(0avgtext+0avgdata 0maxresident)k\n" +
			"0inputs+0outputs (0major+0minor)pagefaults 0swaps\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("an out-of-order delta saturates at zero", func(t *testing.T) {
		start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		clock := &fakeClock{times: []time.Time{start, start.Add(time.Second)}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, nil)
		// The second snapshot holds less than the first one, so the delta
		// of every counter saturates at zero, and the maximum resident set
		// keeps the larger size.
		calls := 0
		rt.ExecUsage = func() (coreutils.ExecUsage, bool) {
			calls++
			if calls == 1 {
				return coreutils.ExecUsage{
					UserTime:    2 * time.Second,
					MaxRSS:      1024,
					MinorFaults: 9,
				}, true
			}
			return coreutils.ExecUsage{
				UserTime:    time.Second,
				MaxRSS:      512,
				MinorFaults: 4,
			}, true
		}

		if err := New(rt).Run(t.Context(), "true"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		want := "0.00user 0.00system 0:01.00elapsed 0%CPU " +
			"(0avgtext+0avgdata 1maxresident)k\n" +
			"0inputs+0outputs (0major+0minor)pagefaults 0swaps\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})
}

func TestUsageUnavailable(t *testing.T) {
	start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	newUnavailableRuntime := func() *coreutils.Runtime {
		clock := &fakeClock{times: []time.Time{start, start.Add(2 * time.Second)}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, nil)
		rt.ExecUsage = failedUsage
		return rt
	}

	t.Run("the default degrades to the POSIX format", func(t *testing.T) {
		rt := newUnavailableRuntime()

		if err := New(rt).Run(t.Context(), "true"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		want := "real 2.00\nuser 0.00\nsys 0.00\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("an explicit format keeps the zeros", func(t *testing.T) {
		rt := newUnavailableRuntime()

		if err := New(rt).Run(t.Context(), "-f", "%U %S %M %F %R %I %O %w %c", "true"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		want := "0.00 0.00 0 0 0 0 0 0 0\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the elapsed time stays real", func(t *testing.T) {
		rt := newUnavailableRuntime()

		if err := New(rt).Run(t.Context(), "-f", "%e|%E", "true"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		want := "2.00|0:02.00\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("no warning is written for a nonzero status", func(t *testing.T) {
		clock := &fakeClock{times: []time.Time{start, start.Add(2 * time.Second)}}
		rec := &execRecord{status: 3}
		rt := newRuntime(clock, rec, nil)
		rt.ExecUsage = failedUsage

		err := New(rt).Run(t.Context(), "false")
		if code := exitCode(t, err); code != 3 {
			t.Errorf("Run() exit status = %d, want 3", code)
		}
		want := "real 2.00\nuser 0.00\nsys 0.00\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})
}

func TestTimeEnvironmentVariable(t *testing.T) {
	environ := func(timeValue string) []string {
		return []string{"PATH=/bin", "TIME=" + timeValue}
	}

	t.Run("the variable is the default format", func(t *testing.T) {
		clock := &fakeClock{times: []time.Time{{}, {}}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, environ("[%E]"))
		rt.ExecUsage = failedUsage

		if err := New(rt).Run(t.Context(), "true"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got, want := stderr(t, rt), "[0:00.00]\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("an empty variable gives an empty format", func(t *testing.T) {
		clock := &fakeClock{times: []time.Time{{}, {}}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, environ(""))
		rt.ExecUsage = failedUsage

		if err := New(rt).Run(t.Context(), "true"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got, want := stderr(t, rt), "\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the -f option overrides the variable", func(t *testing.T) {
		clock := &fakeClock{times: []time.Time{{}, {}}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, environ("[%E]"))
		rt.ExecUsage = failedUsage

		if err := New(rt).Run(t.Context(), "-f", "%C", "true"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got, want := stderr(t, rt), "true\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the -p option overrides the variable", func(t *testing.T) {
		clock := &fakeClock{times: []time.Time{{}, {}}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, environ("[%E]"))
		rt.ExecUsage = failedUsage

		if err := New(rt).Run(t.Context(), "-p", "true"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got, want := stderr(t, rt), "real 0.00\nuser 0.00\nsys 0.00\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the -v option overrides the variable", func(t *testing.T) {
		clock := &fakeClock{times: []time.Time{{}, {}}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, environ("[%E]"))
		rt.ExecUsage = failedUsage

		if err := New(rt).Run(t.Context(), "-v", "true"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got := stderr(t, rt); !strings.Contains(got, "\tExit status: 0\n") {
			t.Errorf("Run() stderr = %q, want the verbose report", got)
		}
	})
}

func TestVerbose(t *testing.T) {
	usage := coreutils.ExecUsage{
		SysTime:           50 * time.Millisecond,
		MaxRSS:            32 * 1024,
		MajorFaults:       3,
		VoluntarySwitches: 11,
		FSInputs:          3,
		FSOutputs:         1,
		Signals:           1,
	}
	want := "\tCommand being timed: \"sleep 4\"\n" +
		"\tUser time (seconds): 0.00\n" +
		"\tSystem time (seconds): 0.05\n" +
		"\tPercent of CPU this job got: 1%\n" +
		"\tElapsed (wall clock) time (h:mm:ss or m:ss): 0:04.26\n" +
		"\tAverage shared text size (kbytes): 0\n" +
		"\tAverage unshared data size (kbytes): 0\n" +
		"\tAverage stack size (kbytes): 0\n" +
		"\tAverage total size (kbytes): 0\n" +
		"\tMaximum resident set size (kbytes): 32\n" +
		"\tAverage resident set size (kbytes): 0\n" +
		"\tMajor (requiring I/O) page faults: 3\n" +
		"\tMinor (reclaiming a frame) page faults: 0\n" +
		"\tVoluntary context switches: 11\n" +
		"\tInvoluntary context switches: 0\n" +
		"\tSwaps: 0\n" +
		"\tFile system inputs: 3\n" +
		"\tFile system outputs: 1\n" +
		"\tSocket messages sent: 0\n" +
		"\tSocket messages received: 0\n" +
		"\tSignals delivered: 1\n" +
		fmt.Sprintf("\tPage size (bytes): %d\n", os.Getpagesize()) +
		"\tExit status: 0\n"

	run := func(t *testing.T, args ...string) *coreutils.Runtime {
		t.Helper()
		start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		clock := &fakeClock{times: []time.Time{
			start, start.Add(4*time.Second + 260*time.Millisecond),
		}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, nil)
		rt.ExecUsage, _ = usageStub(usage)
		if err := New(rt).Run(t.Context(), append(args, "sleep", "4")...); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		return rt
	}

	t.Run("the verbose report of GNU time", func(t *testing.T) {
		if got := stderr(t, run(t, "-v")); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the verbose format wins over -f in either order", func(t *testing.T) {
		if got := stderr(t, run(t, "-f", "%C", "-v")); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
		if got := stderr(t, run(t, "-v", "-f", "%C")); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})
}

func TestOutputFile(t *testing.T) {
	// A run of two seconds with no usage counters available, so the stats
	// are the three POSIX lines.
	stats := "real 2.00\nuser 0.00\nsys 0.00\n"

	run := func(t *testing.T, fsys fsx.MemFS, dir string, args ...string) *coreutils.Runtime {
		t.Helper()
		start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		clock := &fakeClock{times: []time.Time{start, start.Add(2 * time.Second)}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, nil)
		rt.FS = fsys
		rt.WorkingDir = dir
		rt.ExecUsage = failedUsage
		if err := New(rt).Run(t.Context(), args...); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		return rt
	}

	t.Run("the statistics are written to the file", func(t *testing.T) {
		fsys := fsx.MemFS{}
		rt := run(t, fsys, "", "-o", "out.txt", "true")

		f := fsys["out.txt"]
		if f == nil {
			t.Fatal("Run() didn't create the output file")
		}
		if got, want := string(f.Data), stats; got != want {
			t.Errorf("Run() file = %q, want %q", got, want)
		}
		if got := stderr(t, rt); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
	})

	t.Run("the file is overwritten by default", func(t *testing.T) {
		fsys := fsx.MemFS{"out.txt": {Data: []byte("old\n")}}
		run(t, fsys, "", "-o", "out.txt", "true")

		if got, want := string(fsys["out.txt"].Data), stats; got != want {
			t.Errorf("Run() file = %q, want %q", got, want)
		}
	})

	t.Run("the -a option appends", func(t *testing.T) {
		fsys := fsx.MemFS{"out.txt": {Data: []byte("old\n")}}
		run(t, fsys, "", "-o", "out.txt", "-a", "true")

		if got, want := string(fsys["out.txt"].Data), "old\n"+stats; got != want {
			t.Errorf("Run() file = %q, want %q", got, want)
		}
	})

	t.Run("the -a option alone is a no-op", func(t *testing.T) {
		rt := run(t, fsx.MemFS{}, "", "-a", "true")

		if got, want := stderr(t, rt), stats; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the file is relative to the working directory", func(t *testing.T) {
		fsys := fsx.MemFS{"work": {Mode: fs.ModeDir | 0o755}}
		run(t, fsys, "work", "--output=out.txt", "true")

		f := fsys["work/out.txt"]
		if f == nil {
			t.Fatal("Run() didn't create the output file")
		}
		if got, want := string(f.Data), stats; got != want {
			t.Errorf("Run() file = %q, want %q", got, want)
		}
	})

	t.Run("the --output-file spelling is accepted", func(t *testing.T) {
		fsys := fsx.MemFS{}
		run(t, fsys, "", "--output-file=out.txt", "true")

		if fsys["out.txt"] == nil {
			t.Error("Run() didn't create the output file")
		}
	})

	t.Run("an empty output file name fails the run", func(t *testing.T) {
		for _, args := range [][]string{{"-o", ""}, {"--output="}} {
			t.Run(args[0], func(t *testing.T) {
				start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
				clock := &fakeClock{times: []time.Time{start, start.Add(2 * time.Second)}}
				rec := &execRecord{}
				rt := newRuntime(clock, rec, nil)
				rt.FS = fsx.MemFS{}

				err := New(rt).Run(t.Context(), append(args, "true")...)

				if code := exitCode(t, err); code != 125 {
					t.Errorf("Run() exit status = %d, want 125", code)
				}
				if got, want := stderr(t, rt), "time: open : invalid argument\n"; got != want {
					t.Errorf("Run() stderr = %q, want %q", got, want)
				}
				if rec.called {
					t.Errorf("executed %q, want no execution at all", rec.name)
				}
			})
		}
	})

	t.Run("a file which cannot be opened fails the run", func(t *testing.T) {
		start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		clock := &fakeClock{times: []time.Time{start, start.Add(2 * time.Second)}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, nil)
		rt.FS = fsx.MemFS{}

		err := New(rt).Run(t.Context(), "-o", "no/out.txt", "true")

		if code := exitCode(t, err); code != 125 {
			t.Errorf("Run() exit status = %d, want 125", code)
		}
		if got, want := stderr(t, rt),
			"time: open no/out.txt: file does not exist\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
		if rec.called {
			t.Errorf("executed %q, want no execution at all", rec.name)
		}
	})

	t.Run("no file system fails the run", func(t *testing.T) {
		start := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		clock := &fakeClock{times: []time.Time{start, start.Add(2 * time.Second)}}
		rec := &execRecord{}
		rt := newRuntime(clock, rec, nil)

		err := New(rt).Run(t.Context(), "-o", "out.txt", "true")

		if code := exitCode(t, err); code != 125 {
			t.Errorf("Run() exit status = %d, want 125", code)
		}
		if got, want := stderr(t, rt), "time: no file system available\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
		if rec.called {
			t.Errorf("executed %q, want no execution at all", rec.name)
		}
	})
}

func TestTerminationStatus(t *testing.T) {
	t.Run("a nonzero status is announced ahead of the statistics", func(t *testing.T) {
		rt := runFormat(t, 3, "false")

		want := "Command exited with non-zero status 3\n" +
			"1.20user 0.30system 0:02.00elapsed 75%CPU " +
			"(0avgtext+0avgdata 4096maxresident)k\n" +
			"3inputs+1outputs (2major+5minor)pagefaults 9swaps\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the -p format is not announced", func(t *testing.T) {
		rt := runFormat(t, 3, "-p", "false")

		want := "real 2.00\nuser 1.20\nsys 0.30\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the -q option silences the announcement", func(t *testing.T) {
		rt := runFormat(t, 3, "-q", "false")

		want := "1.20user 0.30system 0:02.00elapsed 75%CPU " +
			"(0avgtext+0avgdata 4096maxresident)k\n" +
			"3inputs+1outputs (2major+5minor)pagefaults 9swaps\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("an explicit format is announced", func(t *testing.T) {
		rt := runFormat(t, 7, "-f", "%x", "false")

		want := "Command exited with non-zero status 7\n7\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	// GNU time suppresses the warning by comparing the format with the
	// POSIX one by pointer identity, so only the -p option itself selects
	// the POSIX mode, never a format string of the same text.
	posixText := "real %e\nuser %U\nsys %S"

	t.Run("-p followed by -f of the same text is announced", func(t *testing.T) {
		rt := runFormat(t, 3, "-p", "-f", posixText, "false")

		want := "Command exited with non-zero status 3\nreal 2.00\nuser 1.20\nsys 0.30\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("-f of the POSIX text alone is announced", func(t *testing.T) {
		rt := runFormat(t, 3, "-f", posixText, "false")

		want := "Command exited with non-zero status 3\nreal 2.00\nuser 1.20\nsys 0.30\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("a zero status is never announced", func(t *testing.T) {
		rt := runFormat(t, 0, "-f", "%Tt|%Tx|%Tn|%Ts|%To", "true")

		if got, want := stderr(t, rt), "normal|0|||ok\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the termination status of a nonzero exit", func(t *testing.T) {
		rt := runFormat(t, 7, "-f", "%Tt|%Tx|%Tn|%Ts|%To", "false")

		want := "Command exited with non-zero status 7\nnormal|7|||\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})
}
