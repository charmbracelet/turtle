package sed

import (
	"context"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

func TestRunTextCommands(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a writes text after the line",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{`/a/a\text`},
			Want:  "a\ntext\nb\n",
		},
		{
			Name:  "i writes text before the line",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{`/a/i\text`},
			Want:  "text\na\nb\n",
		},
		{
			Name:  "text may follow on the next line",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"2a\\\n  text"},
			Want:  "a\nb\ntext\n",
		},
		{
			Name:  "the first text line loses its leading blanks",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"2a\\\n  text"},
			Want:  "a\nb\ntext\n",
		},
		{
			Name:  "text continues on lines ending with a backslash",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"2a\\\none\\\ntwo"},
			Want:  "a\nb\none\ntwo\n",
		},
		{
			Name:  "an escaped backslash is literal in the text",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{`2a\one\\`},
			Want:  "a\nb\none\\\n",
		},
		{
			Name:  "c replaces the line",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{`2c\X`},
			Want:  "a\nX\nc\n",
		},
		{
			Name:  "c starts the next cycle",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"2c\\\nX\np"},
			Want:  "a\na\nX\nc\nc\n",
		},
		{
			Name:  "c writes the text only at the end of a range",
			Stdin: strings.NewReader("1\n2\n3\n4\n"),
			Args:  []string{"2,3c\\\nX"},
			Want:  "1\nX\n4\n",
		},
		{
			Name:  "c writes the text on every line a negated range selects",
			Stdin: strings.NewReader("1\n2\n3\n4\n"),
			Args:  []string{`2,3!c\X`},
			Want:  "X\n2\n3\nX\n",
		},
		{
			Name:  "c writes the text on the last line a negated range selects",
			Stdin: strings.NewReader("1\n2\n3\n4\n5\n"),
			Args:  []string{`3,4!c\X`},
			Want:  "X\nX\n3\n4\nX\n",
		},
		{
			Name:  "r copies a file before the next line",
			FS:    stringsFS(map[string]string{"r.txt": "FILE\n"}),
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1r r.txt"},
			Want:  "a\nFILE\nb\n",
		},
		{
			Name:  "r is written at the end of the script",
			FS:    stringsFS(map[string]string{"r.txt": "FILE\n"}),
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"1r r.txt"},
			Want:  "a\nFILE\n",
		},
		{
			Name:  "r is written before q quits",
			FS:    stringsFS(map[string]string{"r.txt": "FILE\n"}),
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1r r.txt\n2q"},
			Want:  "a\nFILE\nb\n",
		},
		{
			Name:  "a text is written before q writes the line",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1a\\TXT\n1q"},
			Want:  "TXT\na\n",
		},
		{
			Name:  "r is written before q writes the line",
			FS:    stringsFS(map[string]string{"r.txt": "FILE\n"}),
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1r r.txt\n1q"},
			Want:  "FILE\na\n",
		},
		{
			Name:     "a missing r file is empty",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1r missing.txt"},
			Want:     "a\n",
			WantExit: 0,
		},
		{
			Name:     "an r command without a filename is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1r"},
			WantErr:  "sed: filename expected for r command\n",
			WantExit: 2,
		},
		{
			Name:     "an r command with only blanks after it is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1r "},
			WantErr:  "sed: filename expected for r command\n",
			WantExit: 2,
		},
		{
			Name:     "a w command without a filename is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"1w"},
			WantErr:  "sed: filename expected for w command\n",
			WantExit: 2,
		},
		{
			Name:     "the w flag of s without a filename is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"s/a/X/w"},
			WantErr:  "sed: filename expected for the w flag of the s command\n",
			WantExit: 2,
		},
		{
			Name:  "r and a are written in the order applied",
			FS:    stringsFS(map[string]string{"r.txt": "FILE\n"}),
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"1a\\first\n1r r.txt\n1a\\last"},
			Want:  "a\nfirst\nFILE\nlast\n",
		},
		{
			Name: "a text scheduled before D waits for the next line read",
			// The D command restarts the cycle without reading a line, so the
			// text stays scheduled until N reads a line or the script ends.
			Stdin: strings.NewReader("a\ny\nz\n"),
			Args:  []string{"-e", "p", "-e", "N", "-e", `/y/a\TXT`, "-e", "D"},
			Want:  "a\ny\nTXT\nz\nTXT\n",
		},
		{
			Name: "a file that can't be read is treated as empty",
			// Opening a directory succeeds but reading it fails, which sed
			// reports as if the file were empty.
			FS:    fsx.MemFS{"r.txt": {Mode: fs.ModeDir | 0o755}},
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"1r r.txt"},
			Want:  "a\n",
		},
		{
			Name:  "text without a backslash is a GNU one-line form",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"1atext"},
			Want:  "a\ntext\n",
		},
		{
			Name:     "text without a backslash is an error with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "1atext"},
			WantErr:  "sed: text must start with \\\n",
			WantExit: 2,
		},
		{
			Name:  "a takes its text on the same line without a backslash",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"2a two words"},
			Want:  "a\nb\ntwo words\n",
		},
		{
			Name:  "i takes its text on the same line without a backslash",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"2i two words"},
			Want:  "a\ntwo words\nb\n",
		},
		{
			Name:  "c takes its text on the same line without a backslash",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"2c two words"},
			Want:  "a\ntwo words\nc\n",
		},
		{
			// The GNU form without a backslash also allows the text to
			// start on the next line.
			Name:  "a takes its text on the next line without a backslash",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"2a\ntext"},
			Want:  "a\nb\ntext\n",
		},
		{
			Name:  "the one-line text continues with an escaped newline",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"2a one\\\ntwo"},
			Want:  "a\nb\none\ntwo\n",
		},
	}
	runCases(t, tests)
}

