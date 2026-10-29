package env

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"charm.land/snail/coreutils"
)

// execRecord records an invocation of the stub Exec.
type execRecord struct {
	called bool
	name   string
	args   []string
	env    []string
	status int
	err    error
	lookup func(string) (string, bool)
	rt     *coreutils.Runtime
}

func (r *execRecord) calledOnce() bool {
	return r.called
}

// newRuntime builds a runtime enumerating environ and recording
// invocations in rec.
func newRuntime(environ []string, rec *execRecord) *coreutils.Runtime {
	rt := &coreutils.Runtime{
		Stdout:  new(bytes.Buffer),
		Stderr:  new(bytes.Buffer),
		Environ: func() []string { return environ },
	}
	if rec != nil {
		rec.rt = rt
		rt.Exec = func(_ context.Context, name string, args []string) (int, error) {
			rec.called, rec.name, rec.args, rec.env = true, name, args, rt.Env
			rec.lookup = rt.LookupEnv
			return rec.status, rec.err
		}
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

func TestRunPrintEnv(t *testing.T) {
	environ := []string{"FOO=1", "BAR=2"}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no operands write the whole environment",
			want: "FOO=1\nBAR=2\n",
		},
		{
			name: "-u removes a variable",
			args: []string{"-u", "FOO"},
			want: "BAR=2\n",
		},
		{
			name: "-u may repeat",
			args: []string{"-u", "FOO", "-u", "BAR"},
			want: "",
		},
		{
			name: "-u of a missing variable changes nothing",
			args: []string{"-u", "MISSING"},
			want: "FOO=1\nBAR=2\n",
		},
		{
			name: "-i empties the environment",
			args: []string{"-i"},
			want: "",
		},
		{
			name: "-i and -u combine",
			args: []string{"-i", "-u", "FOO"},
			want: "",
		},
		{
			name: "an assignment replaces the variable in place",
			args: []string{"FOO=3"},
			want: "FOO=3\nBAR=2\n",
		},
		{
			name: "an assignment of a new variable is appended",
			args: []string{"NEW=4"},
			want: "FOO=1\nBAR=2\nNEW=4\n",
		},
		{
			name: "an empty value sets the variable empty",
			args: []string{"A="},
			want: "FOO=1\nBAR=2\nA=\n",
		},
		{
			name: "assignments apply in order, the last one wins",
			args: []string{"FOO=3", "FOO=4"},
			want: "FOO=4\nBAR=2\n",
		},
		{
			name: "-u and an assignment combine",
			args: []string{"-u", "BAR", "FOO=3"},
			want: "FOO=3\n",
		},
		{
			name: "an assignment wins over -u of the same name",
			args: []string{"-u", "FOO", "FOO=9"},
			want: "BAR=2\nFOO=9\n",
		},
		{
			name: "assignments after -- are applied",
			args: []string{"--", "FOO=3"},
			want: "FOO=3\nBAR=2\n",
		},
		{
			name: "-- alone leaves the environment",
			args: []string{"--"},
			want: "FOO=1\nBAR=2\n",
		},
		{
			name: "-i with an assignment",
			args: []string{"-i", "NEW=4"},
			want: "NEW=4\n",
		},
		{
			name: "options may be grouped, -iu is -i and -u",
			args: []string{"-iu", "FOO"},
			want: "",
		},
		{
			name: "the option-argument may be attached",
			args: []string{"-uFOO"},
			want: "BAR=2\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rt := newRuntime(environ, nil)
			err := New(rt).Run(t.Context(), test.args...)
			if err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}
			if got := stdout(t, rt); got != test.want {
				t.Errorf("Run() stdout = %q, want %q", got, test.want)
			}
			if got := stderr(t, rt); got != "" {
				t.Errorf("Run() stderr = %q, want empty", got)
			}
		})
	}
}

