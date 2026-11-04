package grep

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

func TestRunRecursive(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "single directory prefixes the files",
			args:  []string{"-r", "foo", "dir"},
			files: fsx.MemFS{"dir/a": {Data: []byte("foo\n")}, "dir/b": {Data: []byte("bar\n")}},
			want:  "dir/a:foo\n",
		},
		{
			name:  "single file operand has no prefix",
			args:  []string{"-r", "foo", "dir/a"},
			files: fsx.MemFS{"dir/a": {Data: []byte("foo\n")}},
			want:  "foo\n",
		},
		{
			name:  "no operand searches the working directory",
			args:  []string{"-r", "foo"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}, "sub/b": {Data: []byte("foo\n")}},
			want:  "a:foo\nsub/b:foo\n",
		},
		{
			name:     "no operand with no match",
			args:     []string{"-r", "baz"},
			files:    fsx.MemFS{"a": {Data: []byte("foo\n")}},
			wantCode: 1,
		},
		{
			name:     "missing starting point",
			args:     []string{"-r", "foo", "missing"},
			files:    fsx.MemFS{},
			wantErr:  "grep: missing: file does not exist\n",
			wantCode: 2,
		},
		{
			name:     "missing starting point is suppressed with -s",
			args:     []string{"-r", "-s", "foo", "missing"},
			files:    fsx.MemFS{},
			wantCode: 2,
		},
		{
			name:  "more than one starting point",
			args:  []string{"-r", "foo", "d1", "d2"},
			files: fsx.MemFS{"d1/a": {Data: []byte("foo\n")}, "d2/b": {Data: []byte("foo\n")}},
			want:  "d1/a:foo\nd2/b:foo\n",
		},
		{
			name:  "count per file",
			args:  []string{"-r", "-c", "foo", "dir"},
			files: fsx.MemFS{"dir/a": {Data: []byte("foo\nfoo\n")}, "dir/b": {Data: []byte("bar\n")}},
			want:  "dir/a:2\ndir/b:0\n",
		},
		{
			name:  "file names with -l",
			args:  []string{"-r", "-l", "foo", "dir"},
			files: fsx.MemFS{"dir/a": {Data: []byte("foo\n")}, "dir/b": {Data: []byte("bar\n")}},
			want:  "dir/a\n",
		},
		{
			name:  "quiet stops at the first match",
			args:  []string{"-r", "-q", "foo", "dir"},
			files: fsx.MemFS{"dir/a": {Data: []byte("foo\n")}, "dir/b": {Data: []byte("foo\n")}},
		},
		{
			name:  "long option",
			args:  []string{"--recursive", "foo", "dir"},
			files: fsx.MemFS{"dir/a": {Data: []byte("foo\n")}},
			want:  "dir/a:foo\n",
		},
		{
			name: "-R follows the symbolic links met below",
			args: []string{"-R", "foo", "dir"},
			files: fsx.MemFS{
				"dir/a":  {Data: []byte("foo\n")},
				"dir/ln": {Mode: fs.ModeSymlink, Data: []byte("a")},
			},
			want: "dir/a:foo\ndir/ln:foo\n",
		},
		{
			name: "-r skips the symbolic links met below",
			args: []string{"-r", "foo", "dir"},
			files: fsx.MemFS{
				"dir/a":  {Data: []byte("foo\n")},
				"dir/ln": {Mode: fs.ModeSymlink, Data: []byte("a")},
			},
			want: "dir/a:foo\n",
		},
		{
			name:  "standard input operand is searched as it is",
			args:  []string{"-r", "foo", "-"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "foo\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionD(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "recurse",
			args:  []string{"-d", "recurse", "foo", "dir"},
			files: fsx.MemFS{"dir/a": {Data: []byte("foo\n")}},
			want:  "dir/a:foo\n",
		},
		{
			name:     "skip",
			args:     []string{"-d", "skip", "foo", "dir"},
			files:    fsx.MemFS{"dir/a": {Data: []byte("foo\n")}},
			wantCode: 1,
		},
		{
			name:     "read fails on a directory",
			args:     []string{"-d", "read", "foo", "dir"},
			files:    fsx.MemFS{"dir/a": {Data: []byte("foo\n")}},
			wantErr:  "grep: dir: invalid argument\n",
			wantCode: 2,
		},
		{
			name:     "read is the default",
			args:     []string{"foo", "dir"},
			files:    fsx.MemFS{"dir/a": {Data: []byte("foo\n")}},
			wantErr:  "grep: dir: invalid argument\n",
			wantCode: 2,
		},
		{
			name:     "invalid action",
			args:     []string{"-d", "frob", "foo"},
			wantErr:  "grep: invalid argument 'frob' for '--directories'\nTry 'grep --help' for more information.\n",
			wantCode: 2,
		},
		{
			name:     "invalid device action",
			args:     []string{"-D", "frob", "foo"},
			wantErr:  "grep: invalid argument 'frob' for '--devices'\nTry 'grep --help' for more information.\n",
			wantCode: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunFileSelection(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "include",
			args:  []string{"-r", "--include=*.txt", "foo", "dir"},
			files: fsx.MemFS{"dir/a.txt": {Data: []byte("foo\n")}, "dir/b": {Data: []byte("foo\n")}},
			want:  "dir/a.txt:foo\n",
		},
		{
			name:  "exclude",
			args:  []string{"-r", "--exclude=*.txt", "foo", "dir"},
			files: fsx.MemFS{"dir/a.txt": {Data: []byte("foo\n")}, "dir/b": {Data: []byte("foo\n")}},
			want:  "dir/b:foo\n",
		},
		{
			name:  "the last matching option wins",
			args:  []string{"-r", "--include=*.txt", "--exclude=a.txt", "foo", "dir"},
			files: fsx.MemFS{"dir/a.txt": {Data: []byte("foo\n")}, "dir/b.txt": {Data: []byte("foo\n")}},
			want:  "dir/b.txt:foo\n",
		},
		{
			name:     "no match excludes when the first option includes",
			args:     []string{"-r", "--include=*.log", "foo", "dir"},
			files:    fsx.MemFS{"dir/a.txt": {Data: []byte("foo\n")}},
			wantCode: 1,
		},
		{
			name:  "no match includes when the first option excludes",
			args:  []string{"-r", "--exclude=*.log", "foo", "dir"},
			files: fsx.MemFS{"dir/a.txt": {Data: []byte("foo\n")}},
			want:  "dir/a.txt:foo\n",
		},
		{
			name:  "include matches the command-line name suffixes",
			args:  []string{"-r", "--include=*.txt", "foo", "dir/a.txt", "dir/b"},
			files: fsx.MemFS{"dir/a.txt": {Data: []byte("foo\n")}, "dir/b": {Data: []byte("foo\n")}},
			want:  "dir/a.txt:foo\n",
		},
		{
			name:     "exclude matches the whole command-line name",
			args:     []string{"-r", "--exclude=dir/a.txt", "foo", "dir/a.txt"},
			files:    fsx.MemFS{"dir/a.txt": {Data: []byte("foo\n")}},
			wantCode: 1,
		},
		{
			name:  "exclude-dir",
			args:  []string{"-r", "--exclude-dir=sub", "foo", "dir"},
			files: fsx.MemFS{"dir/a": {Data: []byte("foo\n")}, "dir/sub/b": {Data: []byte("foo\n")}},
			want:  "dir/a:foo\n",
		},
		{
			name:     "exclude-dir matches a command-line directory",
			args:     []string{"-r", "--exclude-dir=sub", "foo", "dir/sub"},
			files:    fsx.MemFS{"dir/sub/b": {Data: []byte("foo\n")}},
			wantCode: 1,
		},
		{
			name:  "exclude-dir ignores trailing slashes",
			args:  []string{"-r", "--exclude-dir=sub/", "foo", "dir"},
			files: fsx.MemFS{"dir/a": {Data: []byte("foo\n")}, "dir/sub/b": {Data: []byte("foo\n")}},
			want:  "dir/a:foo\n",
		},
		{
			name: "exclude-from",
			args: []string{"-r", "--exclude-from=list", "foo", "dir"},
			files: fsx.MemFS{
				"dir/a": {Data: []byte("foo\n")},
				"dir/b": {Data: []byte("foo\n")},
				"dir/c": {Data: []byte("foo\n")},
				"list":  {Data: []byte("# comment\n\nb\n")},
			},
			want: "dir/a:foo\ndir/c:foo\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionM(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "stops after the count",
			args:  []string{"-m", "1", "foo"},
			stdin: bytes.NewBufferString("foo\nbar\nfoo\n"),
			want:  "foo\n",
		},
		{
			name:     "zero stops without reading",
			args:     []string{"-m", "0", "foo"},
			stdin:    bytes.NewBufferString("foo\n"),
			wantCode: 1,
		},
		{
			name:  "minus one never stops",
			args:  []string{"-m", "-1", "foo"},
			stdin: bytes.NewBufferString("foo\nbar\nfoo\n"),
			want:  "foo\nfoo\n",
		},
		{
			name:  "count is capped",
			args:  []string{"-m", "1", "-c", "foo"},
			stdin: bytes.NewBufferString("foo\nfoo\nfoo\n"),
			want:  "1\n",
		},
		{
			name:  "trailing context is still written",
			args:  []string{"-m", "1", "-A", "1", "foo"},
			stdin: bytes.NewBufferString("foo\nbar\nbaz\n"),
			want:  "foo\nbar\n",
		},
		{
			name:     "invalid count",
			args:     []string{"-m", "x", "foo"},
			wantErr:  "grep: invalid max count 'x'\nTry 'grep --help' for more information.\n",
			wantCode: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionO(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "each match on its own line",
			args:  []string{"-o", "foo"},
			stdin: bytes.NewBufferString("foo bar foo\n"),
			want:  "foo\nfoo\n",
		},
		{
			name:  "byte offset of each match",
			args:  []string{"-o", "-b", "foo"},
			stdin: bytes.NewBufferString("afoo bfoo\n"),
			want:  "1:foo\n6:foo\n",
		},
		{
			name:  "line number of each match",
			args:  []string{"-o", "-n", "foo"},
			stdin: bytes.NewBufferString("bar\nfoo foo\n"),
			want:  "2:foo\n2:foo\n",
		},
		{
			name:  "fixed strings",
			args:  []string{"-o", "-F", "a.c"},
			stdin: bytes.NewBufferString("xa.cya.c\n"),
			want:  "a.c\na.c\n",
		},
		{
			name:  "multiple patterns match left to right",
			args:  []string{"-o", "-e", "foo", "-e", "o"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "foo\n",
		},
		{
			name:  "nothing for inverted lines",
			args:  []string{"-o", "-v", "foo"},
			stdin: bytes.NewBufferString("foo\nbar\n"),
		},
		{
			name:  "context options have no effect",
			args:  []string{"-o", "-C", "1", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "foo\n",
		},
		{
			name:  "output lines use the input delimiters with -z",
			args:  []string{"-o", "-z", "foo"},
			stdin: bytes.NewBufferString("foo bfoo"),
			want:  "foo\x00foo\x00",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunPrefixes(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "byte offset of the line",
			args:  []string{"-b", "foo"},
			stdin: bytes.NewBufferString("bar\nfoo\n"),
			want:  "4:foo\n",
		},
		{
			name:  "H forces the name of the standard input",
			args:  []string{"-H", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "(standard input):foo\n",
		},
		{
			name:  "H with more than one file",
			args:  []string{"-H", "foo", "a", "b"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}, "b": {Data: []byte("foo\n")}},
			want:  "a:foo\nb:foo\n",
		},
		{
			name:  "h suppresses the name",
			args:  []string{"-h", "foo", "a", "b"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}, "b": {Data: []byte("foo\n")}},
			want:  "foo\nfoo\n",
		},
		{
			name:  "label of the standard input",
			args:  []string{"-H", "--label=x.c", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "x.c:foo\n",
		},
		{
			name:  "initial tab",
			args:  []string{"-n", "-T", "foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}},
			want:  "1:\tfoo\n",
		},
		{
			name:  "initial tab between the fields",
			args:  []string{"-b", "-n", "-T", "foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}},
			want:  "1:0:\tfoo\n",
		},
		{
			// The numbers of an input of unknown size, like the standard
			// input, take the widest width, like GNU grep does.
			name:  "initial tab on the standard input",
			args:  []string{"-n", "-T", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "                   1:\tfoo\n",
		},
		{
			name:  "field order whatever the option order",
			args:  []string{"-b", "-n", "foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}},
			want:  "1:0:foo\n",
		},
		{
			name:  "null byte after the file name",
			args:  []string{"-Z", "foo", "a", "b"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}, "b": {Data: []byte("foo\n")}},
			want:  "a\x00foo\nb\x00foo\n",
		},
		{
			name:  "null byte after the name of a matching file",
			args:  []string{"-l", "-Z", "foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}},
			want:  "a\x00",
		},
		{
			name:  "null data lines",
			args:  []string{"-z", "foo"},
			stdin: bytes.NewBufferString("foo\nbar\x00foo"),
			want:  "foo\nbar\x00foo\x00",
		},
		{
			name:  "line numbers count the null data lines",
			args:  []string{"-z", "-n", "foo"},
			stdin: bytes.NewBufferString("bar\x00foo"),
			want:  "2:foo\x00",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunContext(t *testing.T) {
	input := "one\ntwo\nthree\nfour\nfive\n"
	tests := []runTestArgs{
		{
			name:  "after context",
			args:  []string{"-A", "1", "three"},
			stdin: bytes.NewBufferString(input),
			want:  "three\nfour\n",
		},
		{
			name:  "before context",
			args:  []string{"-B", "1", "three"},
			stdin: bytes.NewBufferString(input),
			want:  "two\nthree\n",
		},
		{
			name:  "context",
			args:  []string{"-C", "1", "three"},
			stdin: bytes.NewBufferString(input),
			want:  "two\nthree\nfour\n",
		},
		{
			name:  "old style count",
			args:  []string{"-2", "three"},
			stdin: bytes.NewBufferString(input),
			want:  "one\ntwo\nthree\nfour\nfive\n",
		},
		{
			name:  "adjacent groups merge",
			args:  []string{"-E", "-C", "1", "three|five"},
			stdin: bytes.NewBufferString(input),
			want:  "two\nthree\nfour\nfive\n",
		},
		{
			name:  "separator between groups",
			args:  []string{"-E", "-C", "1", "one|five"},
			stdin: bytes.NewBufferString(input),
			want:  "one\ntwo\n--\nfour\nfive\n",
		},
		{
			name:  "no line is written twice",
			args:  []string{"-E", "-C", "2", "two|three"},
			stdin: bytes.NewBufferString(input),
			want:  "one\ntwo\nthree\nfour\nfive\n",
		},
		{
			name:  "context lines are marked with a dash",
			args:  []string{"-E", "-n", "-C", "1", "one|five"},
			stdin: bytes.NewBufferString(input),
			want:  "1:one\n2-two\n--\n4-four\n5:five\n",
		},
		{
			name:  "group separator",
			args:  []string{"-E", "-C", "1", "--group-separator===", "one|five"},
			stdin: bytes.NewBufferString(input),
			want:  "one\ntwo\n==\nfour\nfive\n",
		},
		{
			name:  "no group separator",
			args:  []string{"-E", "-C", "1", "--no-group-separator", "one|five"},
			stdin: bytes.NewBufferString(input),
			want:  "one\ntwo\nfour\nfive\n",
		},
		{
			name:  "inverted context",
			args:  []string{"-v", "-n", "-C", "1", "three"},
			stdin: bytes.NewBufferString(input),
			want:  "1:one\n2:two\n3-three\n4:four\n5:five\n",
		},
		{
			name:  "context of the last line",
			args:  []string{"-A", "1", "five"},
			stdin: bytes.NewBufferString(input),
			want:  "five\n",
		},
		{
			name:  "context with the file name",
			args:  []string{"-C", "1", "foo", "a", "b"},
			files: fsx.MemFS{"a": {Data: []byte("x\nfoo\ny\n")}, "b": {Data: []byte("foo\n")}},
			want:  "a-x\na:foo\na-y\nb:foo\n",
		},
		{
			name:     "invalid context",
			args:     []string{"-A", "x", "foo"},
			wantErr:  "grep: invalid context length argument 'x'\nTry 'grep --help' for more information.\n",
			wantCode: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunBinary(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "default reports a match on the standard output",
			args:  []string{"foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\x00bar\n")}},
			want:  "Binary file a matches\n",
		},
		{
			name:  "text reads it as text",
			args:  []string{"-a", "foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\x00bar\n")}},
			want:  "foo\x00bar\n",
		},
		{
			name:  "text long option",
			args:  []string{"--binary-files=text", "foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\x00bar\n")}},
			want:  "foo\x00bar\n",
		},
		{
			name:     "without match",
			args:     []string{"-I", "foo", "a"},
			files:    fsx.MemFS{"a": {Data: []byte("foo\x00bar\n")}},
			wantCode: 1,
		},
		{
			name:     "without match long option",
			args:     []string{"--binary-files=without-match", "foo", "a"},
			files:    fsx.MemFS{"a": {Data: []byte("foo\x00bar\n")}},
			wantCode: 1,
		},
		{
			name:  "lines before the null byte are output",
			args:  []string{"foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n\x00bar\n")}},
			want:  "foo\n",
		},
		{
			name:  "count of a binary file keeps counting",
			args:  []string{"-c", "foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\x00foo\n")}},
			want:  "1\n",
		},
		{
			name:     "invalid binary type",
			args:     []string{"--binary-files=frob", "foo"},
			wantErr:  "grep: invalid argument 'frob' for '--binary-files'\nTry 'grep --help' for more information.\n",
			wantCode: 2,
		},
		{
			name:  "null data keeps the null bytes ordinary",
			args:  []string{"-z", "foo"},
			stdin: bytes.NewBufferString("foo\x00bar"),
			want:  "foo\x00",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunOptionW(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "word at the edges",
			args:  []string{"-w", "foo"},
			stdin: bytes.NewBufferString("foo bar\n"),
			want:  "foo bar\n",
		},
		{
			name:     "not inside a longer word",
			args:     []string{"-w", "bar"},
			stdin:    bytes.NewBufferString("foobar\n"),
			wantCode: 1,
		},
		{
			name:     "underscore is a word constituent",
			args:     []string{"-w", "a"},
			stdin:    bytes.NewBufferString("a_b\n"),
			wantCode: 1,
		},
		{
			name:  "digits and case",
			args:  []string{"-w", "-i", "x1"},
			stdin: bytes.NewBufferString("X1 y\n"),
			want:  "X1 y\n",
		},
		{
			name:  "non word patterns can still match",
			args:  []string{"-w", "@"},
			stdin: bytes.NewBufferString("a @ b\n"),
			want:  "a @ b\n",
		},
		{
			name:  "inverted",
			args:  []string{"-w", "-v", "foo"},
			stdin: bytes.NewBufferString("foo bar\nbaz\n"),
			want:  "baz\n",
		},
		{
			name:  "only the word matches are printed",
			args:  []string{"-w", "-o", "foo"},
			stdin: bytes.NewBufferString("foo foobar\n"),
			want:  "foo\n",
		},
		{
			name:  "x wins over w",
			args:  []string{"-x", "-w", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "foo\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunFilesWithoutMatch(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "files without a match",
			args:  []string{"-L", "foo", "a", "b"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}, "b": {Data: []byte("bar\n")}},
			want:  "b\n",
		},
		{
			name:     "standard input without a match",
			args:     []string{"-L", "baz"},
			stdin:    bytes.NewBufferString("foo\n"),
			want:     "(standard input)\n",
			wantCode: 1,
		},
		{
			name:  "a binary file with a match is not listed",
			args:  []string{"-L", "foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\x00\n")}},
		},
		{
			name:     "long option",
			args:     []string{"--files-without-match", "baz", "a"},
			files:    fsx.MemFS{"a": {Data: []byte("foo\n")}},
			want:     "a\n",
			wantCode: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

func TestRunColor(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		stdin    *bytes.Buffer
		files    fsx.MemFS
		environ  []string
		want     string
		wantErr  string
		wantCode int
	}{
		{
			name:  "always colors the match",
			args:  []string{"--color=always", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "\x1b[01;31m\x1b[Kfoo\x1b[m\x1b[K\n",
		},
		{
			name:  "plain color option means auto",
			args:  []string{"--color", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "foo\n",
		},
		{
			name:  "never",
			args:  []string{"--color=never", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "foo\n",
		},
		{
			name:  "auto stays off away from a terminal",
			args:  []string{"--color=auto", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "foo\n",
		},
		{
			name:  "colored prefixes and separators",
			args:  []string{"--color=always", "-nH", "foo", "a"},
			files: fsx.MemFS{"a": {Data: []byte("foo\n")}},
			want: "\x1b[35m\x1b[Ka\x1b[m\x1b[K" + "\x1b[36m\x1b[K:\x1b[m\x1b[K" +
				"\x1b[32m\x1b[K1\x1b[m\x1b[K" + "\x1b[36m\x1b[K:\x1b[m\x1b[K" +
				"\x1b[01;31m\x1b[Kfoo\x1b[m\x1b[K" + "\n",
		},
		{
			name:    "grep colors from the environment",
			args:    []string{"--color=always", "foo"},
			stdin:   bytes.NewBufferString("foo\n"),
			environ: []string{"GREP_COLORS=ms=04"},
			want:    "\x1b[04m\x1b[Kfoo\x1b[m\x1b[K\n",
		},
		{
			name:    "ne omits the clearing",
			args:    []string{"--color=always", "foo"},
			stdin:   bytes.NewBufferString("foo\n"),
			environ: []string{"GREP_COLORS=ms=01:ne"},
			want:    "\x1b[01mfoo\x1b[m\n",
		},
		{
			name:    "obsolete grep color variable",
			args:    []string{"--color=always", "foo"},
			stdin:   bytes.NewBufferString("foo\n"),
			environ: []string{"GREP_COLOR=07"},
			want:    "\x1b[07m\x1b[Kfoo\x1b[m\x1b[K\n",
		},
		{
			name:  "only the matched parts are colored",
			args:  []string{"--color=always", "b"},
			stdin: bytes.NewBufferString("abc\n"),
			want:  "a\x1b[01;31m\x1b[Kb\x1b[m\x1b[Kc\n",
		},
		{
			name:     "invalid when",
			args:     []string{"--color=frob", "foo"},
			wantErr:  "grep: invalid argument 'frob' for '--color'\nTry 'grep --help' for more information.\n",
			wantCode: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTestEnv(t, test.args, test.stdin, test.files, test.environ, test.want, test.wantErr, test.wantCode)
		})
	}
}

func TestRunOtherGNUOptions(t *testing.T) {
	tests := []runTestArgs{
		{
			name:  "y ignores case",
			args:  []string{"-y", "FOO"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "foo\n",
		},
		{
			name:     "no ignore case cancels i",
			args:     []string{"-i", "--no-ignore-case", "FOO"},
			stdin:    bytes.NewBufferString("foo\n"),
			wantCode: 1,
		},
		{
			name:  "G selects basic expressions",
			args:  []string{"-E", "-G", `\(a\)`},
			stdin: bytes.NewBufferString("a\n"),
			want:  "a\n",
		},
		{
			name:  "line buffered is accepted",
			args:  []string{"--line-buffered", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "foo\n",
		},
		{
			name:  "mmap is accepted",
			args:  []string{"--mmap", "foo"},
			stdin: bytes.NewBufferString("foo\n"),
			want:  "foo\n",
		},
		{
			name:  "-1 next to a valued option is a pattern",
			args:  []string{"-e", "-1"},
			stdin: bytes.NewBufferString("foo -1\n"),
			want:  "foo -1\n",
		},
		{
			name:  "attached context count",
			args:  []string{"-C2", "three"},
			stdin: bytes.NewBufferString("one\ntwo\nthree\nfour\nfive\n"),
			want:  "one\ntwo\nthree\nfour\nfive\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTest(t, test)
		})
	}
}

// runTestEnv runs grep with an environment and compares its output, its
// standard error and its exit status.
func runTestEnv(t *testing.T, args []string, stdin io.Reader, files fsx.MemFS, environ []string, want, wantErr string, wantCode int) {
	t.Helper()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	runtime := &coreutils.Runtime{
		Stdin: stdin, Stdout: stdout, Stderr: stderr,
		FS: files, Environ: func() []string { return environ },
	}
	err := New(runtime).Run(t.Context(), args...)
	code := 0
	if exitErr, ok := errors.AsType[*coreutils.ExitError](err); ok {
		code = exitErr.Code
	} else if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if code != wantCode {
		t.Errorf("Run() exit code = %d, want %d", code, wantCode)
	}
	if got := stdout.String(); got != want {
		t.Errorf("Run() stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, wantErr)
	}
}
