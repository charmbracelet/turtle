package timeout

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/snail/coreutils"
)

// errKilled is the error an execution reports for the child process killed
// by the cancellation of its context.
var errKilled = errors.New("signal: killed")

// execRecorder records an invocation of the stub Exec, replying with the
// given function.
type execRecorder struct {
	mu    sync.Mutex
	calls int
	ctx   context.Context
	name  string
	args  []string
	env   []string
	reply func(ctx context.Context) (int, error)
}

func (r *execRecorder) exec(ctx context.Context, opts coreutils.ExecOpts) (int, error) {
	r.mu.Lock()
	r.calls++
	r.ctx, r.name, r.args, r.env = ctx, opts.Name, opts.Args, opts.Env
	r.mu.Unlock()
	return r.reply(ctx)
}

// count returns how many times the execution was invoked.
func (r *execRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// contextErr returns the error of the context the execution ran with.
func (r *execRecorder) contextErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx == nil {
		return nil
	}
	return r.ctx.Err()
}

// exits returns a reply where the command finishes at once with the status.
func exits(status int) func(context.Context) (int, error) {
	return func(context.Context) (int, error) { return status, nil }
}

// exitsLate returns a reply where the command runs until the execution
// context is done, and then exits with the status, as if it managed to exit
// in the race with the signal sent at the time limit.
func exitsLate(status int) func(context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		<-ctx.Done()
		return status, nil
	}
}

// killed returns a reply where the command runs until the execution context
// is done, and is then reported as terminated by a signal, as the real
// execution does when the context kills the process.
func killed() func(context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		<-ctx.Done()
		return 125, errKilled
	}
}

// fails returns a reply where the execution itself fails, with the status
// the caller should exit with.
func fails(status int, err error) func(context.Context) (int, error) {
	return func(context.Context) (int, error) { return status, err }
}

// hangs returns a reply standing for a command which never finishes on its
// own and survives being stopped: it returns only once release is closed,
// reporting on exited that it did, so its goroutine is always reaped by the
// test and nothing leaks.
func hangs(release <-chan struct{}, exited chan<- struct{}) func(context.Context) (int, error) {
	return func(context.Context) (int, error) {
		<-release
		exited <- struct{}{}
		return 125, errKilled
	}
}

// elapsedSleeper is a Sleep stub where every wait ends at once, as if the
// duration had elapsed. It records the durations asked for.
type elapsedSleeper struct {
	mu        sync.Mutex
	durations []time.Duration
}

func (s *elapsedSleeper) sleep(_ context.Context, d time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.durations = append(s.durations, d)
	return nil
}

// waited returns the durations the waits were asked for.
func (s *elapsedSleeper) waited() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.durations)
}

// onceElapsedSleeper is a Sleep stub where the first wait ends at once and
// the following ones block forever, as when the command exits during the
// kill grace period.
type onceElapsedSleeper struct {
	mu        sync.Mutex
	durations []time.Duration
}

func (s *onceElapsedSleeper) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	first := len(s.durations) == 0
	s.durations = append(s.durations, d)
	s.mu.Unlock()
	if first {
		return nil
	}
	// The run cancels the wait context when it returns, so the goroutine
	// blocking here always goes away.
	<-ctx.Done()
	return ctx.Err()
}

// blockedSleeper is a Sleep stub where every wait blocks forever, as when
// the durations are long enough that the command finishes first.
type blockedSleeper struct {
	mu        sync.Mutex
	durations []time.Duration
}

func (s *blockedSleeper) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.durations = append(s.durations, d)
	s.mu.Unlock()
	// The run cancels the wait context when it returns, so the goroutine
	// blocking here always goes away.
	<-ctx.Done()
	return ctx.Err()
}

// waited returns the durations the waits were asked for.
func (s *blockedSleeper) waited() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.durations)
}

// canceledSleeper is a Sleep stub where the wait reports the caller's
// context as canceled, as the real waiting does when it is done.
func canceledSleeper(context.Context, time.Duration) error {
	return context.Canceled
}

// graceSleeper is a Sleep stub where the first wait ends at once and the
// following ones report the caller's context as canceled, as when the
// caller is done during the kill grace period.
type graceSleeper struct {
	mu        sync.Mutex
	durations []time.Duration
}

func (s *graceSleeper) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	first := len(s.durations) == 0
	s.durations = append(s.durations, d)
	s.mu.Unlock()
	if first {
		return nil
	}
	return ctx.Err()
}

