package nohup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// execRecord records the options of the stub Exec, replying with a reply
// function when given, or with the status and error held.
type execRecord struct {
	called bool
	opts   coreutils.ExecOpts
	status int
	err    error
	reply  func(ctx context.Context, opts coreutils.ExecOpts) (int, error)
}

func (r *execRecord) exec(ctx context.Context, opts coreutils.ExecOpts) (int, error) {
	r.called, r.opts = true, opts
	if r.reply != nil {
		return r.reply(ctx, opts)
	}
	return r.status, r.err
}

// newRuntime builds a runtime recording invocations in rec, with the
// terminal file descriptors of tty and the environment environ. The file
// system is set only when fsys is not nil, so the runs without one are
// covered too.
func newRuntime(rec *execRecord, fsys fsx.MemFS, tty map[int]bool, environ []string) *coreutils.Runtime {
	rt := &coreutils.Runtime{
		Stdout:     new(bytes.Buffer),
		Stderr:     new(bytes.Buffer),
		IsTerminal: func(fd int) bool { return tty[fd] },
	}
	if fsys != nil {
		rt.FS = fsys
	}
	if environ != nil {
		rt.Environ = func() []string { return environ }
	}
	if rec != nil {
		rt.Exec = rec.exec
	}
	return rt
}

// writeReply writes s to the standard output and error the utility was
// given, or to the runtime streams when the execution kept them, standing
// for the output of the utility itself.
func writeReply(rt *coreutils.Runtime, s string, both bool) func(context.Context, coreutils.ExecOpts) (int, error) {
	return func(_ context.Context, opts coreutils.ExecOpts) (int, error) {
		out, errs := opts.Stdout, io.Discard
		if out == nil {
			out = rt.Stdout
		}
		if out == nil {
			out = io.Discard
		}
		if both {
			errs = opts.Stderr
			if errs == nil {
				errs = rt.Stderr
			}
		}
		fmt.Fprint(out, s)
		fmt.Fprint(errs, s)
		return 0, nil
	}
}

func stdout(t *testing.T, rt *coreutils.Runtime) string {
	t.Helper()
	return rt.Stdout.(*bytes.Buffer).String()
}

func stderr(t *testing.T, rt *coreutils.Runtime) string {
	t.Helper()
	return rt.Stderr.(*bytes.Buffer).String()
}

