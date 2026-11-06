package nice

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"charm.land/snail/coreutils"
)

// execRecord records an invocation of the stub Exec, replying with a reply
// function when given, or with the status and error held.
type execRecord struct {
	called bool
	name   string
	args   []string
	env    []string
	status int
	err    error
	reply  func(ctx context.Context) (int, error)
}

func (r *execRecord) exec(ctx context.Context, opts coreutils.ExecOpts) (int, error) {
	r.called, r.name, r.args, r.env = true, opts.Name, opts.Args, opts.Env
	if r.reply != nil {
		return r.reply(ctx)
	}
	return r.status, r.err
}

// newRuntime builds a runtime recording invocations in rec. The environment
// is set only when environ is not nil, so the runs with no environment are
// covered too.
func newRuntime(rec *execRecord, environ []string) *coreutils.Runtime {
	rt := &coreutils.Runtime{
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
	}
	if environ != nil {
		rt.Environ = func() []string { return environ }
	}
	if rec != nil {
		rt.Exec = rec.exec
	}
	return rt
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

func TestRunInvoke(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		status   int
		wantName string
		wantArgs []string
		wantCode int
	}{
		{
			name:     "runs the utility with its arguments",
			args:     []string{"echo", "hi", "there"},
			wantName: "echo",
			wantArgs: []string{"hi", "there"},
		},
		{
			name:     "the exit status of the utility becomes the exit status of nice",
			args:     []string{"false"},
			status:   3,
			wantName: "false",
			wantCode: 3,
		},
		{
			name:     "-n takes the increment from the next argument",
			args:     []string{"-n", "5", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "-n takes the increment attached",
			args:     []string{"-n5", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "-n takes a negative increment",
			args:     []string{"-n", "-3", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "--adjustment takes the increment from the next argument",
			args:     []string{"--adjustment", "7", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "--adjustment takes the increment attached",
			args:     []string{"--adjustment=7", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "the obsolete form takes a positive increment",
			args:     []string{"-5", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "the obsolete form takes a zero increment",
			args:     []string{"-0", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "the obsolete form takes a negative increment",
			args:     []string{"--5", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "the obsolete form takes a signed increment",
			args:     []string{"-+5", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "the last adjustment given wins",
			args:     []string{"-5", "-n", "3", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "an obsolescent adjustment follows the -n option",
			args:     []string{"-n", "1", "-5", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "an obsolescent adjustment follows a long option",
			args:     []string{"--adjustment=1", "-5", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "the -n option follows an obsolescent adjustment",
			args:     []string{"-5", "-n", "1", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "a later obsolescent adjustment drops an invalid -n value",
			args:     []string{"-n", "99999999999999999999", "-5", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "a later -n value drops an invalid obsolescent adjustment",
			args:     []string{"-999999999999999999999", "-n", "5", "cmd"},
			wantName: "cmd",
		},
		{
			name:     "an obsolete increment which is not a number is the utility name",
			args:     []string{"-x", "ls", "-l"},
			wantName: "-x",
			wantArgs: []string{"ls", "-l"},
		},
		{
			name:     "an obsolete increment which is not an integer is the utility name",
			args:     []string{"-1.5", "ls"},
			wantName: "-1.5",
			wantArgs: []string{"ls"},
		},
		{
			name:     "the arguments after a utility name holding a dash are operands",
			args:     []string{"-x", "-5", "cmd"},
			wantName: "-x",
			wantArgs: []string{"-5", "cmd"},
		},
		{
			name:     "--help after a utility name is its operand",
			args:     []string{"-x", "--help"},
			wantName: "-x",
			wantArgs: []string{"--help"},
		},
		{
			name:     "--version after a utility name is its operand",
			args:     []string{"-x", "--version"},
			wantName: "-x",
			wantArgs: []string{"--version"},
		},
		{
			name:     "-- ends the options before --help",
			args:     []string{"--", "--help", "cmd"},
			wantName: "--help",
			wantArgs: []string{"cmd"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := &execRecord{status: test.status}
			rt := newRuntime(rec, []string{"FOO=1"})
			err := New(rt).Run(t.Context(), test.args...)
			if code := exitCode(t, err); code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if !rec.called {
				t.Fatal("Exec was not called, want the utility to run")
			}
			if rec.name != test.wantName {
				t.Errorf("Exec() name = %q, want %q", rec.name, test.wantName)
			}
			if want := test.wantArgs; want != nil {
				if got := rec.args; len(got) != len(want) {
					t.Fatalf("Exec() args = %v, want %v", got, want)
				}
				for i, arg := range want {
					if rec.args[i] != arg {
						t.Errorf("Exec() args[%d] = %q, want %q", i, rec.args[i], arg)
					}
				}
			} else if len(rec.args) != 0 {
				t.Errorf("Exec() args = %v, want no arguments", rec.args)
			}
			wantEnv := []string{"FOO=1"}
			if got := rec.env; len(got) != len(wantEnv) || got[0] != wantEnv[0] {
				t.Errorf("Exec() environment = %v, want %v", got, wantEnv)
			}
		})
	}
}

func TestRunInvokeEnv(t *testing.T) {
	t.Run("the environment of the runtime is given to the utility", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(rec, []string{"FOO=1", "PATH=/bin"})
		if err := New(rt).Run(t.Context(), "cmd"); err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		want := []string{"FOO=1", "PATH=/bin"}
		if got := rec.env; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("Exec() environment = %v, want %v", got, want)
		}
	})

	t.Run("a runtime without environment gives none", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(rec, nil)
		if err := New(rt).Run(t.Context(), "cmd"); err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if rec.env != nil {
			t.Errorf("Exec() environment = %v, want nil", rec.env)
		}
	})
}

func TestRunUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name: "no operands",
			args: nil,
			wantErr: "nice: missing operand\n" +
				"Try 'nice --help' for more information.\n",
		},
		{
			name: "no utility with an adjustment",
			args: []string{"-n", "5"},
			wantErr: "nice: a command must be given with an adjustment\n" +
				"Try 'nice --help' for more information.\n",
		},
		{
			name: "no utility with an obsolete adjustment",
			args: []string{"-5"},
			wantErr: "nice: a command must be given with an adjustment\n" +
				"Try 'nice --help' for more information.\n",
		},
		{
			name: "no operands after --",
			args: []string{"--"},
			wantErr: "nice: missing operand\n" +
				"Try 'nice --help' for more information.\n",
		},
		{
			name: "-n without a value",
			args: []string{"-n"},
			wantErr: "nice: option requires an argument -- 'n'\n" +
				"Try 'nice --help' for more information.\n",
		},
		{
			name: "--adjustment without a value",
			args: []string{"--adjustment"},
			wantErr: "nice: option '--adjustment' requires an argument\n" +
				"Try 'nice --help' for more information.\n",
		},
		{
			name: "an invalid adjustment",
			args: []string{"-n", "foo", "cmd"},
			wantErr: "nice: invalid adjustment \"foo\"\n" +
				"Try 'nice --help' for more information.\n",
		},
		{
			name: "an adjustment too large to hold",
			args: []string{"-n", "99999999999999999999", "cmd"},
			wantErr: "nice: invalid adjustment \"99999999999999999999\"\n" +
				"Try 'nice --help' for more information.\n",
		},
		{
			name: "an obsolescent adjustment too large to hold",
			args: []string{"-999999999999999999999", "cmd"},
			wantErr: "nice: invalid adjustment \"999999999999999999999\"\n" +
				"Try 'nice --help' for more information.\n",
		},
		{
			name: "a negative obsolescent adjustment too large to hold",
			args: []string{"--999999999999999999999", "cmd"},
			wantErr: "nice: invalid adjustment \"-999999999999999999999\"\n" +
				"Try 'nice --help' for more information.\n",
		},
		{
			name: "a later obsolescent adjustment drops a valid -n value",
			args: []string{"-n", "5", "-999999999999999999999", "cmd"},
			wantErr: "nice: invalid adjustment \"999999999999999999999\"\n" +
				"Try 'nice --help' for more information.\n",
		},
		{
			name: "-n takes --help as its value",
			args: []string{"-n", "--help"},
			wantErr: "nice: invalid adjustment \"--help\"\n" +
				"Try 'nice --help' for more information.\n",
		},
		{
			name: "an unrecognized option",
			args: []string{"--bogus", "cmd"},
			wantErr: "nice: unrecognized option '--bogus'\n" +
				"Try 'nice --help' for more information.\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := &execRecord{}
			rt := newRuntime(rec, nil)
			err := New(rt).Run(t.Context(), test.args...)
			if code := exitCode(t, err); code != 125 {
				t.Errorf("Run() exit code = %d, want 125", code)
			}
			if got := stderr(t, rt); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
			if rec.called {
				t.Error("Exec was called, want the utility not to run")
			}
		})
	}
}

func TestRunExecStatuses(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		err      error
		wantErr  string
		wantCode int
	}{
		{
			name:     "a missing utility",
			status:   127,
			err:      errors.New(`exec: "cmd": executable file not found in $PATH`),
			wantErr:  "nice: exec: \"cmd\": executable file not found in $PATH\n",
			wantCode: 127,
		},
		{
			name:     "a utility which cannot be invoked",
			status:   126,
			err:      errors.New("permission denied"),
			wantErr:  "nice: permission denied\n",
			wantCode: 126,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := &execRecord{status: test.status, err: test.err}
			rt := newRuntime(rec, nil)
			err := New(rt).Run(t.Context(), "cmd")
			if code := exitCode(t, err); code != test.wantCode {
				t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
			}
			if got := stderr(t, rt); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
		})
	}
}

func TestRunNoExec(t *testing.T) {
	rt := newRuntime(nil, nil)
	err := New(rt).Run(t.Context(), "cmd")
	if code := exitCode(t, err); code != 125 {
		t.Errorf("Run() exit code = %d, want 125", code)
	}
	if got, want := stderr(t, rt), "nice: no command execution available\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}

func TestRunContext(t *testing.T) {
	t.Run("canceled before the utility runs", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		rec := &execRecord{}
		rt := newRuntime(rec, nil)
		err := New(rt).Run(ctx, "cmd")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if rec.called {
			t.Error("Exec was called, want the utility not to run")
		}
	})

	t.Run("canceled while the utility runs", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		rec := &execRecord{reply: func(ctx context.Context) (int, error) {
			cancel()
			return 125, ctx.Err()
		}}
		rt := newRuntime(rec, nil)
		err := New(rt).Run(ctx, "cmd")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})
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

	t.Run("help after an obsolescent adjustment", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "-5", "--help"); err != nil {
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
		want := "nice (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
