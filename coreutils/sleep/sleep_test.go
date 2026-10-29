package sleep

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"charm.land/snail/coreutils"
)

type fakeSleeper struct {
	durations []time.Duration
	err       error
}

func (s *fakeSleeper) sleep(ctx context.Context, d time.Duration) error {
	s.durations = append(s.durations, d)
	if s.err != nil {
		return s.err
	}
	return ctx.Err()
}

func runSleep(t *testing.T, sleeper *fakeSleeper, ctx context.Context, args ...string) (string, error) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  bytes.NewReader(nil),
		Stdout: stdout,
		Stderr: stderr,
		Sleep:  sleeper.sleep,
	}
	err := New(rt).Run(ctx, args...)

	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
	return stderr.String(), err
}

func TestRun(t *testing.T) {
	t.Run("time operand", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want []time.Duration
		}{
			{
				name: "zero seconds",
				args: []string{"0"},
				want: []time.Duration{0},
			},
			{
				name: "one second",
				args: []string{"1"},
				want: []time.Duration{time.Second},
			},
			{
				name: "multiple seconds",
				args: []string{"105"},
				want: []time.Duration{105 * time.Second},
			},
			{
				name: "leading zeros are allowed",
				args: []string{"007"},
				want: []time.Duration{7 * time.Second},
			},
			{
				name: "the maximum required value",
				args: []string{"2147483647"},
				want: []time.Duration{2147483647 * time.Second},
			},
			{
				name: "double dash ends the options",
				args: []string{"--", "2"},
				want: []time.Duration{2 * time.Second},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				sleeper := &fakeSleeper{}
				stderr, err := runSleep(t, sleeper, t.Context(), test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if !slices.Equal(sleeper.durations, test.want) {
					t.Errorf("slept for %v, want %v", sleeper.durations, test.want)
				}
				if got := stderr; got != "" {
					t.Errorf("Run() stderr = %q, want empty", got)
				}
			})
		}
	})

	t.Run("context interruption", func(t *testing.T) {
		t.Run("canceled context is returned as-is", func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			sleeper := &fakeSleeper{}
			_, err := runSleep(t, sleeper, ctx, "10")
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v, want context.Canceled", err)
			}
			if err != context.Canceled {
				t.Errorf("Run() error = %v, want the context error itself", err)
			}
			if len(sleeper.durations) != 0 {
				t.Errorf("slept for %v, want no sleep at all", sleeper.durations)
			}
		})

		t.Run("canceled context is returned as-is even for a zero operand", func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			sleeper := &fakeSleeper{}
			_, err := runSleep(t, sleeper, ctx, "0")
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v, want context.Canceled", err)
			}
			if err != context.Canceled {
				t.Errorf("Run() error = %v, want the context error itself", err)
			}
			if len(sleeper.durations) != 0 {
				t.Errorf("slept for %v, want no sleep at all", sleeper.durations)
			}
		})

		t.Run("sleep error is returned as-is", func(t *testing.T) {
			sleepErr := errors.New("interrupted")

			sleeper := &fakeSleeper{err: sleepErr}
			_, err := runSleep(t, sleeper, t.Context(), "1")
			if err != sleepErr {
				t.Fatalf("Run() error = %v, want the sleep error itself", err)
			}
			if !slices.Equal(sleeper.durations, []time.Duration{time.Second}) {
				t.Errorf("slept for %v, want one 1s interval", sleeper.durations)
			}
		})
	})

	t.Run("usage errors", func(t *testing.T) {
		tests := []struct {
			name    string
			args    []string
			wantErr string
		}{
			{
				name:    "missing operand",
				wantErr: "sleep: missing operand\n",
			},
			{
				name:    "extra operand",
				args:    []string{"1", "2"},
				wantErr: "sleep: extra operand: \"2\"\n",
			},
			{
				name:    "invalid option",
				args:    []string{"-x"},
				wantErr: "sleep: invalid option -- 'x'\n",
			},
			{
				name:    "negative operand",
				args:    []string{"-5"},
				wantErr: "sleep: invalid option -- '5'\n",
			},
			{
				name:    "fractional operand",
				args:    []string{"1.5"},
				wantErr: "sleep: invalid number: '1.5'\n",
			},
			{
				name:    "non-numeric operand",
				args:    []string{"abc"},
				wantErr: "sleep: invalid number: 'abc'\n",
			},
			{
				name:    "empty operand",
				args:    []string{""},
				wantErr: "sleep: invalid number: ''\n",
			},
			{
				name:    "trailing garbage",
				args:    []string{"1x"},
				wantErr: "sleep: invalid number: '1x'\n",
			},
			{
				name:    "value too large",
				args:    []string{"9223372036854775808"},
				wantErr: "sleep: invalid number: '9223372036854775808'\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				sleeper := &fakeSleeper{}
				stderr, err := runSleep(t, sleeper, t.Context(), test.args...)

				var exitErr *coreutils.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("Run() error = %v, want a *coreutils.ExitError", err)
				}
				if exitErr.Code != 2 {
					t.Errorf("Run() exit status = %d, want 2", exitErr.Code)
				}
				if got := stderr; got != test.wantErr {
					t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
				}
				if len(sleeper.durations) != 0 {
					t.Errorf("slept for %v, want no sleep at all", sleeper.durations)
				}
			})
		}
	})
}
