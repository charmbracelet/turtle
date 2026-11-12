package which

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
)

const (
	execMode  = fs.FileMode(0o755)
	plainMode = fs.FileMode(0o644)
)

// runWhich runs which with the given environment, working directory, tty
// state and arguments, returning its outputs and error.
func runWhich(t *testing.T, environ []string, workDir string, tty bool, args ...string) (string, string, error) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout:     stdout,
		Stderr:     stderr,
		WorkingDir: workDir,
		Environ:    func() []string { return environ },
		IsTerminal: func(fd int) bool { return tty && fd == 1 },
	}
	err := New(rt).Run(t.Context(), args...)
	return stdout.String(), stderr.String(), err
}

// writeFile creates a file at path with the given mode.
func writeFile(t *testing.T, path string, mode fs.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
}

// pathVar builds the environment holding a PATH with the given
// directories, joined with the separator of the host.
func pathVar(dirs ...string) []string {
	return []string{"PATH=" + strings.Join(dirs, string(filepath.ListSeparator))}
}

// homeVar builds the environment holding only a HOME variable.
func homeVar(home string) []string {
	return []string{"HOME=" + home}
}

// withPath returns the environment built by pathVar with HOME appended.
func withPath(home string, dirs ...string) []string {
	return append(pathVar(dirs...), "HOME="+home)
}

type testCase struct {
	name     string
	args     []string
	environ  []string
	workDir  string
	tty      bool
	want     string
	wantErr  string
	wantExit int
}

func runCases(t *testing.T, tests []testCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, stderr, err := runWhich(t, test.environ, test.workDir, test.tty, test.args...)
			if code := testutil.ExitCode(t, err); code != test.wantExit {
				t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
			}
			if got != test.want {
				t.Errorf("Run() stdout = %q, want %q", got, test.want)
			}
			if stderr != test.wantErr {
				t.Errorf("Run() stderr = %q, want %q", stderr, test.wantErr)
			}
		})
	}
}

