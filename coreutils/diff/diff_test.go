package diff

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

// testFS holds the fixtures used by the tests:
//
//	a.txt:    "1\n2\n3\n"
//	b.txt:    "1\nx\n3\n" (a.txt with line 2 changed)
//	c.txt:    "1\n2\n"    (a.txt with line 3 deleted)
//	d.txt:    "1\n2\n3\n4\n" (a.txt with a line added)
//	nonl1:    "1\n2"      (a.txt without the trailing newline)
//	nonl2:    "1\nx"      (b.txt without the trailing newline)
//	same1.txt, same2.txt: "a\nb\n"
//	empty1.txt, empty2.txt: ""
//	bin1:     {0x00, 'a'}
//	bin2:     {0x00, 'b'}
//	binsame1, binsame2: {0x00, 'a'}
//	blank1.txt: "a b\nc\n"
//	blank2.txt: "a  b\nc\n" (blank1.txt with more blanks)
//	noblank.txt: "ab\nc\n"
//	d1/x:     "a\n"       d2/x: "b\n"
//	d1/only:  "z\n"
//	d1/sub/f: "q\n"       d2/sub/f: "w\n"
//	d1/mix:   "m\n"       d2/mix/f: "n\n" (a file against a directory)
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":       {Data: []byte("1\n2\n3\n")},
		"b.txt":       {Data: []byte("1\nx\n3\n")},
		"c.txt":       {Data: []byte("1\n2\n")},
		"d.txt":       {Data: []byte("1\n2\n3\n4\n")},
		"nonl1":       {Data: []byte("1\n2")},
		"nonl2":       {Data: []byte("1\nx")},
		"same1.txt":   {Data: []byte("a\nb\n")},
		"same2.txt":   {Data: []byte("a\nb\n")},
		"empty1.txt":  {Data: []byte("")},
		"empty2.txt":  {Data: []byte("")},
		"bin1":        {Data: []byte{0x00, 'a'}},
		"bin2":        {Data: []byte{0x00, 'b'}},
		"binsame1":    {Data: []byte{0x00, 'a'}},
		"binsame2":    {Data: []byte{0x00, 'a'}},
		"blank1.txt":  {Data: []byte("a b\nc\n")},
		"blank2.txt":  {Data: []byte("a  b\nc\n")},
		"noblank.txt": {Data: []byte("ab\nc\n")},
		"d1/x":        {Data: []byte("a\n")},
		"d2/x":        {Data: []byte("b\n")},
		"d1/only":     {Data: []byte("z\n")},
		"d1/sub/f":    {Data: []byte("q\n")},
		"d2/sub/f":    {Data: []byte("w\n")},
		"d1/mix":      {Data: []byte("m\n")},
		"d2/mix/f":    {Data: []byte("n\n")},
	}
}

