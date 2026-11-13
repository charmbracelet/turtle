package false

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"charm.land/turtle/coreutils"
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
		{name: "ignores option-like operands", args: []string{"-x", "-y"}},
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

func TestHelpVersion(t *testing.T) {
	// As the GNU manual puts it, false always exits unsuccessfully, even
	// when invoked with --help or --version.
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "help",
			arg:  "--help",
			want: usage,
		},
		{
			name: "version",
			arg:  "--version",
			want: "false (charm.land/turtle/coreutils) " + coreutils.Version + "\nhttps://charm.land/turtle\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
			err := New(rt).Run(t.Context(), test.arg)
			exitErr, ok := errors.AsType[*coreutils.ExitError](err)
			if !ok {
				t.Fatalf("Run() error = %v, want an *coreutils.ExitError", err)
			}
			if exitErr.Code != 1 {
				t.Errorf("Run() exit code = %d, want 1", exitErr.Code)
			}
			if got := stdout.String(); got != test.want {
				t.Errorf("Run() stdout = %q, want %q", got, test.want)
			}
			if got := stderr.String(); got != "" {
				t.Errorf("Run() stderr = %q, want empty", got)
			}
		})
	}
}