func TestRunLineCommands(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "d deletes the line",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1d"},
			Want:  "b\n",
		},
		{
			Name:  "d starts a new cycle",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1d;p"},
			Want:  "b\nb\n",
		},
		{
			Name:  "D deletes up to the first newline",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"N;D"},
			Want:  "",
		},
		{
			Name:  "D restarts the cycle without reading",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"N;P;D"},
			Want:  "a\nb\n",
		},
		{
			Name:  "D works like d without a newline",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1D;2D"},
			Want:  "",
		},
		{
			Name:  "p writes the pattern space",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"p"},
			Want:  "a\na\n",
		},
		{
			Name:  "P writes up to the first newline",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"N;P"},
			Want:  "a\na\nb\n",
		},
		{
			Name:  "n writes and reads the next line",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"1n;d"},
			Want:  "a\n",
		},
		{
			Name:  "n doesn't write with -n",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-n", "n;p"},
			Want:  "b\n",
		},
		{
			Name:  "n quits at the end of the input",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"n;p"},
			Want:  "a\n",
		},
		{
			Name:  "N appends the next line",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"N;p"},
			Want:  "a\nb\na\nb\n",
		},
		{
			Name:  "N counts the lines",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"2N;$="},
			Want:  "a\n3\nb\nc\n",
		},
		{
			Name:  "N quits without output at the end of the input",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1N;2N"},
			Want:  "",
		},
		{
			Name:  "q quits the script",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"2q"},
			Want:  "a\nb\n",
		},
		{
			Name:  "q writes the pattern space",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"2q"},
			Want:  "a\nb\n",
		},
		{
			Name:  "q skips the commands after it",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"1q;d"},
			Want:  "a\n",
		},
		{
			Name:  "= writes the line number",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"="},
			Want:  "1\na\n2\nb\n",
		},
		{
			Name:  "= takes one address",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"$="},
			Want:  "a\nb\n3\nc\n",
		},
		{
			Name:  "the newline of the last line is preserved",
			Stdin: strings.NewReader("a\nb"),
			Args:  []string{""},
			Want:  "a\nb",
		},
		{
			Name:  "p preserves a missing newline",
			Stdin: strings.NewReader("a\nb"),
			Args:  []string{"$!d"},
			Want:  "b",
		},
		{
			Name:  "l writes the pattern space unambiguously",
			Stdin: strings.NewReader("a\tb\n"),
			Args:  []string{"l"},
			Want:  "a\\tb$\na\tb\n",
		},
		{
			Name:  "l escapes every special character",
			Stdin: strings.NewReader("\a\b\f\r\v\\\n"),
			Args:  []string{"l;d"},
			Want:  "\\a\\b\\f\\r\\v\\\\$\n",
		},
		{
			Name:  "l writes non-printable characters in octal",
			Stdin: strings.NewReader("\x01\x7f\n"),
			Args:  []string{"l;d"},
			Want:  "\\001\\177$\n",
		},
		{
			Name:  "l writes the newlines of the pattern space escaped",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"N;l;d"},
			Want:  "a\\nb$\n",
		},
		{
			Name:  "l folds long lines",
			Stdin: strings.NewReader(strings.Repeat("x", 75) + "\n"),
			Args:  []string{"l;d"},
			Want:  strings.Repeat("x", 70) + "\\\n" + strings.Repeat("x", 5) + "$\n",
		},
		{
			Name:  "l takes a wrap width as its argument",
			Stdin: strings.NewReader(strings.Repeat("x", 15) + "\n"),
			Args:  []string{"l10;d"},
			Want:  strings.Repeat("x", 10) + "\\\n" + strings.Repeat("x", 5) + "$\n",
		},
		{
			Name:  "l with a zero width doesn't fold",
			Stdin: strings.NewReader(strings.Repeat("x", 75) + "\n"),
			Args:  []string{"l0;d"},
			Want:  strings.Repeat("x", 75) + "$\n",
		},
		{
			Name:     "q takes an exit status",
			Stdin:    strings.NewReader("a\nb\nc\n"),
			Args:     []string{"2q5"},
			Want:     "a\nb\n",
			WantExit: 5,
		},
	}
	runCases(t, tests)
}

func TestRunHoldSpace(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "h replaces the hold space",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1h;1d;2G"},
			Want:  "b\na\n",
		},
		{
			Name:  "H appends to the hold space",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"1h;2H;$x;p"},
			Want:  "a\na\nb\nb\na\nb\na\nb\n",
		},
		{
			Name:  "g replaces the pattern space",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1h;2g"},
			Want:  "a\na\n",
		},
		{
			Name:  "G appends to the pattern space",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1h;2G"},
			Want:  "a\nb\na\n",
		},
		{
			Name:  "x exchanges the spaces",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1h;1d;2x;p"},
			Want:  "a\na\n",
		},
		{
			Name:  "the hold space starts empty",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"x;p"},
			Want:  "\n\n",
		},
		{
			Name:  "G on an empty hold space adds a newline",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"G"},
			Want:  "a\n\n",
		},
		{
			Name:  "hold and pattern spaces hold newlines",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-n", "N;h;x;p"},
			Want:  "a\nb\n",
		},
	}
	runCases(t, tests)
}

