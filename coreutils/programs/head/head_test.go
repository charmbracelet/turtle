package head

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// testFS holds the fixtures used by the tests:
//
//	a.txt:    "alpha\n"
//	b.txt:    "beta\n"
//	empty.txt: ""
//	nonl.txt: "one\ntwo\nthree" (no trailing newline)
//	lines.txt: "l1\nl2\nl3\nl4\nl5\n"
//	many.txt: 12 lines, "01\n" to "12\n"
//	bytes.txt: "0123456789"
//	big.txt: 4100 bytes, "0123456789" repeated
//	nlines.txt: 2000 lines, "0000\n" to "1999\n"
//	nul.txt: "a\x00b\x00c\x00"
//	nulpart.txt: "x\x00y" (a final item without the NUL)
//	"-n":     "dash-n\n" (a file named like an option)
//	"-5":     "dash-5\n" (a file named like the obsolete form)
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemoryFS {
	return fsx.MemoryFS{
		"a.txt":         {Data: []byte("alpha\n")},
		"b.txt":         {Data: []byte("beta\n")},
		"empty.txt":     {Data: []byte("")},
		"nonl.txt":      {Data: []byte("one\ntwo\nthree")},
		"lines.txt":     {Data: []byte("l1\nl2\nl3\nl4\nl5\n")},
		"many.txt":      {Data: []byte("01\n02\n03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n")},
		"bytes.txt":     {Data: []byte("0123456789")},
		"big.txt":       {Data: bytes.Repeat([]byte("0123456789"), 410)},
		"nlines.txt":    {Data: linesFile(2000)},
		"nul.txt":       {Data: []byte("a\x00b\x00c\x00")},
		"nulpart.txt":   {Data: []byte("x\x00y")},
		"-n":            {Data: []byte("dash-n\n")},
		"-5":            {Data: []byte("dash-5\n")},
		"sub/inner.txt": {Data: []byte("inner\n")},
	}
}

