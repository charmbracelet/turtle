package csplit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

// cancelReader cancels its context on the first read, simulating a context
// that is done while input is being read.
type cancelReader struct {
	cancel context.CancelFunc
}

func (r *cancelReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
}

// failWriteFS wraps a MemFS so writes to created files fail.
type failWriteFS struct {
	fsx.MemFS
	err error
}

// OpenFile implements fsx.WriteFS.
func (m failWriteFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	f, err := m.MemFS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return failWriteFile{File: f, err: m.err}, nil
}

type failWriteFile struct {
	fsx.File
	err error
}

func (f failWriteFile) Write([]byte) (int, error) {
	return 0, f.err
}

type testCase struct {
	name      string
	stdin     io.Reader
	fsys      fsx.FS
	args      []string
	want      map[string]string
	wantOut   string
	wantErr   string
	wantExit  int
	wantFiles []string
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	fsys := test.fsys
	if fsys == nil {
		fsys = fsx.MemFS{}
	}
	before := map[string]bool{}
	if mem, ok := fsys.(fsx.MemFS); ok {
		for name := range mem {
			before[name] = true
		}
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  test.stdin,
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsys,
	}
	err := New(rt).Run(t.Context(), test.args...)

	code := 0
	if exitErr, ok := errors.AsType[*coreutils.ExitError](err); ok {
		code = exitErr.Code
	} else if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.wantOut {
		t.Errorf("Run() stdout = %q, want %q", got, test.wantOut)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}

	mem, ok := fsys.(fsx.MemFS)
	if !ok {
		return
	}
	var got []string
	for name := range mem {
		got = append(got, name)
	}
	sort.Strings(got)
	want := append([]string{}, test.wantFiles...)
	if test.wantFiles == nil {
		for name := range test.want {
			want = append(want, name)
		}
		for name := range before {
			if _, ok := test.want[name]; !ok {
				want = append(want, name)
			}
		}
	}
	sort.Strings(want)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Run() files = %v, want %v", got, want)
	}
	for name, content := range test.want {
		file := mem[name]
		if file == nil {
			t.Errorf("Run() file %s missing", name)
			continue
		}
		if got := string(file.Data); got != content {
			t.Errorf("Run() file %s = %q, want %q", name, got, content)
		}
	}
}

