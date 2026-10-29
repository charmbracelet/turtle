package nl

import (
	"bytes"
	"context"
	"errors"
	"io"
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

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
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

// testFS holds the fixtures used by the tests:
//
//	lorem.txt:  "lorem\nipsum\ndolor\n"
//	blanks.txt: "a\n\n\nb\n\n\n\nc\n"
//	pages.txt:  "\:\:\:\nh1\nh2\n\:\:\nb1\n\nb3\n\:\nf1\nf2\n"
//	regex.txt:  "foo\nbar\nfoobar\n"
//	nonl.txt:   "lorem\nipsum" (no trailing newline)
//	empty.txt:  ""
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"lorem.txt":  {Data: []byte("lorem\nipsum\ndolor\n")},
		"blanks.txt": {Data: []byte("a\n\n\nb\n\n\n\nc\n")},
		"pages.txt":  {Data: []byte("\\:\\:\\:\nh1\nh2\n\\:\\:\nb1\n\nb3\n\\:\nf1\nf2\n")},
		"regex.txt":  {Data: []byte("foo\nbar\nfoobar\n")},
		"nonl.txt":   {Data: []byte("lorem\nipsum")},
		"empty.txt":  {Data: []byte("")},
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
	fsys     fsx.FS
	dir      string
	args     []string
	want     string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      test.stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         test.fsys,
		WorkingDir: test.dir,
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
}

