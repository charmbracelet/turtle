package tac

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

// cancelReader cancels its context on the first read, simulating a context
// that is done while the input is being read.
type cancelReader struct {
	cancel context.CancelFunc
}

func (r *cancelReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
}

// cancelWriter cancels its context on the first write, simulating a context
// that is done between records.
type cancelWriter struct {
	cancel context.CancelFunc
}

func (w *cancelWriter) Write(b []byte) (int, error) {
	w.cancel()
	return len(b), nil
}

// readerFS is an fs.FS whose only file reads from a reader, making the input
// fail to read.
type readerFS struct {
	r io.Reader
}

func (f readerFS) Open(string) (fs.File, error) {
	return readerFile{f.r}, nil
}

func (readerFS) OpenFile(string, int, fs.FileMode) (fsx.File, error) {
	return nil, fs.ErrInvalid
}

func (readerFS) Chtimes(string, time.Time, time.Time) error {
	return fs.ErrInvalid
}

func (readerFS) Chmod(string, fs.FileMode) error {
	return fs.ErrInvalid
}

func (readerFS) Mkdir(string, fs.FileMode) error {
	return fs.ErrInvalid
}

func (readerFS) Remove(string) error {
	return fs.ErrInvalid
}

func (readerFS) Rename(string, string) error {
	return fs.ErrInvalid
}

func (readerFS) Link(string, string) error {
	return fs.ErrInvalid
}

func (readerFS) Symlink(string, string) error {
	return fs.ErrInvalid
}

func (readerFS) ReadLink(string) (string, error) {
	return "", fs.ErrInvalid
}

func (readerFS) Lstat(string) (fs.FileInfo, error) {
	return nil, fs.ErrInvalid
}

type readerFile struct {
	io.Reader
}

func (readerFile) Close() error               { return nil }
func (readerFile) Stat() (fs.FileInfo, error) { return nil, fs.ErrInvalid }

// testFS holds the fixtures used by the tests:
//
//	a.txt:       "a\nb\nc\nd\ne\n"
//	nonl.txt:    "a\nb\nc" (no trailing newline)
//	empty.txt:   ""
//	lines.txt:   "l1\nl2\nl3\n"
//	colon.txt:   "one:two:three:"
//	nul.txt:     "a\x00b\x00"
//	lead.txt:    "\na\nb\nc" (leading newline)
//	dots.txt:    "1._2.__3.___4._"
//	dots2.txt:   "a.___b.__1._2.__3.___4._"
//	before.txt:  "._1._2.__3.___4"
//	before2.txt: ".__x.___y.____z._1._2.__3.___4"
//	"-b":        "dash-b\n" (a file named like an option)
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":         {Data: []byte("a\nb\nc\nd\ne\n")},
		"nonl.txt":      {Data: []byte("a\nb\nc")},
		"empty.txt":     {Data: []byte("")},
		"lines.txt":     {Data: []byte("l1\nl2\nl3\n")},
		"colon.txt":     {Data: []byte("one:two:three:")},
		"nul.txt":       {Data: []byte("a\x00b\x00")},
		"lead.txt":      {Data: []byte("\na\nb\nc")},
		"dots.txt":      {Data: []byte("1._2.__3.___4._")},
		"dots2.txt":     {Data: []byte("a.___b.__1._2.__3.___4._")},
		"before.txt":    {Data: []byte("._1._2.__3.___4")},
		"before2.txt":   {Data: []byte(".__x.___y.____z._1._2.__3.___4")},
		"-b":            {Data: []byte("dash-b\n")},
		"sub/inner.txt": {Data: []byte("inner\n")},
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

