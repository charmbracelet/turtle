package sed

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

func stringsFS(files map[string]string) fsx.MemoryFS {
	fsys := fsx.MemoryFS{}
	for name, content := range files {
		fsys[name] = &fsx.MemoryFile{Data: []byte(content)}
	}
	return fsys
}

// runCases runs the cases like testutil.Run, giving an empty file system to
// the ones which set none, as sed needs one for its file operands and for
// the files the w commands create.
func runCases(t *testing.T, tests []testutil.Case) {
	t.Helper()

	for i := range tests {
		if tests[i].FS == nil {
			tests[i].FS = fsx.MemoryFS{}
		}
	}
	testutil.Run(t, New, tests)
}

func TestRunSubstitute(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the first occurrence is replaced",
			Stdin: strings.NewReader("banana\n"),
			Args:  []string{"s/an/-/"},
			Want:  "b-ana\n",
		},
		{
			Name:  "the g flag replaces every occurrence",
			Stdin: strings.NewReader("banana\n"),
			Args:  []string{"s/an/-/g"},
			Want:  "b--a\n",
		},
		{
			Name:  "a number flag replaces the nth occurrence",
			Stdin: strings.NewReader("banana\n"),
			Args:  []string{"s/an/-/2"},
			Want:  "ban-a\n",
		},
		{
			Name:  "the p flag writes the pattern space after a change",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"s/a/X/p"},
			Want:  "X\nX\nb\n",
		},
		{
			Name:  "the p flag writes nothing when nothing changed",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"s/z/X/p"},
			Want:  "a\nb\n",
		},
		{
			Name:  "the p flag writes with -n too",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-n", "s/a/X/p"},
			Want:  "X\n",
		},
		{
			Name:  "the i flag matches case insensitively",
			Stdin: strings.NewReader("Aa\n"),
			Args:  []string{"s/a/X/gi"},
			Want:  "XX\n",
		},
		{
			Name:  "the w flag appends changed lines to a file",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"s/a/X/w out"},
			Want:  "X\nb\n",
			WantFiles: map[string]string{
				"out": "X\n",
			},
		},
		{
			Name:  "the w flag writes nothing when nothing changed",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"s/z/X/w out"},
			Want:  "a\n",
			WantFiles: map[string]string{
				"out": "",
			},
		},
		{
			Name:  "an ampersand holds the whole match",
			Stdin: strings.NewReader("abc\n"),
			Args:  []string{"s/b/[&]/g"},
			Want:  "a[b]c\n",
		},
		{
			Name:  "an escaped ampersand is literal",
			Stdin: strings.NewReader("abc\n"),
			Args:  []string{"s/b/\\&/g"},
			Want:  "a&c\n",
		},
		{
			Name:  "backreferences hold the groups",
			Stdin: strings.NewReader("abc\n"),
			Args:  []string{`s/\(a\)\(b\)\(c\)/\3\2\1/`},
			Want:  "cba\n",
		},
		{
			Name:  "a missing group is empty",
			Stdin: strings.NewReader("ab\n"),
			Args:  []string{`s/b/\1x/`},
			Want:  "ax\n",
		},
		{
			Name:  "an escaped backreference is literal",
			Stdin: strings.NewReader("abc\n"),
			Args:  []string{`s/\(a\)/\\\1/`},
			Want:  `\a` + "bc\n",
		},
		{
			Name:  "an escaped newline splits the line",
			Stdin: strings.NewReader("ab\n"),
			Args:  []string{`s/b/\` + "\n" + `x/`},
			Want:  "a\nx\n",
		},
		{
			Name:  "a backslash before a plain character is dropped",
			Stdin: strings.NewReader("ab\n"),
			Args:  []string{`s/a/\x/`},
			Want:  "xb\n",
		},
		{
			Name:  "any character delimits the command",
			Stdin: strings.NewReader("a.b\n"),
			Args:  []string{"s,[.],X,"},
			Want:  "aXb\n",
		},
		{
			Name:  "the delimiter is escaped in the regular expression",
			Stdin: strings.NewReader("a/b\n"),
			Args:  []string{`s/a\/b/X/`},
			Want:  "X\n",
		},
		{
			Name:  "the delimiter is escaped in the replacement",
			Stdin: strings.NewReader("ab\n"),
			Args:  []string{`s/a/\/X/`},
			Want:  "/Xb\n",
		},
		{
			Name:  "an empty regular expression uses the last one",
			Stdin: strings.NewReader("abc\n"),
			Args:  []string{"/b/s//X/"},
			Want:  "aXc\n",
		},
		{
			Name:  "an empty regular expression falls back to the last s command",
			Stdin: strings.NewReader("abc\n"),
			Args:  []string{"s/c/Y/;s//Z/"},
			Want:  "abY\n",
		},
		{
			Name:  "an empty regular expression in an address is reused",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"/b/s/b/X/;//d"},
			Want:  "a\nX\n",
		},
		{
			Name:  "a substitution which changes nothing still counts",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"s/a/a/\ntX\n:X\ns/a/Y/"},
			Want:  "Y\n",
		},
		{
			Name:  "a pattern space may hold newlines",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"N;s/a/[/"},
			Want:  "[\nb\n",
		},
		{
			Name:  "the period doesn't match the newlines of the pattern space",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"N;s/a.*/X/"},
			Want:  "X\nb\n",
		},
		{
			Name:  "an escaped newline matches across the pattern space",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{`N;s/a\nb/X/`},
			Want:  "X\n",
		},
		{
			Name:  "the period doesn't match a joined newline",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{`N;s/^a.b/X/`},
			Want:  "a\nb\n",
		},
		{
			Name:  "no match keeps the pattern space",
			Stdin: strings.NewReader("abc\n"),
			Args:  []string{"s/z/X/g"},
			Want:  "abc\n",
		},
		{
			Name:  "a character class in a bracket expression matches",
			Stdin: strings.NewReader("a1b22\n"),
			Args:  []string{"s/[[:digit:]]/X/g"},
			Want:  "aXbXX\n",
		},
		{
			Name:  "a character class in a bracket expression matches with -E",
			Stdin: strings.NewReader("a1b22\n"),
			Args:  []string{"-E", "s/[[:digit:]]/X/g"},
			Want:  "aXbXX\n",
		},
		{
			// The members after the [:digit:] subexpression belong to the
			// bracket expression, so the delimiter inside it doesn't end
			// the regular expression.
			Name:  "a bracket expression may follow a class with members",
			Stdin: strings.NewReader("a1,b\n"),
			Args:  []string{"s,[[:digit:],],X,g"},
			Want:  "aXXb\n",
		},
		{
			Name:  "a close bracket first is a member",
			Stdin: strings.NewReader("a]b\n"),
			Args:  []string{"s/[]]/X/g"},
			Want:  "aXb\n",
		},
		{
			Name:  "a close bracket first is a member with -E",
			Stdin: strings.NewReader("a]b\n"),
			Args:  []string{"-E", "s/[]]/X/g"},
			Want:  "aXb\n",
		},
		{
			// The whole [[.a.]] is one bracket expression, matching the
			// collating element a.
			Name:  "a collating symbol scans as one bracket expression",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"s/[[.a.]]/X/"},
			Want:  "X\n",
		},
		{
			Name:  "the I flag matches case insensitively",
			Stdin: strings.NewReader("Aa\n"),
			Args:  []string{"s/a/X/gI"},
			Want:  "XX\n",
		},
		{
			// The M flag makes ^ match at the newlines the N command
			// joined the lines with.
			Name:  "the M flag matches at the newlines of the pattern space",
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			Args:  []string{"N;s/^b/X/M"},
			Want:  "a\nX\nc\nd\n",
		},
		{
			Name:  "the m flag matches at the newlines of the pattern space",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"N;s/^b/X/m"},
			Want:  "a\nX\n",
		},
		{
			Name:  "a number with the g flag replaces from the nth match on",
			Stdin: strings.NewReader("banana\n"),
			Args:  []string{"s/a/-/2g"},
			Want:  "ban-n-\n",
		},
		{
			Name:  "a number with the g flag beyond the matches changes nothing",
			Stdin: strings.NewReader("banana\n"),
			Args:  []string{"s/a/-/4g"},
			Want:  "banana\n",
		},
		{
			Name:  "\\U writes the rest of the replacement in upper case",
			Stdin: strings.NewReader("x\n"),
			Args:  []string{`s/x/\Uhe llo/`},
			Want:  "HE LLO\n",
		},
		{
			Name:  "\\L writes the rest of the replacement in lower case",
			Stdin: strings.NewReader("x\n"),
			Args:  []string{`s/x/\LHE LLO/`},
			Want:  "he llo\n",
		},
		{
			Name:  "\\E ends the case conversion",
			Stdin: strings.NewReader("x\n"),
			Args:  []string{`s/x/\Uhe\Ello/`},
			Want:  "HEllo\n",
		},
		{
			Name:  "\\u writes the next character in upper case",
			Stdin: strings.NewReader("x\n"),
			Args:  []string{`s/x/\uhello/`},
			Want:  "Hello\n",
		},
		{
			Name:  "\\l writes the next character in lower case",
			Stdin: strings.NewReader("x\n"),
			Args:  []string{`s/x/\lHELLO/`},
			Want:  "hELLO\n",
		},
		{
			Name:  "the case conversion applies to the whole match",
			Stdin: strings.NewReader("ab\n"),
			Args:  []string{`s/a./\U&/`},
			Want:  "AB\n",
		},
		{
			Name:  "the case conversion applies to a backreference",
			Stdin: strings.NewReader("ab\n"),
			Args:  []string{`s/\(a\)\(b\)/\U\2\l\1/`},
			Want:  "Ba\n",
		},
		{
			Name:     "the case conversion escapes are rejected with --posix",
			Stdin:    strings.NewReader("x\n"),
			Args:     []string{"--posix", `s/x/\Ux/`},
			WantErr:  "sed: the \\U escape of a replacement is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
	}
	runCases(t, tests)
}

func TestRunEscapedDelimiters(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "an escaped non-special delimiter matches itself",
			Stdin: strings.NewReader("a%b\n"),
			Args:  []string{`s%a\%b%X%`},
			Want:  "X\n",
		},
		{
			Name:  "an escaped non-special delimiter matches itself in an ERE",
			Stdin: strings.NewReader("a%b\n"),
			Args:  []string{"-E", `s%a\%b%X%`},
			Want:  "X\n",
		},
		{
			Name:  "an escaped punctuation delimiter is unescaped",
			Stdin: strings.NewReader("a_b\n"),
			Args:  []string{`s_a\_b_X_`},
			Want:  "X\n",
		},
		{
			// A backslash before a delimiter that isn't a special BRE
			// character must stand for the delimiter itself, so the pattern
			// is "a(b" and has no opened group.
			Name:  "an escaped parenthesis delimiter is literal in a BRE",
			Stdin: strings.NewReader("a(b\n"),
			Args:  []string{`s(a\(b(X(`},
			Want:  "X\n",
		},
		{
			Name:  "an escaped closing parenthesis delimiter is literal in a BRE",
			Stdin: strings.NewReader("a)b\n"),
			Args:  []string{`s)a\)b)X)`},
			Want:  "X\n",
		},
		{
			Name:  "an escaped non-special delimiter in an address matches itself",
			Stdin: strings.NewReader("a%b\nz\n"),
			Args:  []string{`\%a\%b%d`},
			Want:  "z\n",
		},
		{
			Name:  "an escaped dot delimiter keeps its escape",
			Stdin: strings.NewReader("x.y\n"),
			Args:  []string{`s.x\.y.X.`},
			Want:  "X\n",
		},
		{
			Name:  "an escaped dot delimiter keeps its escape in an ERE",
			Stdin: strings.NewReader("x.y\n"),
			Args:  []string{"-E", `s.x\.y.X.`},
			Want:  "X\n",
		},
		{
			Name:  "an escaped star delimiter keeps its escape",
			Stdin: strings.NewReader("a*b\n"),
			Args:  []string{`s*a\*b*X*`},
			Want:  "X\n",
		},
		{
			Name:  "an escaped caret delimiter keeps its escape",
			Stdin: strings.NewReader("a^b\n"),
			Args:  []string{`s^a\^b^X^`},
			Want:  "X\n",
		},
		{
			Name:  "an escaped plus delimiter keeps its escape in an ERE",
			Stdin: strings.NewReader("a+b\n"),
			Args:  []string{"-E", `s+a\+b+X+`},
			Want:  "X\n",
		},
		{
			Name:  "an escaped pound delimiter in an address keeps its escape",
			Stdin: strings.NewReader("a#b\nz\n"),
			Args:  []string{`\#a\#b#d`},
			Want:  "z\n",
		},
		{
			// The portable form of the context address /\.[0-9]/ with '.' as
			// the delimiter, from the APPLICATION USAGE of POSIX.
			Name:  "the portable dot-delimited address form works",
			Stdin: strings.NewReader("a.5\nfoo\n"),
			Args:  []string{`\.[.][0-9].d`},
			Want:  "foo\n",
		},
		{
			Name:  "the delimiter is literal inside a bracket expression",
			Stdin: strings.NewReader("a.b\n"),
			Args:  []string{`s.[.].X.`},
			Want:  "aXb\n",
		},
	}
	runCases(t, tests)
}

func TestRunSubstituteErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "a missing delimiter is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"s/a"},
			WantErr:  "sed: unterminated regular expression in s command: unterminated text\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown flag is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"s/a/X/q"},
			WantErr:  "sed: unknown flag in s command: q\n",
			WantExit: 2,
		},
		{
			Name:     "a missing delimiter is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"s"},
			WantErr:  "sed: missing delimiter for s command\n",
			WantExit: 2,
		},
		{
			Name:     "an empty regular expression without a previous one is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"s//X/"},
			WantErr:  "sed: no previous regular expression\n",
			WantExit: 2,
		},
		{
			Name:     "an invalid regular expression is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"s/[/X/"},
			WantErr:  "sed: unterminated regular expression in s command: unmatched [\n",
			WantExit: 2,
		},
	}
	runCases(t, tests)
}

func TestRunAddresses(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no address selects every line",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"d"},
			Want:  "",
		},
		{
			Name:  "a line number selects one line",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"2d"},
			Want:  "a\nc\n",
		},
		{
			Name:  "a line number selects nothing past the input",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"9d"},
			Want:  "a\nb\nc\n",
		},
		{
			Name:  "the dollar selects the last line",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"$d"},
			Want:  "a\nb\n",
		},
		{
			Name:  "a regular expression selects the matching lines",
			Stdin: strings.NewReader("ab\nba\ncc\n"),
			Args:  []string{"/a/d"},
			Want:  "cc\n",
		},
		{
			Name:  "a character class address selects the matching lines",
			Stdin: strings.NewReader("a\n1\nb\n2\n"),
			Args:  []string{"/[[:digit:]]/d"},
			Want:  "a\nb\n",
		},
		{
			Name:  "a character class address selects with -E",
			Stdin: strings.NewReader("a\n1\nb\n2\n"),
			Args:  []string{"-E", "/[[:digit:]]/d"},
			Want:  "a\nb\n",
		},
		{
			Name:  "a regular expression matches any part of the line",
			Stdin: strings.NewReader("xy\nyz\n"),
			Args:  []string{"/y/d"},
			Want:  "",
		},
		{
			Name:  "an anchored regular expression selects whole lines",
			Stdin: strings.NewReader("xy\nyz\n"),
			Args:  []string{"/^y/d"},
			Want:  "xy\n",
		},
		{
			Name:  "a custom delimiter is accepted",
			Stdin: strings.NewReader("a%b\nab\n"),
			Args:  []string{`\%[%]%d`},
			Want:  "ab\n",
		},
		{
			Name:  "an escaped delimiter stands for itself",
			Stdin: strings.NewReader("a/b\nb\n"),
			Args:  []string{`/a\/b/d`},
			Want:  "b\n",
		},
		{
			Name:  "a range selects from the first match to the second",
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			Args:  []string{"/b/,/c/d"},
			Want:  "a\nd\n",
		},
		{
			Name:  "a range restarts after it closes",
			Stdin: strings.NewReader("a\nb\nc\nb\nc\nd\n"),
			Args:  []string{"/b/,/c/d"},
			Want:  "a\nd\n",
		},
		{
			Name:  "a range of line numbers is inclusive",
			Stdin: strings.NewReader("1\n2\n3\n4\n"),
			Args:  []string{"2,3d"},
			Want:  "1\n4\n",
		},
		{
			Name:  "a range may mix numbers and regular expressions",
			Stdin: strings.NewReader("1\n2\n3\n4\n"),
			Args:  []string{"2,/^3$/d"},
			Want:  "1\n4\n",
		},
		{
			Name:  "a second number before the first selects one line",
			Stdin: strings.NewReader("1\n2\n3\n4\n"),
			Args:  []string{"3,2d"},
			Want:  "1\n2\n4\n",
		},
		{
			Name:  "a second number at the first line selects one line",
			Stdin: strings.NewReader("1\n2\n3\n4\n"),
			Args:  []string{"2,2d"},
			Want:  "1\n3\n4\n",
		},
		{
			Name:  "a second number below the first line selects one line",
			Stdin: strings.NewReader("1\n2\n3\n4\n"),
			Args:  []string{"2,0d"},
			Want:  "1\n3\n4\n",
		},
		{
			Name:  "a second pattern doesn't close on the opening line",
			Stdin: strings.NewReader("x\ny\nx\nz\n"),
			Args:  []string{"/x/,/x/d"},
			Want:  "z\n",
		},
		{
			Name: "a range without a later second match runs to the end",
			// With a single /x/ line, the range waits for a later match and
			// so selects through the end of the input.
			Stdin: strings.NewReader("a\nx\nb\n"),
			Args:  []string{"/x/,/x/d"},
			Want:  "a\n",
		},
		{
			Name: "a range restarts after a later second match",
			// After the range closes on the second /x/, sed looks again for
			// the first address, and doesn't find it on the last line.
			Stdin: strings.NewReader("a\nx\nb\nx\nc\n"),
			Args:  []string{"/x/,/x/d"},
			Want:  "a\nc\n",
		},
		{
			Name:  "a second number at or before the first selects one line",
			Stdin: strings.NewReader("1\n2\n3\n"),
			Args:  []string{"2,1d"},
			Want:  "1\n3\n",
		},
		{
			Name:  "a second number before the opening pattern line selects one line",
			Stdin: strings.NewReader("1\n2\n3\n4\n5\n"),
			Args:  []string{"/4/,3d"},
			Want:  "1\n2\n3\n5\n",
		},
		{
			Name:  "a range from a pattern to a later number is inclusive",
			Stdin: strings.NewReader("1\n2\n3\n4\n"),
			Args:  []string{"/2/,3d"},
			Want:  "1\n4\n",
		},
		{
			Name:  "a range without a second match runs to the end",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"/b/,$d"},
			Want:  "a\n",
		},
		{
			Name:  "the negation applies to a range",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"/b/,/c/!d"},
			Want:  "b\nc\n",
		},
		{
			Name:  "the negation applies to a single address",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1!d"},
			Want:  "a\n",
		},
		{
			Name:  "blanks may precede the address and the command",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{" 2d"},
			Want:  "a\n",
		},
		{
			Name:  "blank lines count as lines",
			Stdin: strings.NewReader("\n\n"),
			Args:  []string{"2d"},
			Want:  "\n",
		},
		// The GNU address extensions: first~step, +N, ~N and 0,/re/.
		{
			Name:  "a step address selects every step-th line",
			Stdin: strings.NewReader("1\n2\n3\n4\n5\n"),
			Args:  []string{"1~2d"},
			Want:  "2\n4\n",
		},
		{
			Name:  "a step address may start at zero",
			Stdin: strings.NewReader("1\n2\n3\n4\n5\n"),
			Args:  []string{"0~3d"},
			Want:  "1\n2\n4\n5\n",
		},
		{
			Name:  "a step address runs from the first line it names",
			Stdin: strings.NewReader("1\n2\n3\n4\n5\n"),
			Args:  []string{"3~3d"},
			Want:  "1\n2\n4\n5\n",
		},
		{
			Name:  "a range ends N lines after its first line",
			Stdin: strings.NewReader("1\n2\n3\n4\n5\n"),
			Args:  []string{"2,+2d"},
			Want:  "1\n5\n",
		},
		{
			Name:  "a range ends at a line N lines after a pattern",
			Stdin: strings.NewReader("a\nb\nc\nd\ne\n"),
			Args:  []string{"/c/,+1d"},
			Want:  "a\nb\ne\n",
		},
		{
			Name:  "a range ends at the next multiple of N",
			Stdin: strings.NewReader("1\n2\n3\n4\n5\n6\n"),
			Args:  []string{"2,~3d"},
			Want:  "1\n4\n5\n6\n",
		},
		{
			Name:  "a range ends at the next multiple past a multiple start",
			Stdin: strings.NewReader("1\n2\n3\n4\n5\n6\n"),
			Args:  []string{"3,~3d"},
			Want:  "1\n2\n",
		},
		{
			Name:  "a zero range ends on its first line when it matches",
			Stdin: strings.NewReader("x\ny\nx\n"),
			Args:  []string{"0,/x/d"},
			Want:  "y\nx\n",
		},
		{
			Name:  "a zero range ends at the first match after the first line",
			Stdin: strings.NewReader("a\nx\nb\n"),
			Args:  []string{"0,/x/d"},
			Want:  "b\n",
		},
		{
			// Unlike 0,/x/, the plain 1,/x/ range starts its search for
			// the end on the second line.
			Name:  "a one range runs past a match on its first line",
			Stdin: strings.NewReader("x\ny\nx\n"),
			Args:  []string{"1,/x/d"},
			Want:  "",
		},
		// The I and M modifiers of a regular expression address.
		{
			Name:  "the I modifier matches case insensitively",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"/A/Id"},
			Want:  "b\n",
		},
		{
			Name:  "the M modifier matches at the newlines of the pattern space",
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			Args:  []string{"N;/^b/Md"},
			Want:  "c\nd\n",
		},
		{
			Name:     "a missing address after a comma is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"2,d"},
			WantErr:  "sed: missing address after ,\n",
			WantExit: 2,
		},
		{
			Name:     "an unterminated regular expression address is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"/a,"},
			WantErr:  "sed: unterminated regular expression\n",
			WantExit: 2,
		},
		{
			Name:     "a step of zero is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1~0d"},
			WantErr:  "sed: step can't be 0\n",
			WantExit: 2,
		},
		{
			Name:     "a missing step after a tilde is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1~d"},
			WantErr:  "sed: missing step after ~\n",
			WantExit: 2,
		},
		{
			Name:     "a missing number after a plus is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1,+d"},
			WantErr:  "sed: missing number after +\n",
			WantExit: 2,
		},
		{
			Name:     "a missing number after a tilde is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1,~d"},
			WantErr:  "sed: missing number after ~\n",
			WantExit: 2,
		},
		{
			Name:     "a zero number after a tilde is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1,~0d"},
			WantErr:  "sed: the number after ~ must be positive\n",
			WantExit: 2,
		},
		{
			Name:     "a step address can't close a range",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1,2~3d"},
			WantErr:  "sed: a step address can't close a range\n",
			WantExit: 2,
		},
		{
			Name:     "a zero address alone is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"0d"},
			WantErr:  "sed: 0 address can only be the first of a range\n",
			WantExit: 2,
		},
		{
			Name:     "the step address is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "1~2d"},
			WantErr:  "sed: the first~step address is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		{
			Name:     "the +N address is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "1,+2d"},
			WantErr:  "sed: the addr1,+N address is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		{
			Name:     "the ~N address is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "1,~2d"},
			WantErr:  "sed: the addr1,~N address is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		{
			Name:     "the zero address is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "0,/x/d"},
			WantErr:  "sed: the 0 address is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		{
			Name:     "the I address modifier is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "/A/Id"},
			WantErr:  "sed: the I modifier of an address is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
	}
	runCases(t, tests)
}

