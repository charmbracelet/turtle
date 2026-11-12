package dirname

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
)

// runCase runs a single case, also checking that dirname never reads the
// standard input: it only takes operands. It stays local for that extra
// assertion, which testutil.Run has no room for.
func runCase(t *testing.T, test testutil.Case) {
	t.Helper()

	stdin := &testutil.SpyReader{}
	test.Stdin = stdin
	test.Check = func(t *testing.T, rt *coreutils.Runtime) {
		if stdin.Reads > 0 {
			t.Error("Run() used stdin, want it untouched")
		}
	}
	testutil.Once(t, New, test)
}

func TestRun(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		tests := []testutil.Case{
			{
				Name: "a simple name has no directory",
				Args: []string{"usr"},
				Want: ".\n",
			},
			{
				Name: "a null string gives the current directory",
				Args: []string{""},
				Want: ".\n",
			},
			{
				Name: "a single slash is kept",
				Args: []string{"/"},
				Want: "/\n",
			},
			{
				Name: "a double slash collapses to a single slash",
				Args: []string{"//"},
				Want: "/\n",
			},
			{
				Name: "multiple slashes become a single slash",
				Args: []string{"///"},
				Want: "/\n",
			},
			{
				Name: "a trailing slash is not counted",
				Args: []string{"usr/"},
				Want: ".\n",
			},
			{
				Name: "the directory portion of a pathname",
				Args: []string{"/usr/lib"},
				Want: "/usr\n",
			},
			{
				Name: "a trailing slash is removed",
				Args: []string{"/usr/lib/"},
				Want: "/usr\n",
			},
			{
				Name: "redundant slashes are removed",
				Args: []string{"//usr//lib//"},
				Want: "//usr\n",
			},
			{
				Name: "repeated internal slashes are collapsed",
				Args: []string{"a//b"},
				Want: "a\n",
			},
			{
				Name: "leading slashes beyond a double slash keep",
				Args: []string{"///a///b"},
				Want: "///a\n",
			},
			{
				Name: "a trailing slash after a directory",
				Args: []string{"///a//b/"},
				Want: "///a\n",
			},
			{
				Name: "a relative pathname",
				Args: []string{"relative/path"},
				Want: "relative\n",
			},
			{
				Name: "several components",
				Args: []string{"a/b/c/d"},
				Want: "a/b/c\n",
			},
			{
				Name: "a dot component is not removed by itself",
				Args: []string{"dir/."},
				Want: "dir\n",
			},
			{
				Name: "a dot-dot component is not removed",
				Args: []string{"dir/.."},
				Want: "dir\n",
			},
			{
				Name: "a dot-dot before the last component is kept",
				Args: []string{"a/../b"},
				Want: "a/..\n",
			},
			{
				Name: "a lone dot-dot has no directory",
				Args: []string{".."},
				Want: ".\n",
			},
			{
				Name: "a lone dot has no directory",
				Args: []string{"."},
				Want: ".\n",
			},
			{
				Name: "a lone dash is an operand",
				Args: []string{"-"},
				Want: ".\n",
			},
			{
				Name: "multi-byte characters",
				Args: []string{"dir/ção"},
				Want: "dir\n",
			},
		}

		for _, test := range tests {
			t.Run(test.Name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("multiple names", func(t *testing.T) {
		tests := []testutil.Case{
			{
				Name: "two names",
				Args: []string{"dir1/str", "dir2/str"},
				Want: "dir1\ndir2\n",
			},
			{
				Name: "three names",
				Args: []string{"a/b", "c", "/d/e"},
				Want: "a\n.\n/d\n",
			},
			{
				Name: "a name after the first may start with a dash",
				Args: []string{"a/b", "-x"},
				Want: "a\n.\n",
			},
			{
				Name: "an empty name among others",
				Args: []string{"a/b", ""},
				Want: "a\n.\n",
			},
		}

		for _, test := range tests {
			t.Run(test.Name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("zero", func(t *testing.T) {
		tests := []testutil.Case{
			{
				Name: "a single name",
				Args: []string{"-z", "dir/str"},
				Want: "dir\x00",
			},
			{
				Name: "the long form",
				Args: []string{"--zero", "dir/str"},
				Want: "dir\x00",
			},
			{
				Name: "the option may be grouped",
				Args: []string{"-zz", "dir/str"},
				Want: "dir\x00",
			},
			{
				Name: "each name ends with a NUL byte",
				Args: []string{"-z", "dir1/str", "dir2/str"},
				Want: "dir1\x00dir2\x00",
			},
			{
				Name: "names with embedded newlines stay parseable",
				Args: []string{"-z", "dir\nname/str"},
				Want: "dir\nname\x00",
			},
			{
				Name: "options end at a double dash",
				Args: []string{"-z", "--", "-z"},
				Want: ".\x00",
			},
		}

		for _, test := range tests {
			t.Run(test.Name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("double dash", func(t *testing.T) {
		tests := []testutil.Case{
			{
				Name: "a leading double dash is skipped",
				Args: []string{"--", "foo"},
				Want: ".\n",
			},
			{
				Name: "operands after a leading double dash",
				Args: []string{"--", "dir/file"},
				Want: "dir\n",
			},
			{
				Name: "a double dash operand after a leading double dash",
				Args: []string{"--", "--"},
				Want: ".\n",
			},
		}

		for _, test := range tests {
			t.Run(test.Name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})

	t.Run("backslashes", func(t *testing.T) {
		t.Run("unix", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("backslashes are separators on Windows")
			}

			tests := []testutil.Case{
				{
					Name: "backslashes are ordinary characters",
					Args: []string{`a\b\c`},
					Want: ".\n",
				},
				{
					Name: "a forward slash splits the path",
					Args: []string{`a\b/c`},
					Want: "a\\b\n",
				},
			}

			for _, test := range tests {
				t.Run(test.Name, func(t *testing.T) {
					runCase(t, test)
				})
			}
		})

		t.Run("windows", func(t *testing.T) {
			if runtime.GOOS != "windows" {
				t.Skip("backslashes are only separators on Windows")
			}

			tests := []testutil.Case{
				{
					Name: "backslashes are separators",
					Args: []string{`C:\Users\me\file.txt`},
					Want: "C:/Users/me\n",
				},
				{
					Name: "a trailing backslash is not counted",
					Args: []string{`C:\Users\me\`},
					Want: "C:/Users\n",
				},
				{
					Name: "a mix of separators",
					Args: []string{`C:\Users/me/file.txt`},
					Want: "C:/Users/me\n",
				},
			}

			for _, test := range tests {
				t.Run(test.Name, func(t *testing.T) {
					runCase(t, test)
				})
			}
		})
	})

	t.Run("errors", func(t *testing.T) {
		tests := []testutil.Case{
			{
				Name:     "no operands",
				Args:     []string{},
				WantErr:  "dirname: missing operand\nTry 'dirname --help' for more information.\n",
				WantExit: 2,
			},
			{
				Name:     "a lone double dash",
				Args:     []string{"--"},
				WantErr:  "dirname: missing operand\nTry 'dirname --help' for more information.\n",
				WantExit: 2,
			},
			{
				Name:     "only an option",
				Args:     []string{"-z"},
				WantErr:  "dirname: missing operand\nTry 'dirname --help' for more information.\n",
				WantExit: 2,
			},
			{
				Name:     "an invalid option",
				Args:     []string{"-x", "dir/str"},
				WantErr:  "dirname: invalid option -- 'x'\nTry 'dirname --help' for more information.\n",
				WantExit: 2,
			},
			{
				Name:     "an unrecognized long option",
				Args:     []string{"--bogus", "dir/str"},
				WantErr:  "dirname: unrecognized option '--bogus'\nTry 'dirname --help' for more information.\n",
				WantExit: 2,
			},
			{
				Name:     "a long option taking an argument",
				Args:     []string{"--zero=", "dir/str"},
				WantErr:  "dirname: option '--zero=' doesn't allow an argument\nTry 'dirname --help' for more information.\n",
				WantExit: 2,
			},
		}

		for _, test := range tests {
			t.Run(test.Name, func(t *testing.T) {
				runCase(t, test)
			})
		}
	})
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	rt := &coreutils.Runtime{
		Stdin:  &testutil.SpyReader{},
		Stdout: testutil.ErrorWriter{Err: writeErr},
		Stderr: new(bytes.Buffer),
	}
	err := New(rt).Run(t.Context(), "foo/bar")

	if code := testutil.ExitCode(t, err); code != 1 {
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
		Stdin:  &testutil.SpyReader{},
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
	}
	err := New(rt).Run(ctx, "a/b", "c/d")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		runCase(t, testutil.Case{
			Name: "help",
			Args: []string{"--help"},
			Want: usage,
		})
	})

	t.Run("version", func(t *testing.T) {
		runCase(t, testutil.Case{
			Name: "version",
			Args: []string{"--version"},
			Want: "dirname (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n",
		})
	})
}
