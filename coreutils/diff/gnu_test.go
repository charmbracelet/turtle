package diff

import (
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
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
	tests := []testCase{
		{
			name:     "differing files report only that they differ",
			fsys:     testFS(),
			args:     []string{"-q", "a.txt", "b.txt"},
			want:     "Files a.txt and b.txt differ\n",
			wantExit: 1,
		},
		{
			name: "identical files report nothing",
			fsys: testFS(),
			args: []string{"-q", "same1.txt", "same2.txt"},
		},
		{
			name:     "binary files report like the others",
			fsys:     testFS(),
			args:     []string{"-q", "bin1", "bin2"},
			want:     "Files bin1 and bin2 differ\n",
			wantExit: 1,
		},
		{
			name:     "the long form reports the same",
			fsys:     testFS(),
			args:     []string{"--brief", "a.txt", "c.txt"},
			want:     "Files a.txt and c.txt differ\n",
			wantExit: 1,
		},
		{
			name:     "brief wins over an output format",
			fsys:     testFS(),
			args:     []string{"-q", "-u", "a.txt", "b.txt"},
			want:     "Files a.txt and b.txt differ\n",
			wantExit: 1,
		},
		{
			name:     "changes ignored by -b make files equal",
			fsys:     testFS(),
			args:     []string{"-q", "-b", "blank1.txt", "blank2.txt"},
			wantExit: 0,
		},
		{
			name:     "directories report their files without headers",
			fsys:     testFS(),
			args:     []string{"-r", "-q", "d1", "d2"},
			want:     "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\nFiles d1/sub/f and d2/sub/f differ\nFiles d1/x and d2/x differ\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunReportIdentical(t *testing.T) {
	tests := []testCase{
		{
			name: "identical files are reported",
			fsys: testFS(),
			args: []string{"-s", "same1.txt", "same2.txt"},
			want: "Files same1.txt and same2.txt are identical\n",
		},
		{
			name: "the long form reports the same",
			fsys: testFS(),
			args: []string{"--report-identical-files", "a.txt", "a.txt"},
			want: "Files a.txt and a.txt are identical\n",
		},
		{
			name:     "differing files keep their output",
			fsys:     testFS(),
			args:     []string{"-s", "a.txt", "b.txt"},
			want:     "2c2\n< 2\n---\n> x\n",
			wantExit: 1,
		},
		{
			name: "identical binary files are reported",
			fsys: testFS(),
			args: []string{"-s", "binsame1", "binsame2"},
			want: "Files binsame1 and binsame2 are identical\n",
		},
		{
			name: "changes ignored by -b are reported identical",
			fsys: testFS(),
			args: []string{"-s", "-b", "blank1.txt", "blank2.txt"},
			want: "Files blank1.txt and blank2.txt are identical\n",
		},
		{
			name:     "identical files of a directory comparison are reported",
			fsys:     gnuFS(),
			args:     []string{"-r", "-s", "n1", "n1"},
			want:     "Files n1/only and n1/only are identical\nFiles n1/sub/f and n1/sub/f are identical\nFiles n1/x and n1/x are identical\n",
			wantExit: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunIgnoreOptions(t *testing.T) {
	tests := []testCase{
		{
			name:     "case is significant without -i",
			fsys:     gnuFS(),
			args:     []string{"case1.txt", "case2.txt"},
			want:     "1,2c1,2\n< A\n< b\n---\n> a\n> B\n",
			wantExit: 1,
		},
		{
			name: "case is ignored with -i",
			fsys: gnuFS(),
			args: []string{"-i", "case1.txt", "case2.txt"},
		},
		{
			name: "case is ignored with --ignore-case",
			fsys: gnuFS(),
			args: []string{"--ignore-case", "case1.txt", "case2.txt"},
		},
		{
			name:     "a missing blank differs with -b",
			fsys:     gnuFS(),
			args:     []string{"-b", "ws1.txt", "ws2.txt"},
			want:     "1c1\n< a b\n---\n> ab\n",
			wantExit: 1,
		},
		{
			name: "all blanks are ignored with -w",
			fsys: gnuFS(),
			args: []string{"-w", "ws1.txt", "ws2.txt"},
		},
		{
			name: "all blanks are ignored with --ignore-all-space",
			fsys: gnuFS(),
			args: []string{"--ignore-all-space", "ws1.txt", "ws2.txt"},
		},
		{
			name: "tab expansion is ignored with -E",
			fsys: gnuFS(),
			args: []string{"-E", "tab1.txt", "tab2.txt"},
		},
		{
			name: "tab expansion is ignored with --ignore-tab-expansion",
			fsys: gnuFS(),
			args: []string{"--ignore-tab-expansion", "tab1.txt", "tab2.txt"},
		},
		{
			name:     "the amount of spaces differs with -E alone",
			fsys:     gnuFS(),
			args:     []string{"-E", "ws1.txt", "e2.txt"},
			want:     "1c1,2\n< a b\n---\n> a\n> b\n",
			wantExit: 1,
		},
		{
			name: "trailing space is ignored with -Z",
			fsys: gnuFS(),
			args: []string{"-Z", "trail1.txt", "trail2.txt"},
		},
		{
			name: "trailing space is ignored with --ignore-trailing-space",
			fsys: gnuFS(),
			args: []string{"--ignore-trailing-space", "trail1.txt", "trail2.txt"},
		},
		{
			name: "blank lines are ignored with -B",
			fsys: gnuFS(),
			args: []string{"-B", "blank1", "blank2"},
		},
		{
			name: "blank lines are ignored with --ignore-blank-lines",
			fsys: gnuFS(),
			args: []string{"--ignore-blank-lines", "blank1", "blank2"},
		},
		{
			name:     "changes with lines which aren't blank still come out",
			fsys:     fsx.MemFS{"b1": {Data: []byte("a\n\nb\n")}, "b2": {Data: []byte("a\nc\n")}},
			args:     []string{"-B", "b1", "b2"},
			want:     "2,3c2\n< \n< b\n---\n> c\n",
			wantExit: 1,
		},
		{
			name: "lines matching the -I pattern are ignored",
			fsys: gnuFS(),
			args: []string{"-I", "^foo$", "re1.txt", "re2.txt"},
		},
		{
			name: "lines matching the --ignore-matching-lines pattern are ignored",
			fsys: gnuFS(),
			args: []string{"--ignore-matching-lines=^foo$", "re1.txt", "re2.txt"},
		},
		{
			name:     "lines matching nothing keep differing",
			fsys:     gnuFS(),
			args:     []string{"-I", "^bar$", "re1.txt", "re2.txt"},
			want:     "2d1\n< foo\n",
			wantExit: 1,
		},
		{
			name: "patterns match anywhere in the line",
			fsys: gnuFS(),
			args: []string{"-I", "oo", "re1.txt", "re2.txt"},
		},
		{
			name:     "several patterns all ignore their lines",
			fsys:     gnuFS(),
			args:     []string{"-I", "^bar$", "-I", "^foo$", "re1.txt", "re2.txt"},
			wantExit: 0,
		},
		{
			name: "case ignored with the blanks too",
			fsys: fsx.MemFS{
				"c1": {Data: []byte("A\tb\n")},
				"c2": {Data: []byte("a    B\n")},
			},
			args: []string{"-i", "-b", "c1", "c2"},
		},
		{
			name:     "invalid pattern",
			fsys:     gnuFS(),
			args:     []string{"-I", "*", "re1.txt", "re2.txt"},
			wantErr:  "diff: error parsing regexp: missing argument to repetition operator: `*`\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunText(t *testing.T) {
	tests := []testCase{
		{
			name:     "binary files compared as text",
			fsys:     testFS(),
			args:     []string{"-a", "bin1", "bin2"},
			want:     "1c1\n< \x00a\n\\ No newline at end of file\n---\n> \x00b\n\\ No newline at end of file\n",
			wantExit: 1,
		},
		{
			name:     "binary files compared as text with --text",
			fsys:     testFS(),
			args:     []string{"--text", "bin1", "bin2"},
			want:     "1c1\n< \x00a\n\\ No newline at end of file\n---\n> \x00b\n\\ No newline at end of file\n",
			wantExit: 1,
		},
		{
			name: "identical binary files compared as text",
			fsys: testFS(),
			args: []string{"-a", "binsame1", "binsame2"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunStripCR(t *testing.T) {
	tests := []testCase{
		{
			name: "lines ending with a carriage return compare equal",
			fsys: gnuFS(),
			args: []string{"--strip-trailing-cr", "crlf1.txt", "crlf2.txt"},
		},
		{
			name:     "the carriage returns come out without the option",
			fsys:     gnuFS(),
			args:     []string{"crlf1.txt", "crlf2.txt"},
			want:     "1,2c1,2\n< a\r\n< b\r\n---\n> a\n> b\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunObsoleteContext(t *testing.T) {
	tests := []testCase{
		{
			name:     "the count alone is the context of the copied format",
			fsys:     formatFS(),
			args:     []string{"-1", "-c", "m3.txt", "m4.txt"},
			want:     "*** m3.txt\tFri Jan  2 03:04:05 2026\n--- m4.txt\tFri Jan  2 03:04:05 2026\n***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! X\n  3\n",
			wantExit: 1,
		},
		{
			name:     "the count alone lowers the default of the unified format",
			fsys:     formatFS(),
			args:     []string{"-1", "-u", "m3.txt", "m4.txt"},
			want:     "--- m3.txt\t2026-01-02 03:04:05 +0000\n+++ m4.txt\t2026-01-02 03:04:05 +0000\n@@ -1,3 +1,3 @@\n 1\n-2\n+X\n 3\n",
			wantExit: 1,
		},
		{
			name:     "two digits make one count",
			fsys:     formatFS(),
			args:     []string{"-12", "-c", "m3.txt", "m4.txt"},
			want:     "*** m3.txt\tFri Jan  2 03:04:05 2026\n--- m4.txt\tFri Jan  2 03:04:05 2026\n***************\n*** 1,5 ****\n  1\n! 2\n  3\n  4\n  5\n--- 1,5 ----\n  1\n! X\n  3\n  4\n  5\n",
			wantExit: 1,
		},
		{
			name:     "the count alone compares nothing of the context in the normal format",
			fsys:     formatFS(),
			args:     []string{"-5", "m3.txt", "m4.txt"},
			want:     "2c2\n< 2\n---\n> X\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRCS(t *testing.T) {
	tests := []testCase{
		{
			name:     "changed line",
			fsys:     formatFS(),
			args:     []string{"-n", "m1.txt", "m2.txt"},
			want:     "d2 1\na2 1\nx\n",
			wantExit: 1,
		},
		{
			name:     "changed line with --rcs",
			fsys:     formatFS(),
			args:     []string{"--rcs", "m1.txt", "m2.txt"},
			want:     "d2 1\na2 1\nx\n",
			wantExit: 1,
		},
		{
			name:     "added line",
			fsys:     formatFS(),
			args:     []string{"-n", "m2.txt", "m1.txt"},
			want:     "d2 1\na2 1\n2\n",
			wantExit: 1,
		},
		{
			name:     "lines under directories keep the header",
			fsys:     testFS(),
			args:     []string{"-r", "-n", "d1", "d2"},
			want:     "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\ndiff -r -n d1/sub/f d2/sub/f\nd1 1\na1 1\nw\ndiff -r -n d1/x d2/x\nd1 1\na1 1\nb\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunNewFile(t *testing.T) {
	tests := []testCase{
		{
			name:     "a file missing from the first operand",
			fsys:     testFS(),
			args:     []string{"-N", "nope", "b.txt"},
			want:     "0a1,3\n> 1\n> x\n> 3\n",
			wantExit: 1,
		},
		{
			name:     "a file missing from the second operand",
			fsys:     testFS(),
			args:     []string{"-N", "b.txt", "nope"},
			want:     "1,3d0\n< 1\n< x\n< 3\n",
			wantExit: 1,
		},
		{
			name:     "a missing file without the option is an error",
			fsys:     testFS(),
			args:     []string{"b.txt", "nope"},
			wantErr:  "diff: nope: file does not exist\n",
			wantExit: 2,
		},
		{
			name:     "a missing file against a missing file",
			fsys:     testFS(),
			args:     []string{"-N", "nope", "nope2"},
			wantErr:  "diff: nope: file does not exist\n",
			wantExit: 2,
		},
		{
			name:     "the long form compares the same",
			fsys:     testFS(),
			args:     []string{"--new-file", "nope", "b.txt"},
			want:     "0a1,3\n> 1\n> x\n> 3\n",
			wantExit: 1,
		},
		{
			name: "missing files under directories",
			fsys: gnuFS(),
			args: []string{"-r", "-N", "n1", "n2"},
			want: "diff -r -N n1/new n2/new\n0a1\n> n\n" +
				"diff -r -N n1/only n2/only\n1d0\n< z\n" +
				"diff -r -N n1/sub/f n2/sub/f\n1d0\n< q\n" +
				"diff -r -N n1/x n2/x\n1c1\n< a\n---\n> b\n",
			wantExit: 1,
		},
		{
			name: "missing subdirectories under directories",
			fsys: fsx.MemFS{
				"n1/sdir/f": {Data: []byte("k\n")},
				"n2/x":      {Data: []byte("b\n")},
			},
			args: []string{"-r", "-N", "n1/sdir", "n2"},
			want: "diff -r -N n1/sdir/f n2/f\n1d0\n< k\n" +
				"diff -r -N n1/sdir/x n2/x\n0a1\n> b\n",
			wantExit: 1,
		},
		{
			name: "a missing subdirectory without the recursive option is only reported",
			fsys: gnuFS(),
			args: []string{"-N", "n1", "n2"},
			want: "diff -N n1/new n2/new\n0a1\n> n\n" +
				"diff -N n1/only n2/only\n1d0\n< z\n" +
				"Only in n1: sub\n" +
				"diff -N n1/x n2/x\n1c1\n< a\n---\n> b\n",
			wantExit: 1,
		},
		{
			name: "unidirectional treats absent first files as empty",
			fsys: gnuFS(),
			args: []string{"-r", "-P", "n1", "n2"},
			want: "diff -r -P n1/new n2/new\n0a1\n> n\n" +
				"Only in n1: only\n" +
				"Only in n1: sub\n" +
				"diff -r -P n1/x n2/x\n1c1\n< a\n---\n> b\n",
			wantExit: 1,
		},
		{
			name: "the long form of the unidirectional option",
			fsys: gnuFS(),
			args: []string{"-r", "--unidirectional-new-file", "n1", "n2"},
			want: "diff -r --unidirectional-new-file n1/new n2/new\n0a1\n> n\n" +
				"Only in n1: only\n" +
				"Only in n1: sub\n" +
				"diff -r --unidirectional-new-file n1/x n2/x\n1c1\n< a\n---\n> b\n",
			wantExit: 1,
		},
		{
			name:     "missing files report briefly",
			fsys:     testFS(),
			args:     []string{"-q", "-N", "nope", "b.txt"},
			want:     "Files nope and b.txt differ\n",
			wantExit: 1,
		},
		{
			name:     "the standard input counts as present",
			stdin:    strings.NewReader("1\nx\n3\n"),
			fsys:     testFS(),
			args:     []string{"-N", "-", "nope"},
			want:     "1,3d0\n< 1\n< x\n< 3\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLabels(t *testing.T) {
	tests := []testCase{
		{
			name:     "labels in the unified headers",
			fsys:     formatFS(),
			args:     []string{"-L", "one", "-L", "two", "-u", "m1.txt", "m2.txt"},
			want:     "--- one\n+++ two\n@@ -1,3 +1,3 @@\n 1\n-2\n+x\n 3\n",
			wantExit: 1,
		},
		{
			name:     "labels in the context headers",
			fsys:     formatFS(),
			args:     []string{"-L", "one", "-c", "m1.txt", "m2.txt"},
			want:     "*** one\n--- m2.txt\tFri Jan  2 03:04:05 2026\n***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! x\n  3\n",
			wantExit: 1,
		},
		{
			name:     "labels in the brief message",
			fsys:     formatFS(),
			args:     []string{"-q", "-L", "one", "-L", "two", "m1.txt", "m2.txt"},
			want:     "Files one and two differ\n",
			wantExit: 1,
		},
		{
			name:     "labels in the identical message",
			fsys:     testFS(),
			args:     []string{"-s", "-L", "one", "-L", "two", "same1.txt", "same2.txt"},
			want:     "Files one and two are identical\n",
			wantExit: 0,
		},
		{
			name:     "the long form",
			fsys:     formatFS(),
			args:     []string{"--label", "one", "--label", "two", "-u", "m1.txt", "m2.txt"},
			want:     "--- one\n+++ two\n@@ -1,3 +1,3 @@\n 1\n-2\n+x\n 3\n",
			wantExit: 1,
		},
		{
			name:     "too many labels",
			fsys:     formatFS(),
			args:     []string{"-L", "one", "-L", "two", "-L", "three", "m1.txt", "m2.txt"},
			wantErr:  "diff: too many file label options\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunIfdef(t *testing.T) {
	tests := []testCase{
		{
			name:     "merged output",
			fsys:     testFS(),
			args:     []string{"-D", "NAME", "a.txt", "b.txt"},
			want:     "1\n#ifndef NAME\n2\n#else /* NAME */\nx\n#endif /* NAME */\n3\n",
			wantExit: 1,
		},
		{
			name:     "merged output with the long form",
			fsys:     testFS(),
			args:     []string{"--ifdef=NAME", "a.txt", "b.txt"},
			want:     "1\n#ifndef NAME\n2\n#else /* NAME */\nx\n#endif /* NAME */\n3\n",
			wantExit: 1,
		},
		{
			name:     "conflicting output style",
			fsys:     testFS(),
			args:     []string{"-D", "NAME", "-u", "a.txt", "b.txt"},
			wantErr:  "diff: conflicting output style options\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "directories take no merged output",
			fsys:     testFS(),
			args:     []string{"-r", "-D", "NAME", "d1", "d2"},
			wantErr:  "diff: -D option not supported with directories\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFromToFiles(t *testing.T) {
	tests := []testCase{
		{
			name:     "a from-file compares every operand against it",
			fsys:     gnuFS(),
			args:     []string{"--from-file=e1.txt", "e2.txt", "e3.txt"},
			want:     "1a2\n> b\n1c1\n< a\n---\n> b\n",
			wantExit: 1,
		},
		{
			name:     "a to-file compares every operand to it",
			fsys:     gnuFS(),
			args:     []string{"--to-file=e3.txt", "e1.txt", "e2.txt"},
			want:     "1c1\n< a\n---\n> b\n1d0\n< a\n",
			wantExit: 1,
		},
		{
			name:     "an identical comparison writes nothing",
			fsys:     gnuFS(),
			args:     []string{"--from-file=e1.txt", "e1.txt"},
			wantExit: 0,
		},
		{
			name:     "both options together",
			fsys:     gnuFS(),
			args:     []string{"--from-file=e1.txt", "--to-file=e2.txt", "e3.txt"},
			wantErr:  "diff: --from-file and --to-file both specified\n",
			wantExit: 2,
		},
		{
			name:     "no operands compares nothing",
			fsys:     gnuFS(),
			args:     []string{"--from-file=e1.txt"},
			wantExit: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunTabs(t *testing.T) {
	tabs := fsx.MemFS{
		"t1": {Data: []byte("a\tb\n")},
		"t2": {Data: []byte("a b\n")},
	}
	tests := []testCase{
		{
			name:     "tabs stay as they are without the option",
			fsys:     tabs,
			args:     []string{"t1", "t2"},
			want:     "1c1\n< a\tb\n---\n> a b\n",
			wantExit: 1,
		},
		{
			name:     "tabs expand to spaces",
			fsys:     tabs,
			args:     []string{"-t", "t1", "t2"},
			want:     "1c1\n< a       b\n---\n> a b\n",
			wantExit: 1,
		},
		{
			name:     "tabs expand to spaces with the long form",
			fsys:     tabs,
			args:     []string{"--expand-tabs", "t1", "t2"},
			want:     "1c1\n< a       b\n---\n> a b\n",
			wantExit: 1,
		},
		{
			name:     "tabs expand to the tab size given",
			fsys:     tabs,
			args:     []string{"--tabsize=4", "-t", "t1", "t2"},
			want:     "1c1\n< a   b\n---\n> a b\n",
			wantExit: 1,
		},
		{
			name:     "lines get an initial tab",
			fsys:     testFS(),
			args:     []string{"-T", "a.txt", "b.txt"},
			want:     "2c2\n<\t2\n---\n>\tx\n",
			wantExit: 1,
		},
		{
			name:     "lines get an initial tab in the unified format",
			fsys:     testFS(),
			args:     []string{"--initial-tab", "-u", "a.txt", "b.txt"},
			want:     "--- a.txt\t0001-01-01 00:00:00 +0000\n+++ b.txt\t0001-01-01 00:00:00 +0000\n@@ -1,3 +1,3 @@\n\t1\n-\t2\n+\tx\n\t3\n",
			wantExit: 1,
		},
		{
			name:     "lines get an initial tab in the context format",
			fsys:     testFS(),
			args:     []string{"-T", "-c", "a.txt", "b.txt"},
			want:     "*** a.txt\tMon Jan  1 00:00:00 0001\n--- b.txt\tMon Jan  1 00:00:00 0001\n***************\n*** 1,3 ****\n \t1\n!\t2\n \t3\n--- 1,3 ----\n \t1\n!\tx\n \t3\n",
			wantExit: 1,
		},
		{
			name:     "invalid tab size",
			fsys:     tabs,
			args:     []string{"--tabsize=x", "-t", "t1", "t2"},
			wantErr:  "diff: invalid tabsize 'x'\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "zero tab size",
			fsys:     tabs,
			args:     []string{"--tabsize=0", "-t", "t1", "t2"},
			wantErr:  "diff: invalid tabsize '0'\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "conflicting tab sizes",
			fsys:     tabs,
			args:     []string{"--tabsize=4", "--tabsize=8", "t1", "t2"},
			wantErr:  "diff: conflicting tabsize options\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSideBySide(t *testing.T) {
	tests := []testCase{
		{
			name:     "two columns",
			fsys:     testFS(),
			args:     []string{"-y", "-W", "20", "a.txt", "b.txt"},
			want:     "1\t1\n2     |\tx\n3\t3\n",
			wantExit: 1,
		},
		{
			name:     "two columns with the long form",
			fsys:     testFS(),
			args:     []string{"--side-by-side", "-W", "20", "a.txt", "b.txt"},
			want:     "1\t1\n2     |\tx\n3\t3\n",
			wantExit: 1,
		},
		{
			name:     "deleted line keeps the left column alone",
			fsys:     testFS(),
			args:     []string{"-y", "-W", "20", "a.txt", "c.txt"},
			want:     "1\t1\n2\t2\n3     <\n",
			wantExit: 1,
		},
		{
			name:     "added line keeps the right column alone",
			fsys:     testFS(),
			args:     []string{"-y", "-W", "20", "c.txt", "a.txt"},
			want:     "1\t1\n2\t2\n      >\t3\n",
			wantExit: 1,
		},
		{
			name:     "identical files still come out in both columns",
			fsys:     testFS(),
			args:     []string{"-y", "-W", "20", "same1.txt", "same2.txt"},
			want:     "a\ta\nb\tb\n",
			wantExit: 0,
		},
		{
			name:     "the left column only",
			fsys:     testFS(),
			args:     []string{"-y", "-W", "20", "--left-column", "a.txt", "b.txt"},
			want:     "1     (\n2     |\tx\n3     (\n",
			wantExit: 1,
		},
		{
			name:     "suppressed common lines",
			fsys:     testFS(),
			args:     []string{"-y", "-W", "20", "--suppress-common-lines", "a.txt", "b.txt"},
			want:     "2     |\tx\n",
			wantExit: 1,
		},
		{
			name:     "a width of its own with the long form",
			fsys:     testFS(),
			args:     []string{"-y", "--width=20", "a.txt", "b.txt"},
			want:     "1\t1\n2     |\tx\n3\t3\n",
			wantExit: 1,
		},
		{
			name:     "invalid width",
			fsys:     testFS(),
			args:     []string{"-y", "-W", "0", "a.txt", "b.txt"},
			wantErr:  "diff: invalid width '0'\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "conflicting widths",
			fsys:     testFS(),
			args:     []string{"-y", "-W", "20", "-W", "30", "a.txt", "b.txt"},
			wantErr:  "diff: conflicting width options\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "conflicting output style",
			fsys:     testFS(),
			args:     []string{"-y", "-u", "a.txt", "b.txt"},
			wantErr:  "diff: conflicting output style options\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunExcludes(t *testing.T) {
	excludeFS := fsx.MemFS{
		"n1/a":     {Data: []byte("1\n")},
		"n2/a":     {Data: []byte("2\n")},
		"n1/b.log": {Data: []byte("x\n")},
		"n2/b.log": {Data: []byte("y\n")},
		"pat":      {Data: []byte("*.log\n")},
	}
	tests := []testCase{
		{
			name:     "excluded names are not compared",
			fsys:     excludeFS,
			args:     []string{"-r", "-x", "*.log", "n1", "n2"},
			want:     "diff -r -x '*.log' n1/a n2/a\n1c1\n< 1\n---\n> 2\n",
			wantExit: 1,
		},
		{
			name: "names which don't match are compared",
			fsys: excludeFS,
			args: []string{"-r", "-x", "*.txt", "n1", "n2"},
			want: "diff -r -x '*.txt' n1/a n2/a\n1c1\n< 1\n---\n> 2\n" +
				"diff -r -x '*.txt' n1/b.log n2/b.log\n1c1\n< x\n---\n> y\n",
			wantExit: 1,
		},
		{
			name:     "patterns come from a file",
			fsys:     excludeFS,
			args:     []string{"-r", "-X", "pat", "n1", "n2"},
			want:     "diff -r -X pat n1/a n2/a\n1c1\n< 1\n---\n> 2\n",
			wantExit: 1,
		},
		{
			name:     "several patterns exclude together",
			fsys:     excludeFS,
			args:     []string{"-r", "-x", "a", "-x", "*.log", "n1", "n2"},
			wantExit: 0,
		},
		{
			name:     "a missing pattern file",
			fsys:     excludeFS,
			args:     []string{"-r", "-X", "nope", "n1", "n2"},
			wantErr:  "diff: nope: file does not exist\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLongAliases(t *testing.T) {
	tests := []testCase{
		{
			name:     "--recursive",
			fsys:     testFS(),
			args:     []string{"--recursive", "d1", "d2"},
			want:     "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\ndiff --recursive d1/sub/f d2/sub/f\n1c1\n< q\n---\n> w\ndiff --recursive d1/x d2/x\n1c1\n< a\n---\n> b\n",
			wantExit: 1,
		},
		{
			name:     "--ed",
			fsys:     formatFS(),
			args:     []string{"--ed", "m1.txt", "m2.txt"},
			want:     "2c\nx\n.\n",
			wantExit: 1,
		},
		{
			name:     "--forward-ed",
			fsys:     formatFS(),
			args:     []string{"--forward-ed", "m1.txt", "m2.txt"},
			want:     "c2\nx\n.\n",
			wantExit: 1,
		},
		{
			name:     "--context with a count",
			fsys:     formatFS(),
			args:     []string{"--context=1", "m3.txt", "m4.txt"},
			want:     "*** m3.txt\tFri Jan  2 03:04:05 2026\n--- m4.txt\tFri Jan  2 03:04:05 2026\n***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! X\n  3\n",
			wantExit: 1,
		},
		{
			name:     "--context alone gives three lines",
			fsys:     formatFS(),
			args:     []string{"--context", "m1.txt", "m2.txt"},
			want:     "*** m1.txt\tFri Jan  2 03:04:05 2026\n--- m2.txt\tFri Jan  2 03:04:05 2026\n***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! x\n  3\n",
			wantExit: 1,
		},
		{
			name:     "--unified with a count",
			fsys:     formatFS(),
			args:     []string{"--unified=0", "m3.txt", "m4.txt"},
			want:     "--- m3.txt\t2026-01-02 03:04:05 +0000\n+++ m4.txt\t2026-01-02 03:04:05 +0000\n@@ -2 +2 @@\n-2\n+X\n",
			wantExit: 1,
		},
		{
			name:     "--unified alone gives three lines",
			fsys:     formatFS(),
			args:     []string{"--unified", "m1.txt", "m2.txt"},
			want:     "--- m1.txt\t2026-01-02 03:04:05 +0000\n+++ m2.txt\t2026-01-02 03:04:05 +0000\n@@ -1,3 +1,3 @@\n 1\n-2\n+x\n 3\n",
			wantExit: 1,
		},
		{
			name:     "--normal",
			fsys:     formatFS(),
			args:     []string{"--normal", "m1.txt", "m2.txt"},
			want:     "2c2\n< 2\n---\n> x\n",
			wantExit: 1,
		},
		{
			name:     "--ignore-space-change",
			fsys:     testFS(),
			args:     []string{"--ignore-space-change", "blank1.txt", "blank2.txt"},
			wantExit: 0,
		},
		{
			name:     "--binary is accepted",
			fsys:     formatFS(),
			args:     []string{"--binary", "m1.txt", "m2.txt"},
			want:     "2c2\n< 2\n---\n> x\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunInvalidUsage(t *testing.T) {
	tests := []testCase{
		{
			name:     "unrecognized long option",
			fsys:     testFS(),
			args:     []string{"--nope", "a.txt", "b.txt"},
			wantErr:  "diff: unrecognized option '--nope'\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "missing option argument",
			fsys:     testFS(),
			args:     []string{"-x"},
			wantErr:  "diff: option requires an argument -- 'x'\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "argument of an option which takes none",
			fsys:     testFS(),
			args:     []string{"--brief=x", "a.txt", "b.txt"},
			wantErr:  "diff: option '--brief=x' doesn't allow an argument\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "missing label argument",
			fsys:     testFS(),
			args:     []string{"-L"},
			wantErr:  "diff: option requires an argument -- 'L'\nTry 'diff --help' for more information.\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunVersionOption(t *testing.T) {
	runCase(t, testCase{
		name: "-v",
		args: []string{"-v"},
		want: "diff (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n",
	})
}

func TestRunBriefWithIdentical(t *testing.T) {
	runCase(t, testCase{
		name:     "brief and identical together",
		fsys:     testFS(),
		args:     []string{"-q", "-s", "same1.txt", "same2.txt"},
		want:     "Files same1.txt and same2.txt are identical\n",
		wantExit: 0,
	})
}

func TestRunFinalNewline(t *testing.T) {
	// A file whose last line ends without a newline stays different from
	// one whose matching line ends with one, even when the contents are
	// compared loosely: diff doesn't silently complete lines. The blank
	// ignoring comparisons (-Z, -b, -w) are the exception, matching GNU,
	// which treats the missing newline as one more trailing blank there.
	tests := []testCase{
		{
			name:     "a missing final newline stays a difference with -i",
			fsys:     fsx.MemFS{"c1": {Data: []byte("a\nB\n")}, "c2": {Data: []byte("A\nb")}},
			args:     []string{"-i", "c1", "c2"},
			want:     "2c2\n< B\n---\n> b\n\\ No newline at end of file\n",
			wantExit: 1,
		},
		{
			name:     "a missing final newline stays a difference with -E",
			fsys:     testFS(),
			args:     []string{"-E", "c.txt", "nonl1"},
			want:     "2c2\n< 2\n---\n> 2\n\\ No newline at end of file\n",
			wantExit: 1,
		},
		{
			name:     "a missing final newline on both sides compares equal",
			fsys:     testFS(),
			args:     []string{"-i", "nonl1", "nonl2"},
			want:     "2c2\n< 2\n\\ No newline at end of file\n---\n> x\n\\ No newline at end of file\n",
			wantExit: 1,
		},
		{
			name: "a trailing blank comparison completes the final line",
			fsys: testFS(),
			args: []string{"-b", "c.txt", "nonl1"},
		},
		{
			name: "all space comparison completes the final line",
			fsys: testFS(),
			args: []string{"-w", "c.txt", "nonl1"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}