func TestRunLineNumbers(t *testing.T) {
	tests := []testCase{
		{
			name:      "a line number splits off the rest",
			fsys:      fsx.MemFS{"f": {Data: []byte("a\nb\nc\nd\n")}},
			args:      []string{"f", "3"},
			wantOut:   "4\n4\n",
			wantFiles: []string{"f", "xx00", "xx01"},
			want: map[string]string{
				"xx00": "a\nb\n",
				"xx01": "c\nd\n",
			},
		},
		{
			name:      "line one creates an empty first piece",
			fsys:      fsx.MemFS{"f": {Data: []byte("a\nb\n")}},
			args:      []string{"f", "1"},
			wantOut:   "0\n4\n",
			wantFiles: []string{"f", "xx00", "xx01"},
			want: map[string]string{
				"xx00": "",
				"xx01": "a\nb\n",
			},
		},
		{
			name:      "the last line number leaves the last line for the rest",
			fsys:      fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			args:      []string{"f", "3"},
			wantOut:   "4\n2\n",
			wantFiles: []string{"f", "xx00", "xx01"},
			want: map[string]string{
				"xx00": "a\nb\n",
				"xx01": "c\n",
			},
		},
		{
			name:    "silent mode suppresses the sizes",
			fsys:    fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			args:    []string{"-s", "f", "2"},
			wantOut: "",
			want: map[string]string{
				"f":    "a\nb\nc\n",
				"xx00": "a\n",
				"xx01": "b\nc\n",
			},
		},
		{
			name:    "custom prefix and digit count",
			fsys:    fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			args:    []string{"-f", "part", "-n", "3", "f", "2"},
			wantOut: "2\n4\n",
			want: map[string]string{
				"f":       "a\nb\nc\n",
				"part000": "a\n",
				"part001": "b\nc\n",
			},
		},
		{
			name:      "numbers wider than the digit count still work",
			fsys:      fsx.MemFS{"f": {Data: []byte(lines(1, 24))}},
			args:      []string{"-s", "-n", "1", "f", "2", "{11}"},
			wantOut:   "",
			wantFiles: append([]string{"f"}, names("xx", 0, 12)...),
		},
		{
			name:      "input may come from the standard input",
			stdin:     strings.NewReader("a\nb\nc\n"),
			args:      []string{"-", "2"},
			wantOut:   "2\n4\n",
			wantFiles: []string{"xx00", "xx01"},
			want: map[string]string{
				"xx00": "a\n",
				"xx01": "b\nc\n",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRegexps(t *testing.T) {
	tests := []testCase{
		{
			name:    "a regexp ends the piece before the matching line",
			fsys:    fsx.MemFS{"f": {Data: []byte("a\nb\nEND\nd\n")}},
			args:    []string{"f", "/END/"},
			wantOut: "4\n6\n",
			want: map[string]string{
				"xx00": "a\nb\n",
				"xx01": "END\nd\n",
			},
		},
		{
			name:    "the first expression may match the first line",
			fsys:    fsx.MemFS{"f": {Data: []byte("END\nb\nEND\nd\n")}},
			args:    []string{"f", "/END/", "/END/"},
			wantOut: "0\n6\n6\n",
			want: map[string]string{
				"xx00": "",
				"xx01": "END\nb\n",
				"xx02": "END\nd\n",
			},
		},
		{
			name:    "a positive offset includes lines past the match",
			fsys:    fsx.MemFS{"f": {Data: []byte("a\nEND\nc\nd\n")}},
			args:    []string{"f", "/END/+1"},
			wantOut: "6\n4\n",
			want: map[string]string{
				"xx00": "a\nEND\n",
				"xx01": "c\nd\n",
			},
		},
		{
			name:    "a negative offset ends the piece before the match",
			fsys:    fsx.MemFS{"f": {Data: []byte("a\nb\nEND\nd\ne\n")}},
			args:    []string{"f", "/END/-1"},
			wantOut: "2\n10\n",
			want: map[string]string{
				"xx00": "a\n",
				"xx01": "b\nEND\nd\ne\n",
			},
		},
		{
			name:    "a percent expression skips lines without a file",
			fsys:    fsx.MemFS{"f": {Data: []byte("skip\nthis\nEND\nkeep\n")}},
			args:    []string{"f", "%END%"},
			wantOut: "9\n",
			want: map[string]string{
				"xx00": "END\nkeep\n",
			},
		},
		{
			name:    "the search starts after the current line",
			fsys:    fsx.MemFS{"f": {Data: []byte("a\nb\nc\nEND\ne\nf\nEND\nh\n")}},
			args:    []string{"f", "5", "/END/"},
			wantOut: "10\n4\n6\n",
			want: map[string]string{
				"xx00": "a\nb\nc\nEND\n",
				"xx01": "e\nf\n",
				"xx02": "END\nh\n",
			},
		},
		{
			name:    "repeats reapply the search",
			fsys:    fsx.MemFS{"f": {Data: []byte("a\nE\nb\nE\nc\n")}},
			args:    []string{"f", "/^E$/", "{1}"},
			wantOut: "2\n4\n4\n",
			want: map[string]string{
				"xx00": "a\n",
				"xx01": "E\nb\n",
				"xx02": "E\nc\n",
			},
		},
		{
			name:    "basic regular expression features",
			fsys:    fsx.MemFS{"f": {Data: []byte("int x;\n\nint main()\n{\n}\n")}},
			args:    []string{"f", `/^$/`, `/^{/`},
			wantOut: "7\n12\n4\n",
			want: map[string]string{
				"xx00": "int x;\n",
				"xx01": "\nint main()\n",
				"xx02": "{\n}\n",
			},
		},
		{
			name:    "an escaped delimiter is part of the expression",
			fsys:    fsx.MemFS{"f": {Data: []byte("a\nb/c\nd\n")}},
			args:    []string{"f", `/b\/c/`},
			wantOut: "2\n6\n",
			want: map[string]string{
				"xx00": "a\n",
				"xx01": "b/c\nd\n",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRepeats(t *testing.T) {
	tests := []testCase{
		{
			name:    "a line number repeat splits evenly",
			fsys:    fsx.MemFS{"f": {Data: []byte("a\nb\nc\nd\ne\nf\ng\n")}},
			args:    []string{"f", "2", "{2}"},
			wantOut: "2\n4\n4\n4\n",
			want: map[string]string{
				"xx00": "a\n",
				"xx01": "b\nc\n",
				"xx02": "d\ne\n",
				"xx03": "f\ng\n",
			},
		},
		{
			name:    "a repeat of zero applies the operand once",
			fsys:    fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			args:    []string{"f", "2", "{0}"},
			wantOut: "2\n4\n",
			want: map[string]string{
				"xx00": "a\n",
				"xx01": "b\nc\n",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "missing operand",
			args:     nil,
			wantErr:  "csplit: missing operand\n",
			wantExit: 1,
		},
		{
			name:     "missing pattern",
			args:     []string{"f"},
			fsys:     fsx.MemFS{"f": {Data: []byte("a\n")}},
			wantErr:  "csplit: missing operand after 'f'\n",
			wantExit: 1,
		},
		{
			name:     "unknown option",
			args:     []string{"-q", "f", "1"},
			wantErr:  "csplit: invalid option -- 'q'\n",
			wantExit: 1,
		},
		{
			name:     "invalid digit count",
			args:     []string{"-n", "x", "f", "1"},
			wantErr:  "csplit: invalid number: 'x'\n",
			wantExit: 1,
		},
		{
			name:     "missing input file",
			fsys:     fsx.MemFS{},
			args:     []string{"missing", "1"},
			wantErr:  "csplit: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a line number past the end of the file",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			args:     []string{"f", "2", "10"},
			wantOut:  "2\n4\n",
			wantErr:  "csplit: '10': line number out of range\n",
			wantExit: 1,
		},
		{
			name:     "a line number before the current line",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			args:     []string{"f", "3", "1"},
			wantOut:  "4\n",
			wantErr:  "csplit: '1': line number out of range\n",
			wantExit: 1,
		},
		{
			name:     "a regexp without a match",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			args:     []string{"f", "/NOPE/"},
			wantOut:  "6\n",
			wantErr:  "csplit: '/NOPE/': match not found\n",
			wantExit: 1,
		},
		{
			name:     "a regexp without a match on a repetition",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\nE\nb\nc\n")}},
			args:     []string{"f", "/^E$/", "{2}"},
			wantOut:  "2\n6\n",
			wantErr:  "csplit: '/^E$/': match not found on repetition 1\n",
			wantExit: 1,
		},
		{
			name:     "an offset past the end of the file",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			args:     []string{"f", "/c/+5"},
			wantOut:  "6\n",
			wantErr:  "csplit: '/c/+5': line number out of range\n",
			wantExit: 1,
		},
		{
			name:     "an offset before the current line",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\nb\nc\nd\n")}},
			args:     []string{"f", "3", "/d/-3"},
			wantOut:  "4\n",
			wantErr:  "csplit: '/d/-3': line number out of range\n",
			wantExit: 1,
		},
		{
			name:     "an invalid regular expression",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\n")}},
			args:     []string{"f", "/[/"},
			wantErr:  "csplit: '/[/': unmatched [\n",
			wantExit: 1,
		},
		{
			name:     "a missing closing delimiter",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\n")}},
			args:     []string{"f", "/abc"},
			wantErr:  "csplit: /abc: closing delimiter '/' missing\n",
			wantExit: 1,
		},
		{
			name:     "a missing repeat count brace",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\n")}},
			args:     []string{"f", "1", "{2"},
			wantErr:  "csplit: '{2': '}' is required in repeat count\n",
			wantExit: 1,
		},
		{
			name:     "an invalid repeat count",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\n")}},
			args:     []string{"f", "1", "{x}"},
			wantErr:  "csplit: '{x}': integer required between '{' and '}'\n",
			wantExit: 1,
		},
		{
			name:     "a stray repeat count",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\n")}},
			args:     []string{"f", "{2}"},
			wantErr:  "csplit: '{2}': invalid pattern\n",
			wantExit: 1,
		},
		{
			name:     "an invalid pattern",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\n")}},
			args:     []string{"f", "abc"},
			wantErr:  "csplit: 'abc': invalid pattern\n",
			wantExit: 1,
		},
		{
			name:     "a zero line number",
			fsys:     fsx.MemFS{"f": {Data: []byte("a\n")}},
			args:     []string{"f", "0"},
			wantErr:  "csplit: 0: line number must be greater than zero\n",
			wantExit: 1,
		},
		{
			name:     "read error",
			stdin:    errorReader{err: errors.New("boom")},
			args:     []string{"-", "1"},
			wantErr:  "csplit: -: boom\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrorKeepsFiles(t *testing.T) {
	t.Run("created files are removed on error by default", func(t *testing.T) {
		files := fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}}
		runCase(t, testCase{
			fsys:      files,
			args:      []string{"f", "2", "10"},
			wantOut:   "2\n4\n",
			wantErr:   "csplit: '10': line number out of range\n",
			wantExit:  1,
			wantFiles: []string{"f"},
		})
	})

	t.Run("-k keeps the files created", func(t *testing.T) {
		files := fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}}
		runCase(t, testCase{
			fsys:      files,
			args:      []string{"-k", "f", "2", "10"},
			wantOut:   "2\n4\n",
			wantErr:   "csplit: '10': line number out of range\n",
			wantExit:  1,
			wantFiles: []string{"f", "xx00", "xx01"},
			want: map[string]string{
				"xx00": "a\n",
				"xx01": "b\nc\n",
			},
		})
	})

	t.Run("-k keeps the partial file of a failed match", func(t *testing.T) {
		files := fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}}
		runCase(t, testCase{
			fsys:      files,
			args:      []string{"-k", "f", "/NOPE/"},
			wantOut:   "6\n",
			wantErr:   "csplit: '/NOPE/': match not found\n",
			wantExit:  1,
			wantFiles: []string{"f", "xx00"},
			want: map[string]string{
				"xx00": "a\nb\nc\n",
			},
		})
	})

	t.Run("write error", func(t *testing.T) {
		runCase(t, testCase{
			fsys:     failWriteFS{MemFS: fsx.MemFS{}, err: errors.New("disk full")},
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"-", "1"},
			wantOut:  "0\n",
			wantErr:  "csplit: xx01: disk full\n",
			wantExit: 1,
		})
	})
}

func TestRunCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	prog := New(&coreutils.Runtime{
		Stdin:  &cancelReader{cancel: cancel},
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     fsx.MemFS{},
	})
	if err := prog.Run(ctx, "-", "1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func lines(from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&b, "l%d\n", i)
	}
	return b.String()
}

func names(prefix string, from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf("%s%d", prefix, i))
	}
	return out
}
