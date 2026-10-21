package pwd

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
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

func lookupEnv(env map[string]string) coreutils.LookupEnvFunc {
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}

func runPwd(t *testing.T, workDir string, env map[string]string, args ...string) (string, error) {
	t.Helper()

	stdin := &spyReader{}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	rt := &coreutils.Runtime{
		Stdin:      stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		WorkingDir: workDir,
		LookupEnv:  lookupEnv(env),
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
	base := t.TempDir()
	physical := filepath.Join(base, "physical")
	logical := filepath.Join(base, "logical")
	sep := string(filepath.Separator)

	t.Run("success", func(t *testing.T) {
		tests := []struct {
			name    string
			workDir string
			env     map[string]string
			args    []string
			want    string
		}{
			{
				name:    "default uses a valid PWD",
				workDir: physical,
				env:     map[string]string{"PWD": logical},
				want:    logical,
			},
			{
				name:    "L uses a valid PWD",
				workDir: physical,
				env:     map[string]string{"PWD": logical},
				args:    []string{"-L"},
				want:    logical,
			},
			{
				name:    "a relative PWD is rejected",
				workDir: physical,
				env:     map[string]string{"PWD": filepath.Join("relative", "path")},
				want:    physical,
			},
			{
				name:    "a PWD with a dot component is rejected",
				workDir: physical,
				env:     map[string]string{"PWD": base + sep + "." + sep + "foo"},
				want:    physical,
			},
			{
				name:    "a PWD with a dot-dot component is rejected",
				workDir: physical,
				env:     map[string]string{"PWD": base + sep + "foo" + sep + ".." + sep + "bar"},
				want:    physical,
			},
			{
				name:    "a PWD with a trailing slash is kept",
				workDir: physical,
				env:     map[string]string{"PWD": logical + sep},
				want:    logical + sep,
			},
			{
				name:    "an unset PWD falls back to the working directory",
				workDir: physical,
				want:    physical,
			},
			{
				name:    "an empty PWD falls back to the working directory",
				workDir: physical,
				env:     map[string]string{"PWD": ""},
				want:    physical,
			},
			{
				name:    "the working directory is cleaned",
				workDir: base + sep + "bar" + sep + ".." + sep + "baz",
				want:    filepath.Join(base, "baz"),
			},
			{
				name:    "P ignores a valid PWD",
				workDir: physical,
				env:     map[string]string{"PWD": logical},
				args:    []string{"-P"},
				want:    physical,
			},
			{
				name:    "P cleans the working directory",
				workDir: base + sep + "bar" + sep + ".." + sep + "baz",
				args:    []string{"-P"},
				want:    filepath.Join(base, "baz"),
			},
			{
				name:    "the last option wins: LP",
				workDir: physical,
				env:     map[string]string{"PWD": logical},
				args:    []string{"-LP"},
				want:    physical,
			},
			{
				name:    "the last option wins: PL",
				workDir: physical,
				env:     map[string]string{"PWD": logical},
				args:    []string{"-PL"},
				want:    logical,
			},
			{
				name:    "repeated options",
				workDir: physical,
				env:     map[string]string{"PWD": logical},
				args:    []string{"-P", "-L"},
				want:    logical,
			},
			{
				name:    "logical uses a valid PWD",
				workDir: physical,
				env:     map[string]string{"PWD": logical},
				args:    []string{"--logical"},
				want:    logical,
			},
			{
				name:    "physical ignores a valid PWD",
				workDir: physical,
				env:     map[string]string{"PWD": logical},
				args:    []string{"--physical"},
				want:    physical,
			},
			{
				name:    "the last option wins: physical logical",
				workDir: physical,
				env:     map[string]string{"PWD": logical},
				args:    []string{"--physical", "--logical"},
				want:    logical,
			},
			{
				name:    "a lone double dash is accepted",
				workDir: physical,
				args:    []string{"--"},
				want:    physical,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runPwd(t, test.workDir, test.env, test.args...)
				if err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				want := filepath.ToSlash(test.want) + "\n"
				if got != want {
					t.Errorf("Run() stdout = %q, want %q", got, want)
				}
			})
		}
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			workDir string
			args    []string
			want    int
		}{
			{
				name: "an operand is rejected",
				args: []string{"foo"},
				want: 2,
			},
			{
				name: "an operand after a double dash is rejected",
				args: []string{"--", "foo"},
				want: 2,
			},
			{
				name: "an unknown option is rejected",
				args: []string{"-X"},
				want: 2,
			},
			{
				name: "an unknown option in a group is rejected",
				args: []string{"-LPX"},
				want: 2,
			},
			{
				name: "a long option is rejected",
				args: []string{"--help"},
				want: 2,
			},
			{
				name: "an unrecognized long option is rejected",
				args: []string{"--bogus"},
				want: 2,
			},
			{
				name: "a long option with a value it doesn't take is rejected",
				args: []string{"--logical=yes"},
				want: 2,
			},
			{
				name: "a lone dash is rejected",
				args: []string{"-"},
				want: 2,
			},
			{
				name: "an empty argument is rejected",
				args: []string{""},
				want: 2,
			},
			{
				name: "an empty working directory is an error",
				want: 1,
			},
			{
				name:    "a relative working directory is an error",
				workDir: filepath.Join("relative", "path"),
				want:    1,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got, err := runPwd(t, test.workDir, nil, test.args...)
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

func TestRunNilLookupEnv(t *testing.T) {
	workDir := t.TempDir()

	rt := &coreutils.Runtime{
		Stdin:      &spyReader{},
		Stdout:     new(bytes.Buffer),
		Stderr:     new(bytes.Buffer),
		WorkingDir: workDir,
	}
	if err := New(rt).Run(t.Context()); err != nil {
		t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
	}
	want := filepath.ToSlash(workDir) + "\n"
	if got := rt.Stdout.(*bytes.Buffer).String(); got != want {
		t.Errorf("Run() stdout = %q, want %q", got, want)
	}
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	rt := &coreutils.Runtime{
		Stdin:      &spyReader{},
		Stdout:     errorWriter{err: writeErr},
		Stderr:     new(bytes.Buffer),
		WorkingDir: t.TempDir(),
		LookupEnv:  lookupEnv(nil),
	}
	err := New(rt).Run(t.Context())

	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
}
