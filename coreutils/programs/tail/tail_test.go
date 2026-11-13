package tail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// testFS holds the fixtures used by the tests:
//
//	a.txt:     "a\nb\nc\nd\ne\n"
//	nonl.txt:  "a\nb\nc" (no trailing newline)
//	empty.txt: ""
//	bytes.txt: "0123456789"
//	lines.txt: "l1\nl2\nl3\nl4\nl5\n"
//	many.txt:  12 numbered lines, "01\n" to "12\n"
//	nl.txt:    "a\n"
//	"-f":      "dash-f\n" (a file named like an option)
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":         {Data: []byte("a\nb\nc\nd\ne\n")},
		"nonl.txt":      {Data: []byte("a\nb\nc")},
		"empty.txt":     {Data: []byte("")},
		"bytes.txt":     {Data: []byte("0123456789")},
		"lines.txt":     {Data: []byte("l1\nl2\nl3\nl4\nl5\n")},
		"many.txt":      {Data: []byte(manyLines())},
		"nl.txt":        {Data: []byte("a\n")},
		"-f":            {Data: []byte("dash-f\n")},
		"sub/inner.txt": {Data: []byte("inner\n")},
	}
}

// manyLines returns 12 numbered lines, from "01\n" to "12\n".
func manyLines() string {
	var b strings.Builder
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&b, "%02d\n", i)
	}
	return b.String()
}

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no operand reads the standard input",
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
			Name:  "defaults to the last ten lines",
			Stdin: strings.NewReader(manyLines()),
			Want:  "03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n",
		},
		{
			Name: "fewer than ten lines copies the whole file",
			FS:   testFS(),
			Args: []string{"a.txt"},
			Want: "a\nb\nc\nd\ne\n",
		},
		{
			Name: "more than ten lines copies the last ten",
			FS:   testFS(),
			Args: []string{"many.txt"},
			Want: "03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n",
		},
		{
			Name: "file without a trailing newline",
			FS:   testFS(),
			Args: []string{"nonl.txt"},
			Want: "a\nb\nc",
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
			Args: []string{"--", "-f"},
			Want: "dash-f\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLines(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the last lines of a file",
			FS:   testFS(),
			Args: []string{"-n", "2", "lines.txt"},
			Want: "l4\nl5\n",
		},
		{
			Name: "number joined to the option",
			FS:   testFS(),
			Args: []string{"-n2", "lines.txt"},
			Want: "l4\nl5\n",
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
			Name: "exactly the number of lines in the file",
			FS:   testFS(),
			Args: []string{"-n", "5", "a.txt"},
			Want: "a\nb\nc\nd\ne\n",
		},
		{
			Name: "the final line without a newline counts as a line",
			FS:   testFS(),
			Args: []string{"-n", "1", "nonl.txt"},
			Want: "c",
		},
		{
			Name: "counting the final incomplete line",
			FS:   testFS(),
			Args: []string{"-n", "2", "nonl.txt"},
			Want: "b\nc",
		},
		{
			Name: "more lines than the file without a trailing newline",
			FS:   testFS(),
			Args: []string{"-n", "4", "nonl.txt"},
			Want: "a\nb\nc",
		},
		{
			Name: "a negative sign counts from the end",
			FS:   testFS(),
			Args: []string{"-n", "-2", "lines.txt"},
			Want: "l4\nl5\n",
		},
		{
			Name: "from the second line",
			FS:   testFS(),
			Args: []string{"-n", "+2", "lines.txt"},
			Want: "l2\nl3\nl4\nl5\n",
		},
		{
			Name: "from the first line",
			FS:   testFS(),
			Args: []string{"-n", "+1", "lines.txt"},
			Want: "l1\nl2\nl3\nl4\nl5\n",
		},
		{
			Name: "from the last line",
			FS:   testFS(),
			Args: []string{"-n", "+5", "lines.txt"},
			Want: "l5\n",
		},
		{
			Name: "from beyond the last line",
			FS:   testFS(),
			Args: []string{"-n", "+6", "lines.txt"},
			Want: "",
		},
		{
			Name: "from a line of a file without a trailing newline",
			FS:   testFS(),
			Args: []string{"-n", "+2", "nonl.txt"},
			Want: "b\nc",
		},
		{
			Name: "single line file",
			FS:   testFS(),
			Args: []string{"-n", "2", "nl.txt"},
			Want: "a\n",
		},
		{
			Name: "empty file",
			FS:   testFS(),
			Args: []string{"-n", "2", "empty.txt"},
			Want: "",
		},
		{
			Name:  "lines of the standard input",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-n", "2"},
			Want:  "b\nc\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunBytes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the last bytes of a file",
			FS:   testFS(),
			Args: []string{"-c", "3", "bytes.txt"},
			Want: "789",
		},
		{
			Name: "number joined to the option",
			FS:   testFS(),
			Args: []string{"-c3", "bytes.txt"},
			Want: "789",
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
			Args: []string{"-c", "100", "bytes.txt"},
			Want: "0123456789",
		},
		{
			Name: "a negative sign counts from the end",
			FS:   testFS(),
			Args: []string{"-c", "-3", "bytes.txt"},
			Want: "789",
		},
		{
			Name: "from the fourth byte",
			FS:   testFS(),
			Args: []string{"-c", "+4", "bytes.txt"},
			Want: "3456789",
		},
		{
			Name: "from the first byte",
			FS:   testFS(),
			Args: []string{"-c", "+1", "bytes.txt"},
			Want: "0123456789",
		},
		{
			Name: "from beyond the last byte",
			FS:   testFS(),
			Args: []string{"-c", "+11", "bytes.txt"},
			Want: "",
		},
		{
			Name: "bytes can cut through a line",
			FS:   testFS(),
			Args: []string{"-c", "3", "nonl.txt"},
			Want: "b\nc",
		},
		{
			Name: "a single byte",
			FS:   testFS(),
			Args: []string{"-c", "1", "nonl.txt"},
			Want: "c",
		},
		{
			Name: "ending at a newline",
			FS:   testFS(),
			Args: []string{"-c", "2", "lines.txt"},
			Want: "5\n",
		},
		{
			Name: "empty file",
			FS:   testFS(),
			Args: []string{"-c", "3", "empty.txt"},
			Want: "",
		},
		{
			Name:  "bytes of the standard input",
			Stdin: strings.NewReader("abcdef"),
			Args:  []string{"-c", "3"},
			Want:  "def",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLastOptionWins(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "bytes after lines",
			FS:   testFS(),
			Args: []string{"-n", "2", "-c", "2", "lines.txt"},
			Want: "5\n",
		},
		{
			Name: "lines after bytes",
			FS:   testFS(),
			Args: []string{"-c", "2", "-n", "2", "lines.txt"},
			Want: "l4\nl5\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunMultipleOperands(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a header precedes the output of each file",
			FS:   testFS(),
			Args: []string{"a.txt", "lines.txt"},
			Want: "==> a.txt <==\na\nb\nc\nd\ne\n\n==> lines.txt <==\nl1\nl2\nl3\nl4\nl5\n",
		},
		{
			Name: "the operands are processed in order",
			FS:   testFS(),
			Args: []string{"lines.txt", "a.txt"},
			Want: "==> lines.txt <==\nl1\nl2\nl3\nl4\nl5\n\n==> a.txt <==\na\nb\nc\nd\ne\n",
		},
		{
			Name: "the options apply to every file",
			FS:   testFS(),
			Args: []string{"-n", "1", "a.txt", "lines.txt"},
			Want: "==> a.txt <==\ne\n\n==> lines.txt <==\nl5\n",
		},
		{
			Name: "a single operand prints no header",
			FS:   testFS(),
			Args: []string{"a.txt"},
			Want: "a\nb\nc\nd\ne\n",
		},
		{
			Name:  "the standard input among the operands",
			Stdin: strings.NewReader("one\ntwo\n"),
			FS:    testFS(),
			Args:  []string{"a.txt", "-"},
			Want:  "==> a.txt <==\na\nb\nc\nd\ne\n\n==> standard input <==\none\ntwo\n",
		},
		{
			Name: "a file without output still prints its header",
			FS:   testFS(),
			Args: []string{"empty.txt", "a.txt"},
			Want: "==> empty.txt <==\n\n==> a.txt <==\na\nb\nc\nd\ne\n",
		},
		{
			Name:     "a failing file doesn't stop the others",
			FS:       testFS(),
			Args:     []string{"missing.txt", "a.txt"},
			Want:     "==> a.txt <==\na\nb\nc\nd\ne\n",
			WantErr:  "tail: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid zero-length name doesn't stop the others",
			FS:       testFS(),
			Args:     []string{"", "a.txt"},
			Want:     "==> a.txt <==\na\nb\nc\nd\ne\n",
			WantErr:  "tail: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name: "every file failing exits nonzero",
			FS:   testFS(),
			Args: []string{"missing.txt", "also-missing.txt"},
			WantErr: "tail: missing.txt: file does not exist\n" +
				"tail: also-missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a read error doesn't stop the others",
			Stdin:    testutil.ErrorReader{Err: errors.New("boom")},
			FS:       testFS(),
			Args:     []string{"-", "a.txt"},
			Want:     "==> standard input <==\n\n==> a.txt <==\na\nb\nc\nd\ne\n",
			WantErr:  "tail: -: boom\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunHeaders(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "quiet suppresses the headers",
			FS:   testFS(),
			Args: []string{"-q", "a.txt", "lines.txt"},
			Want: "a\nb\nc\nd\ne\nl1\nl2\nl3\nl4\nl5\n",
		},
		{
			Name: "silent is the same as quiet",
			FS:   testFS(),
			Args: []string{"--silent", "a.txt", "lines.txt"},
			Want: "a\nb\nc\nd\ne\nl1\nl2\nl3\nl4\nl5\n",
		},
		{
			Name:     "quiet keeps the headers off failing files too",
			FS:       testFS(),
			Args:     []string{"-q", "missing.txt", "a.txt"},
			Want:     "a\nb\nc\nd\ne\n",
			WantErr:  "tail: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "verbose prints the header of a single file",
			FS:   testFS(),
			Args: []string{"-v", "a.txt"},
			Want: "==> a.txt <==\na\nb\nc\nd\ne\n",
		},
		{
			Name:  "verbose names the standard input",
			Stdin: strings.NewReader("one\n"),
			Args:  []string{"-v"},
			Want:  "==> standard input <==\none\n",
		},
		{
			Name:  "verbose prints the header of the standard input operand",
			Stdin: strings.NewReader("one\n"),
			FS:    testFS(),
			Args:  []string{"--verbose", "-"},
			Want:  "==> standard input <==\none\n",
		},
		{
			Name: "verbose prints the headers of several files",
			FS:   testFS(),
			Args: []string{"-v", "a.txt", "lines.txt"},
			Want: "==> a.txt <==\na\nb\nc\nd\ne\n\n==> lines.txt <==\nl1\nl2\nl3\nl4\nl5\n",
		},
		{
			Name: "the last of quiet and verbose wins",
			FS:   testFS(),
			Args: []string{"-q", "-v", "a.txt"},
			Want: "==> a.txt <==\na\nb\nc\nd\ne\n",
		},
		{
			Name: "quiet after verbose wins the other way",
			FS:   testFS(),
			Args: []string{"-v", "-q", "a.txt", "lines.txt"},
			Want: "a\nb\nc\nd\ne\nl1\nl2\nl3\nl4\nl5\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunFollowMultipleFiles(t *testing.T) {
	t.Run("the followed files get a header when the output switches", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a.txt":     {Data: []byte("a\n")},
			"lines.txt": {Data: []byte("l1\n")},
		}
		steps := []func(){
			func() { fsys["a.txt"].Data = append(fsys["a.txt"].Data, "a2\n"...) },
			func() { fsys["lines.txt"].Data = append(fsys["lines.txt"].Data, "l2\n"...) },
			func() { fsys["a.txt"].Data = append(fsys["a.txt"].Data, "a3\n"...) },
		}
		got, stderr, err := runFollowSteps(t, fsys, []string{"-f", "a.txt", "lines.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		want := "==> a.txt <==\na\n" +
			"\n==> lines.txt <==\nl1\n" +
			"\n==> a.txt <==\na2\n" +
			"\n==> lines.txt <==\nl2\n" +
			"\n==> a.txt <==\na3\n"
		if got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if stderr != "" {
			t.Errorf("Run() stderr = %q, want %q", stderr, "")
		}
	})

	t.Run("-q never prints the headers", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a.txt":     {Data: []byte("a\n")},
			"lines.txt": {Data: []byte("l1\n")},
		}
		steps := []func(){
			func() { fsys["lines.txt"].Data = append(fsys["lines.txt"].Data, "l2\n"...) },
		}
		got, _, err := runFollowSteps(t, fsys, []string{"-fq", "a.txt", "lines.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "a\nl1\nl2\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}

// runFollowSteps runs tail with args on fsys, running each step on a wait,
// and cancelling the context once the steps run out.
func runFollowSteps(t *testing.T, fsys fsx.MemFS, args []string, steps []func()) (string, string, error) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	i := 0
	rt := &coreutils.Runtime{
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsys,
		Sleep: func(ctx context.Context, d time.Duration) error {
			if i >= len(steps) {
				cancel()
				return ctx.Err()
			}
			steps[i]()
			i++
			return nil
		},
	}
	err := New(rt).Run(ctx, args...)
	return stdout.String(), stderr.String(), err
}

func TestRunReverse(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "all lines in reverse",
			FS:   testFS(),
			Args: []string{"-r", "a.txt"},
			Want: "e\nd\nc\nb\na\n",
		},
		{
			Name: "lines are concatenated as they are read",
			FS:   testFS(),
			Args: []string{"-r", "nonl.txt"},
			Want: "cb\na\n",
		},
		{
			Name: "a single line",
			FS:   testFS(),
			Args: []string{"-r", "nl.txt"},
			Want: "a\n",
		},
		{
			Name: "empty file",
			FS:   testFS(),
			Args: []string{"-r", "empty.txt"},
			Want: "",
		},
		{
			Name: "the last lines in reverse",
			FS:   testFS(),
			Args: []string{"-r", "-n", "2", "a.txt"},
			Want: "e\nd\n",
		},
		{
			Name: "number joined to the option",
			FS:   testFS(),
			Args: []string{"-r", "-n2", "a.txt"},
			Want: "e\nd\n",
		},
		{
			Name: "the last lines of a file without a trailing newline",
			FS:   testFS(),
			Args: []string{"-r", "-n", "2", "nonl.txt"},
			Want: "cb\n",
		},
		{
			Name: "more lines than the file in reverse",
			FS:   testFS(),
			Args: []string{"-r", "-n", "100", "a.txt"},
			Want: "e\nd\nc\nb\na\n",
		},
		{
			Name: "zero lines in reverse",
			FS:   testFS(),
			Args: []string{"-r", "-n", "0", "a.txt"},
			Want: "",
		},
		{
			Name:  "lines of the standard input in reverse",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-r"},
			Want:  "c\nb\na\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunNonSeekableInput(t *testing.T) {
	tests := []struct {
		Name string
		data string
		Args []string
		Want string
	}{
		{"last lines", "a\nb\nc\n", []string{"-n", "2"}, "b\nc\n"},
		{"last bytes", "abcde", []string{"-c", "3"}, "cde"},
		{"from a line", "a\nb\nc\n", []string{"-n", "+2"}, "b\nc\n"},
		{"from a byte", "abcde", []string{"-c", "+3"}, "cde"},
		{"all lines in reverse", "a\nb\nc\n", []string{"-r"}, "c\nb\na\n"},
		{"a single line without a newline", "abc", []string{"-r"}, "abc"},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testutil.Once(t, New, testutil.Case{
				FS:   testutil.ReaderFS(strings.NewReader(test.data)),
				Args: append(test.Args, "f.txt"),
				Want: test.Want,
			})
		})
	}
}

