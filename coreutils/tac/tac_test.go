package tac

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

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

// testCase is the fields of a testutil.Case plus a standard output the case
// brings itself: testutil.Once always captures the output on a buffer, so
// the write error case, which fails the writes, needs this one.
type testCase struct {
	Name     string
	Stdin    io.Reader
	Stdout   io.Writer
	FS       fsx.FS
	Dir      string
	Args     []string
	Want     string
	WantErr  string
	WantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	if test.Stdout == nil {
		testutil.Once(t, New, testutil.Case{
			Name:     test.Name,
			Stdin:    test.Stdin,
			FS:       test.FS,
			Dir:      test.Dir,
			Args:     test.Args,
			Want:     test.Want,
			WantErr:  test.WantErr,
			WantExit: test.WantExit,
		})
		return
	}

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      test.Stdin,
		Stdout:     test.Stdout,
		Stderr:     stderr,
		FS:         test.FS,
		WorkingDir: test.Dir,
	}
	err := New(rt).Run(t.Context(), test.Args...)

	if code := testutil.ExitCode(t, err); code != test.WantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.WantExit, err)
	}
	if got := stderr.String(); got != test.WantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.WantErr)
	}
}

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			Name:  "no operand reads the standard input",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Want:  "c\nb\na\n",
		},
		{
			Name:  "empty standard input",
			Stdin: strings.NewReader(""),
			Want:  "",
		},
		{
			Name:  "a single record without a separator",
			Stdin: strings.NewReader("a"),
			Want:  "a",
		},
		{
			Name:  "a single separator",
			Stdin: strings.NewReader("\n"),
			Want:  "\n",
		},
		{
			Name:  "a single record",
			Stdin: strings.NewReader("a\n"),
			Want:  "a\n",
		},
		{
			Name:  "no trailing separator",
			Stdin: strings.NewReader("a\nb"),
			Want:  "ba\n",
		},
		{
			Name:  "empty records are kept",
			Stdin: strings.NewReader("a\n\nb\n"),
			Want:  "b\n\na\n",
		},
		{
			Name:  "a long record",
			Stdin: strings.NewReader("1234567\n8\n"),
			Want:  "8\n1234567\n",
		},
		{
			Name: "the records of a file in reverse",
			FS:   testFS(),
			Args: []string{"a.txt"},
			Want: "e\nd\nc\nb\na\n",
		},
		{
			Name: "a file without a trailing separator",
			FS:   testFS(),
			Args: []string{"nonl.txt"},
			Want: "cb\na\n",
		},
		{
			Name: "empty file",
			FS:   testFS(),
			Args: []string{"empty.txt"},
			Want: "",
		},
		{
			Name:  "the '-' operand reads the standard input",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-"},
			Want:  "b\na\n",
		},
		{
			Name:  "the standard input is read once per '-' operand",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-", "-"},
			Want:  "b\na\n",
		},
		{
			Name: "pathnames are resolved from the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"inner.txt"},
			Want: "inner\n",
		},
		{
			Name: "double dash allows a file named like an option",
			FS:   testFS(),
			Args: []string{"--", "-b"},
			Want: "dash-b\n",
		},
		{
			Name:  "double dash without operands reads the standard input",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"--"},
			Want:  "a\n",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSeparator(t *testing.T) {
	tests := []testCase{
		{
			Name: "the separator option",
			FS:   testFS(),
			Args: []string{"-s", ":", "colon.txt"},
			Want: "three:two:one:",
		},
		{
			Name: "separator attached to the option",
			FS:   testFS(),
			Args: []string{"-s:", "colon.txt"},
			Want: "three:two:one:",
		},
		{
			Name: "long separator option",
			FS:   testFS(),
			Args: []string{"--separator=:", "colon.txt"},
			Want: "three:two:one:",
		},
		{
			Name: "long separator option with the argument on the next argument",
			FS:   testFS(),
			Args: []string{"--separator", ":", "colon.txt"},
			Want: "three:two:one:",
		},
		{
			Name:  "multi-character separator",
			Stdin: strings.NewReader("a::b::c"),
			Args:  []string{"-s", "::"},
			Want:  "cb::a::",
		},
		{
			Name: "a separator missing from the input copies it unchanged",
			FS:   testFS(),
			Args: []string{"-s", "XY", "a.txt"},
			Want: "a\nb\nc\nd\ne\n",
		},
		{
			Name:  "an empty separator means the NUL byte",
			Stdin: strings.NewReader("a\x00b"),
			Args:  []string{"-s", ""},
			Want:  "ba\x00",
		},
		{
			Name: "the NUL byte separator with a trailing separator",
			FS:   testFS(),
			Args: []string{"-s", "", "nul.txt"},
			Want: "b\x00a\x00",
		},
		{
			Name:  "a trailing separator ends the last record",
			Stdin: strings.NewReader("a:b:"),
			Args:  []string{"-s", ":"},
			Want:  "b:a:",
		},
		{
			Name:  "a leading separator starts an empty record",
			Stdin: strings.NewReader(":a:b"),
			Args:  []string{"-s", ":"},
			Want:  "ba::",
		},
		{
			Name:  "matches are right-aligned without overlapping",
			Stdin: strings.NewReader("aabab"),
			Args:  []string{"-s", "ab"},
			Want:  "abaab",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunEscapes(t *testing.T) {
	tests := []testCase{
		{
			Name:  "tab escape",
			Stdin: strings.NewReader("a\tb\tc"),
			Args:  []string{"-s", `\t`},
			Want:  "cb\ta\t",
		},
		{
			Name:  "newline escape behaves like the default",
			Stdin: strings.NewReader("a\nb"),
			Args:  []string{"-s", `\n`},
			Want:  "ba\n",
		},
		{
			Name:  "carriage return escape",
			Stdin: strings.NewReader("a\rb\rc"),
			Args:  []string{"-s", `\r`},
			Want:  "cb\ra\r",
		},
		{
			Name:  "backslash escape",
			Stdin: strings.NewReader(`a\b\c`),
			Args:  []string{"-s", `\\`},
			Want:  `cb\a\`,
		},
		{
			Name:  "alert escape",
			Stdin: strings.NewReader("a\x07b"),
			Args:  []string{"-s", `\a`},
			Want:  "ba\x07",
		},
		{
			Name:  "backspace escape",
			Stdin: strings.NewReader("a\x08b"),
			Args:  []string{"-s", `\b`},
			Want:  "ba\x08",
		},
		{
			Name:  "form feed escape",
			Stdin: strings.NewReader("a\x0cb"),
			Args:  []string{"-s", `\f`},
			Want:  "ba\x0c",
		},
		{
			Name:  "vertical tab escape",
			Stdin: strings.NewReader("a\x0bb"),
			Args:  []string{"-s", `\v`},
			Want:  "ba\x0b",
		},
		{
			Name:  "three-digit octal escape",
			Stdin: strings.NewReader("a0b0"),
			Args:  []string{"-s", `\060`},
			Want:  "b0a0",
		},
		{
			Name:  "octal escape",
			Stdin: strings.NewReader("aAbAc"),
			Args:  []string{"-s", `\101`},
			Want:  "cbAaA",
		},
		{
			Name:  "zero escape is the NUL byte",
			Stdin: strings.NewReader("a\x00b"),
			Args:  []string{"-s", `\0`},
			Want:  "ba\x00",
		},
		{
			Name:  "an unknown escape is kept as is",
			Stdin: strings.NewReader(`a\qb`),
			Args:  []string{"-s", `\q`},
			Want:  `ba\q`,
		},
		{
			Name:  "a separator made only of escapes",
			Stdin: strings.NewReader("a\tb\tc\t"),
			Args:  []string{"-s", `\t`},
			Want:  "c\tb\ta\t",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunBefore(t *testing.T) {
	tests := []testCase{
		{
			Name:  "the separator is attached before the record",
			Stdin: strings.NewReader("\na\nb\nc"),
			Args:  []string{"-b"},
			Want:  "\nc\nb\na",
		},
		{
			Name:  "a final record without a separator",
			Stdin: strings.NewReader("a\nb\nc"),
			Args:  []string{"-b"},
			Want:  "\nc\nba",
		},
		{
			Name:  "a trailing separator is its own last record",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-b"},
			Want:  "\n\nc\nba",
		},
		{
			Name:  "a single record",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-b"},
			Want:  "\na",
		},
		{
			Name:  "empty input",
			Stdin: strings.NewReader(""),
			Args:  []string{"-b"},
			Want:  "",
		},
		{
			Name:  "input without a separator",
			Stdin: strings.NewReader("abc"),
			Args:  []string{"-b"},
			Want:  "abc",
		},
		{
			Name:  "a custom separator",
			Stdin: strings.NewReader(":a:b:c"),
			Args:  []string{"-s", ":", "-b"},
			Want:  ":c:b:a",
		},
		{
			Name:  "long before option",
			Stdin: strings.NewReader("\na\nb\nc"),
			Args:  []string{"--before"},
			Want:  "\nc\nb\na",
		},
		{
			Name:  "grouped with the separator",
			Stdin: strings.NewReader(":a:b:c"),
			Args:  []string{"-bs:"},
			Want:  ":c:b:a",
		},
		{
			Name: "a file with a leading separator",
			FS:   testFS(),
			Args: []string{"-b", "lead.txt"},
			Want: "\nc\nb\na",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRegex(t *testing.T) {
	tests := []testCase{
		{
			Name:  "the default separator as a regular expression",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-r"},
			Want:  "b\na\n",
		},
		{
			Name: "each file is reversed separately",
			FS:   testFS(),
			Args: []string{"-r", "a.txt", "lines.txt"},
			Want: "e\nd\nc\nb\na\nl3\nl2\nl1\n",
		},
		{
			Name: "a regular expression separator",
			FS:   testFS(),
			Args: []string{"-r", "-s", `\._+`, "dots.txt"},
			Want: "4._3.___2.__1._",
		},
		{
			Name: "long regex options",
			FS:   testFS(),
			Args: []string{"--regex", `--separator=\._+`, "dots.txt"},
			Want: "4._3.___2.__1._",
		},
		{
			Name: "records between matches",
			FS:   testFS(),
			Args: []string{"-r", "-s", `\._+`, "dots2.txt"},
			Want: "4._3.___2.__1._b.__a.___",
		},
		{
			Name: "before with a regular expression",
			FS:   testFS(),
			Args: []string{"-b", "-r", "-s", `\._+`, "before.txt"},
			Want: ".___4.__3._2._1",
		},
		{
			Name: "before with a longer input",
			FS:   testFS(),
			Args: []string{"-b", "-r", "-s", `\._+`, "before2.txt"},
			Want: ".___4.__3._2._1.____z.___y.__x",
		},
		{
			Name:  "the start anchor matches at newlines",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-r", "-s", "^"},
			Want:  "c\nb\na\n",
		},
		{
			Name:  "the end anchor matches at newlines",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-r", "-s", "$"},
			Want:  "\n\nc\nba",
		},
		{
			Name:  "empty records with both anchors",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-r", "-s", "^$"},
			Want:  "a\nb\nc\n",
		},
		{
			Name:  "alternation",
			Stdin: strings.NewReader("aXbYc"),
			Args:  []string{"-r", "-s", "X|Y"},
			Want:  "cbYaX",
		},
		{
			Name:     "an invalid pattern",
			Args:     []string{"-r", "-s", "["},
			WantErr:  "tac: invalid separator: error parsing regexp: missing closing ]: `[`\nTry 'tac --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty separator is invalid with -r",
			Args:     []string{"-r", "-s", ""},
			WantErr:  "tac: separator cannot be empty with -r\nTry 'tac --help' for more information.\n",
			WantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFiles(t *testing.T) {
	tests := []testCase{
		{
			Name: "multiple files are reversed each",
			FS:   testFS(),
			Args: []string{"a.txt", "lines.txt"},
			Want: "e\nd\nc\nb\na\nl3\nl2\nl1\n",
		},
		{
			Name:  "the standard input among the files",
			Stdin: strings.NewReader("x\n"),
			FS:    testFS(),
			Args:  []string{"-", "lines.txt"},
			Want:  "x\nl3\nl2\nl1\n",
		},
		{
			Name:     "a missing file doesn't stop the next ones",
			FS:       testFS(),
			Args:     []string{"missing.txt", "lines.txt"},
			Want:     "l3\nl2\nl1\n",
			WantErr:  "tac: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "an empty file among the files",
			FS:   testFS(),
			Args: []string{"empty.txt", "lines.txt"},
			Want: "l3\nl2\nl1\n",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	boom := errors.New("boom")

	tests := []testCase{
		{
			Name:     "invalid option",
			Args:     []string{"-z"},
			WantErr:  "tac: invalid option -- 'z'\nTry 'tac --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unrecognized long option",
			Args:     []string{"--bogus"},
			WantErr:  "tac: unrecognized option '--bogus'\nTry 'tac --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "long option that doesn't allow an argument",
			Args:     []string{"--before=x"},
			WantErr:  "tac: option '--before=x' doesn't allow an argument\nTry 'tac --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing separator argument",
			Args:     []string{"-s"},
			WantErr:  "tac: option requires an argument -- 's'\nTry 'tac --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing long separator argument",
			Args:     []string{"--separator"},
			WantErr:  "tac: option '--separator' requires an argument\nTry 'tac --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing file",
			FS:       testFS(),
			Args:     []string{"missing.txt"},
			WantErr:  "tac: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "invalid zero-length file name",
			Args:     []string{""},
			WantErr:  "tac: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "no file system available",
			Args:     []string{"a.txt"},
			WantErr:  "tac: a.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input available",
			WantErr:  "tac: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "write error",
			Stdin:    strings.NewReader("a\nb\n"),
			Stdout:   testutil.ErrorWriter{Err: boom},
			WantErr:  "tac: write error: boom\n",
			WantExit: 1,
		},
		{
			Name:     "read error",
			FS:       testutil.ReaderFS(testutil.ErrorReader{Err: boom}),
			Args:     []string{"f"},
			WantErr:  "tac: f: boom\n",
			WantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	rt := &coreutils.Runtime{
		Stdin:  &testutil.CancelReader{Cancel: cancel},
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
	stdout := &testutil.CancelWriter{Cancel: cancel}
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
	testutil.Cancelled(t, New, strings.NewReader("a\nb\n"), testFS(), "a.txt", "lines.txt")
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "tac", usage)
}
