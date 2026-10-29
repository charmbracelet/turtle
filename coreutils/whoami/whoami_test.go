package whoami

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
			name:    "prints USER",
			environ: []string{"USER=alice"},
			wantOut: "alice\n",
		},
		{
			name:    "falls back to LOGNAME",
			environ: []string{"LOGNAME=bob"},
			wantOut: "bob\n",
		},
		{
			name:    "prefers USER over LOGNAME",
			environ: []string{"USER=alice", "LOGNAME=bob"},
			wantOut: "alice\n",
		},
		{
			name:    "falls back to LOGNAME when USER is empty",
			environ: []string{"USER=", "LOGNAME=bob"},
			wantOut: "bob\n",
		},
		{
			name:     "fails without USER and LOGNAME",
			wantErr:  "whoami: no user name: neither USER nor LOGNAME is set\n",
			wantCode: 1,
		},
		{
			name:     "rejects an operand",
			environ:  []string{"USER=alice"},
			args:     []string{"alice"},
			wantErr:  `whoami: unexpected operand: "alice"` + "\n",
			wantCode: 1,
		},
		{
			name:     "rejects an option",
			environ:  []string{"USER=alice"},
			args:     []string{"-x"},
			wantErr:  "whoami: invalid option -- 'x'\n",
			wantCode: 1,
		},
		{
			name:     "rejects a long option",
			environ:  []string{"USER=alice"},
			args:     []string{"--help"},
			wantErr:  "whoami: unrecognized option '--help'\n",
			wantCode: 1,
		},
		{
			name:     "reports a write error",
			environ:  []string{"USER=alice"},
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