func TestRunLinesAcrossChunks(t *testing.T) {
	const line = "123456789\n" // 10 bytes
	data := strings.Repeat(line, 1000)

	tests := []testutil.Case{
		{
			Name: "more lines than a single backwards chunk",
			FS:   fsx.MemFS{"big.txt": {Data: []byte(data)}},
			Args: []string{"-n", "500", "big.txt"},
			Want: strings.Repeat(line, 500),
		},
		{
			Name: "the last line",
			FS:   fsx.MemFS{"big.txt": {Data: []byte(data)}},
			Args: []string{"-n", "1", "big.txt"},
			Want: line,
		},
		{
			Name: "more lines than the file",
			FS:   fsx.MemFS{"big.txt": {Data: []byte(data)}},
			Args: []string{"-n", "1001", "big.txt"},
			Want: data,
		},
		{
			Name: "a single long line",
			FS:   fsx.MemFS{"long.txt": {Data: []byte(strings.Repeat("x", 10000))}},
			Args: []string{"-n", "1", "long.txt"},
			Want: strings.Repeat("x", 10000),
		},
		{
			Name: "more lines than a long single line file",
			FS:   fsx.MemFS{"long.txt": {Data: []byte(strings.Repeat("x", 10000))}},
			Args: []string{"-n", "2", "long.txt"},
			Want: strings.Repeat("x", 10000),
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunFollowFile(t *testing.T) {
	tests := []struct {
		Name   string
		data   string
		Args   []string
		chunks []string
		Want   string
	}{
		{
			Name:   "copies data appended to the file",
			data:   "one\n",
			Args:   []string{"-f", "f.txt"},
			chunks: []string{"two\n"},
			Want:   "one\ntwo\n",
		},
		{
			Name:   "copies data appended in several rounds",
			data:   "one\n",
			Args:   []string{"-f", "f.txt"},
			chunks: []string{"two\n", "three\n"},
			Want:   "one\ntwo\nthree\n",
		},
		{
			Name:   "with a line count",
			data:   "one\ntwo\nthree\n",
			Args:   []string{"-f", "-n", "2", "f.txt"},
			chunks: []string{"four\n"},
			Want:   "two\nthree\nfour\n",
		},
		{
			Name:   "with a byte count",
			data:   "abc",
			Args:   []string{"-f", "-c", "2", "f.txt"},
			chunks: []string{"de"},
			Want:   "bcde",
		},
		{
			Name:   "with a zero byte count",
			data:   "abc\n",
			Args:   []string{"-f", "-c", "0", "f.txt"},
			chunks: []string{"de\n"},
			Want:   "de\n",
		},
		{
			Name:   "from the beginning of the file",
			data:   "one\n",
			Args:   []string{"-f", "-n", "+1", "f.txt"},
			chunks: []string{"two\n"},
			Want:   "one\ntwo\n",
		},
		{
			Name:   "appending before the first wait",
			data:   "one\n",
			Args:   []string{"-f", "f.txt"},
			chunks: []string{},
			Want:   "one\n",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			got, stderr, err := runFollow(t, test.data, test.Args, test.chunks)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Run() error = %v, want context.Canceled", err)
			}
			if got != test.Want {
				t.Errorf("Run() stdout = %q, want %q", got, test.Want)
			}
			if stderr != "" {
				t.Errorf("Run() stderr = %q, want %q", stderr, "")
			}
		})
	}
}

// runFollow runs tail with -f, appending each chunk to the file on each wait,
// and cancelling the context once the chunks run out. The context error is
// returned as is.
func runFollow(t *testing.T, data string, args []string, chunks []string) (string, string, error) {
	t.Helper()

	fsys := fsx.MemFS{"f.txt": {Data: []byte(data)}}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	waits := 0
	rt := &coreutils.Runtime{
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsys,
		Sleep: func(ctx context.Context, d time.Duration) error {
			if waits >= len(chunks) {
				cancel()
				return ctx.Err()
			}
			f := fsys["f.txt"]
			f.Data = append(f.Data, chunks[waits]...)
			waits++
			return nil
		},
	}
	err := New(rt).Run(ctx, args...)
	return stdout.String(), stderr.String(), err
}

func TestRunFollowStandardInput(t *testing.T) {
	stdout := new(bytes.Buffer)
	ctx, cancel := context.WithCancel(t.Context())
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("one\ntwo\n"),
		Stdout: stdout,
		Stderr: io.Discard,
		Sleep: func(ctx context.Context, d time.Duration) error {
			cancel()
			return ctx.Err()
		},
	}
	err := New(rt).Run(ctx, "-f")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
	if want := "one\ntwo\n"; stdout.String() != want {
		t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunFollowErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		testutil.Once(t, New, testutil.Case{
			FS:       testFS(),
			Args:     []string{"-f", "missing.txt"},
			WantErr:  "tail: missing.txt: file does not exist\ntail: no files remaining\n",
			WantExit: 1,
		})
	})

	t.Run("a removed name is given up without --retry", func(t *testing.T) {
		fsys := fsx.MemFS{"f.txt": {Data: []byte("one\n")}}
		steps := []func(){
			func() { delete(fsys, "f.txt") },
		}
		got, stderr, err := runFollowSteps(t, fsys, []string{"--follow=name", "--max-unchanged-stats=1", "f.txt"}, steps)
		if code := testutil.ExitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
		if want := "one\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		wantErr := "tail: 'f.txt' has become inaccessible: file does not exist; giving up on this name\n" +
			"tail: no files remaining\n"
		if stderr != wantErr {
			t.Errorf("Run() stderr = %q, want %q", stderr, wantErr)
		}
	})
	t.Run("a read error on the followed standard input", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  testutil.ErrorReader{Err: errors.New("boom")},
			Stdout: stdout,
			Stderr: stderr,
			Sleep: func(ctx context.Context, d time.Duration) error {
				return nil
			},
		}
		err := New(rt).Run(t.Context(), "-f")
		if code := testutil.ExitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
		if stdout.String() != "" {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), "")
		}
		wantErr := "tail: standard input: boom\ntail: no files remaining\n"
		if stderr.String() != wantErr {
			t.Errorf("Run() stderr = %q, want %q", stderr.String(), wantErr)
		}
	})
}

