package pr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
// that is done while input is being read.
type cancelReader struct {
	cancel context.CancelFunc
}

func (r *cancelReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
}

// modTime is the fixed modification time of the fixture files, so the page
// headers are deterministic.
var modTime = time.Date(2020, 1, 2, 3, 4, 5, 0, time.Local)

// testFS holds the fixtures used by the tests:
//
//	lorem.txt:  "lorem\nipsum\ndolor\n"
//	blanks.txt: "a\n\n\nb\n"
//	seven.txt:  "1\n" .. "7\n"
//	ten.txt:    "1\n" .. "10\n"
//	nonl.txt:   "lorem\nipsum" (no trailing newline)
//	empty.txt:  ""
//	tabs.txt:   "a\tb\n"
//	long.txt:   80 "x" characters
//	odd.txt:    "a\nb\nc\nd\ne\n" (5 lines)
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"lorem.txt":     {Data: []byte("lorem\nipsum\ndolor\n"), ModTime: modTime},
		"blanks.txt":    {Data: []byte("a\n\n\nb\n"), ModTime: modTime},
		"seven.txt":     {Data: []byte("1\n2\n3\n4\n5\n6\n7\n"), ModTime: modTime},
		"ten.txt":       {Data: []byte("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"), ModTime: modTime},
		"nonl.txt":      {Data: []byte("lorem\nipsum"), ModTime: modTime},
		"empty.txt":     {Data: []byte(""), ModTime: modTime},
		"tabs.txt":      {Data: []byte("a\tb\n"), ModTime: modTime},
		"long.txt":      {Data: []byte(strings.Repeat("x", 80) + "\n"), ModTime: modTime},
		"odd.txt":       {Data: []byte("a\nb\nc\nd\ne\n"), ModTime: modTime},
		"sub/inner.txt": {Data: []byte("inner\n"), ModTime: modTime},
	}
}

// header returns the expected five-line page header for a fixture file: the
// GNU layout, with the date at the left, the file name centered and the page
// number at the right, padded to the page width.
func header(name string, page int) string {
	return headerOffset(name, page, 0)
}

// headerOffset returns the expected header with the given left margin.
func headerOffset(name string, page, offset int) string {
	date := modTime.In(time.Local).Format("2006-01-02 15:04")
	pageStr := fmt.Sprintf("Page %d", page)
	avail := max(defWidth-len(date)-len(name)-len(pageStr), 0)
	lhs, rhs := avail/2, avail-avail/2
	margin := strings.Repeat(" ", offset)
	return margin + "\n\n" + margin + date + strings.Repeat(" ", lhs) + name +
		strings.Repeat(" ", rhs) + pageStr + "\n\n\n"
}