func TestRunTransliterate(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "y replaces every character",
			Stdin: strings.NewReader("abcabc\n"),
			Args:  []string{"y/abc/xyz/"},
			Want:  "xyzxyz\n",
		},
		{
			Name:  "y keeps the characters beyond string1",
			Stdin: strings.NewReader("abcd\n"),
			Args:  []string{"y/ab/XY/"},
			Want:  "XYcd\n",
		},
		{
			Name:  "y accepts a custom delimiter with escapes",
			Stdin: strings.NewReader("a,b\n"),
			Args:  []string{`y,a\,,X.,`},
			Want:  "X.b\n",
		},
		{
			Name:  "y resolves escapes",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{`N;y/a\n/X\//`},
			Want:  "X/b\n",
		},
		{
			Name:  "y translates multibyte characters",
			Stdin: strings.NewReader("ábc\n"),
			Args:  []string{`y/áé/ÁÉ/`},
			Want:  "Ábc\n",
		},
		{
			Name:     "y fails on different lengths",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"y/ab/X/"},
			WantErr:  "sed: y command strings have different lengths\n",
			WantExit: 2,
		},
		{
			Name:     "y fails on a repeated character",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"y/aa/XY/"},
			WantErr:  "sed: repeated character in y command\n",
			WantExit: 2,
		},
		{
			Name:     "y fails on an invalid escape",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{`y/\t/X/`},
			WantErr:  "sed: invalid escape in y command: \\t\n",
			WantExit: 2,
		},
		{
			Name:     "y fails when unterminated",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"y/a/X"},
			WantErr:  "sed: unterminated string in y command\n",
			WantExit: 2,
		},
	}
	runCases(t, tests)
}

func TestRunBlocksAndLabels(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a block groups commands",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"/a/{s/a/1/;s/^/>/;}"},
			Want:  ">1\nb\n",
		},
		{
			Name:  "a skipped block runs nothing",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"/b/{s/a/1/;s/^/>/;}"},
			Want:  "a\n>b\n",
		},
		{
			Name:  "blocks nest",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"/a/{/a/{s/a/X/;};}"},
			Want:  "X\nb\n",
		},
		{
			Name:  "a block may be negated",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1!{s/^/>/;}"},
			Want:  "a\n>b\n",
		},
		{
			Name:  "a block runs with two addresses",
			Stdin: strings.NewReader("1\n2\n3\n4\n"),
			Args:  []string{"2,3{s/^/>/;}"},
			Want:  "1\n>2\n>3\n4\n",
		},
		{
			Name:  "commands run after a block",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"/a/{p;};d"},
			Want:  "a\n",
		},
		{
			Name:  "b branches forward to a label",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-n", "/b/b end\ns/./X/\np\n:end"},
			Want:  "X\n",
		},
		{
			Name:  "b branches back to a label",
			Stdin: strings.NewReader("xxa\n"),
			Args:  []string{"-n", ":top\n/x/!b end\ns/x//\nb top\n:end\np"},
			Want:  "a\n",
		},
		{
			Name:  "b without a label goes to the end",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1b\ns/./X/"},
			Want:  "a\nX\n",
		},
		{
			Name:  "t branches after a substitution",
			Stdin: strings.NewReader("aa\nbb\n"),
			Args:  []string{"s/a/X/\ntl\ns/b/Y/\n:l"},
			Want:  "Xa\nYb\n",
		},
		{
			Name:  "t doesn't branch without a substitution",
			Stdin: strings.NewReader("cc\nbb\n"),
			Args:  []string{"s/a/X/\ntl\ns/b/Y/\n:l"},
			Want:  "cc\nYb\n",
		},
		{
			Name:  "t is reset when it runs",
			Stdin: strings.NewReader("ab\n"),
			Args:  []string{":l\ns/b//\ntl\ns/a/X/\n:l"},
			Want:  "X\n",
		},
		{
			Name:  "t is reset when a line is read",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"s/a/X/;n\nt\ns/b/Y/"},
			Want:  "X\nY\n",
		},
		{
			Name:  "t is reset when N reads a line",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-n", "s/a/X/\nN\ntX\np\n:X"},
			Want:  "X\nb\n",
		},
		{
			Name:  "t is not reset by D",
			Stdin: strings.NewReader("ab\nc\n"),
			Args:  []string{"-n", "tX\nN\ns/b//\nD\n:X\np"},
			Want:  "c\n",
		},
		{
			Name:  "a loop may process several lines",
			Stdin: strings.NewReader("1\n2\n3\n"),
			Args:  []string{"-n", ":l\np\nn\nbl"},
			Want:  "1\n2\n3\n",
		},
		{
			// The b, t and : labels extend to the end of the line, so a
			// semicolon doesn't end them.
			Name:  "labels ignore blanks",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{":l\n/z/b l\ns/./X/"},
			Want:  "X\n",
		},
		{
			// GNU sed ends a label at the first semicolon, so the d is a
			// command of its own rather than part of the label.
			Name:  "the command after a branch semicolon runs",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"2!b skip; d\n:skip"},
			Want:  "a\n",
		},
		{
			Name:  "a label ends at a semicolon",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{":l ; p\ns/./X/"},
			Want:  "a\nX\n",
		},
		{
			Name:  "comments are ignored",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"# comment\ns/a/X/"},
			Want:  "X\n",
		},
		{
			Name:  "a comment may follow commands",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"s/a/X/ # rest is ignored"},
			Want:  "X\n",
		},
		{
			Name:  "empty commands are ignored",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{";;s/a/X/;;"},
			Want:  "X\n",
		},
		{
			Name:  "commands are separated by semicolons",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"p;p"},
			Want:  "a\na\na\n",
		},
		{
			Name:     "a branch to an unknown label is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"bx"},
			WantErr:  "sed: label not found: x\n",
			WantExit: 2,
		},
		{
			Name:     "a t to an unknown label is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"tx"},
			WantErr:  "sed: label not found: x\n",
			WantExit: 2,
		},
		{
			Name:     "an unmatched open brace is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"{d"},
			WantErr:  "sed: unmatched {\n",
			WantExit: 2,
		},
		{
			Name:     "an unmatched close brace is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"d}"},
			WantErr:  "sed: unexpected }\n",
			WantExit: 2,
		},
		{
			Name:  "a close brace may follow a newline",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"/a/{d\n}"},
			Want:  "b\n",
		},
		{
			Name:  "a close brace may follow a semicolon",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"/a/{d;}"},
			Want:  "b\n",
		},
		{
			Name:  "blanks may precede a close brace",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"/a/{d\n }"},
			Want:  "b\n",
		},
		{
			Name:  "a close brace may start a line",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"/a/{d\n\n}"},
			Want:  "b\n",
		},
		{
			Name:  "close braces may be chained after semicolons",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"{{d;};}"},
			Want:  "",
		},
		{
			Name:     "a close brace right after a command is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"{p}"},
			WantErr:  "sed: unexpected }\n",
			WantExit: 2,
		},
		{
			Name:     "a close brace after chained commands is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"{p;q}"},
			WantErr:  "sed: unexpected }\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown command is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"Z"},
			WantErr:  "sed: unknown command: Z\n",
			WantExit: 2,
		},
	}
	runCases(t, tests)
}

func TestRunOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "-n suppresses the default output",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-n", "p"},
			Want:  "a\nb\n",
		},
		{
			Name:  "-n still writes explicit output",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-n", "s/a/X/p"},
			Want:  "X\n",
		},
		{
			Name:  "options may be grouped",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-n", "p"},
			Want:  "a\nb\n",
		},
		{
			Name:  "-e gives the script",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-e", "d"},
			Want:  "",
		},
		{
			Name:  "-e scripts run in order",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-e", "s/a/1/", "-e", "s/1/2/"},
			Want:  "2\n",
		},
		{
			Name:  "-e scripts are separated by newlines",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-e", "p", "-n"},
			Want:  "a\nb\n",
		},
		{
			Name:  "-f reads the script from a file",
			FS:    stringsFS(map[string]string{"scr": "s/a/X/\n"}),
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-f", "scr"},
			Want:  "X\n",
		},
		{
			Name:  "-f scripts may lack a trailing newline",
			FS:    stringsFS(map[string]string{"scr": "s/a/X/"}),
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-f", "scr"},
			Want:  "X\n",
		},
		{
			Name:  "-f is followed by -e",
			FS:    stringsFS(map[string]string{"scr": "s/a/1/;"}),
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-f", "scr", "-e", "s/1/2/"},
			Want:  "2\n",
		},
		{
			Name:  "-e is followed by -f with a newline between them",
			FS:    stringsFS(map[string]string{"scr": "s/1/2/"}),
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-e", "s/a/1/", "-f", "scr"},
			Want:  "2\n",
		},
		{
			Name:  "-e and -f run in the order given",
			FS:    stringsFS(map[string]string{"scr": "s/a/X/"}),
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-e", "s/X/1/", "-f", "scr"},
			Want:  "X\n",
		},
		{
			Name:  "the script operand is used without -e and -f",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"s/a/X/"},
			Want:  "X\nb\n",
		},
		{
			Name: "operands after the script name the files",
			FS:   stringsFS(map[string]string{"f1": "a\n", "f2": "b\n"}),
			Args: []string{"s/./X/", "f1", "f2"},
			Want: "X\nX\n",
		},
		{
			Name: "-- ends the options",
			FS:   stringsFS(map[string]string{"-n": "a\n"}),
			Args: []string{"--", "s/a/X/", "-n"},
			Want: "X\n",
		},
		{
			Name:  "a #n script suppresses the default output",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"#n\np"},
			Want:  "a\nb\n",
		},
		{
			Name:  "a -e script may start with #n",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"-e", "#n", "-e", "p"},
			Want:  "a\nb\n",
		},
		{
			Name:  "-E matches with extended regular expressions",
			Stdin: strings.NewReader("aab\n"),
			Args:  []string{"-E", "s/a+b/X/"},
			Want:  "X\n",
		},
		{
			Name:  "-E makes the BRE syntax ordinary",
			Stdin: strings.NewReader("a(b\n"),
			Args:  []string{"-E", `s/\(b/X/`},
			Want:  "aX\n",
		},
		{
			Name:     "no script is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-n"},
			WantErr:  "sed: missing script\nTry 'sed --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "no arguments is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{},
			WantErr:  "sed: missing script\nTry 'sed --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown option is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-Y", "d"},
			WantErr:  "sed: invalid option -- 'Y'\nTry 'sed --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "-e without an argument is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-e"},
			WantErr:  "sed: option requires an argument -- 'e'\nTry 'sed --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "-f without an argument is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-f"},
			WantErr:  "sed: option requires an argument -- 'f'\nTry 'sed --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a missing script file is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-f", "missing"},
			WantErr:  "sed: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			// An empty -f option-argument is a file name, not an empty
			// script, so opening it fails.
			Name:     "an empty -f file name is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-f", ""},
			WantErr:  "sed: : invalid argument\n",
			WantExit: 1,
		},
		{
			// An empty -e option-argument is an empty script.
			Name:  "an empty -e script is allowed",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-e", ""},
			Want:  "a\n",
		},
	}
	runCases(t, tests)
}

