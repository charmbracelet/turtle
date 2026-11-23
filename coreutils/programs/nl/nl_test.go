package nl

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// testFS holds the fixtures used by the tests:
//
//	lorem.txt:  "lorem\nipsum\ndolor\n"
//	blanks.txt: "a\n\n\nb\n\n\n\nc\n"
//	pages.txt:  "\:\:\:\nh1\nh2\n\:\:\nb1\n\nb3\n\:\nf1\nf2\n"
//	regex.txt:  "foo\nbar\nfoobar\n"
//	nonl.txt:   "lorem\nipsum" (no trailing newline)
//	empty.txt:  ""
func testFS() fsx.MemoryFS {
	return fsx.MemoryFS{
		"lorem.txt":  {Data: []byte("lorem\nipsum\ndolor\n")},
		"blanks.txt": {Data: []byte("a\n\n\nb\n\n\n\nc\n")},
		"pages.txt":  {Data: []byte("\\:\\:\\:\nh1\nh2\n\\:\\:\nb1\n\nb3\n\\:\nf1\nf2\n")},
		"regex.txt":  {Data: []byte("foo\nbar\nfoobar\n")},
		"nonl.txt":   {Data: []byte("lorem\nipsum")},
		"empty.txt":  {Data: []byte("")},
	}
}