func TestRunInvoke(t *testing.T) {
	t.Run("the utility and its arguments are passed", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(rec, fsx.MemFS{}, nil, nil)

		if err := New(rt).Run(t.Context(), "echo", "-n", "a", "b"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if rec.opts.Name != "echo" {
			t.Errorf("executed %q, want echo", rec.opts.Name)
		}
		if got, want := rec.opts.Args, []string{"-n", "a", "b"}; !slices.Equal(got, want) {
			t.Errorf("executed with args %v, want %v", got, want)
		}
		if rec.opts.Dir != "" {
			t.Errorf("executed in dir %q, want the empty one of the working directory", rec.opts.Dir)
		}
		if got := stderr(t, rt); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
	})

	t.Run("the environment of the runtime is passed", func(t *testing.T) {
		rec := &execRecord{}
		environ := []string{"PATH=/bin", "FOO=bar"}
		rt := newRuntime(rec, fsx.MemFS{}, nil, environ)

		if err := New(rt).Run(t.Context(), "true"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := rec.opts.Env, environ; !slices.Equal(got, want) {
			t.Errorf("executed with env %v, want %v", got, want)
		}
	})

	t.Run("the exit status of the utility becomes the exit status of nohup", func(t *testing.T) {
		rec := &execRecord{status: 3}
		rt := newRuntime(rec, fsx.MemFS{}, nil, nil)

		err := New(rt).Run(t.Context(), "false")
		if code := testutil.ExitCode(t, err); code != 3 {
			t.Errorf("Run() exit status = %d, want 3", code)
		}
	})

	t.Run("double dash makes an option-like utility be invoked", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(rec, fsx.MemFS{}, nil, nil)

		if err := New(rt).Run(t.Context(), "--", "-x"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if rec.opts.Name != "-x" {
			t.Errorf("executed %q, want -x", rec.opts.Name)
		}
		if len(rec.opts.Args) != 0 {
			t.Errorf("executed with args %v, want none", rec.opts.Args)
		}
	})
}

func TestRedirect(t *testing.T) {
	// run runs nohup with the given terminal file descriptors, writing
	// "out\n" to the streams the utility was given, reporting the runtime
	// and the execution record. When both is set, the utility writes to its
	// standard error as well.
	run := func(t *testing.T, fsys fsx.MemFS, tty map[int]bool, both bool, environ []string, args ...string) (*coreutils.Runtime, *execRecord) {
		t.Helper()
		rec := &execRecord{}
		rt := newRuntime(rec, fsys, tty, environ)
		rec.reply = writeReply(rt, "out\n", both)
		if err := New(rt).Run(t.Context(), args...); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		return rt, rec
	}

	t.Run("no terminal stream is redirected", func(t *testing.T) {
		rt, rec := run(t, fsx.MemFS{}, nil, false, nil, "true")

		if rec.opts.Stdout != nil {
			t.Errorf("executed with a standard output of %v, want the runtime stream", rec.opts.Stdout)
		}
		if rec.opts.Stderr != nil {
			t.Errorf("executed with a standard error of %v, want the runtime stream", rec.opts.Stderr)
		}
		if got, want := stdout(t, rt), "out\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if got := stderr(t, rt); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
	})

	t.Run("a terminal standard output is appended to nohup.out", func(t *testing.T) {
		fsys := fsx.MemFS{}
		rt, rec := run(t, fsys, map[int]bool{1: true}, false, nil, "true")

		if rec.opts.Stdout == nil {
			t.Fatal("executed with a nil standard output, want nohup.out")
		}
		if rec.opts.Stderr != nil {
			t.Errorf("executed with a standard error of %v, want the runtime stream", rec.opts.Stderr)
		}
		if got, want := string(fsys["nohup.out"].Data), "out\n"; got != want {
			t.Errorf("nohup.out = %q, want %q", got, want)
		}
		if got, want := stderr(t, rt), "nohup: appending output to 'nohup.out'\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
		if got := stdout(t, rt); got != "" {
			t.Errorf("Run() stdout = %q, want empty", got)
		}
	})

	t.Run("the file is created readable and writable only by its owner", func(t *testing.T) {
		fsys := fsx.MemFS{}
		run(t, fsys, map[int]bool{1: true}, false, nil, "true")

		if got, want := fsys["nohup.out"].Mode.Perm(), fs.FileMode(0o600); got != want {
			t.Errorf("nohup.out mode = %o, want %o", got, want)
		}
	})

	t.Run("the output is appended to an existing nohup.out", func(t *testing.T) {
		fsys := fsx.MemFS{"nohup.out": {Data: []byte("old\n"), Mode: 0o644}}
		run(t, fsys, map[int]bool{1: true}, false, nil, "true")

		if got, want := string(fsys["nohup.out"].Data), "old\nout\n"; got != want {
			t.Errorf("nohup.out = %q, want %q", got, want)
		}
	})

	t.Run("two runs keep appending to nohup.out", func(t *testing.T) {
		fsys := fsx.MemFS{}
		rec := &execRecord{}
		rt := newRuntime(rec, fsys, map[int]bool{1: true}, nil)
		rec.reply = writeReply(rt, "out\n", false)
		for range 2 {
			if err := New(rt).Run(t.Context(), "true"); err != nil {
				t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
			}
		}

		if got, want := string(fsys["nohup.out"].Data), "out\nout\n"; got != want {
			t.Errorf("nohup.out = %q, want %q", got, want)
		}
	})

	t.Run("a terminal standard error goes to the same file", func(t *testing.T) {
		fsys := fsx.MemFS{}
		rt, rec := run(t, fsys, map[int]bool{1: true, 2: true}, true, nil, "true")

		if rec.opts.Stdout == nil || rec.opts.Stderr == nil {
			t.Fatal("executed with a nil stream, want both bound to nohup.out")
		}
		if rec.opts.Stdout != rec.opts.Stderr {
			t.Errorf("standard error %v is not the open file of standard output %v",
				rec.opts.Stderr, rec.opts.Stdout)
		}
		if got, want := string(fsys["nohup.out"].Data), "out\nout\n"; got != want {
			t.Errorf("nohup.out = %q, want %q", got, want)
		}
		if got, want := stderr(t, rt), "nohup: appending output to 'nohup.out'\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("a terminal standard input is announced as ignored", func(t *testing.T) {
		fsys := fsx.MemFS{}
		rt, rec := run(t, fsys, map[int]bool{0: true, 1: true}, false, nil, "true")

		if rec.opts.Stdout == nil {
			t.Fatal("executed with a nil standard output, want nohup.out")
		}
		if got, want := stderr(t, rt), "nohup: ignoring input and appending output to 'nohup.out'\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("an ignored input alone is announced", func(t *testing.T) {
		rt, rec := run(t, fsx.MemFS{}, map[int]bool{0: true}, false, nil, "true")

		if rec.opts.Stdout != nil || rec.opts.Stderr != nil {
			t.Error("executed with redirected streams, want the runtime ones")
		}
		if got, want := stderr(t, rt), "nohup: ignoring input\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("a terminal standard error with a non-terminal output", func(t *testing.T) {
		fsys := fsx.MemFS{}
		rt, rec := run(t, fsys, map[int]bool{2: true}, false, nil, "true")

		if rec.opts.Stdout != nil {
			t.Errorf("executed with a standard output of %v, want the runtime stream", rec.opts.Stdout)
		}
		if rec.opts.Stderr != rt.Stdout {
			t.Errorf("executed with a standard error of %v, want the standard output %v",
				rec.opts.Stderr, rt.Stdout)
		}
		if got, want := stderr(t, rt), "nohup: redirecting standard error to standard output\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
		if got, want := stdout(t, rt), "out\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if fsys["nohup.out"] != nil {
			t.Error("nohup.out created, want none")
		}
	})

	t.Run("a terminal standard error with a closed output", func(t *testing.T) {
		fsys := fsx.MemFS{}
		rec := &execRecord{}
		rt := newRuntime(rec, fsys, map[int]bool{2: true}, nil)
		rec.reply = writeReply(rt, "out\n", true)
		rt.Stdout = nil

		if err := New(rt).Run(t.Context(), "true"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}

		if rec.opts.Stdout != nil {
			t.Errorf("executed with a standard output of %v, want none", rec.opts.Stdout)
		}
		if got, want := string(fsys["nohup.out"].Data), "out\n"; got != want {
			t.Errorf("nohup.out = %q, want %q", got, want)
		}
		want := "nohup: appending output to 'nohup.out'\n" +
			"nohup: redirecting standard error to standard output\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("the output file is closed after the run", func(t *testing.T) {
		fsys := fsx.MemFS{}
		rec := &execRecord{}
		rt := newRuntime(rec, fsys, map[int]bool{1: true, 2: true}, nil)
		rec.reply = writeReply(rt, "out\n", true)

		if err := New(rt).Run(t.Context(), "true"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}

		if _, err := rec.opts.Stdout.Write([]byte("late")); !errors.Is(err, fs.ErrClosed) {
			t.Errorf("Write() error = %v, want %v", err, fs.ErrClosed)
		}
	})

	t.Run("the output file is closed when the execution fails", func(t *testing.T) {
		fsys := fsx.MemFS{}
		rec := &execRecord{status: 127, err: notFoundErr}
		rt := newRuntime(rec, fsys, map[int]bool{1: true}, nil)

		if err := New(rt).Run(t.Context(), "missing"); err == nil {
			t.Fatal("Run() error = nil, want an error")
		}

		if _, err := rec.opts.Stdout.Write([]byte("late")); !errors.Is(err, fs.ErrClosed) {
			t.Errorf("Write() error = %v, want %v", err, fs.ErrClosed)
		}
	})

	t.Run("the output file is closed when no execution is available", func(t *testing.T) {
		// The execution never runs, so the open file can only be taken
		// from the file system itself.
		fsys := &recFS{MemFS: fsx.MemFS{}}
		rec := &execRecord{}
		rt := newRuntime(rec, nil, map[int]bool{1: true}, nil)
		rt.FS = fsys
		rt.Exec = nil

		if err := New(rt).Run(t.Context(), "true"); err == nil {
			t.Fatal("Run() error = nil, want an error")
		}

		if len(fsys.files) != 1 {
			t.Fatalf("opened %d files, want 1", len(fsys.files))
		}
		if _, err := fsys.files[0].Write([]byte("late")); !errors.Is(err, fs.ErrClosed) {
			t.Errorf("Write() error = %v, want %v", err, fs.ErrClosed)
		}
	})
}