func TestRunFiles(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "files are read in order",
			FS:   stringsFS(map[string]string{"b.txt": "b\n", "a.txt": "a\n"}),
			Args: []string{"d", "a.txt", "b.txt"},
			Want: "",
		},
		{
			Name: "line numbers count across files",
			FS:   stringsFS(map[string]string{"a.txt": "a\nb\n", "b.txt": "c\nd\n"}),
			Args: []string{"3d", "a.txt", "b.txt"},
			Want: "a\nb\nd\n",
		},
		{
			Name: "$ addresses the last line of all files",
			FS:   stringsFS(map[string]string{"a.txt": "a\nb\n", "b.txt": "c\n"}),
			Args: []string{"$d", "a.txt", "b.txt"},
			Want: "a\nb\n",
		},
		{
			Name: "an empty file contributes no lines",
			FS:   stringsFS(map[string]string{"a.txt": "", "b.txt": "b\n"}),
			Args: []string{"1d", "a.txt", "b.txt"},
			Want: "",
		},
		{
			Name: "a missing last newline is kept",
			FS:   stringsFS(map[string]string{"a.txt": "a", "b.txt": "b\n"}),
			Args: []string{"", "a.txt", "b.txt"},
			Want: "ab\n",
		},
		{
			Name:  "- is the standard input",
			FS:    stringsFS(map[string]string{"a.txt": "a\n"}),
			Stdin: strings.NewReader("s\n"),
			Args:  []string{"d", "a.txt", "-", "a.txt"},
			Want:  "",
		},
		{
			Name: "the working directory is honored",
			FS:   stringsFS(map[string]string{"sub/a.txt": "a\n"}),
			Dir:  "sub",
			Args: []string{"d", "a.txt"},
			Want: "",
		},
		{
			Name:     "a missing file is an error",
			FS:       stringsFS(map[string]string{"a.txt": "a\n"}),
			Args:     []string{"d", "a.txt", "missing.txt"},
			Want:     "",
			WantErr:  "sed: can't read missing.txt: file does not exist\n",
			WantExit: 2,
		},
		{
			// The lookahead that tells whether a line is the last one is
			// the first read to hit the missing file, so the lines read
			// before it must still be processed.
			Name:     "lines read before a missing file are kept",
			FS:       stringsFS(map[string]string{"a.txt": "a\nb\n"}),
			Args:     []string{"p", "a.txt", "missing.txt"},
			Want:     "a\na\nb\nb\n",
			WantErr:  "sed: can't read missing.txt: file does not exist\n",
			WantExit: 2,
		},
		{
			// A file that can't be opened doesn't keep the ones after it
			// from being processed, and the exit status is 2.
			Name:     "the files after a missing one are still processed",
			FS:       stringsFS(map[string]string{"a.txt": "a\n", "b.txt": "b\n"}),
			Args:     []string{"p", "a.txt", "missing.txt", "b.txt"},
			Want:     "a\na\nb\nb\n",
			WantErr:  "sed: can't read missing.txt: file does not exist\n",
			WantExit: 2,
		},
		{
			Name: "w appends to a file",
			FS:   stringsFS(map[string]string{"in": "a\nb\n"}),
			Args: []string{"-n", "1w out", "in"},
			Want: "",
			WantFiles: map[string]string{
				"out": "a\n",
			},
		},
		{
			Name: "w truncates an existing file",
			FS:   stringsFS(map[string]string{"in": "a\nb\n", "out": "OLD\n"}),
			Args: []string{"-n", "1w out", "in"},
			WantFiles: map[string]string{
				"out": "a\n",
			},
		},
		{
			Name: "two w commands may share a file",
			FS:   stringsFS(map[string]string{"in": "a\nb\n"}),
			Args: []string{"-n", "1w out\n2w out", "in"},
			WantFiles: map[string]string{
				"out": "a\nb\n",
			},
		},
		{
			Name: "w writes the whole pattern space",
			FS:   stringsFS(map[string]string{"in": "a\nb\n"}),
			Args: []string{"-n", "N;w out", "in"},
			WantFiles: map[string]string{
				"out": "a\nb\n",
			},
		},
		{
			Name: "a w file is created with the default mode",
			FS:   stringsFS(map[string]string{"in": "a\n"}),
			Args: []string{"-n", "1w out", "in"},
			WantFiles: map[string]string{
				"out": "a\n",
			},
			WantMode: map[string]fs.FileMode{"out": 0o666},
		},
		{
			Name:  "a w file respects the umask",
			FS:    stringsFS(map[string]string{"in": "a\n"}),
			Umask: 0o022,
			Args:  []string{"-n", "1w out", "in"},
			WantFiles: map[string]string{
				"out": "a\n",
			},
			WantMode: map[string]fs.FileMode{"out": 0o644},
		},
		{
			Name:     "a w file which can't be created is an error",
			FS:       stringsFS(map[string]string{"in": "a\n"}),
			Args:     []string{"-n", "w nodir/out", "in"},
			WantErr:  "sed: can't create nodir/out: file does not exist\n",
			WantExit: 1,
		},
	}
	runCases(t, tests)
}

// delayedReader waits for its context to be done before returning its data
// once, so that the context expires between the lines of the input no matter
// how fast the script runs.
type delayedReader struct {
	ctx  context.Context
	data string
	sent bool
}

func (r *delayedReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, io.EOF
	}
	<-r.ctx.Done()
	r.sent = true
	return copy(p, r.data), nil
}

func TestRunContext(t *testing.T) {
	t.Run("a canceled context is returned as is", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		rt := testRuntime(strings.NewReader("a\nb\n"))
		err := New(rt).Run(ctx, "d")
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want %v", err, context.Canceled)
		}
	})

	t.Run("a context canceled while reading is returned", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		stdin := &testutil.CancelReader{Cancel: cancel}

		rt := testRuntime(stdin)
		err := New(rt).Run(ctx, "d")
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want %v", err, context.Canceled)
		}
	})

	t.Run("a context canceled between lines is returned", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()

		rt := testRuntime(&delayedReader{ctx: ctx, data: "a\nb\n"})
		err := New(rt).Run(ctx, "d")
		if err != context.DeadlineExceeded {
			t.Errorf("Run() error = %v, want %v", err, context.DeadlineExceeded)
		}
	})
}

