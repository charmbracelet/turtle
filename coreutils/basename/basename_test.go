package basename

import (
	"bytes"
	"errors"
	"io"
	"runtime"
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

func runBasename(t *testing.T, args ...string) (string, error) {
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

func exitCode(t *testing.T, err error) int {
	t.Helper()

	if err == nil {
		return 0
	}
	var exitErr *coreutils.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
	}
	return exitErr.Code
}

func TestRun(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "a simple name is returned unchanged",
				args: []string{"usr"},
				want: "usr",
			},
			{
				name: "a trailing slash is removed",
				args: []string{"usr/"},
				want: "usr",
			},
			{
				name: "a null string gives an empty output",
				args: []string{""},
				want: "",
			},
			{
				name: "a single slash is kept",
				args: []string{"/"},
				want: "/",
			},
			{
				name: "a double slash becomes a single slash",
				args: []string{"//"},
				want: "/",
			},
			{
				name: "multiple slashes become a single slash",
				args: []string{"///"},
				want: "/",
			},
			{
				name: "a trailing slash after a directory",
				args: []string{"/usr/"},
				want: "usr",
			},
			{
				name: "the last component of a pathname",
				args: []string{"/usr/lib"},
				want: "lib",
			},
			{
				name: "repeated slashes are collapsed",
				args: []string{"//usr//lib//"},
				want: "lib",
			},
			{
				name: "internal repeated slashes",
				args: []string{"/home//dwc//test"},
				want: "test",
			},
			{
				name: "dot and dot-dot components are not special",
				args: []string{"/home/.././test"},
				want: "test",
			},
			{
				name: "a dot component is kept",
				args: []string{"/home/dwc/."},
				want: ".",
			},
			{
				name: "a trailing double slash is removed",
				args: []string{"a//b//"},
				want: "b",
			},
			{
				name: "a relative pathname",
				args: []string{"relative/path"},
				want: "path",
			},
			{
				name: "a lone dash is an operand",
				args: []string{"-"},
				want: "-",
			},
			{
				name: "multi-byte characters",
				args: []string{"dir/ção"},
				want: "ção",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runBasename(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if want := test.want + "\n"; got != want {
					t.Errorf("Run() stdout = %q, want %q", got, want)
				}
			})
		}
	})

	t.Run("suffix", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "a matching suffix is removed",
				args: []string{"/usr/src/cmd/cat.c", ".c"},
				want: "cat",
			},
			{
				name: "only the last occurrence is removed",
				args: []string{"file.c.c", ".c"},
				want: "file.c",
			},
			{
				name: "a suffix identical to the result is kept",
				args: []string{"file", "file"},
				want: "file",
			},
			{
				name: "the suffix is ignored when the string is all slashes",
				args: []string{"/", "/"},
				want: "/",
			},
			{
				name: "an empty suffix leaves the result untouched",
				args: []string{"file.c", ""},
				want: "file.c",
			},
			{
				name: "a suffix not present is not an error",
				args: []string{"file.c", "x"},
				want: "file.c",
			},
			{
				name: "a suffix longer than the result",
				args: []string{"ab", "abab"},
				want: "ab",
			},
			{
				name: "a partial suffix match",
				args: []string{"file.txt", "txt"},
				want: "file.",
			},
			{
				name: "the suffix applies after directory removal",
				args: []string{"/a.c/b", ".c"},
				want: "b",
			},
			{
				name: "a trailing slash is removed before the suffix",
				args: []string{"x/y/", "y"},
				want: "y",
			},
			{
				name: "the suffix must match the last component",
				args: []string{"dir/file", "dir"},
				want: "file",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runBasename(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if want := test.want + "\n"; got != want {
					t.Errorf("Run() stdout = %q, want %q", got, want)
				}
			})
		}
	})

	t.Run("double dash", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "a leading double dash is skipped",
				args: []string{"--", "foo"},
				want: "foo",
			},
			{
				name: "operands after a leading double dash",
				args: []string{"--", "dir/file.c", ".c"},
				want: "file",
			},
			{
				name: "a double dash after the string is the suffix",
				args: []string{"foo", "--"},
				want: "foo",
			},
			{
				name: "a double dash operand after a leading double dash",
				args: []string{"--", "--"},
				want: "--",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runBasename(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if want := test.want + "\n"; got != want {
					t.Errorf("Run() stdout = %q, want %q", got, want)
				}
			})
		}
	})

	t.Run("backslashes", func(t *testing.T) {
		t.Run("unix", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("backslashes are separators on Windows")
			}

			tests := []struct {
				name string
				args []string
				want string
			}{
				{
					name: "backslashes are ordinary characters",
					args: []string{`a\b\c`},
					want: `a\b\c`,
				},
				{
					name: "a suffix is still removed",
					args: []string{`a\b.c`, ".c"},
					want: `a\b`,
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					got, err := runBasename(t, test.args...)
					if err != nil {
						t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
					}
					if want := test.want + "\n"; got != want {
						t.Errorf("Run() stdout = %q, want %q", got, want)
					}
				})
			}
		})

		t.Run("windows", func(t *testing.T) {
			if runtime.GOOS != "windows" {
				t.Skip("backslashes are only separators on Windows")
			}

			tests := []struct {
				name string
				args []string
				want string
			}{
				{
					name: "backslashes are separators",
					args: []string{`C:\Users\me\file.txt`},
					want: "file.txt",
				},
				{
					name: "a trailing backslash is removed",
					args: []string{`C:\Users\me\`},
					want: "me",
				},
				{
					name: "a suffix is removed",
					args: []string{`C:\Users\me\file.txt`, ".txt"},
					want: "file",
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					got, err := runBasename(t, test.args...)
					if err != nil {
						t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
					}
					if want := test.want + "\n"; got != want {
						t.Errorf("Run() stdout = %q, want %q", got, want)
					}
				})
			}
		})
	})

	t.Run("multiple", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "each operand is a name",
				args: []string{"-a", "/usr/bin/sort", "/usr/bin/cat"},
				want: "sort\ncat",
			},
			{
				name: "the long form",
				args: []string{"--multiple", "/usr/bin/sort", "/usr/bin/cat"},
				want: "sort\ncat",
			},
			{
				name: "a single name",
				args: []string{"-a", "usr"},
				want: "usr",
			},
			{
				name: "an operand that looks like a suffix is a name",
				args: []string{"-a", "include/stdio.h", ".h"},
				want: "stdio.h\n.h",
			},
			{
				name: "the traditional suffix operand is gone",
				args: []string{"-a", "file.c", ".c"},
				want: "file.c\n.c",
			},
			{
				name: "options must precede operands",
				args: []string{"foo", "-a"},
				want: "foo",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runBasename(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if want := test.want + "\n"; got != want {
					t.Errorf("Run() stdout = %q, want %q", got, want)
				}
			})
		}
	})

	t.Run("suffix option", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "the option-argument is held by the next argument",
				args: []string{"-s", ".h", "include/stdio.h"},
				want: "stdio",
			},
			{
				name: "the option-argument is glued to the option",
				args: []string{"-s.h", "include/stdio.h"},
				want: "stdio",
			},
			{
				name: "the long form with an attached option-argument",
				args: []string{"--suffix=.h", "include/stdio.h"},
				want: "stdio",
			},
			{
				name: "the long form with a separate option-argument",
				args: []string{"--suffix", ".h", "include/stdio.h"},
				want: "stdio",
			},
			{
				name: "the suffix option implies multiple",
				args: []string{"-s", ".h", "include/stdio.h", "include/stdlib.h"},
				want: "stdio\nstdlib",
			},
			{
				name: "the last suffix option wins",
				args: []string{"-s", ".c", "-s", ".h", "include/stdio.h"},
				want: "stdio",
			},
			{
				name: "a suffix identical to the result is kept",
				args: []string{"-s", "file", "file"},
				want: "file",
			},
			{
				name: "a suffix with slashes does nothing",
				args: []string{"-s", "usr/lib", "/usr/lib"},
				want: "lib",
			},
			{
				name: "the suffix applies after trailing slashes are removed",
				args: []string{"-s", "y", "x/y/"},
				want: "y",
			},
			{
				name: "an empty suffix leaves the result untouched",
				args: []string{"-s", "", "file.c"},
				want: "file.c",
			},
			{
				name: "the suffix must match the last component",
				args: []string{"-s", "dir", "dir/file"},
				want: "file",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runBasename(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if want := test.want + "\n"; got != want {
					t.Errorf("Run() stdout = %q, want %q", got, want)
				}
			})
		}
	})

	t.Run("zero", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{
				name: "the output ends in a NUL byte",
				args: []string{"-z", "dir/file"},
				want: "file\x00",
			},
			{
				name: "the long form",
				args: []string{"--zero", "dir/file"},
				want: "file\x00",
			},
			{
				name: "each result ends in a NUL byte",
				args: []string{"-z", "-a", "dir/file", "other/file"},
				want: "file\x00file\x00",
			},
			{
				name: "with the suffix option",
				args: []string{"-z", "-s", ".h", "a.h", "b.h"},
				want: "a\x00b\x00",
			},
			{
				name: "grouped with the multiple option",
				args: []string{"-az", "dir/file", "other"},
				want: "file\x00other\x00",
			},
			{
				name: "with the traditional suffix operand",
				args: []string{"-z", "a.c", ".c"},
				want: "a\x00",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runBasename(t, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got != test.want {
					t.Errorf("Run() stdout = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want int
		}{
			{
				name: "no operands",
				want: 2,
			},
			{
				name: "an unknown short option",
				args: []string{"-x"},
				want: 2,
			},
			{
				name: "an unknown long option",
				args: []string{"--bogus"},
				want: 2,
			},
			{
				name: "a long option that takes no argument",
				args: []string{"--zero=x"},
				want: 2,
			},
			{
				name: "the multiple long option takes no argument",
				args: []string{"--multiple=x"},
				want: 2,
			},
			{
				name: "a suffix option without its option-argument",
				args: []string{"-s"},
				want: 2,
			},
			{
				name: "a suffix long option without its option-argument",
				args: []string{"--suffix"},
				want: 2,
			},
			{
				name: "no names with the multiple option",
				args: []string{"-a"},
				want: 2,
			},
			{
				name: "no names with the suffix option",
				args: []string{"-s", ".h"},
				want: 2,
			},
			{
				name: "extra operands",
				args: []string{"a", "b", "c"},
				want: 2,
			},
			{
				name: "a lone double dash",
				args: []string{"--"},
				want: 2,
			},
			{
				name: "extra operands after a double dash",
				args: []string{"--", "a", "b", "c"},
				want: 2,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runBasename(t, test.args...)
				if code := exitCode(t, err); code != test.want {
					t.Fatalf("Run() exit code = %d, want %d (error = %v)", code, test.want, err)
				}
				if got != "" {
					t.Errorf("Run() stdout = %q, want empty", got)
				}
			})
		}
	})
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	rt := &coreutils.Runtime{
		Stdin:  &spyReader{},
		Stdout: errorWriter{err: writeErr},
		Stderr: new(bytes.Buffer),
	}
	err := New(rt).Run(t.Context(), "foo")

	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
}