// linesFile builds the contents of a file with n numbered lines.
func linesFile(n int) []byte {
	var data []byte
	for i := range n {
		data = fmt.Appendf(data, "%04d\n", i)
	}
	return data
}

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no operands reads the standard input",
			Stdin: strings.NewReader("hello\n"),
			Want:  "hello\n",
		},
		{
			Name:  "empty standard input",
			Stdin: strings.NewReader(""),
			Want:  "",
		},
		{
			Name:  "standard input without a trailing newline",
			Stdin: strings.NewReader("no trailing newline"),
			Want:  "no trailing newline",
		},
		{
			Name: "single file",
			FS:   testFS(),
			Args: []string{"a.txt"},
			Want: "alpha\n",
		},
		{
			Name: "fewer than ten lines copies the whole file",
			FS:   testFS(),
			Args: []string{"lines.txt"},
			Want: "l1\nl2\nl3\nl4\nl5\n",
		},
		{
			Name: "defaults to the first ten lines",
			FS:   testFS(),
			Args: []string{"many.txt"},
			Want: "01\n02\n03\n04\n05\n06\n07\n08\n09\n10\n",
		},
		{
			Name: "file without a trailing newline",
			FS:   testFS(),
			Args: []string{"nonl.txt"},
			Want: "one\ntwo\nthree",
		},
		{
			Name: "empty file",
			FS:   testFS(),
			Args: []string{"empty.txt"},
			Want: "",
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
			Args: []string{"--", "-n"},
			Want: "dash-n\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLines(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the first lines of a file",
			FS:   testFS(),
			Args: []string{"-n", "2", "lines.txt"},
			Want: "l1\nl2\n",
		},
		{
			Name: "number joined to the option",
			FS:   testFS(),
			Args: []string{"-n2", "lines.txt"},
			Want: "l1\nl2\n",
		},
		{
			Name: "zero lines",
			FS:   testFS(),
			Args: []string{"-n", "0", "lines.txt"},
			Want: "",
		},
		{
			Name: "more lines than the file copies it in its entirety",
			FS:   testFS(),
			Args: []string{"-n", "100", "lines.txt"},
			Want: "l1\nl2\nl3\nl4\nl5\n",
		},
		{
			Name: "a final line without a newline counts as a line",
			FS:   testFS(),
			Args: []string{"-n", "2", "nonl.txt"},
			Want: "one\ntwo\n",
		},
		{
			Name: "counting the final incomplete line",
			FS:   testFS(),
			Args: []string{"-n", "3", "nonl.txt"},
			Want: "one\ntwo\nthree",
		},
		{
			Name:  "lines of the standard input",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-n", "2"},
			Want:  "a\nb\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunBytes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the first bytes of a file",
			FS:   testFS(),
			Args: []string{"-c", "3", "bytes.txt"},
			Want: "012",
		},
		{
			Name: "number joined to the option",
			FS:   testFS(),
			Args: []string{"-c3", "bytes.txt"},
			Want: "012",
		},
		{
			Name: "zero bytes",
			FS:   testFS(),
			Args: []string{"-c", "0", "bytes.txt"},
			Want: "",
		},
		{
			Name: "more bytes than the file copies it in its entirety",
			FS:   testFS(),
			Args: []string{"-c", "20", "bytes.txt"},
			Want: "0123456789",
		},
		{
			Name: "bytes can cut through a line",
			FS:   testFS(),
			Args: []string{"-c", "4", "lines.txt"},
			Want: "l1\nl",
		},
		{
			Name: "bytes ending at a newline",
			FS:   testFS(),
			Args: []string{"-c", "3", "lines.txt"},
			Want: "l1\n",
		},
		{
			Name:  "bytes of the standard input",
			Stdin: strings.NewReader("hello world"),
			Args:  []string{"-c", "5"},
			Want:  "hello",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunMultipleFiles(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "each file is preceded by a header",
			FS:   testFS(),
			Args: []string{"a.txt", "b.txt"},
			Want: "==> a.txt <==\nalpha\n\n==> b.txt <==\nbeta\n",
		},
		{
			Name: "headers with a line count",
			FS:   testFS(),
			Args: []string{"-n", "1", "lines.txt", "many.txt"},
			Want: "==> lines.txt <==\nl1\n\n==> many.txt <==\n01\n",
		},
		{
			Name: "the same file twice",
			FS:   testFS(),
			Args: []string{"a.txt", "a.txt"},
			Want: "==> a.txt <==\nalpha\n\n==> a.txt <==\nalpha\n",
		},
		{
			Name: "empty file",
			FS:   testFS(),
			Args: []string{"a.txt", "empty.txt"},
			Want: "==> a.txt <==\nalpha\n\n==> empty.txt <==\n",
		},
		{
			Name:     "a failing file doesn't stop the others",
			FS:       testFS(),
			Args:     []string{"missing.txt", "a.txt"},
			Want:     "==> a.txt <==\nalpha\n",
			WantErr:  "head: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a failing file between valid ones",
			FS:       testFS(),
			Args:     []string{"a.txt", "missing.txt", "b.txt"},
			Want:     "==> a.txt <==\nalpha\n\n==> b.txt <==\nbeta\n",
			WantErr:  "head: missing.txt: file does not exist\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunStandardInputOperand(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("stdin\n"),
			Args:  []string{"-"},
			Want:  "stdin\n",
		},
		{
			Name:  "dash combined with a file",
			Stdin: strings.NewReader("stdin\n"),
			FS:    testFS(),
			Args:  []string{"-", "a.txt"},
			Want:  "==> - <==\nstdin\n\n==> a.txt <==\nalpha\n",
		},
		{
			Name:  "double dash with no operands reads the standard input",
			Stdin: strings.NewReader("stdin\n"),
			Args:  []string{"--"},
			Want:  "stdin\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLastOptionWins(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "bytes after lines",
			FS:   testFS(),
			Args: []string{"-n", "2", "-c", "4", "lines.txt"},
			Want: "l1\nl",
		},
		{
			Name: "lines after bytes",
			FS:   testFS(),
			Args: []string{"-c", "4", "-n", "2", "lines.txt"},
			Want: "l1\nl2\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "lines takes its value in the next argument",
			FS:   testFS(),
			Args: []string{"--lines", "2", "lines.txt"},
			Want: "l1\nl2\n",
		},
		{
			Name: "lines takes its value glued to it",
			FS:   testFS(),
			Args: []string{"--lines=2", "lines.txt"},
			Want: "l1\nl2\n",
		},
		{
			Name: "bytes takes its value glued to it",
			FS:   testFS(),
			Args: []string{"--bytes=4", "bytes.txt"},
			Want: "0123",
		},
		{
			Name:     "lines requires an argument",
			FS:       testFS(),
			Args:     []string{"--lines"},
			WantErr:  "head: option '--lines' requires an argument\nTry 'head --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "missing file",
			FS:       testFS(),
			Args:     []string{"missing.txt"},
			WantErr:  "head: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "zero-length file name",
			FS:       testFS(),
			Args:     []string{""},
			WantErr:  "head: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("boom")},
			Args:     []string{"-"},
			WantErr:  "head: -: boom\n",
			WantExit: 1,
		},
		{
			Name:     "no file system available",
			Args:     []string{"a.txt"},
			WantErr:  "head: a.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input available",
			WantErr:  "head: standard input: no standard input available\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunInvalidUsage(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "invalid option",
			Args:     []string{"-x", "a.txt"},
			FS:       testFS(),
			WantErr:  "head: invalid option -- 'x'\nTry 'head --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unrecognized long option",
			Args:     []string{"--number", "a.txt"},
			FS:       testFS(),
			WantErr:  "head: unrecognized option '--number'\nTry 'head --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid number",
			Args:     []string{"-n", "x", "a.txt"},
			FS:       testFS(),
			WantErr:  "head: invalid number of lines: 'x'\nTry 'head --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing option argument",
			Args:     []string{"-n"},
			FS:       testFS(),
			WantErr:  "head: option requires an argument -- 'n'\nTry 'head --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "non-decimal number",
			Args:     []string{"-c", "1.5", "a.txt"},
			FS:       testFS(),
			WantErr:  "head: invalid number of bytes: '1.5'\nTry 'head --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "empty number",
			Args:     []string{"-n", "", "a.txt"},
			FS:       testFS(),
			WantErr:  "head: invalid number of lines: ''\nTry 'head --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid suffix",
			Args:     []string{"-c", "1J", "a.txt"},
			FS:       testFS(),
			WantErr:  "head: invalid number of bytes: '1J'\nTry 'head --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "suffix too large",
			Args:     []string{"-c", "1Z", "a.txt"},
			FS:       testFS(),
			WantErr:  "head: invalid number of bytes: '1Z'\nTry 'head --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "obsolete number after another option",
			Args:     []string{"-n", "2", "-5", "a.txt"},
			FS:       testFS(),
			WantErr:  "head: invalid trailing option -- 5\nTry 'head --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunSuffixes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a kibi prefix multiplies the byte count",
			FS:   testFS(),
			Args: []string{"-c", "1K", "big.txt"},
			Want: strings.Repeat("0123456789", 103)[:1024],
		},
		{
			Name: "a kilo prefix multiplies the byte count by 1000",
			FS:   testFS(),
			Args: []string{"-c", "1KB", "big.txt"},
			Want: strings.Repeat("0123456789", 101)[:1000],
		},
		{
			Name: "a binary prefix multiplies the byte count",
			FS:   testFS(),
			Args: []string{"-c", "10KiB", "big.txt"},
			Want: string(testFS()["big.txt"].Data),
		},
		{
			Name: "a block suffix counts 512 bytes",
			FS:   testFS(),
			Args: []string{"-c", "3b", "big.txt"},
			Want: strings.Repeat("0123456789", 154)[:1536],
		},
		{
			Name: "suffixes are case insensitive",
			FS:   testFS(),
			Args: []string{"-c", "1kb", "big.txt"},
			Want: strings.Repeat("0123456789", 101)[:1000],
		},
		{
			Name: "a mebi prefix multiplies the byte count",
			FS:   testFS(),
			Args: []string{"-c", "2M", "big.txt"},
			Want: string(testFS()["big.txt"].Data),
		},
		{
			Name: "a suffix multiplies the line count",
			FS:   testFS(),
			Args: []string{"-n", "1000c", "nlines.txt"},
			Want: string(linesFile(1000)),
		},
		{
			Name: "a kibi suffix multiplies the line count",
			FS:   testFS(),
			Args: []string{"-n", "1k", "nlines.txt"},
			Want: string(linesFile(1024)),
		},
		{
			Name: "a suffix on a long option",
			FS:   testFS(),
			Args: []string{"--bytes=1K", "big.txt"},
			Want: strings.Repeat("0123456789", 103)[:1024],
		},
		{
			Name:  "a suffix on the standard input",
			Stdin: strings.NewReader(strings.Repeat("x", 2000)),
			Args:  []string{"-c", "1K"},
			Want:  strings.Repeat("x", 1024),
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunNegativeCounts(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "all but the last lines",
			FS:   testFS(),
			Args: []string{"-n", "-2", "lines.txt"},
			Want: "l1\nl2\nl3\n",
		},
		{
			Name: "more lines to elide than the file has",
			FS:   testFS(),
			Args: []string{"-n", "-10", "lines.txt"},
			Want: "",
		},
		{
			Name: "all but the last bytes",
			FS:   testFS(),
			Args: []string{"-c", "-4", "lines.txt"},
			Want: "l1\nl2\nl3\nl4",
		},
		{
			Name: "more bytes to elide than the file has",
			FS:   testFS(),
			Args: []string{"-c", "-20", "lines.txt"},
			Want: "",
		},
		{
			Name: "zero lines to elide copies everything",
			FS:   testFS(),
			Args: []string{"-n", "-0", "lines.txt"},
			Want: "l1\nl2\nl3\nl4\nl5\n",
		},
		{
			Name: "a negative count on a final incomplete line",
			FS:   testFS(),
			Args: []string{"-n", "-1", "nonl.txt"},
			Want: "one\ntwo\n",
		},
		{
			Name:  "a negative count on the standard input",
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			Args:  []string{"-n", "-1"},
			Want:  "a\nb\nc\n",
		},
		{
			Name: "a leading plus is accepted and ignored on lines",
			FS:   testFS(),
			Args: []string{"-n", "+2", "lines.txt"},
			Want: "l1\nl2\n",
		},
		{
			Name: "a leading plus is accepted and ignored on bytes",
			FS:   testFS(),
			Args: []string{"-c", "+3", "bytes.txt"},
			Want: "012",
		},
		{
			Name: "a leading plus joined to the option",
			FS:   testFS(),
			Args: []string{"-n+2", "lines.txt"},
			Want: "l1\nl2\n",
		},
		{
			Name: "a negative count on a long option",
			FS:   testFS(),
			Args: []string{"--lines=-2", "lines.txt"},
			Want: "l1\nl2\nl3\n",
		},
		{
			Name: "a negative count with a suffix",
			FS:   testFS(),
			Args: []string{"-c", "-1K", "big.txt"},
			Want: strings.Repeat("0123456789", 308)[:3076],
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunObsoleteSyntax(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a lone number counts lines",
			FS:   testFS(),
			Args: []string{"-5", "many.txt"},
			Want: "01\n02\n03\n04\n05\n",
		},
		{
			Name:  "a lone number on the standard input",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-2"},
			Want:  "a\nb\n",
		},
		{
			Name: "c counts bytes",
			FS:   testFS(),
			Args: []string{"-3c", "bytes.txt"},
			Want: "012",
		},
		{
			Name: "b counts 512-byte blocks",
			FS:   testFS(),
			Args: []string{"-2b", "big.txt"},
			Want: strings.Repeat("0123456789", 103)[:1024],
		},
		{
			Name: "k counts 1024 bytes",
			FS:   testFS(),
			Args: []string{"-1k", "big.txt"},
			Want: strings.Repeat("0123456789", 103)[:1024],
		},
		{
			Name: "m counts 1024 kibibytes",
			FS:   testFS(),
			Args: []string{"-2m", "big.txt"},
			Want: string(testFS()["big.txt"].Data),
		},
		{
			Name: "l counts lines",
			FS:   testFS(),
			Args: []string{"-2l", "lines.txt"},
			Want: "l1\nl2\n",
		},
		{
			Name: "the last multiplier letter wins",
			FS:   testFS(),
			Args: []string{"-2bk", "big.txt"},
			Want: strings.Repeat("0123456789", 205)[:2048],
		},
		{
			Name: "c resets the multiplier",
			FS:   testFS(),
			Args: []string{"-2bc", "bytes.txt"},
			Want: "01",
		},
		{
			Name: "q never prints headers",
			FS:   testFS(),
			Args: []string{"-5q", "a.txt", "b.txt"},
			Want: "alpha\nbeta\n",
		},
		{
			Name: "v always prints headers",
			FS:   testFS(),
			Args: []string{"-5v", "a.txt"},
			Want: "==> a.txt <==\nalpha\n",
		},
		{
			Name: "z terminates the items with a NUL byte",
			FS:   testFS(),
			Args: []string{"-2z", "nul.txt"},
			Want: "a\x00b\x00",
		},
		{
			Name: "the obsolete form is combined with normal options",
			FS:   testFS(),
			Args: []string{"-5", "-c", "3", "bytes.txt"},
			Want: "012",
		},
		{
			Name:     "an invalid trailing option",
			FS:       testFS(),
			Args:     []string{"-5x", "a.txt"},
			WantErr:  "head: invalid trailing option -- x\nTry 'head --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid trailing option after digits and flags",
			FS:       testFS(),
			Args:     []string{"-5qzK", "a.txt"},
			WantErr:  "head: invalid trailing option -- K\nTry 'head --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name: "a number alone is not the obsolete form after --",
			FS:   testFS(),
			Args: []string{"--", "-5"},
			Want: "dash-5\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunHeaders(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "quiet suppresses the headers",
			FS:   testFS(),
			Args: []string{"-q", "a.txt", "b.txt"},
			Want: "alpha\nbeta\n",
		},
		{
			Name: "quiet on a long form",
			FS:   testFS(),
			Args: []string{"--quiet", "a.txt", "b.txt"},
			Want: "alpha\nbeta\n",
		},
		{
			Name: "silent suppresses the headers",
			FS:   testFS(),
			Args: []string{"--silent", "a.txt", "b.txt"},
			Want: "alpha\nbeta\n",
		},
		{
			Name:     "quiet still reads every file",
			FS:       testFS(),
			Args:     []string{"-q", "a.txt", "missing.txt"},
			Want:     "alpha\n",
			WantErr:  "head: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "verbose prints a header for a single file",
			FS:   testFS(),
			Args: []string{"-v", "a.txt"},
			Want: "==> a.txt <==\nalpha\n",
		},
		{
			Name:  "verbose prints a header for the standard input",
			Stdin: strings.NewReader("stdin\n"),
			Args:  []string{"-v"},
			Want:  "==> standard input <==\nstdin\n",
		},
		{
			Name: "verbose on a long form",
			FS:   testFS(),
			Args: []string{"--verbose", "a.txt"},
			Want: "==> a.txt <==\nalpha\n",
		},
		{
			Name: "the last of quiet and verbose wins",
			FS:   testFS(),
			Args: []string{"-q", "-v", "a.txt"},
			Want: "==> a.txt <==\nalpha\n",
		},
		{
			Name: "the last of verbose and quiet wins",
			FS:   testFS(),
			Args: []string{"-v", "-q", "a.txt"},
			Want: "alpha\n",
		},
		{
			Name: "verbose prints headers with a byte count",
			FS:   testFS(),
			Args: []string{"-c", "2", "-v", "a.txt"},
			Want: "==> a.txt <==\nal",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunZeroTerminated(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the first NUL-terminated items",
			FS:   testFS(),
			Args: []string{"-z", "-n", "2", "nul.txt"},
			Want: "a\x00b\x00",
		},
		{
			Name: "a final item without a NUL counts as an item",
			FS:   testFS(),
			Args: []string{"-z", "-n", "1", "nulpart.txt"},
			Want: "x\x00",
		},
		{
			Name: "the final item without a NUL is copied as is",
			FS:   testFS(),
			Args: []string{"-z", "-n", "2", "nulpart.txt"},
			Want: "x\x00y",
		},
		{
			Name: "zero-terminated on a long form",
			FS:   testFS(),
			Args: []string{"--zero-terminated", "-n", "1", "nul.txt"},
			Want: "a\x00",
		},
		{
			Name: "more items than the file has",
			FS:   testFS(),
			Args: []string{"-z", "-n", "10", "nul.txt"},
			Want: "a\x00b\x00c\x00",
		},
		{
			Name: "all but the last NUL-terminated item",
			FS:   testFS(),
			Args: []string{"-z", "-n", "-1", "nul.txt"},
			Want: "a\x00b\x00",
		},
		{
			Name: "bytes are unaffected by zero-termination",
			FS:   testFS(),
			Args: []string{"-z", "-c", "3", "nul.txt"},
			Want: "a\x00b",
		},
		{
			Name: "lines are still newline-terminated without -z",
			FS:   testFS(),
			Args: []string{"-n", "1", "nul.txt"},
			Want: "a\x00b\x00c\x00",
		},
		{
			Name:  "zero-terminated items of the standard input",
			Stdin: strings.NewReader("a\x00b\x00c\x00"),
			Args:  []string{"-z", "-n", "2"},
			Want:  "a\x00b\x00",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunWriteError(t *testing.T) {
	rt := &coreutils.Runtime{
		Stdout: testutil.ErrorWriter{Err: errors.New("boom")},
		Stderr: new(bytes.Buffer),
		FS:     testFS(),
	}
	err := New(rt).Run(t.Context(), "a.txt")
	if code := testutil.ExitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if got, want := rt.Stderr.(*bytes.Buffer).String(), "head: write error: boom\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}

func TestRunContextCanceled(t *testing.T) {
	testutil.Cancelled(t, New, &testutil.CancelReader{}, nil)
}

func TestRunDoesNotReadUnusedStandardInput(t *testing.T) {
	stdin := &testutil.SpyReader{}
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     testFS(),
	}
	if err := New(rt).Run(t.Context(), "a.txt"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdin.Reads != 0 {
		t.Error("the standard input was read")
	}
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "head", usage)
}