func TestRunAddressLimits(t *testing.T) {
	tests := []testutil.Case{
		// Valid boundaries: the commands taking up to two addresses accept
		// none, one, or two.
		{
			Name:  "a 2addr command accepts two addresses",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"1,2d"},
			Want:  "c\n",
		},
		{
			Name:  "a 2addr command accepts one address",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1d"},
			Want:  "b\n",
		},
		{
			Name:  "a 2addr command accepts no address",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"d"},
			Want:  "",
		},
		{
			Name:  "a 2addr s command accepts two addresses",
			Stdin: strings.NewReader("a\nba\n"),
			Args:  []string{"1,2s/a/X/"},
			Want:  "X\nbX\n",
		},
		{
			Name:  "a 2addr y command accepts two addresses",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1,2y/a/X/"},
			Want:  "X\nb\n",
		},
		{
			Name:  "a 2addr { command accepts two addresses",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1,2{p;}"},
			Want:  "a\na\nb\nb\n",
		},
		{
			Name:     "a 2addr { command requires a separator before }",
			Stdin:    strings.NewReader("a\nb\n"),
			Args:     []string{"1,2{p}"},
			WantErr:  "sed: unexpected }\n",
			WantExit: 2,
		},
		{
			Name:  "a 2addr b command accepts two addresses",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1,2b\np"},
			Want:  "a\nb\n",
		},
		{
			Name:  "a 2addr c command accepts two addresses",
			Stdin: strings.NewReader("1\n2\n3\n"),
			Args:  []string{`1,2c\X`},
			Want:  "X\n3\n",
		},
		// Valid boundaries: the commands taking at most one address.
		{
			Name:  "a 1addr command accepts one address",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1q"},
			Want:  "a\n",
		},
		{
			Name:  "a 1addr command accepts no address",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"q"},
			Want:  "a\n",
		},
		{
			Name:  "a 1addr a command accepts one address",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{`1a\text`},
			Want:  "a\ntext\nb\n",
		},
		{
			Name:  "a 1addr i command accepts one address",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{`1i\text`},
			Want:  "text\na\nb\n",
		},
		{
			Name:  "a 1addr = command accepts one address",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1="},
			Want:  "1\na\nb\n",
		},
		{
			Name:  "a 1addr r command accepts one address",
			FS:    stringsFS(map[string]string{"r.txt": "FILE\n"}),
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1r r.txt"},
			Want:  "a\nFILE\nb\n",
		},
		// Valid boundaries: the commands taking no address at all.
		{
			Name:  "a 0addr : command accepts no address",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{":l\np"},
			Want:  "a\na\n",
		},
		{
			Name:  "a 0addr # command accepts no address",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"#comment\np"},
			Want:  "a\na\n",
		},
		// Violations of the limits, rejected like other script errors.
		{
			Name:     "a 1addr q command rejects two addresses",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1,2q"},
			WantErr:  "sed: too many addresses for command: q\n",
			WantExit: 2,
		},
		// Valid boundaries: the GNU extensions letting a, i, r and = take
		// a range. The text and files go out at the ends of the range.
		{
			Name:  "a 2addr a command appends after the range",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{`1,2a\text`},
			Want:  "a\nb\ntext\nc\n",
		},
		{
			Name:  "a 2addr i command inserts before the range",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{`1,2i\text`},
			Want:  "text\na\nb\nc\n",
		},
		{
			Name:  "a 2addr r command appends after the range",
			FS:    stringsFS(map[string]string{"r.txt": "FILE\n"}),
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"1,2r r.txt"},
			Want:  "a\nb\nFILE\nc\n",
		},
		{
			Name:  "a 2addr = command prints each line number",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"1,2="},
			Want:  "1\na\n2\nb\nc\n",
		},
		{
			Name:     "a 2addr a command is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", `1,2a\text`},
			WantErr:  "sed: too many addresses for command: a\n",
			WantExit: 2,
		},
		{
			Name:     "a 0addr : command rejects one address",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1:l\np"},
			WantErr:  "sed: too many addresses for command: :\n",
			WantExit: 2,
		},
		{
			Name:     "a 0addr # command rejects one address",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1#comment"},
			WantErr:  "sed: too many addresses for command: #\n",
			WantExit: 2,
		},
		{
			Name:     "a 0addr } command rejects one address",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1}"},
			WantErr:  "sed: too many addresses for command: }\n",
			WantExit: 2,
		},
		{
			Name:     "a 0addr : command rejects two addresses",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1,2:l\np"},
			WantErr:  "sed: too many addresses for command: :\n",
			WantExit: 2,
		},
	}
	runCases(t, tests)
}