// padded returns the page content followed by the newlines that fill the
// page and form the trailer.
func padded(content string, body, lines int) string {
	return content + strings.Repeat("\n", body-lines+5)
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
	environ  []string
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
		Environ: func() []string {
			return test.environ
		},
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

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			name: "a file is paginated with a header and a trailer",
			fsys: testFS(),
			args: []string{"lorem.txt"},
			want: header("lorem.txt", 1) + padded("lorem\nipsum\ndolor\n", 56, 3),
		},
		{
			name: "the file name in the header is the operand",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt"},
			want: header("inner.txt", 1) + padded("inner\n", 56, 1),
		},
		{
			name:  "the standard input has no name in the header",
			stdin: strings.NewReader("lorem\n"),
			args:  []string{"-t"},
			want:  "lorem\n",
		},
		{
			name:  "a dash operand is the standard input",
			stdin: strings.NewReader("lorem\n"),
			args:  []string{"-t", "-"},
			want:  "lorem\n",
		},
		{
			name: "-t suppresses the header and the trailer",
			fsys: testFS(),
			args: []string{"-t", "lorem.txt"},
			want: "lorem\nipsum\ndolor\n",
		},
		{
			name: "an empty file produces no output",
			fsys: testFS(),
			args: []string{"empty.txt"},
			want: "",
		},
		{
			name: "an unterminated last line is terminated on output",
			fsys: testFS(),
			args: []string{"-t", "nonl.txt"},
			want: "lorem\nipsum\n",
		},
		{
			name: "single column lines are not truncated by -w",
			fsys: testFS(),
			args: []string{"-t", "-w", "10", "long.txt"},
			want: strings.Repeat("x", 80) + "\n",
		},
		{
			name: "two files are paginated separately",
			fsys: testFS(),
			args: []string{"-l", "12", "lorem.txt", "odd.txt"},
			want: header("lorem.txt", 1) + padded("lorem\nipsum\n", 2, 2) +
				header("lorem.txt", 2) + padded("dolor\n", 2, 1) +
				header("odd.txt", 1) + padded("a\nb\n", 2, 2) +
				header("odd.txt", 2) + padded("c\nd\n", 2, 2) +
				header("odd.txt", 3) + padded("e\n", 2, 1),
		},
		{
			name:     "options stop at the first operand",
			fsys:     testFS(),
			args:     []string{"lorem.txt", "-t"},
			want:     header("lorem.txt", 1) + padded("lorem\nipsum\ndolor\n", 56, 3),
			wantErr:  "pr: -t: file does not exist\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunPages(t *testing.T) {
	tests := []testCase{
		{
			name: "a short page length",
			fsys: testFS(),
			args: []string{"-l", "12", "lorem.txt"},
			want: header("lorem.txt", 1) + padded("lorem\nipsum\n", 2, 2) +
				header("lorem.txt", 2) + padded("dolor\n", 2, 1),
		},
		{
			name: "a page length not greater than the header and trailer suppresses both",
			fsys: testFS(),
			args: []string{"-l", "10", "lorem.txt"},
			want: "lorem\nipsum\ndolor\n",
		},
		{
			name: "+page starts the output at the given page",
			fsys: testFS(),
			args: []string{"+2", "-l", "12", "lorem.txt"},
			want: header("lorem.txt", 2) + padded("dolor\n", 2, 1),
		},
		{
			name: "+page beyond the input produces no output",
			fsys: testFS(),
			args: []string{"+9", "-l", "12", "lorem.txt"},
			want: "",
		},
		{
			name: "+page applies to each file",
			fsys: testFS(),
			args: []string{"-t", "+2", "-l", "2", "lorem.txt", "odd.txt"},
			want: "dolor\n" + "c\nd\ne\n",
		},
		{
			name: "-d double-spaces the output",
			fsys: testFS(),
			args: []string{"-t", "-d", "lorem.txt"},
			want: "lorem\n\nipsum\n\ndolor\n\n",
		},
		{
			name: "-d fits double-spaced lines in the page",
			fsys: testFS(),
			args: []string{"-d", "-l", "14", "lorem.txt"},
			want: header("lorem.txt", 1) + "lorem\n\nipsum\n\n" +
				strings.Repeat("\n", 5) +
				header("lorem.txt", 2) + "dolor\n\n" + strings.Repeat("\n", 7),
		},
		{
			name: "-d on an odd-length page pads the extra line",
			fsys: testFS(),
			args: []string{"-d", "-l", "13", "odd.txt"},
			want: header("odd.txt", 1) + "a\n\n" + strings.Repeat("\n", 6) +
				header("odd.txt", 2) + "b\n\n" + strings.Repeat("\n", 6) +
				header("odd.txt", 3) + "c\n\n" + strings.Repeat("\n", 6) +
				header("odd.txt", 4) + "d\n\n" + strings.Repeat("\n", 6) +
				header("odd.txt", 5) + "e\n\n" + strings.Repeat("\n", 6),
		},
		{
			name: "-F terminates pages with a form feed",
			fsys: testFS(),
			args: []string{"-F", "-l", "12", "lorem.txt"},
			want: header("lorem.txt", 1) + "lorem\nipsum\n\f" +
				header("lorem.txt", 2) + "dolor\n\f",
		},
		{
			name: "-f is a form feed too",
			fsys: testFS(),
			args: []string{"-f", "-l", "12", "lorem.txt"},
			want: header("lorem.txt", 1) + "lorem\nipsum\n\f" +
				header("lorem.txt", 2) + "dolor\n\f",
		},
		{
			name: "-t -F is a no-op for the page separator",
			fsys: testFS(),
			args: []string{"-t", "-F", "-l", "2", "lorem.txt"},
			want: "lorem\nipsum\ndolor\n",
		},
		{
			name: "-h replaces the file name in the header",
			fsys: testFS(),
			args: []string{"-h", "My Header", "-l", "12", "lorem.txt"},
			want: header("My Header", 1) + padded("lorem\nipsum\n", 2, 2) +
				header("My Header", 2) + padded("dolor\n", 2, 1),
		},
		{
			name: "-h with an empty header",
			fsys: testFS(),
			args: []string{"-h", "", "-l", "12", "odd.txt"},
			want: header("", 1) + padded("a\nb\n", 2, 2) +
				header("", 2) + padded("c\nd\n", 2, 2) +
				header("", 3) + padded("e\n", 2, 1),
		},
		{
			name: "-o offsets the header and the lines",
			fsys: testFS(),
			args: []string{"-o", "4", "-l", "12", "odd.txt"},
			want: headerOffset("odd.txt", 1, 4) + padded("    a\n    b\n", 2, 2) +
				headerOffset("odd.txt", 2, 4) + padded("    c\n    d\n", 2, 2) +
				headerOffset("odd.txt", 3, 4) + padded("    e\n", 2, 1),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunNumbers(t *testing.T) {
	tests := []testCase{
		{
			name: "-n numbers the lines",
			fsys: testFS(),
			args: []string{"-t", "-n", "lorem.txt"},
			want: "    1\tlorem\n    2\tipsum\n    3\tdolor\n",
		},
		{
			name: "-n with a separator and a width",
			fsys: testFS(),
			args: []string{"-t", "-n.3", "lorem.txt"},
			want: "  1.lorem\n  2.ipsum\n  3.dolor\n",
		},
		{
			name: "-n with a width only",
			fsys: testFS(),
			args: []string{"-t", "-n2", "lorem.txt"},
			want: " 1\tlorem\n 2\tipsum\n 3\tdolor\n",
		},
		{
			name: "wide numbers keep the low-order digits",
			fsys: testFS(),
			args: []string{"-t", "-n1", "ten.txt"},
			want: "1\t1\n2\t2\n3\t3\n4\t4\n5\t5\n6\t6\n7\t7\n8\t8\n9\t9\n0\t10\n",
		},
		{
			name: "numbers restart for each file",
			fsys: testFS(),
			args: []string{"-t", "-n", "lorem.txt", "odd.txt"},
			want: "    1\tlorem\n    2\tipsum\n    3\tdolor\n" +
				"    1\ta\n    2\tb\n    3\tc\n    4\td\n    5\te\n",
		},
		{
			name: "+page numbers the lines of the input, not the printed page",
			fsys: testFS(),
			args: []string{"-t", "+2", "-l", "2", "-n", "lorem.txt"},
			want: "    3\tdolor\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunColumns(t *testing.T) {
	tests := []testCase{
		{
			name: "columns are filled down the page",
			fsys: testFS(),
			args: []string{"-t", "-2", "odd.txt"},
			want: "a\t\t\t\t    d\n" +
				"b\t\t\t\t    e\n" +
				"c\n",
		},
		{
			name: "column padding is contracted to tabs",
			fsys: testFS(),
			args: []string{"-t", "-2", "-w", "20", "odd.txt"},
			want: "a\t  d\nb\t  e\nc\n",
		},
		{
			name: "columns are truncated to their width",
			fsys: testFS(),
			args: []string{"-t", "-2", "long.txt"},
			want: strings.Repeat("x", 35) + "\n",
		},
		{
			name: "-a fills columns across the page",
			fsys: testFS(),
			args: []string{"-t", "-a", "-2", "odd.txt"},
			want: "a\t\t\t\t    b\n" +
				"c\t\t\t\t    d\n" +
				"e\n",
		},
		{
			name: "-a with a separator",
			fsys: testFS(),
			args: []string{"-t", "-a", "-2", "-s|", "odd.txt"},
			want: "a|b\nc|d\ne\n",
		},
		{
			name: "-s separates columns without padding",
			fsys: testFS(),
			args: []string{"-t", "-2", "-s|", "odd.txt"},
			want: "a|d\nb|e\nc\n",
		},
		{
			name: "-s with a tab contracts back to tab stops",
			fsys: testFS(),
			args: []string{"-t", "-2", "-s", "odd.txt"},
			want: "a\td\nb\te\nc\n",
		},
		{
			name: "a full page is not rebalanced",
			fsys: testFS(),
			args: []string{"-t", "-2", "-l", "4", "ten.txt"},
			want: "1\t\t\t\t    5\n" +
				"2\t\t\t\t    6\n" +
				"3\t\t\t\t    7\n" +
				"4\t\t\t\t    8\n" +
				"9\t\t\t\t    10\n",
		},
		{
			name: "a partial page is rebalanced",
			fsys: testFS(),
			args: []string{"-t", "-2", "-l", "4", "blanks.txt"},
			want: "a\t\t\t\t    \n\t\t\t\t    b\n",
		},
		{
			name: "a nearly full page is not rebalanced",
			fsys: testFS(),
			args: []string{"-t", "-2", "-l", "4", "seven.txt"},
			want: "1\t\t\t\t    5\n" +
				"2\t\t\t\t    6\n" +
				"3\t\t\t\t    7\n" +
				"4\n",
		},
		{
			name: "columns with line numbers",
			fsys: testFS(),
			args: []string{"-t", "-2", "-l", "4", "-n.2", "odd.txt"},
			want: " 1.a\t\t\t\t     4.d\n" +
				" 2.b\t\t\t\t     5.e\n" +
				" 3.c\n",
		},
		{
			name: "+page with columns skips whole formatted pages",
			fsys: testFS(),
			args: []string{"-t", "+2", "-2", "-l", "4", "ten.txt"},
			want: "9\t\t\t\t    10\n",
		},
		{
			name: "columns with a header",
			fsys: testFS(),
			args: []string{"-2", "-l", "12", "odd.txt"},
			want: header("odd.txt", 1) +
				"a\t\t\t\t    c\n" +
				"b\t\t\t\t    d\n" +
				strings.Repeat("\n", 5) +
				header("odd.txt", 2) +
				"e\n" +
				strings.Repeat("\n", 6),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunMerge(t *testing.T) {
	tests := []testCase{
		{
			name: "merge files side by side",
			fsys: testFS(),
			args: []string{"-t", "-m", "lorem.txt", "odd.txt"},
			want: "lorem\t\t\t\t    a\n" +
				"ipsum\t\t\t\t    b\n" +
				"dolor\t\t\t\t    c\n" +
				"\t\t\t\t    d\n" +
				"\t\t\t\t    e\n",
		},
		{
			name: "merge with a separator",
			fsys: testFS(),
			args: []string{"-t", "-m", "-s|", "lorem.txt", "odd.txt"},
			want: "lorem|a\nipsum|b\ndolor|c\n|d\n|e\n",
		},
		{
			name: "merge with line numbers",
			fsys: testFS(),
			args: []string{"-t", "-m", "-n.2", "odd.txt", "lorem.txt"},
			want: " 1.a\t\t\t\t      lorem\n" +
				" 2.b\t\t\t\t      ipsum\n" +
				" 3.c\t\t\t\t      dolor\n" +
				" 4.d\t\t\t\t      \n" +
				" 5.e\t\t\t\t      \n",
		},
		{
			name: "the columns of an ended file keep their padding",
			fsys: testFS(),
			args: []string{"-t", "-m", "odd.txt", "nonl.txt"},
			want: "a\t\t\t\t    lorem\n" +
				"b\t\t\t\t    ipsum\n" +
				"c\t\t\t\t    \n" +
				"d\t\t\t\t    \n" +
				"e\t\t\t\t    \n",
		},
		{
			name: "merging one file is single column output",
			fsys: testFS(),
			args: []string{"-t", "-m", "lorem.txt"},
			want: "lorem\nipsum\ndolor\n",
		},
		{
			name: "the merged header has a blank middle",
			fsys: testFS(),
			args: []string{"-m", "-l", "12", "-D", "DATE", "lorem.txt", "odd.txt"},
			want: "\n\nDATE" + strings.Repeat(" ", 62) + "Page 1\n\n\n" +
				"lorem\t\t\t\t    a\nipsum\t\t\t\t    b\n" +
				strings.Repeat("\n", 7) +
				"DATE" + strings.Repeat(" ", 62) + "Page 2\n\n\n" +
				"dolor\t\t\t\t    c\n\t\t\t\t    d\n" +
				strings.Repeat("\n", 7) +
				"DATE" + strings.Repeat(" ", 62) + "Page 3\n\n\n" +
				"\t\t\t\t    e\n" + strings.Repeat("\n", 6),
		},
		{
			name: "merged files that are all empty produce no output",
			fsys: testFS(),
			args: []string{"-m", "empty.txt", "empty.txt"},
			want: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunTabs(t *testing.T) {
	tests := []testCase{
		{
			name: "-e expands input tabs",
			fsys: testFS(),
			args: []string{"-t", "-e", "tabs.txt"},
			want: "a       b\n",
		},
		{
			name: "-e with a gap",
			fsys: testFS(),
			args: []string{"-t", "-e4", "tabs.txt"},
			want: "a   b\n",
		},
		{
			name: "-e with a custom tab character",
			fsys: testFS(),
			args: []string{"-t", "-e,3", "blanks.txt"},
			want: "a\n\n\nb\n",
		},
		{
			name:  "-i contracts blanks to tabs",
			stdin: strings.NewReader("a        b\n"),
			args:  []string{"-t", "-i"},
			want:  "a\t b\n",
		},
		{
			name:  "-i keeps a single blank",
			stdin: strings.NewReader("a b\n"),
			args:  []string{"-t", "-i"},
			want:  "a b\n",
		},
		{
			name:  "-i with a custom tab character",
			stdin: strings.NewReader("a        b\n"),
			args:  []string{"-t", "-i."},
			want:  "a. b\n",
		},
		{
			name: "input tabs are left alone without -e",
			fsys: testFS(),
			args: []string{"-t", "tabs.txt"},
			want: "a\tb\n",
		},
		{
			name: "input tabs are expanded in column mode",
			fsys: testFS(),
			args: []string{"-t", "-2", "-w", "20", "tabs.txt"},
			want: "a\tb\n",
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
			name:     "an invalid option",
			args:     []string{"-q"},
			wantErr:  "pr: invalid option -- 'q'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a missing option argument",
			args:     []string{"-h"},
			wantErr:  "pr: option requires an argument -- 'h'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid page number",
			args:     []string{"+0"},
			wantErr:  "pr: invalid page range '0'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid page option",
			args:     []string{"+x"},
			wantErr:  "pr: invalid option -- 'x'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid column count",
			args:     []string{"-0"},
			wantErr:  "pr: -columns must be 1 or more\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid page length",
			args:     []string{"-l", "0"},
			wantErr:  "pr: number of lines must be 1 or more: '0'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid offset",
			args:     []string{"-o", "-1"},
			wantErr:  "pr: invalid offset: '-1'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid width",
			args:     []string{"-w", "0"},
			wantErr:  "pr: -w width must be 1 or more: '0'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid number width",
			args:     []string{"-n0"},
			wantErr:  "pr: -n width must be 1 or more: '0'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid -e argument",
			args:     []string{"-e:x"},
			wantErr:  "pr: invalid value for -e: 'x'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid -s argument",
			args:     []string{"-s||"},
			wantErr:  "pr: invalid value for -s: '||'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "-a requires columns",
			args:     []string{"-a"},
			wantErr:  "pr: -a flag requires multiple columns\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "-m cannot be used with columns",
			args:     []string{"-2", "-m", "a", "b"},
			wantErr:  "pr: -m cannot be used with multiple columns\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "-m cannot be used with -a",
			args:     []string{"-m", "-a", "a", "b"},
			wantErr:  "pr: -m cannot be used with -a\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a too narrow page width",
			fsys:     testFS(),
			args:     []string{"-t", "-3", "-w", "2", "lorem.txt"},
			wantErr:  "pr: page width too narrow\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an unknown long option",
			args:     []string{"--bogus"},
			wantErr:  "pr: unrecognized option '--bogus'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a long option without its argument",
			args:     []string{"--width"},
			wantErr:  "pr: option '--width' requires an argument\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a long option which takes no argument",
			args:     []string{"--across=1"},
			wantErr:  "pr: option '--across=1' doesn't allow an argument\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "--pages without a range",
			args:     []string{"--pages"},
			wantErr:  "pr: option '--pages' requires an argument\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a negative page range",
			args:     []string{"--pages=1:-1"},
			wantErr:  "pr: invalid page range '1:-1'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a decreasing page range",
			args:     []string{"+2:1"},
			wantErr:  "pr: invalid page range '2:1'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a non numeric page range",
			args:     []string{"+2:x"},
			wantErr:  "pr: invalid page range '2:x'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid starting line number",
			args:     []string{"-N", "x"},
			wantErr:  "pr: invalid starting line number: 'x'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid -W width",
			args:     []string{"-W", "0"},
			wantErr:  "pr: -W width must be 1 or more: '0'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a long separator without its argument",
			args:     []string{"--separator"},
			wantErr:  "pr: option '--separator' requires an argument\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid column count",
			args:     []string{"--columns=0"},
			wantErr:  "pr: invalid number of columns: '0'\nTry 'pr --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a missing file",
			fsys:     testFS(),
			args:     []string{"missing.txt"},
			wantErr:  "pr: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing file doesn't stop the next one",
			fsys:     testFS(),
			args:     []string{"-t", "missing.txt", "lorem.txt"},
			want:     "lorem\nipsum\ndolor\n",
			wantErr:  "pr: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "-r suppresses the diagnostic but not the error",
			fsys:     testFS(),
			args:     []string{"-t", "-r", "missing.txt", "lorem.txt"},
			want:     "lorem\nipsum\ndolor\n",
			wantExit: 1,
		},
		{
			name:     "an operand after -- is a file",
			fsys:     testFS(),
			args:     []string{"-t", "--", "-l"},
			wantErr:  "pr: -l: file does not exist\n",
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
		FS:     testFS(),
	}
	err := New(rt).Run(t.Context(), "-t")

	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if got := stderr.String(); got != "pr: standard input: read failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "pr: standard input: read failed\n")
	}
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("lorem\n"),
		Stdout: errorWriter{err: writeErr},
		Stderr: stderr,
		FS:     testFS(),
	}
	err := New(rt).Run(t.Context(), "-t")

	if code := exitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
	if got := stderr.String(); got != "pr: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "pr: write error: write failed\n")
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
		FS:     testFS(),
	}
	err := New(rt).Run(ctx, "-t")

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

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "pr (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}

func TestRunShowChars(t *testing.T) {
	tests := []testCase{
		{
			name:  "-c prints control characters in hat notation",
			stdin: strings.NewReader("a\x07b\x7f\n"),
			args:  []string{"-t", "-c"},
			want:  "a^Gb^?\n",
		},
		{
			name:  "-c prints other nonprinting characters in octal",
			stdin: strings.NewReader("a\xc3b\n"),
			args:  []string{"-t", "-c"},
			want:  "a\\303b\n",
		},
		{
			name:  "-c keeps tabs as they are",
			stdin: strings.NewReader("a\tb\n"),
			args:  []string{"-t", "-c"},
			want:  "a\tb\n",
		},
		{
			name:  "-v prints nonprinting characters in octal",
			stdin: strings.NewReader("a\x07b\n"),
			args:  []string{"-t", "-v"},
			want:  "a\\007b\n",
		},
		{
			name:  "-v keeps tabs as they are",
			stdin: strings.NewReader("a\tb\n"),
			args:  []string{"-t", "-v"},
			want:  "a\tb\n",
		},
		{
			name:  "-v counts the octal escapes towards the column width",
			stdin: strings.NewReader("a\x07b\n"),
			args:  []string{"-t", "-W4", "-v"},
			want:  "a\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDateFormat(t *testing.T) {
	tests := []testCase{
		{
			name: "-D formats the header date as date +FORMAT does",
			fsys: testFS(),
			args: []string{"-l", "12", "-D", "Year %Y", "lorem.txt"},
			want: "\n\nYear 2020                        lorem.txt" +
				"                        Page 1\n\n\n" + padded("lorem\nipsum\n", 2, 2) +
				"\n\nYear 2020                        lorem.txt" +
				"                        Page 2\n\n\n" + padded("dolor\n", 2, 1),
		},
		{
			name: "-D accepts a literal text in place of the date",
			fsys: testFS(),
			args: []string{"-l", "12", "-D", "-- Date/Time --", "lorem.txt"},
			want: "\n\n-- Date/Time --                     lorem.txt" +
				"                     Page 1\n\n\n" + padded("lorem\nipsum\n", 2, 2) +
				"\n\n-- Date/Time --                     lorem.txt" +
				"                     Page 2\n\n\n" + padded("dolor\n", 2, 1),
		},
		{
			name:    "POSIXLY_CORRECT gives the POSIX date layout",
			fsys:    testFS(),
			args:    []string{"-l", "12", "lorem.txt"},
			environ: []string{"POSIXLY_CORRECT=1"},
			want: "\n\nJan  2 03:04 2020                    lorem.txt" +
				"                    Page 1\n\n\n" + padded("lorem\nipsum\n", 2, 2) +
				"\n\nJan  2 03:04 2020                    lorem.txt" +
				"                    Page 2\n\n\n" + padded("dolor\n", 2, 1),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunJoin(t *testing.T) {
	tests := []testCase{
		{
			name: "-J joins full lines with the default separator",
			fsys: testFS(),
			args: []string{"-t", "-2", "-J", "odd.txt"},
			want: "a\td\nb\te\nc\n",
		},
		{
			name: "-J keeps the -S separator",
			fsys: testFS(),
			args: []string{"-t", "-2", "-J", "-S::", "odd.txt"},
			want: "a::d\nb::e\nc\n",
		},
		{
			name: "-J turns off the -W truncation",
			fsys: testFS(),
			args: []string{"-t", "-W", "10", "-J", "-2", "odd.txt"},
			want: "a\td\nb\te\nc\n",
		},
		{
			name: "-J with -m joins the lines of the files",
			fsys: testFS(),
			args: []string{"-t", "-J", "-m", "lorem.txt", "odd.txt"},
			want: "lorem\ta\nipsum\tb\ndolor\tc\n\td\n\te\n",
		},
		{
			name:  "-J with -m joins the lines of the standard input",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-t", "--join-lines", "--merge"},
			want:  "a\nb\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFirstLineNumber(t *testing.T) {
	tests := []testCase{
		{
			name: "-N starts the numbering at the given number",
			fsys: testFS(),
			args: []string{"-t", "-n", "-N", "10", "lorem.txt"},
			want: "   10\tlorem\n   11\tipsum\n   12\tdolor\n",
		},
		{
			name: "-N may be negative",
			fsys: testFS(),
			args: []string{"-t", "-n", "-N", "-1", "lorem.txt"},
			want: "   -1\tlorem\n    0\tipsum\n    1\tdolor\n",
		},
		{
			name: "-N overflows keeping the low-order digits",
			fsys: testFS(),
			args: []string{"-t", "-n2", "-N", "98", "ten.txt"},
			want: "98\t1\n99\t2\n00\t3\n01\t4\n02\t5\n03\t6\n04\t7\n05\t8\n06\t9\n07\t10\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSepString(t *testing.T) {
	tests := []testCase{
		{
			name: "-S separates the columns with a string",
			fsys: testFS(),
			args: []string{"-t", "-2", "-S::", "odd.txt"},
			want: "a\t\t\t\t   ::d\nb\t\t\t\t   ::e\nc\n",
		},
		{
			name: "-S keeps the column alignment and truncation",
			fsys: testFS(),
			args: []string{"-t", "-2", "-S::", "long.txt"},
			want: strings.Repeat("x", 35) + "\n",
		},
		{
			name: "a bare -S gives an empty separator",
			fsys: testFS(),
			args: []string{"-t", "-2", "-S", "odd.txt"},
			want: "a\t\t\t\t    d\nb\t\t\t\t    e\nc\n",
		},
		{
			name: "-S dominates a later -s",
			fsys: testFS(),
			args: []string{"-t", "-2", "-S::", "-s|", "odd.txt"},
			want: "a\t\t\t\t   ::d\nb\t\t\t\t   ::e\nc\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOmitPagination(t *testing.T) {
	tests := []testCase{
		{
			name:  "-T ignores a form feed in the middle of a line",
			stdin: strings.NewReader("a\fb\n"),
			args:  []string{"-t", "-T"},
			want:  "ab\n",
		},
		{
			name:  "-T drops a form feed with its newline",
			stdin: strings.NewReader("a\n\f\nb\n"),
			args:  []string{"-t", "-T"},
			want:  "a\nb\n",
		},
		{
			name:  "-t keeps a form feed in the middle of a line",
			stdin: strings.NewReader("a\fb\n"),
			args:  []string{"-t"},
			want:  "a\n\fb\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunPageWidth(t *testing.T) {
	tests := []testCase{
		{
			name: "-W truncates single column lines",
			fsys: testFS(),
			args: []string{"-t", "-W", "10", "long.txt"},
			want: strings.Repeat("x", 10) + "\n",
		},
		{
			name: "-w does not truncate single column lines",
			fsys: testFS(),
			args: []string{"-t", "-w", "10", "long.txt"},
			want: strings.Repeat("x", 80) + "\n",
		},
		{
			name: "a later -w turns the truncation off, as GNU pr does",
			fsys: testFS(),
			args: []string{"-t", "-W", "10", "-w", "40", "long.txt"},
			want: strings.Repeat("x", 80) + "\n",
		},
		{
			name: "-W truncates the lines numbered with -n",
			fsys: testFS(),
			args: []string{"-t", "-W", "10", "-n2", "long.txt"},
			want: " 1\t" + strings.Repeat("x", 2) + "\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunPageRanges(t *testing.T) {
	tests := []testCase{
		{
			name: "+FIRST:LAST stops with the last page",
			fsys: testFS(),
			args: []string{"+2:3", "-l", "12", "lorem.txt"},
			want: header("lorem.txt", 2) + padded("dolor\n", 2, 1),
		},
		{
			name: "--pages selects the same range",
			fsys: testFS(),
			args: []string{"--pages=2:2", "-l", "12", "lorem.txt"},
			want: header("lorem.txt", 2) + padded("dolor\n", 2, 1),
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
			name: "the long forms of the page layout options",
			fsys: testFS(),
			args: []string{
				"--omit-header", "--columns=2", "--sep-string=|",
				"--length=8", "ten.txt",
			},
			want: "1\t\t\t\t   |6\n2\t\t\t\t   |7\n3\t\t\t\t   |8\n" +
				"4\t\t\t\t   |9\n5\t\t\t\t   |10\n",
		},
		{
			name: "the long forms of the merge options",
			fsys: testFS(),
			args: []string{
				"--merge", "--omit-header", "--number-lines=.2",
				"--indent=2", "odd.txt",
			},
			want: "   1.a\n   2.b\n   3.c\n   4.d\n   5.e\n",
		},
		{
			name: "the long forms of the tab options",
			fsys: testFS(),
			args: []string{"-t", "--expand-tabs=4", "tabs.txt"},
			want: "a   b\n",
		},
		{
			name: "the long form of the double spacing option",
			fsys: testFS(),
			args: []string{"-t", "--double-space", "lorem.txt"},
			want: "lorem\n\nipsum\n\ndolor\n\n",
		},
		{
			name: "the long form of the form feed option",
			fsys: testFS(),
			args: []string{"--form-feed", "--length=12", "lorem.txt"},
			want: header("lorem.txt", 1) + "lorem\nipsum\n\f" +
				header("lorem.txt", 2) + "dolor\n\f",
		},
		{
			name: "the long form of the header option",
			fsys: testFS(),
			args: []string{"--header=My Header", "-l", "12", "lorem.txt"},
			want: header("My Header", 1) + padded("lorem\nipsum\n", 2, 2) +
				header("My Header", 2) + padded("dolor\n", 2, 1),
		},
		{
			name:     "the long form of the warnings option",
			fsys:     testFS(),
			args:     []string{"--no-file-warnings", "-t", "missing.txt", "lorem.txt"},
			want:     "lorem\nipsum\ndolor\n",
			wantExit: 1,
		},
		{
			name: "long options take their argument in the next argument",
			fsys: testFS(),
			args: []string{"--length", "12", "lorem.txt"},
			want: header("lorem.txt", 1) + padded("lorem\nipsum\n", 2, 2) +
				header("lorem.txt", 2) + padded("dolor\n", 2, 1),
		},
		{
			name: "--header takes its argument in the next argument",
			fsys: testFS(),
			args: []string{"--header", "My Header", "--length", "12", "lorem.txt"},
			want: header("My Header", 1) + padded("lorem\nipsum\n", 2, 2) +
				header("My Header", 2) + padded("dolor\n", 2, 1),
		},
		{
			name: "--pages takes its range in the next argument",
			fsys: testFS(),
			args: []string{"--pages", "2:2", "--length", "12", "lorem.txt"},
			want: header("lorem.txt", 2) + padded("dolor\n", 2, 1),
		},
		{
			name: "--width takes its value in the next argument",
			fsys: testFS(),
			args: []string{"-t", "--width", "10", "long.txt"},
			want: strings.Repeat("x", 80) + "\n",
		},
		{
			name: "--columns takes its count in the next argument",
			fsys: testFS(),
			args: []string{"-t", "--columns", "2", "odd.txt"},
			want: "a\t\t\t\t    d\nb\t\t\t\t    e\nc\n",
		},
		{
			name: "--separator takes its character in the next argument",
			fsys: testFS(),
			args: []string{"-t", "--separator", "|", "--columns", "2", "odd.txt"},
			want: "a|d\nb|e\nc\n",
		},
		{
			name: "--first-line-number takes its value in the next argument",
			fsys: testFS(),
			args: []string{"-t", "-n", "--first-line-number", "10", "lorem.txt"},
			want: "   10\tlorem\n   11\tipsum\n   12\tdolor\n",
		},
		{
			name: "--date-format takes its format in the next argument",
			fsys: testFS(),
			args: []string{"--date-format", "Year %Y", "-l", "12", "lorem.txt"},
			want: "\n\nYear 2020                        lorem.txt" +
				"                        Page 1\n\n\n" + padded("lorem\nipsum\n", 2, 2) +
				"\n\nYear 2020                        lorem.txt" +
				"                        Page 2\n\n\n" + padded("dolor\n", 2, 1),
		},
		{
			name: "--indent takes its margin in the next argument",
			fsys: testFS(),
			args: []string{"--indent", "4", "-l", "12", "odd.txt"},
			want: headerOffset("odd.txt", 1, 4) + padded("    a\n    b\n", 2, 2) +
				headerOffset("odd.txt", 2, 4) + padded("    c\n    d\n", 2, 2) +
				headerOffset("odd.txt", 3, 4) + padded("    e\n", 2, 1),
		},
		{
			name: "the long form of the printing across option",
			fsys: testFS(),
			args: []string{"-t", "--across", "--columns=2", "odd.txt"},
			want: "a\t\t\t\t    b\nc\t\t\t\t    d\ne\n",
		},
		{
			name: "the long form of the width option",
			fsys: testFS(),
			args: []string{"-t", "--page-width=10", "long.txt"},
			want: strings.Repeat("x", 10) + "\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}
