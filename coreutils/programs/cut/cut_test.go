package cut

import (
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// testFS holds the fixtures used by the tests:
//
//	a.txt:         "abcdefg\nxyz\n"
//	passwd.txt:    passwd-like lines with ":" fields
//	tabs.txt:      lines with tab-separated fields, and a plain one
//	utf8.txt:      "ábçde\n日本語x\n" (multi-byte characters)
//	invalid.txt:   "a\xffb\n" (an invalid UTF-8 byte)
//	empty.txt:     ""
//	nonl.txt:      "no newline" (no trailing newline)
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemoryFS {
	return fsx.MemoryFS{
		"a.txt":         {Data: []byte("abcdefg\nxyz\n")},
		"passwd.txt":    {Data: []byte("root:x:0:0:root:/root:/bin/sh\ndaemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin\n")},
		"tabs.txt":      {Data: []byte("f1\tf2\tf3\nplain\n")},
		"utf8.txt":      {Data: []byte("ábçde\n日本語x\n")},
		"invalid.txt":   {Data: []byte{'a', 0xff, 'b', '\n'}},
		"empty.txt":     {Data: []byte("")},
		"nonl.txt":      {Data: []byte("no newline")},
		"sub/inner.txt": {Data: []byte("inner\n")},
	}
}

func TestRunBytes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a single byte",
			Stdin: strings.NewReader("abcdefg\nxyz\n"),
			Args:  []string{"-b", "1"},
			Want:  "a\nx\n",
		},
		{
			Name:  "a list of bytes",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "1,3,5"},
			Want:  "ace\n",
		},
		{
			Name:  "a range of bytes",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "2-4"},
			Want:  "bcd\n",
		},
		{
			Name:  "a range from the start",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "-3"},
			Want:  "abc\n",
		},
		{
			Name:  "a range to the end",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "5-"},
			Want:  "efg\n",
		},
		{
			Name:  "an attached list",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b2-4"},
			Want:  "bcd\n",
		},
		{
			Name:  "a blank-separated list",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "1 3"},
			Want:  "ac\n",
		},
		{
			Name:  "positions follow the order of the input data",
			Stdin: strings.NewReader("abcdefghi\n"),
			Args:  []string{"-b", "6,2,4-7,1"},
			Want:  "abdefg\n",
		},
		{
			Name:  "overlapping ranges are written once",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "1-3,2-4"},
			Want:  "abcd\n",
		},
		{
			Name:  "repeated positions are written once",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "2,2,2"},
			Want:  "b\n",
		},
		{
			Name:  "positions past the end of the line are not an error",
			Stdin: strings.NewReader("abc\n"),
			Args:  []string{"-b", "5-10"},
			Want:  "\n",
		},
		{
			Name:  "a range partially past the end of the line is clamped",
			Stdin: strings.NewReader("abc\n"),
			Args:  []string{"-b", "2-10"},
			Want:  "bc\n",
		},
		{
			Name:  "an empty line stays empty",
			Stdin: strings.NewReader("\n"),
			Args:  []string{"-b", "1"},
			Want:  "\n",
		},
		{
			Name: "a missing final newline is added",
			FS:   testFS(),
			Args: []string{"-b", "1-2", "nonl.txt"},
			Want: "no\n",
		},
		{
			Name: "an empty file produces no output",
			FS:   testFS(),
			Args: []string{"-b", "1", "empty.txt"},
			Want: "",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunBytesNoSplit(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a byte inside a character selects the whole character",
			FS:   testFS(),
			Args: []string{"-b", "2", "-n", "utf8.txt"},
			Want: "á\n\n",
		},
		{
			Name: "the first byte of a multi-byte character alone selects nothing",
			FS:   testFS(),
			Args: []string{"-b", "1", "-n", "utf8.txt"},
			Want: "\n\n",
		},
		{
			Name: "a whole character is selected when its last byte is",
			FS:   testFS(),
			Args: []string{"-b", "3", "-n", "utf8.txt"},
			Want: "b\n日\n",
		},
		{
			Name: "a range ending mid character stops before it",
			FS:   testFS(),
			Args: []string{"-b", "2-3", "-n", "utf8.txt"},
			Want: "áb\n日\n",
		},
		{
			Name:  "ranges are partitioned at character boundaries",
			Stdin: strings.NewReader("aé日\n"),
			Args:  []string{"-b", "1-2", "-n"},
			Want:  "a\n",
		},
		{
			Name:  "the complementary range gets the rest",
			Stdin: strings.NewReader("aé日\n"),
			Args:  []string{"-b", "3-", "-n"},
			Want:  "é日\n",
		},
		{
			Name: "a range that ends before the first character is dropped",
			FS:   testFS(),
			Args: []string{"-b", "1-1", "-n", "utf8.txt"},
			Want: "\n\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunChars(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a multi-byte character",
			FS:   testFS(),
			Args: []string{"-c", "1", "utf8.txt"},
			Want: "á\n日\n",
		},
		{
			Name: "a range of characters",
			FS:   testFS(),
			Args: []string{"-c", "2-3", "utf8.txt"},
			Want: "bç\n本語\n",
		},
		{
			Name: "characters follow the order of the input data",
			FS:   testFS(),
			Args: []string{"-c", "4,2", "utf8.txt"},
			Want: "bd\n本x\n",
		},
		{
			Name: "positions past the end of the line are not an error",
			FS:   testFS(),
			Args: []string{"-c", "3-10", "utf8.txt"},
			Want: "çde\n語x\n",
		},
		{
			Name: "a byte that starts no valid encoding counts as one character",
			FS:   testFS(),
			Args: []string{"-c", "2", "invalid.txt"},
			Want: "\xff\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunFields(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the default delimiter is a tab",
			FS:   testFS(),
			Args: []string{"-f", "2", "tabs.txt"},
			Want: "f2\nplain\n",
		},
		{
			Name: "a custom delimiter",
			FS:   testFS(),
			Args: []string{"-d", ":", "-f", "1", "passwd.txt"},
			Want: "root\ndaemon\n",
		},
		{
			Name: "an attached delimiter",
			FS:   testFS(),
			Args: []string{"-d:", "-f", "6", "passwd.txt"},
			Want: "/root\n/usr/sbin\n",
		},
		{
			Name: "a list of fields",
			FS:   testFS(),
			Args: []string{"-d:", "-f", "1,6", "passwd.txt"},
			Want: "root:/root\ndaemon:/usr/sbin\n",
		},
		{
			Name: "a range of fields",
			FS:   testFS(),
			Args: []string{"-d:", "-f", "2-3", "passwd.txt"},
			Want: "x:0\nx:1\n",
		},
		{
			Name: "a range to the last field",
			FS:   testFS(),
			Args: []string{"-d:", "-f", "6-", "passwd.txt"},
			Want: "/root:/bin/sh\n/usr/sbin:/usr/sbin/nologin\n",
		},
		{
			Name: "fields follow the order of the input data",
			FS:   testFS(),
			Args: []string{"-d:", "-f", "3,1", "passwd.txt"},
			Want: "root:0\ndaemon:1\n",
		},
		{
			Name:  "missing fields are skipped",
			Stdin: strings.NewReader("a:b:c\n"),
			Args:  []string{"-d:", "-f", "1,4,2"},
			Want:  "a:b\n",
		},
		{
			Name:  "a line with only missing fields comes out empty",
			Stdin: strings.NewReader("a:b\n"),
			Args:  []string{"-d:", "-f", "3"},
			Want:  "\n",
		},
		{
			Name:  "empty fields are preserved",
			Stdin: strings.NewReader("a::c\n"),
			Args:  []string{"-d:", "-f", "1-3"},
			Want:  "a::c\n",
		},
		{
			Name: "a line without the delimiter is passed through",
			FS:   testFS(),
			Args: []string{"-f", "2", "tabs.txt"},
			Want: "f2\nplain\n",
		},
		{
			Name: "s suppresses lines without the delimiter",
			FS:   testFS(),
			Args: []string{"-f", "2", "-s", "tabs.txt"},
			Want: "f2\n",
		},
		{
			Name:  "an empty line is passed through",
			Stdin: strings.NewReader("\n"),
			Args:  []string{"-f", "1"},
			Want:  "\n",
		},
		{
			Name:  "s suppresses an empty line",
			Stdin: strings.NewReader("\n"),
			Args:  []string{"-f", "1", "-s"},
			Want:  "",
		},
		{
			Name:  "the delimiter is not special in the middle of quotes",
			Stdin: strings.NewReader(`a:"b:c"` + "\n"),
			Args:  []string{"-d:", "-f", "2"},
			Want:  "\"b\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunFiles(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "multiple files are concatenated in order",
			FS:   testFS(),
			Args: []string{"-b", "1", "a.txt", "nonl.txt"},
			Want: "a\nx\nn\n",
		},
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("stdin\n"),
			Args:  []string{"-b", "1-3", "-"},
			Want:  "std\n",
		},
		{
			Name:  "dash between files reads the standard input at that point",
			Stdin: strings.NewReader("stdin\n"),
			FS:    testFS(),
			Args:  []string{"-b", "1", "a.txt", "-", "nonl.txt"},
			Want:  "a\nx\ns\nn\n",
		},
		{
			Name: "pathnames are resolved from the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"-b", "1-5", "inner.txt"},
			Want: "inner\n",
		},
		{
			Name: "the last list wins when the same option is repeated",
			FS:   testFS(),
			Args: []string{"-b", "1", "-b", "2", "a.txt"},
			Want: "b\ny\n",
		},
		{
			Name:     "options are not parsed after the first operand",
			FS:       testFS(),
			Args:     []string{"-b", "1", "a.txt", "-n"},
			Want:     "a\nx\n",
			WantErr:  "cut: -n: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file doesn't stop the other files",
			FS:       testFS(),
			Args:     []string{"-b", "1", "missing.txt", "a.txt"},
			Want:     "a\nx\n",
			WantErr:  "cut: missing.txt: file does not exist\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunListErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "zero is not a valid position",
			Args:     []string{"-b", "0"},
			WantErr:  "cut: invalid position: '0'\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a zero range end is not valid",
			Args:     []string{"-b", "1-0"},
			WantErr:  "cut: invalid position: '0'\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a non-numeric position",
			Args:     []string{"-b", "x"},
			WantErr:  "cut: invalid position: 'x'\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty list",
			Args:     []string{"-b", ""},
			WantErr:  "cut: invalid position: ''\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty element",
			Args:     []string{"-b", "1,,2"},
			WantErr:  "cut: invalid position: ''\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a lone dash",
			Args:     []string{"-b", "-"},
			WantErr:  "cut: invalid list: '-'\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a decreasing range",
			Args:     []string{"-b", "3-2"},
			WantErr:  "cut: invalid decreasing range: '3-2'\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a position with a sign",
			Args:     []string{"-b", "+1"},
			WantErr:  "cut: invalid position: '+1'\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOptionErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "no list option",
			Args:     []string{},
			WantErr:  "cut: you must specify a list of bytes, characters, or fields\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "b and c conflict",
			Args:     []string{"-b", "1", "-c", "1"},
			WantErr:  "cut: only one of -b, -c or -f may be specified\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "b and f conflict",
			Args:     []string{"-f", "1", "-b", "1"},
			WantErr:  "cut: only one of -b, -c or -f may be specified\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "d is only meaningful with f",
			Args:     []string{"-b", "1", "-d", ":"},
			WantErr:  "cut: the delimiter is only meaningful with the -f option\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "s is only meaningful with f",
			Args:     []string{"-c", "1", "-s"},
			WantErr:  "cut: the -s option is only meaningful with the -f option\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "n is only meaningful with b",
			Args:     []string{"-f", "1", "-n"},
			WantErr:  "cut: the -n option is only meaningful with the -b option\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "the delimiter must be a single character",
			Args:     []string{"-f", "1", "-d", "::"},
			WantErr:  "cut: the delimiter must be a single character\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty delimiter",
			Args:     []string{"-f", "1", "-d", ""},
			WantErr:  "cut: the delimiter must be a single character\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a multi-byte delimiter",
			Args:     []string{"-f", "1", "-d", "é"},
			WantErr:  "cut: the delimiter must be a single character\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option",
			Args:     []string{"-x"},
			WantErr:  "cut: invalid option -- 'x'\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing option argument",
			Args:     []string{"-b"},
			WantErr:  "cut: option requires an argument -- 'b'\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing output delimiter argument",
			Args:     []string{"-b", "1", "-O"},
			WantErr:  "cut: option requires an argument -- 'O'\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing long output delimiter argument",
			Args:     []string{"-b", "1", "--output-delimiter"},
			WantErr:  "cut: option '--output-delimiter' requires an argument\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "complement takes no argument",
			Args:     []string{"-b", "1", "--complement=x"},
			WantErr:  "cut: option '--complement=x' doesn't allow an argument\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "bytes",
			FS:   testFS(),
			Args: []string{"--bytes", "1", "a.txt"},
			Want: "a\nx\n",
		},
		{
			Name: "characters",
			FS:   testFS(),
			Args: []string{"--characters", "1", "utf8.txt"},
			Want: "á\n日\n",
		},
		{
			Name: "fields and delimiter",
			FS:   testFS(),
			Args: []string{"--fields", "1", "--delimiter", ":", "passwd.txt"},
			Want: "root\ndaemon\n",
		},
		{
			Name: "only-delimited",
			FS:   testFS(),
			Args: []string{"--fields", "2", "--only-delimited", "tabs.txt"},
			Want: "f2\n",
		},
		{
			Name:  "complement",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"--complement", "-b", "2"},
			Want:  "acdefg\n",
		},
		{
			Name:     "unknown long option",
			Args:     []string{"--partial", "-b", "1"},
			WantErr:  "cut: unrecognized option '--partial'\nTry 'cut --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunComplement(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a single byte",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "2", "--complement"},
			Want:  "acdefg\n",
		},
		{
			Name:  "a range of bytes",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "2-4", "--complement"},
			Want:  "aefg\n",
		},
		{
			Name:  "a range from the start",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "-2", "--complement"},
			Want:  "cdefg\n",
		},
		{
			Name:  "a range to the end",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "5-", "--complement"},
			Want:  "abcd\n",
		},
		{
			Name:  "a list of bytes",
			Stdin: strings.NewReader("abcdef\n"),
			Args:  []string{"-b", "2,4", "--complement"},
			Want:  "acef\n",
		},
		{
			Name:  "the whole line leaves nothing",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "1-", "--complement"},
			Want:  "\n",
		},
		{
			Name:  "positions past the end of the line are not an error",
			Stdin: strings.NewReader("abc\n"),
			Args:  []string{"-b", "5-10", "--complement"},
			Want:  "abc\n",
		},
		{
			Name:  "an empty line stays empty",
			Stdin: strings.NewReader("\n"),
			Args:  []string{"-b", "1", "--complement"},
			Want:  "\n",
		},
		{
			Name:  "a multi-byte character",
			Stdin: strings.NewReader("ábçde\n"),
			Args:  []string{"-c", "2", "--complement"},
			Want:  "áçde\n",
		},
		{
			Name:  "a range of characters",
			Stdin: strings.NewReader("ábçde\n日本語x\n"),
			Args:  []string{"-c", "2-3", "--complement"},
			Want:  "áde\n日x\n",
		},
		{
			Name: "fields",
			FS:   testFS(),
			Args: []string{"-d:", "-f", "1", "--complement", "passwd.txt"},
			Want: "x:0:0:root:/root:/bin/sh\nx:1:1:daemon:/usr/sbin:/usr/sbin/nologin\n",
		},
		{
			Name: "a line without the delimiter is passed through",
			FS:   testFS(),
			Args: []string{"-f", "2", "--complement", "tabs.txt"},
			Want: "f1\tf3\nplain\n",
		},
		{
			Name: "s suppresses lines without the delimiter",
			FS:   testFS(),
			Args: []string{"-f", "2", "-s", "--complement", "tabs.txt"},
			Want: "f1\tf3\n",
		},
		{
			Name:  "a line with all the fields selected comes out empty",
			Stdin: strings.NewReader("a:b\n"),
			Args:  []string{"-d:", "-f", "1-", "--complement"},
			Want:  "\n",
		},
		{
			Name:  "a whole character is selected in the complement when its last byte is",
			Stdin: strings.NewReader("aé日\n"),
			Args:  []string{"-b", "3", "-n", "--complement"},
			Want:  "a日\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOutputDelimiter(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "fields are joined by the input delimiter by default",
			Stdin: strings.NewReader("a:b:c\n"),
			Args:  []string{"-d:", "-f", "1,3"},
			Want:  "a:c\n",
		},
		{
			Name:  "a custom output delimiter",
			Stdin: strings.NewReader("a:b:c\n"),
			Args:  []string{"-d:", "-f", "1,3", "-O", "/"},
			Want:  "a/c\n",
		},
		{
			Name:  "an attached output delimiter",
			Stdin: strings.NewReader("a:b:c\n"),
			Args:  []string{"-d:", "-f", "1,3", "-O/"},
			Want:  "a/c\n",
		},
		{
			Name:  "the long option with an attached value",
			Stdin: strings.NewReader("a:b:c\n"),
			Args:  []string{"-d:", "-f", "1,3", "--output-delimiter=/"},
			Want:  "a/c\n",
		},
		{
			Name:  "the long option with the value in the next argument",
			Stdin: strings.NewReader("a:b:c\n"),
			Args:  []string{"-d:", "-f", "1,3", "--output-delimiter", "/"},
			Want:  "a/c\n",
		},
		{
			Name:  "an empty output delimiter means NUL",
			Stdin: strings.NewReader("a:b:c\n"),
			Args:  []string{"-d:", "-f", "1,3", "-O", ""},
			Want:  "a\x00c\n",
		},
		{
			Name:  "the default with fields is the input delimiter",
			Stdin: strings.NewReader("a:b:c\n"),
			Args:  []string{"-d:", "-f", "1,3", "-O", ":"},
			Want:  "a:c\n",
		},
		{
			Name:  "bytes are not delimited by default",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "1-2,4-5"},
			Want:  "abde\n",
		},
		{
			Name:  "between ranges of bytes",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "1-2,4-5", "-O", "-"},
			Want:  "ab-de\n",
		},
		{
			Name:  "adjacent ranges of bytes are merged",
			Stdin: strings.NewReader("abcdefg\n"),
			Args:  []string{"-b", "1-2,3-4", "-O", "-"},
			Want:  "abcd\n",
		},
		{
			Name:  "a range past the end of the line writes no delimiter",
			Stdin: strings.NewReader("abc\n"),
			Args:  []string{"-b", "1,4", "-O", "-"},
			Want:  "a\n",
		},
		{
			Name:  "between ranges of characters",
			Stdin: strings.NewReader("ábçde\n"),
			Args:  []string{"-c", "1,3", "-O", "-"},
			Want:  "á-ç\n",
		},
		{
			Name:  "a range of characters is not delimited inside",
			Stdin: strings.NewReader("ábçde\n"),
			Args:  []string{"-c", "2-3", "-O", "-"},
			Want:  "bç\n",
		},
		{
			Name:  "with the complement",
			Stdin: strings.NewReader("abcdef\n"),
			Args:  []string{"-b", "2,4", "--complement", "-O", "-"},
			Want:  "a-c-ef\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunZeroTerminated(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "items are NUL-terminated",
			Stdin: strings.NewReader("a\x00b\x00"),
			Args:  []string{"-z", "-b", "1"},
			Want:  "a\x00b\x00",
		},
		{
			Name:  "a missing final NUL is added",
			Stdin: strings.NewReader("ab"),
			Args:  []string{"-z", "-b", "1"},
			Want:  "a\x00",
		},
		{
			Name:  "newlines are part of the items",
			Stdin: strings.NewReader("a\nb\x00c"),
			Args:  []string{"-z", "-b", "1"},
			Want:  "a\x00c\x00",
		},
		{
			Name: "an empty file produces no output",
			FS:   testFS(),
			Args: []string{"-z", "-b", "1", "empty.txt"},
			Want: "",
		},
		{
			Name:  "fields",
			Stdin: strings.NewReader("a:b\x00plain\x00"),
			Args:  []string{"-z", "-d:", "-f", "1"},
			Want:  "a\x00plain\x00",
		},
		{
			Name:  "fields with a missing final NUL",
			Stdin: strings.NewReader("a:b"),
			Args:  []string{"-z", "-d:", "-f", "2"},
			Want:  "b\x00",
		},
		{
			Name:  "s suppresses items without the delimiter",
			Stdin: strings.NewReader("a:b\x00plain\x00"),
			Args:  []string{"-z", "-d:", "-f", "1", "-s"},
			Want:  "a\x00",
		},
		{
			Name:  "the long option",
			Stdin: strings.NewReader("a\x00b\x00"),
			Args:  []string{"--zero-terminated", "-b", "1"},
			Want:  "a\x00b\x00",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "missing file",
			FS:       testFS(),
			Args:     []string{"-b", "1", "missing.txt"},
			WantErr:  "cut: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "directory operand",
			FS:       fsx.MemoryFS{"dir": {Mode: fs.ModeDir}},
			Args:     []string{"-b", "1", "dir"},
			WantErr:  "cut: dir: invalid argument\n",
			WantExit: 1,
		},
		{
			Name:     "zero-length file name",
			FS:       testFS(),
			Args:     []string{"-b", "1", ""},
			WantErr:  "cut: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "file operand without a file system",
			Args:     []string{"-b", "1", "a.txt"},
			WantErr:  "cut: a.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input",
			Args:     []string{"-b", "1"},
			WantErr:  "cut: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "standard input read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			Args:     []string{"-b", "1"},
			WantErr:  "cut: standard input: read failed\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("hello\n"),
		Stdout: testutil.ErrorWriter{Err: writeErr},
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context(), "-b", "1")

	if code := testutil.ExitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
	if got := stderr.String(); got != "cut: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "cut: write error: write failed\n")
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		testutil.Cancelled(t, New, strings.NewReader("hello\n"), nil, "-b", "1")
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		testutil.Cancelled(t, New, &testutil.CancelReader{}, nil, "-b", "1")
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "cut", usage)
}