func TestRunGNUCommands(t *testing.T) {
	tests := []testutil.Case{
		// The T command branches when no substitution happened.
		{
			Name:  "T branches without a substitution",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"s/b/X/\nTl\ns/./Y/\n:l"},
			Want:  "a\nY\n",
		},
		{
			Name:  "T doesn't branch after a substitution",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"s/a/X/\nTl\ns/./Y/\n:l"},
			Want:  "Y\n",
		},
		{
			// The first T only takes the flag away, so the second one
			// finds no substitution and branches, skipping the d.
			Name:  "T resets the substitution flag when it doesn't branch",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"s/a/x/;T done;T done;d;:done"},
			Want:  "x\n",
		},
		{
			// The same flag is what the t looks at, so the T which took it
			// away keeps the t from branching too.
			Name:  "t doesn't branch after a T took the flag away",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"s/a/x/\nTl\ntl\nd\n:l"},
			Want:  "",
		},
		{
			Name:     "T to an unknown label is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"Tx"},
			WantErr:  "sed: label not found: x\n",
			WantExit: 2,
		},
		{
			Name:     "the T command is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "Tl"},
			WantErr:  "sed: the T command is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		// The Q command quits without writing the pattern space.
		{
			Name:  "Q quits without writing the pattern space",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1Q"},
			Want:  "",
		},
		{
			Name:  "Q writes the text scheduled before it",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1a\\text\n1Q"},
			Want:  "text\n",
		},
		{
			Name:     "Q takes an exit status",
			Stdin:    strings.NewReader("a\nb\n"),
			Args:     []string{"1Q5"},
			Want:     "",
			WantExit: 5,
		},
		{
			Name:     "an out of range exit status is an error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"q256"},
			WantErr:  "sed: exit status out of range\n",
			WantExit: 2,
		},
		{
			Name:     "the Q command is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "1Q"},
			WantErr:  "sed: the Q command is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		// The z command empties the pattern space.
		{
			Name:  "z empties the pattern space",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"1z"},
			Want:  "\nb\n",
		},
		{
			Name:     "the z command is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "z"},
			WantErr:  "sed: the z command is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		// The F command writes the name of the current input file.
		{
			Name: "F writes the name of the current file",
			FS:   stringsFS(map[string]string{"a.txt": "a\n", "b.txt": "b\n"}),
			Args: []string{"F", "a.txt", "b.txt"},
			Want: "a.txt\na\nb.txt\nb\n",
		},
		{
			Name:  "F writes - for the standard input",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"F"},
			Want:  "-\na\n",
		},
		{
			Name:     "the F command is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "F"},
			WantErr:  "sed: the F command is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		// The W command writes the first line of the pattern space.
		{
			Name: "W writes the first line of the pattern space",
			FS:   stringsFS(map[string]string{"in": "a\nb\nc\n"}),
			Args: []string{"-n", "N;W out", "in"},
			Want: "",
			WantFiles: map[string]string{
				"out": "a\n",
			},
		},
		{
			Name: "W writes the first line of a joined pattern space",
			FS:   stringsFS(map[string]string{"in": "a\nb\nc\n"}),
			Args: []string{"-n", "2N;3W out", "in"},
			WantFiles: map[string]string{
				"out": "b\n",
			},
		},
		{
			Name:     "the W command is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "W out"},
			WantErr:  "sed: the W command is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		// The R command queues one line of a file each time it runs.
		{
			Name: "R queues one line of the file per cycle",
			FS:   stringsFS(map[string]string{"r.txt": "l1\nl2\n", "in": "a\nb\nc\n"}),
			Args: []string{"-n", "R r.txt", "in"},
			Want: "l1\nl2\n",
		},
		{
			Name: "R keeps reading where it left off",
			FS:   stringsFS(map[string]string{"r.txt": "l1\nl2\nl3\n", "in": "a\nb\nc\n"}),
			Args: []string{"-n", "R r.txt", "in"},
			Want: "l1\nl2\nl3\n",
		},
		{
			Name: "R writes after the pattern space of the cycle",
			FS:   stringsFS(map[string]string{"r.txt": "l1\n", "in": "a\nb\n"}),
			Args: []string{"R r.txt", "in"},
			Want: "a\nl1\nb\n",
		},
		{
			Name: "a missing R file has no lines",
			FS:   stringsFS(map[string]string{"in": "a\nb\n"}),
			Args: []string{"R missing.txt", "in"},
			Want: "a\nb\n",
		},
		{
			Name:     "the R command is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "R r.txt"},
			WantErr:  "sed: the R command is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		// The e command runs a shell, which the runtime doesn't provide.
		{
			Name:     "the e command is unknown",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"e"},
			WantErr:  "sed: unknown command: e\n",
			WantExit: 2,
		},
	}
	runCases(t, tests)
}

func TestRunStdStreamNames(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "w writes to /dev/stdout",
			FS:   stringsFS(map[string]string{"in": "a\nb\n"}),
			Args: []string{"-n", "1w /dev/stdout", "in"},
			Want: "a\n",
		},
		{
			Name:    "w writes to /dev/stderr",
			FS:      stringsFS(map[string]string{"in": "a\nb\n"}),
			Args:    []string{"-n", "1w /dev/stderr", "in"},
			WantErr: "a\n",
		},
		{
			Name: "the w flag of s writes to /dev/stdout",
			FS:   stringsFS(map[string]string{"in": "a\nb\n"}),
			Args: []string{"-n", "s/a/X/w /dev/stdout", "in"},
			Want: "X\n",
		},
		{
			Name: "W writes to /dev/stdout",
			FS:   stringsFS(map[string]string{"in": "a\nb\nc\n"}),
			Args: []string{"-n", "N;W /dev/stdout", "in"},
			Want: "a\n",
		},
		{
			Name:  "r reads from /dev/stdin",
			FS:    stringsFS(map[string]string{"in": "a\nb\n"}),
			Stdin: strings.NewReader("EXTRA\n"),
			Args:  []string{"1r /dev/stdin", "in"},
			Want:  "a\nEXTRA\nb\n",
		},
	}
	runCases(t, tests)
}