// recFS records the files OpenFile hands out, so a test can tell whether
// they were closed.
type recFS struct {
	fsx.MemFS
	files []fsx.File
}

// OpenFile implements fsx.WriteFS.
func (m *recFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	f, err := m.MemFS.OpenFile(name, flag, perm)
	if err == nil {
		m.files = append(m.files, f)
	}
	return f, err
}

func TestFallback(t *testing.T) {
	// A file system whose working directory cannot hold nohup.out, with a
	// home directory that can: the runs fall back to $HOME/nohup.out.
	newFS := func() fsx.MemFS {
		return fsx.MemFS{"home": {Mode: fs.ModeDir | 0o755}}
	}
	environ := []string{"HOME=home"}

	t.Run("the home file is used when the working directory cannot", func(t *testing.T) {
		fsys := newFS()
		rec := &execRecord{}
		rt := newRuntime(rec, fsys, map[int]bool{1: true}, environ)
		rec.reply = writeReply(rt, "out\n", false)
		rt.WorkingDir = "missing"

		if err := New(rt).Run(t.Context(), "true"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}

		if fsys["nohup.out"] != nil {
			t.Error("nohup.out created, want none")
		}
		if got, want := string(fsys["home/nohup.out"].Data), "out\n"; got != want {
			t.Errorf("home/nohup.out = %q, want %q", got, want)
		}
		if got, want := stderr(t, rt), "nohup: appending output to 'home/nohup.out'\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})

	t.Run("without HOME the failure is reported", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(rec, newFS(), map[int]bool{1: true}, nil)
		rt.WorkingDir = "missing"

		err := New(rt).Run(t.Context(), "true")

		if code := testutil.ExitCode(t, err); code != 125 {
			t.Errorf("Run() exit status = %d, want 125", code)
		}
		if got, want := stderr(t, rt), "nohup: failed to open 'nohup.out': file does not exist\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
		if rec.called {
			t.Errorf("executed %q, want no execution at all", rec.opts.Name)
		}
	})

	t.Run("both failures are reported", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(rec, newFS(), map[int]bool{1: true}, []string{"HOME=nohome"})
		rt.WorkingDir = "missing"

		err := New(rt).Run(t.Context(), "true")

		if code := testutil.ExitCode(t, err); code != 125 {
			t.Errorf("Run() exit status = %d, want 125", code)
		}
		want := "nohup: failed to open 'nohup.out': file does not exist\n" +
			"nohup: failed to open 'nohome/nohup.out': file does not exist\n"
		if got := stderr(t, rt); got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
		if rec.called {
			t.Errorf("executed %q, want no execution at all", rec.opts.Name)
		}
	})

	t.Run("no file system fails the run", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(rec, nil, map[int]bool{1: true}, nil)

		err := New(rt).Run(t.Context(), "true")

		if code := testutil.ExitCode(t, err); code != 125 {
			t.Errorf("Run() exit status = %d, want 125", code)
		}
		if got, want := stderr(t, rt), "nohup: failed to open 'nohup.out': no file system available\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
		if rec.called {
			t.Errorf("executed %q, want no execution at all", rec.opts.Name)
		}
	})
}

