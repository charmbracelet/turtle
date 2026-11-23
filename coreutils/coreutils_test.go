package coreutils

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// envWithPath returns a function enumerating the process environment with
// PATH set to the given value.
func envWithPath(path string) func() []string {
	return func() []string {
		return append(envWithoutPath(), "PATH="+path)
	}
}

// envWithoutPath returns the process environment without PATH.
func envWithoutPath() []string {
	var env []string
	for _, entry := range os.Environ() {
		if name, _, _ := strings.Cut(entry, "="); name != "PATH" {
			env = append(env, entry)
		}
	}
	return env
}

func TestExitError(t *testing.T) {
	inner := errors.New("boom")

	tests := []struct {
		name    string
		err     *ExitError
		wantMsg string
		wantErr error
	}{
		{
			name:    "code only",
			err:     &ExitError{Code: 1},
			wantMsg: "exit status 1",
		},
		{
			name:    "code and wrapped error",
			err:     &ExitError{Code: 2, Err: inner},
			wantMsg: "exit status 2: boom",
			wantErr: inner,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.wantMsg {
				t.Errorf("Error() = %q, want %q", got, test.wantMsg)
			}
			if got := test.err.Unwrap(); got != test.wantErr {
				t.Errorf("Unwrap() = %v, want %v", got, test.wantErr)
			}
			if test.wantErr != nil && !errors.Is(test.err, test.wantErr) {
				t.Errorf("errors.Is(%v, %v) = false, want true", test.err, test.wantErr)
			}
		})
	}
}

func TestOsExec(t *testing.T) {
	t.Run("runtime without a lookup function", func(t *testing.T) {
		rt := &Runtime{}
		status, err := osExec(t.Context(), rt, ExecOpts{Name: "definitely-not-a-utility"})
		if status != 127 {
			t.Errorf("status = %d, want 127", status)
		}
		if err == nil {
			t.Error("error = nil, want an error")
		}
	})

	t.Run("utility not found", func(t *testing.T) {
		rt := SystemRuntime("")
		status, err := osExec(t.Context(), rt, ExecOpts{Name: "definitely-not-a-utility"})
		if status != 127 {
			t.Errorf("status = %d, want 127", status)
		}
		if err == nil {
			t.Error("error = nil, want an error")
		}
	})

	t.Run("executable text file without a shebang", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs the command interpreter fallback")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "myutil")
		if err := os.WriteFile(path, []byte("exit 3\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		rt := SystemRuntime("")
		rt.Environ = envWithPath(dir)
		if status, err := osExec(t.Context(), rt, ExecOpts{Name: "myutil"}); err != nil || status != 3 {
			t.Errorf("osExec() = %d, %v, want 3, nil", status, err)
		}
	})

	t.Run("utility named by a path that doesn't exist", func(t *testing.T) {
		rt := SystemRuntime("")
		status, err := osExec(t.Context(), rt, ExecOpts{Name: "./definitely-not-a-utility"})
		if status != 127 {
			t.Errorf("status = %d, want 127", status)
		}
		if err == nil {
			t.Error("error = nil, want an error")
		}
	})

	t.Run("uses the PATH of the runtime", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs the executable permission bits")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "myutil")
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		rt := SystemRuntime("")
		rt.Environ = envWithPath(dir)
		if status, err := osExec(t.Context(), rt, ExecOpts{Name: "myutil"}); err != nil || status != 0 {
			t.Errorf("osExec() = %d, %v, want 0, nil", status, err)
		}
	})

	t.Run("utility found but not executable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs the executable permission bits")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "myutil")
		if err := os.WriteFile(path, []byte("not executable\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		rt := SystemRuntime("")
		rt.Environ = envWithPath(dir)
		if status, err := osExec(t.Context(), rt, ExecOpts{Name: "myutil"}); status != 126 || err == nil {
			t.Errorf("osExec() = %d, %v, want 126, an error", status, err)
		}
	})

	t.Run("relative PATH entry resolved in the working directory", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs the executable permission bits")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "myutil")
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		rt := SystemRuntime("")
		rt.WorkingDir = dir
		rt.Environ = envWithPath(".")
		if status, err := osExec(t.Context(), rt, ExecOpts{Name: "myutil"}); err != nil || status != 0 {
			t.Errorf("osExec() = %d, %v, want 0, nil", status, err)
		}
	})

	t.Run("relative working directory", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs the executable permission bits")
		}
		dir := t.TempDir()
		sub := filepath.Join(dir, "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(sub, "myutil")
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Chdir(dir)
		rt := SystemRuntime("")
		rt.WorkingDir = "sub"
		rt.Environ = envWithPath(".")
		if status, err := osExec(t.Context(), rt, ExecOpts{Name: "myutil"}); err != nil || status != 0 {
			t.Errorf("osExec() = %d, %v, want 0, nil", status, err)
		}
	})

	t.Run("empty PATH means the working directory", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs the executable permission bits")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "myutil")
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		rt := SystemRuntime("")
		rt.WorkingDir = dir
		rt.Environ = envWithPath("")
		if status, err := osExec(t.Context(), rt, ExecOpts{Name: "myutil"}); err != nil || status != 0 {
			t.Errorf("osExec() = %d, %v, want 0, nil", status, err)
		}
	})

	t.Run("unset PATH means the default search path", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs the default Unix search path")
		}
		rt := SystemRuntime("")
		rt.Environ = func() []string { return envWithoutPath() }
		if status, err := osExec(t.Context(), rt, ExecOpts{Name: "sh", Args: []string{"-c", "exit 0"}}); err != nil || status != 0 {
			t.Errorf("osExec() = %d, %v, want 0, nil", status, err)
		}
	})

	t.Run("PATH entry that cannot be searched", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs directory permission bits")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "myutil")
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(dir, 0o755) })
		rt := SystemRuntime("")
		rt.Environ = envWithPath(dir)
		if status, err := osExec(t.Context(), rt, ExecOpts{Name: "myutil"}); status != 126 || err == nil {
			t.Errorf("osExec() = %d, %v, want 126, an error", status, err)
		}
	})
}