// waited returns the durations the waits were asked for.
func (s *graceSleeper) waited() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.durations)
}

// run builds a runtime with the given stubs and runs the utility with them.
func run(t *testing.T, exec coreutils.ExecFunc, sleep coreutils.SleepFunc, environ []string, args ...string) (string, string, error) {
	t.Helper()

	rt := &coreutils.Runtime{
		Stdout:  new(bytes.Buffer),
		Stderr:  new(bytes.Buffer),
		Environ: func() []string { return environ },
	}
	if exec != nil {
		rt.Exec = exec
	}
	if sleep != nil {
		rt.Sleep = sleep
	}
	err := New(rt).Run(t.Context(), args...)
	return rt.Stdout.(*bytes.Buffer).String(), rt.Stderr.(*bytes.Buffer).String(), err
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

func TestParseDuration(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			operand  string
			want     time.Duration
			infinite bool
		}{
			{"0", 0, false},
			{"5", 5 * time.Second, false},
			{"3.5", 3500 * time.Millisecond, false},
			{"2s", 2 * time.Second, false},
			{"2m", 2 * time.Minute, false},
			{"2h", 2 * time.Hour, false},
			{"2d", 48 * time.Hour, false},
			{"1.5m", 90 * time.Second, false},
			{"0.5d", 12 * time.Hour, false},
			{"1e2", 100 * time.Second, false},
			{"0x1p0d", 24 * time.Hour, false},
			{"inf", 0, true},
			{"INF", 0, true},
			{"infd", 0, true},
			{"1e-300", 1, false},
			{"1e-400", 1, false},
			{"1e-400d", 1, false},
			{"0x1p-5000", 1, false},
			{"0xap-2000", 1, false},
			{"0xap-2000d", 1, false},
			{"0xep-2000", 1, false},
			{"0XAP-2000", 1, false},
			{"0x0p-5000", 0, false},
			{"0x0.0p-5000", 0, false},
			{"0e1", 0, false},
			{"0.0e-999", 0, false},
			{"1e30", 0, true},
			{"1e400", 0, true},
			{"100000000000", 0, true},
		}
		for _, test := range tests {
			t.Run(test.operand, func(t *testing.T) {
				t.Parallel()
				d, infinite, err := parseDuration(test.operand)
				if err != nil {
					t.Fatalf("parseDuration(%q) error = %v, want nil", test.operand, err)
				}
				if d != test.want || infinite != test.infinite {
					t.Errorf("parseDuration(%q) = %v, %v, want %v, %v",
						test.operand, d, infinite, test.want, test.infinite)
				}
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()
		for _, operand := range []string{"", "x", "1x", "5S", "5M", "5H", "5D", "-1", "-5s", "nan", "0x10", "1.2.3", " 1", "-1e400", "-1e-400", "-0xap-2000"} {
			_, _, err := parseDuration(operand)
			if err == nil {
				t.Errorf("parseDuration(%q) = nil error, want an error", operand)
				continue
			}
			if want := "invalid time interval '" + operand + "'"; err.Error() != want {
				t.Errorf("parseDuration(%q) error = %q, want %q", operand, err, want)
			}
		}
	})
}

func TestParseSignal(t *testing.T) {
	tests := []struct {
		spec    string
		want    int
		wantErr bool
	}{
		{"TERM", 15, false},
		{"term", 15, false},
		{"Term", 15, false},
		{"SIGTERM", 15, false},
		{"sigint", 2, false},
		{"HUP", 1, false},
		{"KILL", 9, false},
		{"ALRM", 14, false},
		{"15", 15, false},
		{"9", 9, false},
		{"64", 64, false},
		{"0", 0, true},
		{"65", 0, true},
		{"-1", 0, true},
		{"", 0, true},
		{"NOPE", 0, true},
		{"1.5", 0, true},
	}
	for _, test := range tests {
		t.Run(test.spec, func(t *testing.T) {
			t.Parallel()
			sig, err := parseSignal(test.spec)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseSignal(%q) = %d, want an error", test.spec, sig)
				}
				if want := "invalid signal spec '" + test.spec + "'"; err.Error() != want {
					t.Errorf("parseSignal(%q) error = %q, want %q", test.spec, err, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSignal(%q) error = %v, want nil", test.spec, err)
			}
			if sig != test.want {
				t.Errorf("parseSignal(%q) = %d, want %d", test.spec, sig, test.want)
			}
		})
	}
}

type completionCase struct {
	name     string
	args     []string
	reply    func(context.Context) (int, error)
	wantCmd  string
	wantExit int
	wantErr  string
	check    func(t *testing.T, rec *execRecorder, sleeper *blockedSleeper)
}

func TestRunCompletion(t *testing.T) {
	tests := []completionCase{
		{
			name:    "a successful command is silent",
			args:    []string{"5", "true"},
			reply:   exits(0),
			wantCmd: "true",
		},
		{
			name:     "the status of the command propagates",
			args:     []string{"5", "false"},
			reply:    exits(3),
			wantCmd:  "false",
			wantExit: 3,
		},
		{
			name:  "the arguments go to the command",
			args:  []string{"5", "cmd", "-x", "y"},
			reply: exits(0),
			check: func(t *testing.T, rec *execRecorder, _ *blockedSleeper) {
				checkArgs(t, rec, "-x", "y")
			},
		},
		{
			name:     "a command which cannot be found reports 127",
			args:     []string{"5", "nope"},
			reply:    fails(127, errKilled),
			wantCmd:  "nope",
			wantExit: 127,
			wantErr:  "timeout: nope: signal: killed\n",
		},
		{
			name:     "a command which cannot be invoked reports 126",
			args:     []string{"5", "cmd"},
			reply:    fails(126, errKilled),
			wantExit: 126,
			wantErr:  "timeout: cmd: signal: killed\n",
		},
		{
			name:     "a command terminated by another signal reports 125",
			args:     []string{"5", "cmd"},
			reply:    fails(125, errKilled),
			wantExit: 125,
			wantErr:  "timeout: cmd: signal: killed\n",
		},
		{
			name:    "options stop at the first operand",
			args:    []string{"5", "-v", "x"},
			reply:   exits(0),
			wantCmd: "-v",
			check: func(t *testing.T, rec *execRecorder, _ *blockedSleeper) {
				checkArgs(t, rec, "x")
			},
		},
		{
			name:    "-- before the duration starts the operands",
			args:    []string{"--", "5", "-v", "x"},
			reply:   exits(0),
			wantCmd: "-v",
			check: func(t *testing.T, rec *execRecorder, _ *blockedSleeper) {
				checkArgs(t, rec, "x")
			},
		},
		{
			name:    "-- after the duration is an operand like any other",
			args:    []string{"5", "--", "-o"},
			reply:   exits(0),
			wantCmd: "--",
			check: func(t *testing.T, rec *execRecorder, _ *blockedSleeper) {
				checkArgs(t, rec, "-o")
			},
		},
		{
			name:  "a fractional duration waits for the whole duration",
			args:  []string{"1.5", "cmd"},
			reply: exits(0),
		},
		{
			name:     "the duration of 0 enforces no time limit",
			args:     []string{"0", "cmd"},
			reply:    exits(7),
			wantExit: 7,
			check: func(t *testing.T, _ *execRecorder, sleeper *blockedSleeper) {
				if waited := sleeper.waited(); len(waited) != 0 {
					t.Errorf("Run() sleeps = %v, want none", waited)
				}
			},
		},
		{
			name:     "-f and -p change nothing on a completion",
			args:     []string{"-f", "-p", "5", "cmd"},
			reply:    exits(4),
			wantExit: 4,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			rec := &execRecorder{reply: test.reply}
			sleeper := &blockedSleeper{}
			stdout, stderr, err := run(t, rec.exec, sleeper.sleep, nil, test.args...)
			if stdout != "" {
				t.Errorf("Run() stdout = %q, want empty", stdout)
			}
			if stderr != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", stderr, test.wantErr)
			}
			if code := exitCode(t, err); code != test.wantExit {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantExit)
			}
			if rec.calls != 1 {
				t.Errorf("Run() executions = %d, want 1", rec.calls)
			}
			wantCmd := test.wantCmd
			if wantCmd == "" {
				wantCmd = "cmd"
			}
			if rec.name != wantCmd {
				t.Errorf("Run() command = %q, want %q", rec.name, wantCmd)
			}
			if test.check != nil {
				test.check(t, rec, sleeper)
			}
		})
	}

	t.Run("the environment is passed along", func(t *testing.T) {
		t.Parallel()
		rec := &execRecorder{reply: exits(0)}
		_, _, err := run(t, rec.exec, (&blockedSleeper{}).sleep, []string{"A=1", "B=2"}, "5", "cmd")
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if !slices.Equal(rec.env, []string{"A=1", "B=2"}) {
			t.Errorf("Run() env = %v, want [A=1 B=2]", rec.env)
		}
	})
}

