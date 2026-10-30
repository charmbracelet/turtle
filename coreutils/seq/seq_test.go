package seq

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
)

// runSeq runs seq with the given arguments, returning its standard output and
// error. The standard error must stay empty on success.
func runSeq(t *testing.T, args ...string) (string, error) {
	t.Helper()
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
	err := New(rt).Run(t.Context(), args...)
	if err == nil && stderr.Len() > 0 {
		t.Errorf("Run() stderr = %q, want empty", stderr.String())
	}
	return stdout.String(), err
}

// runSeqErr runs seq with the given arguments, expecting a failure, and
// returns its standard output, standard error and exit status.
func runSeqErr(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
	err := New(rt).Run(t.Context(), args...)
	if err == nil {
		t.Fatalf("Run() error = nil, want a failure")
	}
	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok {
		t.Fatalf("Run() error = %v, want an *coreutils.ExitError", err)
	}
	return stdout.String(), stderr.String(), exitErr.Code
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestRun(t *testing.T) {
	t.Run("one operand", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "last only",
				args: []string{"1"},
				want: "1\n",
			},
			{
				name: "counts up to last",
				args: []string{"5"},
				want: "1\n2\n3\n4\n5\n",
			},
			{
				name: "zero produces no output",
				args: []string{"0"},
				want: "",
			},
			{
				name: "negative last produces no output",
				args: []string{"-1"},
				want: "",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runSeq(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("two operands", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "first and last",
				args: []string{"1", "5"},
				want: "1\n2\n3\n4\n5\n",
			},
			{
				name: "the increment defaults to 1 even when first is larger",
				args: []string{"10", "5"},
				want: "",
			},
			{
				name: "equal first and last",
				args: []string{"5", "5"},
				want: "5\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runSeq(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("three operands", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "increment",
				args: []string{"1", "2", "10"},
				want: "1\n3\n5\n7\n9\n",
			},
			{
				name: "the sequence ends when the sum would pass last",
				args: []string{"1", "10", "10"},
				want: "1\n",
			},
			{
				name: "decreasing sequence",
				args: []string{"10", "-1", "5"},
				want: "10\n9\n8\n7\n6\n5\n",
			},
			{
				name: "decreasing sequence by two",
				args: []string{"10", "-2", "1"},
				want: "10\n8\n6\n4\n2\n",
			},
			{
				name: "negative increment with first below last produces no output",
				args: []string{"5", "-1", "10"},
				want: "",
			},
			{
				name: "last below the range of a negative infinity",
				args: []string{"5", "-1", "inf"},
				want: "",
			},
			{
				name: "negative numbers are operands",
				args: []string{"-8", "-2"},
				want: "-8\n-7\n-6\n-5\n-4\n-3\n-2\n",
			},
			{
				name: "spec example: seq 1048575 1024 1050623",
				args: []string{"1048575", "1024", "1050623"},
				want: "1048575\n1049599\n1050623\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runSeq(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("fractional", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "fractional increment",
				args: []string{"0", "0.5", "2"},
				want: "0.0\n0.5\n1.0\n1.5\n2.0\n",
			},
			{
				name: "fractional first and increment",
				args: []string{"0.5", "0.5", "2"},
				want: "0.5\n1.0\n1.5\n2.0\n",
			},
			{
				name: "fractional last rounds to the precision of the others",
				args: []string{"1", "1", "1.5"},
				want: "1\n",
			},
			{
				name: "fractional first with integer last",
				args: []string{"0.5", "2"},
				want: "0.5\n1.5\n",
			},
			{
				name: "last rounded to the precision of the format is printed",
				args: []string{"0", "0.1", "0.3"},
				want: "0.0\n0.1\n0.2\n0.3\n",
			},
			{
				name: "tiny increments",
				args: []string{"0", "0.000001", "0.000003"},
				want: "0.000000\n0.000001\n0.000002\n0.000003\n",
			},
			{
				name: "hexadecimal floating point uses %g",
				args: []string{"0x.ep-3", "1.0"},
				want: "0.109375\n",
			},
			{
				name: "exponent notation uses %g",
				args: []string{"1e6", "1e6"},
				want: "1e+06\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runSeq(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("equal width", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "zero pads integers",
				args: []string{"-w", "9", "11"},
				want: "09\n10\n11\n",
			},
			{
				name: "zero pads from the first number",
				args: []string{"-w", "5", "10"},
				want: "05\n06\n07\n08\n09\n10\n",
			},
			{
				name: "fractional numbers are padded too",
				args: []string{"-w", "9.5", "1", "12"},
				want: "09.5\n10.5\n11.5\n",
			},
			{
				name: "already equal widths are untouched",
				args: []string{"-w", "0", "0.5", "2"},
				want: "0.0\n0.5\n1.0\n1.5\n2.0\n",
			},
			{
				name: "negative numbers pad after the sign",
				args: []string{"-w", "-2", "2"},
				want: "-2\n-1\n00\n01\n02\n",
			},
			{
				name: "last rounds before its width counts",
				args: []string{"-w", "8", "1", "10.5"},
				want: "08\n09\n10\n",
			},
			{
				name: "long form",
				args: []string{"--equal-width", "95", "10", "115"},
				want: "095\n105\n115\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runSeq(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("separator", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "space",
				args: []string{"-s", " ", "1", "3"},
				want: "1 2 3\n",
			},
			{
				name: "attached",
				args: []string{"-s,", "1", "3"},
				want: "1,2,3\n",
			},
			{
				name: "multiple characters",
				args: []string{"-s", " -- ", "1", "3"},
				want: "1 -- 2 -- 3\n",
			},
			{
				name: "empty",
				args: []string{"-s", "", "1", "3"},
				want: "123\n",
			},
			{
				name: "long form",
				args: []string{"--separator=:", "1", "3"},
				want: "1:2:3\n",
			},
			{
				name: "empty sequence writes nothing",
				args: []string{"-s", ",", "5", "4"},
				want: "",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runSeq(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("format", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "spec example: seq -f '(%9.2E)' -9e5 1.1e6 1.3e6",
				args: []string{"-f", "(%9.2E)", "-9e5", "1.1e6", "1.3e6"},
				want: "(-9.00E+05)\n( 2.00E+05)\n( 1.30E+06)\n",
			},
			{
				name: "%% writes a literal percent",
				args: []string{"-f", "%% %g", "1", "3"},
				want: "% 1\n% 2\n% 3\n",
			},
			{
				name: "%g uses six significant digits like printf",
				args: []string{"-f", "%g", "1234567", "1234567"},
				want: "1.23457e+06\n",
			},
			{
				name: "precision",
				args: []string{"-f", "%.2f", "1", "1.5", "5"},
				want: "1.00\n2.50\n4.00\n",
			},
			{
				name: "flags and width",
				args: []string{"-f", "%+05.1f", "1", "3"},
				want: "+01.0\n+02.0\n+03.0\n",
			},
			{
				name: "default precision of %e",
				args: []string{"-f", "%e", "1"},
				want: "1.000000e+00\n",
			},
			{
				name: "default precision of %f",
				args: []string{"-f", "%f", "1", "2", "3"},
				want: "1.000000\n3.000000\n",
			},
			{
				name: "attached value",
				args: []string{"-f%g", "1", "3"},
				want: "1\n2\n3\n",
			},
			{
				name: "%a prints a single exponent digit",
				args: []string{"-f", "%a", "1"},
				want: "0x1p+0\n",
			},
			{
				name: "%a trims the zero of a two-digit negative exponent",
				args: []string{"-f", "%a", "0.0625", "0.0625"},
				want: "0x1p-4\n",
			},
			{
				name: "%a of zero, a negative exponent and a fraction",
				args: []string{"-f", "%a", "0", "0.5", "1.5"},
				want: "0x0p+0\n0x1p-1\n0x1p+0\n0x1.8p+0\n",
			},
			{
				name: "%A is uppercase",
				args: []string{"-f", "%A", "1.5", "1.5"},
				want: "0X1.8P+0\n",
			},
			{
				name: "%a keeps the literal text around it",
				args: []string{"-f", "v=%a;", "0.5", "0.5"},
				want: "v=0x1p-1;\n",
			},
			{
				name: "long form",
				args: []string{"--format=%g", "1", "3"},
				want: "1\n2\n3\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runSeq(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("options", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "double dash ends the options",
				args: []string{"--", "5"},
				want: "1\n2\n3\n4\n5\n",
			},
			{
				name: "double dash makes a negative operand",
				args: []string{"--", "-5"},
				want: "",
			},
			{
				name: "grouped options",
				args: []string{"-ws,", "1", "3"},
				want: "1,2,3\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runSeq(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}

		t.Run("flags after the first operand are operands", func(t *testing.T) {
			_, stderr, code := runSeqErr(t, "1", "-w", "5")
			if code != 1 {
				t.Errorf("Run() exit status = %d, want 1", code)
			}
			want := "seq: invalid floating point argument: '-w'\nTry 'seq --help' for more information.\n"
			if stderr != want {
				t.Errorf("Run() stderr = %q, want %q", stderr, want)
			}
		})
	})

	t.Run("spec example: seq 1000000", func(t *testing.T) {
		got, err := runSeq(t, "1000000")
		if err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		if len(lines) != 1000000 {
			t.Errorf("Run() wrote %d lines, want 1000000", len(lines))
		}
		if last := lines[len(lines)-1]; last != "1000000" {
			t.Errorf("Run() last line = %q, want %q", last, "1000000")
		}
	})
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing operand",
			args: nil,
			want: "seq: missing operand\nTry 'seq --help' for more information.\n",
		},
		{
			name: "extra operand",
			args: []string{"1", "2", "3", "4"},
			want: "seq: extra operand '4'\nTry 'seq --help' for more information.\n",
		},
		{
			name: "invalid floating point argument",
			args: []string{"x"},
			want: "seq: invalid floating point argument: 'x'\nTry 'seq --help' for more information.\n",
		},
		{
			name: "invalid floating point increment",
			args: []string{"1", "y", "5"},
			want: "seq: invalid floating point argument: 'y'\nTry 'seq --help' for more information.\n",
		},
		{
			name: "NaN last",
			args: []string{"nan"},
			want: "seq: invalid 'not-a-number' argument: 'nan'\nTry 'seq --help' for more information.\n",
		},
		{
			name: "NaN increment",
			args: []string{"1", "NaN", "5"},
			want: "seq: invalid 'not-a-number' argument: 'NaN'\nTry 'seq --help' for more information.\n",
		},
		{
			name: "zero increment",
			args: []string{"1", "0", "5"},
			want: "seq: invalid Zero increment value: '0'\nTry 'seq --help' for more information.\n",
		},
		{
			name: "invalid option",
			args: []string{"-x", "5"},
			want: "seq: invalid option -- 'x'\nTry 'seq --help' for more information.\n",
		},
		{
			name: "unrecognized long option",
			args: []string{"--bogus"},
			want: "seq: unrecognized option '--bogus'\nTry 'seq --help' for more information.\n",
		},
		{
			name: "-f requires an argument",
			args: []string{"-f"},
			want: "seq: option requires an argument -- 'f'\nTry 'seq --help' for more information.\n",
		},
		{
			name: "-s requires an argument",
			args: []string{"-s"},
			want: "seq: option requires an argument -- 's'\nTry 'seq --help' for more information.\n",
		},
		{
			name: "--format requires an argument",
			args: []string{"--format"},
			want: "seq: option '--format' requires an argument\nTry 'seq --help' for more information.\n",
		},
		{
			name: "-f with -w",
			args: []string{"-f", "%g", "-w", "1", "3"},
			want: "seq: format string may not be specified when printing equal width strings\nTry 'seq --help' for more information.\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, code := runSeqErr(t, test.args...)
			if stdout != "" {
				t.Errorf("Run() stdout = %q, want empty", stdout)
			}
			if code != 1 {
				t.Errorf("Run() exit status = %d, want 1", code)
			}
			if stderr != test.want {
				t.Errorf("Run() stderr = %q, want %q", stderr, test.want)
			}
		})
	}
}

func TestFormatErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no directive",
			args: []string{"-f", "abc", "1"},
			want: "seq: format 'abc' has no % directive\n",
		},
		{
			name: "only a literal percent",
			args: []string{"-f", "%%", "1"},
			want: "seq: format '%%' has no % directive\n",
		},
		{
			name: "empty format",
			args: []string{"-f", "", "1"},
			want: "seq: format '' has no % directive\n",
		},
		{
			name: "too many directives",
			args: []string{"-f", "%f%f", "1"},
			want: "seq: format '%f%f' has too many % directives\n",
		},
		{
			name: "unknown directive",
			args: []string{"-f", "%d", "1"},
			want: "seq: format '%d' has unknown %d directive\n",
		},
		{
			name: "unknown string directive",
			args: []string{"-f", "%s", "1"},
			want: "seq: format '%s' has unknown %s directive\n",
		},
		{
			name: "ends in percent",
			args: []string{"-f", "x%", "1"},
			want: "seq: format 'x%' ends in %\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, code := runSeqErr(t, test.args...)
			if stdout != "" {
				t.Errorf("Run() stdout = %q, want empty", stdout)
			}
			if code != 1 {
				t.Errorf("Run() exit status = %d, want 1", code)
			}
			if stderr != test.want {
				t.Errorf("Run() stderr = %q, want %q", stderr, test.want)
			}
		})
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout, err := runSeq(t, "--help")
		if err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if stdout != usage {
			t.Errorf("Run() stdout = %q, want the usage", stdout)
		}
	})

	t.Run("help ignores the operands", func(t *testing.T) {
		stdout, err := runSeq(t, "--help", "x")
		if err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if stdout != usage {
			t.Errorf("Run() stdout = %q, want the usage", stdout)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout, err := runSeq(t, "--version")
		if err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "seq (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if stdout != want {
			t.Errorf("Run() stdout = %q, want %q", stdout, want)
		}
	})
}

