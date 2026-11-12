package cmp

import (
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

// testFS holds the fixtures used by the tests:
//
//	a.txt:      "ab\ncd\n"
//	b.txt:      "ab\ncx\n" (a.txt with a difference at byte 5, line 2)
//	c.txt:      "ab\n" (a prefix of a.txt)
//	nonl.txt:   "ab\ncd" (a prefix of a.txt, without trailing newline)
//	d.txt:      "xb\ncd\n" (a.txt with a difference at byte 1)
//	same1.txt:  "hello\nworld\n"
//	same2.txt:  "hello\nworld\n"
//	empty.txt:  ""
//	bin1.txt:   {0x01, 'A', 0x07}
//	bin2.txt:   {0x01, 'A', 0x0a}
//	multi1.txt: "abcabc"
//	multi2.txt: "xbcybz"
//	skip1.txt:  "1234abcd" (skip2.txt with a prefix)
//	skip2.txt:  "abcd"
//	hi1.txt:    {0xff}
//	hi2.txt:    {0x80}
//	del1.txt:   {0x7f}
//	oct1.txt:   "12345678x"
//	oct2.txt:   "12345678y" (oct1.txt with a difference at byte 9)
//	sub/a.txt:  "z\n"
//	sub/b.txt:  "y\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":      {Data: []byte("ab\ncd\n")},
		"b.txt":      {Data: []byte("ab\ncx\n")},
		"c.txt":      {Data: []byte("ab\n")},
		"nonl.txt":   {Data: []byte("ab\ncd")},
		"d.txt":      {Data: []byte("xb\ncd\n")},
		"same1.txt":  {Data: []byte("hello\nworld\n")},
		"same2.txt":  {Data: []byte("hello\nworld\n")},
		"empty.txt":  {Data: []byte("")},
		"bin1.txt":   {Data: []byte{0x01, 'A', 0x07}},
		"bin2.txt":   {Data: []byte{0x01, 'A', 0x0a}},
		"multi1.txt": {Data: []byte("abcabc")},
		"multi2.txt": {Data: []byte("xbcybz")},
		"skip1.txt":  {Data: []byte("1234abcd")},
		"skip2.txt":  {Data: []byte("abcd")},
		"hi1.txt":    {Data: []byte{0xff}},
		"hi2.txt":    {Data: []byte{0x80}},
		"del1.txt":   {Data: []byte{0x7f}},
		"oct1.txt":   {Data: []byte("12345678x")},
		"oct2.txt":   {Data: []byte("12345678y")},
		"sub/a.txt":  {Data: []byte("z\n")},
		"sub/b.txt":  {Data: []byte("y\n")},
	}
}