func checkArgs(t *testing.T, rec *execRecorder, want ...string) {
	t.Helper()
	if !slices.Equal(rec.args, want) {
		t.Errorf("Run() args = %v, want %v", rec.args, want)
	}
}

func TestRunTimeout(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		reply    func(context.Context) (int, error)
		wantExit int
		wantErr  string
	}{
		{
			name:     "the time limit reports 124",
			args:     []string{"5", "cmd"},
			reply:    killed(),
			wantExit: 124,
		},
		{
			name:     "a duration too small to hold still times out",
			args:     []string{"1e-300", "cmd"},
			reply:    killed(),
			wantExit: 124,
		},
		{
			name:     "a hex duration too small to hold still times out",
			args:     []string{"0xap-2000d", "cmd"},
			reply:    killed(),
			wantExit: 124,
		},
		{
			name:     "--preserve-status mimics dying from the signal",
			args:     []string{"-p", "5", "cmd"},
			reply:    killed(),
			wantExit: 143,
		},
		{
			name:     "the long form of --preserve-status works",
			args:     []string{"--preserve-status", "5", "cmd"},
			reply:    killed(),
			wantExit: 143,
		},
		{
			name:     "the long form of --signal works",
			args:     []string{"-p", "--signal=INT", "5", "cmd"},
			reply:    killed(),
			wantExit: 130,
		},
		{
			name:     "--preserve-status takes the signal given",
			args:     []string{"-p", "-s", "INT", "5", "cmd"},
			reply:    killed(),
			wantExit: 130,
		},
		{
			name:     "--preserve-status takes the signal number given",
			args:     []string{"-p", "-s", "1", "5", "cmd"},
			reply:    killed(),
			wantExit: 129,
		},
		{
			name:     "--preserve-status keeps the status of a command exiting in the race",
			args:     []string{"-p", "5", "cmd"},
			reply:    exitsLate(7),
			wantExit: 7,
		},
		{
			name:     "without --preserve-status the race still reports 124",
			args:     []string{"5", "cmd"},
			reply:    exitsLate(7),
			wantExit: 124,
		},
		{
			name:     "-v reports the signal sent",
			args:     []string{"-v", "5", "cmd"},
			reply:    killed(),
			wantExit: 124,
			wantErr:  "timeout: sending signal TERM to command 'cmd'\n",
		},
		{
			name:     "-v names the signal given",
			args:     []string{"-v", "-s", "HUP", "5", "cmd"},
			reply:    killed(),
			wantExit: 124,
			wantErr:  "timeout: sending signal HUP to command 'cmd'\n",
		},
		{
			name:     "--verbose works as the long form",
			args:     []string{"--verbose", "5", "cmd"},
			reply:    killed(),
			wantExit: 124,
			wantErr:  "timeout: sending signal TERM to command 'cmd'\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			rec := &execRecorder{reply: test.reply}
			sleeper := &elapsedSleeper{}
			stdout, stderr, err := run(t, rec.exec, sleeper.sleep, nil, test.args...)
			if stdout != "" {
				t.Errorf("Run() stdout = %q, want empty", stdout)
			}
			if stderr != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", stderr, test.wantErr)
			}
			if code := exitCode(t, err); code != test.wantExit {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantExit)
			}
			if rec.count() != 1 {
				t.Errorf("Run() executions = %d, want 1", rec.count())
			}
			if rec.contextErr() == nil {
				t.Error("Run() left the execution context running")
			}
		})
	}

	t.Run("the caller context canceled during the wait", func(t *testing.T) {
		t.Parallel()
		rec := &execRecorder{reply: killed()}
		_, _, err := run(t, rec.exec, canceledSleeper, nil, "5", "cmd")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want the context error as-is", err)
		}
		if _, ok := errors.AsType[*coreutils.ExitError](err); ok {
			t.Errorf("Run() error = %v, want no exit error", err)
		}
		// The execution context is canceled before returning, but the
		// execution may not have started yet, so its context is not
		// checked here.
	})
}

