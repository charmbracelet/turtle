package grep

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

func TestRun(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "basic pattern on stdin",
			args:  []string{"foo"},
			stdin: bytes.NewBufferString("foo\nbar\nfoobar\n"),
			want:  "foo\nfoobar\n",
		},
		{
			name:     "no match exits with one",
			args:     []string{"baz"},
			stdin:    bytes.NewBufferString("foo\nbar\n"),
			wantCode: 1,
		},
		{
			name:  "bre anchors",
			args:  []string{"^bar$"},
			stdin: bytes.NewBufferString("foo\nbar\nfoobar\n"),
			want:  "bar\n",
		},
		{
			name:  "bre star at start is ordinary",
			args:  []string{"*oo"},
			stdin: bytes.NewBufferString("foo\n*oo\n"),
			want:  "*oo\n",
		},
		{
			name:  "bre group and interval",
			args:  []string{`\(ab\)\{2\}`},
			stdin: bytes.NewBufferString("ab\nabab\n"),
			want:  "abab\n",
		},
		{
			name:  "null pattern matches every line",
			args:  []string{"-e", "\n"},
			stdin: bytes.NewBufferString("foo\nbar\n"),
			want:  "foo\nbar\n",
		},
		{
			name:  "last line without newline",
			args:  []string{"foo"},
			stdin: bytes.NewBufferString("bar\nfoo"),
			want:  "foo\n",
		},
		{
			name:  "file operand",
			args:  []string{"foo", "file"},
			files: fsx.MemFS{"file": {Data: []byte("foo\nbar\n")}},
			want:  "foo\n",
		},
		{
			name:  "dash operand reads stdin",
			args:  []string{"foo", "-"},
			stdin: bytes.NewBufferString("foo\nbar\n"),
			want:  "foo\n",
		},
		{
			name:  "prefix with more than one file",
			args:  []string{"foo", "a", "b"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}, "b": {Data: []byte("foo\nbar\n")}},
			want:  "a:foo\nb:foo\n",
		},
		{
			name:  "no prefix with one file",
			args:  []string{"foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}},
			want:  "foo\n",
		},
		{
			name:  "standard input label without a file operand",
			args:  []string{"-l", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "(standard input)\n",
		},
		{
			name:  "standard input label with more than one file",
			args:  []string{"foo", "-", "a"},
			stdin: bytes.NewBufferString("foo\n"),
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}},
			want:  "(standard input):foo\na:foo\n",
		},
		{
			name:  "dash dash ends the options",
			args:  []string{"--", "-v"},
			stdin: bytes.NewBufferString("-v\nfoo\n"),
			want:  "-v\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionE(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "alternation",
			args:  []string{"-E", "foo|bar"},
			stdin: bytes.NewBufferString("foo\nbaz\nbar\n"),
			want:  "foo\nbar\n",
		},
		{
			name:  "plus quantifier",
			args:  []string{"-E", "ab+c"},
			stdin: bytes.NewBufferString("ac\nabc\nabbc\n"),
			want:  "abc\nabbc\n",
		},
		{
			name:  "interval",
			args:  []string{"-E", "a{2}"},
			stdin: bytes.NewBufferString("a\naa\naaa\n"),
			want:  "aa\naaa\n",
		},
		{
			name:  "plain parentheses are grouping",
			args:  []string{"-E", "(ab)*c"},
			stdin: bytes.NewBufferString("ababc\nc\n"),
			want:  "ababc\nc\n",
		},
		{
			name:     "invalid expression",
			args:     []string{"-E", "("},
			wantErr:  "grep: invalid regular expression: error parsing regexp: missing closing ): `(`\n",
			wantCode: 2,
		},
		{
			name:     "unmatched interval",
			args:     []string{"-E", "a{"},
			wantErr:  "grep: unmatched \\{\n",
			wantCode: 2,
		},
		{
			name:  "last mode wins",
			args:  []string{"-E", "-F", "a|b"},
			stdin: bytes.NewBufferString("a|b\nab\n"),
			want:  "a|b\n",
		},
		{
			name:  "long option",
			args:  []string{"--extended-regexp", "foo|bar"},
			stdin: bytes.NewBufferString("bar\nbaz\n"),
			want:  "bar\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionF(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "metacharacters are literal",
			args:  []string{"-F", "a.c*"},
			stdin: bytes.NewBufferString("abc\na.c*\naxc\n"),
			want:  "a.c*\n",
		},
		{
			name:  "newline separated patterns",
			args:  []string{"-F", "abc\ndef"},
			stdin: bytes.NewBufferString("abc\nxyz\ndef\n"),
			want:  "abc\ndef\n",
		},
		{
			name:  "long option",
			args:  []string{"--fixed-strings", "a.b"},
			stdin: bytes.NewBufferString("a.b\naxb\n"),
			want:  "a.b\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionC(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "count on stdin",
			args:  []string{"-c", "foo"},
			stdin: bytes.NewBufferString("foo\nbar\nfoo\n"),
			want:  "2\n",
		},
		{
			name:     "zero count",
			args:     []string{"-c", "baz"},
			stdin:    bytes.NewBufferString("foo\n"),
			want:     "0\n",
			wantCode: 1,
		},
		{
			name:  "count with line numbers is still a count",
			args:  []string{"-c", "-n", "foo"},
			stdin: bytes.NewBufferString("foo\nfoo\n"),
			want:  "2\n",
		},
		{
			name:  "prefix with more than one file",
			args:  []string{"-c", "foo", "a", "b"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}, "b": {Data: []byte("bar\n")}},
			want:  "a:1\nb:0\n",
		},
		{
			name:  "long option",
			args:  []string{"--count", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "1\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionEAndF(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "multiple e options",
			args:  []string{"-e", "foo", "-e", "bar"},
			stdin: bytes.NewBufferString("foo\nbaz\nbar\n"),
			want:  "foo\nbar\n",
		},
		{
			name:  "pattern list with newline separator",
			args:  []string{"-e", "foo\nbar"},
			stdin: bytes.NewBufferString("foo\nbaz\nbar\n"),
			want:  "foo\nbar\n",
		},
		{
			name:  "null pattern between newlines",
			args:  []string{"-e", "foo\n\nbar"},
			stdin: bytes.NewBufferString("foo\nbaz\n"),
			want:  "foo\nbaz\n",
		},
		{
			name:  "pattern starting with a hyphen",
			args:  []string{"-e", "-v"},
			stdin: bytes.NewBufferString("-v\nfoo\n"),
			want:  "-v\n",
		},
		{
			name:  "long option",
			args:  []string{"--regexp=foo"},
			stdin: bytes.NewBufferString("foo\nbar\n"),
			want:  "foo\n",
		},
		{
			name:  "operand is the pattern when e is given",
			args:  []string{"-e", "foo", "file"},
			files: fsx.MemFS{"file": {Data: []byte("file\nfoo\n")}},
			want:  "foo\n",
		},
		{
			name:  "file is the pattern operand without e",
			args:  []string{"file"},
			stdin: bytes.NewBufferString("file\nfoo\n"),
			want:  "file\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionFFile(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "patterns from a file",
			args:  []string{"-f", "patterns"},
			stdin: bytes.NewBufferString("foo\nbaz\nbar\n"),
			files: fsx.MemFS{"patterns": {Data: []byte("foo\nbar\n")}},
			want:  "foo\nbar\n",
		},
		{
			name:  "empty line is a null pattern",
			args:  []string{"-f", "patterns"},
			stdin: bytes.NewBufferString("foo\nbaz\n"),
			files: fsx.MemFS{"patterns": {Data: []byte("foo\n\n")}},
			want:  "foo\nbaz\n",
		},
		{
			name:  "last pattern without a newline",
			args:  []string{"-f", "patterns"},
			stdin: bytes.NewBufferString("foo\nbar\n"),
			files: fsx.MemFS{"patterns": {Data: []byte("foo\nbar")}},
			want:  "foo\nbar\n",
		},
		{
			name:  "e and f together",
			args:  []string{"-e", "baz", "-f", "patterns"},
			stdin: bytes.NewBufferString("foo\nbaz\nbar\n"),
			files: fsx.MemFS{"patterns": {Data: []byte("foo\nbar\n")}},
			want:  "foo\nbaz\nbar\n",
		},
		{
			name:     "patterns from the standard input leave it drained",
			args:     []string{"-f", "-"},
			stdin:    bytes.NewBufferString("foo\n"),
			wantCode: 1,
		},
		{
			name:  "long option",
			args:  []string{"--file", "patterns"},
			stdin: bytes.NewBufferString("foo\n"),
			files: fsx.MemFS{"patterns": {Data: []byte("foo\n")}},
			want:  "foo\n",
		},
		{
			name:     "missing pattern file",
			args:     []string{"-f", "missing"},
			wantErr:  "grep: missing: file does not exist\n",
			wantCode: 2,
		},
		{
			name:     "no pattern at all",
			args:     []string{},
			wantErr:  "grep: no pattern given\nTry 'grep --help' for more information.\n",
			wantCode: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionI(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "bre",
			args:  []string{"-i", "Foo"},
			stdin: bytes.NewBufferString("foo\nFOO\nbar\n"),
			want:  "foo\nFOO\n",
		},
		{
			name:  "ere",
			args:  []string{"-i", "-E", "foo|bar"},
			stdin: bytes.NewBufferString("FOO\nBar\nbaz\n"),
			want:  "FOO\nBar\n",
		},
		{
			name:  "fixed string",
			args:  []string{"-i", "-F", "Foo"},
			stdin: bytes.NewBufferString("foo\nFOO\nbar\n"),
			want:  "foo\nFOO\n",
		},
		{
			name:  "long option",
			args:  []string{"--ignore-case", "FOO"},
			stdin: bytes.NewBufferString("foo\nbar\n"),
			want:  "foo\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionL(t *testing.T) {
	tests := []runTestArgs{
		{
			name: "file names once per file",
			args: []string{"-l", "foo", "a", "b", "c"},
			files: fsx.MemFS{
				"a": {Data: []byte("foo\nfoo\n")},
				"b": {Data: []byte("bar\n")},
				"c": {Data: []byte("foo\n")},
			},
			want: "a\nc\n",
		},
		{
			name:  "standard input label",
			args:  []string{"-l", "foo", "-"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "(standard input)\n",
		},
		{
			name:  "l wins when it is the last output option",
			args:  []string{"-c", "-n", "-l", "foo", "a", "b"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}, "b": {Data: []byte("bar\n")}},
			want:  "a\n",
		},
		{
			name:  "last output option wins",
			args:  []string{"-l", "-c", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "1\n",
		},
		{
			name:  "long option",
			args:  []string{"--files-with-matches", "foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}},
			want:  "a\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionN(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "line numbers restart per file",
			args:  []string{"-n", "foo", "a", "b"},
			files: fsx.MemFS{"a": {Data: []byte("bar\nfoo\n")}, "b": {Data: []byte("foo\n")}},
			want:  "a:2:foo\nb:1:foo\n",
		},
		{
			name:  "single file has no file prefix",
			args:  []string{"-n", "foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}},
			want:  "1:foo\n",
		},
		{
			name:  "long option",
			args:  []string{"--line-number", "foo"},
			stdin: bytes.NewBufferString("bar\nfoo\n"),
			want:  "2:foo\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionQ(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "nothing is written on a match",
			args:  []string{"-q", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
		},
		{
			name:     "no match exits with one",
			args:     []string{"-q", "baz"},
			stdin:    bytes.NewBufferString("foo\n"),
			wantCode: 1,
		},
		{
			name:     "match wins over an earlier error",
			args:     []string{"-q", "foo", "missing", "a"},
			files:    fsx.MemFS{"a": {Data: []byte("foo\n")}},
			wantErr:  "grep: missing: file does not exist\n",
			wantCode: 0,
		},
		{
			name:     "error without a match still fails",
			args:     []string{"-q", "baz", "missing"},
			wantErr:  "grep: missing: file does not exist\n",
			wantCode: 2,
		},
		{
			name:  "long option",
			args:  []string{"--quiet", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
		},
		{
			name:  "silent alias",
			args:  []string{"--silent", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionS(t *testing.T) {
	tests := []runTestArgs{
		{
			name:     "missing file message is suppressed",
			args:     []string{"-s", "foo", "missing"},
			wantCode: 2,
		},
		{
			name:     "invalid pattern message is kept",
			args:     []string{"-s", "-E", "("},
			wantErr:  "grep: invalid regular expression: error parsing regexp: missing closing ): `(`\n",
			wantCode: 2,
		},
		{
			name:     "long option",
			args:     []string{"--no-messages", "foo", "missing"},
			wantCode: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionV(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "non-matching lines",
			args:  []string{"-v", "foo"},
			stdin: bytes.NewBufferString("foo\nbar\n"),
			want:  "bar\n",
		},
		{
			name:  "inverts multiple patterns",
			args:  []string{"-v", "-e", "foo", "-e", "bar"},
			stdin: bytes.NewBufferString("foo\nbaz\nbar\n"),
			want:  "baz\n",
		},
		{
			name:     "null pattern selects nothing",
			args:     []string{"-v", "-e", "\n"},
			stdin:    bytes.NewBufferString("foo\n"),
			wantCode: 1,
		},
		{
			name:  "long option",
			args:  []string{"--revert-match", "foo"},
			stdin: bytes.NewBufferString("foo\nbar\n"),
			want:  "bar\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionX(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "whole bre line",
			args:  []string{"-x", "foo"},
			stdin: bytes.NewBufferString("foo\nfoobar\n"),
			want:  "foo\n",
		},
		{
			name:  "whole ere alternation",
			args:  []string{"-x", "-E", "foo|bar"},
			stdin: bytes.NewBufferString("foo\nfoobar\nbar\n"),
			want:  "foo\nbar\n",
		},
		{
			name:  "whole fixed string",
			args:  []string{"-x", "-F", "a.c"},
			stdin: bytes.NewBufferString("a.c\nxa.c\n"),
			want:  "a.c\n",
		},
		{
			name:  "fixed string list",
			args:  []string{"-x", "-F", "foo\nbar"},
			stdin: bytes.NewBufferString("foo\nfoobar\nbar\n"),
			want:  "foo\nbar\n",
		},
		{
			name:  "anchors are still whole lines",
			args:  []string{"-x", "^foo$"},
			stdin: bytes.NewBufferString("foo\nfoobar\n"),
			want:  "foo\n",
		},
		{
			name:  "null pattern matches only empty lines",
			args:  []string{"-x", "-e", "\n"},
			stdin: bytes.NewBufferString("foo\n\n"),
			want:  "\n",
		},
		{
			name:  "long option",
			args:  []string{"--line-regexp", "foo"},
			stdin: bytes.NewBufferString("foo\nfoobar\n"),
			want:  "foo\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunInvalidUsage(t *testing.T) {
	tests := []runTestArgs{
		{
			name:    "invalid option",
			args:    []string{"-j", "foo"},
			wantErr: "grep: invalid option -- 'j'\nTry 'grep --help' for more information.\n",
		},
		{
			name:    "unrecognized long option",
			args:    []string{"--frobnicate", "foo"},
			wantErr: "grep: unrecognized option '--frobnicate'\nTry 'grep --help' for more information.\n",
		},
		{
			name:    "missing e argument",
			args:    []string{"-e"},
			wantErr: "grep: option requires an argument -- 'e'\nTry 'grep --help' for more information.\n",
		},
		{
			name:    "missing f argument",
			args:    []string{"-f"},
			wantErr: "grep: option requires an argument -- 'f'\nTry 'grep --help' for more information.\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, struct {
				name     string
				args     []string
				stdin    io.Reader
				files    fsx.MemFS
				want     string
				wantErr  string
				wantCode int
			}{name: test.name, args: test.args, wantErr: test.wantErr, wantCode: 2})
		})
	}
}

func TestRunMissingFile(t *testing.T) {
	runTest(t, struct {
		name     string
		args     []string
		stdin    io.Reader
		files    fsx.MemFS
		want     string
		wantErr  string
		wantCode int
	}{
		name:     "missing file exits with two",
		args:     []string{"foo", "missing"},
		wantErr:  "grep: missing: file does not exist\n",
		wantCode: 2,
	})
}

func TestRunCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	prog := New(&coreutils.Runtime{
		Stdin:  bytes.NewBufferString("input"),
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	if err := prog.Run(ctx, "foo"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "grep (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}

func TestSplitPatterns(t *testing.T) {
	tests := []struct {
		list string
		want []string
	}{
		{list: "", want: nil},
		{list: "foo", want: []string{"foo"}},
		{list: "foo\n", want: []string{"foo"}},
		{list: "foo\nbar", want: []string{"foo", "bar"}},
		{list: "foo\nbar\n", want: []string{"foo", "bar"}},
		{list: "\n", want: []string{""}},
		{list: "\n\n", want: []string{"", ""}},
		{list: "foo\n\nbar\n", want: []string{"foo", "", "bar"}},
	}
	for _, test := range tests {
		got := splitPatterns(test.list)
		if strings.Join(got, "\x00") != strings.Join(test.want, "\x00") {
			t.Errorf("splitPatterns(%q) = %q, want %q", test.list, got, test.want)
		}
	}
}

type runTestArgs struct {
	name     string
	args     []string
	stdin    io.Reader
	files    fsx.MemFS
	want     string
	wantErr  string
	wantCode int
}

func runTest(t *testing.T, test runTestArgs) {
	t.Helper()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	runtime := &coreutils.Runtime{Stdin: test.stdin, Stdout: stdout, Stderr: stderr, FS: test.files}
	err := New(runtime).Run(t.Context(), test.args...)
	code := 0
	if exitErr, ok := errors.AsType[*coreutils.ExitError](err); ok {
		code = exitErr.Code
	} else if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if code != test.wantCode {
		t.Errorf("Run() exit code = %d, want %d", code, test.wantCode)
	}
	if got := stdout.String(); got != test.want {
		t.Errorf("Run() stdout = %q, want %q", got, test.want)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
}
