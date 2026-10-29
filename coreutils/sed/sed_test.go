package sed

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

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
	name string
	// stdin is the standard input, used when no files are given.
	stdin io.Reader
	// fsys holds the input files, and receives the files the script writes.
	fsys     fsx.MemFS
	dir      string
	args     []string
	want     string
	wantErr  string
	wantExit int
	// umask is the file mode creation mask of the runtime.
	umask fs.FileMode
	// wantFiles checks the files written by the w commands and the w flag of
	// the s command.
	wantFiles map[string]string
	// wantModes checks the modes of the files the script creates.
	wantModes map[string]fs.FileMode
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	fsys := test.fsys
	if fsys == nil {
		fsys = fsx.MemFS{}
	}
	rt := &coreutils.Runtime{
		Stdin:      test.stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         fsys,
		WorkingDir: test.dir,
		Umask:      test.umask,
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
	for name, want := range test.wantFiles {
		file, ok := fsys[name]
		if !ok {
			t.Errorf("Run() didn't create %s", name)
			continue
		}
		if got := string(file.Data); got != want {
			t.Errorf("Run() file %s = %q, want %q", name, got, want)
		}
	}
	for name, want := range test.wantModes {
		fi, err := fs.Stat(fsys, name)
		if err != nil {
			t.Errorf("Run() Stat(%s) error = %v", name, err)
			continue
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("Run() file %s mode = %v, want %v", name, got, want)
		}
	}
}

func stringsFS(files map[string]string) fsx.MemFS {
	fsys := fsx.MemFS{}
	for name, content := range files {
		fsys[name] = &fsx.MemFile{Data: []byte(content)}
	}
	return fsys
}