func TestRun(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	work := t.TempDir()

	writeFile(t, filepath.Join(dir1, "foo"), execMode)
	writeFile(t, filepath.Join(dir1, "bar"), execMode)
	writeFile(t, filepath.Join(dir1, "plain"), plainMode)
	if err := os.Mkdir(filepath.Join(dir1, "prog"), execMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir1, "sub"), execMode); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir1, "sub", "deep"), execMode)
	writeFile(t, filepath.Join(dir2, "foo"), execMode)
	writeFile(t, filepath.Join(dir2, "baz"), execMode)
	writeFile(t, filepath.Join(dir2, "local"), execMode)
	writeFile(t, filepath.Join(work, "local"), execMode)
	writeFile(t, filepath.Join(work, "sub", "tool"), execMode)
	writeFile(t, filepath.Join(work, "~", "tilded"), execMode)

	environ := withPath(dir1, dir1, dir2)
	pathList := strings.Join([]string{dir1, dir2}, string(filepath.ListSeparator))

	t.Run("search", func(t *testing.T) {
		tests := []testCase{
			{
				name:    "the first match is printed",
				args:    []string{"foo"},
				environ: environ,
				want:    filepath.Join(dir1, "foo") + "\n",
			},
			{
				name:    "the directories are searched in order",
				args:    []string{"baz"},
				environ: environ,
				want:    filepath.Join(dir2, "baz") + "\n",
			},
			{
				name:    "every operand is printed in order",
				args:    []string{"foo", "bar", "baz"},
				environ: environ,
				want: filepath.Join(dir1, "foo") + "\n" +
					filepath.Join(dir1, "bar") + "\n" +
					filepath.Join(dir2, "baz") + "\n",
			},
			{
				name:     "a command which is not found is diagnosed",
				args:     []string{"missing"},
				environ:  environ,
				wantErr:  "which: no missing in (" + pathList + ")\n",
				wantExit: 1,
			},
			{
				name:    "the exit status counts the failures",
				args:    []string{"foo", "missing", "gone"},
				environ: environ,
				want:    filepath.Join(dir1, "foo") + "\n",
				wantErr: "which: no missing in (" + pathList + ")\n" +
					"which: no gone in (" + pathList + ")\n",
				wantExit: 2,
			},
			{
				name:     "a directory is not a match",
				args:     []string{"prog"},
				environ:  environ,
				wantErr:  "which: no prog in (" + pathList + ")\n",
				wantExit: 1,
			},
			{
				name:    "a directory entry which is missing is skipped",
				args:    []string{"foo"},
				environ: pathVar(filepath.Join(work, "nope"), dir1),
				want:    filepath.Join(dir1, "foo") + "\n",
			},
		}

		runCases(t, tests)
	})

	t.Run("all", func(t *testing.T) {
		tests := []testCase{
			{
				name:    "every match is printed in order",
				args:    []string{"-a", "foo"},
				environ: environ,
				want: filepath.Join(dir1, "foo") + "\n" +
					filepath.Join(dir2, "foo") + "\n",
			},
			{
				name:    "--all means the same",
				args:    []string{"--all", "foo"},
				environ: environ,
				want: filepath.Join(dir1, "foo") + "\n" +
					filepath.Join(dir2, "foo") + "\n",
			},
			{
				name:    "a command with a single match prints it once",
				args:    []string{"-a", "bar"},
				environ: environ,
				want:    filepath.Join(dir1, "bar") + "\n",
			},
			{
				name:     "a command which is not found is still diagnosed",
				args:     []string{"-a", "missing"},
				environ:  environ,
				wantErr:  "which: no missing in (" + pathList + ")\n",
				wantExit: 1,
			},
		}

		runCases(t, tests)
	})

	t.Run("executables", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows accepts any file found on the PATH")
		}

		runCases(t, []testCase{
			{
				name:     "a file without an executable bit is no match",
				args:     []string{"plain"},
				environ:  environ,
				wantErr:  "which: no plain in (" + pathList + ")\n",
				wantExit: 1,
			},
			{
				name:     "-a still skips a file without an executable bit",
				args:     []string{"-a", "plain"},
				environ:  environ,
				wantErr:  "which: no plain in (" + pathList + ")\n",
				wantExit: 1,
			},
		})
	})

	t.Run("names with a separator", func(t *testing.T) {
		tests := []testCase{
			{
				name:    "a relative name is checked from the working directory",
				args:    []string{"sub/tool"},
				environ: environ,
				workDir: work,
				want:    filepath.Join(work, "sub", "tool") + "\n",
			},
			{
				name:    "a name starting with a dot",
				args:    []string{"./local"},
				environ: environ,
				workDir: work,
				want:    filepath.Join(work, "local") + "\n",
			},
			{
				name:    "--show-dot abbreviates a relative match",
				args:    []string{"--show-dot", "./local"},
				environ: environ,
				workDir: work,
				want:    "." + string(filepath.Separator) + "local\n",
			},
			{
				name:    "--show-dot abbreviates a relative match in a subdirectory",
				args:    []string{"--show-dot", "sub/tool"},
				environ: environ,
				workDir: work,
				want:    "." + string(filepath.Separator) + "sub" + string(filepath.Separator) + "tool\n",
			},
			{
				name:    "--show-tilde abbreviates a match under HOME",
				args:    []string{"--show-tilde", "./local"},
				environ: append(homeVar(work), "PATH="+work),
				workDir: work,
				want:    "~" + string(filepath.Separator) + "local\n",
			},
			{
				name:    "an absolute name is checked as it is",
				args:    []string{filepath.Join(dir1, "foo")},
				environ: environ,
				want:    filepath.Join(dir1, "foo") + "\n",
			},
			{
				name:     "a missing relative name names its typed directory",
				args:     []string{"sub/gone"},
				environ:  environ,
				workDir:  work,
				wantErr:  "which: no gone in (./sub)\n",
				wantExit: 1,
			},
			{
				name:     "a directory holding a separator is no match",
				args:     []string{"./prog"},
				environ:  environ,
				workDir:  work,
				wantErr:  "which: no prog in (.)\n",
				wantExit: 1,
			},
			{
				name:     "a tilde name names its typed directory",
				args:     []string{"~/gone"},
				environ:  environ,
				wantErr:  "which: no gone in (~)\n",
				wantExit: 1,
			},
		}

		runCases(t, tests)
	})

	t.Run("path", func(t *testing.T) {
		tests := []testCase{
			{
				name:     "an unset PATH searches the default path",
				args:     []string{"foo"},
				environ:  homeVar(dir1),
				wantErr:  "which: no foo in (/bin:/usr/bin)\n",
				wantExit: 1,
			},
			{
				name:     "an empty environment searches the default path",
				args:     []string{"foo"},
				wantErr:  "which: no foo in (/bin:/usr/bin)\n",
				wantExit: 1,
			},
			{
				name:     "an empty PATH searches the current directory",
				args:     []string{"local"},
				environ:  []string{"PATH="},
				workDir:  work,
				want:     filepath.Join(work, "local") + "\n",
				wantExit: 0,
			},
			{
				name:     "an empty entry means the current directory",
				args:     []string{"local"},
				environ:  pathVar("", dir1),
				workDir:  work,
				want:     filepath.Join(work, "local") + "\n",
				wantExit: 0,
			},
			{
				name:     "the diagnostic shows the path as it is",
				args:     []string{"missing"},
				environ:  pathVar("~", dir1),
				wantErr:  "which: no missing in (~" + string(filepath.ListSeparator) + dir1 + ")\n",
				wantExit: 1,
			},
		}

		runCases(t, tests)
	})

	t.Run("skip-dot", func(t *testing.T) {
		writeFile(t, filepath.Join(work, "bin", "shim"), execMode)
		writeFile(t, filepath.Join(dir2, "shim"), execMode)
		tests := []testCase{
			{
				name:    "a dot entry matches without the option",
				args:    []string{"local"},
				environ: pathVar(".", dir2),
				workDir: work,
				want:    filepath.Join(work, "local") + "\n",
			},
			{
				name:    "a dot entry is skipped",
				args:    []string{"--skip-dot", "local"},
				environ: pathVar(".", dir2),
				workDir: work,
				want:    filepath.Join(dir2, "local") + "\n",
			},
			{
				name:    "a relative entry without a dot is searched without the option",
				args:    []string{"shim"},
				environ: pathVar("bin", dir2),
				workDir: work,
				want:    filepath.Join(work, "bin", "shim") + "\n",
			},
			{
				name:    "a relative entry without a dot is skipped too",
				args:    []string{"--skip-dot", "shim"},
				environ: pathVar("bin", dir2),
				workDir: work,
				want:    filepath.Join(dir2, "shim") + "\n",
			},
			{
				name:    "another relative entry without a dot is skipped too",
				args:    []string{"--skip-dot", "local"},
				environ: pathVar("sub", "."),
				workDir: work,
				wantErr: "which: no local in (sub" +
					string(filepath.ListSeparator) + ".)\n",
				wantExit: 1,
			},
			{
				name:    "an absolute entry is kept",
				args:    []string{"--skip-dot", "foo"},
				environ: pathVar(dir1, "."),
				workDir: work,
				want:    filepath.Join(dir1, "foo") + "\n",
			},
		}

		runCases(t, tests)
	})

	t.Run("skip-tilde", func(t *testing.T) {
		tests := []testCase{
			{
				name:    "a tilde entry is not expanded and matches as it is",
				args:    []string{"tilded"},
				environ: pathVar("~", dir1),
				workDir: work,
				want:    filepath.Join(work, "~", "tilded") + "\n",
			},
			{
				name:    "a tilde entry is skipped",
				args:    []string{"--skip-tilde", "tilded"},
				environ: pathVar("~", dir1),
				workDir: work,
				wantErr: "which: no tilded in (~" +
					string(filepath.ListSeparator) + dir1 + ")\n",
				wantExit: 1,
			},
			{
				name:    "an executable under HOME matches without the option",
				args:    []string{"foo"},
				environ: withPath(dir1, dir1),
				want:    filepath.Join(dir1, "foo") + "\n",
			},
			{
				name:     "an executable under HOME is skipped",
				args:     []string{"--skip-tilde", "foo"},
				environ:  withPath(dir1, dir1),
				wantErr:  "which: no foo in (" + dir1 + ")\n",
				wantExit: 1,
			},
			{
				name:    "an executable outside HOME is kept",
				args:    []string{"--skip-tilde", "baz"},
				environ: withPath(dir1, dir2),
				want:    filepath.Join(dir2, "baz") + "\n",
			},
			{
				name:    "an unset HOME matches nothing under it",
				args:    []string{"--skip-tilde", "foo"},
				environ: pathVar(dir1),
				want:    filepath.Join(dir1, "foo") + "\n",
			},
		}

		runCases(t, tests)
	})

	t.Run("show-dot", func(t *testing.T) {
		dot := "." + string(filepath.Separator)
		tests := []testCase{
			{
				name:    "a dot entry is printed in full without the option",
				args:    []string{"local"},
				environ: pathVar("."),
				workDir: work,
				want:    filepath.Join(work, "local") + "\n",
			},
			{
				name:    "a match under the working directory prints a dot",
				args:    []string{"--show-dot", "local"},
				environ: pathVar("."),
				workDir: work,
				want:    dot + "local\n",
			},
			{
				name:    "a match under a dot entry keeps the rest of the path",
				args:    []string{"--show-dot", "tool"},
				environ: pathVar("./sub"),
				workDir: work,
				want:    dot + "sub" + string(filepath.Separator) + "tool\n",
			},
			{
				name:    "an absolute entry is not abbreviated",
				args:    []string{"--show-dot", "foo"},
				environ: pathVar(dir1),
				want:    filepath.Join(dir1, "foo") + "\n",
			},
			{
				name:    "-a abbreviates every match",
				args:    []string{"-a", "--show-dot", "local"},
				environ: pathVar(".", dir2),
				workDir: work,
				want:    dot + "local\n" + filepath.Join(dir2, "local") + "\n",
			},
		}

		runCases(t, tests)
	})

	t.Run("show-tilde", func(t *testing.T) {
		tilde := "~" + string(filepath.Separator)
		tests := []testCase{
			{
				name:    "a match under HOME prints a tilde",
				args:    []string{"--show-tilde", "foo"},
				environ: withPath(dir1, dir1),
				want:    tilde + "foo\n",
			},
			{
				name:    "a match outside HOME prints the path",
				args:    []string{"--show-tilde", "baz"},
				environ: withPath(dir1, dir2),
				want:    filepath.Join(dir2, "baz") + "\n",
			},
			{
				name:    "a match under HOME keeps the rest of the path",
				args:    []string{"--show-tilde", "deep"},
				environ: withPath(dir1, filepath.Join(dir1, "sub")),
				want:    tilde + "sub" + string(filepath.Separator) + "deep\n",
			},
			{
				name:    "a match prints the path when HOME is unset",
				args:    []string{"--show-tilde", "foo"},
				environ: pathVar(dir1),
				want:    filepath.Join(dir1, "foo") + "\n",
			},
			{
				name:    "a match prints the path without the option",
				args:    []string{"foo"},
				environ: withPath(dir1, dir1),
				want:    filepath.Join(dir1, "foo") + "\n",
			},
		}

		runCases(t, tests)
	})

	t.Run("tty-only", func(t *testing.T) {
		tests := []testCase{
			{
				name:    "the options on the right are ignored off a tty",
				args:    []string{"--tty-only", "--show-dot", "local"},
				environ: pathVar("."),
				workDir: work,
				want:    filepath.Join(work, "local") + "\n",
			},
			{
				name:    "the options on the right are kept on a tty",
				args:    []string{"--tty-only", "--show-dot", "local"},
				environ: pathVar("."),
				workDir: work,
				tty:     true,
				want:    "." + string(filepath.Separator) + "local\n",
			},
			{
				name:    "the options on the left are kept off a tty",
				args:    []string{"--show-dot", "--tty-only", "local"},
				environ: pathVar("."),
				workDir: work,
				want:    "." + string(filepath.Separator) + "local\n",
			},
			{
				name:    "--all is not a cosmetic option",
				args:    []string{"--tty-only", "-a", "foo"},
				environ: environ,
				want: filepath.Join(dir1, "foo") + "\n" +
					filepath.Join(dir2, "foo") + "\n",
			},
		}

		runCases(t, tests)
	})

	t.Run("options", func(t *testing.T) {
		tryHint := "Try 'which --help' for more information.\n"
		tests := []testCase{
			{
				name:    "--help prints the usage text",
				args:    []string{"--help"},
				environ: environ,
				want:    usage,
			},
			{
				name:    "--version prints the version text",
				args:    []string{"--version"},
				environ: environ,
				want:    "which (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n",
			},
			{
				name:    "-v means --version",
				args:    []string{"-v"},
				environ: environ,
				want:    "which (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n",
			},
			{
				name:    "-V means --version",
				args:    []string{"-V"},
				environ: environ,
				want:    "which (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n",
			},
			{
				name:     "an invalid short option is a usage error",
				args:     []string{"-Z", "foo"},
				environ:  environ,
				wantErr:  "which: invalid option -- 'Z'\n" + tryHint,
				wantExit: 1,
			},
			{
				name:     "an unrecognized long option is a usage error",
				args:     []string{"--bogus", "foo"},
				environ:  environ,
				wantErr:  "which: unrecognized option '--bogus'\n" + tryHint,
				wantExit: 1,
			},
			{
				name:     "a long option taking no argument rejects one",
				args:     []string{"--all=x", "foo"},
				environ:  environ,
				wantErr:  "which: option '--all=x' doesn't allow an argument\n" + tryHint,
				wantExit: 1,
			},
			{
				name:     "no command prints the usage text to the error",
				args:     []string{},
				environ:  environ,
				wantErr:  usage,
				wantExit: 255,
			},
			{
				name:    "-- ends the options",
				args:    []string{"--", "--version"},
				environ: pathVar(dir1),
				want:    filepath.Join(dir1, "--version") + "\n",
			},
			{
				name:     "an operand after a command is not an option",
				args:     []string{"foo", "--help"},
				environ:  environ,
				want:     filepath.Join(dir1, "foo") + "\n",
				wantErr:  "which: no --help in (" + pathList + ")\n",
				wantExit: 1,
			},
			{
				name: "the alias and function options are accepted and ignored",
				args: []string{
					"--read-alias", "--skip-alias",
					"--read-functions", "--skip-functions", "foo",
				},
				environ: environ,
				want:    filepath.Join(dir1, "foo") + "\n",
			},
			{
				name:    "-i means --read-alias",
				args:    []string{"-i", "foo"},
				environ: environ,
				want:    filepath.Join(dir1, "foo") + "\n",
			},
		}

		// A file named like the version option backs the "--" case.
		writeFile(t, filepath.Join(dir1, "--version"), execMode)

		runCases(t, tests)
	})
}

