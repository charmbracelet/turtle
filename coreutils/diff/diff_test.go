package diff

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// cancelReader cancels its context on the first read, simulating a context
// that is done while input is being read.
type cancelReader struct {
	cancel context.CancelFunc
}

func (r *cancelReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

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
	fsys     fsx.MemFS
	dir      string
	args     []string
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
		WorkingDir: test.dir,
	}
	if test.fsys != nil {
		rt.FS = test.fsys
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
			name: "identical files produce no output",
			fsys: testFS(),
			args: []string{"same1.txt", "same2.txt"},
		},
		{
			name: "a file compares identical to itself",
			fsys: testFS(),
			args: []string{"a.txt", "a.txt"},
		},
		{
			name:     "changed line",
			fsys:     testFS(),
			args:     []string{"a.txt", "b.txt"},
			want:     "2c2\n< 2\n---\n> x\n",
			wantExit: 1,
		},
		{
			name:     "deleted line",
			fsys:     testFS(),
			args:     []string{"a.txt", "c.txt"},
			want:     "3d2\n< 3\n",
			wantExit: 1,
		},
		{
			name:     "added line",
			fsys:     testFS(),
			args:     []string{"a.txt", "d.txt"},
			want:     "3a4\n> 4\n",
			wantExit: 1,
		},
		{
			name:     "missing newline at end of file",
			fsys:     testFS(),
			args:     []string{"nonl1", "nonl2"},
			want:     "2c2\n< 2\n\\ No newline at end of file\n---\n> x\n\\ No newline at end of file\n",
			wantExit: 1,
		},
		{
			name:     "newline added at end of file",
			fsys:     testFS(),
			args:     []string{"nonl1", "c.txt"},
			want:     "2c2\n< 2\n\\ No newline at end of file\n---\n> 2\n",
			wantExit: 1,
		},
		{
			name: "empty files",
			fsys: testFS(),
			args: []string{"empty1.txt", "empty2.txt"},
		},
		{
			name:     "empty against non-empty file",
			fsys:     testFS(),
			args:     []string{"empty1.txt", "c.txt"},
			want:     "0a1,2\n> 1\n> 2\n",
			wantExit: 1,
		},
		{
			name:     "binary files",
			fsys:     testFS(),
			args:     []string{"bin1", "bin2"},
			want:     "Binary files bin1 and bin2 differ\n",
			wantExit: 1,
		},
		{
			name: "identical binary files",
			fsys: testFS(),
			args: []string{"binsame1", "binsame2"},
		},
		{
			name:     "text against binary file",
			fsys:     testFS(),
			args:     []string{"a.txt", "bin2"},
			want:     "Binary files a.txt and bin2 differ\n",
			wantExit: 1,
		},
		{
			name:     "the standard input",
			stdin:    strings.NewReader("1\n2\n4\n"),
			fsys:     testFS(),
			args:     []string{"-", "a.txt"},
			want:     "3c3\n< 4\n---\n> 3\n",
			wantExit: 1,
		},
		{
			name:     "missing operand",
			fsys:     testFS(),
			args:     []string{"a.txt"},
			wantErr:  "diff: missing operand\n",
			wantExit: 2,
		},
		{
			name:     "extra operand",
			fsys:     testFS(),
			args:     []string{"a.txt", "b.txt", "c.txt"},
			wantErr:  "diff: extra operand 'c.txt'\n",
			wantExit: 2,
		},
		{
			name:     "invalid option",
			fsys:     testFS(),
			args:     []string{"-x", "a.txt", "b.txt"},
			wantErr:  "diff: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "missing file",
			fsys:     testFS(),
			args:     []string{"a.txt", "nope.txt"},
			wantErr:  "diff: nope.txt: file does not exist\n",
			wantExit: 2,
		},
		{
			name:     "missing file of a directory operand",
			fsys:     testFS(),
			args:     []string{"a.txt", "d1"},
			wantErr:  "diff: d1/a.txt: file does not exist\n",
			wantExit: 2,
		},
		{
			name:     "no file system",
			args:     []string{"a.txt", "b.txt"},
			wantErr:  "diff: no file system available\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunBlank(t *testing.T) {
	tests := []testCase{
		{
			name: "files differing in blank amount only",
			fsys: testFS(),
			args: []string{"-b", "blank1.txt", "blank2.txt"},
		},
		{
			name:     "missing blanks still differ",
			fsys:     testFS(),
			args:     []string{"-b", "blank1.txt", "noblank.txt"},
			want:     "1c1\n< a b\n---\n> ab\n",
			wantExit: 1,
		},
		{
			name:     "changes computed over blanks",
			stdin:    strings.NewReader("x \ny\n"),
			fsys:     testFS(),
			args:     []string{"-b", "-", "b.txt"},
			want:     "0a1\n> 1\n2c3\n< y\n---\n> 3\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDirectories(t *testing.T) {
	tests := []testCase{
		{
			name: "common subdirectories only",
			fsys: fsx.MemFS{
				"d1/sub/f": {Data: []byte("q\n")},
				"d2/sub/f": {Data: []byte("q\n")},
			},
			args: []string{"d1", "d2"},
			want: "Common subdirectories: d1/sub and d2/sub\n",
		},
		{
			name: "identical directories",
			fsys: fsx.MemFS{
				"d1/x": {Data: []byte("a\n")},
				"d2/x": {Data: []byte("a\n")},
			},
			args: []string{"d1", "d2"},
		},
		{
			name:     "file only in the first directory",
			fsys:     testFS(),
			args:     []string{"d1", "d2"},
			want:     "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\nCommon subdirectories: d1/sub and d2/sub\ndiff d1/x d2/x\n1c1\n< a\n---\n> b\n",
			wantExit: 1,
		},
		{
			name:     "file operand of a directory",
			fsys:     testFS(),
			args:     []string{"d1/x", "d2"},
			want:     "1c1\n< a\n---\n> b\n",
			wantExit: 1,
		},
		{
			name:     "directory operand of a file",
			fsys:     testFS(),
			args:     []string{"d2", "d1/x"},
			want:     "1c1\n< b\n---\n> a\n",
			wantExit: 1,
		},
		{
			name:     "options in the header",
			fsys:     testFS(),
			args:     []string{"-b", "d1", "d2"},
			want:     "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\nCommon subdirectories: d1/sub and d2/sub\ndiff -b d1/x d2/x\n1c1\n< a\n---\n> b\n",
			wantExit: 1,
		},
		{
			name:     "a directory compares identical to itself",
			fsys:     testFS(),
			args:     []string{"d1", "d1"},
			want:     "Common subdirectories: d1/sub and d1/sub\n",
			wantExit: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunWriteError(t *testing.T) {
	rt := &coreutils.Runtime{
		Stdout: errorWriter{err: errors.New("write failed")},
		Stderr: new(bytes.Buffer),
		FS:     testFS(),
	}
	err := New(rt).Run(t.Context(), "a.txt", "b.txt")
	if code := exitCode(t, err); code != 2 {
		t.Errorf("Run() exit code = %d, want 2", code)
	}
	if got := rt.Stderr.(*bytes.Buffer).String(); got != "diff: write error: write failed\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "diff: write error: write failed\n")
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		runCancelled(t, ctx, strings.NewReader("hello\n"))
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runCancelled(t, ctx, &cancelReader{cancel: cancel})
	})
}

// runCancelled runs the utility with a context that is either already done
// or becomes done while reading, and checks that the context error is
// returned without any output.
func runCancelled(t *testing.T, ctx context.Context, stdin io.Reader) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsx.MemFS{"a.txt": {Data: []byte("hello\n")}},
	}
	err := New(rt).Run(ctx, "-", "a.txt")

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
	tests := []testCase{
		{
			name: "context format",
			fsys: formatFS(),
			args: []string{"-c", "m1.txt", "m2.txt"},
			want: "*** m1.txt\tFri Jan  2 03:04:05 2026\n--- m2.txt\tFri Jan  2 03:04:05 2026\n" +
				"***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! x\n  3\n",
			wantExit: 1,
		},
		{
			name: "context format with a count",
			fsys: formatFS(),
			args: []string{"-C", "1", "m3.txt", "m4.txt"},
			want: "*** m3.txt\tFri Jan  2 03:04:05 2026\n--- m4.txt\tFri Jan  2 03:04:05 2026\n" +
				"***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! X\n  3\n",
			wantExit: 1,
		},
		{
			name: "context format with an attached count",
			fsys: formatFS(),
			args: []string{"-C2", "m3.txt", "m4.txt"},
			want: "*** m3.txt\tFri Jan  2 03:04:05 2026\n--- m4.txt\tFri Jan  2 03:04:05 2026\n" +
				"***************\n*** 1,4 ****\n  1\n! 2\n  3\n  4\n--- 1,4 ----\n  1\n! X\n  3\n  4\n",
			wantExit: 1,
		},
		{
			name: "unified format",
			fsys: formatFS(),
			args: []string{"-u", "m1.txt", "m2.txt"},
			want: "--- m1.txt\t2026-01-02 03:04:05 +0000\n+++ m2.txt\t2026-01-02 03:04:05 +0000\n" +
				"@@ -1,3 +1,3 @@\n 1\n-2\n+x\n 3\n",
			wantExit: 1,
		},
		{
			name: "unified format with a count",
			fsys: formatFS(),
			args: []string{"-U", "1", "m3.txt", "m4.txt"},
			want: "--- m3.txt\t2026-01-02 03:04:05 +0000\n+++ m4.txt\t2026-01-02 03:04:05 +0000\n" +
				"@@ -1,3 +1,3 @@\n 1\n-2\n+X\n 3\n",
			wantExit: 1,
		},
		{
			name: "unified format without context",
			fsys: formatFS(),
			args: []string{"-U0", "m3.txt", "m4.txt"},
			want: "--- m3.txt\t2026-01-02 03:04:05 +0000\n+++ m4.txt\t2026-01-02 03:04:05 +0000\n" +
				"@@ -2 +2 @@\n-2\n+X\n",
			wantExit: 1,
		},
		{
			name:     "ed format",
			fsys:     formatFS(),
			args:     []string{"-e", "m1.txt", "m2.txt"},
			want:     "2c\nx\n.\n",
			wantExit: 1,
		},
		{
			name:     "forward ed format",
			fsys:     formatFS(),
			args:     []string{"-f", "m1.txt", "m2.txt"},
			want:     "c2\nx\n.\n",
			wantExit: 1,
		},
		{
			name: "identical files in the unified format",
			fsys: formatFS(),
			args: []string{"-u", "m1.txt", "m1.txt"},
		},
		{
			name:     "binary files in the unified format",
			fsys:     testFS(),
			args:     []string{"-u", "bin1", "bin2"},
			want:     "Binary files bin1 and bin2 differ\n",
			wantExit: 1,
		},
		{
			name: "context lines keep the first file contents with -b",
			fsys: fsx.MemFS{"w1": {Data: []byte("1\n2 \n3\n"), ModTime: stamp}, "w2": {Data: []byte("1\n2\n4\n"), ModTime: stamp}},
			args: []string{"-b", "-c", "w1", "w2"},
			want: "*** w1\tFri Jan  2 03:04:05 2026\n--- w2\tFri Jan  2 03:04:05 2026\n" +
				"***************\n*** 1,3 ****\n  1\n  2 \n! 3\n--- 1,3 ----\n  1\n  2\n! 4\n",
			wantExit: 1,
		},
		{
			name:     "conflicting output styles",
			fsys:     formatFS(),
			args:     []string{"-c", "-u", "m1.txt", "m2.txt"},
			wantErr:  "diff: conflicting output style options\n",
			wantExit: 2,
		},
		{
			name: "same output styles combine",
			fsys: formatFS(),
			args: []string{"-c", "-C", "1", "m3.txt", "m4.txt"},
			want: "*** m3.txt\tFri Jan  2 03:04:05 2026\n--- m4.txt\tFri Jan  2 03:04:05 2026\n" +
				"***************\n*** 1,3 ****\n  1\n! 2\n  3\n--- 1,3 ----\n  1\n! X\n  3\n",
			wantExit: 1,
		},
		{
			name:     "invalid context count",
			fsys:     formatFS(),
			args:     []string{"-C", "0", "m1.txt", "m2.txt"},
			wantErr:  "diff: invalid context length '0'\n",
			wantExit: 2,
		},
		{
			name:     "invalid context count argument",
			fsys:     formatFS(),
			args:     []string{"-C", "x", "m1.txt", "m2.txt"},
			wantErr:  "diff: invalid context length 'x'\n",
			wantExit: 2,
		},
		{
			name:     "negative unified context count",
			fsys:     formatFS(),
			args:     []string{"-U", "-1", "m1.txt", "m2.txt"},
			wantErr:  "diff: invalid context length '-1'\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
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
	if code := exitCode(t, err); code != 1 {
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
	tests := []testCase{
		{
			name: "directories compared recursively",
			fsys: testFS(),
			args: []string{"-r", "d1", "d2"},
			want: "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\n" +
				"diff -r d1/sub/f d2/sub/f\n1c1\n< q\n---\n> w\n" +
				"diff -r d1/x d2/x\n1c1\n< a\n---\n> b\n",
			wantExit: 1,
		},
		{
			name: "deeply nested files",
			fsys: fsx.MemFS{
				"d3/a/b/c.txt": {Data: []byte("1\n")},
				"d4/a/b/c.txt": {Data: []byte("2\n")},
			},
			args:     []string{"-r", "d3", "d4"},
			want:     "diff -r d3/a/b/c.txt d4/a/b/c.txt\n1c1\n< 1\n---\n> 2\n",
			wantExit: 1,
		},
		{
			name: "recursion options in the header",
			fsys: fsx.MemFS{
				"d3/a/b/c.txt": {Data: []byte("1\n2\n3\n4\n5\n"), ModTime: stamp},
				"d4/a/b/c.txt": {Data: []byte("1\nX\n3\n4\n5\n"), ModTime: stamp},
			},
			args: []string{"-r", "-C", "2", "d3", "d4"},
			want: "diff -r -C 2 d3/a/b/c.txt d4/a/b/c.txt\n" +
				"*** d3/a/b/c.txt\tFri Jan  2 03:04:05 2026\n--- d4/a/b/c.txt\tFri Jan  2 03:04:05 2026\n" +
				"***************\n*** 1,4 ****\n  1\n! 2\n  3\n  4\n--- 1,4 ----\n  1\n! X\n  3\n  4\n",
			wantExit: 1,
		},
		{
			name:     "ed format keeps the header under directories",
			fsys:     testFS(),
			args:     []string{"-r", "-e", "d1", "d2"},
			want:     "File d1/mix is a regular file while file d2/mix is a directory\nOnly in d1: only\ndiff -r -e d1/sub/f d2/sub/f\n1c\nw\n.\ndiff -r -e d1/x d2/x\n1c\nb\n.\n",
			wantExit: 1,
		},
		{
			name: "identical directories compare equal recursively",
			fsys: fsx.MemFS{
				"d1/x":   {Data: []byte("a\n")},
				"d2/x":   {Data: []byte("a\n")},
				"d1/s/y": {Data: []byte("b\n")},
				"d2/s/y": {Data: []byte("b\n")},
			},
			args: []string{"-r", "d1", "d2"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestHelpVersion(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"help", []string{"--help"}, usage},
		{
			"version",
			[]string{"--version"},
			"diff (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, testCase{name: test.name, args: test.args, want: test.want})
		})
	}
}
