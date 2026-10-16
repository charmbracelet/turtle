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
				name: "an argument starting with a dash is an operand",
				args: []string{"-x"},
				want: "-x",
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