// notFoundErr and notInvokedErr stand for the errors the real execution
// reports for a utility which cannot be found or invoked.
var (
	notFoundErr   = errors.New(`exec: "missing": executable file not found in $PATH`)
	notInvokedErr = errors.New("fork/exec /bin/missing: permission denied")
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
			wantErr:  "nohup: " + notFoundErr.Error() + "\n",
		},
		{
			name:     "utility not invocable exits with 126",
			status:   126,
			err:      notInvokedErr,
			wantCode: 126,
			wantErr:  "nohup: " + notInvokedErr.Error() + "\n",
		},
		{
			name:     "terminated by a signal exits with 125",
			status:   125,
			err:      terminatedErr,
			wantCode: 125,
			wantErr:  "nohup: " + terminatedErr.Error() + "\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := &execRecord{status: test.status, err: test.err}
			rt := newRuntime(rec, fsx.MemFS{}, nil, nil)

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
			wantErr: "nohup: missing operand\nTry 'nohup --help' for more information.\n",
		},
		{
			name:    "invalid option",
			args:    []string{"-x", "true"},
			wantErr: "nohup: invalid option -- 'x'\nTry 'nohup --help' for more information.\n",
		},
		{
			name:    "unrecognized long option",
			args:    []string{"--posix", "true"},
			wantErr: "nohup: unrecognized option '--posix'\nTry 'nohup --help' for more information.\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := &execRecord{}
			rt := newRuntime(rec, fsx.MemFS{}, nil, nil)

			err := New(rt).Run(t.Context(), test.args...)

			var exitErr *coreutils.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
			}
			if exitErr.Code != 125 {
				t.Errorf("Run() exit status = %d, want 125", exitErr.Code)
			}
			if got := stderr(t, rt); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
			if rec.called {
				t.Errorf("executed %q, want no execution at all", rec.opts.Name)
			}
		})
	}
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "nohup", usage)

	t.Run("help after the utility name is an argument", func(t *testing.T) {
		rec := &execRecord{}
		rt := newRuntime(rec, nil, nil, nil)

		if err := New(rt).Run(t.Context(), "true", "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := rec.opts.Name, "true"; got != want {
			t.Errorf("executed %q, want %q", got, want)
		}
		if got, want := rec.opts.Args, []string{"--help"}; !slices.Equal(got, want) {
			t.Errorf("executed with args %v, want %v", got, want)
		}
	})
}