func TestRunDefaults(t *testing.T) {
	big := fsx.MemFS{
		"big1.txt": {Data: []byte(strings.Repeat("a", 40000) + "b")},
		"big2.txt": {Data: []byte(strings.Repeat("a", 40000) + "c")},
		"big0.txt": {Data: []byte(strings.Repeat("a", 40000))},
	}

	tests := []testutil.Case{
		{
			Name: "identical files produce no output",
			FS:   testFS(),
			Args: []string{"same1.txt", "same2.txt"},
		},
		{
			Name: "a file compares identical to itself",
			FS:   testFS(),
			Args: []string{"a.txt", "a.txt"},
		},
		{
			Name:     "a file with itself and different skips still compares",
			FS:       testFS(),
			Args:     []string{"a.txt", "a.txt", "1"},
			Want:     "a.txt a.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "the first difference is reported",
			FS:       testFS(),
			Args:     []string{"a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 5, line 2\n",
			WantExit: 1,
		},
		{
			Name:     "a difference in the first byte is on line 1",
			FS:       testFS(),
			Args:     []string{"a.txt", "d.txt"},
			Want:     "a.txt d.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "binary files are compared byte by byte",
			FS:       testFS(),
			Args:     []string{"bin1.txt", "bin2.txt"},
			Want:     "bin1.txt bin2.txt differ: byte 3, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "a shorter file ending after a newline reports the line",
			FS:       testFS(),
			Args:     []string{"a.txt", "c.txt"},
			WantErr:  "cmp: EOF on 'c.txt' after byte 3, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "a shorter file ending inside a line reports being in the line",
			FS:       testFS(),
			Args:     []string{"a.txt", "nonl.txt"},
			WantErr:  "cmp: EOF on 'nonl.txt' after byte 5, in line 2\n",
			WantExit: 1,
		},
		{
			Name:     "an empty first file",
			FS:       testFS(),
			Args:     []string{"empty.txt", "a.txt"},
			WantErr:  "cmp: EOF on 'empty.txt' which is empty\n",
			WantExit: 1,
		},
		{
			Name:     "an empty second file",
			FS:       testFS(),
			Args:     []string{"a.txt", "empty.txt"},
			WantErr:  "cmp: EOF on 'empty.txt' which is empty\n",
			WantExit: 1,
		},
		{
			Name: "two empty files are identical",
			FS:   testFS(),
			Args: []string{"empty.txt", "empty.txt"},
		},
		{
			Name:     "the first file can be the standard input",
			FS:       testFS(),
			Stdin:    strings.NewReader("ab\ncx\n"),
			Args:     []string{"-", "a.txt"},
			Want:     "- a.txt differ: byte 5, line 2\n",
			WantExit: 1,
		},
		{
			Name:     "the second file can be the standard input",
			FS:       testFS(),
			Stdin:    strings.NewReader("xb\ncd\n"),
			Args:     []string{"a.txt", "-"},
			Want:     "a.txt - differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:  "a missing FILE2 is the standard input",
			FS:    testFS(),
			Stdin: strings.NewReader("ab\ncd\n"),
			Args:  []string{"a.txt"},
		},
		{
			Name:     "the missing FILE2 is reported as '-'",
			FS:       testFS(),
			Stdin:    strings.NewReader("ab\ncx\n"),
			Args:     []string{"a.txt"},
			Want:     "a.txt - differ: byte 5, line 2\n",
			WantExit: 1,
		},
		{
			Name:     "the standard input as FILE2 can be the shorter file",
			FS:       testFS(),
			Stdin:    strings.NewReader("ab\n"),
			Args:     []string{"a.txt"},
			WantErr:  "cmp: EOF on '-' after byte 3, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "the standard input is the shorter file",
			FS:       testFS(),
			Stdin:    strings.NewReader("ab\n"),
			Args:     []string{"-", "a.txt"},
			WantErr:  "cmp: EOF on '-' after byte 3, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "the standard input is the longer file",
			FS:       testFS(),
			Stdin:    strings.NewReader("ab\ncd\nextra\n"),
			Args:     []string{"-", "a.txt"},
			WantErr:  "cmp: EOF on 'a.txt' after byte 6, line 2\n",
			WantExit: 1,
		},
		{
			Name:     "a double dash ends the options",
			FS:       testFS(),
			Args:     []string{"--", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 5, line 2\n",
			WantExit: 1,
		},
		{
			Name:     "operands resolve against the working directory",
			FS:       testFS(),
			Dir:      "sub",
			Args:     []string{"a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "a difference beyond the first block is reported",
			FS:       big,
			Args:     []string{"big1.txt", "big2.txt"},
			Want:     "big1.txt big2.txt differ: byte 40001, line 1\n",
			WantExit: 1,
		},
		{
			Name: "identical files bigger than a block",
			FS:   big,
			Args: []string{"big1.txt", "big1.txt"},
		},
		{
			Name:     "a shorter file bigger than a block",
			FS:       big,
			Args:     []string{"big1.txt", "big0.txt"},
			WantErr:  "cmp: EOF on 'big0.txt' after byte 40000, in line 1\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-l lists every differing byte in octal",
			FS:   testFS(),
			Args: []string{"-l", "bin1.txt", "bin2.txt"},
			// The byte number and values are unpadded, as POSIX specifies.
			Want:     "3 7 12\n",
			WantExit: 1,
		},
		{
			Name:     "-l lists multiple differences",
			FS:       testFS(),
			Args:     []string{"-l", "multi1.txt", "multi2.txt"},
			Want:     "1 141 170\n4 141 171\n6 143 172\n",
			WantExit: 1,
		},
		{
			Name: "-l reports identical files with no output",
			FS:   testFS(),
			Args: []string{"-l", "same1.txt", "same2.txt"},
		},
		{
			Name:     "-l reports the end of a shorter file without a line number",
			FS:       testFS(),
			Args:     []string{"-l", "a.txt", "c.txt"},
			WantErr:  "cmp: EOF on 'c.txt' after byte 3\n",
			WantExit: 1,
		},
		{
			Name:     "-l reports an empty file",
			FS:       testFS(),
			Args:     []string{"-l", "a.txt", "empty.txt"},
			WantErr:  "cmp: EOF on 'empty.txt' which is empty\n",
			WantExit: 1,
		},
		{
			Name:     "-l reads the standard input",
			FS:       testFS(),
			Stdin:    strings.NewReader("ab\ncx\n"),
			Args:     []string{"-l", "-", "a.txt"},
			Want:     "5 170 144\n",
			WantExit: 1,
		},
		{
			Name:     "--verbose stands for -l",
			FS:       testFS(),
			Args:     []string{"--verbose", "bin1.txt", "bin2.txt"},
			Want:     "3 7 12\n",
			WantExit: 1,
		},
		{
			Name:     "-s suppresses the difference message",
			FS:       testFS(),
			Args:     []string{"-s", "a.txt", "b.txt"},
			WantExit: 1,
		},
		{
			Name: "-s keeps the identical status",
			FS:   testFS(),
			Args: []string{"-s", "same1.txt", "same2.txt"},
		},
		{
			Name:     "-s suppresses the end of a shorter file",
			FS:       testFS(),
			Args:     []string{"-s", "a.txt", "c.txt"},
			WantExit: 1,
		},
		{
			Name:     "--silent stands for -s",
			FS:       testFS(),
			Args:     []string{"--silent", "a.txt", "b.txt"},
			WantExit: 1,
		},
		{
			Name:     "--quiet stands for -s",
			FS:       testFS(),
			Args:     []string{"--quiet", "a.txt", "b.txt"},
			WantExit: 1,
		},
		{
			Name:     "an option can be repeated",
			FS:       testFS(),
			Args:     []string{"-s", "-s", "a.txt", "b.txt"},
			WantExit: 1,
		},
		{
			Name:     "-l can be repeated",
			FS:       testFS(),
			Args:     []string{"-l", "-l", "bin1.txt", "bin2.txt"},
			Want:     "3 7 12\n",
			WantExit: 1,
		},
		{
			Name:     "-l and -s are incompatible",
			FS:       testFS(),
			Args:     []string{"-l", "-s", "a.txt", "b.txt"},
			WantErr:  "cmp: options -l and -s are incompatible\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "-s and -l are incompatible",
			FS:       testFS(),
			Args:     []string{"-s", "-l", "a.txt", "b.txt"},
			WantErr:  "cmp: options -l and -s are incompatible\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "-l and -s grouped are incompatible",
			FS:       testFS(),
			Args:     []string{"-ls", "a.txt", "b.txt"},
			WantErr:  "cmp: options -l and -s are incompatible\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunPrintBytes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "-b appends the differing bytes to the message",
			FS:       testFS(),
			Args:     []string{"-b", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 5, line 2 is 144 d 170 x\n",
			WantExit: 1,
		},
		{
			Name:     "-b quotes control bytes",
			FS:       testFS(),
			Args:     []string{"-b", "bin1.txt", "bin2.txt"},
			Want:     "bin1.txt bin2.txt differ: byte 3, line 1 is   7 ^G  12 ^J\n",
			WantExit: 1,
		},
		{
			Name:     "-b quotes the delete byte",
			FS:       testFS(),
			Args:     []string{"-b", "del1.txt", "a.txt"},
			Want:     "del1.txt a.txt differ: byte 1, line 1 is 177 ^? 141 a\n",
			WantExit: 1,
		},
		{
			Name:     "-b quotes meta bytes",
			FS:       testFS(),
			Args:     []string{"-b", "hi1.txt", "hi2.txt"},
			Want:     "hi1.txt hi2.txt differ: byte 1, line 1 is 377 M-^? 200 M-^@\n",
			WantExit: 1,
		},
		{
			Name:     "--print-bytes stands for -b",
			FS:       testFS(),
			Args:     []string{"--print-bytes", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 5, line 2 is 144 d 170 x\n",
			WantExit: 1,
		},
		{
			Name:     "-b can be repeated",
			FS:       testFS(),
			Args:     []string{"-bb", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 5, line 2 is 144 d 170 x\n",
			WantExit: 1,
		},
		{
			Name:     "-lb lists the bytes of every difference",
			FS:       testFS(),
			Args:     []string{"-lb", "multi1.txt", "multi2.txt"},
			Want:     "1 141 a    170 x\n4 141 a    171 y\n6 143 c    172 z\n",
			WantExit: 1,
		},
		{
			Name:     "-lb quotes the bytes of a difference",
			FS:       testFS(),
			Args:     []string{"-lb", "bin1.txt", "bin2.txt"},
			Want:     "3   7 ^G    12 ^J\n",
			WantExit: 1,
		},
		{
			Name:     "-lb quotes meta bytes without padding them",
			FS:       testFS(),
			Args:     []string{"-lb", "hi1.txt", "hi2.txt"},
			Want:     "1 377 M-^? 200 M-^@\n",
			WantExit: 1,
		},
		{
			Name:     "-b keeps the EOF diagnostic of a shorter file",
			FS:       testFS(),
			Args:     []string{"-b", "a.txt", "c.txt"},
			WantErr:  "cmp: EOF on 'c.txt' after byte 3, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "-b is silent with -s",
			FS:       testFS(),
			Args:     []string{"-sb", "a.txt", "b.txt"},
			WantExit: 1,
		},
		{
			Name:     "-b reads the standard input",
			FS:       testFS(),
			Stdin:    strings.NewReader("ab\ncx\n"),
			Args:     []string{"-b", "a.txt"},
			Want:     "a.txt - differ: byte 5, line 2 is 144 d 170 x\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunIgnoreInitial(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "-i skips the first bytes of both files",
			FS:       testFS(),
			Args:     []string{"-i", "4", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "-i resets the byte and line numbering",
			FS:       testFS(),
			Args:     []string{"-i", "1", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 4, line 2\n",
			WantExit: 1,
		},
		{
			Name:     "-i with a count for each file",
			FS:       testFS(),
			Args:     []string{"-i", "1:3", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "--ignore-initial stands for -i",
			FS:       testFS(),
			Args:     []string{"--ignore-initial=4", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "--ignore-initial takes its argument separately",
			FS:       testFS(),
			Args:     []string{"--ignore-initial", "4", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "the skip can be grouped with other options",
			FS:       testFS(),
			Args:     []string{"-li3", "multi1.txt", "multi2.txt"},
			Want:     "1 141 171\n3 143 172\n",
			WantExit: 1,
		},
		{
			Name:     "the skip counts restart the -l numbering",
			FS:       testFS(),
			Args:     []string{"-l", "-i", "3", "multi1.txt", "multi2.txt"},
			Want:     "1 141 171\n3 143 172\n",
			WantExit: 1,
		},
		{
			Name:     "the skip can be hexadecimal",
			FS:       testFS(),
			Args:     []string{"-i", "0x4", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name: "the skip can be octal",
			FS:   testFS(),
			Args: []string{"-i", "010", "a.txt", "b.txt"},
		},
		{
			Name: "the skip can have a suffix",
			FS:   testFS(),
			Args: []string{"-i", "1K", "a.txt", "b.txt"},
		},
		{
			Name: "a skip beyond both files compares nothing",
			FS:   testFS(),
			Args: []string{"-i", "100", "a.txt", "b.txt"},
		},
		{
			Name:     "a skip beyond one file only reports it as empty",
			FS:       testFS(),
			Args:     []string{"-i", "100:0", "a.txt", "b.txt"},
			WantErr:  "cmp: EOF on 'a.txt' which is empty\n",
			WantExit: 1,
		},
		{
			Name:     "repeated -i keeps the largest count",
			FS:       testFS(),
			Args:     []string{"-i", "4", "-i", "2", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "a single count of repeated -i applies to both files",
			FS:       testFS(),
			Args:     []string{"-i", "1:3", "-i", "2", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "-s keeps the status of a skipped difference",
			FS:       testFS(),
			Args:     []string{"-si", "4", "a.txt", "b.txt"},
			WantExit: 1,
		},
		{
			Name: "-s keeps the identical status beyond a difference",
			FS:   testFS(),
			Args: []string{"-si", "5", "a.txt", "b.txt"},
		},
		{
			Name:     "-i reads the standard input",
			FS:       testFS(),
			Stdin:    strings.NewReader("xxabcd"),
			Args:     []string{"-i", "2", "skip1.txt"},
			Want:     "skip1.txt - differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "-i with a skip on the standard input only",
			FS:       testFS(),
			Stdin:    strings.NewReader("xxabcd"),
			Args:     []string{"-i", "2:0", "-", "skip1.txt"},
			Want:     "- skip1.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunSkipOperands(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "a SKIP1 operand skips FILE1 only",
			FS:       testFS(),
			Args:     []string{"skip1.txt", "skip2.txt", "4"},
			WantExit: 0,
		},
		{
			Name:     "SKIP1 and SKIP2 skip each file",
			FS:       testFS(),
			Args:     []string{"skip1.txt", "skip2.txt", "4", "4"},
			WantErr:  "cmp: EOF on 'skip2.txt' which is empty\n",
			WantExit: 1,
		},
		{
			Name:     "a skip beyond one file reports it as empty",
			FS:       testFS(),
			Args:     []string{"a.txt", "b.txt", "100"},
			WantErr:  "cmp: EOF on 'a.txt' which is empty\n",
			WantExit: 1,
		},
		{
			Name:     "the skip operands combine with -i, keeping the largest",
			FS:       testFS(),
			Args:     []string{"-i", "2", "a.txt", "b.txt", "5"},
			WantErr:  "cmp: EOF on 'a.txt' after byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "the skip operand can be hexadecimal",
			FS:       testFS(),
			Args:     []string{"a.txt", "b.txt", "0x4"},
			Want:     "a.txt b.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "the skip operands work with -l",
			FS:       testFS(),
			Args:     []string{"-l", "multi1.txt", "multi2.txt", "3"},
			Want:     "1 141 170\n",
			WantErr:  "cmp: EOF on 'multi1.txt' after byte 3\n",
			WantExit: 1,
		},
		{
			Name:     "the skip operands are accepted without a warning",
			FS:       testFS(),
			Args:     []string{"a.txt", "b.txt", "4", "4"},
			Want:     "a.txt b.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "the skip operands read the standard input",
			FS:       testFS(),
			Stdin:    strings.NewReader("xx1234abcd"),
			Args:     []string{"-", "skip2.txt", "2", "0"},
			Want:     "- skip2.txt differ: byte 1, line 1\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunBytesLimit(t *testing.T) {
	big := fsx.MemFS{
		"big1.txt": {Data: []byte(strings.Repeat("a", 40000) + "b")},
		"big2.txt": {Data: []byte(strings.Repeat("a", 40000) + "c")},
		"big0.txt": {Data: []byte(strings.Repeat("a", 40000))},
	}

	tests := []testutil.Case{
		{
			Name:     "a difference within the limit is reported",
			FS:       testFS(),
			Args:     []string{"-n", "5", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 5, line 2\n",
			WantExit: 1,
		},
		{
			Name:     "a limit beyond the files still reports the difference",
			FS:       testFS(),
			Args:     []string{"-n", "100", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 5, line 2\n",
			WantExit: 1,
		},
		{
			Name: "a difference beyond the limit is not reported",
			FS:   testFS(),
			Args: []string{"-n", "4", "a.txt", "b.txt"},
		},
		{
			Name: "identical files compare equal within the limit",
			FS:   testFS(),
			Args: []string{"-n", "3", "same1.txt", "same2.txt"},
		},
		{
			Name: "a zero limit compares nothing",
			FS:   testFS(),
			Args: []string{"-n", "0", "a.txt", "b.txt"},
		},
		{
			Name:     "an empty file is reported within the limit",
			FS:       testFS(),
			Args:     []string{"-n", "3", "empty.txt", "a.txt"},
			WantErr:  "cmp: EOF on 'empty.txt' which is empty\n",
			WantExit: 1,
		},
		{
			Name:     "a prefix ending within the limit is reported",
			FS:       testFS(),
			Args:     []string{"-n", "5", "a.txt", "c.txt"},
			WantErr:  "cmp: EOF on 'c.txt' after byte 3, line 1\n",
			WantExit: 1,
		},
		{
			Name: "a prefix ending beyond the limit is not reported",
			FS:   testFS(),
			Args: []string{"-n", "2", "a.txt", "c.txt"},
		},
		{
			Name: "a file ending exactly at the limit is not reported",
			FS:   testFS(),
			Args: []string{"-n", "3", "a.txt", "c.txt"},
		},
		{
			Name:     "a difference after a block within the limit is reported",
			FS:       big,
			Args:     []string{"-n", "40001", "big1.txt", "big2.txt"},
			Want:     "big1.txt big2.txt differ: byte 40001, line 1\n",
			WantExit: 1,
		},
		{
			Name: "a difference after a block beyond the limit is not reported",
			FS:   big,
			Args: []string{"-n", "40000", "big1.txt", "big2.txt"},
		},
		{
			Name:     "a prefix after a block within the limit is reported",
			FS:       big,
			Args:     []string{"-n", "40001", "big1.txt", "big0.txt"},
			WantErr:  "cmp: EOF on 'big0.txt' after byte 40000, in line 1\n",
			WantExit: 1,
		},
		{
			Name:     "-l stops listing differences after the limit",
			FS:       testFS(),
			Args:     []string{"-l", "-n", "3", "multi1.txt", "multi2.txt"},
			Want:     "1 141 170\n",
			WantExit: 1,
		},
		{
			Name:     "-l lists every difference within the limit",
			FS:       testFS(),
			Args:     []string{"-l", "-n", "5", "multi1.txt", "multi2.txt"},
			Want:     "1 141 170\n4 141 171\n",
			WantExit: 1,
		},
		{
			Name:     "-l with a limit beyond the files",
			FS:       testFS(),
			Args:     []string{"-l", "-n", "10", "multi1.txt", "multi2.txt"},
			Want:     "1 141 170\n4 141 171\n6 143 172\n",
			WantExit: 1,
		},
		{
			Name:     "-l reports the end of a prefix within the limit",
			FS:       testFS(),
			Args:     []string{"-l", "-n", "5", "a.txt", "c.txt"},
			WantErr:  "cmp: EOF on 'c.txt' after byte 3\n",
			WantExit: 1,
		},
		{
			Name:     "-s keeps the status of a difference within the limit",
			FS:       testFS(),
			Args:     []string{"-s", "-n", "6", "a.txt", "b.txt"},
			WantExit: 1,
		},
		{
			Name: "-s keeps the identical status beyond a difference",
			FS:   testFS(),
			Args: []string{"-s", "-n", "4", "a.txt", "b.txt"},
		},
		{
			Name:     "the limit can be attached",
			FS:       testFS(),
			Args:     []string{"-n5", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 5, line 2\n",
			WantExit: 1,
		},
		{
			Name:     "the limit can be grouped with other options",
			FS:       testFS(),
			Args:     []string{"-ln3", "multi1.txt", "multi2.txt"},
			Want:     "1 141 170\n",
			WantExit: 1,
		},
		{
			Name:     "--bytes stands for -n",
			FS:       testFS(),
			Args:     []string{"--bytes=5", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 5, line 2\n",
			WantExit: 1,
		},
		{
			Name:     "--bytes takes its argument separately",
			FS:       testFS(),
			Args:     []string{"--bytes", "5", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 5, line 2\n",
			WantExit: 1,
		},
		{
			Name:     "the limit can be hexadecimal",
			FS:       testFS(),
			Args:     []string{"-n", "0x5", "a.txt", "b.txt"},
			Want:     "a.txt b.txt differ: byte 5, line 2\n",
			WantExit: 1,
		},
		{
			Name:     "the limit can be octal",
			FS:       testFS(),
			Args:     []string{"-n", "011", "oct1.txt", "oct2.txt"},
			Want:     "oct1.txt oct2.txt differ: byte 9, line 1\n",
			WantExit: 1,
		},
		{
			Name: "an octal limit counts the leading zero",
			FS:   testFS(),
			Args: []string{"-n", "010", "oct1.txt", "oct2.txt"},
		},
		{
			Name:     "the limit can have a binary suffix",
			FS:       testFS(),
			Args:     []string{"-n", "1K", "oct1.txt", "oct2.txt"},
			Want:     "oct1.txt oct2.txt differ: byte 9, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "the limit can have a decimal suffix",
			FS:       testFS(),
			Args:     []string{"-n", "1kB", "oct1.txt", "oct2.txt"},
			Want:     "oct1.txt oct2.txt differ: byte 9, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "the limit can have an explicit binary suffix",
			FS:       testFS(),
			Args:     []string{"-n", "1KiB", "oct1.txt", "oct2.txt"},
			Want:     "oct1.txt oct2.txt differ: byte 9, line 1\n",
			WantExit: 1,
		},
		{
			Name:     "the limit can be signed with a plus",
			FS:       testFS(),
			Args:     []string{"-n", "+9", "oct1.txt", "oct2.txt"},
			Want:     "oct1.txt oct2.txt differ: byte 9, line 1\n",
			WantExit: 1,
		},
		{
			Name: "the least of repeated limits is used",
			FS:   testFS(),
			Args: []string{"-n", "5", "-n", "4", "a.txt", "b.txt"},
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunInvalidUsage(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "no operands",
			FS:       testFS(),
			WantErr:  "cmp: missing operand\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "the standard input may only be specified once",
			FS:       testFS(),
			Stdin:    strings.NewReader("ab\ncd\n"),
			Args:     []string{"-", "-"},
			WantErr:  "cmp: the standard input may only be specified once\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a missing FILE2 makes the standard input twice",
			FS:       testFS(),
			Stdin:    strings.NewReader("ab\ncd\n"),
			Args:     []string{"-"},
			WantErr:  "cmp: the standard input may only be specified once\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "the standard input is rejected twice with skips",
			FS:       testFS(),
			Stdin:    strings.NewReader("ab\ncd\n"),
			Args:     []string{"-", "-", "1", "2"},
			WantErr:  "cmp: the standard input may only be specified once\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an extra operand",
			FS:       testFS(),
			Args:     []string{"a.txt", "b.txt", "1", "2", "third.txt"},
			WantErr:  "cmp: extra operand 'third.txt'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid option",
			FS:       testFS(),
			Args:     []string{"-x", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid option -- 'x'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unsupported long option",
			FS:       testFS(),
			Args:     []string{"--initial", "a.txt", "b.txt"},
			WantErr:  "cmp: unrecognized option '--initial'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid limit",
			FS:       testFS(),
			Args:     []string{"-n", "x", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --bytes value 'x'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative limit",
			FS:       testFS(),
			Args:     []string{"-n", "-1", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --bytes value '-1'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty limit",
			FS:       testFS(),
			Args:     []string{"--bytes=", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --bytes value ''\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a limit with an unsupported suffix",
			FS:       testFS(),
			Args:     []string{"-n", "1b", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --bytes value '1b'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a limit with trailing junk",
			FS:       testFS(),
			Args:     []string{"-n", "5x", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --bytes value '5x'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a hexadecimal limit without digits",
			FS:       testFS(),
			Args:     []string{"-n", "0x", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --bytes value '0x'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a number with a non-octal digit after a zero",
			FS:       testFS(),
			Args:     []string{"-n", "09", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --bytes value '09'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid skip value",
			FS:       testFS(),
			Args:     []string{"-i", "x", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --ignore-initial value 'x'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative skip value",
			FS:       testFS(),
			Args:     []string{"-i", "-1", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --ignore-initial value '-1'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an empty skip value",
			FS:       testFS(),
			Args:     []string{"--ignore-initial=", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --ignore-initial value ''\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a skip value ending with a colon",
			FS:       testFS(),
			Args:     []string{"-i", "4:", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --ignore-initial value ''\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a skip value starting with a colon",
			FS:       testFS(),
			Args:     []string{"-i", ":4", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --ignore-initial value ':4'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a skip value with an invalid second count",
			FS:       testFS(),
			Args:     []string{"-i", "4:x", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --ignore-initial value 'x'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a skip value with two colons",
			FS:       testFS(),
			Args:     []string{"-i", "1:1:1", "a.txt", "b.txt"},
			WantErr:  "cmp: invalid --ignore-initial value '1:1'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid SKIP1 operand",
			FS:       testFS(),
			Args:     []string{"a.txt", "b.txt", "x"},
			WantErr:  "cmp: invalid --ignore-initial value 'x'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid SKIP2 operand",
			FS:       testFS(),
			Args:     []string{"a.txt", "b.txt", "1", "x"},
			WantErr:  "cmp: invalid --ignore-initial value 'x'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "-i without an argument",
			FS:       testFS(),
			Args:     []string{"-i"},
			WantErr:  "cmp: option requires an argument -- 'i'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "--ignore-initial without an argument",
			FS:       testFS(),
			Args:     []string{"--ignore-initial"},
			WantErr:  "cmp: option '--ignore-initial' requires an argument\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "--print-bytes doesn't take an argument",
			FS:       testFS(),
			Args:     []string{"--print-bytes=1", "a.txt", "b.txt"},
			WantErr:  "cmp: option '--print-bytes=1' doesn't allow an argument\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "-n without an argument",
			FS:       testFS(),
			Args:     []string{"-n"},
			WantErr:  "cmp: option requires an argument -- 'n'\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "--bytes without an argument",
			FS:       testFS(),
			Args:     []string{"--bytes"},
			WantErr:  "cmp: option '--bytes' requires an argument\nTry 'cmp --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "missing first file",
			FS:       testFS(),
			Args:     []string{"missing.txt", "a.txt"},
			WantErr:  "cmp: missing.txt: file does not exist\n",
			WantExit: 2,
		},
		{
			Name:     "missing second file",
			FS:       testFS(),
			Args:     []string{"a.txt", "missing.txt"},
			WantErr:  "cmp: missing.txt: file does not exist\n",
			WantExit: 2,
		},
		{
			Name:     "the first missing file is reported when both are missing",
			FS:       testFS(),
			Args:     []string{"missing1.txt", "missing2.txt"},
			WantErr:  "cmp: missing1.txt: file does not exist\n",
			WantExit: 2,
		},
		{
			Name:     "directory input",
			FS:       fsx.MemFS{"dir": {Mode: fs.ModeDir}, "a.txt": {Data: []byte("a\n")}},
			Args:     []string{"dir", "a.txt"},
			WantErr:  "cmp: dir: invalid argument\n",
			WantExit: 2,
		},
		{
			Name:     "input file without a file system",
			Args:     []string{"a.txt", "b.txt"},
			WantErr:  "cmp: a.txt: no file system available\n",
			WantExit: 2,
		},
		{
			Name:     "no standard input",
			FS:       testFS(),
			Args:     []string{"-", "a.txt"},
			WantErr:  "cmp: -: no standard input available\n",
			WantExit: 2,
		},
		{
			Name:     "standard input read error",
			FS:       testFS(),
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			Args:     []string{"-", "a.txt"},
			WantErr:  "cmp: -: read failed\n",
			WantExit: 2,
		},
		{
			Name:     "-s suppresses the message of an opening error",
			FS:       testFS(),
			Args:     []string{"-s", "missing.txt", "a.txt"},
			WantExit: 2,
		},
		{
			Name:     "-s doesn't suppress read errors",
			FS:       testFS(),
			Stdin:    testutil.ErrorReader{Err: errors.New("read failed")},
			Args:     []string{"-s", "-", "a.txt"},
			WantErr:  "cmp: -: read failed\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunWriteError(t *testing.T) {
	writeErr := errors.New("write failed")

	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: testutil.ErrorWriter{Err: writeErr},
		Stderr: stderr,
		FS:     testFS(),
	}
	err := New(rt).Run(t.Context(), "a.txt", "b.txt")

	if code := testutil.ExitCode(t, err); code != 2 {
		t.Fatalf("Run() exit code = %d, want 2 (error = %v)", code, err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("errors.Is(Run(), writeErr) = false, want true (error = %v)", err)
	}
	if got := stderr.String(); got != "cmp: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "cmp: write error: write failed\n")
	}
}

func TestRunContext(t *testing.T) {
	files := fsx.MemFS{"a.txt": {Data: []byte("hello\n")}}

	t.Run("cancelled before running", func(t *testing.T) {
		testutil.Cancelled(t, New, strings.NewReader("hello\n"), files, "-", "a.txt")
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		testutil.Cancelled(t, New, &testutil.CancelReader{}, files, "-", "a.txt")
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "cmp", usage)

	t.Run("short version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "-v"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "cmp (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("short version wins over the operands", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "-v", "a.txt"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "cmp (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