func TestRunKillAfter(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		hang     bool
		reply    func(context.Context) (int, error)
		sleep    coreutils.SleepFunc
		wantExit int
		wantErr  string
		check    func(t *testing.T, sleeper *elapsedSleeper)
	}{
		{
			name:     "the KILL signal sent reports 137",
			args:     []string{"-k", "3", "5", "cmd"},
			hang:     true,
			wantExit: 137,
			check:    checkSleeps(5*time.Second, 3*time.Second),
		},
		{
			name:     "the KILL signal sent reports 137 with --preserve-status",
			args:     []string{"-p", "-k", "3", "5", "cmd"},
			hang:     true,
			wantExit: 137,
		},
		{
			name:     "-v reports both signals",
			args:     []string{"-v", "-k", "3", "5", "cmd"},
			hang:     true,
			wantExit: 137,
			wantErr: "timeout: sending signal TERM to command 'cmd'\n" +
				"timeout: sending signal KILL to command 'cmd'\n",
		},
		{
			name:     "the long form of -k works",
			args:     []string{"--kill-after=3", "5", "cmd"},
			hang:     true,
			wantExit: 137,
		},
		{
			name:     "a command exiting during the grace period keeps its status with --preserve-status",
			args:     []string{"-p", "-k", "3", "5", "cmd"},
			reply:    exitsLate(7),
			sleep:    (&onceElapsedSleeper{}).sleep,
			wantExit: 7,
		},
		{
			name:     "a command exiting during the grace period reports 124 without --preserve-status",
			args:     []string{"-k", "3", "5", "cmd"},
			reply:    exitsLate(7),
			sleep:    (&onceElapsedSleeper{}).sleep,
			wantExit: 124,
		},
		{
			name:     "a kill-after of 0 never fires",
			args:     []string{"-k", "0", "5", "cmd"},
			reply:    killed(),
			wantExit: 124,
			check:    checkSleeps(5 * time.Second),
		},
		{
			name:     "a kill-after of 0 never fires with --preserve-status",
			args:     []string{"-p", "-k", "0", "5", "cmd"},
			reply:    killed(),
			wantExit: 143,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			reply := test.reply
			var release, exited chan struct{}
			if test.hang {
				release, exited = make(chan struct{}), make(chan struct{})
				reply = hangs(release, exited)
			}
			rec := &execRecorder{reply: reply}
			sleeper := &elapsedSleeper{}
			sleep := sleeper.sleep
			if test.sleep != nil {
				sleep = test.sleep
			}
			_, stderr, err := run(t, rec.exec, sleep, nil, test.args...)
			if stderr != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", stderr, test.wantErr)
			}
			if code := exitCode(t, err); code != test.wantExit {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantExit)
			}
			if test.check != nil {
				test.check(t, sleeper)
			}
			if test.hang {
				// Let the surviving command go, and wait for its
				// goroutine to report back, so nothing leaks.
				close(release)
				<-exited
			}
		})
	}

	t.Run("the kill-after waits for no execution when the duration is 0", func(t *testing.T) {
		t.Parallel()
		rec := &execRecorder{reply: exits(2)}
		sleeper := &blockedSleeper{}
		_, _, err := run(t, rec.exec, sleeper.sleep, nil, "-k", "5", "0", "cmd")
		if code := exitCode(t, err); code != 2 {
			t.Errorf("Run() exit code = %d, want 2", code)
		}
		if waited := sleeper.waited(); len(waited) != 0 {
			t.Errorf("Run() sleeps = %v, want none", waited)
		}
	})
}

