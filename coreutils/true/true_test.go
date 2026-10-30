package true

import (
	"bytes"
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
			if err != nil {
				t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
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
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "true (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
