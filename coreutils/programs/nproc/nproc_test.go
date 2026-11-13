package nproc

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
)

type testCase struct {
	name     string
	args     []string
	environ  []string
	numCPU   int
	want     string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout:  stdout,
		Stderr:  stderr,
		Environ: func() []string { return test.environ },
	}
	prog := &Prog{Runtime: rt, numCPU: func() int { return test.numCPU }}

	err := prog.Run(t.Context(), test.args...)

	if code := testutil.ExitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.want {
		t.Errorf("Run() stdout = %q, want %q", got, test.want)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
}

func TestRun(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		tests := []testCase{
			{
				name:   "the available count is printed",
				numCPU: 8,
				want:   "8\n",
			},
			{
				name:   "a single unit is printed as is",
				numCPU: 1,
				want:   "1\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("all", func(t *testing.T) {
		tests := []testCase{
			{
				name:   "--all prints the same count",
				args:   []string{"--all"},
				numCPU: 8,
				want:   "8\n",
			},
			{
				name: "--all ignores the OpenMP variables",
				args: []string{"--all"},
				environ: []string{
					"OMP_NUM_THREADS=16",
					"OMP_THREAD_LIMIT=2",
				},
				numCPU: 8,
				want:   "8\n",
			},
			{
				name:   "--all still honors --ignore",
				args:   []string{"--all", "--ignore=3"},
				numCPU: 8,
				want:   "5\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("ignore", func(t *testing.T) {
		tests := []testCase{
			{
				name:   "--ignore excludes that many units",
				args:   []string{"--ignore=3"},
				numCPU: 8,
				want:   "5\n",
			},
			{
				name:   "--ignore accepts zero units",
				args:   []string{"--ignore=0"},
				numCPU: 8,
				want:   "8\n",
			},
			{
				name:   "--ignore never goes below one",
				args:   []string{"--ignore=100"},
				numCPU: 8,
				want:   "1\n",
			},
			{
				name:   "--ignore never goes below one when the count is one",
				args:   []string{"--ignore=1"},
				numCPU: 1,
				want:   "1\n",
			},
			{
				name:   "--ignore accepts an attached value",
				args:   []string{"--ignore=2"},
				numCPU: 8,
				want:   "6\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("environment", func(t *testing.T) {
		tests := []testCase{
			{
				name:    "without OpenMP variables the count is unchanged",
				environ: []string{"PATH=/bin"},
				numCPU:  8,
				want:    "8\n",
			},
			{
				name:    "OMP_THREAD_LIMIT caps the count",
				environ: []string{"OMP_THREAD_LIMIT=2"},
				numCPU:  8,
				want:    "2\n",
			},
			{
				name:    "OMP_THREAD_LIMIT above the count changes nothing",
				environ: []string{"OMP_THREAD_LIMIT=16"},
				numCPU:  8,
				want:    "8\n",
			},
			{
				name:    "OMP_NUM_THREADS raises the count",
				environ: []string{"OMP_NUM_THREADS=16"},
				numCPU:  8,
				want:    "16\n",
			},
			{
				name:    "OMP_NUM_THREADS below the count changes nothing",
				environ: []string{"OMP_NUM_THREADS=2"},
				numCPU:  8,
				want:    "8\n",
			},
			{
				name: "both variables clamp the count",
				environ: []string{
					"OMP_NUM_THREADS=16",
					"OMP_THREAD_LIMIT=2",
				},
				numCPU: 8,
				want:   "2\n",
			},
			{
				name:    "an invalid OMP_THREAD_LIMIT is ignored",
				environ: []string{"OMP_THREAD_LIMIT=abc"},
				numCPU:  8,
				want:    "8\n",
			},
			{
				name:    "an invalid OMP_NUM_THREADS is ignored",
				environ: []string{"OMP_NUM_THREADS=0"},
				numCPU:  8,
				want:    "8\n",
			},
			{
				name:    "the variables apply before --ignore",
				environ: []string{"OMP_THREAD_LIMIT=4"},
				args:    []string{"--ignore=2"},
				numCPU:  8,
				want:    "2\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("usage errors", func(t *testing.T) {
		tests := []testCase{
			{
				name:     "an invalid ignore value is a usage error",
				args:     []string{"--ignore=x"},
				numCPU:   8,
				wantErr:  "nproc: invalid --ignore argument 'x'\nTry 'nproc --help' for more information.\n",
				wantExit: 2,
			},
			{
				name:     "a negative ignore value is a usage error",
				args:     []string{"--ignore=-1"},
				numCPU:   8,
				wantErr:  "nproc: invalid --ignore argument '-1'\nTry 'nproc --help' for more information.\n",
				wantExit: 2,
			},
			{
				name:     "--ignore without a value is a usage error",
				args:     []string{"--ignore"},
				numCPU:   8,
				wantErr:  "nproc: option '--ignore' requires an argument\nTry 'nproc --help' for more information.\n",
				wantExit: 2,
			},
			{
				name:     "an unknown option is a usage error",
				args:     []string{"--verbose"},
				numCPU:   8,
				wantErr:  "nproc: unrecognized option '--verbose'\nTry 'nproc --help' for more information.\n",
				wantExit: 2,
			},
			{
				name:     "an operand is a usage error",
				args:     []string{"4"},
				numCPU:   8,
				wantErr:  "nproc: extra operand '4'\nTry 'nproc --help' for more information.\n",
				wantExit: 2,
			},
			{
				name:     "an option-like operand is a usage error",
				args:     []string{"--", "--all"},
				numCPU:   8,
				wantErr:  "nproc: extra operand '--all'\nTry 'nproc --help' for more information.\n",
				wantExit: 2,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("write error", func(t *testing.T) {
		writeErr := errors.New("broken pipe")
		rt := &coreutils.Runtime{
			Stdout: testutil.ErrorWriter{Err: writeErr},
			Stderr: new(bytes.Buffer),
		}
		prog := &Prog{Runtime: rt, numCPU: func() int { return 8 }}

		err := prog.Run(t.Context())

		if code := testutil.ExitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
		if !errors.Is(err, writeErr) {
			t.Errorf("Run() error = %v, want it to wrap %v", err, writeErr)
		}
	})

	t.Run("context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		rt := &coreutils.Runtime{
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
		}
		prog := &Prog{Runtime: rt, numCPU: func() int { return 8 }}

		err := prog.Run(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want %v", err, context.Canceled)
		}
		if got := rt.Stdout.(*bytes.Buffer).String(); got != "" {
			t.Errorf("Run() stdout = %q, want empty", got)
		}
	})

	t.Run("new returns the default count", func(t *testing.T) {
		if got := runtime.NumCPU(); got < 1 {
			t.Fatalf("runtime.NumCPU() = %d, want at least 1", got)
		}
		prog := New(&coreutils.Runtime{Stdout: new(bytes.Buffer)})
		p, ok := prog.(*Prog)
		if !ok {
			t.Fatalf("New() = %T, want *Prog", prog)
		}
		if p.numCPU != nil {
			t.Error("New() set numCPU, want nil to fall back to runtime.NumCPU")
		}
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "nproc", usage)
}