// checkSleeps returns a check of the durations the waits were asked for.
func checkSleeps(want ...time.Duration) func(t *testing.T, sleeper *elapsedSleeper) {
	return func(t *testing.T, sleeper *elapsedSleeper) {
		t.Helper()
		if waited := sleeper.waited(); !slices.Equal(waited, want) {
			t.Errorf("Run() sleeps = %v, want %v", waited, want)
		}
	}
}

func TestRunUsageErrors(t *testing.T) {
	tryHelp := "Try 'timeout --help' for more information.\n"
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "an invalid option",
			args:    []string{"-z", "5", "cmd"},
			wantErr: "timeout: invalid option -- 'z'\n" + tryHelp,
		},
		{
			name:    "an unknown long option",
			args:    []string{"--bogus", "5", "cmd"},
			wantErr: "timeout: unrecognized option '--bogus'\n" + tryHelp,
		},
		{
			name:    "a missing option argument",
			args:    []string{"-k"},
			wantErr: "timeout: option requires an argument -- 'k'\n" + tryHelp,
		},
		{
			name:    "a missing long option argument",
			args:    []string{"--signal"},
			wantErr: "timeout: option '--signal' requires an argument\n" + tryHelp,
		},
		{
			name:    "no operands at all",
			args:    nil,
			wantErr: "timeout: missing operand\n" + tryHelp,
		},
		{
			name:    "no command operand",
			args:    []string{"5"},
			wantErr: "timeout: missing command\n" + tryHelp,
		},
		{
			name:    "no operands after --",
			args:    []string{"--"},
			wantErr: "timeout: missing operand\n" + tryHelp,
		},
		{
			name:    "an invalid duration",
			args:    []string{"abc", "cmd"},
			wantErr: "timeout: invalid time interval 'abc'\n" + tryHelp,
		},
		{
			name:    "a NaN duration",
			args:    []string{"nan", "cmd"},
			wantErr: "timeout: invalid time interval 'nan'\n" + tryHelp,
		},
		{
			name:    "an invalid kill-after",
			args:    []string{"-k", "abc", "5", "cmd"},
			wantErr: "timeout: invalid time interval 'abc'\n" + tryHelp,
		},
		{
			name:    "an invalid signal",
			args:    []string{"-s", "NOPE", "5", "cmd"},
			wantErr: "timeout: invalid signal spec 'NOPE'\n" + tryHelp,
		},
		{
			name:    "the signal 0 is not accepted",
			args:    []string{"-s", "0", "5", "cmd"},
			wantErr: "timeout: invalid signal spec '0'\n" + tryHelp,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := run(t, nil, nil, nil, test.args...)
			if stdout != "" {
				t.Errorf("Run() stdout = %q, want empty", stdout)
			}
			if stderr != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", stderr, test.wantErr)
			}
			if code := exitCode(t, err); code != 1 {
				t.Errorf("Run() exit code = %d, want 1", code)
			}
		})
	}
}

