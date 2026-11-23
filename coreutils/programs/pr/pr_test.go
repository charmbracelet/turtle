package pr

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

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
func testFS() fsx.MemoryFS {
	return fsx.MemoryFS{
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

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a file is paginated with a header and a trailer",
			FS:   testFS(),
			Args: []string{"lorem.txt"},
			Want: header("lorem.txt", 1) + padded("lorem\nipsum\ndolor\n", 56, 3),
		},
		{
			Name: "the file name in the header is the operand",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"inner.txt"},
			Want: header("inner.txt", 1) + padded("inner\n", 56, 1),
		},
		{
			Name:  "the standard input has no name in the header",
			Stdin: strings.NewReader("lorem\n"),
			Args:  []string{"-t"},
			Want:  "lorem\n",
		},
		{
			Name:  "a dash operand is the standard input",
			Stdin: strings.NewReader("lorem\n"),
			Args:  []string{"-t", "-"},
			Want:  "lorem\n",
		},
		{
			Name: "-t suppresses the header and the trailer",
			FS:   testFS(),
			Args: []string{"-t", "lorem.txt"},
			Want: "lorem\nipsum\ndolor\n",
		},
		{
			Name: "an empty file produces no output",
			FS:   testFS(),
			Args: []string{"empty.txt"},
			Want: "",
		},
		{
			Name: "an unterminated last line is terminated on output",
			FS:   testFS(),
			Args: []string{"-t", "nonl.txt"},
			Want: "lorem\nipsum\n",
		},
		{
			Name: "single column lines are not truncated by -w",
			FS:   testFS(),
			Args: []string{"-t", "-w", "10", "long.txt"},
			Want: strings.Repeat("x", 80) + "\n",
		},
		{
			Name: "two files are paginated separately",
			FS:   testFS(),
			Args: []string{"-l", "12", "lorem.txt", "odd.txt"},
			Want: header("lorem.txt", 1) + padded("lorem\nipsum\n", 2, 2) +
				header("lorem.txt", 2) + padded("dolor\n", 2, 1) +
				header("odd.txt", 1) + padded("a\nb\n", 2, 2) +
				header("odd.txt", 2) + padded("c\nd\n", 2, 2) +
				header("odd.txt", 3) + padded("e\n", 2, 1),
		},
		{
			Name:     "options stop at the first operand",
			FS:       testFS(),
			Args:     []string{"lorem.txt", "-t"},
			Want:     header("lorem.txt", 1) + padded("lorem\nipsum\ndolor\n", 56, 3),
			WantErr:  "pr: -t: file does not exist\n",
			WantExit: 1,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunPages(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a short page length",
			FS:   testFS(),
			Args: []string{"-l", "12", "lorem.txt"},
			Want: header("lorem.txt", 1) + padded("lorem\nipsum\n", 2, 2) +
				header("lorem.txt", 2) + padded("dolor\n", 2, 1),
		},
		{
			Name: "a page length not greater than the header and trailer suppresses both",
			FS:   testFS(),
			Args: []string{"-l", "10", "lorem.txt"},
			Want: "lorem\nipsum\ndolor\n",
		},
		{
			Name: "+page starts the output at the given page",
			FS:   testFS(),
			Args: []string{"+2", "-l", "12", "lorem.txt"},
			Want: header("lorem.txt", 2) + padded("dolor\n", 2, 1),
		},
		{
			Name: "+page beyond the input produces no output",
			FS:   testFS(),
			Args: []string{"+9", "-l", "12", "lorem.txt"},
			Want: "",
		},
		{
			Name: "+page applies to each file",
			FS:   testFS(),
			Args: []string{"-t", "+2", "-l", "2", "lorem.txt", "odd.txt"},
			Want: "dolor\n" + "c\nd\ne\n",
		},
		{
			Name: "-d double-spaces the output",
			FS:   testFS(),
			Args: []string{"-t", "-d", "lorem.txt"},
			Want: "lorem\n\nipsum\n\ndolor\n\n",
		},
		{
			Name: "-d fits double-spaced lines in the page",
			FS:   testFS(),
			Args: []string{"-d", "-l", "14", "lorem.txt"},
			Want: header("lorem.txt", 1) + "lorem\n\nipsum\n\n" +
				strings.Repeat("\n", 5) +
				header("lorem.txt", 2) + "dolor\n\n" + strings.Repeat("\n", 7),
		},
		{
			Name: "-d on an odd-length page pads the extra line",
			FS:   testFS(),
			Args: []string{"-d", "-l", "13", "odd.txt"},
			Want: header("odd.txt", 1) + "a\n\n" + strings.Repeat("\n", 6) +
				header("odd.txt", 2) + "b\n\n" + strings.Repeat("\n", 6) +
				header("odd.txt", 3) + "c\n\n" + strings.Repeat("\n", 6) +
				header("odd.txt", 4) + "d\n\n" + strings.Repeat("\n", 6) +
				header("odd.txt", 5) + "e\n\n" + strings.Repeat("\n", 6),
		},
		{
			Name: "-F terminates pages with a form feed",
			FS:   testFS(),
			Args: []string{"-F", "-l", "12", "lorem.txt"},
			Want: header("lorem.txt", 1) + "lorem\nipsum\n\f" +
				header("lorem.txt", 2) + "dolor\n\f",
		},
		{
			Name: "-f is a form feed too",
			FS:   testFS(),
			Args: []string{"-f", "-l", "12", "lorem.txt"},
			Want: header("lorem.txt", 1) + "lorem\nipsum\n\f" +
				header("lorem.txt", 2) + "dolor\n\f",
		},
		{
			Name: "-t -F is a no-op for the page separator",
			FS:   testFS(),
			Args: []string{"-t", "-F", "-l", "2", "lorem.txt"},
			Want: "lorem\nipsum\ndolor\n",
		},
		{
			Name: "-h replaces the file name in the header",
			FS:   testFS(),
			Args: []string{"-h", "My Header", "-l", "12", "lorem.txt"},
			Want: header("My Header", 1) + padded("lorem\nipsum\n", 2, 2) +
				header("My Header", 2) + padded("dolor\n", 2, 1),
		},
		{
			Name: "-h with an empty header",
			FS:   testFS(),
			Args: []string{"-h", "", "-l", "12", "odd.txt"},
			Want: header("", 1) + padded("a\nb\n", 2, 2) +
				header("", 2) + padded("c\nd\n", 2, 2) +
				header("", 3) + padded("e\n", 2, 1),
		},
		{
			Name: "-o offsets the header and the lines",
			FS:   testFS(),
			Args: []string{"-o", "4", "-l", "12", "odd.txt"},
			Want: headerOffset("odd.txt", 1, 4) + padded("    a\n    b\n", 2, 2) +
				headerOffset("odd.txt", 2, 4) + padded("    c\n    d\n", 2, 2) +
				headerOffset("odd.txt", 3, 4) + padded("    e\n", 2, 1),
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunNumbers(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-n numbers the lines",
			FS:   testFS(),
			Args: []string{"-t", "-n", "lorem.txt"},
			Want: "    1\tlorem\n    2\tipsum\n    3\tdolor\n",
		},
		{
			Name: "-n with a separator and a width",
			FS:   testFS(),
			Args: []string{"-t", "-n.3", "lorem.txt"},
			Want: "  1.lorem\n  2.ipsum\n  3.dolor\n",
		},
		{
			Name: "-n with a width only",
			FS:   testFS(),
			Args: []string{"-t", "-n2", "lorem.txt"},
			Want: " 1\tlorem\n 2\tipsum\n 3\tdolor\n",
		},
		{
			Name: "wide numbers keep the low-order digits",
			FS:   testFS(),
			Args: []string{"-t", "-n1", "ten.txt"},
			Want: "1\t1\n2\t2\n3\t3\n4\t4\n5\t5\n6\t6\n7\t7\n8\t8\n9\t9\n0\t10\n",
		},
		{
			Name: "numbers restart for each file",
			FS:   testFS(),
			Args: []string{"-t", "-n", "lorem.txt", "odd.txt"},
			Want: "    1\tlorem\n    2\tipsum\n    3\tdolor\n" +
				"    1\ta\n    2\tb\n    3\tc\n    4\td\n    5\te\n",
		},
		{
			Name: "+page numbers the lines of the input, not the printed page",
			FS:   testFS(),
			Args: []string{"-t", "+2", "-l", "2", "-n", "lorem.txt"},
			Want: "    3\tdolor\n",
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunColumns(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "columns are filled down the page",
			FS:   testFS(),
			Args: []string{"-t", "-2", "odd.txt"},
			Want: "a\t\t\t\t    d\n" +
				"b\t\t\t\t    e\n" +
				"c\n",
		},
		{
			Name: "column padding is contracted to tabs",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-w", "20", "odd.txt"},
			Want: "a\t  d\nb\t  e\nc\n",
		},
		{
			Name: "columns are truncated to their width",
			FS:   testFS(),
			Args: []string{"-t", "-2", "long.txt"},
			Want: strings.Repeat("x", 35) + "\n",
		},
		{
			Name: "-a fills columns across the page",
			FS:   testFS(),
			Args: []string{"-t", "-a", "-2", "odd.txt"},
			Want: "a\t\t\t\t    b\n" +
				"c\t\t\t\t    d\n" +
				"e\n",
		},
		{
			Name: "-a with a separator",
			FS:   testFS(),
			Args: []string{"-t", "-a", "-2", "-s|", "odd.txt"},
			Want: "a|b\nc|d\ne\n",
		},
		{
			Name: "-s separates columns without padding",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-s|", "odd.txt"},
			Want: "a|d\nb|e\nc\n",
		},
		{
			Name: "-s with a tab contracts back to tab stops",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-s", "odd.txt"},
			Want: "a\td\nb\te\nc\n",
		},
		{
			Name: "a full page is not rebalanced",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-l", "4", "ten.txt"},
			Want: "1\t\t\t\t    5\n" +
				"2\t\t\t\t    6\n" +
				"3\t\t\t\t    7\n" +
				"4\t\t\t\t    8\n" +
				"9\t\t\t\t    10\n",
		},
		{
			Name: "a partial page is rebalanced",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-l", "4", "blanks.txt"},
			Want: "a\t\t\t\t    \n\t\t\t\t    b\n",
		},
		{
			Name: "a nearly full page is not rebalanced",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-l", "4", "seven.txt"},
			Want: "1\t\t\t\t    5\n" +
				"2\t\t\t\t    6\n" +
				"3\t\t\t\t    7\n" +
				"4\n",
		},
		{
			Name: "columns with line numbers",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-l", "4", "-n.2", "odd.txt"},
			Want: " 1.a\t\t\t\t     4.d\n" +
				" 2.b\t\t\t\t     5.e\n" +
				" 3.c\n",
		},
		{
			Name: "+page with columns skips whole formatted pages",
			FS:   testFS(),
			Args: []string{"-t", "+2", "-2", "-l", "4", "ten.txt"},
			Want: "9\t\t\t\t    10\n",
		},
		{
			Name: "columns with a header",
			FS:   testFS(),
			Args: []string{"-2", "-l", "12", "odd.txt"},
			Want: header("odd.txt", 1) +
				"a\t\t\t\t    c\n" +
				"b\t\t\t\t    d\n" +
				strings.Repeat("\n", 5) +
				header("odd.txt", 2) +
				"e\n" +
				strings.Repeat("\n", 6),
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunMerge(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "merge files side by side",
			FS:   testFS(),
			Args: []string{"-t", "-m", "lorem.txt", "odd.txt"},
			Want: "lorem\t\t\t\t    a\n" +
				"ipsum\t\t\t\t    b\n" +
				"dolor\t\t\t\t    c\n" +
				"\t\t\t\t    d\n" +
				"\t\t\t\t    e\n",
		},
		{
			Name: "merge with a separator",
			FS:   testFS(),
			Args: []string{"-t", "-m", "-s|", "lorem.txt", "odd.txt"},
			Want: "lorem|a\nipsum|b\ndolor|c\n|d\n|e\n",
		},
		{
			Name: "merge with line numbers",
			FS:   testFS(),
			Args: []string{"-t", "-m", "-n.2", "odd.txt", "lorem.txt"},
			Want: " 1.a\t\t\t\t      lorem\n" +
				" 2.b\t\t\t\t      ipsum\n" +
				" 3.c\t\t\t\t      dolor\n" +
				" 4.d\t\t\t\t      \n" +
				" 5.e\t\t\t\t      \n",
		},
		{
			Name: "the columns of an ended file keep their padding",
			FS:   testFS(),
			Args: []string{"-t", "-m", "odd.txt", "nonl.txt"},
			Want: "a\t\t\t\t    lorem\n" +
				"b\t\t\t\t    ipsum\n" +
				"c\t\t\t\t    \n" +
				"d\t\t\t\t    \n" +
				"e\t\t\t\t    \n",
		},
		{
			Name: "merging one file is single column output",
			FS:   testFS(),
			Args: []string{"-t", "-m", "lorem.txt"},
			Want: "lorem\nipsum\ndolor\n",
		},
		{
			Name: "the merged header has a blank middle",
			FS:   testFS(),
			Args: []string{"-m", "-l", "12", "-D", "DATE", "lorem.txt", "odd.txt"},
			Want: "\n\nDATE" + strings.Repeat(" ", 62) + "Page 1\n\n\n" +
				"lorem\t\t\t\t    a\nipsum\t\t\t\t    b\n" +
				strings.Repeat("\n", 7) +
				"DATE" + strings.Repeat(" ", 62) + "Page 2\n\n\n" +
				"dolor\t\t\t\t    c\n\t\t\t\t    d\n" +
				strings.Repeat("\n", 7) +
				"DATE" + strings.Repeat(" ", 62) + "Page 3\n\n\n" +
				"\t\t\t\t    e\n" + strings.Repeat("\n", 6),
		},
		{
			Name: "merged files that are all empty produce no output",
			FS:   testFS(),
			Args: []string{"-m", "empty.txt", "empty.txt"},
			Want: "",
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunTabs(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-e expands input tabs",
			FS:   testFS(),
			Args: []string{"-t", "-e", "tabs.txt"},
			Want: "a       b\n",
		},
		{
			Name: "-e with a gap",
			FS:   testFS(),
			Args: []string{"-t", "-e4", "tabs.txt"},
			Want: "a   b\n",
		},
		{
			Name: "-e with a custom tab character",
			FS:   testFS(),
			Args: []string{"-t", "-e,3", "blanks.txt"},
			Want: "a\n\n\nb\n",
		},
		{
			Name:  "-i contracts blanks to tabs",
			Stdin: strings.NewReader("a        b\n"),
			Args:  []string{"-t", "-i"},
			Want:  "a\t b\n",
		},
		{
			Name:  "-i keeps a single blank",
			Stdin: strings.NewReader("a b\n"),
			Args:  []string{"-t", "-i"},
			Want:  "a b\n",
		},
		{
			Name:  "-i with a custom tab character",
			Stdin: strings.NewReader("a        b\n"),
			Args:  []string{"-t", "-i."},
			Want:  "a. b\n",
		},
		{
			Name: "input tabs are left alone without -e",
			FS:   testFS(),
			Args: []string{"-t", "tabs.txt"},
			Want: "a\tb\n",
		},
		{
			Name: "input tabs are expanded in column mode",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-w", "20", "tabs.txt"},
			Want: "a\tb\n",
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "an invalid option",
			Args:     []string{"-q"},
			WantErr:  "pr: invalid option -- 'q'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing option argument",
			Args:     []string{"-h"},
			WantErr:  "pr: option requires an argument -- 'h'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid page number",
			Args:     []string{"+0"},
			WantErr:  "pr: invalid page range '0'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid page option",
			Args:     []string{"+x"},
			WantErr:  "pr: invalid option -- 'x'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid column count",
			Args:     []string{"-0"},
			WantErr:  "pr: -columns must be 1 or more\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid page length",
			Args:     []string{"-l", "0"},
			WantErr:  "pr: number of lines must be 1 or more: '0'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid offset",
			Args:     []string{"-o", "-1"},
			WantErr:  "pr: invalid offset: '-1'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid width",
			Args:     []string{"-w", "0"},
			WantErr:  "pr: -w width must be 1 or more: '0'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid number width",
			Args:     []string{"-n0"},
			WantErr:  "pr: -n width must be 1 or more: '0'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid -e argument",
			Args:     []string{"-e:x"},
			WantErr:  "pr: invalid value for -e: 'x'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid -s argument",
			Args:     []string{"-s||"},
			WantErr:  "pr: invalid value for -s: '||'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "-a requires columns",
			Args:     []string{"-a"},
			WantErr:  "pr: -a flag requires multiple columns\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "-m cannot be used with columns",
			Args:     []string{"-2", "-m", "a", "b"},
			WantErr:  "pr: -m cannot be used with multiple columns\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "-m cannot be used with -a",
			Args:     []string{"-m", "-a", "a", "b"},
			WantErr:  "pr: -m cannot be used with -a\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a too narrow page width",
			FS:       testFS(),
			Args:     []string{"-t", "-3", "-w", "2", "lorem.txt"},
			WantErr:  "pr: page width too narrow\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an unknown long option",
			Args:     []string{"--bogus"},
			WantErr:  "pr: unrecognized option '--bogus'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a long option without its argument",
			Args:     []string{"--width"},
			WantErr:  "pr: option '--width' requires an argument\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a long option which takes no argument",
			Args:     []string{"--across=1"},
			WantErr:  "pr: option '--across=1' doesn't allow an argument\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "--pages without a range",
			Args:     []string{"--pages"},
			WantErr:  "pr: option '--pages' requires an argument\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a negative page range",
			Args:     []string{"--pages=1:-1"},
			WantErr:  "pr: invalid page range '1:-1'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a decreasing page range",
			Args:     []string{"+2:1"},
			WantErr:  "pr: invalid page range '2:1'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a non numeric page range",
			Args:     []string{"+2:x"},
			WantErr:  "pr: invalid page range '2:x'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid starting line number",
			Args:     []string{"-N", "x"},
			WantErr:  "pr: invalid starting line number: 'x'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid -W width",
			Args:     []string{"-W", "0"},
			WantErr:  "pr: -W width must be 1 or more: '0'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a long separator without its argument",
			Args:     []string{"--separator"},
			WantErr:  "pr: option '--separator' requires an argument\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid column count",
			Args:     []string{"--columns=0"},
			WantErr:  "pr: invalid number of columns: '0'\nTry 'pr --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file",
			FS:       testFS(),
			Args:     []string{"missing.txt"},
			WantErr:  "pr: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file doesn't stop the next one",
			FS:       testFS(),
			Args:     []string{"-t", "missing.txt", "lorem.txt"},
			Want:     "lorem\nipsum\ndolor\n",
			WantErr:  "pr: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "-r suppresses the diagnostic but not the error",
			FS:       testFS(),
			Args:     []string{"-t", "-r", "missing.txt", "lorem.txt"},
			Want:     "lorem\nipsum\ndolor\n",
			WantExit: 1,
		},
		{
			Name:     "an operand after -- is a file",
			FS:       testFS(),
			Args:     []string{"-t", "--", "-l"},
			WantErr:  "pr: -l: file does not exist\n",
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
		FS:     testFS(),
	}
	err := New(rt).Run(t.Context(), "-t")

	if code := testutil.ExitCode(t, err); code != 1 {
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
		Stdout: testutil.ErrorWriter{Err: writeErr},
		Stderr: stderr,
		FS:     testFS(),
	}
	err := New(rt).Run(t.Context(), "-t")

	if code := testutil.ExitCode(t, err); code != 1 {
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
		testutil.Cancelled(t, New, strings.NewReader("lorem\n"), testFS(), "-t")
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		testutil.Cancelled(t, New, &testutil.CancelReader{}, testFS(), "-t")
	})
}

func TestRunShowChars(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "-c prints control characters in hat notation",
			Stdin: strings.NewReader("a\x07b\x7f\n"),
			Args:  []string{"-t", "-c"},
			Want:  "a^Gb^?\n",
		},
		{
			Name:  "-c prints other nonprinting characters in octal",
			Stdin: strings.NewReader("a\xc3b\n"),
			Args:  []string{"-t", "-c"},
			Want:  "a\\303b\n",
		},
		{
			Name:  "-c keeps tabs as they are",
			Stdin: strings.NewReader("a\tb\n"),
			Args:  []string{"-t", "-c"},
			Want:  "a\tb\n",
		},
		{
			Name:  "-v prints nonprinting characters in octal",
			Stdin: strings.NewReader("a\x07b\n"),
			Args:  []string{"-t", "-v"},
			Want:  "a\\007b\n",
		},
		{
			Name:  "-v keeps tabs as they are",
			Stdin: strings.NewReader("a\tb\n"),
			Args:  []string{"-t", "-v"},
			Want:  "a\tb\n",
		},
		{
			Name:  "-v counts the octal escapes towards the column width",
			Stdin: strings.NewReader("a\x07b\n"),
			Args:  []string{"-t", "-W4", "-v"},
			Want:  "a\n",
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunDateFormat(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-D formats the header date as date +FORMAT does",
			FS:   testFS(),
			Args: []string{"-l", "12", "-D", "Year %Y", "lorem.txt"},
			Want: "\n\nYear 2020                        lorem.txt" +
				"                        Page 1\n\n\n" + padded("lorem\nipsum\n", 2, 2) +
				"\n\nYear 2020                        lorem.txt" +
				"                        Page 2\n\n\n" + padded("dolor\n", 2, 1),
		},
		{
			Name: "-D accepts a literal text in place of the date",
			FS:   testFS(),
			Args: []string{"-l", "12", "-D", "-- Date/Time --", "lorem.txt"},
			Want: "\n\n-- Date/Time --                     lorem.txt" +
				"                     Page 1\n\n\n" + padded("lorem\nipsum\n", 2, 2) +
				"\n\n-- Date/Time --                     lorem.txt" +
				"                     Page 2\n\n\n" + padded("dolor\n", 2, 1),
		},
		{
			Name:    "POSIXLY_CORRECT gives the POSIX date layout",
			FS:      testFS(),
			Args:    []string{"-l", "12", "lorem.txt"},
			Environ: []string{"POSIXLY_CORRECT=1"},
			Want: "\n\nJan  2 03:04 2020                    lorem.txt" +
				"                    Page 1\n\n\n" + padded("lorem\nipsum\n", 2, 2) +
				"\n\nJan  2 03:04 2020                    lorem.txt" +
				"                    Page 2\n\n\n" + padded("dolor\n", 2, 1),
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunJoin(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-J joins full lines with the default separator",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-J", "odd.txt"},
			Want: "a\td\nb\te\nc\n",
		},
		{
			Name: "-J keeps the -S separator",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-J", "-S::", "odd.txt"},
			Want: "a::d\nb::e\nc\n",
		},
		{
			Name: "-J turns off the -W truncation",
			FS:   testFS(),
			Args: []string{"-t", "-W", "10", "-J", "-2", "odd.txt"},
			Want: "a\td\nb\te\nc\n",
		},
		{
			Name: "-J with -m joins the lines of the files",
			FS:   testFS(),
			Args: []string{"-t", "-J", "-m", "lorem.txt", "odd.txt"},
			Want: "lorem\ta\nipsum\tb\ndolor\tc\n\td\n\te\n",
		},
		{
			Name:  "-J with -m joins the lines of the standard input",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-t", "--join-lines", "--merge"},
			Want:  "a\nb\n",
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunFirstLineNumber(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-N starts the numbering at the given number",
			FS:   testFS(),
			Args: []string{"-t", "-n", "-N", "10", "lorem.txt"},
			Want: "   10\tlorem\n   11\tipsum\n   12\tdolor\n",
		},
		{
			Name: "-N may be negative",
			FS:   testFS(),
			Args: []string{"-t", "-n", "-N", "-1", "lorem.txt"},
			Want: "   -1\tlorem\n    0\tipsum\n    1\tdolor\n",
		},
		{
			Name: "-N overflows keeping the low-order digits",
			FS:   testFS(),
			Args: []string{"-t", "-n2", "-N", "98", "ten.txt"},
			Want: "98\t1\n99\t2\n00\t3\n01\t4\n02\t5\n03\t6\n04\t7\n05\t8\n06\t9\n07\t10\n",
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunSepString(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-S separates the columns with a string",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-S::", "odd.txt"},
			Want: "a\t\t\t\t   ::d\nb\t\t\t\t   ::e\nc\n",
		},
		{
			Name: "-S keeps the column alignment and truncation",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-S::", "long.txt"},
			Want: strings.Repeat("x", 35) + "\n",
		},
		{
			Name: "a bare -S gives an empty separator",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-S", "odd.txt"},
			Want: "a\t\t\t\t    d\nb\t\t\t\t    e\nc\n",
		},
		{
			Name: "-S dominates a later -s",
			FS:   testFS(),
			Args: []string{"-t", "-2", "-S::", "-s|", "odd.txt"},
			Want: "a\t\t\t\t   ::d\nb\t\t\t\t   ::e\nc\n",
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunOmitPagination(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "-T ignores a form feed in the middle of a line",
			Stdin: strings.NewReader("a\fb\n"),
			Args:  []string{"-t", "-T"},
			Want:  "ab\n",
		},
		{
			Name:  "-T drops a form feed with its newline",
			Stdin: strings.NewReader("a\n\f\nb\n"),
			Args:  []string{"-t", "-T"},
			Want:  "a\nb\n",
		},
		{
			Name:  "-t keeps a form feed in the middle of a line",
			Stdin: strings.NewReader("a\fb\n"),
			Args:  []string{"-t"},
			Want:  "a\n\fb\n",
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunPageWidth(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-W truncates single column lines",
			FS:   testFS(),
			Args: []string{"-t", "-W", "10", "long.txt"},
			Want: strings.Repeat("x", 10) + "\n",
		},
		{
			Name: "-w does not truncate single column lines",
			FS:   testFS(),
			Args: []string{"-t", "-w", "10", "long.txt"},
			Want: strings.Repeat("x", 80) + "\n",
		},
		{
			Name: "a later -w turns the truncation off, as GNU pr does",
			FS:   testFS(),
			Args: []string{"-t", "-W", "10", "-w", "40", "long.txt"},
			Want: strings.Repeat("x", 80) + "\n",
		},
		{
			Name: "-W truncates the lines numbered with -n",
			FS:   testFS(),
			Args: []string{"-t", "-W", "10", "-n2", "long.txt"},
			Want: " 1\t" + strings.Repeat("x", 2) + "\n",
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunPageRanges(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "+FIRST:LAST stops with the last page",
			FS:   testFS(),
			Args: []string{"+2:3", "-l", "12", "lorem.txt"},
			Want: header("lorem.txt", 2) + padded("dolor\n", 2, 1),
		},
		{
			Name: "--pages selects the same range",
			FS:   testFS(),
			Args: []string{"--pages=2:2", "-l", "12", "lorem.txt"},
			Want: header("lorem.txt", 2) + padded("dolor\n", 2, 1),
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the long forms of the page layout options",
			FS:   testFS(),
			Args: []string{
				"--omit-header", "--columns=2", "--sep-string=|",
				"--length=8", "ten.txt",
			},
			Want: "1\t\t\t\t   |6\n2\t\t\t\t   |7\n3\t\t\t\t   |8\n" +
				"4\t\t\t\t   |9\n5\t\t\t\t   |10\n",
		},
		{
			Name: "the long forms of the merge options",
			FS:   testFS(),
			Args: []string{
				"--merge", "--omit-header", "--number-lines=.2",
				"--indent=2", "odd.txt",
			},
			Want: "   1.a\n   2.b\n   3.c\n   4.d\n   5.e\n",
		},
		{
			Name: "the long forms of the tab options",
			FS:   testFS(),
			Args: []string{"-t", "--expand-tabs=4", "tabs.txt"},
			Want: "a   b\n",
		},
		{
			Name: "the long form of the double spacing option",
			FS:   testFS(),
			Args: []string{"-t", "--double-space", "lorem.txt"},
			Want: "lorem\n\nipsum\n\ndolor\n\n",
		},
		{
			Name: "the long form of the form feed option",
			FS:   testFS(),
			Args: []string{"--form-feed", "--length=12", "lorem.txt"},
			Want: header("lorem.txt", 1) + "lorem\nipsum\n\f" +
				header("lorem.txt", 2) + "dolor\n\f",
		},
		{
			Name: "the long form of the header option",
			FS:   testFS(),
			Args: []string{"--header=My Header", "-l", "12", "lorem.txt"},
			Want: header("My Header", 1) + padded("lorem\nipsum\n", 2, 2) +
				header("My Header", 2) + padded("dolor\n", 2, 1),
		},
		{
			Name:     "the long form of the warnings option",
			FS:       testFS(),
			Args:     []string{"--no-file-warnings", "-t", "missing.txt", "lorem.txt"},
			Want:     "lorem\nipsum\ndolor\n",
			WantExit: 1,
		},
		{
			Name: "long options take their argument in the next argument",
			FS:   testFS(),
			Args: []string{"--length", "12", "lorem.txt"},
			Want: header("lorem.txt", 1) + padded("lorem\nipsum\n", 2, 2) +
				header("lorem.txt", 2) + padded("dolor\n", 2, 1),
		},
		{
			Name: "--header takes its argument in the next argument",
			FS:   testFS(),
			Args: []string{"--header", "My Header", "--length", "12", "lorem.txt"},
			Want: header("My Header", 1) + padded("lorem\nipsum\n", 2, 2) +
				header("My Header", 2) + padded("dolor\n", 2, 1),
		},
		{
			Name: "--pages takes its range in the next argument",
			FS:   testFS(),
			Args: []string{"--pages", "2:2", "--length", "12", "lorem.txt"},
			Want: header("lorem.txt", 2) + padded("dolor\n", 2, 1),
		},
		{
			Name: "--width takes its value in the next argument",
			FS:   testFS(),
			Args: []string{"-t", "--width", "10", "long.txt"},
			Want: strings.Repeat("x", 80) + "\n",
		},
		{
			Name: "--columns takes its count in the next argument",
			FS:   testFS(),
			Args: []string{"-t", "--columns", "2", "odd.txt"},
			Want: "a\t\t\t\t    d\nb\t\t\t\t    e\nc\n",
		},
		{
			Name: "--separator takes its character in the next argument",
			FS:   testFS(),
			Args: []string{"-t", "--separator", "|", "--columns", "2", "odd.txt"},
			Want: "a|d\nb|e\nc\n",
		},
		{
			Name: "--first-line-number takes its value in the next argument",
			FS:   testFS(),
			Args: []string{"-t", "-n", "--first-line-number", "10", "lorem.txt"},
			Want: "   10\tlorem\n   11\tipsum\n   12\tdolor\n",
		},
		{
			Name: "--date-format takes its format in the next argument",
			FS:   testFS(),
			Args: []string{"--date-format", "Year %Y", "-l", "12", "lorem.txt"},
			Want: "\n\nYear 2020                        lorem.txt" +
				"                        Page 1\n\n\n" + padded("lorem\nipsum\n", 2, 2) +
				"\n\nYear 2020                        lorem.txt" +
				"                        Page 2\n\n\n" + padded("dolor\n", 2, 1),
		},
		{
			Name: "--indent takes its margin in the next argument",
			FS:   testFS(),
			Args: []string{"--indent", "4", "-l", "12", "odd.txt"},
			Want: headerOffset("odd.txt", 1, 4) + padded("    a\n    b\n", 2, 2) +
				headerOffset("odd.txt", 2, 4) + padded("    c\n    d\n", 2, 2) +
				headerOffset("odd.txt", 3, 4) + padded("    e\n", 2, 1),
		},
		{
			Name: "the long form of the printing across option",
			FS:   testFS(),
			Args: []string{"-t", "--across", "--columns=2", "odd.txt"},
			Want: "a\t\t\t\t    b\nc\t\t\t\t    d\ne\n",
		},
		{
			Name: "the long form of the width option",
			FS:   testFS(),
			Args: []string{"-t", "--page-width=10", "long.txt"},
			Want: strings.Repeat("x", 10) + "\n",
		},
	}
	testutil.Run(t, New, tests)
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "pr", usage)
}