func TestWindowsExtensions(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific behavior")
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "foo.exe"), execMode)
	writeFile(t, filepath.Join(dir, "bar"), execMode)
	writeFile(t, filepath.Join(dir, "plain"), plainMode)

	t.Run("the .exe extension is tried first", func(t *testing.T) {
		stdout, _, err := runWhich(t, pathVar(dir), "", false, "foo")
		if code := testutil.ExitCode(t, err); code != 0 {
			t.Fatalf("Run() exit code = %d, want 0 (error = %v)", code, err)
		}
		if want := filepath.Join(dir, "foo.exe") + "\n"; stdout != want {
			t.Errorf("Run() stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("a file without an extension is accepted", func(t *testing.T) {
		stdout, _, err := runWhich(t, pathVar(dir), "", false, "bar")
		if code := testutil.ExitCode(t, err); code != 0 {
			t.Fatalf("Run() exit code = %d, want 0 (error = %v)", code, err)
		}
		if want := filepath.Join(dir, "bar") + "\n"; stdout != want {
			t.Errorf("Run() stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("a name with an extension keeps it", func(t *testing.T) {
		stdout, _, err := runWhich(t, pathVar(dir), "", false, "plain")
		if code := testutil.ExitCode(t, err); code != 0 {
			t.Fatalf("Run() exit code = %d, want 0 (error = %v)", code, err)
		}
		if want := filepath.Join(dir, "plain") + "\n"; stdout != want {
			t.Errorf("Run() stdout = %q, want %q", stdout, want)
		}
	})
}
