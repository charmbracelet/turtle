package false

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"charm.land/snail/coreutils"
)

type spyReader struct {
	read bool
}

func (r *spyReader) Read([]byte) (int, error) {
	r.read = true
	return 0, io.EOF
}

func TestRun(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no arguments"},
		{name: "discards a lone double dash", args: []string{"--"}},
		{name: "ignores operands", args: []string{"foo", "bar"}},
		{name: "ignores option-like operands", args: []string{"--help", "--version"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdin := &spyReader{}
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)

			rt := &coreutils.Runtime{
				Stdin:  stdin,
				Stdout: stdout,
				Stderr: stderr,
			}
			prog := New(rt)

			err := prog.Run(t.Context(), test.args...)

			var exitErr *coreutils.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
			}
			if exitErr.Code < 1 || exitErr.Code > 125 {
				t.Errorf("Run() exit code = %d, want between 1 and 125", exitErr.Code)
			}
			if stdin.read {
				t.Error("Run() used stdin, want it untouched")
			}
			if got := stdout.String(); got != "" {
				t.Errorf("Run() stdout = %q, want empty", got)
			}
			if got := stderr.String(); got != "" {
				t.Errorf("Run() stderr = %q, want empty", got)
			}
		})
	}
}
