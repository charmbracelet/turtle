package logname

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"charm.land/snail/coreutils"
)

// errWriter fails every write with its error.
type errWriter struct {
	err error
}

func (w errWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestRun(t *testing.T) {
	writeErr := errors.New("write error")

	tests := []struct {
		name     string
		environ  []string
		args     []string
		canceled bool
		writeErr error
		wantOut  string
		wantErr  string
		wantCode int
	}{
		{
			name:    "prints LOGNAME",
			environ: []string{"LOGNAME=alice"},
			wantOut: "alice\n",
		},
		{
			name:     "treats empty LOGNAME as unset",
			environ:  []string{"LOGNAME="},
			wantErr:  "logname: no login name\n",
			wantCode: 1,
		},
		{
			name:     "fails without LOGNAME",
			wantErr:  "logname: no login name\n",
			wantCode: 1,
		},
		{
			name:     "rejects an operand",
			environ:  []string{"LOGNAME=alice"},
			args:     []string{"alice"},
			wantErr:  `logname: unexpected operand: "alice"` + "\nTry 'logname --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "rejects an option",
			environ:  []string{"LOGNAME=alice"},
			args:     []string{"-x"},
			wantErr:  "logname: invalid option -- 'x'\nTry 'logname --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "rejects a long option",
			environ:  []string{"LOGNAME=alice"},
			args:     []string{"--bogus"},
			wantErr:  "logname: unrecognized option '--bogus'\nTry 'logname --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "reports a write error",
			environ:  []string{"LOGNAME=alice"},
			writeErr: writeErr,
			wantCode: 1,
		},
		{
			name:     "stops on a canceled context",
			canceled: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			if test.canceled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			stdout := new(bytes.Buffer)
			var out io.Writer = stdout
			if test.writeErr != nil {
				out = errWriter{err: test.writeErr}
			}
			stderr := new(bytes.Buffer)

			rt := &coreutils.Runtime{
				Stdout:  out,
				Stderr:  stderr,
				Environ: func() []string { return test.environ },
			}
			err := New(rt).Run(ctx, test.args...)

			if test.canceled {
				if !errors.Is(err, context.Canceled) {
					t.Errorf("Run() error = %v, want context.Canceled", err)
				}
				if got := stdout.String(); got != "" {
					t.Errorf("Run() stdout = %q, want empty", got)
				}
				return
			}

			if test.wantCode == 0 {
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
			} else {
				var exitErr *coreutils.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("Run() error = %v, want an ExitError", err)
				}
				if exitErr.Code != test.wantCode {
					t.Errorf("Run() exit code = %d, want %d", exitErr.Code, test.wantCode)
				}
			}
			if test.writeErr != nil {
				if !errors.Is(err, test.writeErr) {
					t.Errorf("Run() error = %v, want it to wrap the write error", err)
				}
				return
			}
			if got := stdout.String(); got != test.wantOut {
				t.Errorf("Run() stdout = %q, want %q", got, test.wantOut)
			}
			if got := stderr.String(); got != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
			}
		})
	}
}

func TestHelpVersion(t *testing.T) {
	for _, arg := range []string{"--help", "--version"} {
		t.Run(arg, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			rt := &coreutils.Runtime{
				Stdout: stdout,
				Stderr: stderr,
				Environ: func() []string {
					return []string{"LOGNAME=alice"}
				},
			}
			if err := New(rt).Run(t.Context(), arg); err != nil {
				t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
			}
			if got := stderr.String(); got != "" {
				t.Errorf("Run() stderr = %q, want empty", got)
			}
			want := map[string]string{
				"--help":    usage,
				"--version": "logname (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n",
			}[arg]
			if got := stdout.String(); got != want {
				t.Errorf("Run() stdout = %q, want %q", got, want)
			}
		})
	}
}