type testCase struct {
	name     string
	stdin    io.Reader
	stdout   io.Writer
	fsys     fsx.FS
	dir      string
	args     []string
	want     string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	if test.stdout == nil {
		test.stdout = new(bytes.Buffer)
	}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      test.stdin,
		Stdout:     test.stdout,
		Stderr:     stderr,
		FS:         test.fsys,
		WorkingDir: test.dir,
	}
	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if buf, ok := test.stdout.(*bytes.Buffer); ok && buf.String() != test.want {
		t.Errorf("Run() stdout = %q, want %q", buf.String(), test.want)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			name:  "no operand reads the standard input",
			stdin: strings.NewReader("a\nb\nc\n"),
			want:  "c\nb\na\n",
		},
		{
			name:  "empty standard input",
			stdin: strings.NewReader(""),
			want:  "",
		},
		{
			name:  "a single record without a separator",
			stdin: strings.NewReader("a"),
			want:  "a",
		},
		{
			name:  "a single separator",
			stdin: strings.NewReader("\n"),
			want:  "\n",
		},
		{
			name:  "a single record",
			stdin: strings.NewReader("a\n"),
			want:  "a\n",
		},
		{
			name:  "no trailing separator",
			stdin: strings.NewReader("a\nb"),
			want:  "ba\n",
		},
		{
			name:  "empty records are kept",
			stdin: strings.NewReader("a\n\nb\n"),
			want:  "b\n\na\n",
		},
		{
			name:  "a long record",
			stdin: strings.NewReader("1234567\n8\n"),
			want:  "8\n1234567\n",
		},
		{
			name: "the records of a file in reverse",
			fsys: testFS(),
			args: []string{"a.txt"},
			want: "e\nd\nc\nb\na\n",
		},
		{
			name: "a file without a trailing separator",
			fsys: testFS(),
			args: []string{"nonl.txt"},
			want: "cb\na\n",
		},
		{
			name: "empty file",
			fsys: testFS(),
			args: []string{"empty.txt"},
			want: "",
		},
		{
			name:  "the '-' operand reads the standard input",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-"},
			want:  "b\na\n",
		},
		{
			name:  "the standard input is read once per '-' operand",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-", "-"},
			want:  "b\na\n",
		},
		{
			name: "pathnames are resolved from the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt"},
			want: "inner\n",
		},
		{
			name: "double dash allows a file named like an option",
			fsys: testFS(),
			args: []string{"--", "-b"},
			want: "dash-b\n",
		},
		{
			name:  "double dash without operands reads the standard input",
			stdin: strings.NewReader("a\n"),
			args:  []string{"--"},
			want:  "a\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSeparator(t *testing.T) {
	tests := []testCase{
		{
			name: "the separator option",
			fsys: testFS(),
			args: []string{"-s", ":", "colon.txt"},
			want: "three:two:one:",
		},
		{
			name: "separator attached to the option",
			fsys: testFS(),
			args: []string{"-s:", "colon.txt"},
			want: "three:two:one:",
		},
		{
			name: "long separator option",
			fsys: testFS(),
			args: []string{"--separator=:", "colon.txt"},
			want: "three:two:one:",
		},
		{
			name: "long separator option with the argument on the next argument",
			fsys: testFS(),
			args: []string{"--separator", ":", "colon.txt"},
			want: "three:two:one:",
		},
		{
			name:  "multi-character separator",
			stdin: strings.NewReader("a::b::c"),
			args:  []string{"-s", "::"},
			want:  "cb::a::",
		},
		{
			name: "a separator missing from the input copies it unchanged",
			fsys: testFS(),
			args: []string{"-s", "XY", "a.txt"},
			want: "a\nb\nc\nd\ne\n",
		},
		{
			name:  "an empty separator means the NUL byte",
			stdin: strings.NewReader("a\x00b"),
			args:  []string{"-s", ""},
			want:  "ba\x00",
		},
		{
			name: "the NUL byte separator with a trailing separator",
			fsys: testFS(),
			args: []string{"-s", "", "nul.txt"},
			want: "b\x00a\x00",
		},
		{
			name:  "a trailing separator ends the last record",
			stdin: strings.NewReader("a:b:"),
			args:  []string{"-s", ":"},
			want:  "b:a:",
		},
		{
			name:  "a leading separator starts an empty record",
			stdin: strings.NewReader(":a:b"),
			args:  []string{"-s", ":"},
			want:  "ba::",
		},
		{
			name:  "matches are right-aligned without overlapping",
			stdin: strings.NewReader("aabab"),
			args:  []string{"-s", "ab"},
			want:  "abaab",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunEscapes(t *testing.T) {
	tests := []testCase{
		{
			name:  "tab escape",
			stdin: strings.NewReader("a\tb\tc"),
			args:  []string{"-s", `\t`},
			want:  "cb\ta\t",
		},
		{
			name:  "newline escape behaves like the default",
			stdin: strings.NewReader("a\nb"),
			args:  []string{"-s", `\n`},
			want:  "ba\n",
		},
		{
			name:  "carriage return escape",
			stdin: strings.NewReader("a\rb\rc"),
			args:  []string{"-s", `\r`},
			want:  "cb\ra\r",
		},
		{
			name:  "backslash escape",
			stdin: strings.NewReader(`a\b\c`),
			args:  []string{"-s", `\\`},
			want:  `cb\a\`,
		},
		{
			name:  "alert escape",
			stdin: strings.NewReader("a\x07b"),
			args:  []string{"-s", `\a`},
			want:  "ba\x07",
		},
		{
			name:  "backspace escape",
			stdin: strings.NewReader("a\x08b"),
			args:  []string{"-s", `\b`},
			want:  "ba\x08",
		},
		{
			name:  "form feed escape",
			stdin: strings.NewReader("a\x0cb"),
			args:  []string{"-s", `\f`},
			want:  "ba\x0c",
		},
		{
			name:  "vertical tab escape",
			stdin: strings.NewReader("a\x0bb"),
			args:  []string{"-s", `\v`},
			want:  "ba\x0b",
		},
		{
			name:  "three-digit octal escape",
			stdin: strings.NewReader("a0b0"),
			args:  []string{"-s", `\060`},
			want:  "b0a0",
		},
		{
			name:  "octal escape",
			stdin: strings.NewReader("aAbAc"),
			args:  []string{"-s", `\101`},
			want:  "cbAaA",
		},
		{
			name:  "zero escape is the NUL byte",
			stdin: strings.NewReader("a\x00b"),
			args:  []string{"-s", `\0`},
			want:  "ba\x00",
		},
		{
			name:  "an unknown escape is kept as is",
			stdin: strings.NewReader(`a\qb`),
			args:  []string{"-s", `\q`},
			want:  `ba\q`,
		},
		{
			name:  "a separator made only of escapes",
			stdin: strings.NewReader("a\tb\tc\t"),
			args:  []string{"-s", `\t`},
			want:  "c\tb\ta\t",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunBefore(t *testing.T) {
	tests := []testCase{
		{
			name:  "the separator is attached before the record",
			stdin: strings.NewReader("\na\nb\nc"),
			args:  []string{"-b"},
			want:  "\nc\nb\na",
		},
		{
			name:  "a final record without a separator",
			stdin: strings.NewReader("a\nb\nc"),
			args:  []string{"-b"},
			want:  "\nc\nba",
		},
		{
			name:  "a trailing separator is its own last record",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-b"},
			want:  "\n\nc\nba",
		},
		{
			name:  "a single record",
			stdin: strings.NewReader("a\n"),
			args:  []string{"-b"},
			want:  "\na",
		},
		{
			name:  "empty input",
			stdin: strings.NewReader(""),
			args:  []string{"-b"},
			want:  "",
		},
		{
			name:  "input without a separator",
			stdin: strings.NewReader("abc"),
			args:  []string{"-b"},
			want:  "abc",
		},
		{
			name:  "a custom separator",
			stdin: strings.NewReader(":a:b:c"),
			args:  []string{"-s", ":", "-b"},
			want:  ":c:b:a",
		},
		{
			name:  "long before option",
			stdin: strings.NewReader("\na\nb\nc"),
			args:  []string{"--before"},
			want:  "\nc\nb\na",
		},
		{
			name:  "grouped with the separator",
			stdin: strings.NewReader(":a:b:c"),
			args:  []string{"-bs:"},
			want:  ":c:b:a",
		},
		{
			name: "a file with a leading separator",
			fsys: testFS(),
			args: []string{"-b", "lead.txt"},
			want: "\nc\nb\na",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRegex(t *testing.T) {
	tests := []testCase{
		{
			name:  "the default separator as a regular expression",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-r"},
			want:  "b\na\n",
		},
		{
			name: "each file is reversed separately",
			fsys: testFS(),
			args: []string{"-r", "a.txt", "lines.txt"},
			want: "e\nd\nc\nb\na\nl3\nl2\nl1\n",
		},
		{
			name: "a regular expression separator",
			fsys: testFS(),
			args: []string{"-r", "-s", `\._+`, "dots.txt"},
			want: "4._3.___2.__1._",
		},
		{
			name: "long regex options",
			fsys: testFS(),
			args: []string{"--regex", `--separator=\._+`, "dots.txt"},
			want: "4._3.___2.__1._",
		},
		{
			name: "records between matches",
			fsys: testFS(),
			args: []string{"-r", "-s", `\._+`, "dots2.txt"},
			want: "4._3.___2.__1._b.__a.___",
		},
		{
			name: "before with a regular expression",
			fsys: testFS(),
			args: []string{"-b", "-r", "-s", `\._+`, "before.txt"},
			want: ".___4.__3._2._1",
		},
		{
			name: "before with a longer input",
			fsys: testFS(),
			args: []string{"-b", "-r", "-s", `\._+`, "before2.txt"},
			want: ".___4.__3._2._1.____z.___y.__x",
		},
		{
			name:  "the start anchor matches at newlines",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-r", "-s", "^"},
			want:  "c\nb\na\n",
		},
		{
			name:  "the end anchor matches at newlines",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-r", "-s", "$"},
			want:  "\n\nc\nba",
		},
		{
			name:  "empty records with both anchors",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-r", "-s", "^$"},
			want:  "a\nb\nc\n",
		},
		{
			name:  "alternation",
			stdin: strings.NewReader("aXbYc"),
			args:  []string{"-r", "-s", "X|Y"},
			want:  "cbYaX",
		},
		{
			name:     "an invalid pattern",
			args:     []string{"-r", "-s", "["},
			wantErr:  "tac: invalid separator: error parsing regexp: missing closing ]: `[`\n",
			wantExit: 2,
		},
		{
			name:     "an empty separator is invalid with -r",
			args:     []string{"-r", "-s", ""},
			wantErr:  "tac: separator cannot be empty with -r\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFiles(t *testing.T) {
	tests := []testCase{
		{
			name: "multiple files are reversed each",
			fsys: testFS(),
			args: []string{"a.txt", "lines.txt"},
			want: "e\nd\nc\nb\na\nl3\nl2\nl1\n",
		},
		{
			name:  "the standard input among the files",
			stdin: strings.NewReader("x\n"),
			fsys:  testFS(),
			args:  []string{"-", "lines.txt"},
			want:  "x\nl3\nl2\nl1\n",
		},
		{
			name:     "a missing file doesn't stop the next ones",
			fsys:     testFS(),
			args:     []string{"missing.txt", "lines.txt"},
			want:     "l3\nl2\nl1\n",
			wantErr:  "tac: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name: "an empty file among the files",
			fsys: testFS(),
			args: []string{"empty.txt", "lines.txt"},
			want: "l3\nl2\nl1\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	boom := errors.New("boom")

	tests := []testCase{
		{
			name:     "invalid option",
			args:     []string{"-z"},
			wantErr:  "tac: invalid option -- 'z'\n",
			wantExit: 2,
		},
		{
			name:     "unrecognized long option",
			args:     []string{"--bogus"},
			wantErr:  "tac: unrecognized option '--bogus'\n",
			wantExit: 2,
		},
		{
			name:     "long option that doesn't allow an argument",
			args:     []string{"--before=x"},
			wantErr:  "tac: option '--before=x' doesn't allow an argument\n",
			wantExit: 2,
		},
		{
			name:     "missing separator argument",
			args:     []string{"-s"},
			wantErr:  "tac: option requires an argument -- 's'\n",
			wantExit: 2,
		},
		{
			name:     "missing long separator argument",
			args:     []string{"--separator"},
			wantErr:  "tac: option '--separator' requires an argument\n",
			wantExit: 2,
		},
		{
			name:     "missing file",
			fsys:     testFS(),
			args:     []string{"missing.txt"},
			wantErr:  "tac: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "invalid zero-length file name",
			args:     []string{""},
			wantErr:  "tac: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "no file system available",
			args:     []string{"a.txt"},
			wantErr:  "tac: a.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "no standard input available",
			wantErr:  "tac: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "write error",
			stdin:    strings.NewReader("a\nb\n"),
			stdout:   errorWriter{err: boom},
			wantErr:  "tac: write error: boom\n",
			wantExit: 1,
		},
		{
			name:     "read error",
			fsys:     readerFS{r: errorReader{err: boom}},
			args:     []string{"f"},
			wantErr:  "tac: f: boom\n",
			wantExit: 1,
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
	rt := &coreutils.Runtime{
		Stdin:  &cancelReader{cancel: cancel},
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     testFS(),
	}
	if err := New(rt).Run(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRunContextCanceledBetweenRecords(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	stdout := &cancelWriter{cancel: cancel}
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("a\nb\n"),
		Stdout: stdout,
		Stderr: io.Discard,
	}
	if err := New(rt).Run(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRunContextCanceledBeforeFile(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	stdout := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("a\nb\n"),
		Stdout: stdout,
		Stderr: io.Discard,
		FS:     testFS(),
	}
	err := New(rt).Run(ctx, "a.txt", "lines.txt")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want %q", got, "")
	}
}