func TestRunNoExec(t *testing.T) {
	t.Parallel()
	_, stderr, err := run(t, nil, nil, nil, "5", "cmd")
	if stderr != "timeout: no command execution available\n" {
		t.Errorf("Run() stderr = %q", stderr)
	}
	if code := exitCode(t, err); code != 125 {
		t.Errorf("Run() exit code = %d, want 125", code)
	}
}

func TestHelp(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := run(t, nil, nil, nil, "--help")
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if stderr != "" {
		t.Errorf("Run() stderr = %q, want empty", stderr)
	}
	if !strings.HasPrefix(stdout, "Usage: timeout [OPTION] DURATION COMMAND [ARG]...\n") {
		t.Errorf("Run() stdout = %q", stdout)
	}
	for _, option := range []string{"-f, --foreground", "-k, --kill-after=DURATION", "-s, --signal=SIGNAL", "-v, --verbose", "--preserve-status"} {
		if !strings.Contains(stdout, option+"\n") {
			t.Errorf("Run() stdout misses the %s option", option)
		}
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()
	stdout, _, err := run(t, nil, nil, nil, "--version")
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if !strings.HasPrefix(stdout, "timeout (charm.land/snail/coreutils) ") {
		t.Errorf("Run() stdout = %q", stdout)
	}
}

func TestRunInfDurationEnforcesNoTimeLimit(t *testing.T) {
	t.Parallel()
	rec := &execRecorder{reply: exits(0)}
	sleeper := &elapsedSleeper{}
	stdout, _, err := run(t, rec.exec, sleeper.sleep, nil, "inf", "cmd")
	if code := exitCode(t, err); code != 0 {
		t.Errorf("Run() exit code = %d, want 0", code)
	}
	if stdout != "" {
		t.Errorf("Run() stdout = %q, want empty", stdout)
	}
	if waited := sleeper.waited(); len(waited) != 0 {
		t.Errorf("Run() sleeps = %v, want none", waited)
	}
}

func TestRunCallerContextDone(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	rt := &coreutils.Runtime{
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
		Exec: func(context.Context, coreutils.ExecOpts) (int, error) {
			t.Error("the command was executed")
			return 0, nil
		},
	}
	err := New(rt).Run(ctx, "5", "cmd")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want the context error as-is", err)
	}
}