func TestRunInput(t *testing.T) {
	tests := []testCase{
		{
			name: "a file",
			fsys: testFS(),
			args: []string{"lorem.txt"},
			want: "     1\tlorem\n     2\tipsum\n     3\tdolor\n",
		},
		{
			name:  "standard input",
			stdin: strings.NewReader("lorem\nipsum\n"),
			want:  "     1\tlorem\n     2\tipsum\n",
		},
		{
			name:  "a dash operand is the standard input",
			stdin: strings.NewReader("lorem\n"),
			args:  []string{"-"},
			want:  "     1\tlorem\n",
		},
		{
			name:     "options stop at the first operand",
			fsys:     testFS(),
			args:     []string{"lorem.txt", "-b", "a"},
			want:     "     1\tlorem\n     2\tipsum\n     3\tdolor\n",
			wantErr:  "nl: -b: file does not exist\nnl: a: file does not exist\n",
			wantExit: 1,
		},
		{
			name: "multiple files are numbered as one document",
			fsys: testFS(),
			args: []string{"lorem.txt", "nonl.txt"},
			want: "     1\tlorem\n     2\tipsum\n     3\tdolor\n     4\tlorem\n     5\tipsum",
		},
		{
			name:     "a file which can't be read doesn't stop the remaining ones",
			fsys:     testFS(),
			args:     []string{"missing.txt", "lorem.txt"},
			want:     "     1\tlorem\n     2\tipsum\n     3\tdolor\n",
			wantErr:  "nl: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:  "the standard input is concatenated between files",
			fsys:  testFS(),
			stdin: strings.NewReader("b\n"),
			args:  []string{"lorem.txt", "-"},
			want:  "     1\tlorem\n     2\tipsum\n     3\tdolor\n     4\tb\n",
		},
		{
			name:     "missing file",
			fsys:     testFS(),
			args:     []string{"missing.txt"},
			wantErr:  "nl: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:  "blank lines are not numbered by default",
			stdin: strings.NewReader("a\n\nb\n"),
			want:  "     1\ta\n      \t\n     2\tb\n",
		},
		{
			name:  "a line holding only blanks is numbered",
			stdin: strings.NewReader("a\n   \nb\n"),
			want:  "     1\ta\n     2\t   \n     3\tb\n",
		},
		{
			name: "an unterminated last line is numbered",
			fsys: testFS(),
			args: []string{"nonl.txt"},
			want: "     1\tlorem\n     2\tipsum",
		},
		{
			name: "an empty file produces no output",
			fsys: testFS(),
			args: []string{"empty.txt"},
			want: "",
		},
		{
			name: "options after -- are operands",
			fsys: testFS(),
			args: []string{"--", "lorem.txt"},
			want: "     1\tlorem\n     2\tipsum\n     3\tdolor\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunStyles(t *testing.T) {
	tests := []testCase{
		{
			name: "number all lines",
			fsys: testFS(),
			args: []string{"-b", "a", "blanks.txt"},
			want: "     1\ta\n     2\t\n     3\t\n     4\tb\n" +
				"     5\t\n     6\t\n     7\t\n     8\tc\n",
		},
		{
			name: "number no lines",
			fsys: testFS(),
			args: []string{"-b", "n", "lorem.txt"},
			want: "      \tlorem\n      \tipsum\n      \tdolor\n",
		},
		{
			name: "number non-empty lines",
			fsys: testFS(),
			args: []string{"-b", "t", "blanks.txt"},
			want: "     1\ta\n      \t\n      \t\n     2\tb\n" +
				"      \t\n      \t\n      \t\n     3\tc\n",
		},
		{
			name: "number lines matching a regular expression",
			fsys: testFS(),
			args: []string{"-bpfoo", "regex.txt"},
			want: "     1\tfoo\n      \tbar\n     2\tfoobar\n",
		},
		{
			name: "number lines matching an anchored regular expression",
			fsys: testFS(),
			args: []string{"-b", "p^bar$", "regex.txt"},
			want: "      \tfoo\n     1\tbar\n      \tfoobar\n",
		},
		{
			name: "an empty regular expression matches every line",
			fsys: testFS(),
			args: []string{"-bp", "blanks.txt"},
			want: "     1\ta\n     2\t\n     3\t\n     4\tb\n" +
				"     5\t\n     6\t\n     7\t\n     8\tc\n",
		},
		{
			name:  "a basic regular expression keeps its grouping syntax",
			stdin: strings.NewReader("ab\naab\nb\n"),
			args:  []string{"-b", `p^\(a*b\)$`},
			want:  "     1\tab\n     2\taab\n     3\tb\n",
		},
		{
			name:     "an invalid body style",
			fsys:     testFS(),
			args:     []string{"-b", "q", "lorem.txt"},
			wantErr:  "nl: invalid body numbering style: 'q'\n",
			wantExit: 1,
		},
		{
			name:     "an invalid header style",
			fsys:     testFS(),
			args:     []string{"-h", "q", "lorem.txt"},
			wantErr:  "nl: invalid header numbering style: 'q'\n",
			wantExit: 1,
		},
		{
			name:     "an invalid footer style",
			fsys:     testFS(),
			args:     []string{"-f", "q", "lorem.txt"},
			wantErr:  "nl: invalid footer numbering style: 'q'\n",
			wantExit: 1,
		},
		{
			name:     "an invalid regular expression",
			fsys:     testFS(),
			args:     []string{"-b", `p\(`, "lorem.txt"},
			wantErr:  "nl: unmatched \\(\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSections(t *testing.T) {
	tests := []testCase{
		{
			name: "header and footer lines are not numbered by default",
			fsys: testFS(),
			args: []string{"pages.txt"},
			want: "\n      \th1\n      \th2\n\n     1\tb1\n      \t\n     2\tb3\n" +
				"\n      \tf1\n      \tf2\n",
		},
		{
			name: "numbering restarts at each section",
			fsys: testFS(),
			args: []string{"-h", "a", "-f", "a", "-b", "a", "pages.txt"},
			want: "\n     1\th1\n     2\th2\n\n     1\tb1\n     2\t\n     3\tb3\n" +
				"\n     1\tf1\n     2\tf2\n",
		},
		{
			name: "numbering restarts at each logical page",
			stdin: strings.NewReader("\\:\\:\\:\nh1\n\\:\\:\nb1\n" +
				"\\:\\:\\:\nh2\n\\:\\:\nb2\n"),
			args: []string{"-h", "a", "-b", "a"},
			want: "\n     1\th1\n\n     1\tb1\n\n     1\th2\n\n     1\tb2\n",
		},
		{
			name: "numbering does not restart with -p",
			stdin: strings.NewReader("\\:\\:\\:\nh1\n\\:\\:\nb1\n" +
				"\\:\\:\\:\nh2\n\\:\\:\nb2\n"),
			args: []string{"-p", "-h", "a", "-b", "a"},
			want: "\n     1\th1\n\n     2\tb1\n\n     3\th2\n\n     4\tb2\n",
		},
		{
			name:  "the input starts in a logical page body",
			stdin: strings.NewReader("a\n\\:\\:\\:\nh\n"),
			args:  []string{"-h", "a"},
			want:  "     1\ta\n\n     1\th\n",
		},
		{
			name:  "custom delimiter characters",
			stdin: strings.NewReader("!+!+!+\nh\n!+!+\nb\n!+\nf\n"),
			args:  []string{"-d", "!+", "-h", "a", "-f", "a"},
			want:  "\n     1\th\n\n     1\tb\n\n     1\tf\n",
		},
		{
			name:  "a single delimiter character implies a colon",
			stdin: strings.NewReader("!:!:!:\nh\n!:!:\nb\n"),
			args:  []string{"-d", "!", "-h", "a"},
			want:  "\n     1\th\n\n     1\tb\n",
		},
		{
			name:  "an empty delimiter disables section matching",
			stdin: strings.NewReader("\\:\\:\\:\nb\n"),
			args:  []string{"-d", ""},
			want:  "     1\t\\:\\:\\:\n     2\tb\n",
		},
		{
			name:  "a line holding extra characters is not a delimiter",
			stdin: strings.NewReader("\\: \nb\n"),
			want:  "     1\t\\: \n     2\tb\n",
		},
		{
			name: "numbering is not reset between files",
			fsys: fsx.MemFS{
				"page1.txt": {Data: []byte("\\:\\:\\:\nh1\n\\:\\:\nb1\n")},
				"page2.txt": {Data: []byte("\\:\\:\nb2\n")},
			},
			args: []string{"-h", "a", "page1.txt", "page2.txt"},
			want: "\n     1\th1\n\n     1\tb1\n\n     1\tb2\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLongOptions(t *testing.T) {
	tests := []testCase{
		{
			name:  "header numbering",
			stdin: strings.NewReader("\\:\\:\\:\nh\n"),
			args:  []string{"--header-numbering=a"},
			want:  "\n     1\th\n",
		},
		{
			name:  "body numbering",
			stdin: strings.NewReader("a\n"),
			args:  []string{"--body-numbering=n"},
			want:  "      \ta\n",
		},
		{
			name:  "footer numbering",
			stdin: strings.NewReader("\\:\nf\n"),
			args:  []string{"--footer-numbering=a"},
			want:  "\n     1\tf\n",
		},
		{
			name:  "section delimiter",
			stdin: strings.NewReader("!+!+!+\nh\n"),
			args:  []string{"--section-delimiter=!+", "--header-numbering=a"},
			want:  "\n     1\th\n",
		},
		{
			name:  "starting line number",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"--starting-line-number=10"},
			want:  "    10\ta\n    11\tb\n",
		},
		{
			name:  "line increment",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"--line-increment=5"},
			want:  "     1\ta\n     6\tb\n",
		},
		{
			name:  "join blank lines",
			stdin: strings.NewReader("a\n\n\nb\n"),
			args:  []string{"--body-numbering=a", "--join-blank-lines=2"},
			want:  "     1\ta\n      \t\n     2\t\n     3\tb\n",
		},
		{
			name:  "number format and width",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"--number-format=rz", "--number-width=3"},
			want:  "001\ta\n002\tb\n",
		},
		{
			name:  "number separator",
			stdin: strings.NewReader("a\n"),
			args:  []string{"--number-separator=::"},
			want:  "     1::a\n",
		},
		{
			name:  "an argument in the next argument",
			stdin: strings.NewReader("a\n"),
			args:  []string{"--number-width", "3"},
			want:  "  1\ta\n",
		},
		{
			name:  "no renumber",
			stdin: strings.NewReader("\\:\\:\\:\nh\n\\:\\:\nb\n\\:\nc\n"),
			args:  []string{"--no-renumber", "--header-numbering=a"},
			want:  "\n     1\th\n\n     2\tb\n\n      \tc\n",
		},
		{
			name:     "a missing argument",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"--number-width"},
			wantErr:  "nl: option '--number-width' requires an argument\n",
			wantExit: 1,
		},
		{
			name:     "an unrecognized option",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"--bogus"},
			wantErr:  "nl: unrecognized option '--bogus'\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunNumbering(t *testing.T) {
	tests := []testCase{
		{
			name:  "starting number and increment",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-v", "10", "-i", "10"},
			want:  "    10\ta\n    20\tb\n    30\tc\n",
		},
		{
			name:  "a negative increment",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"-i", "-1"},
			want:  "     1\ta\n     0\tb\n    -1\tc\n",
		},
		{
			name:  "a negative starting number",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-v", "-5"},
			want:  "    -5\ta\n    -4\tb\n",
		},
		{
			name:  "left justified numbers",
			stdin: strings.NewReader("ab\ncd\n"),
			args:  []string{"-n", "ln", "-w", "3"},
			want:  "1  \tab\n2  \tcd\n",
		},
		{
			name:  "right justified numbers with leading zeros",
			stdin: strings.NewReader("ab\ncd\n"),
			args:  []string{"-n", "rz"},
			want:  "000001\tab\n000002\tcd\n",
		},
		{
			name:  "a number wider than the field",
			stdin: strings.NewReader("ab\n"),
			args:  []string{"-v", "1234567"},
			want:  "1234567\tab\n",
		},
		{
			name:  "a custom separator",
			stdin: strings.NewReader("ab\n"),
			args:  []string{"-s", "##"},
			want:  "     1##ab\n",
		},
		{
			name:  "an empty separator",
			stdin: strings.NewReader("ab\n"),
			args:  []string{"-s", ""},
			want:  "     1ab\n",
		},
		{
			name:  "blank lines are grouped with -l",
			stdin: strings.NewReader("a\n\n\n\nb\n"),
			args:  []string{"-b", "a", "-l", "2"},
			want:  "     1\ta\n      \t\n     2\t\n      \t\n     3\tb\n",
		},
		{
			name:  "blank line groups restart after a numbered line",
			stdin: strings.NewReader("a\n\n\n\n\nb\n"),
			args:  []string{"-b", "a", "-l", "2"},
			want:  "     1\ta\n      \t\n     2\t\n      \t\n     3\t\n     4\tb\n",
		},
		{
			name:  "-l does not apply to the default style",
			stdin: strings.NewReader("a\n\n\n\nb\n"),
			args:  []string{"-l", "2"},
			want:  "     1\ta\n      \t\n      \t\n      \t\n     2\tb\n",
		},
		{
			name:  "an unnumbered line has a blank number field",
			stdin: strings.NewReader("a\n"),
			args:  []string{"-b", "n"},
			want:  "      \ta\n",
		},
		{
			name:  "the blank field uses spaces even with -n rz",
			stdin: strings.NewReader("a\n\nb\n"),
			args:  []string{"-n", "rz", "-b", "a", "-l", "2"},
			want:  "000001\ta\n      \t\n000002\tb\n",
		},
		{
			name:  "the blank field follows the width and the separator",
			stdin: strings.NewReader("a\n\nb\n"),
			args:  []string{"-b", "t", "-w", "3", "-s", "##"},
			want:  "  1##a\n   ##\n  2##b\n",
		},
		{
			name:     "an invalid starting number",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-v", "x"},
			wantErr:  "nl: invalid starting line number: 'x'\n",
			wantExit: 1,
		},
		{
			name:     "an invalid increment",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-i", "x"},
			wantErr:  "nl: invalid line number increment: 'x'\n",
			wantExit: 1,
		},
		{
			name:     "an invalid blank line count",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-l", "0"},
			wantErr:  "nl: invalid line number of blank lines: '0'\n",
			wantExit: 1,
		},
		{
			name:     "an invalid numbering format",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-n", "q"},
			wantErr:  "nl: invalid line numbering format: 'q'\n",
			wantExit: 1,
		},
		{
			name:     "an invalid width",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-w", "0"},
			wantErr:  "nl: invalid line number field width: '0'\n",
			wantExit: 1,
		},
		{
			name:     "an invalid option",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-q"},
			wantErr:  "nl: invalid option -- 'q'\n",
			wantExit: 1,
		},
		{
			name:     "a missing option argument",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-b"},
			wantErr:  "nl: option requires an argument -- 'b'\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOverflow(t *testing.T) {
	tests := []testCase{
		{
			name:  "the greatest number is still printed",
			stdin: strings.NewReader("a\n"),
			args:  []string{"-v", "9223372036854775807"},
			want:  "9223372036854775807\ta\n",
		},
		{
			name:     "an overflowing increment is an error on the next line",
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"-v", "9223372036854775807"},
			want:     "9223372036854775807\ta\n",
			wantErr:  "nl: line number overflow\n",
			wantExit: 1,
		},
		{
			name:     "an overflowing negative increment is an error",
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"-v", "-9223372036854775808", "-i", "-1"},
			want:     "-9223372036854775808\ta\n",
			wantErr:  "nl: line number overflow\n",
			wantExit: 1,
		},
		{
			name:     "an out of range starting number",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-v", "9223372036854775808"},
			wantErr:  "nl: invalid starting line number: '9223372036854775808'\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunReadError(t *testing.T) {
	readErr := errors.New("read failed")

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  errorReader{err: readErr},
		Stdout: new(bytes.Buffer),
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context())

	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if got := stderr.String(); got != "nl: standard input: read failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "nl: standard input: read failed\n")
	}
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("lorem\n"),
		Stdout: errorWriter{err: writeErr},
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context())

	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
	if got := stderr.String(); got != "nl: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "nl: write error: write failed\n")
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		runCancelled(t, ctx, strings.NewReader("lorem\n"))
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runCancelled(t, ctx, &cancelReader{cancel: cancel})
	})
}

// runCancelled runs the utility with a context that is either already done or
// becomes done while reading, and checks that the context error is returned
// without any output.
func runCancelled(t *testing.T, ctx context.Context, stdin io.Reader) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
	err := New(rt).Run(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
}
