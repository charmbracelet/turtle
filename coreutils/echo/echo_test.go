package echo

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
)

type spyReader struct {
	read bool
}

func (r *spyReader) Read([]byte) (int, error) {
	r.read = true
	return 0, io.EOF
}

func runEcho(t *testing.T, args ...string) (string, error) {
	t.Helper()

	stdin := &spyReader{}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context(), args...)

	if stdin.read {
		t.Error("Run() used stdin, want it untouched")
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
	return stdout.String(), err
}

func TestRun(t *testing.T) {
	t.Run("operands", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "no arguments writes only a newline",
				want: "\n",
			},
			{
				name: "single operand",
				args: []string{"foo"},
				want: "foo\n",
			},
			{
				name: "operands are separated by single spaces",
				args: []string{"foo", "bar", "baz"},
				want: "foo bar baz\n",
			},
			{
				name: "empty operands",
				args: []string{"", ""},
				want: " \n",
			},
			{
				name: "double dash is a string operand",
				args: []string{"--", "foo"},
				want: "-- foo\n",
			},
			{
				name: "lone dash is a string operand",
				args: []string{"-"},
				want: "-\n",
			},
			{
				name: "unknown flags are string operands",
				args: []string{"-x", "--help"},
				want: "-x --help\n",
			},
			{
				name: "flags after the first operand are string operands",
				args: []string{"foo", "-n"},
				want: "foo -n\n",
			},
			{
				name: "backslashes are literal by default",
				args: []string{`a\tb`},
				want: `a\tb` + "\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runEcho(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("flags", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "n suppresses the trailing newline",
				args: []string{"-n", "foo"},
				want: "foo",
			},
			{
				name: "n alone outputs nothing",
				args: []string{"-n"},
				want: "",
			},
			{
				name: "e enables escape interpretation",
				args: []string{"-e", `a\tb`},
				want: "a\tb\n",
			},
			{
				name: "E keeps escapes literal",
				args: []string{"-E", `a\tb`},
				want: `a\tb` + "\n",
			},
			{
				name: "flags can be grouped",
				args: []string{"-ne", `a\tb`},
				want: "a\tb",
			},
			{
				name: "flags can be repeated",
				args: []string{"-n", "-e", "foo"},
				want: "foo",
			},
			{
				name: "grouped flag with unknown letter is an operand",
				args: []string{"-nx"},
				want: "-nx\n",
			},
			{
				name: "e then E leaves escapes off",
				args: []string{"-e", "-E", `a\tb`},
				want: `a\tb` + "\n",
			},
			{
				name: "E then e leaves escapes on",
				args: []string{"-E", "-e", `a\tb`},
				want: "a\tb\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runEcho(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("escapes", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "alert",
				args: []string{"-e", `\a`},
				want: "\a\n",
			},
			{
				name: "backspace",
				args: []string{"-e", `\b`},
				want: "\b\n",
			},
			{
				name: "form feed",
				args: []string{"-e", `\f`},
				want: "\f\n",
			},
			{
				name: "newline",
				args: []string{"-e", `\n`},
				want: "\n\n",
			},
			{
				name: "carriage return",
				args: []string{"-e", `\r`},
				want: "\r\n",
			},
			{
				name: "tab",
				args: []string{"-e", `\t`},
				want: "\t\n",
			},
			{
				name: "vertical tab",
				args: []string{"-e", `\v`},
				want: "\v\n",
			},
			{
				name: "escape",
				args: []string{"-e", `\e`},
				want: "\x1b\n",
			},
			{
				name: "backslash",
				args: []string{"-e", `\\`},
				want: "\\\n",
			},
			{
				name: "escaped backslash does not escape the next letter",
				args: []string{"-e", `\\n`},
				want: `\n` + "\n",
			},
			{
				name: "c suppresses the newline and the rest of the operand",
				args: []string{"-e", `foo\cbar`},
				want: "foo",
			},
			{
				name: "c ignores all following operands",
				args: []string{"-e", "foo", `a\cb`, "bar"},
				want: "foo a",
			},
			{
				name: "octal number",
				args: []string{"-e", `\0101`},
				want: "A\n",
			},
			{
				name: "octal number with one digit",
				args: []string{"-e", `\07`},
				want: "\a\n",
			},
			{
				name: "octal number with no digits is a null byte",
				args: []string{"-e", `a\0b`},
				want: "a\x00b\n",
			},
			{
				name: "octal number with too many digits wraps around",
				args: []string{"-e", `\0777`},
				want: "\xff\n",
			},
			{
				name: "octal number followed by a non-octal digit",
				args: []string{"-e", `\08`},
				want: "\x008\n",
			},
			{
				name: "octal number without the leading zero",
				args: []string{"-e", `\101`},
				want: "A\n",
			},
			{
				name: "octal number without the leading zero with one digit",
				args: []string{"-e", `\7`},
				want: "\a\n",
			},
			{
				name: "octal number without the leading zero with two digits",
				args: []string{"-e", `\12`},
				want: "\n\n",
			},
			{
				name: "octal number without the leading zero with too many digits wraps around",
				args: []string{"-e", `\777`},
				want: "\xff\n",
			},
			{
				name: "octal number without the leading zero followed by a non-octal digit",
				args: []string{"-e", `\18`},
				want: "\x018\n",
			},
			{
				name: "non-octal digit after a backslash is kept literally",
				args: []string{"-e", `\8`},
				want: `\8` + "\n",
			},
			{
				name: "hexadecimal number",
				args: []string{"-e", `\x41`},
				want: "A\n",
			},
			{
				name: "hexadecimal number with one digit",
				args: []string{"-e", `\x9`},
				want: "\t\n",
			},
			{
				name: "hexadecimal number with uppercase digits",
				args: []string{"-e", `\x1B`},
				want: "\x1b\n",
			},
			{
				name: "hexadecimal number with more than two digits consumes only two",
				args: []string{"-e", `\x123`},
				want: "\x123\n",
			},
			{
				name: "hexadecimal number with no digits is kept literally",
				args: []string{"-e", `a\xzb`},
				want: `a\xzb` + "\n",
			},
			{
				name: "trailing x escape is kept literally",
				args: []string{"-e", `foo\x`},
				want: `foo\x` + "\n",
			},
			{
				name: "unrecognized escape is kept literally",
				args: []string{"-e", `\q`},
				want: `\q` + "\n",
			},
			{
				name: "trailing backslash is kept literally",
				args: []string{"-e", `foo\`},
				want: `foo\` + "\n",
			},
			{
				name: "multi-byte characters pass through",
				args: []string{"-e", "ão\tç"},
				want: "ão\tç\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runEcho(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("write error", func(t *testing.T) {
		writeErr := errors.New("disk full")

		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  &spyReader{},
			Stdout: testutil.ErrorWriter{Err: writeErr},
			Stderr: stderr,
		}

		err := New(rt).Run(t.Context(), "foo")

		var exitErr *coreutils.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
		}
		if exitErr.Code != 1 {
			t.Errorf("Run() exit code = %d, want 1", exitErr.Code)
		}
		if !errors.Is(err, writeErr) {
			t.Errorf("Run() error = %v, want it to wrap %v", err, writeErr)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
	})
}