func TestRuntimeWait(t *testing.T) {
	t.Run("waits for the given duration", func(t *testing.T) {
		rt := &Runtime{}
		if err := rt.Wait(t.Context(), time.Millisecond); err != nil {
			t.Errorf("Wait() error = %v, want nil", err)
		}
	})

	t.Run("returns the context error when it's done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		rt := &Runtime{}
		if err := rt.Wait(ctx, time.Hour); !errors.Is(err, context.Canceled) {
			t.Errorf("Wait() error = %v, want context.Canceled", err)
		}
	})

	t.Run("uses the Sleep function when set", func(t *testing.T) {
		var (
			gotCtx context.Context
			gotDur time.Duration
		)
		sleepErr := errors.New("boom")
		rt := &Runtime{
			Sleep: func(ctx context.Context, d time.Duration) error {
				gotCtx, gotDur = ctx, d
				return sleepErr
			},
		}

		ctx := t.Context()
		if err := rt.Wait(ctx, 5*time.Second); !errors.Is(err, sleepErr) {
			t.Errorf("Wait() error = %v, want %v", err, sleepErr)
		}
		if gotCtx != ctx {
			t.Error("Wait() didn't pass the context to Sleep")
		}
		if gotDur != 5*time.Second {
			t.Errorf("Wait() duration = %v, want %v", gotDur, 5*time.Second)
		}
	})
}

func TestRuntimeNow(t *testing.T) {
	t.Run("uses the Clock function when set", func(t *testing.T) {
		want := time.Date(1990, 6, 26, 9, 58, 10, 0, time.UTC)
		rt := &Runtime{Clock: func() time.Time { return want }}

		if got := rt.Now(); !got.Equal(want) {
			t.Errorf("Now() = %v, want %v", got, want)
		}
	})

	t.Run("falls back to real time", func(t *testing.T) {
		before := time.Now()
		got := (&Runtime{}).Now()
		after := time.Now()
		if got.Before(before) || got.After(after) {
			t.Errorf("Now() = %v, want a time between %v and %v", got, before, after)
		}
	})
}

