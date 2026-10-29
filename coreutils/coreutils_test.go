package coreutils

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

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
		status, err := osExec(t.Context(), rt, "definitely-not-a-utility", nil)
		if status != 127 {
			t.Errorf("status = %d, want 127", status)
		}
		if err == nil {
			t.Error("error = nil, want an error")
		}
	})

	t.Run("utility not found", func(t *testing.T) {
		rt := OsRuntime()
		status, err := osExec(t.Context(), rt, "definitely-not-a-utility", nil)
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
		rt := OsRuntime()
		rt.LookupEnv = func(key string) (string, bool) {
			if key == "PATH" {
				return dir, true
			}
			return os.LookupEnv(key)
		}
		if status, err := osExec(t.Context(), rt, "myutil", nil); err != nil || status != 3 {
			t.Errorf("osExec() = %d, %v, want 3, nil", status, err)
		}
	})

	t.Run("utility named by a path that doesn't exist", func(t *testing.T) {
		rt := OsRuntime()
		status, err := osExec(t.Context(), rt, "./definitely-not-a-utility", nil)
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
		rt := OsRuntime()
		rt.LookupEnv = func(key string) (string, bool) {
			if key == "PATH" {
				return dir, true
			}
			return os.LookupEnv(key)
		}
		if status, err := osExec(t.Context(), rt, "myutil", nil); err != nil || status != 0 {
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
		rt := OsRuntime()
		rt.LookupEnv = func(key string) (string, bool) {
			if key == "PATH" {
				return dir, true
			}
			return os.LookupEnv(key)
		}
		if status, err := osExec(t.Context(), rt, "myutil", nil); status != 126 || err == nil {
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
		rt := OsRuntime()
		rt.WorkingDir = dir
		rt.LookupEnv = func(key string) (string, bool) {
			if key == "PATH" {
				return ".", true
			}
			return os.LookupEnv(key)
		}
		if status, err := osExec(t.Context(), rt, "myutil", nil); err != nil || status != 0 {
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
		rt := OsRuntime()
		rt.WorkingDir = "sub"
		rt.LookupEnv = func(key string) (string, bool) {
			if key == "PATH" {
				return ".", true
			}
			return os.LookupEnv(key)
		}
		if status, err := osExec(t.Context(), rt, "myutil", nil); err != nil || status != 0 {
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
		rt := OsRuntime()
		rt.WorkingDir = dir
		rt.LookupEnv = func(key string) (string, bool) {
			if key == "PATH" {
				return "", true
			}
			return os.LookupEnv(key)
		}
		if status, err := osExec(t.Context(), rt, "myutil", nil); err != nil || status != 0 {
			t.Errorf("osExec() = %d, %v, want 0, nil", status, err)
		}
	})

	t.Run("unset PATH means the default search path", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs the default Unix search path")
		}
		rt := OsRuntime()
		rt.LookupEnv = func(key string) (string, bool) {
			if key == "PATH" {
				return "", false
			}
			return os.LookupEnv(key)
		}
		if status, err := osExec(t.Context(), rt, "sh", []string{"-c", "exit 0"}); err != nil || status != 0 {
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
		rt := OsRuntime()
		rt.LookupEnv = func(key string) (string, bool) {
			if key == "PATH" {
				return dir, true
			}
			return os.LookupEnv(key)
		}
		if status, err := osExec(t.Context(), rt, "myutil", nil); status != 126 || err == nil {
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

// TestIsTerminal covers the platform helper itself.
func TestIsTerminal(t *testing.T) {
	t.Run("a regular file", func(t *testing.T) {
		f, err := os.Create(filepath.Join(t.TempDir(), "f"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })

		if isTerminal(int(f.Fd())) {
			t.Error("isTerminal() = true for a regular file, want false")
		}
	})

	t.Run("a character device", func(t *testing.T) {
		f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })

		if isTerminal(int(f.Fd())) {
			t.Error("isTerminal() = true for a character device, want false")
		}
	})

	t.Run("a terminal", func(t *testing.T) {
		f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			t.Skipf("opening /dev/tty: %v", err)
		}
		t.Cleanup(func() { f.Close() })

		if !isTerminal(int(f.Fd())) {
			t.Error("isTerminal() = false for a terminal, want true")
		}
	})
}

func TestOsRuntime(t *testing.T) {
	rt := OsRuntime()

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
	if rt.LookupEnv == nil {
		t.Error("LookupEnv was not set")
	}
	if rt.Environ == nil {
		t.Error("Environ was not set")
	}
	if rt.Clock == nil {
		t.Error("Clock was not set")
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

	t.Run("inherits the current environment when Env is nil", func(t *testing.T) {
		t.Setenv("FOO", "inherited")
		rt := OsRuntime()
		rt.LookupEnv = func(key string) (string, bool) {
			if key == "PATH" {
				return dir, true
			}
			return os.LookupEnv(key)
		}
		out := new(bytes.Buffer)
		rt.Stdout = out
		if status, err := osExec(t.Context(), rt, "myutil", nil); err != nil || status != 0 {
			t.Fatalf("osExec() = %d, %v, want 0, nil", status, err)
		}
		if got := out.String(); got != "FOO=inherited\n" {
			t.Errorf("utility saw environment %q, want %q", got, "FOO=inherited\n")
		}
	})

	t.Run("runs the utility with the environment set in the runtime", func(t *testing.T) {
		rt := OsRuntime()
		rt.LookupEnv = func(key string) (string, bool) {
			if key == "PATH" {
				return dir, true
			}
			return os.LookupEnv(key)
		}
		rt.Env = []string{"FOO=overridden", "PATH=" + dir}
		out := new(bytes.Buffer)
		rt.Stdout = out
		if status, err := osExec(t.Context(), rt, "myutil", nil); err != nil || status != 0 {
			t.Fatalf("osExec() = %d, %v, want 0, nil", status, err)
		}
		if got := out.String(); got != "FOO=overridden\n" {
			t.Errorf("utility saw environment %q, want %q", got, "FOO=overridden\n")
		}
	})

	t.Run("runs the utility with an empty environment", func(t *testing.T) {
		rt := OsRuntime()
		rt.LookupEnv = func(key string) (string, bool) {
			if key == "PATH" {
				return dir, true
			}
			return os.LookupEnv(key)
		}
		rt.Env = []string{"PATH=" + dir}
		out := new(bytes.Buffer)
		rt.Stdout = out
		if status, err := osExec(t.Context(), rt, "myutil", nil); err != nil || status != 0 {
			t.Fatalf("osExec() = %d, %v, want 0, nil", status, err)
		}
		if got := out.String(); got != "FOO=\n" {
			t.Errorf("utility saw environment %q, want %q", got, "FOO=\n")
		}
	})
}
