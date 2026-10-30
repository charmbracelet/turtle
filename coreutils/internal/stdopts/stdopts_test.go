package stdopts

import (
	"errors"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
)

const usage = "Usage: prog [OPTION]... [FILE]...\nDo something.\n"

// set is the option set of a fictional prog, with a valued "o" option.
var set = getopt.Set{Valued: "o", Long: map[string]rune{
	"output": 'o',
	"quiet":  'q',
}}

func TestCheck(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		handled  bool
		contains string
	}{
		{
			name:     "help",
			args:     []string{"--help"},
			handled:  true,
			contains: usage,
		},
		{
			name:     "version",
			args:     []string{"--version"},
			handled:  true,
			contains: "(charm.land/snail/coreutils)",
		},
		{
			name:     "after other options",
			args:     []string{"-q", "--help"},
			handled:  true,
			contains: usage,
		},
		{
			name:    "valued option takes the help as its argument",
			args:    []string{"-o", "--help"},
			handled: false,
		},
		{
			name:    "valued long option takes the help as its value",
			args:    []string{"--output=--help"},
			handled: false,
		},
		{
			name:    "operand stops the options",
			args:    []string{"file", "--help"},
			handled: false,
		},
		{
			name:    "dash dash ends the options",
			args:    []string{"--", "--help"},
			handled: false,
		},
		{
			name:    "invalid option before the help stops the scan",
			args:    []string{"--bogus", "--help"},
			handled: false,
		},
		{
			name:    "missing option argument stops the scan",
			args:    []string{"-o"},
			handled: false,
		},
		{
			name:     "first one wins",
			args:     []string{"--version", "--help"},
			handled:  true,
			contains: "(charm.land/snail/coreutils)",
		},
		{
			name:     "empty arguments",
			args:     nil,
			handled:  false,
			contains: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout := new(strings.Builder)
			rt := &coreutils.Runtime{Stdout: stdout}
			if got, err := Check(rt, "prog", usage, set, test.args); got != test.handled || (got && err != nil) {
				t.Errorf("Check() = %v, %v, want %v, nil", got, err, test.handled)
			}
			if !test.handled {
				return
			}
			if got := stdout.String(); !strings.Contains(got, test.contains) {
				t.Errorf("stdout = %q, want it to contain %q", got, test.contains)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	stdout := new(strings.Builder)
	rt := &coreutils.Runtime{Stdout: stdout}
	if done, err := Check(rt, "prog", usage, getopt.Set{}, []string{"--version"}); !done || err != nil {
		t.Fatalf("Check() = %v, %v, want true, nil", done, err)
	}
	want := "prog (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestWriteError(t *testing.T) {
	boom := errors.New("boom")
	rt := &coreutils.Runtime{Stdout: errWriter{boom}}
	done, err := Check(rt, "prog", usage, getopt.Set{}, []string{"--help"})
	if !done {
		t.Fatal("Check() = false, want true")
	}
	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok {
		t.Fatalf("Check() error = %v, want an *coreutils.ExitError", err)
	}
	if exitErr.Code != 1 {
		t.Errorf("Check() exit status = %d, want 1", exitErr.Code)
	}
	if !errors.Is(exitErr.Err, boom) {
		t.Errorf("Check() error = %v, want it to wrap %v", exitErr.Err, boom)
	}
}

type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

func TestTryHelp(t *testing.T) {
	stderr := new(strings.Builder)
	rt := &coreutils.Runtime{Stderr: stderr}
	TryHelp(rt, "prog")
	want := "Try 'prog --help' for more information.\n"
	if got := stderr.String(); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}