func TestRuntimeTerminal(t *testing.T) {
	t.Run("uses the IsTerminal function when set", func(t *testing.T) {
		var gotFd int
		rt := &Runtime{
			IsTerminal: func(fd int) bool {
				gotFd = fd
				return fd == 2
			},
		}

		if !rt.Terminal(2) {
			t.Error("Terminal(2) = false, want true")
		}
		if gotFd != 2 {
			t.Errorf("IsTerminal called with %d, want 2", gotFd)
		}
		if rt.Terminal(1) {
			t.Error("Terminal(1) = true, want false")
		}
	})

	t.Run("falls back to the standard streams", func(t *testing.T) {
		t.Run("a character device file is not a terminal", func(t *testing.T) {
			f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { f.Close() })

			rt := &Runtime{Stdin: f}
			if rt.Terminal(0) {
				t.Error("Terminal(0) = true, want false for a character device")
			}
		})

		t.Run("a regular file is not a terminal", func(t *testing.T) {
			f, err := os.Create(filepath.Join(t.TempDir(), "f"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { f.Close() })

			rt := &Runtime{Stdout: f}
			if rt.Terminal(1) {
				t.Error("Terminal(1) = true, want false for a regular file")
			}
		})

		t.Run("a stream that is not a file", func(t *testing.T) {
			rt := &Runtime{Stderr: new(bytes.Buffer)}
			if rt.Terminal(2) {
				t.Error("Terminal(2) = true, want false for a non-file")
			}
		})

		t.Run("a descriptor without a stream", func(t *testing.T) {
			rt := &Runtime{}
			if rt.Terminal(3) {
				t.Error("Terminal(3) = true, want false")
			}
		})

		t.Run("a real terminal when the process has one", func(t *testing.T) {
			f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
			if err != nil {
				t.Skipf("opening /dev/tty: %v", err)
			}
			t.Cleanup(func() { f.Close() })

			rt := &Runtime{Stdin: f}
			if !rt.Terminal(0) {
				t.Error("Terminal(0) = false, want true for a terminal")
			}
		})
	})
}

func TestSystemRuntime(t *testing.T) {
	rt := SystemRuntime("")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	if rt.WorkingDir != wd {
		t.Errorf("WorkingDir = %q, want %q", rt.WorkingDir, wd)
	}
	if rt.FS == nil {
		t.Error("FS was not set")
	}
	if rt.Environ == nil {
		t.Error("Environ was not set")
	}
	if rt.Clock == nil {
		t.Error("Clock was not set")
	}
}

func TestSystemRuntimeWorkingDir(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		dir  string
	}{
		{name: "absolute", dir: dir},
		{name: "relative", dir: relative},
	} {
		t.Run(test.name, func(t *testing.T) {
			rt := SystemRuntime(test.dir)
			if rt.WorkingDir != dir {
				t.Errorf("WorkingDir = %q, want %q", rt.WorkingDir, dir)
			}
			want := filepath.ToSlash(filepath.Join(dir, "file.txt"))
			if got := rt.FsPath("file.txt"); got != want {
				t.Errorf("FsPath() = %q, want %q", got, want)
			}
		})
	}
}

func TestOsExecEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs the executable permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "myutil")
	script := []byte("#!/bin/sh\necho \"FOO=$FOO\"\n")
	if err := os.WriteFile(path, script, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("runs the utility with the environment given", func(t *testing.T) {
		rt := SystemRuntime("")
		rt.Environ = envWithPath(dir)
		out := new(bytes.Buffer)
		rt.Stdout = out
		env := []string{"FOO=overridden", "PATH=" + dir}
		if status, err := osExec(t.Context(), rt, ExecOpts{Name: "myutil", Env: env}); err != nil || status != 0 {
			t.Fatalf("osExec() = %d, %v, want 0, nil", status, err)
		}
		if got := out.String(); got != "FOO=overridden\n" {
			t.Errorf("utility saw environment %q, want %q", got, "FOO=overridden\n")
		}
	})

	t.Run("a nil environment is an empty one", func(t *testing.T) {
		t.Setenv("FOO", "inherited")
		rt := SystemRuntime("")
		rt.Environ = nil
		out := new(bytes.Buffer)
		rt.Stdout = out
		if status, err := osExec(t.Context(), rt, ExecOpts{Name: path}); err != nil || status != 0 {
			t.Fatalf("osExec() = %d, %v, want 0, nil", status, err)
		}
		if got := out.String(); got != "FOO=\n" {
			t.Errorf("utility saw environment %q, want %q", got, "FOO=\n")
		}
	})
}

func TestRuntimeEnv(t *testing.T) {
	environ := []string{"FOO=1", "BAR=2"}

	t.Run("LookupEnv", func(t *testing.T) {
		t.Run("returns set variables", func(t *testing.T) {
			rt := &Runtime{Environ: func() []string { return environ }}
			if got, ok := rt.LookupEnv("FOO"); !ok || got != "1" {
				t.Errorf("LookupEnv(\"FOO\") = %q, %v, want \"1\", true", got, ok)
			}
		})

		t.Run("reports unset variables", func(t *testing.T) {
			rt := &Runtime{Environ: func() []string { return environ }}
			if got, ok := rt.LookupEnv("MISSING"); ok || got != "" {
				t.Errorf("LookupEnv(\"MISSING\") = %q, %v, want \"\", false", got, ok)
			}
		})

		t.Run("returns the first match", func(t *testing.T) {
			rt := &Runtime{Environ: func() []string { return []string{"FOO=1", "FOO=2"} }}
			if got, ok := rt.LookupEnv("FOO"); !ok || got != "1" {
				t.Errorf("LookupEnv(\"FOO\") = %q, %v, want \"1\", true", got, ok)
			}
		})

		t.Run("finds nothing without an Environ function", func(t *testing.T) {
			rt := &Runtime{}
			if got, ok := rt.LookupEnv("FOO"); ok || got != "" {
				t.Errorf("LookupEnv(\"FOO\") = %q, %v, want \"\", false", got, ok)
			}
		})
	})

	t.Run("Getenv", func(t *testing.T) {
		t.Run("returns set variables", func(t *testing.T) {
			rt := &Runtime{Environ: func() []string { return environ }}
			if got := rt.Getenv("BAR"); got != "2" {
				t.Errorf("Getenv(\"BAR\") = %q, want \"2\"", got)
			}
		})

		t.Run("returns the empty string for unset variables", func(t *testing.T) {
			rt := &Runtime{Environ: func() []string { return environ }}
			if got := rt.Getenv("MISSING"); got != "" {
				t.Errorf("Getenv(\"MISSING\") = %q, want empty", got)
			}
		})

		t.Run("returns the empty string without an Environ function", func(t *testing.T) {
			rt := &Runtime{}
			if got := rt.Getenv("FOO"); got != "" {
				t.Errorf("Getenv(\"FOO\") = %q, want empty", got)
			}
		})
	})

	t.Run("EnvList", func(t *testing.T) {
		t.Run("returns the environment of the runtime", func(t *testing.T) {
			rt := &Runtime{Environ: func() []string { return environ }}
			if got := rt.EnvList(); !slices.Equal(got, environ) {
				t.Errorf("EnvList() = %v, want %v", got, environ)
			}
		})

		t.Run("returns nil without an Environ function", func(t *testing.T) {
			rt := &Runtime{}
			if got := rt.EnvList(); got != nil {
				t.Errorf("EnvList() = %v, want nil", got)
			}
		})
	})
}
