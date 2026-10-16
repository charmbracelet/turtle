package coreutils

import (
	"context"
	"errors"
	"os"
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
}
