package dirname

import (
	"bytes"
	"context"
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

type testCase struct {
	name     string
	args     []string
	want     string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdin := &spyReader{}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.want {
		t.Errorf("Run() stdout = %q, want %q", got, test.want)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
	if stdin.read {
		t.Error("Run() used stdin, want it untouched")
	}
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
		tests := []testCase{
			{
				name: "a simple name has no directory",
				args: []string{"usr"},
				want: ".\n",
			},
			{
				name: "a null string gives the current directory",
				args: []string{""},
				want: ".\n",
			},
			{
				name: "a single slash is kept",
				args: []string{"/"},
				want: "/\n",
			},
			{
				name: "a double slash collapses to a single slash",
				args: []string{"//"},
				want: "/\n",
			},
			{
				name: "multiple slashes become a single slash",
				args: []string{"///"},
				want: "/\n",
			},
			{
				name: "a trailing slash is not counted",
				args: []string{"usr/"},
				want: ".\n",
			},
			{
				name: "the directory portion of a pathname",
				args: []string{"/usr/lib"},
				want: "/usr\n",
			},
			{
				name: "a trailing slash is removed",
				args: []string{"/usr/lib/"},
				want: "/usr\n",
			},
			{
				name: "redundant slashes are removed",
				args: []string{"//usr//lib//"},
				want: "//usr\n",
			},
			{
				name: "repeated internal slashes are collapsed",
				args: []string{"a//b"},
				want: "a\n",
			},
			{
				name: "leading slashes beyond a double slash keep",
				args: []string{"///a///b"},
				want: "///a\n",
			},
			{
				name: "a trailing slash after a directory",
				args: []string{"///a//b/"},
				want: "///a\n",
			},
			{
				name: "a relative pathname",
				args: []string{"relative/path"},
				want: "relative\n",
			},
			{
				name: "several components",
				args: []string{"a/b/c/d"},
				want: "a/b/c\n",
			},
			{
				name: "a dot component is not removed by itself",
				args: []string{"dir/."},
				want: "dir\n",
			},
			{
				name: "a dot-dot component is not removed",
				args: []string{"dir/.."},
				want: "dir\n",
			},
			{
				name: "a dot-dot before the last component is kept",
				args: []string{"a/../b"},
				want: "a/..\n",
			},
			{
				name: "a lone dot-dot has no directory",
				args: []string{".."},
				want: ".\n",
			},
			{
				name: "a lone dot has no directory",
				args: []string{"."},
				want: ".\n",
			},
			{
				name: "a lone dash is an operand",
				args: []string{"-"},
				want: ".\n",
			},
			{
				name: "multi-byte characters",
				args: []string{"dir/ção"},
				want: "dir\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("multiple names", func(t *testing.T) {
		tests := []testCase{
			{
				name: "two names",
				args: []string{"dir1/str", "dir2/str"},
				want: "dir1\ndir2\n",
			},
			{
				name: "three names",
				args: []string{"a/b", "c", "/d/e"},
				want: "a\n.\n/d\n",
			},
			{
				name: "a name after the first may start with a dash",
				args: []string{"a/b", "-x"},
				want: "a\n.\n",
			},
			{
				name: "an empty name among others",
				args: []string{"a/b", ""},
				want: "a\n.\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("zero", func(t *testing.T) {
		tests := []testCase{
			{
				name: "a single name",
				args: []string{"-z", "dir/str"},
				want: "dir\x00",
			},
			{
				name: "the long form",
				args: []string{"--zero", "dir/str"},
				want: "dir\x00",
			},
			{
				name: "the option may be grouped",
				args: []string{"-zz", "dir/str"},
				want: "dir\x00",
			},
			{
				name: "each name ends with a NUL byte",
				args: []string{"-z", "dir1/str", "dir2/str"},
				want: "dir1\x00dir2\x00",
			},
			{
				name: "names with embedded newlines stay parseable",
				args: []string{"-z", "dir\nname/str"},
				want: "dir\nname\x00",
			},
			{
				name: "options end at a double dash",
				args: []string{"-z", "--", "-z"},
				want: ".\x00",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("double dash", func(t *testing.T) {
		tests := []testCase{
			{
				name: "a leading double dash is skipped",
				args: []string{"--", "foo"},
				want: ".\n",
			},
			{
				name: "operands after a leading double dash",
				args: []string{"--", "dir/file"},
				want: "dir\n",
			},
			{
				name: "a double dash operand after a leading double dash",
				args: []string{"--", "--"},
				want: ".\n",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("backslashes", func(t *testing.T) {
		t.Run("unix", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("backslashes are separators on Windows")
			}

			tests := []testCase{
				{
					name: "backslashes are ordinary characters",
					args: []string{`a\b\c`},
					want: ".\n",
				},
				{
					name: "a forward slash splits the path",
					args: []string{`a\b/c`},
					want: "a\\b\n",
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					runCase(t, test)
				})
			}
		})

		t.Run("windows", func(t *testing.T) {
			if runtime.GOOS != "windows" {
				t.Skip("backslashes are only separators on Windows")
			}

			tests := []testCase{
				{
					name: "backslashes are separators",
					args: []string{`C:\Users\me\file.txt`},
					want: "C:/Users/me\n",
				},
				{
					name: "a trailing backslash is not counted",
					args: []string{`C:\Users\me\`},
					want: "C:/Users\n",
				},
				{
					name: "a mix of separators",
					args: []string{`C:\Users/me/file.txt`},
					want: "C:/Users/me\n",
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					runCase(t, test)
				})
			}
		})
	})

	t.Run("errors", func(t *testing.T) {
		tests := []testCase{
			{
				name:     "no operands",
				args:     []string{},
				wantErr:  "dirname: missing operand\n",
				wantExit: 2,
			},
			{
				name:     "a lone double dash",
				args:     []string{"--"},
				wantErr:  "dirname: missing operand\n",
				wantExit: 2,
			},
			{
				name:     "only an option",
				args:     []string{"-z"},
				wantErr:  "dirname: missing operand\n",
				wantExit: 2,
			},
			{
				name:     "an invalid option",
				args:     []string{"-x", "dir/str"},
				wantErr:  "dirname: invalid option -- 'x'\n",
				wantExit: 2,
			},
			{
				name:     "an unrecognized long option",
				args:     []string{"--bogus", "dir/str"},
				wantErr:  "dirname: unrecognized option '--bogus'\n",
				wantExit: 2,
			},
			{
				name:     "a long option taking an argument",
				args:     []string{"--zero=", "dir/str"},
				wantErr:  "dirname: option '--zero=' doesn't allow an argument\n",
				wantExit: 2,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				runCase(t, test)
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

func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rt := &coreutils.Runtime{
		Stdin:  &spyReader{},
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
	}
	err := New(rt).Run(ctx, "a/b", "c/d")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}