func TestRunInvoke(t *testing.T) {
	environ := []string{"FOO=1", "BAR=2"}
	notFound := errors.New("exec: \"utility\": executable file not found in $PATH")
	permission := errors.New("permission denied")
	killed := errors.New("signal: killed")

	tests := []struct {
		name     string
		args     []string
		status   int
		execErr  error
		wantName string
		wantArgs []string
		wantEnv  []string
		wantCode int
	}{
		{
			name:     "the utility runs with the inherited environment",
			args:     []string{"utility"},
			wantName: "utility",
			wantEnv:  environ,
		},
		{
			name:     "the arguments are passed to the utility",
			args:     []string{"utility", "one", "two"},
			wantName: "utility",
			wantArgs: []string{"one", "two"},
			wantEnv:  environ,
		},
		{
			name:     "an assignment reaches the utility",
			args:     []string{"NEW=4", "utility"},
			wantName: "utility",
			wantEnv:  []string{"FOO=1", "BAR=2", "NEW=4"},
		},
		{
			name:     "-u removes a variable from the utility environment",
			args:     []string{"-u", "FOO", "utility"},
			wantName: "utility",
			wantEnv:  []string{"BAR=2"},
		},
		{
			name:     "-i starts the utility with an empty environment",
			args:     []string{"-i", "utility"},
			wantName: "utility",
			wantEnv:  []string{},
		},
		{
			name:     "-i with an assignment",
			args:     []string{"-i", "NEW=4", "utility"},
			wantName: "utility",
			wantEnv:  []string{"NEW=4"},
		},
		{
			name:     "the exit status of the utility is propagated",
			args:     []string{"utility"},
			status:   7,
			wantName: "utility",
			wantEnv:  environ,
			wantCode: 7,
		},
		{
			name:     "a missing utility exits with 127",
			args:     []string{"utility"},
			status:   127,
			execErr:  notFound,
			wantName: "utility",
			wantEnv:  environ,
			wantCode: 127,
		},
		{
			name:     "a utility found but not invocable exits with 126",
			args:     []string{"utility"},
			status:   126,
			execErr:  permission,
			wantName: "utility",
			wantEnv:  environ,
			wantCode: 126,
		},
		{
			name:     "a utility terminated by a signal exits with 125",
			args:     []string{"utility"},
			status:   125,
			execErr:  killed,
			wantName: "utility",
			wantEnv:  environ,
			wantCode: 125,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := &execRecord{status: test.status, err: test.execErr}
			rt := newRuntime(environ, rec)
			err := New(rt).Run(t.Context(), test.args...)
			if test.wantCode != 0 {
				exitErr, ok := errors.AsType[*coreutils.ExitError](err)
				if !ok {
					t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
				}
				if exitErr.Code != test.wantCode {
					t.Errorf("Run() exit code = %d, want %d", exitErr.Code, test.wantCode)
				}
				if test.execErr != nil {
					if !errors.Is(err, test.execErr) {
						t.Errorf("Run() error = %v, want it wrapping %v", err, test.execErr)
					}
					if want := "env: " + test.execErr.Error() + "\n"; stderr(t, rt) != want {
						t.Errorf("Run() stderr = %q, want %q", stderr(t, rt), want)
					}
				}
			} else if err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}
			if rec.name != test.wantName {
				t.Errorf("Exec() name = %q, want %q", rec.name, test.wantName)
			}
			if len(rec.args) != len(test.wantArgs) {
				t.Fatalf("Exec() args = %v, want %v", rec.args, test.wantArgs)
			}
			for i, arg := range rec.args {
				if arg != test.wantArgs[i] {
					t.Errorf("Exec() args[%d] = %q, want %q", i, arg, test.wantArgs[i])
				}
			}
			if len(rec.env) != len(test.wantEnv) {
				t.Fatalf("Exec() environment = %v, want %v", rec.env, test.wantEnv)
			}
			for i, entry := range rec.env {
				if entry != test.wantEnv[i] {
					t.Errorf("Exec() environment[%d] = %q, want %q", i, entry, test.wantEnv[i])
				}
			}
			if test.wantCode == 0 && stderr(t, rt) != "" {
				t.Errorf("Run() stderr = %q, want empty", stderr(t, rt))
			}
		})
	}
}

func TestRunInvokeNoExec(t *testing.T) {
	rt := newRuntime([]string{"FOO=1"}, nil)
	err := New(rt).Run(t.Context(), "utility")

	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok {
		t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
	}
	if exitErr.Code != 125 {
		t.Errorf("Run() exit code = %d, want 125", exitErr.Code)
	}
	if want := "env: no command execution available\n"; stderr(t, rt) != want {
		t.Errorf("Run() stderr = %q, want %q", stderr(t, rt), want)
	}
}

func TestRunInvokeLookupEnv(t *testing.T) {
	t.Run("a PATH assignment is used to look the utility up", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime([]string{"PATH=/old"}, rec)
		if err := New(rt).Run(t.Context(), "PATH=/new", "utility"); err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if got, ok := rec.lookup("PATH"); !ok || got != "/new" {
			t.Errorf("LookupEnv(\"PATH\") = %q, %v, want %q, true", got, ok, "/new")
		}
	})

	t.Run("the inherited variables are looked up", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime([]string{"FOO=1"}, rec)
		if err := New(rt).Run(t.Context(), "utility"); err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if got, ok := rec.lookup("FOO"); !ok || got != "1" {
			t.Errorf("LookupEnv(\"FOO\") = %q, %v, want %q, true", got, ok, "1")
		}
	})

	t.Run("-i leaves no variables to look up", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime([]string{"PATH=/old"}, rec)
		if err := New(rt).Run(t.Context(), "-i", "utility"); err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if _, ok := rec.lookup("PATH"); ok {
			t.Error("LookupEnv(\"PATH\") found the variable, want it unset")
		}
	})

	t.Run("the runtime environment and lookup are restored", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime([]string{"FOO=1"}, rec)
		if err := New(rt).Run(t.Context(), "A=2", "utility"); err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if rt.Env != nil {
			t.Errorf("Runtime.Env = %v, want it restored to nil", rt.Env)
		}
		if rt.LookupEnv != nil {
			t.Error("Runtime.LookupEnv was not restored to nil")
		}
	})
}