func TestContextCancellation(t *testing.T) {
	t.Run("canceled context runs nothing", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		fsys := fsx.MemFS{}
		rec := &execRecord{}
		rt := newRuntime(rec, fsys, map[int]bool{1: true}, nil)

		if err := New(rt).Run(ctx, "true"); err != context.Canceled {
			t.Fatalf("Run() error = %v, want the context error itself", err)
		}
		if rec.called {
			t.Errorf("executed %q, want no execution at all", rec.opts.Name)
		}
		if got := stderr(t, rt); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
		if fsys["nohup.out"] != nil {
			t.Error("nohup.out created, want none")
		}
	})

	t.Run("a canceled context error of the execution is returned as-is", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		rec := &execRecord{reply: func(context.Context, coreutils.ExecOpts) (int, error) {
			return 125, errors.New("signal: killed")
		}}
		rt := newRuntime(rec, fsx.MemFS{}, nil, nil)

		if err := New(rt).Run(ctx, "true"); err != context.Canceled {
			t.Fatalf("Run() error = %v, want the context error itself", err)
		}
		if got := stderr(t, rt); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
	})
}

func TestNoExec(t *testing.T) {
	rt := newRuntime(nil, fsx.MemFS{}, nil, nil)

	err := New(rt).Run(t.Context(), "true")

	if code := testutil.ExitCode(t, err); code != 125 {
		t.Errorf("Run() exit status = %d, want 125", code)
	}
	if got, want := stderr(t, rt), "nohup: no command execution available\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
	if got := stdout(t, rt); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
}