func TestRunFollowTruncate(t *testing.T) {
	fsys := fsx.MemFS{"f.txt": {Data: []byte("one\ntwo\nthree\n")}}
	steps := []func(){
		func() { fsys["f.txt"].Data = fsys["f.txt"].Data[:4] },
	}
	got, stderr, err := runFollowSteps(t, fsys, []string{"-f", "f.txt"}, steps)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
	if want := "one\ntwo\nthree\none\n"; got != want {
		t.Errorf("Run() stdout = %q, want %q", got, want)
	}
	if want := "tail: f.txt: file truncated\n"; stderr != want {
		t.Errorf("Run() stderr = %q, want %q", stderr, want)
	}
}

func TestRunFollowName(t *testing.T) {
	t.Run("a replaced file is followed from its beginning", func(t *testing.T) {
		fsys := fsx.MemFS{"f.txt": {Data: []byte("old\n")}}
		steps := []func(){
			func() { fsys["f.txt"] = &fsx.MemFile{Data: []byte("new\n")} },
		}
		got, stderr, err := runFollowSteps(t, fsys, []string{"--follow=name", "--max-unchanged-stats=1", "f.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "old\nnew\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if want := "tail: 'f.txt' has been replaced;  following new file\n"; stderr != want {
			t.Errorf("Run() stderr = %q, want %q", stderr, want)
		}
	})

	t.Run("a reappeared file is followed from its beginning with --retry", func(t *testing.T) {
		fsys := fsx.MemFS{"f.txt": {Data: []byte("old\n")}}
		steps := []func(){
			func() { delete(fsys, "f.txt") },
			func() { fsys["f.txt"] = &fsx.MemFile{Data: []byte("new\n")} },
		}
		got, stderr, err := runFollowSteps(t, fsys, []string{"--follow=name", "--retry", "--max-unchanged-stats=1", "f.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "old\nnew\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		wantErr := "tail: 'f.txt' has become inaccessible: file does not exist\n" +
			"tail: 'f.txt' has appeared;  following new file\n"
		if stderr != wantErr {
			t.Errorf("Run() stderr = %q, want %q", stderr, wantErr)
		}
	})

	t.Run("-F follows a missing file once it appears", func(t *testing.T) {
		fsys := fsx.MemFS{}
		steps := []func(){
			func() { fsys["f.txt"] = &fsx.MemFile{Data: []byte("hello\n")} },
		}
		got, stderr, err := runFollowSteps(t, fsys, []string{"-F", "f.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "hello\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		wantErr := "tail: f.txt: file does not exist\n" +
			"tail: 'f.txt' has appeared;  following new file\n"
		if stderr != wantErr {
			t.Errorf("Run() stderr = %q, want %q", stderr, wantErr)
		}
	})

	t.Run("a stdin operand can't be followed by name", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader("one\n"),
			Stdout: io.Discard,
			Stderr: io.Discard,
			FS:     testFS(),
		}
		err := New(rt).Run(t.Context(), "--follow=name", "-")
		if code := testutil.ExitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
	})

	t.Run("the standard input is followed by descriptor by name", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		ctx, cancel := context.WithCancel(t.Context())
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader("one\ntwo\n"),
			Stdout: stdout,
			Stderr: io.Discard,
			Sleep: func(ctx context.Context, d time.Duration) error {
				cancel()
				return ctx.Err()
			},
		}
		err := New(rt).Run(ctx, "--follow=name")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "one\ntwo\n"; stdout.String() != want {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	})
}

func TestRunFollowPID(t *testing.T) {
	t.Run("the follow stops when all the writers exit", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdout: stdout,
			Stderr: io.Discard,
			FS:     fsx.MemFS{"f.txt": {Data: []byte("one\n")}},
			ProcessAlive: func(pid int) bool {
				return false
			},
			Sleep: func(ctx context.Context, d time.Duration) error {
				return nil
			},
		}
		err := New(rt).Run(t.Context(), "-f", "--pid=1234", "f.txt")
		if err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
		if want := "one\n"; stdout.String() != want {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	})

	t.Run("--pid is not supported on this system", func(t *testing.T) {
		fsys := fsx.MemFS{"f.txt": {Data: []byte("one\n")}}
		steps := []func(){}
		got, stderr, err := runFollowSteps(t, fsys, []string{"-f", "--pid=1234", "f.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "one\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		wantErr := "tail: warning: --pid=PID is not supported on this system\n"
		if stderr != wantErr {
			t.Errorf("Run() stderr = %q, want %q", stderr, wantErr)
		}
	})

	t.Run("--pid is ignored without following", func(t *testing.T) {
		testutil.Once(t, New, testutil.Case{
			FS:       testFS(),
			Args:     []string{"--pid=1", "a.txt"},
			Want:     "a\nb\nc\nd\ne\n",
			WantErr:  "tail: warning: PID ignored; --pid=PID is useful only when following\n",
			WantExit: 0,
		})
	})
}

func TestRunFollowWarnings(t *testing.T) {
	t.Run("--retry is useless without following", func(t *testing.T) {
		testutil.Once(t, New, testutil.Case{
			FS:       testFS(),
			Args:     []string{"--retry", "a.txt"},
			Want:     "a\nb\nc\nd\ne\n",
			WantErr:  "tail: warning: --retry ignored; --retry is useful only when following\n",
			WantExit: 0,
		})
	})

	t.Run("--retry only affects the initial open by descriptor", func(t *testing.T) {
		fsys := fsx.MemFS{"f.txt": {Data: []byte("one\n")}}
		steps := []func(){}
		_, stderr, err := runFollowSteps(t, fsys, []string{"-f", "--retry", "f.txt"}, steps)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "tail: warning: --retry only effective for the initial open\n"; stderr != want {
			t.Errorf("Run() stderr = %q, want %q", stderr, want)
		}
	})
}

func TestRunFollowSleepInterval(t *testing.T) {
	t.Run("the default is one second", func(t *testing.T) {
		var got time.Duration
		fsys := fsx.MemFS{"f.txt": {Data: []byte("one\n")}}
		stdout := new(bytes.Buffer)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		rt := &coreutils.Runtime{
			Stdout: stdout,
			Stderr: io.Discard,
			FS:     fsys,
			Sleep: func(ctx context.Context, d time.Duration) error {
				got = d
				cancel()
				return ctx.Err()
			},
		}
		err := New(rt).Run(ctx, "-f", "f.txt")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := time.Second; got != want {
			t.Errorf("sleep interval = %v, want %v", got, want)
		}
	})

	for name, value := range map[string]string{
		"a decimal number":  "0.5",
		"a comma separator": "0,5",
		"zero seconds":      "0",
	} {
		t.Run(name, func(t *testing.T) {
			var got time.Duration
			fsys := fsx.MemFS{"f.txt": {Data: []byte("one\n")}}
			stdout := new(bytes.Buffer)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			rt := &coreutils.Runtime{
				Stdout: stdout,
				Stderr: io.Discard,
				FS:     fsys,
				Sleep: func(ctx context.Context, d time.Duration) error {
					got = d
					cancel()
					return ctx.Err()
				},
			}
			err := New(rt).Run(ctx, "-f", "-s", value, "f.txt")
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Run() error = %v, want context.Canceled", err)
			}
			want := time.Duration(0)
			if value != "0" {
				want = 500 * time.Millisecond
			}
			if got != want {
				t.Errorf("sleep interval = %v, want %v", got, want)
			}
		})
	}
}

func TestRunFollowDebug(t *testing.T) {
	tests := []struct {
		Name    string
		Args    []string
		WantErr string
	}{
		{
			Name:    "following by descriptor polls",
			Args:    []string{"--debug", "-f", "f.txt"},
			WantErr: "tail: using polling mode\n",
		},
		{
			Name:    "following by name polls",
			Args:    []string{"--debug", "-F", "f.txt"},
			WantErr: "tail: using polling mode\n",
		},
		{
			Name:    "several files poll",
			Args:    []string{"--debug", "-f", "f.txt", "g.txt"},
			WantErr: "tail: using polling mode\n",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			fsys := fsx.MemFS{"f.txt": {Data: []byte("one\n")}, "g.txt": {Data: []byte("1\n")}}
			_, stderr, err := runFollowSteps(t, fsys, test.Args, nil)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Run() error = %v, want context.Canceled", err)
			}
			if stderr != test.WantErr {
				t.Errorf("Run() stderr = %q, want %q", stderr, test.WantErr)
			}
		})
	}
}

// pipeStdin is a standard input which stats as a FIFO or a pipe.
type pipeStdin struct{ io.Reader }

func (pipeStdin) Stat() (fs.FileInfo, error) { return pipeInfo{}, nil }

// pipeInfo describes a FIFO.
type pipeInfo struct{}

func (pipeInfo) Name() string       { return "stdin" }
func (pipeInfo) Size() int64        { return 0 }
func (pipeInfo) Mode() fs.FileMode  { return fs.ModeNamedPipe | 0o600 }
func (pipeInfo) ModTime() time.Time { return time.Time{} }
func (pipeInfo) IsDir() bool        { return false }
func (pipeInfo) Sys() any           { return nil }

func TestRunFollowStdinPipe(t *testing.T) {
	t.Run("-f is ignored with no file operand", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  pipeStdin{strings.NewReader("hi\n")},
			Stdout: stdout,
			Stderr: io.Discard,
			Sleep: func(ctx context.Context, d time.Duration) error {
				t.Error("the follow waited")
				return nil
			},
		}
		err := New(rt).Run(t.Context(), "-f")
		if err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
		if want := "hi\n"; stdout.String() != want {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	})

	t.Run("a stdin operand isn't followed", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		rt := &coreutils.Runtime{
			Stdin:  pipeStdin{strings.NewReader("hi\n")},
			Stdout: stdout,
			Stderr: io.Discard,
			FS:     fsx.MemFS{"a.txt": {Data: []byte("a\n")}},
			Sleep: func(ctx context.Context, d time.Duration) error {
				cancel()
				return ctx.Err()
			},
		}
		err := New(rt).Run(ctx, "-f", "-", "a.txt")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		want := "==> standard input <==\nhi\n\n==> a.txt <==\na\n"
		if stdout.String() != want {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	})

	t.Run("a stdin operand alone returns after copying", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  pipeStdin{strings.NewReader("hi\n")},
			Stdout: stdout,
			Stderr: io.Discard,
			Sleep: func(ctx context.Context, d time.Duration) error {
				t.Error("the follow waited")
				return nil
			},
		}
		err := New(rt).Run(t.Context(), "-f", "-")
		if err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
		if want := "hi\n"; stdout.String() != want {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	})

	t.Run("a stdin operand with a pipe isn't followed by name", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  pipeStdin{strings.NewReader("hi\n")},
			Stdout: stdout,
			Stderr: io.Discard,
			Sleep: func(ctx context.Context, d time.Duration) error {
				t.Error("the follow waited")
				return nil
			},
		}
		err := New(rt).Run(t.Context(), "--follow=name", "-")
		if err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
		if want := "hi\n"; stdout.String() != want {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	})
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "lines takes its value in the next argument",
			FS:   testFS(),
			Args: []string{"--lines", "2", "lines.txt"},
			Want: "l4\nl5\n",
		},
		{
			Name: "lines takes its value glued to it",
			FS:   testFS(),
			Args: []string{"--lines=2", "lines.txt"},
			Want: "l4\nl5\n",
		},
		{
			Name: "bytes takes its value glued to it",
			FS:   testFS(),
			Args: []string{"--bytes=3", "bytes.txt"},
			Want: "789",
		},
		{
			Name:     "lines requires an argument",
			FS:       testFS(),
			Args:     []string{"--lines"},
			WantErr:  "tail: option '--lines' requires an argument\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)

	t.Run("follow copies data appended to the file", func(t *testing.T) {
		got, stderr, err := runFollow(t, "one\n", []string{"--follow", "f.txt"}, []string{"two\n"})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "one\ntwo\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if stderr != "" {
			t.Errorf("Run() stderr = %q, want %q", stderr, "")
		}
	})
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "missing file",
			FS:       testFS(),
			Args:     []string{"missing.txt"},
			WantErr:  "tail: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("boom")},
			Args:     []string{"-"},
			WantErr:  "tail: -: boom\n",
			WantExit: 1,
		},
		{
			Name:     "zero-length file name",
			FS:       testFS(),
			Args:     []string{""},
			WantErr:  "tail: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "no file system available",
			Args:     []string{"a.txt"},
			WantErr:  "tail: a.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input available",
			WantErr:  "tail: standard input: no standard input available\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunInvalidUsage(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "invalid option",
			FS:       testFS(),
			Args:     []string{"-x", "a.txt"},
			WantErr:  "tail: invalid option -- 'x'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unrecognized long option",
			FS:       testFS(),
			Args:     []string{"--bogus", "a.txt"},
			WantErr:  "tail: unrecognized option '--bogus'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid number",
			FS:       testFS(),
			Args:     []string{"-n", "x", "a.txt"},
			WantErr:  "tail: invalid number: 'x'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "non-decimal number",
			FS:       testFS(),
			Args:     []string{"-c", "1.5", "a.txt"},
			WantErr:  "tail: invalid number: '1.5'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "empty number",
			FS:       testFS(),
			Args:     []string{"-n", "", "a.txt"},
			WantErr:  "tail: invalid number: ''\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "plus zero",
			FS:       testFS(),
			Args:     []string{"-n", "+0", "a.txt"},
			WantErr:  "tail: invalid number: '+0'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "plus zero for bytes",
			FS:       testFS(),
			Args:     []string{"-c", "+0", "a.txt"},
			WantErr:  "tail: invalid number: '+0'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing option argument",
			FS:       testFS(),
			Args:     []string{"-n"},
			WantErr:  "tail: option requires an argument -- 'n'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "bytes with reverse",
			FS:       testFS(),
			Args:     []string{"-r", "-c", "2", "a.txt"},
			WantErr:  "tail: the -c option is not valid with -r\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "follow with reverse",
			FS:       testFS(),
			Args:     []string{"-r", "-f", "a.txt"},
			WantErr:  "tail: the -f option is not valid with -r\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "signed number with reverse",
			FS:       testFS(),
			Args:     []string{"-r", "-n", "+2", "a.txt"},
			WantErr:  "tail: invalid number: '+2'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a NaN number of seconds",
			FS:       testFS(),
			Args:     []string{"-s", "nan", "-f", "a.txt"},
			WantErr:  "tail: invalid number of seconds: 'nan'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid follow method",
			FS:       testFS(),
			Args:     []string{"--follow=bogus", "a.txt"},
			WantErr:  "tail: invalid argument 'bogus' for '--follow'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid number of seconds",
			FS:       testFS(),
			Args:     []string{"-s", "x", "-f", "a.txt"},
			WantErr:  "tail: invalid number of seconds: 'x'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative number of seconds",
			FS:       testFS(),
			Args:     []string{"-s", "-1", "-f", "a.txt"},
			WantErr:  "tail: invalid number of seconds: '-1'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid process id",
			FS:       testFS(),
			Args:     []string{"--pid=x", "-f", "a.txt"},
			WantErr:  "tail: invalid process id: 'x'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a non-positive process id",
			FS:       testFS(),
			Args:     []string{"--pid=0", "-f", "a.txt"},
			WantErr:  "tail: invalid process id: '0'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid max-unchanged-stats",
			FS:       testFS(),
			Args:     []string{"--max-unchanged-stats=x", "-F", "a.txt"},
			WantErr:  "tail: invalid number: 'x'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
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
	err := New(rt).Run(t.Context(), "-n", "2", "a.txt")
	if code := testutil.ExitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
	}
	if got, want := rt.Stderr.(*bytes.Buffer).String(), "tail: write error: boom\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
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
	if stdin.Reads > 0 {
		t.Error("the standard input was read")
	}
}

