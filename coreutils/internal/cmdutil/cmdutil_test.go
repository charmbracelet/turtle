package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"charm.land/snail/coreutils"
)

func TestRunExitStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "success",
			want: 0,
		},
		{
			name: "exit error",
			err:  &coreutils.ExitError{Code: 2},
			want: 2,
		},
		{
			name: "wrapped exit error",
			err:  fmt.Errorf("run: %w", &coreutils.ExitError{Code: 3}),
			want: 3,
		},
		{
			name: "other error",
			err:  errors.New("boom"),
			want: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			new := programFunc(func(context.Context, ...string) error {
				return test.err
			})
			if got := run(new, nil); got != test.want {
				t.Errorf("run() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestRunArguments(t *testing.T) {
	var got []string
	new := programFunc(func(_ context.Context, args ...string) error {
		got = args
		return nil
	})

	args := []string{"-n", "hello"}
	if code := run(new, args); code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
	if !slices.Equal(got, args) {
		t.Errorf("Run() args = %v, want %v", got, args)
	}
}

// programFunc adapts a function to the coreutils.NewFunc signature.
func programFunc(run func(context.Context, ...string) error) coreutils.NewFunc {
	return func(*coreutils.Runtime) coreutils.Program {
		return program{run: run}
	}
}

type program struct {
	run func(context.Context, ...string) error
}

func (p program) Run(ctx context.Context, args ...string) error {
	return p.run(ctx, args...)
}
