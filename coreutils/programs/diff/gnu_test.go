package diff

import (
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// gnuFS holds the fixtures of the GNU extensions:
//
//	case1.txt, case2.txt: the same lines in the other case
//	ws1.txt: "a b\n"     ws2.txt: "ab\n" (no blank at all)
//	tab1.txt: "a\tb\n"   tab2.txt: "a       b\n" (the tab expanded)
//	trail1.txt: "a \n"   trail2.txt: "a\n" (without the trailing blank)
//	blank1: "a\n\nb\n"   blank2: "a\nb\n" (without the blank line)
//	re1.txt: "a\nfoo\nb\n"  re2.txt: "a\nb\n" (without the matching line)
//	crlf1.txt: "a\r\nb\r\n" crlf2.txt: "a\nb\n"
//	e1.txt: "a\n" e2.txt: "a\nb\n" e3.txt: "b\n"
//	n1/x, n2/x: differing files; n1/only: "z\n"; n2/new: "n\n"
//	n1/sub/f, n2/sub: only one of them holds the sub directory
func gnuFS() fsx.MemFS {
	return fsx.MemFS{
		"case1.txt":  {Data: []byte("A\nb\n")},
		"case2.txt":  {Data: []byte("a\nB\n")},
		"ws1.txt":    {Data: []byte("a b\n")},
		"ws2.txt":    {Data: []byte("ab\n")},
		"tab1.txt":   {Data: []byte("a\tb\n")},
		"tab2.txt":   {Data: []byte("a       b\n")},
		"trail1.txt": {Data: []byte("a \n")},
		"trail2.txt": {Data: []byte("a\n")},
		"blank1":     {Data: []byte("a\n\nb\n")},
		"blank2":     {Data: []byte("a\nb\n")},
		"re1.txt":    {Data: []byte("a\nfoo\nb\n")},
		"re2.txt":    {Data: []byte("a\nb\n")},
		"crlf1.txt":  {Data: []byte("a\r\nb\r\n")},
		"crlf2.txt":  {Data: []byte("a\nb\n")},
		"e1.txt":     {Data: []byte("a\n")},
		"e2.txt":     {Data: []byte("a\nb\n")},
		"e3.txt":     {Data: []byte("b\n")},
		"n1/x":       {Data: []byte("a\n")},
		"n2/x":       {Data: []byte("b\n")},
		"n1/only":    {Data: []byte("z\n")},
		"n2/new":     {Data: []byte("n\n")},
		"n1/sub/f":   {Data: []byte("q\n")},
	}
}

func TestRunBrief(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "differing files report only that they differ",
			FS:       testFS(),
			Args:     []string{"-q", "a.txt", "b.txt"},
			Want:     "Files a.txt and b.txt differ\n",
			WantExit: 1,
		},
		{
			Name: "identical files report nothing",
			FS:   testFS(),
			Args: []string{"-q", "same1.txt", "same2.txt"},
		},
		{
			Name:     "binary files report like the others",
			FS:       testFS(),
			Args:     []string{"-q", "bin1", "bin2"},
			Want:     "Files bin1 and bin2 differ\n",
			WantExit: 1,
		},
		{
			Name:     "the long form reports the same",
			FS:       testFS(),
			Args:     []string{"--brief", "a.txt", "c.txt"},
			Want:     "Files a.txt and c.txt differ\n",
			WantExit: 1,
		},
		{
			Name:     "brief wins over an output format",
			FS:       testFS(),
			Args:     []string{"-q", "-u", "a.txt", "b.txt"},
			Want:     "Files a.txt and b.txt differ\n",
			WantExit: 1,
		},
		{
			Name:     "changes ignored by -b make files equal",
			FS:       testFS(),
			Args:     []string{"-q", "-b", "blank1.txt", "blank2.txt"},
			WantExit: 0,
		},
		{
			Name:     "directories report their files without headers",
			FS:       testFS(),
			Args:     []string{"-r", "-q", "d1", "d2"},
			Want:     "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\nFiles d1/sub/f and d2/sub/f differ\nFiles d1/x and d2/x differ\n",
			WantExit: 1,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunReportIdentical(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "identical files are reported",
			FS:   testFS(),
			Args: []string{"-s", "same1.txt", "same2.txt"},
			Want: "Files same1.txt and same2.txt are identical\n",
		},
		{
			Name: "the long form reports the same",
			FS:   testFS(),
			Args: []string{"--report-identical-files", "a.txt", "a.txt"},
			Want: "Files a.txt and a.txt are identical\n",
		},
		{
			Name:     "differing files keep their output",
			FS:       testFS(),
			Args:     []string{"-s", "a.txt", "b.txt"},
			Want:     "2c2\n< 2\n---\n> x\n",
			WantExit: 1,
		},
		{
			Name: "identical binary files are reported",
			FS:   testFS(),
			Args: []string{"-s", "binsame1", "binsame2"},
			Want: "Files binsame1 and binsame2 are identical\n",
		},
		{
			Name: "changes ignored by -b are reported identical",
			FS:   testFS(),
			Args: []string{"-s", "-b", "blank1.txt", "blank2.txt"},
			Want: "Files blank1.txt and blank2.txt are identical\n",
		},
		{
			Name:     "identical files of a directory comparison are reported",
			FS:       gnuFS(),
			Args:     []string{"-r", "-s", "n1", "n1"},
			Want:     "Files n1/only and n1/only are identical\nFiles n1/sub/f and n1/sub/f are identical\nFiles n1/x and n1/x are identical\n",
			WantExit: 0,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunIgnoreOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "case is significant without -i",
			FS:       gnuFS(),
			Args:     []string{"case1.txt", "case2.txt"},
			Want:     "1,2c1,2\n< A\n< b\n---\n> a\n> B\n",
			WantExit: 1,
		},
		{
			Name: "case is ignored with -i",
			FS:   gnuFS(),
			Args: []string{"-i", "case1.txt", "case2.txt"},
		},
		{
			Name: "case is ignored with --ignore-case",
			FS:   gnuFS(),
			Args: []string{"--ignore-case", "case1.txt", "case2.txt"},
		},
		{
			Name:     "a missing blank differs with -b",
			FS:       gnuFS(),
			Args:     []string{"-b", "ws1.txt", "ws2.txt"},
			Want:     "1c1\n< a b\n---\n> ab\n",
			WantExit: 1,
		},
		{
			Name: "all blanks are ignored with -w",
			FS:   gnuFS(),
			Args: []string{"-w", "ws1.txt", "ws2.txt"},
		},
		{
			Name: "all blanks are ignored with --ignore-all-space",
			FS:   gnuFS(),
			Args: []string{"--ignore-all-space", "ws1.txt", "ws2.txt"},
		},
		{
			Name: "tab expansion is ignored with -E",
			FS:   gnuFS(),
			Args: []string{"-E", "tab1.txt", "tab2.txt"},
		},
		{
			Name: "tab expansion is ignored with --ignore-tab-expansion",
			FS:   gnuFS(),
			Args: []string{"--ignore-tab-expansion", "tab1.txt", "tab2.txt"},
		},
		{
			Name:     "the amount of spaces differs with -E alone",
			FS:       gnuFS(),
			Args:     []string{"-E", "ws1.txt", "e2.txt"},
			Want:     "1c1,2\n< a b\n---\n> a\n> b\n",
			WantExit: 1,
		},
		{
			Name: "trailing space is ignored with -Z",
			FS:   gnuFS(),
			Args: []string{"-Z", "trail1.txt", "trail2.txt"},
		},
		{
			Name: "trailing space is ignored with --ignore-trailing-space",
			FS:   gnuFS(),
			Args: []string{"--ignore-trailing-space", "trail1.txt", "trail2.txt"},
		},
		{
			Name: "blank lines are ignored with -B",
			FS:   gnuFS(),
			Args: []string{"-B", "blank1", "blank2"},
		},
		{
			Name: "blank lines are ignored with --ignore-blank-lines",
			FS:   gnuFS(),
			Args: []string{"--ignore-blank-lines", "blank1", "blank2"},
		},
		{
			Name:     "changes with lines which aren't blank still come out",
			FS:       fsx.MemFS{"b1": {Data: []byte("a\n\nb\n")}, "b2": {Data: []byte("a\nc\n")}},
			Args:     []string{"-B", "b1", "b2"},
			Want:     "2,3c2\n< \n< b\n---\n> c\n",
			WantExit: 1,
		},
		{
			Name: "lines matching the -I pattern are ignored",
			FS:   gnuFS(),
			Args: []string{"-I", "^foo$", "re1.txt", "re2.txt"},
		},
		{
			Name: "lines matching the --ignore-matching-lines pattern are ignored",
			FS:   gnuFS(),
			Args: []string{"--ignore-matching-lines=^foo$", "re1.txt", "re2.txt"},
		},
		{
			Name:     "lines matching nothing keep differing",
			FS:       gnuFS(),
			Args:     []string{"-I", "^bar$", "re1.txt", "re2.txt"},
			Want:     "2d1\n< foo\n",
			WantExit: 1,
		},
		{
			Name: "patterns match anywhere in the line",
			FS:   gnuFS(),
			Args: []string{"-I", "oo", "re1.txt", "re2.txt"},
		},
		{
			Name:     "several patterns all ignore their lines",
			FS:       gnuFS(),
			Args:     []string{"-I", "^bar$", "-I", "^foo$", "re1.txt", "re2.txt"},
			WantExit: 0,
		},
		{
			Name: "case ignored with the blanks too",
			FS: fsx.MemFS{
				"c1": {Data: []byte("A\tb\n")},
				"c2": {Data: []byte("a    B\n")},
			},
			Args: []string{"-i", "-b", "c1", "c2"},
		},
		{
			Name:     "invalid pattern",
			FS:       gnuFS(),
			Args:     []string{"-I", "*", "re1.txt", "re2.txt"},
			WantErr:  "diff: error parsing regexp: missing argument to repetition operator: `*`\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunText(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "binary files compared as text",
			FS:       testFS(),
			Args:     []string{"-a", "bin1", "bin2"},
			Want:     "1c1\n< \x00a\n\\ No newline at end of file\n---\n> \x00b\n\\ No newline at end of file\n",
			WantExit: 1,
		},
		{
			Name:     "binary files compared as text with --text",
			FS:       testFS(),
			Args:     []string{"--text", "bin1", "bin2"},
			Want:     "1c1\n< \x00a\n\\ No newline at end of file\n---\n> \x00b\n\\ No newline at end of file\n",
			WantExit: 1,
		},
		{
			Name: "identical binary files compared as text",
			FS:   testFS(),
			Args: []string{"-a", "binsame1", "binsame2"},
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunStripCR(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "lines ending with a carriage return compare equal",
			FS:   gnuFS(),
			Args: []string{"--strip-trailing-cr", "crlf1.txt", "crlf2.txt"},
		},
		{
			Name:     "the carriage returns come out without the option",
			FS:       gnuFS(),
			Args:     []string{"crlf1.txt", "crlf2.txt"},
			Want:     "1,2c1,2\n< a\r\n< b\r\n---\n> a\n> b\n",
			WantExit: 1,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunObsoleteContext(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "the count alone is the context of the copied format",
			FS:       formatFS(),
			Args:     []string{"-1", "-c", "m3.txt", "m4.txt"},
			Want:     "*** m3.txt\tFri Jan  2 03:04:05 2026\n--- m4.txt\tFri Jan  2 03:04:05 2026\n***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! X\n  3\n",
			WantExit: 1,
		},
		{
			Name:     "the count alone lowers the default of the unified format",
			FS:       formatFS(),
			Args:     []string{"-1", "-u", "m3.txt", "m4.txt"},
			Want:     "--- m3.txt\t2026-01-02 03:04:05 +0000\n+++ m4.txt\t2026-01-02 03:04:05 +0000\n@@ -1,3 +1,3 @@\n 1\n-2\n+X\n 3\n",
			WantExit: 1,
		},
		{
			Name:     "two digits make one count",
			FS:       formatFS(),
			Args:     []string{"-12", "-c", "m3.txt", "m4.txt"},
			Want:     "*** m3.txt\tFri Jan  2 03:04:05 2026\n--- m4.txt\tFri Jan  2 03:04:05 2026\n***************\n*** 1,5 ****\n  1\n! 2\n  3\n  4\n  5\n--- 1,5 ----\n  1\n! X\n  3\n  4\n  5\n",
			WantExit: 1,
		},
		{
			Name:     "the count alone compares nothing of the context in the normal format",
			FS:       formatFS(),
			Args:     []string{"-5", "m3.txt", "m4.txt"},
			Want:     "2c2\n< 2\n---\n> X\n",
			WantExit: 1,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunRCS(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "changed line",
			FS:       formatFS(),
			Args:     []string{"-n", "m1.txt", "m2.txt"},
			Want:     "d2 1\na2 1\nx\n",
			WantExit: 1,
		},
		{
			Name:     "changed line with --rcs",
			FS:       formatFS(),
			Args:     []string{"--rcs", "m1.txt", "m2.txt"},
			Want:     "d2 1\na2 1\nx\n",
			WantExit: 1,
		},
		{
			Name:     "added line",
			FS:       formatFS(),
			Args:     []string{"-n", "m2.txt", "m1.txt"},
			Want:     "d2 1\na2 1\n2\n",
			WantExit: 1,
		},
		{
			Name:     "lines under directories keep the header",
			FS:       testFS(),
			Args:     []string{"-r", "-n", "d1", "d2"},
			Want:     "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\ndiff -r -n d1/sub/f d2/sub/f\nd1 1\na1 1\nw\ndiff -r -n d1/x d2/x\nd1 1\na1 1\nb\n",
			WantExit: 1,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunNewFile(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "a file missing from the first operand",
			FS:       testFS(),
			Args:     []string{"-N", "nope", "b.txt"},
			Want:     "0a1,3\n> 1\n> x\n> 3\n",
			WantExit: 1,
		},
		{
			Name:     "a file missing from the second operand",
			FS:       testFS(),
			Args:     []string{"-N", "b.txt", "nope"},
			Want:     "1,3d0\n< 1\n< x\n< 3\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file without the option is an error",
			FS:       testFS(),
			Args:     []string{"b.txt", "nope"},
			WantErr:  "diff: nope: file does not exist\n",
			WantExit: 2,
		},
		{
			Name:     "a missing file against a missing file",
			FS:       testFS(),
			Args:     []string{"-N", "nope", "nope2"},
			WantErr:  "diff: nope: file does not exist\n",
			WantExit: 2,
		},
		{
			Name:     "the long form compares the same",
			FS:       testFS(),
			Args:     []string{"--new-file", "nope", "b.txt"},
			Want:     "0a1,3\n> 1\n> x\n> 3\n",
			WantExit: 1,
		},
		{
			Name: "missing files under directories",
			FS:   gnuFS(),
			Args: []string{"-r", "-N", "n1", "n2"},
			Want: "diff -r -N n1/new n2/new\n0a1\n> n\n" +
				"diff -r -N n1/only n2/only\n1d0\n< z\n" +
				"diff -r -N n1/sub/f n2/sub/f\n1d0\n< q\n" +
				"diff -r -N n1/x n2/x\n1c1\n< a\n---\n> b\n",
			WantExit: 1,
		},
		{
			Name: "missing subdirectories under directories",
			FS: fsx.MemFS{
				"n1/sdir/f": {Data: []byte("k\n")},
				"n2/x":      {Data: []byte("b\n")},
			},
			Args: []string{"-r", "-N", "n1/sdir", "n2"},
			Want: "diff -r -N n1/sdir/f n2/f\n1d0\n< k\n" +
				"diff -r -N n1/sdir/x n2/x\n0a1\n> b\n",
			WantExit: 1,
		},
		{
			Name: "a missing subdirectory without the recursive option is only reported",
			FS:   gnuFS(),
			Args: []string{"-N", "n1", "n2"},
			Want: "diff -N n1/new n2/new\n0a1\n> n\n" +
				"diff -N n1/only n2/only\n1d0\n< z\n" +
				"Only in n1: sub\n" +
				"diff -N n1/x n2/x\n1c1\n< a\n---\n> b\n",
			WantExit: 1,
		},
		{
			Name: "unidirectional treats absent first files as empty",
			FS:   gnuFS(),
			Args: []string{"-r", "-P", "n1", "n2"},
			Want: "diff -r -P n1/new n2/new\n0a1\n> n\n" +
				"Only in n1: only\n" +
				"Only in n1: sub\n" +
				"diff -r -P n1/x n2/x\n1c1\n< a\n---\n> b\n",
			WantExit: 1,
		},
		{
			Name: "the long form of the unidirectional option",
			FS:   gnuFS(),
			Args: []string{"-r", "--unidirectional-new-file", "n1", "n2"},
			Want: "diff -r --unidirectional-new-file n1/new n2/new\n0a1\n> n\n" +
				"Only in n1: only\n" +
				"Only in n1: sub\n" +
				"diff -r --unidirectional-new-file n1/x n2/x\n1c1\n< a\n---\n> b\n",
			WantExit: 1,
		},
		{
			Name:     "missing files report briefly",
			FS:       testFS(),
			Args:     []string{"-q", "-N", "nope", "b.txt"},
			Want:     "Files nope and b.txt differ\n",
			WantExit: 1,
		},
		{
			Name:     "the standard input counts as present",
			Stdin:    strings.NewReader("1\nx\n3\n"),
			FS:       testFS(),
			Args:     []string{"-N", "-", "nope"},
			Want:     "1,3d0\n< 1\n< x\n< 3\n",
			WantExit: 1,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunLabels(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "labels in the unified headers",
			FS:       formatFS(),
			Args:     []string{"-L", "one", "-L", "two", "-u", "m1.txt", "m2.txt"},
			Want:     "--- one\n+++ two\n@@ -1,3 +1,3 @@\n 1\n-2\n+x\n 3\n",
			WantExit: 1,
		},
		{
			Name:     "labels in the context headers",
			FS:       formatFS(),
			Args:     []string{"-L", "one", "-c", "m1.txt", "m2.txt"},
			Want:     "*** one\n--- m2.txt\tFri Jan  2 03:04:05 2026\n***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! x\n  3\n",
			WantExit: 1,
		},
		{
			Name:     "labels in the brief message",
			FS:       formatFS(),
			Args:     []string{"-q", "-L", "one", "-L", "two", "m1.txt", "m2.txt"},
			Want:     "Files one and two differ\n",
			WantExit: 1,
		},
		{
			Name:     "labels in the identical message",
			FS:       testFS(),
			Args:     []string{"-s", "-L", "one", "-L", "two", "same1.txt", "same2.txt"},
			Want:     "Files one and two are identical\n",
			WantExit: 0,
		},
		{
			Name:     "the long form",
			FS:       formatFS(),
			Args:     []string{"--label", "one", "--label", "two", "-u", "m1.txt", "m2.txt"},
			Want:     "--- one\n+++ two\n@@ -1,3 +1,3 @@\n 1\n-2\n+x\n 3\n",
			WantExit: 1,
		},
		{
			Name:     "too many labels",
			FS:       formatFS(),
			Args:     []string{"-L", "one", "-L", "two", "-L", "three", "m1.txt", "m2.txt"},
			WantErr:  "diff: too many file label options\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunIfdef(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "merged output",
			FS:       testFS(),
			Args:     []string{"-D", "NAME", "a.txt", "b.txt"},
			Want:     "1\n#ifndef NAME\n2\n#else /* NAME */\nx\n#endif /* NAME */\n3\n",
			WantExit: 1,
		},
		{
			Name:     "merged output with the long form",
			FS:       testFS(),
			Args:     []string{"--ifdef=NAME", "a.txt", "b.txt"},
			Want:     "1\n#ifndef NAME\n2\n#else /* NAME */\nx\n#endif /* NAME */\n3\n",
			WantExit: 1,
		},
		{
			Name:     "conflicting output style",
			FS:       testFS(),
			Args:     []string{"-D", "NAME", "-u", "a.txt", "b.txt"},
			WantErr:  "diff: conflicting output style options\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "directories take no merged output",
			FS:       testFS(),
			Args:     []string{"-r", "-D", "NAME", "d1", "d2"},
			WantErr:  "diff: -D option not supported with directories\n",
			WantExit: 2,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunFromToFiles(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "a from-file compares every operand against it",
			FS:       gnuFS(),
			Args:     []string{"--from-file=e1.txt", "e2.txt", "e3.txt"},
			Want:     "1a2\n> b\n1c1\n< a\n---\n> b\n",
			WantExit: 1,
		},
		{
			Name:     "a to-file compares every operand to it",
			FS:       gnuFS(),
			Args:     []string{"--to-file=e3.txt", "e1.txt", "e2.txt"},
			Want:     "1c1\n< a\n---\n> b\n1d0\n< a\n",
			WantExit: 1,
		},
		{
			Name:     "an identical comparison writes nothing",
			FS:       gnuFS(),
			Args:     []string{"--from-file=e1.txt", "e1.txt"},
			WantExit: 0,
		},
		{
			Name:     "both options together",
			FS:       gnuFS(),
			Args:     []string{"--from-file=e1.txt", "--to-file=e2.txt", "e3.txt"},
			WantErr:  "diff: --from-file and --to-file both specified\n",
			WantExit: 2,
		},
		{
			Name:     "no operands compares nothing",
			FS:       gnuFS(),
			Args:     []string{"--from-file=e1.txt"},
			WantExit: 0,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunTabs(t *testing.T) {
	tabs := fsx.MemFS{
		"t1": {Data: []byte("a\tb\n")},
		"t2": {Data: []byte("a b\n")},
	}
	tests := []testutil.Case{
		{
			Name:     "tabs stay as they are without the option",
			FS:       tabs,
			Args:     []string{"t1", "t2"},
			Want:     "1c1\n< a\tb\n---\n> a b\n",
			WantExit: 1,
		},
		{
			Name:     "tabs expand to spaces",
			FS:       tabs,
			Args:     []string{"-t", "t1", "t2"},
			Want:     "1c1\n< a       b\n---\n> a b\n",
			WantExit: 1,
		},
		{
			Name:     "tabs expand to spaces with the long form",
			FS:       tabs,
			Args:     []string{"--expand-tabs", "t1", "t2"},
			Want:     "1c1\n< a       b\n---\n> a b\n",
			WantExit: 1,
		},
		{
			Name:     "tabs expand to the tab size given",
			FS:       tabs,
			Args:     []string{"--tabsize=4", "-t", "t1", "t2"},
			Want:     "1c1\n< a   b\n---\n> a b\n",
			WantExit: 1,
		},
		{
			Name:     "lines get an initial tab",
			FS:       testFS(),
			Args:     []string{"-T", "a.txt", "b.txt"},
			Want:     "2c2\n<\t2\n---\n>\tx\n",
			WantExit: 1,
		},
		{
			Name:     "lines get an initial tab in the unified format",
			FS:       testFS(),
			Args:     []string{"--initial-tab", "-u", "a.txt", "b.txt"},
			Want:     "--- a.txt\t0001-01-01 00:00:00 +0000\n+++ b.txt\t0001-01-01 00:00:00 +0000\n@@ -1,3 +1,3 @@\n\t1\n-\t2\n+\tx\n\t3\n",
			WantExit: 1,
		},
		{
			Name:     "lines get an initial tab in the context format",
			FS:       testFS(),
			Args:     []string{"-T", "-c", "a.txt", "b.txt"},
			Want:     "*** a.txt\tMon Jan  1 00:00:00 0001\n--- b.txt\tMon Jan  1 00:00:00 0001\n***************\n*** 1,3 ****\n \t1\n!\t2\n \t3\n--- 1,3 ----\n \t1\n!\tx\n \t3\n",
			WantExit: 1,
		},
		{
			Name:     "invalid tab size",
			FS:       tabs,
			Args:     []string{"--tabsize=x", "-t", "t1", "t2"},
			WantErr:  "diff: invalid tabsize 'x'\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "zero tab size",
			FS:       tabs,
			Args:     []string{"--tabsize=0", "-t", "t1", "t2"},
			WantErr:  "diff: invalid tabsize '0'\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "conflicting tab sizes",
			FS:       tabs,
			Args:     []string{"--tabsize=4", "--tabsize=8", "t1", "t2"},
			WantErr:  "diff: conflicting tabsize options\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunSideBySide(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "two columns",
			FS:       testFS(),
			Args:     []string{"-y", "-W", "20", "a.txt", "b.txt"},
			Want:     "1\t1\n2     |\tx\n3\t3\n",
			WantExit: 1,
		},
		{
			Name:     "two columns with the long form",
			FS:       testFS(),
			Args:     []string{"--side-by-side", "-W", "20", "a.txt", "b.txt"},
			Want:     "1\t1\n2     |\tx\n3\t3\n",
			WantExit: 1,
		},
		{
			Name:     "deleted line keeps the left column alone",
			FS:       testFS(),
			Args:     []string{"-y", "-W", "20", "a.txt", "c.txt"},
			Want:     "1\t1\n2\t2\n3     <\n",
			WantExit: 1,
		},
		{
			Name:     "added line keeps the right column alone",
			FS:       testFS(),
			Args:     []string{"-y", "-W", "20", "c.txt", "a.txt"},
			Want:     "1\t1\n2\t2\n      >\t3\n",
			WantExit: 1,
		},
		{
			Name:     "identical files still come out in both columns",
			FS:       testFS(),
			Args:     []string{"-y", "-W", "20", "same1.txt", "same2.txt"},
			Want:     "a\ta\nb\tb\n",
			WantExit: 0,
		},
		{
			Name:     "the left column only",
			FS:       testFS(),
			Args:     []string{"-y", "-W", "20", "--left-column", "a.txt", "b.txt"},
			Want:     "1     (\n2     |\tx\n3     (\n",
			WantExit: 1,
		},
		{
			Name:     "suppressed common lines",
			FS:       testFS(),
			Args:     []string{"-y", "-W", "20", "--suppress-common-lines", "a.txt", "b.txt"},
			Want:     "2     |\tx\n",
			WantExit: 1,
		},
		{
			Name:     "a width of its own with the long form",
			FS:       testFS(),
			Args:     []string{"-y", "--width=20", "a.txt", "b.txt"},
			Want:     "1\t1\n2     |\tx\n3\t3\n",
			WantExit: 1,
		},
		{
			Name:     "invalid width",
			FS:       testFS(),
			Args:     []string{"-y", "-W", "0", "a.txt", "b.txt"},
			WantErr:  "diff: invalid width '0'\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "conflicting widths",
			FS:       testFS(),
			Args:     []string{"-y", "-W", "20", "-W", "30", "a.txt", "b.txt"},
			WantErr:  "diff: conflicting width options\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "conflicting output style",
			FS:       testFS(),
			Args:     []string{"-y", "-u", "a.txt", "b.txt"},
			WantErr:  "diff: conflicting output style options\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunExcludes(t *testing.T) {
	excludeFS := fsx.MemFS{
		"n1/a":     {Data: []byte("1\n")},
		"n2/a":     {Data: []byte("2\n")},
		"n1/b.log": {Data: []byte("x\n")},
		"n2/b.log": {Data: []byte("y\n")},
		"pat":      {Data: []byte("*.log\n")},
	}
	tests := []testutil.Case{
		{
			Name:     "excluded names are not compared",
			FS:       excludeFS,
			Args:     []string{"-r", "-x", "*.log", "n1", "n2"},
			Want:     "diff -r -x '*.log' n1/a n2/a\n1c1\n< 1\n---\n> 2\n",
			WantExit: 1,
		},
		{
			Name: "names which don't match are compared",
			FS:   excludeFS,
			Args: []string{"-r", "-x", "*.txt", "n1", "n2"},
			Want: "diff -r -x '*.txt' n1/a n2/a\n1c1\n< 1\n---\n> 2\n" +
				"diff -r -x '*.txt' n1/b.log n2/b.log\n1c1\n< x\n---\n> y\n",
			WantExit: 1,
		},
		{
			Name:     "patterns come from a file",
			FS:       excludeFS,
			Args:     []string{"-r", "-X", "pat", "n1", "n2"},
			Want:     "diff -r -X pat n1/a n2/a\n1c1\n< 1\n---\n> 2\n",
			WantExit: 1,
		},
		{
			Name:     "several patterns exclude together",
			FS:       excludeFS,
			Args:     []string{"-r", "-x", "a", "-x", "*.log", "n1", "n2"},
			WantExit: 0,
		},
		{
			Name:     "a missing pattern file",
			FS:       excludeFS,
			Args:     []string{"-r", "-X", "nope", "n1", "n2"},
			WantErr:  "diff: nope: file does not exist\n",
			WantExit: 2,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunLongAliases(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "--recursive",
			FS:       testFS(),
			Args:     []string{"--recursive", "d1", "d2"},
			Want:     "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\ndiff --recursive d1/sub/f d2/sub/f\n1c1\n< q\n---\n> w\ndiff --recursive d1/x d2/x\n1c1\n< a\n---\n> b\n",
			WantExit: 1,
		},
		{
			Name:     "--ed",
			FS:       formatFS(),
			Args:     []string{"--ed", "m1.txt", "m2.txt"},
			Want:     "2c\nx\n.\n",
			WantExit: 1,
		},
		{
			Name:     "--forward-ed",
			FS:       formatFS(),
			Args:     []string{"--forward-ed", "m1.txt", "m2.txt"},
			Want:     "c2\nx\n.\n",
			WantExit: 1,
		},
		{
			Name:     "--context with a count",
			FS:       formatFS(),
			Args:     []string{"--context=1", "m3.txt", "m4.txt"},
			Want:     "*** m3.txt\tFri Jan  2 03:04:05 2026\n--- m4.txt\tFri Jan  2 03:04:05 2026\n***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! X\n  3\n",
			WantExit: 1,
		},
		{
			Name:     "--context alone gives three lines",
			FS:       formatFS(),
			Args:     []string{"--context", "m1.txt", "m2.txt"},
			Want:     "*** m1.txt\tFri Jan  2 03:04:05 2026\n--- m2.txt\tFri Jan  2 03:04:05 2026\n***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! x\n  3\n",
			WantExit: 1,
		},
		{
			Name:     "--unified with a count",
			FS:       formatFS(),
			Args:     []string{"--unified=0", "m3.txt", "m4.txt"},
			Want:     "--- m3.txt\t2026-01-02 03:04:05 +0000\n+++ m4.txt\t2026-01-02 03:04:05 +0000\n@@ -2 +2 @@\n-2\n+X\n",
			WantExit: 1,
		},
		{
			Name:     "--unified alone gives three lines",
			FS:       formatFS(),
			Args:     []string{"--unified", "m1.txt", "m2.txt"},
			Want:     "--- m1.txt\t2026-01-02 03:04:05 +0000\n+++ m2.txt\t2026-01-02 03:04:05 +0000\n@@ -1,3 +1,3 @@\n 1\n-2\n+x\n 3\n",
			WantExit: 1,
		},
		{
			Name:     "--normal",
			FS:       formatFS(),
			Args:     []string{"--normal", "m1.txt", "m2.txt"},
			Want:     "2c2\n< 2\n---\n> x\n",
			WantExit: 1,
		},
		{
			Name:     "--ignore-space-change",
			FS:       testFS(),
			Args:     []string{"--ignore-space-change", "blank1.txt", "blank2.txt"},
			WantExit: 0,
		},
		{
			Name:     "--binary is accepted",
			FS:       formatFS(),
			Args:     []string{"--binary", "m1.txt", "m2.txt"},
			Want:     "2c2\n< 2\n---\n> x\n",
			WantExit: 1,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunInvalidUsage(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "unrecognized long option",
			FS:       testFS(),
			Args:     []string{"--nope", "a.txt", "b.txt"},
			WantErr:  "diff: unrecognized option '--nope'\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing option argument",
			FS:       testFS(),
			Args:     []string{"-x"},
			WantErr:  "diff: option requires an argument -- 'x'\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "argument of an option which takes none",
			FS:       testFS(),
			Args:     []string{"--brief=x", "a.txt", "b.txt"},
			WantErr:  "diff: option '--brief=x' doesn't allow an argument\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing label argument",
			FS:       testFS(),
			Args:     []string{"-L"},
			WantErr:  "diff: option requires an argument -- 'L'\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunVersionOption(t *testing.T) {
	testutil.Once(t, New, testutil.Case{
		Name: "-v",
		Args: []string{"-v"},
		Want: "diff (charm.land/turtle/coreutils) " + coreutils.Version + "\nhttps://charm.land/turtle\n",
	})
}

func TestRunBriefWithIdentical(t *testing.T) {
	testutil.Once(t, New, testutil.Case{
		Name:     "brief and identical together",
		FS:       testFS(),
		Args:     []string{"-q", "-s", "same1.txt", "same2.txt"},
		Want:     "Files same1.txt and same2.txt are identical\n",
		WantExit: 0,
	})
}

func TestRunFinalNewline(t *testing.T) {
	// A file whose last line ends without a newline stays different from
	// one whose matching line ends with one, even when the contents are
	// compared loosely: diff doesn't silently complete lines. The blank
	// ignoring comparisons (-Z, -b, -w) are the exception, matching GNU,
	// which treats the missing newline as one more trailing blank there.
	tests := []testutil.Case{
		{
			Name:     "a missing final newline stays a difference with -i",
			FS:       fsx.MemFS{"c1": {Data: []byte("a\nB\n")}, "c2": {Data: []byte("A\nb")}},
			Args:     []string{"-i", "c1", "c2"},
			Want:     "2c2\n< B\n---\n> b\n\\ No newline at end of file\n",
			WantExit: 1,
		},
		{
			Name:     "a missing final newline stays a difference with -E",
			FS:       testFS(),
			Args:     []string{"-E", "c.txt", "nonl1"},
			Want:     "2c2\n< 2\n---\n> 2\n\\ No newline at end of file\n",
			WantExit: 1,
		},
		{
			Name:     "a missing final newline on both sides compares equal",
			FS:       testFS(),
			Args:     []string{"-i", "nonl1", "nonl2"},
			Want:     "2c2\n< 2\n\\ No newline at end of file\n---\n> x\n\\ No newline at end of file\n",
			WantExit: 1,
		},
		{
			Name: "a trailing blank comparison completes the final line",
			FS:   testFS(),
			Args: []string{"-b", "c.txt", "nonl1"},
		},
		{
			Name: "all space comparison completes the final line",
			FS:   testFS(),
			Args: []string{"-w", "c.txt", "nonl1"},
		},
	}
	testutil.Run(t, New, tests)
}