func TestRunSizeSuffixes(t *testing.T) {
	// big.txt holds 3000 bytes.
	fsys := fsx.MemFS{"big.txt": {Data: []byte(strings.Repeat("x", 3000))}}
	last := func(n int) string { return strings.Repeat("x", n) }
	tests := []testutil.Case{
		{
			Name: "bytes with a binary suffix",
			FS:   fsys,
			Args: []string{"-c", "1K", "big.txt"},
			Want: last(1024),
		},
		{
			Name: "bytes with a binary suffix glued to the option",
			FS:   fsys,
			Args: []string{"-c1K", "big.txt"},
			Want: last(1024),
		},
		{
			Name: "bytes with a binary i suffix",
			FS:   fsys,
			Args: []string{"-c", "1KiB", "big.txt"},
			Want: last(1024),
		},
		{
			Name: "bytes with a decimal suffix",
			FS:   fsys,
			Args: []string{"-c", "1KB", "big.txt"},
			Want: last(1000),
		},
		{
			Name: "bytes with a megabyte suffix counting from the end",
			FS:   fsys,
			Args: []string{"-c", "-1K", "big.txt"},
			Want: last(1024),
		},
		{
			Name: "bytes with a megabyte suffix larger than the file",
			FS:   fsys,
			Args: []string{"-c", "1M", "big.txt"},
			Want: last(3000),
		},
		{
			Name: "bytes with a block suffix",
			FS:   fsys,
			Args: []string{"-c", "2b", "big.txt"},
			Want: last(1024),
		},
		{
			Name: "bytes with a block suffix counting from the start",
			FS:   fsys,
			Args: []string{"-c", "+2b", "big.txt"},
			Want: last(3000 - 1023),
		},
		{
			Name: "lines with a suffix larger than the file",
			FS:   testFS(),
			Args: []string{"-n", "1K", "many.txt"},
			Want: manyLines(),
		},
		{
			Name: "lines with a megabyte suffix",
			FS:   testFS(),
			Args: []string{"-n", "2M", "many.txt"},
			Want: manyLines(),
		},
		{
			Name: "the long forms take suffixes too",
			FS:   fsys,
			Args: []string{"--bytes=1K", "big.txt"},
			Want: last(1024),
		},
		{
			Name:     "an unknown suffix is an invalid number",
			FS:       testFS(),
			Args:     []string{"-n", "5x", "a.txt"},
			WantErr:  "tail: invalid number: '5x'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a suffix without digits is an invalid number",
			FS:       testFS(),
			Args:     []string{"-c", "K", "a.txt"},
			WantErr:  "tail: invalid number: 'K'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a fraction with a suffix is an invalid number",
			FS:       testFS(),
			Args:     []string{"-c", "1.5K", "a.txt"},
			WantErr:  "tail: invalid number: '1.5K'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunZeroTerminated(t *testing.T) {
	fsys := fsx.MemFS{
		"z.txt":     {Data: []byte("a\x00b\x00c\x00")},
		"znonl.txt": {Data: []byte("a\x00b\x00c")},
	}
	tests := []testutil.Case{
		{
			Name: "the last items",
			FS:   fsys,
			Args: []string{"-z", "-n", "2", "z.txt"},
			Want: "b\x00c\x00",
		},
		{
			Name: "the final item without a terminator counts as an item",
			FS:   fsys,
			Args: []string{"-z", "-n", "1", "znonl.txt"},
			Want: "c",
		},
		{
			Name: "the last items without a final terminator",
			FS:   fsys,
			Args: []string{"-z", "-n", "2", "znonl.txt"},
			Want: "b\x00c",
		},
		{
			Name: "from an item of the start",
			FS:   fsys,
			Args: []string{"-z", "-n", "+2", "znonl.txt"},
			Want: "b\x00c",
		},
		{
			Name: "from beyond the last item",
			FS:   fsys,
			Args: []string{"-z", "-n", "+4", "z.txt"},
			Want: "",
		},
		{
			Name: "zero items",
			FS:   fsys,
			Args: []string{"-z", "-n", "0", "z.txt"},
			Want: "",
		},
		{
			Name: "the items in reverse",
			FS:   fsys,
			Args: []string{"-z", "-r", "znonl.txt"},
			Want: "cb\x00a\x00",
		},
		{
			Name: "the last items in reverse",
			FS:   fsys,
			Args: []string{"-z", "-r", "-n", "2", "z.txt"},
			Want: "c\x00b\x00",
		},
		{
			Name: "bytes are unaffected",
			FS:   fsys,
			Args: []string{"-z", "-c", "2", "znonl.txt"},
			Want: "\x00c",
		},
		{
			Name: "the long form",
			FS:   fsys,
			Args: []string{"--zero-terminated", "-n", "2", "z.txt"},
			Want: "b\x00c\x00",
		},
		{
			Name:  "the last items of the standard input",
			Stdin: strings.NewReader("a\x00b\x00c"),
			Args:  []string{"-z", "-n", "2"},
			Want:  "b\x00c",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunZeroTerminatedNonSeekable(t *testing.T) {
	tests := []struct {
		Name string
		data string
		Args []string
		Want string
	}{
		{"last items", "a\x00b\x00c", []string{"-z", "-n", "2"}, "b\x00c"},
		{"from an item", "a\x00b\x00c", []string{"-z", "-n", "+2"}, "b\x00c"},
		{"all items in reverse", "a\x00b\x00c", []string{"-z", "-r"}, "cb\x00a\x00"},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testutil.Once(t, New, testutil.Case{
				FS:   testutil.ReaderFS(strings.NewReader(test.data)),
				Args: append(test.Args, "f.txt"),
				Want: test.Want,
			})
		})
	}
}

func TestRunObsolete(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the last lines",
			FS:   testFS(),
			Args: []string{"-5", "many.txt"},
			Want: "08\n09\n10\n11\n12\n",
		},
		{
			Name: "no count defaults to ten lines",
			FS:   testFS(),
			Args: []string{"-l", "many.txt"},
			Want: "03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n",
		},
		{
			Name: "from a line of the start",
			FS:   testFS(),
			Args: []string{"+3", "many.txt"},
			Want: "03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n",
		},
		{
			Name: "plus zero is the whole input",
			FS:   testFS(),
			Args: []string{"+0", "lines.txt"},
			Want: "l1\nl2\nl3\nl4\nl5\n",
		},
		{
			Name: "a line count with a size letter",
			FS:   testFS(),
			Args: []string{"-2l", "lines.txt"},
			Want: "l4\nl5\n",
		},
		{
			Name: "a byte count with a size letter",
			FS:   testFS(),
			Args: []string{"-3c", "bytes.txt"},
			Want: "789",
		},
		{
			Name: "a byte count from the start",
			FS:   testFS(),
			Args: []string{"+4c", "bytes.txt"},
			Want: "3456789",
		},
		{
			Name: "the letter alone defaults to ten",
			FS:   fsx.MemFS{"big.txt": {Data: []byte(strings.Repeat("x", 3000))}},
			Args: []string{"-b", "big.txt"},
			Want: strings.Repeat("x", 3000),
		},
		{
			Name: "a block count",
			FS:   fsx.MemFS{"big.txt": {Data: []byte(strings.Repeat("x", 3000))}},
			Args: []string{"-2b", "big.txt"},
			Want: strings.Repeat("x", 1024),
		},
		{
			Name: "a block count from the start",
			FS:   fsx.MemFS{"big.txt": {Data: []byte(strings.Repeat("x", 3000))}},
			Args: []string{"+2b", "big.txt"},
			Want: strings.Repeat("x", 3000-1023),
		},
		{
			Name:  "a count of the standard input operand",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-2", "-"},
			Want:  "b\nc\n",
		},
		{
			Name: "double dash still ends the options",
			FS:   testFS(),
			Args: []string{"-2", "--", "lines.txt"},
			Want: "l4\nl5\n",
		},
		{
			Name: "double dash allows a file named like an option",
			FS:   testFS(),
			Args: []string{"-2", "--", "-f"},
			Want: "dash-f\n",
		},
		{
			Name: "the options of the standard syntax still apply",
			FS:   testFS(),
			Args: []string{"-2", "a.txt"},
			Want: "d\ne\n",
		},
		{
			Name: "options after the obsolete form are rejected",
			FS: fsx.MemFS{
				"a.txt": {Data: []byte("a\n")},
				"b.txt": {Data: []byte("b\n")},
			},
			Args:     []string{"-1", "--", "-q", "a.txt", "b.txt"},
			WantErr:  "tail: option used in invalid context -- '1'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunObsoleteFollow(t *testing.T) {
	t.Run("a trailing f follows the file", func(t *testing.T) {
		got, stderr, err := runFollow(t, "one\ntwo\nthree\n", []string{"-2f", "f.txt"}, []string{"four\n"})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "two\nthree\nfour\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
		if stderr != "" {
			t.Errorf("Run() stderr = %q, want %q", stderr, "")
		}
	})

	t.Run("a trailing f after a size letter follows the file", func(t *testing.T) {
		got, _, err := runFollow(t, "one\ntwo\nthree\n", []string{"-2lf", "f.txt"}, []string{"four\n"})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "two\nthree\nfour\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("a leading plus follows from the start", func(t *testing.T) {
		got, _, err := runFollow(t, "one\ntwo\n", []string{"+2f", "f.txt"}, []string{"three\n"})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if want := "two\nthree\n"; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}

func TestRunObsoleteInvalid(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "a digit after other options",
			FS:       testFS(),
			Args:     []string{"-n", "2", "-5", "a.txt"},
			WantErr:  "tail: option used in invalid context -- '5'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a digit before other options",
			FS:       testFS(),
			Args:     []string{"-5", "-n", "2", "a.txt"},
			WantErr:  "tail: option used in invalid context -- '5'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a digit with two file operands",
			FS:       testFS(),
			Args:     []string{"-5", "a.txt", "lines.txt"},
			WantErr:  "tail: option used in invalid context -- '5'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a letter which is not a size letter",
			FS:       testFS(),
			Args:     []string{"-5x", "a.txt"},
			WantErr:  "tail: option used in invalid context -- '5'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "the standard form takes over a glued count",
			FS:       testFS(),
			Args:     []string{"-c5", "bytes.txt"},
			Want:     "56789",
			WantExit: 0,
		},
		{
			Name:     "an option needing its argument is left alone",
			FS:       testFS(),
			Args:     []string{"-c"},
			WantErr:  "tail: option requires an argument -- 'c'\nTry 'tail --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an operand which starts with a digit",
			FS:       testFS(),
			Args:     []string{"-n", "2", "5"},
			WantErr:  "tail: 5: file does not exist\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "tail", usage)
}