func testRuntime(stdin io.Reader) *coreutils.Runtime {
	return &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
		FS:     fsx.MemoryFS{},
	}
}

// closeTrackingFS wraps a MemoryFS, failing to open one file and recording the
// names of the files closed through it.
type closeTrackingFS struct {
	fsx.MemoryFS
	fail   string
	closed *[]string
}

func (f *closeTrackingFS) OpenFile(name string, flag int, perm fs.FileMode) (fsx.File, error) {
	if name == f.fail {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	file, err := f.MemoryFS.OpenFile(name, flag, perm)
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
	fsys := &closeTrackingFS{MemoryFS: fsx.MemoryFS{}, fail: "bad", closed: &closed}
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
	tests := []testutil.Case{
		{
			Name: "a file is edited in place",
			FS:   stringsFS(map[string]string{"f.txt": "banana\n"}),
			Args: []string{"-i", "s/an/-/g", "f.txt"},
			Want: "",
			WantFiles: map[string]string{
				"f.txt": "b--a\n",
			},
		},
		{
			Name: "a missing trailing newline is kept",
			FS:   stringsFS(map[string]string{"f": "a\nb"}),
			Args: []string{"-i", "s/a/X/", "f"},
			WantFiles: map[string]string{
				"f": "X\nb",
			},
		},
		{
			Name: "a suffix preserves the original contents",
			FS:   stringsFS(map[string]string{"f.txt": "a\n"}),
			Args: []string{"-i.bak", "s/a/b/", "f.txt"},
			WantFiles: map[string]string{
				"f.txt":     "b\n",
				"f.txt.bak": "a\n",
			},
		},
		{
			Name: "a suffix attaches to a short option group",
			FS:   stringsFS(map[string]string{"f": "a\nb\n"}),
			Args: []string{"-nibak", "s/a/X/p", "f"},
			WantFiles: map[string]string{
				"f":    "X\n",
				"fbak": "a\nb\n",
			},
		},
		{
			Name: "the long form takes an attached suffix",
			FS:   stringsFS(map[string]string{"f.txt": "a\n"}),
			Args: []string{"--in-place=.suf", "s/a/b/", "f.txt"},
			WantFiles: map[string]string{
				"f.txt":     "b\n",
				"f.txt.suf": "a\n",
			},
		},
		{
			Name: "the long form without a value replaces the file",
			FS:   stringsFS(map[string]string{"f.txt": "a\n"}),
			Args: []string{"--in-place", "s/a/b/", "f.txt"},
			WantFiles: map[string]string{
				"f.txt": "b\n",
			},
		},
		{
			// GNU sed handles each file like a separate invocation, so the
			// line numbers start over for every one of them.
			Name: "each file starts a fresh script state",
			FS:   stringsFS(map[string]string{"one": "a\nb\n", "two": "c\nd\n"}),
			Args: []string{"-i", "=", "one", "two"},
			WantFiles: map[string]string{
				"one": "1\na\n2\nb\n",
				"two": "1\nc\n2\nd\n",
			},
		},
		{
			Name: "a file without any output is emptied",
			FS:   stringsFS(map[string]string{"f": "a\nb\n"}),
			Args: []string{"-i", "d", "f"},
			WantFiles: map[string]string{
				"f": "",
			},
		},
		{
			Name: "q quits one file but not the remaining ones",
			FS:   stringsFS(map[string]string{"one": "1\n2\n", "two": "3\n4\n"}),
			Args: []string{"-i", "1q", "one", "two"},
			WantFiles: map[string]string{
				"one": "1\n",
				"two": "3\n",
			},
		},
		{
			Name: "-n writes only what the script prints",
			FS:   stringsFS(map[string]string{"f": "a\nb\n"}),
			Args: []string{"-i", "-n", "s/a/X/p", "f"},
			WantFiles: map[string]string{
				"f": "X\n",
			},
		},
		{
			Name: "-E takes extended regular expressions",
			FS:   stringsFS(map[string]string{"f": "abc\n"}),
			Args: []string{"-i", "-E", "s/a+/X/", "f"},
			WantFiles: map[string]string{
				"f": "Xbc\n",
			},
		},
		{
			Name: "-e and -f parts join into the script",
			FS:   stringsFS(map[string]string{"f": "a\nb\n", "script.sed": "s/a/X/\n"}),
			Args: []string{"-i", "-f", "script.sed", "-e", "s/b/Y/", "f"},
			WantFiles: map[string]string{
				"f": "X\nY\n",
			},
		},
		{
			Name:     "stdin with -i is a usage error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-i", "s/a/b/"},
			WantErr:  "sed: -i may not be used with stdin\nTry 'sed --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a - operand with -i is a usage error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-i", "s/a/b/", "-"},
			WantErr:  "sed: -i may not be used with stdin\nTry 'sed --help' for more information.\n",
			WantExit: 2,
		},
		{
			// A file that fails doesn't keep the ones after it from
			// being edited.
			Name:     "a missing input file fails without touching it",
			FS:       stringsFS(map[string]string{"ok": "x\n"}),
			Args:     []string{"-i", "s/x/y/", "missing", "ok"},
			WantErr:  "sed: can't read missing: file does not exist\n",
			WantExit: 2,
			WantFiles: map[string]string{
				"ok": "y\n",
			},
		},
	}
	runCases(t, tests)
}