func TestInterrupted(t *testing.T) {
	t.Run("done context stops an infinite sequence", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		rt := &coreutils.Runtime{Stdout: new(bytes.Buffer), Stderr: new(bytes.Buffer)}
		err := New(rt).Run(ctx, "1", "inf")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want %v", err, context.Canceled)
		}
	})

	t.Run("write error stops an infinite sequence", func(t *testing.T) {
		boom := errors.New("boom")
		rt := &coreutils.Runtime{Stdout: errorWriter{err: boom}, Stderr: new(bytes.Buffer)}
		err := New(rt).Run(t.Context(), "1", "inf")
		exitErr, ok := errors.AsType[*coreutils.ExitError](err)
		if !ok {
			t.Fatalf("Run() error = %v, want an *coreutils.ExitError", err)
		}
		if exitErr.Code != 1 {
			t.Errorf("Run() exit status = %d, want 1", exitErr.Code)
		}
		if !errors.Is(exitErr.Err, boom) {
			t.Errorf("Run() error = %v, want it to wrap %v", exitErr.Err, boom)
		}
	})

	t.Run("write error on the usage output", func(t *testing.T) {
		boom := errors.New("boom")
		rt := &coreutils.Runtime{Stdout: errorWriter{err: boom}, Stderr: new(bytes.Buffer)}
		err := New(rt).Run(t.Context(), "--help")
		exitErr, ok := errors.AsType[*coreutils.ExitError](err)
		if !ok {
			t.Fatalf("Run() error = %v, want an *coreutils.ExitError", err)
		}
		if exitErr.Code != 1 {
			t.Errorf("Run() exit status = %d, want 1", exitErr.Code)
		}
	})
}
