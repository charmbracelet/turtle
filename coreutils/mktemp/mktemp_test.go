package mktemp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

type testCase struct {
	name       string
	files      fsx.MemFS
	umask      fs.FileMode
	env        map[string]string
	workingDir string
	args       []string

	wantName   string      // regexp the printed name must match
	wantMode   fs.FileMode // mode of the entry the name points to
	wantAbsent bool        // the printed name must not exist
	wantErr    string      // regexp the diagnostic must match
	wantExit   int         // expected exit status
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         test.files,
		Umask:      test.umask,
		WorkingDir: test.workingDir,
		Environ:    func() []string { return environ(test.env) },
	}

	before := keys(test.files)
	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	got := strings.TrimSuffix(stderr.String(), "\n")
	switch {
	case test.wantErr == "" && got != "":
		t.Errorf("Run() stderr = %q, want empty", got)
	case test.wantErr != "":
		if !regexp.MustCompile(test.wantErr).MatchString(got) {
			t.Errorf("Run() stderr = %q, want match %q", got, test.wantErr)
		}
	}

	name, ok := strings.CutSuffix(stdout.String(), "\n")
	switch {
	case test.wantName == "" && stdout.Len() != 0:
		t.Errorf("Run() stdout = %q, want empty", stdout.String())
	case test.wantName != "" && !ok:
		t.Errorf("Run() stdout = %q, want a single line", stdout.String())
	case test.wantName != "" && !regexp.MustCompile(test.wantName).MatchString(name):
		t.Errorf("Run() stdout = %q, want match %q", name, test.wantName)
	}
	if test.wantName == "" {
		return
	}

	fspath := rt.ResolvePath(name)
	fi, statErr := fs.Stat(test.files, fspath)
	if test.wantAbsent {
		if !errors.Is(statErr, fs.ErrNotExist) {
			t.Errorf("Stat(%q) error = %v, want fs.ErrNotExist", fspath, statErr)
		}
		if after := keys(test.files); !slices.Equal(before, after) {
			t.Errorf("entries changed from %v to %v", before, after)
		}
		return
	}
	if statErr != nil {
		t.Fatalf("Stat(%q) error = %v, want it to exist", fspath, statErr)
	}
	if got, want := fi.Mode(), test.wantMode; got != want {
		t.Errorf("%s mode = %v, want %v", fspath, got, want)
	}
	if fi.IsDir() != test.wantMode.IsDir() {
		t.Errorf("%s is a directory = %v, want %v", fspath, fi.IsDir(), test.wantMode.IsDir())
	}
	if !fi.IsDir() && fi.Size() != 0 {
		t.Errorf("%s size = %d, want 0", fspath, fi.Size())
	}
}

func keys(files fsx.MemFS) []string {
	return slices.Sorted(maps.Keys(files))
}