func TestRunSeparate(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "-s restarts the line numbers for each file",
			FS:   stringsFS(map[string]string{"a.txt": "a\nb\n", "b.txt": "c\nd\n"}),
			Args: []string{"-s", "=", "a.txt", "b.txt"},
			Want: "1\na\n2\nb\n1\nc\n2\nd\n",
		},
		{
			Name: "the line numbers continue without -s",
			FS:   stringsFS(map[string]string{"a.txt": "a\nb\n", "b.txt": "c\nd\n"}),
			Args: []string{"=", "a.txt", "b.txt"},
			Want: "1\na\n2\nb\n3\nc\n4\nd\n",
		},
		{
			Name: "-s takes $ as the last line of each file",
			FS:   stringsFS(map[string]string{"a.txt": "a\nb\n", "b.txt": "c\nd\n"}),
			Args: []string{"-s", "$d", "a.txt", "b.txt"},
			Want: "a\nc\n",
		},
		{
			Name: "-s restarts the ranges for each file",
			FS:   stringsFS(map[string]string{"one": "a\nb\nc\n", "two": "a\nb\n"}),
			Args: []string{"-s", "/a/,/b/d", "one", "two"},
			Want: "c\n",
		},
		{
			// GNU sed empties the hold space as well as the line counter
			// and the ranges when it starts each file.
			Name: "-s empties the hold space for each file",
			FS:   stringsFS(map[string]string{"one": "a\n", "two": "b\n"}),
			Args: []string{"-n", "-s", "/a/h\n$x\np", "one", "two"},
			Want: "a\n\n",
		},
		{
			Name: "the --separate long form restarts the line numbers",
			FS:   stringsFS(map[string]string{"a.txt": "a\nb\n", "b.txt": "c\nd\n"}),
			Args: []string{"--separate", "=", "a.txt", "b.txt"},
			Want: "1\na\n2\nb\n1\nc\n2\nd\n",
		},
		{
			// The n command which hits the end of one file doesn't end the
			// run: with -s, the next file still gets its own stream.
			Name: "-s goes on after the end of a file",
			FS:   stringsFS(map[string]string{"one": "a\n", "two": "b\n"}),
			Args: []string{"-s", "n;p", "one", "two"},
			Want: "a\nb\n",
		},
		{
			// Without -s, a missing file stops the run; with it, each file
			// is its own stream, and the error only takes the status.
			Name:     "-s reports a missing file and keeps going",
			FS:       stringsFS(map[string]string{"a.txt": "a\n", "b.txt": "b\n"}),
			Args:     []string{"-s", "p", "a.txt", "missing.txt", "b.txt"},
			Want:     "a\na\nb\nb\n",
			WantErr:  "sed: can't read missing.txt: file does not exist\n",
			WantExit: 2,
		},
	}
	runCases(t, tests)
}

func TestRunNullData(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "-z takes NUL-delimited records",
			Stdin: strings.NewReader("a\x00b\x00"),
			Args:  []string{"-z", "s/a/X/"},
			Want:  "X\x00b\x00",
		},
		{
			Name:  "-z keeps a last record without a NUL",
			Stdin: strings.NewReader("a\x00b"),
			Args:  []string{"-z", "s/a/X/"},
			Want:  "X\x00b",
		},
		{
			Name:  "-z joins the records of N with a NUL",
			Stdin: strings.NewReader("a\x00b\x00"),
			Args:  []string{"-z", "N;p"},
			Want:  "a\x00b\x00a\x00b\x00",
		},
		{
			Name:  "-z counts the NUL-delimited records",
			Stdin: strings.NewReader("a\x00b\x00c\x00"),
			Args:  []string{"-z", "2d"},
			Want:  "a\x00c\x00",
		},
		{
			Name:  "-z lets the period match a newline of the record",
			Stdin: strings.NewReader("a\nb\x00"),
			Args:  []string{"-z", "s/a.b/X/"},
			Want:  "X\x00",
		},
		{
			Name:  "the --null-data long form takes NUL-delimited records",
			Stdin: strings.NewReader("a\x00b\x00"),
			Args:  []string{"--null-data", "s/a/X/"},
			Want:  "X\x00b\x00",
		},
		{
			Name:  "the --zero-terminated long form takes NUL-delimited records",
			Stdin: strings.NewReader("a\x00b\x00"),
			Args:  []string{"--zero-terminated", "s/a/X/"},
			Want:  "X\x00b\x00",
		},
	}
	runCases(t, tests)
}

func TestRunLineLength(t *testing.T) {
	long := strings.Repeat("x", 12)
	tests := []testutil.Case{
		{
			Name:  "-l sets the wrap width of l",
			Stdin: strings.NewReader(long + "\n"),
			Args:  []string{"-l", "5", "l;d"},
			Want:  "xxxxx\\\nxxxxx\\\nxx$\n",
		},
		{
			Name:  "-l takes an attached width",
			Stdin: strings.NewReader(long + "\n"),
			Args:  []string{"-l5", "l;d"},
			Want:  "xxxxx\\\nxxxxx\\\nxx$\n",
		},
		{
			Name:  "the --line-length long form sets the wrap width",
			Stdin: strings.NewReader(long + "\n"),
			Args:  []string{"--line-length=5", "l;d"},
			Want:  "xxxxx\\\nxxxxx\\\nxx$\n",
		},
		{
			Name:  "a zero width leaves the output unfolded",
			Stdin: strings.NewReader(long + "\n"),
			Args:  []string{"-l", "0", "l;d"},
			Want:  long + "$\n",
		},
		{
			Name:  "the width of the l command wins over -l",
			Stdin: strings.NewReader(long + "\n"),
			Args:  []string{"-l", "5", "l10;d"},
			Want:  "xxxxxxxxxx\\\nxx$\n",
		},
		{
			Name:     "-l without a value is a usage error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-l"},
			WantErr:  "sed: option requires an argument -- 'l'\nTry 'sed --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a width which isn't a number is a usage error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-l", "x", "d"},
			WantErr:  "sed: invalid line length: x\nTry 'sed --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative width is a usage error",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-l", "-1", "d"},
			WantErr:  "sed: invalid line length: -1\nTry 'sed --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "the --line-length long form rejects a bad value",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--line-length=x", "d"},
			WantErr:  "sed: invalid line length: x\nTry 'sed --help' for more information.\n",
			WantExit: 2,
		},
	}
	runCases(t, tests)
}

func TestRunGNUOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the --expression long form gives the script",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"--expression", "s/a/X/"},
			Want:  "X\n",
		},
		{
			Name:  "the --expression long form takes an attached script",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"--expression=s/a/X/"},
			Want:  "X\n",
		},
		{
			Name:  "the --file long form reads the script from a file",
			FS:    stringsFS(map[string]string{"scr": "s/a/X/\n"}),
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"--file", "scr"},
			Want:  "X\n",
		},
		{
			Name:  "the --file long form takes an attached name",
			FS:    stringsFS(map[string]string{"scr": "s/a/X/\n"}),
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"--file=scr"},
			Want:  "X\n",
		},
		{
			Name:  "-f - reads the script from the standard input",
			FS:    stringsFS(map[string]string{"in": "a\n"}),
			Stdin: strings.NewReader("s/a/X/\n"),
			Args:  []string{"-f", "-", "in"},
			Want:  "X\n",
		},
		{
			Name:  "-r takes extended regular expressions",
			Stdin: strings.NewReader("aab\n"),
			Args:  []string{"-r", "s/a+b/X/"},
			Want:  "X\n",
		},
		{
			Name:  "the --regexp-extended long form takes EREs",
			Stdin: strings.NewReader("aab\n"),
			Args:  []string{"--regexp-extended", "s/a+b/X/"},
			Want:  "X\n",
		},
		{
			Name:  "the --extended-regexp long form takes EREs",
			Stdin: strings.NewReader("aab\n"),
			Args:  []string{"--extended-regexp", "s/a+b/X/"},
			Want:  "X\n",
		},
		{
			Name:  "the --quiet long form suppresses the default output",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"--quiet", "p"},
			Want:  "a\nb\n",
		},
		{
			Name:  "-u is accepted",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"-u", "s/a/X/"},
			Want:  "X\n",
		},
		{
			Name:  "--posix accepts a POSIX script",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"--posix", "s/a/X/"},
			Want:  "X\n",
		},
		{
			Name:  "--follow-symlinks without -i is accepted",
			Stdin: strings.NewReader("a\n"),
			Args:  []string{"--follow-symlinks", "s/a/X/"},
			Want:  "X\n",
		},
		// The --posix rejections of the GNU script syntax.
		{
			Name:     "the I flag of s is rejected with --posix",
			Stdin:    strings.NewReader("A\n"),
			Args:     []string{"--posix", "s/a/X/I"},
			WantErr:  "sed: the I flag of the s command is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		{
			// POSIX itself defines the lowercase i as the flag selecting
			// case-insensitive matching.
			Name:  "the i flag of s is accepted with --posix",
			Stdin: strings.NewReader("A\n"),
			Args:  []string{"--posix", "s/a/X/i"},
			Want:  "X\n",
		},
		{
			Name:     "the M flag of s is rejected with --posix",
			Stdin:    strings.NewReader("a\nb\n"),
			Args:     []string{"--posix", "N;s/^b/X/M"},
			WantErr:  "sed: the M flag of the s command is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		{
			Name:     "a number with the g flag of s is rejected with --posix",
			Stdin:    strings.NewReader("banana\n"),
			Args:     []string{"--posix", "s/a/-/2g"},
			WantErr:  "sed: a number with the g flag of the s command is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		{
			Name:     "the M modifier of an address is rejected with --posix",
			Stdin:    strings.NewReader("a\nb\nc\nd\n"),
			Args:     []string{"--posix", "N;/^b/Md"},
			WantErr:  "sed: the M modifier of an address is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		{
			Name:     "the exit status of q is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "1q5"},
			WantErr:  "sed: the exit status of the q command is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
		{
			// POSIX defines the numeric flag of s, only leaving its
			// combination with g unspecified, so --posix accepts it.
			Name:  "a bare number flag is accepted with --posix",
			Stdin: strings.NewReader("banana\n"),
			Args:  []string{"--posix", "s/a/-/2"},
			Want:  "ban-na\n",
		},
		{
			// POSIX itself says a script whose first two characters are #n
			// suppresses the default output, so --posix accepts it too.
			Name:  "a #n script is accepted with --posix",
			Stdin: strings.NewReader("a\nb\n"),
			Args:  []string{"--posix", "#n\np"},
			Want:  "a\nb\n",
		},
		{
			Name:     "the width of l is rejected with --posix",
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"--posix", "l10"},
			WantErr:  "sed: the width of the l command is a GNU extension, not allowed with --posix\n",
			WantExit: 2,
		},
	}
	runCases(t, tests)
}

func TestRunExitStatus(t *testing.T) {
	tests := []testutil.Case{
		{
			// The q exit status is used when no input file failed.
			Name:     "the exit status of q wins without a file error",
			FS:       stringsFS(map[string]string{"a.txt": "a\n"}),
			Args:     []string{"q5", "a.txt"},
			Want:     "a\n",
			WantExit: 5,
		},
		{
			// An input file which can't be opened takes the status 2 over
			// the one q asked for, since the failure is reported last.
			Name:     "a file open failure wins over the exit status of q",
			FS:       stringsFS(map[string]string{"a.txt": "a\n"}),
			Args:     []string{"q0", "a.txt", "missing.txt"},
			Want:     "a\n",
			WantErr:  "sed: can't read missing.txt: file does not exist\n",
			WantExit: 2,
		},
	}
	runCases(t, tests)
}