func TestRunInput(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a file",
			FS:   testFS(),
			Args: []string{"lorem.txt"},
			Want: "     1\tlorem\n     2\tipsum\n     3\tdolor\n",
		},
		{
			Name:  "standard input",
			Stdin: strings.NewReader("lorem\nipsum\n"),
			Want:  "     1\tlorem\n     2\tipsum\n",
		},
		{
			Name:  "a dash operand is the standard input",
			Stdin: strings.NewReader("lorem\n"),
			Args:  []string{"-"},
			Want:  "     1\tlorem\n",
		},
		{
			Name:     "options stop at the first operand",
			FS:       testFS(),
			Args:     []string{"lorem.txt", "-b", "a"},
			Want:     "     1\tlorem\n     2\tipsum\n     3\tdolor\n",
			WantErr:  "nl: -b: file does not exist\nnl: a: file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "multiple files are numbered as one document",
			FS:   testFS(),
			Args: []string{"lorem.txt", "nonl.txt"},
			Want: "     1\tlorem\n     2\tipsum\n     3\tdolor\n     4\tlorem\n     5\tipsum",
		},
		{
			Name:     "a file which can't be read doesn't stop the remaining ones",
			FS:       testFS(),
			Args:     []string{"missing.txt", "lorem.txt"},
			Want:     "     1\tlorem\n     2\tipsum\n     3\tdolor\n",
			WantErr:  "nl: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:  "the standard input is concatenated between files",
			FS:    testFS(),
			Stdin: strings.NewReader("b\n"),
			Args:  []string{"lorem.txt", "-"},
			Want:  "     1\tlorem\n     2\tipsum\n     3\tdolor\n     4\tb\n",
		},
		{
			Name:     "missing file",
			FS:       testFS(),
			Args:     []string{"missing.txt"},
			WantErr:  "nl: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:  "blank lines are not numbered by default",
			Stdin: strings.NewReader("a\n\nb\n"),
			Want:  "     1\ta\n      \t\n     2\tb\n",
		},
		{
			Name:  "a line holding only blanks is numbered",
			Stdin: strings.NewReader("a\n   \nb\n"),
			Want:  "     1\ta\n     2\t   \n     3\tb\n",
		},
		{
			Name: "an unterminated last line is numbered",
			FS:   testFS(),
			Args: []string{"nonl.txt"},
			Want: "     1\tlorem\n     2\tipsum",
		},
		{
			Name: "an empty file produces no output",
			FS:   testFS(),
			Args: []string{"empty.txt"},
			Want: "",
		},
		{
			Name: "options after -- are operands",
			FS:   testFS(),
			Args: []string{"--", "lorem.txt"},
			Want: "     1\tlorem\n     2\tipsum\n     3\tdolor\n",
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunStyles(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "number all lines",
			FS:   testFS(),
			Args: []string{"-b", "a", "blanks.txt"},
			Want: "     1\ta\n     2\t\n     3\t\n     4\tb\n" +
				"     5\t\n     6\t\n     7\t\n     8\tc\n",
		},
		{
			Name: "number no lines",
			FS:   testFS(),
			Args: []string{"-b", "n", "lorem.txt"},
			Want: "      \tlorem\n      \tipsum\n      \tdolor\n",
		},
		{
			Name: "number non-empty lines",
			FS:   testFS(),
			Args: []string{"-b", "t", "blanks.txt"},
			Want: "     1\ta\n      \t\n      \t\n     2\tb\n" +
				"      \t\n      \t\n      \t\n     3\tc\n",
		},
		{
			Name: "number lines matching a regular expression",
			FS:   testFS(),
			Args: []string{"-bpfoo", "regex.txt"},
			Want: "     1\tfoo\n      \tbar\n     2\tfoobar\n",
		},
		{
			Name: "number lines matching an anchored regular expression",
			FS:   testFS(),
			Args: []string{"-b", "p^bar$", "regex.txt"},
			Want: "      \tfoo\n     1\tbar\n      \tfoobar\n",
		},
		{
			Name: "an empty regular expression matches every line",
			FS:   testFS(),
			Args: []string{"-bp", "blanks.txt"},
			Want: "     1\ta\n     2\t\n     3\t\n     4\tb\n" +
				"     5\t\n     6\t\n     7\t\n     8\tc\n",
		},
		{
			Name:  "a basic regular expression keeps its grouping syntax",
			Stdin: strings.NewReader("ab\naab\nb\n"),
			Args:  []string{"-b", `p^\(a*b\)$`},
			Want:  "     1\tab\n     2\taab\n     3\tb\n",
		},
		{
			Name:     "an invalid body style",
			FS:       testFS(),
			Args:     []string{"-b", "q", "lorem.txt"},
			WantErr:  "nl: invalid body numbering style: 'q'\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid header style",
			FS:       testFS(),
			Args:     []string{"-h", "q", "lorem.txt"},
			WantErr:  "nl: invalid header numbering style: 'q'\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid footer style",
			FS:       testFS(),
			Args:     []string{"-f", "q", "lorem.txt"},
			WantErr:  "nl: invalid footer numbering style: 'q'\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid regular expression",
			FS:       testFS(),
			Args:     []string{"-b", `p\(`, "lorem.txt"},
			WantErr:  "nl: unmatched \\(\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunSections(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "header and footer lines are not numbered by default",
			FS:   testFS(),
			Args: []string{"pages.txt"},
			Want: "\n      \th1\n      \th2\n\n     1\tb1\n      \t\n     2\tb3\n" +
				"\n      \tf1\n      \tf2\n",
		},
		{
			Name: "numbering restarts at each section",
			FS:   testFS(),
			Args: []string{"-h", "a", "-f", "a", "-b", "a", "pages.txt"},
			Want: "\n     1\th1\n     2\th2\n\n     1\tb1\n     2\t\n     3\tb3\n" +
				"\n     1\tf1\n     2\tf2\n",
		},
		{
			Name: "numbering restarts at each logical page",
			Stdin: strings.NewReader("\\:\\:\\:\nh1\n\\:\\:\nb1\n" +
				"\\:\\:\\:\nh2\n\\:\\:\nb2\n"),
			Args: []string{"-h", "a", "-b", "a"},
			Want: "\n     1\th1\n\n     1\tb1\n\n     1\th2\n\n     1\tb2\n",
		},
		{
			Name: "numbering does not restart with -p",
			Stdin: strings.NewReader("\\:\\:\\:\nh1\n\\:\\:\nb1\n" +
				"\\:\\:\\:\nh2\n\\:\\:\nb2\n"),
			Args: []string{"-p", "-h", "a", "-b", "a"},
			Want: "\n     1\th1\n\n     2\tb1\n\n     3\th2\n\n     4\tb2\n",
		},
		{
			Name:  "the input starts in a logical page body",
			Stdin: strings.NewReader("a\n\\:\\:\\:\nh\n"),
			Args:  []string{"-h", "a"},
			Want:  "     1\ta\n\n     1\th\n",
		},
		{
			Name:  "custom delimiter characters",
			Stdin: strings.NewReader("!+!+!+\nh\n!+!+\nb\n!+\nf\n"),
			Args:  []string{"-d", "!+", "-h", "a", "-f", "a"},
			Want:  "\n     1\th\n\n     1\tb\n\n     1\tf\n",
		},
		{
			Name:  "a single delimiter character implies a colon",
			Stdin: strings.NewReader("!:!:!:\nh\n!:!:\nb\n"),
			Args:  []string{"-d", "!", "-h", "a"},
			Want:  "\n     1\th\n\n     1\tb\n",
		},
		{
			Name:  "an empty delimiter disables section matching",
			Stdin: strings.NewReader("\\:\\:\\:\nb\n"),
			Args:  []string{"-d", ""},
			Want:  "     1\t\\:\\:\\:\n     2\tb\n",
		},
		{
			Name:  "a line holding extra characters is not a delimiter",
			Stdin: strings.NewReader("\\: \nb\n"),
			Want:  "     1\t\\: \n     2\tb\n",
		},
		{
			Name: "numbering is not reset between files",
			FS: fsx.MemoryFS{
				"page1.txt": {Data: []byte("\\:\\:\\:\nh1\n\\:\\:\nb1\n")},
				"page2.txt": {Data: []byte("\\:\\:\nb2\n")},
			},
			Args: []string{"-h", "a", "page1.txt", "page2.txt"},
			Want: "\n     1\th1\n\n     1\tb1\n\n     1\tb2\n",
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "header numbering",
			Stdin: strings.NewReader("\\:\\:\\:\nh\n"),
			Args:  []string{"--header-numbering=a"},
			Want:  "\n     1\th\n",
		},
		{
			Name:  "body numbering",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"--body-numbering=n"},
			Want:  "      \ta\n",
		},
		{
			Name:  "footer numbering",
			Stdin: strings.NewReader("\\:\nf\n"),
			Args:  []string{"--footer-numbering=a"},
			Want:  "\n     1\tf\n",
		},
		{
			Name:  "section delimiter",
			Stdin: strings.NewReader("!+!+!+\nh\n"),
			Args:  []string{"--section-delimiter=!+", "--header-numbering=a"},
			Want:  "\n     1\th\n",
		},
		{
			Name:  "starting line number",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"--starting-line-number=10"},
			Want:  "    10\ta\n    11\tb\n",
		},
		{
			Name:  "line increment",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"--line-increment=5"},
			Want:  "     1\ta\n     6\tb\n",
		},
		{
			Name:  "join blank lines",
			Stdin: strings.NewReader("a\n\n\nb\n"),
			Args:  []string{"--body-numbering=a", "--join-blank-lines=2"},
			Want:  "     1\ta\n      \t\n     2\t\n     3\tb\n",
		},
		{
			Name:  "number format and width",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"--number-format=rz", "--number-width=3"},
			Want:  "001\ta\n002\tb\n",
		},
		{
			Name:  "number separator",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"--number-separator=::"},
			Want:  "     1::a\n",
		},
		{
			Name:  "an argument in the next argument",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"--number-width", "3"},
			Want:  "  1\ta\n",
		},
		{
			Name:  "no renumber",
			Stdin: strings.NewReader("\\:\\:\\:\nh\n\\:\\:\nb\n\\:\nc\n"),
			Args:  []string{"--no-renumber", "--header-numbering=a"},
			Want:  "\n     1\th\n\n     2\tb\n\n      \tc\n",
		},
		{
			Name:     "a missing argument",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--number-width"},
			WantErr:  "nl: option '--number-width' requires an argument\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an unrecognized option",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--bogus"},
			WantErr:  "nl: unrecognized option '--bogus'\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunNumbering(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "starting number and increment",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-v", "10", "-i", "10"},
			Want:  "    10\ta\n    20\tb\n    30\tc\n",
		},
		{
			Name:  "a negative increment",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-i", "-1"},
			Want:  "     1\ta\n     0\tb\n    -1\tc\n",
		},
		{
			Name:  "a negative starting number",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-v", "-5"},
			Want:  "    -5\ta\n    -4\tb\n",
		},
		{
			Name:  "left justified numbers",
			Stdin: strings.NewReader("ab\ncd\n"),
			Args:  []string{"-n", "ln", "-w", "3"},
			Want:  "1  \tab\n2  \tcd\n",
		},
		{
			Name:  "right justified numbers with leading zeros",
			Stdin: strings.NewReader("ab\ncd\n"),
			Args:  []string{"-n", "rz"},
			Want:  "000001\tab\n000002\tcd\n",
		},
		{
			Name:  "a number wider than the field",
			Stdin: strings.NewReader("ab\n"),
			Args:  []string{"-v", "1234567"},
			Want:  "1234567\tab\n",
		},
		{
			Name:  "a custom separator",
			Stdin: strings.NewReader("ab\n"),
			Args:  []string{"-s", "##"},
			Want:  "     1##ab\n",
		},
		{
			Name:  "an empty separator",
			Stdin: strings.NewReader("ab\n"),
			Args:  []string{"-s", ""},
			Want:  "     1ab\n",
		},
		{
			Name:  "blank lines are grouped with -l",
			Stdin: strings.NewReader("a\n\n\n\nb\n"),
			Args:  []string{"-b", "a", "-l", "2"},
			Want:  "     1\ta\n      \t\n     2\t\n      \t\n     3\tb\n",
		},
		{
			Name:  "blank line groups restart after a numbered line",
			Stdin: strings.NewReader("a\n\n\n\n\nb\n"),
			Args:  []string{"-b", "a", "-l", "2"},
			Want:  "     1\ta\n      \t\n     2\t\n      \t\n     3\t\n     4\tb\n",
		},
		{
			Name:  "-l does not apply to the default style",
			Stdin: strings.NewReader("a\n\n\n\nb\n"),
			Args:  []string{"-l", "2"},
			Want:  "     1\ta\n      \t\n      \t\n      \t\n     2\tb\n",
		},
		{
			Name:  "an unnumbered line has a blank number field",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-b", "n"},
			Want:  "      \ta\n",
		},
		{
			Name:  "the blank field uses spaces even with -n rz",
			Stdin: strings.NewReader("a\n\nb\n"),
			Args:  []string{"-n", "rz", "-b", "a", "-l", "2"},
			Want:  "000001\ta\n      \t\n000002\tb\n",
		},
		{
			Name:  "the blank field follows the width and the separator",
			Stdin: strings.NewReader("a\n\nb\n"),
			Args:  []string{"-b", "t", "-w", "3", "-s", "##"},
			Want:  "  1##a\n   ##\n  2##b\n",
		},
		{
			Name:     "an invalid starting number",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-v", "x"},
			WantErr:  "nl: invalid starting line number: 'x'\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid increment",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-i", "x"},
			WantErr:  "nl: invalid line number increment: 'x'\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid blank line count",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-l", "0"},
			WantErr:  "nl: invalid line number of blank lines: '0'\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid numbering format",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-n", "q"},
			WantErr:  "nl: invalid line numbering format: 'q'\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid width",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-w", "0"},
			WantErr:  "nl: invalid line number field width: '0'\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid option",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-q"},
			WantErr:  "nl: invalid option -- 'q'\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing option argument",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-b"},
			WantErr:  "nl: option requires an argument -- 'b'\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunOverflow(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the greatest number is still printed",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-v", "9223372036854775807"},
			Want:  "9223372036854775807\ta\n",
		},
		{
			Name:     "an overflowing increment is an error on the next line",
			Stdin:    strings.NewReader("a\nb\n"),
			Args:     []string{"-v", "9223372036854775807"},
			Want:     "9223372036854775807\ta\n",
			WantErr:  "nl: line number overflow\n",
			WantExit: 1,
		},
		{
			Name:     "an overflowing negative increment is an error",
			Stdin:    strings.NewReader("a\nb\n"),
			Args:     []string{"-v", "-9223372036854775808", "-i", "-1"},
			Want:     "-9223372036854775808\ta\n",
			WantErr:  "nl: line number overflow\n",
			WantExit: 1,
		},
		{
			Name:     "an out of range starting number",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-v", "9223372036854775808"},
			WantErr:  "nl: invalid starting line number: '9223372036854775808'\nTry 'nl --help' for more information.\n",
			WantExit: 1,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunReadError(t *testing.T) {
	readErr := errors.New("read failed")

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  testutil.ErrorReader{Err: readErr},
		Stdout: new(bytes.Buffer),
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context())

	if code := testutil.ExitCode(t, err); code != 1 {
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
		Stdout: testutil.ErrorWriter{Err: writeErr},
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context())

	if code := testutil.ExitCode(t, err); code != 1 {
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
		testutil.Cancelled(t, New, strings.NewReader("lorem\n"), nil)
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		testutil.Cancelled(t, New, &testutil.CancelReader{}, nil)
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "nl", usage)
}
