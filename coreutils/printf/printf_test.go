package printf

import (
	"bytes"
	"errors"
	"io"
	"strings"
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

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func runPrintf(t *testing.T, args ...string) (string, string, error) {
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
	return stdout.String(), stderr.String(), err
}

func TestRun(t *testing.T) {
	t.Run("output", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "format only",
				args: []string{"hello\n"},
				want: "hello\n",
			},
			{
				name: "empty format",
				args: []string{""},
				want: "",
			},
			{
				name: "literal percent",
				args: []string{"%%"},
				want: "%",
			},
			{
				name: "double dash is the format operand",
				args: []string{"--", "%d\n", "5"},
				want: "--",
			},
			{
				name: "double dash followed by an operand",
				args: []string{"--", "hello"},
				want: "--",
			},
			{
				name: "double dash alone",
				args: []string{"--"},
				want: "--",
			},
			{
				name: "no conversions ignores arguments",
				args: []string{"hi\n", "a"},
				want: "hi\n",
			},
			{
				name: "format is reused",
				args: []string{"%d.", "1", "2"},
				want: "1.2.",
			},
			{
				// Example from the POSIX specification.
				name: "format reuse satisfies a missing specifier with zero",
				args: []string{"%5d%4d\n", "1", "21", "321", "4321", "54321"},
				want: "    1  21\n  3214321\n54321   0\n",
			},
			{
				// Example from the POSIX specification.
				name: "C integer constants",
				args: []string{"%d\n", "10", "010", "0x10"},
				want: "10\n8\n16\n",
			},
			{
				name: "signed decimal constants",
				args: []string{"%d %d %d", "+3", "-3", "3"},
				want: "3 -3 3",
			},
			{
				name: "quoted characters",
				args: []string{"%d %d", "'3", "'A"},
				want: "51 65",
			},
			{
				name: "percent character",
				args: []string{"(%s%%)", "done"},
				want: "(done%)",
			},
			{
				name: "string conversion",
				args: []string{"[%s]", "foo"},
				want: "[foo]",
			},
			{
				name: "string conversion of empty argument",
				args: []string{"[%s]", ""},
				want: "[]",
			},
			{
				name: "character conversion writes the first byte",
				args: []string{"%c", "abc"},
				want: "a",
			},
			{
				name: "character conversion of empty argument",
				args: []string{"[%c]", ""},
				want: "[]",
			},
			{
				name: "octal conversion",
				args: []string{"%o", "010"},
				want: "10",
			},
			{
				name: "octal conversion has no leading zeros",
				args: []string{"%o", "8"},
				want: "10",
			},
			{
				name: "unsigned conversion",
				args: []string{"%u", "5"},
				want: "5",
			},
			{
				name: "unsigned conversion wraps negative values",
				args: []string{"%u", "-3"},
				want: "18446744073709551613",
			},
			{
				name: "hexadecimal conversion",
				args: []string{"%x %X", "255", "255"},
				want: "ff FF",
			},
			{
				name: "float conversion",
				args: []string{"%f", "10.1e2"},
				want: "1010.000000",
			},
			{
				name: "hexadecimal float without exponent",
				args: []string{"%.2f", "0x10"},
				want: "16.00",
			},
			{
				// Example from the POSIX specification.
				name: "hexadecimal float with exponent",
				args: []string{"%.2f", "0x10.1p2"},
				want: "64.25",
			},
			{
				name: "exponent conversions",
				args: []string{"%e %E", "1", "1"},
				want: "1.000000e+00 1.000000E+00",
			},
			{
				name: "general conversions",
				args: []string{"%g %G", "100000", "1000000"},
				want: "100000 1E+06",
			},
			{
				name: "general conversions use six significant digits by default",
				args: []string{"%g %G", "1.23456789", "1.23456789"},
				want: "1.23457 1.23457",
			},
			{
				name: "b conversion expands escape sequences",
				args: []string{"%b", `a\tb\n`},
				want: "a\tb\n",
			},
			{
				name: "b conversion octal escape",
				args: []string{"%b", `\0101`},
				want: "A",
			},
			{
				name: "b conversion octal escape without leading zero",
				args: []string{"%b", `\101`},
				want: `\101`,
			},
			{
				name: "b conversion keeps unrecognized escapes",
				args: []string{"%b", `a\zb`},
				want: `a\zb`,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, stderr, err := runPrintf(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if stderr != "" {
					t.Errorf("Run() stderr = %q, want empty", stderr)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("flags, width and precision", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "left justification",
				args: []string{"%-8s|", "foo"},
				want: "foo     |",
			},
			{
				name: "right justification",
				args: []string{"%8s|", "foo"},
				want: "     foo|",
			},
			{
				name: "string precision truncates",
				args: []string{"%.2s|", "foo"},
				want: "fo|",
			},
			{
				name: "zero string precision",
				args: []string{"%.s|", "foo"},
				want: "|",
			},
			{
				name: "zero padding and precision",
				args: []string{"%08.3f", "3.14159"},
				want: "0003.142",
			},
			{
				name: "plus flag",
				args: []string{"%+d", "5"},
				want: "+5",
			},
			{
				name: "space flag",
				args: []string{"% d", "5"},
				want: " 5",
			},
			{
				name: "zero padding",
				args: []string{"%05d", "42"},
				want: "00042",
			},
			{
				name: "precision is a minimum digit count",
				args: []string{"%5.3d|", "42"},
				want: "  042|",
			},
			{
				name: "alternate form octal",
				args: []string{"%#o", "8"},
				want: "010",
			},
			{
				name: "alternate form hexadecimal",
				args: []string{"%#x %#X", "255", "255"},
				want: "0xff 0XFF",
			},
			{
				name: "alternate form hexadecimal of zero has no prefix",
				args: []string{"%#x %#X", "0", "0"},
				want: "0 0",
			},
			{
				name: "alternate form hexadecimal of zero with zero precision",
				args: []string{"%#.0x|%#.0X|", "0", "0"},
				want: "||",
			},
			{
				name: "width and precision",
				args: []string{"%6.2f|", "3.14159"},
				want: "  3.14|",
			},
			{
				name: "character conversion width",
				args: []string{"%3c|", "ab"},
				want: "  a|",
			},
			{
				name: "character conversion left justification",
				args: []string{"%-3c|", "ab"},
				want: "a  |",
			},
			{
				name: "float precision",
				args: []string{"%.2f", "10"},
				want: "10.00",
			},
			{
				name: "width above one mebibyte",
				args: []string{"%1048600d", "1"},
				want: strings.Repeat(" ", 1048600-1) + "1",
			},
			{
				name: "b conversion width",
				args: []string{"%6b|", `a\tb`},
				want: "   a\tb|",
			},
			{
				name: "b conversion precision",
				args: []string{"%.2b|", "abc"},
				want: "ab|",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, stderr, err := runPrintf(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if stderr != "" {
					t.Errorf("Run() stderr = %q, want empty", stderr)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("format escapes", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "tab and newline",
				args: []string{`a\tb\n`},
				want: "a\tb\n",
			},
			{
				name: "alert, backspace, form feed, vertical tab and carriage return",
				args: []string{"\a\b\f\v\r"},
				want: "\a\b\f\v\r",
			},
			{
				name: "backslash",
				args: []string{`\\`},
				want: `\`,
			},
			{
				name: "octal escape",
				args: []string{`\101`},
				want: "A",
			},
			{
				name: "octal escape consumes at most three digits",
				args: []string{`\0102`},
				want: "\b2",
			},
			{
				name: "null octal escape",
				args: []string{`\0`},
				want: "\x00",
			},
			{
				name: "octal escape wraps past a byte",
				args: []string{`\777`},
				want: "\xff",
			},
			{
				name: "unrecognized escape is kept",
				args: []string{`a\zb`},
				want: `a\zb`,
			},
			{
				name: "lone trailing backslash is kept",
				args: []string{`a\`},
				want: `a\`,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, stderr, err := runPrintf(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if stderr != "" {
					t.Errorf("Run() stderr = %q, want empty", stderr)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("backslash c stops output", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "in the format operand",
				args: []string{`a\cb`},
				want: "a",
			},
			{
				name: "ignores the rest of the format and of the arguments",
				args: []string{"%s\\c%s\n", "foo", "bar"},
				want: "foo",
			},
			{
				name: "in a b conversion argument",
				args: []string{"%b|%s\n", `foo\cbar`, "baz"},
				want: "foo",
			},
			{
				name: "before any output",
				args: []string{`\c`, "x"},
				want: "",
			},
			{
				name: "after the b conversion precision is reached",
				args: []string{"%.2b\n", `ab\cd`},
				want: "ab\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, stderr, err := runPrintf(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if stderr != "" {
					t.Errorf("Run() stderr = %q, want empty", stderr)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("missing arguments", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "string conversions take a null string",
				args: []string{"(%s)"},
				want: "()",
			},
			{
				name: "numeric conversions take zero",
				args: []string{"%s:%d\n"},
				want: ":0\n",
			},
			{
				name: "only some specifiers are satisfied",
				args: []string{"%d%d\n", "5"},
				want: "50\n",
			},
			{
				name: "no arguments at all",
				args: []string{"%s\n"},
				want: "\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, stderr, err := runPrintf(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if stderr != "" {
					t.Errorf("Run() stderr = %q, want empty", stderr)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("numbered arguments", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "arguments in any order",
				args: []string{"%2$s %1$s\n", "a", "b"},
				want: "b a\n",
			},
			{
				name: "gaps are allowed",
				args: []string{"%3$s %1$s\n", "a", "b", "c"},
				want: "c a\n",
			},
			{
				name: "format reuse follows the highest numbered argument",
				args: []string{"%1$s:%1$s\n", "a", "b", "c"},
				want: "a:a\nb:b\nc:c\n",
			},
			{
				name: "percent conversion can be mixed",
				args: []string{"%1$s%%\n", "a", "b"},
				want: "a%\nb%\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, stderr, err := runPrintf(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if stderr != "" {
					t.Errorf("Run() stderr = %q, want empty", stderr)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("diagnostics", func(t *testing.T) {
		tests := []struct {
			name       string
			args       []string
			want       string
			wantStderr string
		}{
			{
				name:       "no format operand",
				args:       nil,
				wantStderr: "printf: missing format operand\n",
			},
			{
				// Example from the POSIX specification.
				name:       "argument not completely converted",
				args:       []string{"%d\n", "5a"},
				want:       "5\n",
				wantStderr: `printf: "5a" not completely converted` + "\n",
			},
			{
				// Example from the POSIX specification.
				name:       "argument without any numeric value",
				args:       []string{"%d\n", "ABC"},
				want:       "0\n",
				wantStderr: `printf: "ABC" expected numeric value` + "\n",
			},
			{
				// Example from the POSIX specification, on 64-bit integers.
				name:       "positive arithmetic overflow",
				args:       []string{"%d\n", "99999999999999999999"},
				want:       "9223372036854775807\n",
				wantStderr: `printf: "99999999999999999999" arithmetic overflow` + "\n",
			},
			{
				name:       "negative arithmetic overflow",
				args:       []string{"%d\n", "-99999999999999999999"},
				want:       "-9223372036854775808\n",
				wantStderr: `printf: "-99999999999999999999" arithmetic overflow` + "\n",
			},
			{
				name:       "unsigned arithmetic overflow",
				args:       []string{"%u\n", "99999999999999999999"},
				want:       "18446744073709551615\n",
				wantStderr: `printf: "99999999999999999999" arithmetic overflow` + "\n",
			},
			{
				// Example from the POSIX specification: processing continues.
				name: "remaining operands keep being processed",
				args: []string{"(%d)(%d)\n", "5a", "6a"},
				want: "(5)(6)\n",
				wantStderr: `printf: "5a" not completely converted` + "\n" +
					`printf: "6a" not completely converted` + "\n",
			},
			{
				name:       "quoted character with extra characters",
				args:       []string{"%d", "'-3"},
				want:       "45",
				wantStderr: `printf: "'-3" not completely converted` + "\n",
			},
			{
				name:       "float argument not completely converted",
				args:       []string{"%.2f\n", "10.1e2x"},
				want:       "1010.00\n",
				wantStderr: `printf: "10.1e2x" not completely converted` + "\n",
			},
			{
				name:       "float argument without any numeric value",
				args:       []string{"%f\n", "abc"},
				want:       "0.000000\n",
				wantStderr: `printf: "abc" expected numeric value` + "\n",
			},
			{
				name:       "float arithmetic overflow",
				args:       []string{"%f\n", "1e999"},
				want:       "+Inf\n",
				wantStderr: `printf: "1e999" arithmetic overflow` + "\n",
			},
			{
				name:       "invalid directive",
				args:       []string{"a%zb"},
				want:       "a",
				wantStderr: `printf: "%z" invalid directive` + "\n",
			},
			{
				name:       "incomplete directive",
				args:       []string{"100%"},
				want:       "100",
				wantStderr: `printf: "%" invalid directive` + "\n",
			},
			{
				name:       "missing numbered argument",
				args:       []string{"%2$s\n", "a"},
				want:       "\n",
				wantStderr: `printf: "2" missing argument` + "\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, stderr, err := runPrintf(t, test.args...)
				if err == nil {
					t.Fatalf("Run() error = nil, want non-zero exit status")
				}
				exitErr, ok := errors.AsType[*coreutils.ExitError](err)
				if !ok {
					t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
				}
				if exitErr.Code == 0 {
					t.Errorf("Run() exit status = 0, want non-zero")
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
				if stderr != test.wantStderr {
					t.Errorf("Run() stderr = %q, want %q", stderr, test.wantStderr)
				}
			})
		}
	})

	t.Run("write error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdout: errorWriter{err: errors.New("boom")},
			Stderr: new(bytes.Buffer),
		}
		err := New(rt).Run(t.Context(), "hi\n")
		if err == nil {
			t.Fatalf("Run() error = nil, want *coreutils.ExitError")
		}
		exitErr, ok := errors.AsType[*coreutils.ExitError](err)
		if !ok || exitErr.Code != 1 {
			t.Errorf("Run() error = %v, want exit status 1", err)
		}
		if !errors.Is(err, rt.Stdout.(errorWriter).err) {
			t.Errorf("Run() error = %v, want it to wrap the write error", err)
		}
	})
}