func TestRunDefaults(t *testing.T) {
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
			Name:     "changed line",
			FS:       testFS(),
			Args:     []string{"a.txt", "b.txt"},
			Want:     "2c2\n< 2\n---\n> x\n",
			WantExit: 1,
		},
		{
			Name:     "deleted line",
			FS:       testFS(),
			Args:     []string{"a.txt", "c.txt"},
			Want:     "3d2\n< 3\n",
			WantExit: 1,
		},
		{
			Name:     "added line",
			FS:       testFS(),
			Args:     []string{"a.txt", "d.txt"},
			Want:     "3a4\n> 4\n",
			WantExit: 1,
		},
		{
			Name:     "missing newline at end of file",
			FS:       testFS(),
			Args:     []string{"nonl1", "nonl2"},
			Want:     "2c2\n< 2\n\\ No newline at end of file\n---\n> x\n\\ No newline at end of file\n",
			WantExit: 1,
		},
		{
			Name:     "newline added at end of file",
			FS:       testFS(),
			Args:     []string{"nonl1", "c.txt"},
			Want:     "2c2\n< 2\n\\ No newline at end of file\n---\n> 2\n",
			WantExit: 1,
		},
		{
			Name: "empty files",
			FS:   testFS(),
			Args: []string{"empty1.txt", "empty2.txt"},
		},
		{
			Name:     "empty against non-empty file",
			FS:       testFS(),
			Args:     []string{"empty1.txt", "c.txt"},
			Want:     "0a1,2\n> 1\n> 2\n",
			WantExit: 1,
		},
		{
			Name:     "binary files",
			FS:       testFS(),
			Args:     []string{"bin1", "bin2"},
			Want:     "Binary files bin1 and bin2 differ\n",
			WantExit: 1,
		},
		{
			Name: "identical binary files",
			FS:   testFS(),
			Args: []string{"binsame1", "binsame2"},
		},
		{
			Name:     "text against binary file",
			FS:       testFS(),
			Args:     []string{"a.txt", "bin2"},
			Want:     "Binary files a.txt and bin2 differ\n",
			WantExit: 1,
		},
		{
			Name:     "the standard input",
			Stdin:    strings.NewReader("1\n2\n4\n"),
			FS:       testFS(),
			Args:     []string{"-", "a.txt"},
			Want:     "3c3\n< 4\n---\n> 3\n",
			WantExit: 1,
		},
		{
			Name:     "missing operand",
			FS:       testFS(),
			Args:     []string{"a.txt"},
			WantErr:  "diff: missing operand\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "extra operand",
			FS:       testFS(),
			Args:     []string{"a.txt", "b.txt", "c.txt"},
			WantErr:  "diff: extra operand 'c.txt'\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option",
			FS:       testFS(),
			Args:     []string{"-j", "a.txt", "b.txt"},
			WantErr:  "diff: invalid option -- 'j'\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing file",
			FS:       testFS(),
			Args:     []string{"a.txt", "nope.txt"},
			WantErr:  "diff: nope.txt: file does not exist\n",
			WantExit: 2,
		},
		{
			Name:     "missing file of a directory operand",
			FS:       testFS(),
			Args:     []string{"a.txt", "d1"},
			WantErr:  "diff: d1/a.txt: file does not exist\n",
			WantExit: 2,
		},
		{
			Name:     "no file system",
			Args:     []string{"a.txt", "b.txt"},
			WantErr:  "diff: no file system available\n",
			WantExit: 2,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunBlank(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "files differing in blank amount only",
			FS:   testFS(),
			Args: []string{"-b", "blank1.txt", "blank2.txt"},
		},
		{
			Name:     "missing blanks still differ",
			FS:       testFS(),
			Args:     []string{"-b", "blank1.txt", "noblank.txt"},
			Want:     "1c1\n< a b\n---\n> ab\n",
			WantExit: 1,
		},
		{
			Name:     "changes computed over blanks",
			Stdin:    strings.NewReader("x \ny\n"),
			FS:       testFS(),
			Args:     []string{"-b", "-", "b.txt"},
			Want:     "0a1\n> 1\n2c3\n< y\n---\n> 3\n",
			WantExit: 1,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunDirectories(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "common subdirectories only",
			FS: fsx.MemFS{
				"d1/sub/f": {Data: []byte("q\n")},
				"d2/sub/f": {Data: []byte("q\n")},
			},
			Args: []string{"d1", "d2"},
			Want: "Common subdirectories: d1/sub and d2/sub\n",
		},
		{
			Name: "identical directories",
			FS: fsx.MemFS{
				"d1/x": {Data: []byte("a\n")},
				"d2/x": {Data: []byte("a\n")},
			},
			Args: []string{"d1", "d2"},
		},
		{
			Name:     "file only in the first directory",
			FS:       testFS(),
			Args:     []string{"d1", "d2"},
			Want:     "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\nCommon subdirectories: d1/sub and d2/sub\ndiff d1/x d2/x\n1c1\n< a\n---\n> b\n",
			WantExit: 1,
		},
		{
			Name:     "file operand of a directory",
			FS:       testFS(),
			Args:     []string{"d1/x", "d2"},
			Want:     "1c1\n< a\n---\n> b\n",
			WantExit: 1,
		},
		{
			Name:     "directory operand of a file",
			FS:       testFS(),
			Args:     []string{"d2", "d1/x"},
			Want:     "1c1\n< b\n---\n> a\n",
			WantExit: 1,
		},
		{
			Name:     "options in the header",
			FS:       testFS(),
			Args:     []string{"-b", "d1", "d2"},
			Want:     "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\nCommon subdirectories: d1/sub and d2/sub\ndiff -b d1/x d2/x\n1c1\n< a\n---\n> b\n",
			WantExit: 1,
		},
		{
			Name:     "a directory compares identical to itself",
			FS:       testFS(),
			Args:     []string{"d1", "d1"},
			Want:     "Common subdirectories: d1/sub and d1/sub\n",
			WantExit: 0,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunWriteError(t *testing.T) {
	rt := &coreutils.Runtime{
		Stdout: testutil.ErrorWriter{Err: errors.New("write failed")},
		Stderr: new(bytes.Buffer),
		FS:     testFS(),
	}
	err := New(rt).Run(t.Context(), "a.txt", "b.txt")
	if code := testutil.ExitCode(t, err); code != 2 {
		t.Errorf("Run() exit code = %d, want 2", code)
	}
	if got := rt.Stderr.(*bytes.Buffer).String(); got != "diff: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "diff: write error: write failed\n")
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

// stamp is the fixed modification time of the fixtures of the format tests.
var stamp = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// formatFS holds the fixtures used by the format tests:
//
//	m1.txt, m2.txt: a three line file and one with its middle line changed
//	m3.txt, m4.txt: a five line file and one with its second line changed
func formatFS() fsx.MemFS {
	return fsx.MemFS{
		"m1.txt": {Data: []byte("1\n2\n3\n"), ModTime: stamp},
		"m2.txt": {Data: []byte("1\nx\n3\n"), ModTime: stamp},
		"m3.txt": {Data: []byte("1\n2\n3\n4\n5\n"), ModTime: stamp},
		"m4.txt": {Data: []byte("1\nX\n3\n4\n5\n"), ModTime: stamp},
	}
}

func TestRunFormats(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "context format",
			FS:   formatFS(),
			Args: []string{"-c", "m1.txt", "m2.txt"},
			Want: "*** m1.txt\tFri Jan  2 03:04:05 2026\n--- m2.txt\tFri Jan  2 03:04:05 2026\n" +
				"***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! x\n  3\n",
			WantExit: 1,
		},
		{
			Name: "context format with a count",
			FS:   formatFS(),
			Args: []string{"-C", "1", "m3.txt", "m4.txt"},
			Want: "*** m3.txt\tFri Jan  2 03:04:05 2026\n--- m4.txt\tFri Jan  2 03:04:05 2026\n" +
				"***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! X\n  3\n",
			WantExit: 1,
		},
		{
			Name: "context format with an attached count",
			FS:   formatFS(),
			Args: []string{"-C2", "m3.txt", "m4.txt"},
			Want: "*** m3.txt\tFri Jan  2 03:04:05 2026\n--- m4.txt\tFri Jan  2 03:04:05 2026\n" +
				"***************\n*** 1,4 ****\n  1\n! 2\n  3\n  4\n--- 1,4 ----\n  1\n! X\n  3\n  4\n",
			WantExit: 1,
		},
		{
			Name: "unified format",
			FS:   formatFS(),
			Args: []string{"-u", "m1.txt", "m2.txt"},
			Want: "--- m1.txt\t2026-01-02 03:04:05 +0000\n+++ m2.txt\t2026-01-02 03:04:05 +0000\n" +
				"@@ -1,3 +1,3 @@\n 1\n-2\n+x\n 3\n",
			WantExit: 1,
		},
		{
			Name: "unified format with a count",
			FS:   formatFS(),
			Args: []string{"-U", "1", "m3.txt", "m4.txt"},
			Want: "--- m3.txt\t2026-01-02 03:04:05 +0000\n+++ m4.txt\t2026-01-02 03:04:05 +0000\n" +
				"@@ -1,3 +1,3 @@\n 1\n-2\n+X\n 3\n",
			WantExit: 1,
		},
		{
			Name: "unified format without context",
			FS:   formatFS(),
			Args: []string{"-U0", "m3.txt", "m4.txt"},
			Want: "--- m3.txt\t2026-01-02 03:04:05 +0000\n+++ m4.txt\t2026-01-02 03:04:05 +0000\n" +
				"@@ -2 +2 @@\n-2\n+X\n",
			WantExit: 1,
		},
		{
			Name:     "ed format",
			FS:       formatFS(),
			Args:     []string{"-e", "m1.txt", "m2.txt"},
			Want:     "2c\nx\n.\n",
			WantExit: 1,
		},
		{
			Name:     "forward ed format",
			FS:       formatFS(),
			Args:     []string{"-f", "m1.txt", "m2.txt"},
			Want:     "c2\nx\n.\n",
			WantExit: 1,
		},
		{
			Name: "identical files in the unified format",
			FS:   formatFS(),
			Args: []string{"-u", "m1.txt", "m1.txt"},
		},
		{
			Name:     "binary files in the unified format",
			FS:       testFS(),
			Args:     []string{"-u", "bin1", "bin2"},
			Want:     "Binary files bin1 and bin2 differ\n",
			WantExit: 1,
		},
		{
			Name: "context lines keep the first file contents with -b",
			FS:   fsx.MemFS{"w1": {Data: []byte("1\n2 \n3\n"), ModTime: stamp}, "w2": {Data: []byte("1\n2\n4\n"), ModTime: stamp}},
			Args: []string{"-b", "-c", "w1", "w2"},
			Want: "*** w1\tFri Jan  2 03:04:05 2026\n--- w2\tFri Jan  2 03:04:05 2026\n" +
				"***************\n*** 1,3 ****\n  1\n  2 \n! 3\n--- 1,3 ----\n  1\n  2\n! 4\n",
			WantExit: 1,
		},
		{
			Name:     "conflicting output styles",
			FS:       formatFS(),
			Args:     []string{"-c", "-u", "m1.txt", "m2.txt"},
			WantErr:  "diff: conflicting output style options\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name: "same output styles combine",
			FS:   formatFS(),
			Args: []string{"-c", "-C", "1", "m3.txt", "m4.txt"},
			Want: "*** m3.txt\tFri Jan  2 03:04:05 2026\n--- m4.txt\tFri Jan  2 03:04:05 2026\n" +
				"***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! X\n  3\n",
			WantExit: 1,
		},
		{
			Name:     "invalid context count",
			FS:       formatFS(),
			Args:     []string{"-C", "0", "m1.txt", "m2.txt"},
			WantErr:  "diff: invalid context length '0'\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid context count argument",
			FS:       formatFS(),
			Args:     []string{"-C", "x", "m1.txt", "m2.txt"},
			WantErr:  "diff: invalid context length 'x'\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "negative unified context count",
			FS:       formatFS(),
			Args:     []string{"-U", "-1", "m1.txt", "m2.txt"},
			WantErr:  "diff: invalid context length '-1'\nTry 'diff --help' for more information.\n",
			WantExit: 2,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunFormatsStandardInput(t *testing.T) {
	// The standard input holds no modification time, so only the hunks are
	// compared exactly, with the file headers checked loosely.
	stdout := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("1\nx\n3\n"),
		Stdout: stdout,
		Stderr: new(bytes.Buffer),
		FS:     formatFS(),
	}
	err := New(rt).Run(t.Context(), "-u", "-", "m1.txt")
	if code := testutil.ExitCode(t, err); code != 1 {
		t.Fatalf("Run() exit code = %d, want 1", code)
	}
	got := stdout.String()
	if !strings.HasPrefix(got, "--- -\t") || !strings.HasPrefix(got[strings.IndexByte(got, '\n')+1:], "+++ m1.txt\t") {
		t.Errorf("Run() headers = %q", got)
	}
	want := "@@ -1,3 +1,3 @@\n 1\n-x\n+2\n 3\n"
	if tail := got[strings.Index(got, "@@"):]; tail != want {
		t.Errorf("Run() hunks = %q, want %q", tail, want)
	}
}

func TestRunRecursive(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "directories compared recursively",
			FS:   testFS(),
			Args: []string{"-r", "d1", "d2"},
			Want: "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\n" +
				"diff -r d1/sub/f d2/sub/f\n1c1\n< q\n---\n> w\n" +
				"diff -r d1/x d2/x\n1c1\n< a\n---\n> b\n",
			WantExit: 1,
		},
		{
			Name: "deeply nested files",
			FS: fsx.MemFS{
				"d3/a/b/c.txt": {Data: []byte("1\n")},
				"d4/a/b/c.txt": {Data: []byte("2\n")},
			},
			Args:     []string{"-r", "d3", "d4"},
			Want:     "diff -r d3/a/b/c.txt d4/a/b/c.txt\n1c1\n< 1\n---\n> 2\n",
			WantExit: 1,
		},
		{
			Name: "recursion options in the header",
			FS: fsx.MemFS{
				"d3/a/b/c.txt": {Data: []byte("1\n2\n3\n4\n5\n"), ModTime: stamp},
				"d4/a/b/c.txt": {Data: []byte("1\nX\n3\n4\n5\n"), ModTime: stamp},
			},
			Args: []string{"-r", "-C", "2", "d3", "d4"},
			Want: "diff -r -C 2 d3/a/b/c.txt d4/a/b/c.txt\n" +
				"*** d3/a/b/c.txt\tFri Jan  2 03:04:05 2026\n--- d4/a/b/c.txt\tFri Jan  2 03:04:05 2026\n" +
				"***************\n*** 1,4 ****\n  1\n! 2\n  3\n  4\n--- 1,4 ----\n  1\n! X\n  3\n  4\n",
			WantExit: 1,
		},
		{
			Name:     "ed format keeps the header under directories",
			FS:       testFS(),
			Args:     []string{"-r", "-e", "d1", "d2"},
			Want:     "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\ndiff -r -e d1/sub/f d2/sub/f\n1c\nw\n.\ndiff -r -e d1/x d2/x\n1c\nb\n.\n",
			WantExit: 1,
		},
		{
			Name: "identical directories compare equal recursively",
			FS: fsx.MemFS{
				"d1/x":   {Data: []byte("a\n")},
				"d2/x":   {Data: []byte("a\n")},
				"d1/s/y": {Data: []byte("b\n")},
				"d2/s/y": {Data: []byte("b\n")},
			},
			Args: []string{"-r", "d1", "d2"},
		},
	}
	testutil.Run(t, New, tests)
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "diff", usage)
}