// environ returns the test environment as a list of KEY=VALUE strings.
func environ(values map[string]string) []string {
	entries := make([]string, 0, len(values))
	for name, value := range values {
		entries = append(entries, name+"="+value)
	}
	return entries
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

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			name:     "creates a file in the current directory",
			files:    fsx.MemFS{},
			args:     []string{"file.XXXX"},
			wantName: `^file\.[A-Za-z0-9]{4}$`,
			wantMode: 0o600,
		},
		{
			name:     "replaces the whole final run of X's",
			files:    fsx.MemFS{},
			args:     []string{"x-XXXXXXXXXX"},
			wantName: `^x-[A-Za-z0-9]{10}$`,
			wantMode: 0o600,
		},
		{
			name:     "keeps the X's before the final run",
			files:    fsx.MemFS{},
			args:     []string{"file-XXXX-XXXX.txt"},
			wantName: `^file-XXXX-[A-Za-z0-9]{4}\.txt$`,
			wantMode: 0o600,
		},
		{
			name:     "the default template goes to the temporary directory",
			files:    fsx.MemFS{"td/keep": {Data: []byte("keep")}},
			env:      map[string]string{"TMPDIR": "td"},
			args:     nil,
			wantName: `^td/tmp\.[A-Za-z0-9]{10}$`,
			wantMode: 0o600,
		},
		{
			name:     "a template after a double dash",
			files:    fsx.MemFS{},
			args:     []string{"--", "file.XXXX"},
			wantName: `^file\.[A-Za-z0-9]{4}$`,
			wantMode: 0o600,
		},
		{
			name:     "the umask reduces the file mode",
			files:    fsx.MemFS{},
			umask:    0o200,
			args:     []string{"f.XXXX"},
			wantName: `^f\.[A-Za-z0-9]{4}$`,
			wantMode: 0o400,
		},
		{
			name:     "a final run of less than three X's",
			files:    fsx.MemFS{},
			args:     []string{"XX"},
			wantErr:  `^mktemp: too few X's in template 'XX'$`,
			wantExit: 1,
		},
		{
			name:     "the X's must be in the last component",
			files:    fsx.MemFS{},
			args:     []string{"XXXX/dir"},
			wantErr:  `^mktemp: too few X's in template 'XXXX/dir'$`,
			wantExit: 1,
		},
		{
			name:     "no X's at all",
			files:    fsx.MemFS{},
			args:     []string{"file.txt"},
			wantErr:  `^mktemp: too few X's in template 'file\.txt'$`,
			wantExit: 1,
		},
		{
			name:     "an explicitly empty template",
			files:    fsx.MemFS{},
			args:     []string{""},
			wantErr:  `^mktemp: too few X's in template ''$`,
			wantExit: 1,
		},
		{
			name:     "an explicitly empty template with a suffix",
			files:    fsx.MemFS{},
			args:     []string{"--suffix=.txt", ""},
			wantErr:  `^mktemp: with --suffix, template '' must end in X$`,
			wantExit: 1,
		},
		{
			name:     "too many operands",
			files:    fsx.MemFS{},
			args:     []string{"a.XXXX", "b.XXXX"},
			wantErr:  `^mktemp: extra operand 'b\.XXXX'$`,
			wantExit: 2,
		},
		{
			name:     "an invalid option",
			files:    fsx.MemFS{},
			args:     []string{"-z"},
			wantErr:  `^mktemp: invalid option -- 'z'$`,
			wantExit: 2,
		},
		{
			name:     "an unrecognized long option",
			files:    fsx.MemFS{},
			args:     []string{"--bogus"},
			wantErr:  `^mktemp: unrecognized option '--bogus'$`,
			wantExit: 2,
		},
		{
			name:     "-p missing its argument",
			files:    fsx.MemFS{},
			args:     []string{"-p"},
			wantErr:  `^mktemp: option requires an argument -- 'p'$`,
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDirectory(t *testing.T) {
	tests := []testCase{
		{
			name:     "creates a directory",
			files:    fsx.MemFS{},
			args:     []string{"-d", "d.XXX"},
			wantName: `^d\.[A-Za-z0-9]{3}$`,
			wantMode: fs.ModeDir | 0o700,
		},
		{
			name:     "the long form",
			files:    fsx.MemFS{},
			args:     []string{"--directory", "d.XXX"},
			wantName: `^d\.[A-Za-z0-9]{3}$`,
			wantMode: fs.ModeDir | 0o700,
		},
		{
			name:     "the umask reduces the directory mode",
			files:    fsx.MemFS{},
			umask:    0o100,
			args:     []string{"-d", "d.XXX"},
			wantName: `^d\.[A-Za-z0-9]{3}$`,
			wantMode: fs.ModeDir | 0o600,
		},
		{
			name:     "with the default template",
			files:    fsx.MemFS{"td/keep": {Data: []byte("keep")}},
			env:      map[string]string{"TMPDIR": "td"},
			args:     []string{"-d"},
			wantName: `^td/tmp\.[A-Za-z0-9]{10}$`,
			wantMode: fs.ModeDir | 0o700,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDryRun(t *testing.T) {
	tests := []testCase{
		{
			name:       "prints a name without creating anything",
			files:      fsx.MemFS{},
			args:       []string{"-u", "file.XXX"},
			wantName:   `^file\.[A-Za-z0-9]{3}$`,
			wantAbsent: true,
		},
		{
			name:       "the long form",
			files:      fsx.MemFS{},
			args:       []string{"--dry-run", "file.XXX"},
			wantName:   `^file\.[A-Za-z0-9]{3}$`,
			wantAbsent: true,
		},
		{
			name:       "with the default template",
			files:      fsx.MemFS{"td/keep": {Data: []byte("keep")}},
			env:        map[string]string{"TMPDIR": "td"},
			args:       []string{"-u"},
			wantName:   `^td/tmp\.[A-Za-z0-9]{10}$`,
			wantAbsent: true,
		},
		{
			name:       "keeps the entries of the file system",
			files:      fsx.MemFS{"file.txt": {Data: []byte("keep")}},
			args:       []string{"-u", "file.XXX"},
			wantName:   `^file\.[A-Za-z0-9]{3}$`,
			wantAbsent: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSuffix(t *testing.T) {
	tests := []testCase{
		{
			name:     "an attached value",
			files:    fsx.MemFS{},
			args:     []string{"--suffix=.txt", "file-XXXX"},
			wantName: `^file-[A-Za-z0-9]{4}\.txt$`,
			wantMode: 0o600,
		},
		{
			name:     "a value in the next argument",
			files:    fsx.MemFS{},
			args:     []string{"--suffix", ".txt", "file-XXXX"},
			wantName: `^file-[A-Za-z0-9]{4}\.txt$`,
			wantMode: 0o600,
		},
		{
			name:     "the default template with a suffix",
			files:    fsx.MemFS{"td/keep": {Data: []byte("keep")}},
			env:      map[string]string{"TMPDIR": "td"},
			args:     []string{"--suffix=.txt"},
			wantName: `^td/tmp\.[A-Za-z0-9]{10}\.txt$`,
			wantMode: 0o600,
		},
		{
			name:     "the X's of the suffix stay literal",
			files:    fsx.MemFS{},
			args:     []string{"--suffix=X", "file.XXX"},
			wantName: `^file\.[A-Za-z0-9]{3}X$`,
			wantMode: 0o600,
		},
		{
			name:     "a suffix that starts with X",
			files:    fsx.MemFS{},
			args:     []string{"--suffix=Xyz", "f.XXX"},
			wantName: `^f\.[A-Za-z0-9]{3}Xyz$`,
			wantMode: 0o600,
		},
		{
			name:     "the template must end in X",
			files:    fsx.MemFS{},
			args:     []string{"--suffix=.txt", "file.txt"},
			wantErr:  `^mktemp: with --suffix, template 'file\.txt' must end in X$`,
			wantExit: 1,
		},
		{
			name:     "the suffix must not contain a slash",
			files:    fsx.MemFS{},
			args:     []string{"--suffix=a/b", "f.XXXX"},
			wantErr:  `^mktemp: invalid suffix 'a/b', contains directory separator$`,
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunTmpDir(t *testing.T) {
	tests := []testCase{
		{
			name:     "-p with the directory in the next argument",
			files:    fsx.MemFS{"sub/keep": {Data: []byte("keep")}},
			args:     []string{"-p", "sub", "t.XXXX"},
			wantName: `^sub/t\.[A-Za-z0-9]{4}$`,
			wantMode: 0o600,
		},
		{
			name:     "-p with the directory glued to the option",
			files:    fsx.MemFS{"sub/keep": {Data: []byte("keep")}},
			args:     []string{"-psub", "t.XXXX"},
			wantName: `^sub/t\.[A-Za-z0-9]{4}$`,
			wantMode: 0o600,
		},
		{
			name:     "--tmpdir with an attached directory",
			files:    fsx.MemFS{"sub/keep": {Data: []byte("keep")}},
			args:     []string{"--tmpdir=sub", "t.XXXX"},
			wantName: `^sub/t\.[A-Za-z0-9]{4}$`,
			wantMode: 0o600,
		},
		{
			name:     "--tmpdir without a directory uses TMPDIR",
			files:    fsx.MemFS{"td/keep": {Data: []byte("keep")}},
			env:      map[string]string{"TMPDIR": "td"},
			args:     []string{"--tmpdir", "t.XXXX"},
			wantName: `^td/t\.[A-Za-z0-9]{4}$`,
			wantMode: 0o600,
		},
		{
			name:     "-p with an empty directory uses TMPDIR",
			files:    fsx.MemFS{"td/keep": {Data: []byte("keep")}},
			env:      map[string]string{"TMPDIR": "td"},
			args:     []string{"-p", "", "t.XXXX"},
			wantName: `^td/t\.[A-Za-z0-9]{4}$`,
			wantMode: 0o600,
		},
		{
			name:     "--tmpdir without TMPDIR falls back to /tmp",
			files:    fsx.MemFS{},
			args:     []string{"--tmpdir", "t.XXXX"},
			wantErr:  `^mktemp: failed to create file via template '/tmp/t\.[A-Za-z0-9]{4}': invalid argument$`,
			wantExit: 1,
		},
		{
			name:     "an absolute template is rejected",
			files:    fsx.MemFS{"sub/keep": {Data: []byte("keep")}},
			args:     []string{"-p", "sub", "/tmp/t.XXXX"},
			wantErr:  `^mktemp: template '/tmp/t\.XXXX' must not be absolute$`,
			wantExit: 1,
		},
		{
			name:     "an absolute template is rejected with --tmpdir",
			files:    fsx.MemFS{"sub/keep": {Data: []byte("keep")}},
			args:     []string{"--tmpdir=sub", "/tmp/t.XXXX"},
			wantErr:  `^mktemp: template '/tmp/t\.XXXX' must not be absolute$`,
			wantExit: 1,
		},
		{
			name:     "a missing directory is an error",
			files:    fsx.MemFS{},
			args:     []string{"-p", "missing", "t.XXXX"},
			wantErr:  `^mktemp: failed to create file via template 'missing/t\.[A-Za-z0-9]{4}': file does not exist$`,
			wantExit: 1,
		},
		{
			name:     "a missing directory with -d is an error",
			files:    fsx.MemFS{},
			args:     []string{"-d", "-p", "missing", "t.XXXX"},
			wantErr:  `^mktemp: failed to create directory via template 'missing/t\.[A-Za-z0-9]{4}': file does not exist$`,
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDeprecated(t *testing.T) {
	tests := []testCase{
		{
			name:     "the template is relative to TMPDIR",
			files:    fsx.MemFS{"td/keep": {Data: []byte("keep")}},
			env:      map[string]string{"TMPDIR": "td"},
			args:     []string{"-t", "n.XXXX"},
			wantName: `^td/n\.[A-Za-z0-9]{4}$`,
			wantMode: 0o600,
		},
		{
			name:     "TMPDIR wins over -p",
			files:    fsx.MemFS{"td/keep": {Data: []byte("keep")}},
			env:      map[string]string{"TMPDIR": "td"},
			args:     []string{"-t", "-p", "sub", "n.XXX"},
			wantName: `^td/n\.[A-Za-z0-9]{3}$`,
			wantMode: 0o600,
		},
		{
			name:     "falls back to -p without TMPDIR",
			files:    fsx.MemFS{"sub/keep": {Data: []byte("keep")}},
			args:     []string{"-t", "-p", "sub", "n.XXX"},
			wantName: `^sub/n\.[A-Za-z0-9]{3}$`,
			wantMode: 0o600,
		},
		{
			name:     "the template must not contain a slash",
			files:    fsx.MemFS{},
			args:     []string{"-t", "a/n.XXXX"},
			wantErr:  `^mktemp: invalid template 'a/n\.XXXX', contains directory separator$`,
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunQuiet(t *testing.T) {
	tests := []testCase{
		{
			name:     "suppresses the diagnostic of a bad template",
			files:    fsx.MemFS{},
			args:     []string{"-q", "XX"},
			wantExit: 1,
		},
		{
			name:     "suppresses the diagnostic of a failure",
			files:    fsx.MemFS{},
			args:     []string{"-q", "-p", "missing", "t.XXXX"},
			wantExit: 1,
		},
		{
			name:     "the long form",
			files:    fsx.MemFS{},
			args:     []string{"--quiet", "-p", "missing", "t.XXXX"},
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunWorkingDir(t *testing.T) {
	tests := []testCase{
		{
			name:       "a relative template is created in the working directory",
			files:      fsx.MemFS{"base/keep": {Data: []byte("keep")}},
			workingDir: "base",
			args:       []string{"f.XXXX"},
			wantName:   `^f\.[A-Za-z0-9]{4}$`,
			wantMode:   0o600,
		},
		{
			name:       "a relative -p directory is resolved in the working directory",
			files:      fsx.MemFS{"base/sub/keep": {Data: []byte("keep")}},
			workingDir: "base",
			args:       []string{"-p", "sub", "t.XXXX"},
			wantName:   `^sub/t\.[A-Za-z0-9]{4}$`,
			wantMode:   0o600,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsx.MemFS{},
	}

	err := New(rt).Run(ctx, "file.XXXX")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
}

func TestTempDir(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "TMPDIR", env: map[string]string{"TMPDIR": "d"}, want: "d"},
		{name: "TMPDIR wins over TEMP and TMP", env: map[string]string{"TMPDIR": "d", "TEMP": "w", "TMP": "t"}, want: "d"},
		{name: "TEMP", env: map[string]string{"TEMP": "w"}, want: "w"},
		{name: "TMP", env: map[string]string{"TMP": "t"}, want: "t"},
		{name: "none", env: map[string]string{}, want: "/tmp"},
		{name: "no environment", env: nil, want: "/tmp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := &Prog{Runtime: &coreutils.Runtime{Environ: func() []string { return environ(test.env) }}}
			if got := p.tempDir(); got != test.want {
				t.Errorf("tempDir() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestXRun(t *testing.T) {
	tests := []struct {
		tmpl  string
		first int
		end   int
	}{
		{tmpl: "tmp.XXXXXXXXXX", first: 4, end: 14},
		{tmpl: "file-XXXX-XXXX.txt", first: 10, end: 14},
		{tmpl: "XXXX/dir", first: 0, end: 4},
		{tmpl: "file.txt", first: 0, end: 0},
		{tmpl: "", first: 0, end: 0},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%q", test.tmpl), func(t *testing.T) {
			first, end := xRun(test.tmpl)
			if first != test.first || end != test.end {
				t.Errorf("xRun(%q) = (%d, %d), want (%d, %d)", test.tmpl, first, end, test.first, test.end)
			}
		})
	}
}