func TestRunInPlaceSystemFS(t *testing.T) {
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
			FS:         fsx.SystemFS(dir),
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
	files := fsx.MemoryFS{"f.txt": {Data: []byte("a\nb\n"), Mode: 0o600}}
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

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "sed", usage)
}

func TestRunInPlaceSuffixStar(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a star in the suffix is replaced with the file name",
			FS:   stringsFS(map[string]string{"f.txt": "a\n"}),
			Args: []string{"-i*.bak", "s/a/b/", "f.txt"},
			WantFiles: map[string]string{
				"f.txt":     "b\n",
				"f.txt.bak": "a\n",
			},
		},
		{
			// The star makes it easy to put the backup in another
			// directory, as GNU sed documents.
			Name: "a star in the suffix puts the backup in another directory",
			FS:   stringsFS(map[string]string{"f.txt": "a\n", "orig/keep": ""}),
			Args: []string{"-iorig/*", "s/a/b/", "f.txt"},
			WantFiles: map[string]string{
				"f.txt":      "b\n",
				"orig/f.txt": "a\n",
			},
		},
	}
	runCases(t, tests)
}

func TestRunInPlaceFollowSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink("target", link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	rt := &coreutils.Runtime{
		Stdin:      strings.NewReader(""),
		Stdout:     new(bytes.Buffer),
		Stderr:     new(bytes.Buffer),
		FS:         fsx.SystemFS(dir),
		WorkingDir: dir,
	}

	t.Run("the link is followed and kept", func(t *testing.T) {
		if err := New(rt).Run(t.Context(), "--follow-symlinks", "-i", "s/a/X/", "link"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), "X\nb\n"; got != want {
			t.Errorf("target = %q, want %q", got, want)
		}
		fi, err := os.Lstat(link)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("link is no longer a symlink: %v", fi.Mode())
		}
	})

	t.Run("without it, the link is replaced", func(t *testing.T) {
		if err := New(rt).Run(t.Context(), "-i", "s/X/Y/", "link"); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		fi, err := os.Lstat(link)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			t.Errorf("link is still a symlink: %v", fi.Mode())
		}
	})
}

func TestRunInPlaceNoBackup(t *testing.T) {
	// A suffix of just a star asks for no backup at all, leaving the file
	// as the only one in its directory, as GNU sed does.
	fsys := stringsFS(map[string]string{"f.txt": "a\n"})
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader(""),
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
		FS:     fsys,
	}
	if err := New(rt).Run(t.Context(), "-i*", "s/a/b/", "f.txt"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := string(fsys["f.txt"].Data), "b\n"; got != want {
		t.Errorf("f.txt = %q, want %q", got, want)
	}
	if len(fsys) != 1 {
		t.Errorf("directory holds %d files, want 1", len(fsys))
	}
}