func TestRunInvokeEmptyUtility(t *testing.T) {
	notFound := errors.New("exec: \"\": executable file not found in $PATH")
	rec := &execRecord{status: 127, err: notFound}
	rt := newRuntime([]string{"FOO=1"}, rec)
	err := New(rt).Run(t.Context(), "")

	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok {
		t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
	}
	if exitErr.Code != 127 {
		t.Errorf("Run() exit code = %d, want 127", exitErr.Code)
	}
	if !rec.calledOnce() {
		t.Fatal("Exec was not called, want the empty utility to be invoked")
	}
	if rec.name != "" {
		t.Errorf("Exec() name = %q, want the empty name", rec.name)
	}
	if want := "env: " + notFound.Error() + "\n"; stderr(t, rt) != want {
		t.Errorf("Run() stderr = %q, want %q", stderr(t, rt), want)
	}
	if got := stdout(t, rt); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
}

func TestRunInvokeRealUtility(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs the executable permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "myutil")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho \"FOO=$FOO\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	rt := coreutils.OsRuntime()
	out := new(bytes.Buffer)
	rt.Stdout = out
	if err := New(rt).Run(t.Context(), "FOO=fromenv", "PATH="+dir, "myutil"); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if got := out.String(); got != "FOO=fromenv\n" {
		t.Errorf("Run() output = %q, want %q", got, "FOO=fromenv\n")
	}
	if rt.Env != nil {
		t.Errorf("Runtime.Env = %v, want it restored to nil", rt.Env)
	}
}

func TestRunUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "an invalid short option",
			args:    []string{"-x"},
			wantErr: "env: invalid option -- 'x'\n",
		},
		{
			name:    "an unrecognized long option",
			args:    []string{"--foo"},
			wantErr: "env: unrecognized option '--foo'\n",
		},
		{
			name:    "-u without an argument",
			args:    []string{"-u"},
			wantErr: "env: option requires an argument -- 'u'\n",
		},
		{
			name:    "-u of an empty name",
			args:    []string{"-u", ""},
			wantErr: "env: cannot unset \"\": invalid name\n",
		},
		{
			name:    "-u of a name holding a value",
			args:    []string{"-u", "FOO=1"},
			wantErr: "env: cannot unset \"FOO=1\": invalid name\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rt := newRuntime([]string{"FOO=1"}, nil)
			err := New(rt).Run(t.Context(), test.args...)

			exitErr, ok := errors.AsType[*coreutils.ExitError](err)
			if !ok {
				t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
			}
			if exitErr.Code != 125 {
				t.Errorf("Run() exit code = %d, want 125", exitErr.Code)
			}
			if got := stderr(t, rt); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
			if got := stdout(t, rt); got != "" {
				t.Errorf("Run() stdout = %q, want empty", got)
			}
		})
	}
}

func TestRunInvalidAssignment(t *testing.T) {
	// An operand which isn't a name=value assignment is the utility to
	// invoke, and fails with 127 when it can't be run.
	rec := &execRecord{status: 127, err: errors.New("not found")}
	rt := newRuntime([]string{"FOO=1"}, rec)
	err := New(rt).Run(t.Context(), "1FOO=1")

	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok {
		t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
	}
	if exitErr.Code != 127 {
		t.Errorf("Run() exit code = %d, want 127", exitErr.Code)
	}
	if rec.name != "1FOO=1" {
		t.Errorf("Exec() name = %q, want %q", rec.name, "1FOO=1")
	}
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("broken pipe")
	rt := newRuntime([]string{"FOO=1", "BAR=2"}, nil)
	rt.Stdout = errWriter{err: writeErr}
	err := New(rt).Run(t.Context())

	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok {
		t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
	}
	if exitErr.Code != 125 {
		t.Errorf("Run() exit code = %d, want 125", exitErr.Code)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("Run() error = %v, want it wrapping %v", err, writeErr)
	}
	if got := stderr(t, rt); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
}

type errWriter struct{ err error }

func (w errWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

func TestRunContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rec := &execRecord{}
	rt := newRuntime([]string{"FOO=1"}, rec)
	err := New(rt).Run(ctx, "utility")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want %v", err, context.Canceled)
	}
	if _, ok := errors.AsType[*coreutils.ExitError](err); ok {
		t.Fatalf("Run() error = %v, want the context error as-is", err)
	}
	if rec.calledOnce() {
		t.Error("Exec was called, want the utility not to run")
	}
	if got := stdout(t, rt); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
}

func TestRunPrintEnvNoStdout(t *testing.T) {
	rt := &coreutils.Runtime{
		Stderr:  new(bytes.Buffer),
		Environ: func() []string { return []string{"FOO=1"} },
	}
	err := New(rt).Run(t.Context())

	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok {
		t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
	}
	if exitErr.Code != 125 {
		t.Errorf("Run() exit code = %d, want 125", exitErr.Code)
	}
	got := stderr(t, rt)
	if want := "env: no standard output available\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}
