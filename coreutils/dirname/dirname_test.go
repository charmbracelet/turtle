package dirname

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

func runDirname(t *testing.T, args ...string) (string, error) {
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
				name: "a simple name has no directory",
				args: []string{"usr"},
				want: ".",
			},
			{
				name: "a null string gives the current directory",
				args: []string{""},
				want: ".",
			},
			{
				name: "a single slash is kept",
				args: []string{"/"},
				want: "/",
			},
			{
				name: "a double slash collapses to a single slash",
				args: []string{"//"},
				want: "/",
			},
			{
				name: "multiple slashes become a single slash",
				args: []string{"///"},
				want: "/",
			},
			{
				name: "a trailing slash is not counted",
				args: []string{"usr/"},
				want: ".",
			},
			{
				name: "the directory portion of a pathname",
				args: []string{"/usr/lib"},
				want: "/usr",
			},
			{
				name: "a trailing slash is removed",
				args: []string{"/usr/lib/"},
				want: "/usr",
			},
			{
				name: "redundant slashes are removed",
				args: []string{"//usr//lib//"},
				want: "//usr",
			},
			{
				name: "repeated internal slashes are collapsed",
				args: []string{"a//b"},
				want: "a",
			},
			{
				name: "leading slashes beyond a double slash keep",
				args: []string{"///a///b"},
				want: "///a",
			},
			{
				name: "a trailing slash after a directory",
				args: []string{"///a//b/"},
				want: "///a",
			},
			{
				name: "a relative pathname",
				args: []string{"relative/path"},
				want: "relative",
			},
			{
				name: "several components",
				args: []string{"a/b/c/d"},
				want: "a/b/c",
			},
			{
				name: "a dot component is not removed by itself",
				args: []string{"dir/."},
				want: "dir",
			},
			{
				name: "a dot-dot component is not removed",
				args: []string{"dir/.."},
				want: "dir",
			},
			{
				name: "a dot-dot before the last component is kept",
				args: []string{"a/../b"},
				want: "a/..",
			},
			{
				name: "a lone dot-dot has no directory",
				args: []string{".."},
				want: ".",
			},
			{
				name: "a lone dot has no directory",
				args: []string{"."},
				want: ".",
			},
			{
				name: "a lone dash is an operand",
				args: []string{"-"},
				want: ".",
			},
			{
				name: "an argument starting with a dash is an operand",
				args: []string{"-x"},
				want: ".",
			},
			{
				name: "multi-byte characters",
				args: []string{"dir/ção"},
				want: "dir",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runDirname(t, test.args...)
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
				want: ".",
			},
			{
				name: "operands after a leading double dash",
				args: []string{"--", "dir/file"},
				want: "dir",
			},
			{
				name: "a double dash operand after a leading double dash",
				args: []string{"--", "--"},
				want: ".",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runDirname(t, test.args...)
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
					want: ".",
				},
				{
					name: "a forward slash splits the path",
					args: []string{`a\b/c`},
					want: `a\b`,
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					got, err := runDirname(t, test.args...)
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
					want: "C:/Users/me",
				},
				{
					name: "a trailing backslash is not counted",
					args: []string{`C:\Users\me\`},
					want: "C:/Users",
				},
				{
					name: "a mix of separators",
					args: []string{`C:\Users/me/file.txt`},
					want: "C:/Users/me",
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					got, err := runDirname(t, test.args...)
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
				name: "a lone double dash",
				args: []string{"--"},
				want: 2,
			},
			{
				name: "extra operands",
				args: []string{"a", "b"},
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
				got, err := runDirname(t, test.args...)
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
	err := New(rt).Run(t.Context(), "foo/bar")

	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
}