func TestRunSubstitute(t *testing.T) {
	tests := []testCase{
		{
			name:  "the first occurrence is replaced",
			stdin: strings.NewReader("banana\n"),
			args:  []string{"s/an/-/"},
			want:  "b-ana\n",
		},
		{
			name:  "the g flag replaces every occurrence",
			stdin: strings.NewReader("banana\n"),
			args:  []string{"s/an/-/g"},
			want:  "b--a\n",
		},
		{
			name:  "a number flag replaces the nth occurrence",
			stdin: strings.NewReader("banana\n"),
			args:  []string{"s/an/-/2"},
			want:  "ban-a\n",
		},
		{
			name:  "the p flag writes the pattern space after a change",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"s/a/X/p"},
			want:  "X\nX\nb\n",
		},
		{
			name:  "the p flag writes nothing when nothing changed",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"s/z/X/p"},
			want:  "a\nb\n",
		},
		{
			name:  "the p flag writes with -n too",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-n", "s/a/X/p"},
			want:  "X\n",
		},
		{
			name:  "the i flag matches case insensitively",
			stdin: strings.NewReader("Aa\n"),
			args:  []string{"s/a/X/gi"},
			want:  "XX\n",
		},
		{
			name:  "the w flag appends changed lines to a file",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"s/a/X/w out"},
			want:  "X\nb\n",
			wantFiles: map[string]string{
				"out": "X\n",
			},
		},
		{
			name:  "the w flag writes nothing when nothing changed",
			stdin: strings.NewReader("a\n"),
			args:  []string{"s/z/X/w out"},
			want:  "a\n",
			wantFiles: map[string]string{
				"out": "",
			},
		},
		{
			name:  "an ampersand holds the whole match",
			stdin: strings.NewReader("abc\n"),
			args:  []string{"s/b/[&]/g"},
			want:  "a[b]c\n",
		},
		{
			name:  "an escaped ampersand is literal",
			stdin: strings.NewReader("abc\n"),
			args:  []string{"s/b/\\&/g"},
			want:  "a&c\n",
		},
		{
			name:  "backreferences hold the groups",
			stdin: strings.NewReader("abc\n"),
			args:  []string{`s/\(a\)\(b\)\(c\)/\3\2\1/`},
			want:  "cba\n",
		},
		{
			name:  "a missing group is empty",
			stdin: strings.NewReader("ab\n"),
			args:  []string{`s/b/\1x/`},
			want:  "ax\n",
		},
		{
			name:  "an escaped backreference is literal",
			stdin: strings.NewReader("abc\n"),
			args:  []string{`s/\(a\)/\\\1/`},
			want:  `\a` + "bc\n",
		},
		{
			name:  "an escaped newline splits the line",
			stdin: strings.NewReader("ab\n"),
			args:  []string{`s/b/\` + "\n" + `x/`},
			want:  "a\nx\n",
		},
		{
			name:  "a backslash before a plain character is dropped",
			stdin: strings.NewReader("ab\n"),
			args:  []string{`s/a/\x/`},
			want:  "xb\n",
		},
		{
			name:  "any character delimits the command",
			stdin: strings.NewReader("a.b\n"),
			args:  []string{"s,[.],X,"},
			want:  "aXb\n",
		},
		{
			name:  "the delimiter is escaped in the regular expression",
			stdin: strings.NewReader("a/b\n"),
			args:  []string{`s/a\/b/X/`},
			want:  "X\n",
		},
		{
			name:  "the delimiter is escaped in the replacement",
			stdin: strings.NewReader("ab\n"),
			args:  []string{`s/a/\/X/`},
			want:  "/Xb\n",
		},
		{
			name:  "an empty regular expression uses the last one",
			stdin: strings.NewReader("abc\n"),
			args:  []string{"/b/s//X/"},
			want:  "aXc\n",
		},
		{
			name:  "an empty regular expression falls back to the last s command",
			stdin: strings.NewReader("abc\n"),
			args:  []string{"s/c/Y/;s//Z/"},
			want:  "abY\n",
		},
		{
			name:  "an empty regular expression in an address is reused",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"/b/s/b/X/;//d"},
			want:  "a\nX\n",
		},
		{
			name:  "a substitution which changes nothing still counts",
			stdin: strings.NewReader("a\n"),
			args:  []string{"s/a/a/\ntX\n:X\ns/a/Y/"},
			want:  "Y\n",
		},
		{
			name:  "a pattern space may hold newlines",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"N;s/a/[/"},
			want:  "[\nb\n",
		},
		{
			name:  "the period doesn't match the newlines of the pattern space",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"N;s/a.*/X/"},
			want:  "X\nb\n",
		},
		{
			name:  "an escaped newline matches across the pattern space",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{`N;s/a\nb/X/`},
			want:  "X\n",
		},
		{
			name:  "the period doesn't match a joined newline",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{`N;s/^a.b/X/`},
			want:  "a\nb\n",
		},
		{
			name:  "no match keeps the pattern space",
			stdin: strings.NewReader("abc\n"),
			args:  []string{"s/z/X/g"},
			want:  "abc\n",
		},
		{
			name:  "a character class in a bracket expression matches",
			stdin: strings.NewReader("a1b22\n"),
			args:  []string{"s/[[:digit:]]/X/g"},
			want:  "aXbXX\n",
		},
		{
			name:  "a character class in a bracket expression matches with -E",
			stdin: strings.NewReader("a1b22\n"),
			args:  []string{"-E", "s/[[:digit:]]/X/g"},
			want:  "aXbXX\n",
		},
		{
			// The members after the [:digit:] subexpression belong to the
			// bracket expression, so the delimiter inside it doesn't end
			// the regular expression.
			name:  "a bracket expression may follow a class with members",
			stdin: strings.NewReader("a1,b\n"),
			args:  []string{"s,[[:digit:],],X,g"},
			want:  "aXXb\n",
		},
		{
			name:  "a close bracket first is a member",
			stdin: strings.NewReader("a]b\n"),
			args:  []string{"s/[]]/X/g"},
			want:  "aXb\n",
		},
		{
			name:  "a close bracket first is a member with -E",
			stdin: strings.NewReader("a]b\n"),
			args:  []string{"-E", "s/[]]/X/g"},
			want:  "aXb\n",
		},
		{
			// The whole [[.a.]] is one bracket expression; Go's regexp
			// doesn't implement collating symbols, so matching it is left
			// to the class it parses to.
			name:  "a collating symbol scans as one bracket expression",
			stdin: strings.NewReader("a\n"),
			args:  []string{"s/[[.a.]]/X/"},
			want:  "a\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunEscapedDelimiters(t *testing.T) {
	tests := []testCase{
		{
			name:  "an escaped non-special delimiter matches itself",
			stdin: strings.NewReader("a%b\n"),
			args:  []string{`s%a\%b%X%`},
			want:  "X\n",
		},
		{
			name:  "an escaped non-special delimiter matches itself in an ERE",
			stdin: strings.NewReader("a%b\n"),
			args:  []string{"-E", `s%a\%b%X%`},
			want:  "X\n",
		},
		{
			name:  "an escaped punctuation delimiter is unescaped",
			stdin: strings.NewReader("a_b\n"),
			args:  []string{`s_a\_b_X_`},
			want:  "X\n",
		},
		{
			// A backslash before a delimiter that isn't a special BRE
			// character must stand for the delimiter itself, so the pattern
			// is "a(b" and has no opened group.
			name:  "an escaped parenthesis delimiter is literal in a BRE",
			stdin: strings.NewReader("a(b\n"),
			args:  []string{`s(a\(b(X(`},
			want:  "X\n",
		},
		{
			name:  "an escaped closing parenthesis delimiter is literal in a BRE",
			stdin: strings.NewReader("a)b\n"),
			args:  []string{`s)a\)b)X)`},
			want:  "X\n",
		},
		{
			name:  "an escaped non-special delimiter in an address matches itself",
			stdin: strings.NewReader("a%b\nz\n"),
			args:  []string{`\%a\%b%d`},
			want:  "z\n",
		},
		{
			name:  "an escaped dot delimiter keeps its escape",
			stdin: strings.NewReader("x.y\n"),
			args:  []string{`s.x\.y.X.`},
			want:  "X\n",
		},
		{
			name:  "an escaped dot delimiter keeps its escape in an ERE",
			stdin: strings.NewReader("x.y\n"),
			args:  []string{"-E", `s.x\.y.X.`},
			want:  "X\n",
		},
		{
			name:  "an escaped star delimiter keeps its escape",
			stdin: strings.NewReader("a*b\n"),
			args:  []string{`s*a\*b*X*`},
			want:  "X\n",
		},
		{
			name:  "an escaped caret delimiter keeps its escape",
			stdin: strings.NewReader("a^b\n"),
			args:  []string{`s^a\^b^X^`},
			want:  "X\n",
		},
		{
			name:  "an escaped plus delimiter keeps its escape in an ERE",
			stdin: strings.NewReader("a+b\n"),
			args:  []string{"-E", `s+a\+b+X+`},
			want:  "X\n",
		},
		{
			name:  "an escaped pound delimiter in an address keeps its escape",
			stdin: strings.NewReader("a#b\nz\n"),
			args:  []string{`\#a\#b#d`},
			want:  "z\n",
		},
		{
			// The portable form of the context address /\.[0-9]/ with '.' as
			// the delimiter, from the APPLICATION USAGE of POSIX.
			name:  "the portable dot-delimited address form works",
			stdin: strings.NewReader("a.5\nfoo\n"),
			args:  []string{`\.[.][0-9].d`},
			want:  "foo\n",
		},
		{
			name:  "the delimiter is literal inside a bracket expression",
			stdin: strings.NewReader("a.b\n"),
			args:  []string{`s.[.].X.`},
			want:  "aXb\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSubstituteErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "a missing delimiter is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"s/a"},
			wantErr:  "sed: unterminated regular expression in s command: unterminated text\n",
			wantExit: 2,
		},
		{
			name:     "an unknown flag is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"s/a/X/q"},
			wantErr:  "sed: unknown flag in s command: q\n",
			wantExit: 2,
		},
		{
			name:     "a missing delimiter is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"s"},
			wantErr:  "sed: missing delimiter for s command\n",
			wantExit: 2,
		},
		{
			name:     "an empty regular expression without a previous one is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"s//X/"},
			wantErr:  "sed: no previous regular expression\n",
			wantExit: 2,
		},
		{
			name:     "an invalid regular expression is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"s/[/X/"},
			wantErr:  "sed: unterminated regular expression in s command: unmatched [\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunAddresses(t *testing.T) {
	tests := []testCase{
		{
			name:  "no address selects every line",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"d"},
			want:  "",
		},
		{
			name:  "a line number selects one line",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"2d"},
			want:  "a\nc\n",
		},
		{
			name:  "a line number selects nothing past the input",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"9d"},
			want:  "a\nb\nc\n",
		},
		{
			name:  "the dollar selects the last line",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"$d"},
			want:  "a\nb\n",
		},
		{
			name:  "a regular expression selects the matching lines",
			stdin: strings.NewReader("ab\nba\ncc\n"),
			args:  []string{"/a/d"},
			want:  "cc\n",
		},
		{
			name:  "a character class address selects the matching lines",
			stdin: strings.NewReader("a\n1\nb\n2\n"),
			args:  []string{"/[[:digit:]]/d"},
			want:  "a\nb\n",
		},
		{
			name:  "a character class address selects with -E",
			stdin: strings.NewReader("a\n1\nb\n2\n"),
			args:  []string{"-E", "/[[:digit:]]/d"},
			want:  "a\nb\n",
		},
		{
			name:  "a regular expression matches any part of the line",
			stdin: strings.NewReader("xy\nyz\n"),
			args:  []string{"/y/d"},
			want:  "",
		},
		{
			name:  "an anchored regular expression selects whole lines",
			stdin: strings.NewReader("xy\nyz\n"),
			args:  []string{"/^y/d"},
			want:  "xy\n",
		},
		{
			name:  "a custom delimiter is accepted",
			stdin: strings.NewReader("a%b\nab\n"),
			args:  []string{`\%[%]%d`},
			want:  "ab\n",
		},
		{
			name:  "an escaped delimiter stands for itself",
			stdin: strings.NewReader("a/b\nb\n"),
			args:  []string{`/a\/b/d`},
			want:  "b\n",
		},
		{
			name:  "a range selects from the first match to the second",
			stdin: strings.NewReader("a\nb\nc\nd\n"),
			args:  []string{"/b/,/c/d"},
			want:  "a\nd\n",
		},
		{
			name:  "a range restarts after it closes",
			stdin: strings.NewReader("a\nb\nc\nb\nc\nd\n"),
			args:  []string{"/b/,/c/d"},
			want:  "a\nd\n",
		},
		{
			name:  "a range of line numbers is inclusive",
			stdin: strings.NewReader("1\n2\n3\n4\n"),
			args:  []string{"2,3d"},
			want:  "1\n4\n",
		},
		{
			name:  "a range may mix numbers and regular expressions",
			stdin: strings.NewReader("1\n2\n3\n4\n"),
			args:  []string{"2,/^3$/d"},
			want:  "1\n4\n",
		},
		{
			name:  "a second number before the first selects one line",
			stdin: strings.NewReader("1\n2\n3\n4\n"),
			args:  []string{"3,2d"},
			want:  "1\n2\n4\n",
		},
		{
			name:  "a second number at the first line selects one line",
			stdin: strings.NewReader("1\n2\n3\n4\n"),
			args:  []string{"2,2d"},
			want:  "1\n3\n4\n",
		},
		{
			name:  "a second number below the first line selects one line",
			stdin: strings.NewReader("1\n2\n3\n4\n"),
			args:  []string{"2,0d"},
			want:  "1\n3\n4\n",
		},
		{
			name:  "a second pattern doesn't close on the opening line",
			stdin: strings.NewReader("x\ny\nx\nz\n"),
			args:  []string{"/x/,/x/d"},
			want:  "z\n",
		},
		{
			name: "a range without a later second match runs to the end",
			// With a single /x/ line, the range waits for a later match and
			// so selects through the end of the input.
			stdin: strings.NewReader("a\nx\nb\n"),
			args:  []string{"/x/,/x/d"},
			want:  "a\n",
		},
		{
			name: "a range restarts after a later second match",
			// After the range closes on the second /x/, sed looks again for
			// the first address, and doesn't find it on the last line.
			stdin: strings.NewReader("a\nx\nb\nx\nc\n"),
			args:  []string{"/x/,/x/d"},
			want:  "a\nc\n",
		},
		{
			name:  "a second number at or before the first selects one line",
			stdin: strings.NewReader("1\n2\n3\n"),
			args:  []string{"2,1d"},
			want:  "1\n3\n",
		},
		{
			name:  "a second number before the opening pattern line selects one line",
			stdin: strings.NewReader("1\n2\n3\n4\n5\n"),
			args:  []string{"/4/,3d"},
			want:  "1\n2\n3\n5\n",
		},
		{
			name:  "a range from a pattern to a later number is inclusive",
			stdin: strings.NewReader("1\n2\n3\n4\n"),
			args:  []string{"/2/,3d"},
			want:  "1\n4\n",
		},
		{
			name:  "a range without a second match runs to the end",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"/b/,$d"},
			want:  "a\n",
		},
		{
			name:  "the negation applies to a range",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"/b/,/c/!d"},
			want:  "b\nc\n",
		},
		{
			name:  "the negation applies to a single address",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1!d"},
			want:  "a\n",
		},
		{
			name:  "blanks may precede the address and the command",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{" 2d"},
			want:  "a\n",
		},
		{
			name:  "blank lines count as lines",
			stdin: strings.NewReader("\n\n"),
			args:  []string{"2d"},
			want:  "\n",
		},
		{
			name:     "a missing address after a comma is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"2,d"},
			wantErr:  "sed: missing address after ,\n",
			wantExit: 2,
		},
		{
			name:     "an unterminated regular expression address is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"/a,"},
			wantErr:  "sed: unterminated regular expression\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunAddressLimits(t *testing.T) {
	tests := []testCase{
		// Valid boundaries: the commands taking up to two addresses accept
		// none, one, or two.
		{
			name:  "a 2addr command accepts two addresses",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"1,2d"},
			want:  "c\n",
		},
		{
			name:  "a 2addr command accepts one address",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1d"},
			want:  "b\n",
		},
		{
			name:  "a 2addr command accepts no address",
			stdin: strings.NewReader("a\n"),
			args:  []string{"d"},
			want:  "",
		},
		{
			name:  "a 2addr s command accepts two addresses",
			stdin: strings.NewReader("a\nba\n"),
			args:  []string{"1,2s/a/X/"},
			want:  "X\nbX\n",
		},
		{
			name:  "a 2addr y command accepts two addresses",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1,2y/a/X/"},
			want:  "X\nb\n",
		},
		{
			name:  "a 2addr { command accepts two addresses",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1,2{p;}"},
			want:  "a\na\nb\nb\n",
		},
		{
			name:     "a 2addr { command requires a separator before }",
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"1,2{p}"},
			wantErr:  "sed: unexpected }\n",
			wantExit: 2,
		},
		{
			name:  "a 2addr b command accepts two addresses",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1,2b\np"},
			want:  "a\nb\n",
		},
		{
			name:  "a 2addr c command accepts two addresses",
			stdin: strings.NewReader("1\n2\n3\n"),
			args:  []string{`1,2c\X`},
			want:  "X\n3\n",
		},
		// Valid boundaries: the commands taking at most one address.
		{
			name:  "a 1addr command accepts one address",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1q"},
			want:  "a\n",
		},
		{
			name:  "a 1addr command accepts no address",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"q"},
			want:  "a\n",
		},
		{
			name:  "a 1addr a command accepts one address",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{`1a\text`},
			want:  "a\ntext\nb\n",
		},
		{
			name:  "a 1addr i command accepts one address",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{`1i\text`},
			want:  "text\na\nb\n",
		},
		{
			name:  "a 1addr = command accepts one address",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1="},
			want:  "1\na\nb\n",
		},
		{
			name:  "a 1addr r command accepts one address",
			fsys:  stringsFS(map[string]string{"r.txt": "FILE\n"}),
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1r r.txt"},
			want:  "a\nFILE\nb\n",
		},
		// Valid boundaries: the commands taking no address at all.
		{
			name:  "a 0addr : command accepts no address",
			stdin: strings.NewReader("a\n"),
			args:  []string{":l\np"},
			want:  "a\na\n",
		},
		{
			name:  "a 0addr # command accepts no address",
			stdin: strings.NewReader("a\n"),
			args:  []string{"#comment\np"},
			want:  "a\na\n",
		},
		// Violations of the limits, rejected like other script errors.
		{
			name:     "a 1addr q command rejects two addresses",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1,2q"},
			wantErr:  "sed: too many addresses for command: q\n",
			wantExit: 2,
		},
		{
			name:     "a 1addr = command rejects two addresses",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1,2="},
			wantErr:  "sed: too many addresses for command: =\n",
			wantExit: 2,
		},
		{
			name:     "a 1addr a command rejects two addresses",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1,2a\\text"},
			wantErr:  "sed: too many addresses for command: a\n",
			wantExit: 2,
		},
		{
			name:     "a 1addr i command rejects two addresses",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1,2i\\text"},
			wantErr:  "sed: too many addresses for command: i\n",
			wantExit: 2,
		},
		{
			name:     "a 1addr r command rejects two addresses",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1,2r r.txt"},
			wantErr:  "sed: too many addresses for command: r\n",
			wantExit: 2,
		},
		{
			name:     "a 0addr : command rejects one address",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1:l\np"},
			wantErr:  "sed: too many addresses for command: :\n",
			wantExit: 2,
		},
		{
			name:     "a 0addr # command rejects one address",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1#comment"},
			wantErr:  "sed: too many addresses for command: #\n",
			wantExit: 2,
		},
		{
			name:     "a 0addr } command rejects one address",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1}"},
			wantErr:  "sed: too many addresses for command: }\n",
			wantExit: 2,
		},
		{
			name:     "a 0addr : command rejects two addresses",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1,2:l\np"},
			wantErr:  "sed: too many addresses for command: :\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func testRuntime(stdin io.Reader) *coreutils.Runtime {
	return &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
		FS:     fsx.MemFS{},
	}
}