// TestRunCallerContextDoneDuringKillAfter runs a command whose time limit
// elapses, and whose caller is done while the kill grace period runs.
func TestRunCallerContextDoneDuringKillAfter(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	release, exited := make(chan struct{}), make(chan struct{})
	rec := &execRecorder{reply: hangs(release, exited)}
	var (
		mu      sync.Mutex
		waited  []time.Duration
		graceOn bool
	)
	sleep := func(waitCtx context.Context, d time.Duration) error {
		mu.Lock()
		first := !graceOn
		graceOn = true
		waited = append(waited, d)
		mu.Unlock()
		if first {
			return nil
		}
		// The grace period began; the caller is done by then.
		cancel()
		return waitCtx.Err()
	}
	rt := &coreutils.Runtime{
		Stdout:  new(bytes.Buffer),
		Stderr:  new(bytes.Buffer),
		Environ: func() []string { return nil },
		Exec:    rec.exec,
		Sleep:   sleep,
	}
	err := New(rt).Run(ctx, "-k", "3", "5", "cmd")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want the context error as-is", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(waited, []time.Duration{5 * time.Second, 3 * time.Second}) {
		t.Errorf("Run() sleeps = %v, want [5s 3s]", waited)
	}
	// Let the surviving command go, and wait for its goroutine to report
	// back, so nothing leaks.
	close(release)
	<-exited
}

// stubMarkers are the stack fragments of the test stubs a leaked
// goroutine would be parked in.
var stubMarkers = []string{
	"timeout.(*execRecorder).exec",
	"timeout.hangs",
	"timeout.(*blockedSleeper).sleep",
	"timeout.(*onceElapsedSleeper).sleep",
}

// parkedInStubs reports whether any goroutine is still parked in one of
// the test stubs, which would mean the run leaked it.
func parkedInStubs() bool {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	stack := string(buf[:n])
	for _, marker := range stubMarkers {
		if strings.Contains(stack, marker) {
			return true
		}
	}
	return false
}

// TestNoLeakedGoroutines runs the scenarios whose goroutines park in the
// test stubs, and checks that every one of them is gone by the end. It
// runs sequentially, so no other test keeps goroutines in the stubs while
// it looks.
func TestNoLeakedGoroutines(t *testing.T) {
	scenarios := []struct {
		name    string
		args    []string
		hang    bool
		sleep   coreutils.SleepFunc
		reply   func(context.Context) (int, error)
		wantOut int
	}{
		{
			name:    "killed at the time limit",
			args:    []string{"5", "cmd"},
			sleep:   (&elapsedSleeper{}).sleep,
			reply:   killed(),
			wantOut: 124,
		},
		{
			name:    "killed after the kill-after",
			args:    []string{"-k", "3", "5", "cmd"},
			sleep:   (&elapsedSleeper{}).sleep,
			hang:    true,
			wantOut: 137,
		},
		{
			name:    "command finishing first",
			args:    []string{"5", "cmd"},
			sleep:   (&blockedSleeper{}).sleep,
			reply:   exits(3),
			wantOut: 3,
		},
		{
			name:    "no time limit",
			args:    []string{"0", "cmd"},
			sleep:   (&blockedSleeper{}).sleep,
			reply:   exits(7),
			wantOut: 7,
		},
		{
			name:    "command exiting during the grace period",
			args:    []string{"-p", "-k", "3", "5", "cmd"},
			sleep:   (&onceElapsedSleeper{}).sleep,
			reply:   exitsLate(9),
			wantOut: 9,
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			reply := scenario.reply
			var release, exited chan struct{}
			if scenario.hang {
				release, exited = make(chan struct{}), make(chan struct{})
				reply = hangs(release, exited)
			}
			rec := &execRecorder{reply: reply}
			_, stderr, err := run(t, rec.exec, scenario.sleep, nil, scenario.args...)
			if stderr != "" {
				t.Errorf("Run() stderr = %q, want empty", stderr)
			}
			if code := exitCode(t, err); code != scenario.wantOut {
				t.Errorf("Run() exit code = %d, want %d", code, scenario.wantOut)
			}
			if scenario.hang {
				close(release)
				<-exited
			}
		})
	}
	// The waiting goroutines the run leaves behind go away once the
	// canceled wait contexts reach them; give them a moment, backing off
	// while polling.
	delay := time.Millisecond
	deadline := time.Now().Add(5 * time.Second)
	for parkedInStubs() {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			t.Fatalf("goroutines still parked in the test stubs:\n%s", buf[:n])
		}
		time.Sleep(delay)
		delay = min(2*delay, 100*time.Millisecond)
	}
}