// closeTrackingFS wraps a MemFS, failing to open one file and recording the
// names of the files closed through it.
type closeTrackingFS struct {
	fsx.MemFS
	fail   string
	closed *[]string
}

func (f *closeTrackingFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	if name == f.fail {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	file, err := f.MemFS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &closeTrackingFile{File: file, name: name, closed: f.closed}, nil
}

// closeTrackingFile records its closing in the list of the wrapping FS.
type closeTrackingFile struct {
	fsx.File
	name   string
	closed *[]string
}

func (f *closeTrackingFile) Close() error {
	*f.closed = append(*f.closed, f.name)
	return f.File.Close()
}

func TestRunClosesWritersWhenOpenFails(t *testing.T) {
	var closed []string
	fsys := &closeTrackingFS{MemFS: fsx.MemFS{}, fail: "bad", closed: &closed}
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader("a\n"),
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
		FS:     fsys,
	}
	if err := New(rt).Run(t.Context(), "1w out\n2w bad"); err == nil {
		t.Fatal("Run() error = nil, want failure creating bad")
	}
	if len(closed) != 1 || closed[0] != "out" {
		t.Errorf("files closed on failure = %v, want [out]", closed)
	}
}

func TestRunInPlace(t *testing.T) {
	tests := []testCase{
		{
			name: "a file is edited in place",
			fsys: stringsFS(map[string]string{"f.txt": "banana\n"}),
			args: []string{"-i", "s/an/-/g", "f.txt"},
			want: "",
			wantFiles: map[string]string{
				"f.txt": "b--a\n",
			},
		},
		{
			name: "a missing trailing newline is kept",
			fsys: stringsFS(map[string]string{"f": "a\nb"}),
			args: []string{"-i", "s/a/X/", "f"},
			wantFiles: map[string]string{
				"f": "X\nb",
			},
		},
		{
			name: "a suffix preserves the original contents",
			fsys: stringsFS(map[string]string{"f.txt": "a\n"}),
			args: []string{"-i.bak", "s/a/b/", "f.txt"},
			wantFiles: map[string]string{
				"f.txt":     "b\n",
				"f.txt.bak": "a\n",
			},
		},
		{
			name: "a suffix attaches to a short option group",
			fsys: stringsFS(map[string]string{"f": "a\nb\n"}),
			args: []string{"-nibak", "s/a/X/p", "f"},
			wantFiles: map[string]string{
				"f":    "X\n",
				"fbak": "a\nb\n",
			},
		},
		{
			name: "the long form takes an attached suffix",
			fsys: stringsFS(map[string]string{"f.txt": "a\n"}),
			args: []string{"--in-place=.suf", "s/a/b/", "f.txt"},
			wantFiles: map[string]string{
				"f.txt":     "b\n",
				"f.txt.suf": "a\n",
			},
		},
		{
			name: "the long form without a value replaces the file",
			fsys: stringsFS(map[string]string{"f.txt": "a\n"}),
			args: []string{"--in-place", "s/a/b/", "f.txt"},
			wantFiles: map[string]string{
				"f.txt": "b\n",
			},
		},
		{
			// GNU sed handles each file like a separate invocation, so the
			// line numbers start over for every one of them.
			name: "each file starts a fresh script state",
			fsys: stringsFS(map[string]string{"one": "a\nb\n", "two": "c\nd\n"}),
			args: []string{"-i", "=", "one", "two"},
			wantFiles: map[string]string{
				"one": "1\na\n2\nb\n",
				"two": "1\nc\n2\nd\n",
			},
		},
		{
			name: "a file without any output is emptied",
			fsys: stringsFS(map[string]string{"f": "a\nb\n"}),
			args: []string{"-i", "d", "f"},
			wantFiles: map[string]string{
				"f": "",
			},
		},
		{
			name: "q quits one file but not the remaining ones",
			fsys: stringsFS(map[string]string{"one": "1\n2\n", "two": "3\n4\n"}),
			args: []string{"-i", "1q", "one", "two"},
			wantFiles: map[string]string{
				"one": "1\n",
				"two": "3\n",
			},
		},
		{
			name: "-n writes only what the script prints",
			fsys: stringsFS(map[string]string{"f": "a\nb\n"}),
			args: []string{"-i", "-n", "s/a/X/p", "f"},
			wantFiles: map[string]string{
				"f": "X\n",
			},
		},
		{
			name: "-E takes extended regular expressions",
			fsys: stringsFS(map[string]string{"f": "abc\n"}),
			args: []string{"-i", "-E", "s/a+/X/", "f"},
			wantFiles: map[string]string{
				"f": "Xbc\n",
			},
		},
		{
			name: "-e and -f parts join into the script",
			fsys: stringsFS(map[string]string{"f": "a\nb\n", "script.sed": "s/a/X/\n"}),
			args: []string{"-i", "-f", "script.sed", "-e", "s/b/Y/", "f"},
			wantFiles: map[string]string{
				"f": "X\nY\n",
			},
		},
		{
			name:     "stdin with -i is a usage error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-i", "s/a/b/"},
			wantErr:  "sed: -i may not be used with stdin\n",
			wantExit: 2,
		},
		{
			name:     "a - operand with -i is a usage error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-i", "s/a/b/", "-"},
			wantErr:  "sed: -i may not be used with stdin\n",
			wantExit: 2,
		},
		{
			// A file that fails doesn't keep the ones after it from
			// being edited.
			name:     "a missing input file fails without touching it",
			fsys:     stringsFS(map[string]string{"ok": "x\n"}),
			args:     []string{"-i", "s/x/y/", "missing", "ok"},
			wantErr:  "sed: can't read missing: file does not exist\n",
			wantExit: 1,
			wantFiles: map[string]string{
				"ok": "y\n",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunInPlaceOsFS(t *testing.T) {
	// runOs edits f.txt in place, returning the error of the run and the
	// names the directory holds afterwards.
	runOs := func(t *testing.T, args ...string) (error, []string) {
		t.Helper()

		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		rt := &coreutils.Runtime{
			Stdin:      strings.NewReader(""),
			Stdout:     new(bytes.Buffer),
			Stderr:     new(bytes.Buffer),
			FS:         fsx.OsFS(dir),
			WorkingDir: dir,
		}
		err := New(rt).Run(t.Context(), args...)
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			t.Fatal(readErr)
		}
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		return err, names
	}

	t.Run("the file is replaced with the output", func(t *testing.T) {
		err, names := runOs(t, "-i.bak", "s/a/X/", "f.txt")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if want := []string{"f.txt", "f.txt.bak"}; !slicesEqual(names, want) {
			t.Errorf("directory = %v, want %v", names, want)
		}
	})

	t.Run("a failed run leaves no temporary file", func(t *testing.T) {
		err, names := runOs(t, "-i", "s/a/X/", "missing")
		if err == nil {
			t.Fatal("Run() error = nil, want failure reading missing")
		}
		if want := []string{"f.txt"}; !slicesEqual(names, want) {
			t.Errorf("directory = %v, want %v", names, want)
		}
	})
}

func TestRunInPlaceMode(t *testing.T) {
	files := fsx.MemFS{"f.txt": {Data: []byte("a\nb\n"), Mode: 0o600}}
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader(""),
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
		FS:     files,
	}

	if err := New(rt).Run(t.Context(), "-i", "s/a/X/", "f.txt"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	fi, err := fs.Stat(files, "f.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fi.Mode().Perm(), fs.FileMode(0o600); got != want {
		t.Errorf("f.txt mode = %v, want %v", got, want)
	}
}

// slicesEqual tells whether two lists hold the same members, in any order.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	slices := make([]string, len(b))
	copy(slices, b)
	for _, s := range a {
		found := false
		for i, t := range slices {
			if s == t {
				slices = append(slices[:i], slices[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
