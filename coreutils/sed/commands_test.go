package sed

import (
	"context"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"charm.land/snail/fsx"
)

func TestRunTextCommands(t *testing.T) {
	tests := []testCase{
		{
			name:  "a writes text after the line",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{`/a/a\text`},
			want:  "a\ntext\nb\n",
		},
		{
			name:  "i writes text before the line",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{`/a/i\text`},
			want:  "text\na\nb\n",
		},
		{
			name:  "text may follow on the next line",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"2a\\\n  text"},
			want:  "a\nb\ntext\n",
		},
		{
			name:  "the first text line loses its leading blanks",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"2a\\\n  text"},
			want:  "a\nb\ntext\n",
		},
		{
			name:  "text continues on lines ending with a backslash",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"2a\\\none\\\ntwo"},
			want:  "a\nb\none\ntwo\n",
		},
		{
			name:  "an escaped backslash is literal in the text",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{`2a\one\\`},
			want:  "a\nb\none\\\n",
		},
		{
			name:  "c replaces the line",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{`2c\X`},
			want:  "a\nX\nc\n",
		},
		{
			name:  "c starts the next cycle",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"2c\\\nX\np"},
			want:  "a\na\nX\nc\nc\n",
		},
		{
			name:  "c writes the text only at the end of a range",
			stdin: strings.NewReader("1\n2\n3\n4\n"),
			args:  []string{"2,3c\\\nX"},
			want:  "1\nX\n4\n",
		},
		{
			name:  "c writes the text on every line a negated range selects",
			stdin: strings.NewReader("1\n2\n3\n4\n"),
			args:  []string{`2,3!c\X`},
			want:  "X\n2\n3\nX\n",
		},
		{
			name:  "c writes the text on the last line a negated range selects",
			stdin: strings.NewReader("1\n2\n3\n4\n5\n"),
			args:  []string{`3,4!c\X`},
			want:  "X\nX\n3\n4\nX\n",
		},
		{
			name:  "r copies a file before the next line",
			fsys:  stringsFS(map[string]string{"r.txt": "FILE\n"}),
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1r r.txt"},
			want:  "a\nFILE\nb\n",
		},
		{
			name:  "r is written at the end of the script",
			fsys:  stringsFS(map[string]string{"r.txt": "FILE\n"}),
			stdin: strings.NewReader("a\n"),
			args:  []string{"1r r.txt"},
			want:  "a\nFILE\n",
		},
		{
			name:  "r is written before q quits",
			fsys:  stringsFS(map[string]string{"r.txt": "FILE\n"}),
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1r r.txt\n2q"},
			want:  "a\nFILE\nb\n",
		},
		{
			name:  "a text is written before q writes the line",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1a\\TXT\n1q"},
			want:  "TXT\na\n",
		},
		{
			name:  "r is written before q writes the line",
			fsys:  stringsFS(map[string]string{"r.txt": "FILE\n"}),
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1r r.txt\n1q"},
			want:  "FILE\na\n",
		},
		{
			name:     "a missing r file is empty",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1r missing.txt"},
			want:     "a\n",
			wantExit: 0,
		},
		{
			name:     "an r command without a filename is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1r"},
			wantErr:  "sed: filename expected for r command\n",
			wantExit: 2,
		},
		{
			name:     "an r command with only blanks after it is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1r "},
			wantErr:  "sed: filename expected for r command\n",
			wantExit: 2,
		},
		{
			name:     "a w command without a filename is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1w"},
			wantErr:  "sed: filename expected for w command\n",
			wantExit: 2,
		},
		{
			name:     "the w flag of s without a filename is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"s/a/X/w"},
			wantErr:  "sed: filename expected for the w flag of the s command\n",
			wantExit: 2,
		},
		{
			name:  "r and a are written in the order applied",
			fsys:  stringsFS(map[string]string{"r.txt": "FILE\n"}),
			stdin: strings.NewReader("a\n"),
			args:  []string{"1a\\first\n1r r.txt\n1a\\last"},
			want:  "a\nfirst\nFILE\nlast\n",
		},
		{
			name: "a text scheduled before D waits for the next line read",
			// The D command restarts the cycle without reading a line, so the
			// text stays scheduled until N reads a line or the script ends.
			stdin: strings.NewReader("a\ny\nz\n"),
			args:  []string{"-e", "p", "-e", "N", "-e", `/y/a\TXT`, "-e", "D"},
			want:  "a\ny\nTXT\nz\nTXT\n",
		},
		{
			name: "a file that can't be read is treated as empty",
			// Opening a directory succeeds but reading it fails, which sed
			// reports as if the file were empty.
			fsys:  fsx.MemFS{"r.txt": {Mode: fs.ModeDir | 0o755}},
			stdin: strings.NewReader("a\n"),
			args:  []string{"1r r.txt"},
			want:  "a\n",
		},
		{
			name:  "text without a backslash is a GNU one-line form",
			stdin: strings.NewReader("a\n"),
			args:  []string{"1atext"},
			want:  "a\ntext\n",
		},
		{
			name:     "text without a backslash is an error with --posix",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"--posix", "1atext"},
			wantErr:  "sed: text must start with \\\n",
			wantExit: 2,
		},
		{
			name:  "a takes its text on the same line without a backslash",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"2a two words"},
			want:  "a\nb\ntwo words\n",
		},
		{
			name:  "i takes its text on the same line without a backslash",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"2i two words"},
			want:  "a\ntwo words\nb\n",
		},
		{
			name:  "c takes its text on the same line without a backslash",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"2c two words"},
			want:  "a\ntwo words\nc\n",
		},
		{
			// The GNU form without a backslash also allows the text to
			// start on the next line.
			name:  "a takes its text on the next line without a backslash",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"2a\ntext"},
			want:  "a\nb\ntext\n",
		},
		{
			name:  "the one-line text continues with an escaped newline",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"2a one\\\ntwo"},
			want:  "a\nb\none\ntwo\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLineCommands(t *testing.T) {
	tests := []testCase{
		{
			name:  "d deletes the line",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1d"},
			want:  "b\n",
		},
		{
			name:  "d starts a new cycle",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1d;p"},
			want:  "b\nb\n",
		},
		{
			name:  "D deletes up to the first newline",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"N;D"},
			want:  "",
		},
		{
			name:  "D restarts the cycle without reading",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"N;P;D"},
			want:  "a\nb\n",
		},
		{
			name:  "D works like d without a newline",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1D;2D"},
			want:  "",
		},
		{
			name:  "p writes the pattern space",
			stdin: strings.NewReader("a\n"),
			args:  []string{"p"},
			want:  "a\na\n",
		},
		{
			name:  "P writes up to the first newline",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"N;P"},
			want:  "a\na\nb\n",
		},
		{
			name:  "n writes and reads the next line",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"1n;d"},
			want:  "a\n",
		},
		{
			name:  "n doesn't write with -n",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-n", "n;p"},
			want:  "b\n",
		},
		{
			name:  "n quits at the end of the input",
			stdin: strings.NewReader("a\n"),
			args:  []string{"n;p"},
			want:  "a\n",
		},
		{
			name:  "N appends the next line",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"N;p"},
			want:  "a\nb\na\nb\n",
		},
		{
			name:  "N counts the lines",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"2N;$="},
			want:  "a\n3\nb\nc\n",
		},
		{
			name:  "N quits without output at the end of the input",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1N;2N"},
			want:  "",
		},
		{
			name:  "q quits the script",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"2q"},
			want:  "a\nb\n",
		},
		{
			name:  "q writes the pattern space",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"2q"},
			want:  "a\nb\n",
		},
		{
			name:  "q skips the commands after it",
			stdin: strings.NewReader("a\n"),
			args:  []string{"1q;d"},
			want:  "a\n",
		},
		{
			name:  "= writes the line number",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"="},
			want:  "1\na\n2\nb\n",
		},
		{
			name:  "= takes one address",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"$="},
			want:  "a\nb\n3\nc\n",
		},
		{
			name:  "the newline of the last line is preserved",
			stdin: strings.NewReader("a\nb"),
			args:  []string{""},
			want:  "a\nb",
		},
		{
			name:  "p preserves a missing newline",
			stdin: strings.NewReader("a\nb"),
			args:  []string{"$!d"},
			want:  "b",
		},
		{
			name:  "l writes the pattern space unambiguously",
			stdin: strings.NewReader("a\tb\n"),
			args:  []string{"l"},
			want:  "a\\tb$\na\tb\n",
		},
		{
			name:  "l escapes every special character",
			stdin: strings.NewReader("\a\b\f\r\v\\\n"),
			args:  []string{"l;d"},
			want:  "\\a\\b\\f\\r\\v\\\\$\n",
		},
		{
			name:  "l writes non-printable characters in octal",
			stdin: strings.NewReader("\x01\x7f\n"),
			args:  []string{"l;d"},
			want:  "\\001\\177$\n",
		},
		{
			name:  "l writes the newlines of the pattern space escaped",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"N;l;d"},
			want:  "a\\nb$\n",
		},
		{
			name:  "l folds long lines",
			stdin: strings.NewReader(strings.Repeat("x", 75) + "\n"),
			args:  []string{"l;d"},
			want:  strings.Repeat("x", 70) + "\\\n" + strings.Repeat("x", 5) + "$\n",
		},
		{
			name:  "l takes a wrap width as its argument",
			stdin: strings.NewReader(strings.Repeat("x", 15) + "\n"),
			args:  []string{"l10;d"},
			want:  strings.Repeat("x", 10) + "\\\n" + strings.Repeat("x", 5) + "$\n",
		},
		{
			name:  "l with a zero width doesn't fold",
			stdin: strings.NewReader(strings.Repeat("x", 75) + "\n"),
			args:  []string{"l0;d"},
			want:  strings.Repeat("x", 75) + "$\n",
		},
		{
			name:     "q takes an exit status",
			stdin:    strings.NewReader("a\nb\nc\n"),
			args:     []string{"2q5"},
			want:     "a\nb\n",
			wantExit: 5,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunHoldSpace(t *testing.T) {
	tests := []testCase{
		{
			name:  "h replaces the hold space",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1h;1d;2G"},
			want:  "b\na\n",
		},
		{
			name:  "H appends to the hold space",
			stdin: strings.NewReader("a\nb\nc\n"),
			args:  []string{"1h;2H;$x;p"},
			want:  "a\na\nb\nb\na\nb\na\nb\n",
		},
		{
			name:  "g replaces the pattern space",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1h;2g"},
			want:  "a\na\n",
		},
		{
			name:  "G appends to the pattern space",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1h;2G"},
			want:  "a\nb\na\n",
		},
		{
			name:  "x exchanges the spaces",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1h;1d;2x;p"},
			want:  "a\na\n",
		},
		{
			name:  "the hold space starts empty",
			stdin: strings.NewReader("a\n"),
			args:  []string{"x;p"},
			want:  "\n\n",
		},
		{
			name:  "G on an empty hold space adds a newline",
			stdin: strings.NewReader("a\n"),
			args:  []string{"G"},
			want:  "a\n\n",
		},
		{
			name:  "hold and pattern spaces hold newlines",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-n", "N;h;x;p"},
			want:  "a\nb\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunTransliterate(t *testing.T) {
	tests := []testCase{
		{
			name:  "y replaces every character",
			stdin: strings.NewReader("abcabc\n"),
			args:  []string{"y/abc/xyz/"},
			want:  "xyzxyz\n",
		},
		{
			name:  "y keeps the characters beyond string1",
			stdin: strings.NewReader("abcd\n"),
			args:  []string{"y/ab/XY/"},
			want:  "XYcd\n",
		},
		{
			name:  "y accepts a custom delimiter with escapes",
			stdin: strings.NewReader("a,b\n"),
			args:  []string{`y,a\,,X.,`},
			want:  "X.b\n",
		},
		{
			name:  "y resolves escapes",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{`N;y/a\n/X\//`},
			want:  "X/b\n",
		},
		{
			name:  "y translates multibyte characters",
			stdin: strings.NewReader("ábc\n"),
			args:  []string{`y/áé/ÁÉ/`},
			want:  "Ábc\n",
		},
		{
			name:     "y fails on different lengths",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"y/ab/X/"},
			wantErr:  "sed: y command strings have different lengths\n",
			wantExit: 2,
		},
		{
			name:     "y fails on a repeated character",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"y/aa/XY/"},
			wantErr:  "sed: repeated character in y command\n",
			wantExit: 2,
		},
		{
			name:     "y fails on an invalid escape",
			stdin:    strings.NewReader("a\n"),
			args:     []string{`y/\t/X/`},
			wantErr:  "sed: invalid escape in y command: \\t\n",
			wantExit: 2,
		},
		{
			name:     "y fails when unterminated",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"y/a/X"},
			wantErr:  "sed: unterminated string in y command\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunBlocksAndLabels(t *testing.T) {
	tests := []testCase{
		{
			name:  "a block groups commands",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"/a/{s/a/1/;s/^/>/;}"},
			want:  ">1\nb\n",
		},
		{
			name:  "a skipped block runs nothing",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"/b/{s/a/1/;s/^/>/;}"},
			want:  "a\n>b\n",
		},
		{
			name:  "blocks nest",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"/a/{/a/{s/a/X/;};}"},
			want:  "X\nb\n",
		},
		{
			name:  "a block may be negated",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1!{s/^/>/;}"},
			want:  "a\n>b\n",
		},
		{
			name:  "a block runs with two addresses",
			stdin: strings.NewReader("1\n2\n3\n4\n"),
			args:  []string{"2,3{s/^/>/;}"},
			want:  "1\n>2\n>3\n4\n",
		},
		{
			name:  "commands run after a block",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"/a/{p;};d"},
			want:  "a\n",
		},
		{
			name:  "b branches forward to a label",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-n", "/b/b end\ns/./X/\np\n:end"},
			want:  "X\n",
		},
		{
			name:  "b branches back to a label",
			stdin: strings.NewReader("xxa\n"),
			args:  []string{"-n", ":top\n/x/!b end\ns/x//\nb top\n:end\np"},
			want:  "a\n",
		},
		{
			name:  "b without a label goes to the end",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1b\ns/./X/"},
			want:  "a\nX\n",
		},
		{
			name:  "t branches after a substitution",
			stdin: strings.NewReader("aa\nbb\n"),
			args:  []string{"s/a/X/\ntl\ns/b/Y/\n:l"},
			want:  "Xa\nYb\n",
		},
		{
			name:  "t doesn't branch without a substitution",
			stdin: strings.NewReader("cc\nbb\n"),
			args:  []string{"s/a/X/\ntl\ns/b/Y/\n:l"},
			want:  "cc\nYb\n",
		},
		{
			name:  "t is reset when it runs",
			stdin: strings.NewReader("ab\n"),
			args:  []string{":l\ns/b//\ntl\ns/a/X/\n:l"},
			want:  "X\n",
		},
		{
			name:  "t is reset when a line is read",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"s/a/X/;n\nt\ns/b/Y/"},
			want:  "X\nY\n",
		},
		{
			name:  "t is reset when N reads a line",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-n", "s/a/X/\nN\ntX\np\n:X"},
			want:  "X\nb\n",
		},
		{
			name:  "t is not reset by D",
			stdin: strings.NewReader("ab\nc\n"),
			args:  []string{"-n", "tX\nN\ns/b//\nD\n:X\np"},
			want:  "c\n",
		},
		{
			name:  "a loop may process several lines",
			stdin: strings.NewReader("1\n2\n3\n"),
			args:  []string{"-n", ":l\np\nn\nbl"},
			want:  "1\n2\n3\n",
		},
		{
			// The b, t and : labels extend to the end of the line, so a
			// semicolon doesn't end them.
			name:  "labels ignore blanks",
			stdin: strings.NewReader("a\n"),
			args:  []string{":l\n/z/b l\ns/./X/"},
			want:  "X\n",
		},
		{
			// GNU sed ends a label at the first semicolon, so the d is a
			// command of its own rather than part of the label.
			name:  "the command after a branch semicolon runs",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"2!b skip; d\n:skip"},
			want:  "a\n",
		},
		{
			name:  "a label ends at a semicolon",
			stdin: strings.NewReader("a\n"),
			args:  []string{":l ; p\ns/./X/"},
			want:  "a\nX\n",
		},
		{
			name:  "comments are ignored",
			stdin: strings.NewReader("a\n"),
			args:  []string{"# comment\ns/a/X/"},
			want:  "X\n",
		},
		{
			name:  "a comment may follow commands",
			stdin: strings.NewReader("a\n"),
			args:  []string{"s/a/X/ # rest is ignored"},
			want:  "X\n",
		},
		{
			name:  "empty commands are ignored",
			stdin: strings.NewReader("a\n"),
			args:  []string{";;s/a/X/;;"},
			want:  "X\n",
		},
		{
			name:  "commands are separated by semicolons",
			stdin: strings.NewReader("a\n"),
			args:  []string{"p;p"},
			want:  "a\na\na\n",
		},
		{
			name:     "a branch to an unknown label is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"bx"},
			wantErr:  "sed: label not found: x\n",
			wantExit: 2,
		},
		{
			name:     "a t to an unknown label is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"tx"},
			wantErr:  "sed: label not found: x\n",
			wantExit: 2,
		},
		{
			name:     "an unmatched open brace is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"{d"},
			wantErr:  "sed: unmatched {\n",
			wantExit: 2,
		},
		{
			name:     "an unmatched close brace is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"d}"},
			wantErr:  "sed: unexpected }\n",
			wantExit: 2,
		},
		{
			name:  "a close brace may follow a newline",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"/a/{d\n}"},
			want:  "b\n",
		},
		{
			name:  "a close brace may follow a semicolon",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"/a/{d;}"},
			want:  "b\n",
		},
		{
			name:  "blanks may precede a close brace",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"/a/{d\n }"},
			want:  "b\n",
		},
		{
			name:  "a close brace may start a line",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"/a/{d\n\n}"},
			want:  "b\n",
		},
		{
			name:  "close braces may be chained after semicolons",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"{{d;};}"},
			want:  "",
		},
		{
			name:     "a close brace right after a command is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"{p}"},
			wantErr:  "sed: unexpected }\n",
			wantExit: 2,
		},
		{
			name:     "a close brace after chained commands is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"{p;q}"},
			wantErr:  "sed: unexpected }\n",
			wantExit: 2,
		},
		{
			name:     "an unknown command is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"Z"},
			wantErr:  "sed: unknown command: Z\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOptions(t *testing.T) {
	tests := []testCase{
		{
			name:  "-n suppresses the default output",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-n", "p"},
			want:  "a\nb\n",
		},
		{
			name:  "-n still writes explicit output",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-n", "s/a/X/p"},
			want:  "X\n",
		},
		{
			name:  "options may be grouped",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-n", "p"},
			want:  "a\nb\n",
		},
		{
			name:  "-e gives the script",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-e", "d"},
			want:  "",
		},
		{
			name:  "-e scripts run in order",
			stdin: strings.NewReader("a\n"),
			args:  []string{"-e", "s/a/1/", "-e", "s/1/2/"},
			want:  "2\n",
		},
		{
			name:  "-e scripts are separated by newlines",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-e", "p", "-n"},
			want:  "a\nb\n",
		},
		{
			name:  "-f reads the script from a file",
			fsys:  stringsFS(map[string]string{"scr": "s/a/X/\n"}),
			stdin: strings.NewReader("a\n"),
			args:  []string{"-f", "scr"},
			want:  "X\n",
		},
		{
			name:  "-f scripts may lack a trailing newline",
			fsys:  stringsFS(map[string]string{"scr": "s/a/X/"}),
			stdin: strings.NewReader("a\n"),
			args:  []string{"-f", "scr"},
			want:  "X\n",
		},
		{
			name:  "-f is followed by -e",
			fsys:  stringsFS(map[string]string{"scr": "s/a/1/;"}),
			stdin: strings.NewReader("a\n"),
			args:  []string{"-f", "scr", "-e", "s/1/2/"},
			want:  "2\n",
		},
		{
			name:  "-e is followed by -f with a newline between them",
			fsys:  stringsFS(map[string]string{"scr": "s/1/2/"}),
			stdin: strings.NewReader("a\n"),
			args:  []string{"-e", "s/a/1/", "-f", "scr"},
			want:  "2\n",
		},
		{
			name:  "-e and -f run in the order given",
			fsys:  stringsFS(map[string]string{"scr": "s/a/X/"}),
			stdin: strings.NewReader("a\n"),
			args:  []string{"-e", "s/X/1/", "-f", "scr"},
			want:  "X\n",
		},
		{
			name:  "the script operand is used without -e and -f",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"s/a/X/"},
			want:  "X\nb\n",
		},
		{
			name: "operands after the script name the files",
			fsys: stringsFS(map[string]string{"f1": "a\n", "f2": "b\n"}),
			args: []string{"s/./X/", "f1", "f2"},
			want: "X\nX\n",
		},
		{
			name: "-- ends the options",
			fsys: stringsFS(map[string]string{"-n": "a\n"}),
			args: []string{"--", "s/a/X/", "-n"},
			want: "X\n",
		},
		{
			name:  "a #n script suppresses the default output",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"#n\np"},
			want:  "a\nb\n",
		},
		{
			name:  "a -e script may start with #n",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"-e", "#n", "-e", "p"},
			want:  "a\nb\n",
		},
		{
			name:  "-E matches with extended regular expressions",
			stdin: strings.NewReader("aab\n"),
			args:  []string{"-E", "s/a+b/X/"},
			want:  "X\n",
		},
		{
			name:  "-E makes the BRE syntax ordinary",
			stdin: strings.NewReader("a(b\n"),
			args:  []string{"-E", `s/\(b/X/`},
			want:  "aX\n",
		},
		{
			name:     "no script is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-n"},
			wantErr:  "sed: missing script\nTry 'sed --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "no arguments is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{},
			wantErr:  "sed: missing script\nTry 'sed --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "an unknown option is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-Y", "d"},
			wantErr:  "sed: invalid option -- 'Y'\nTry 'sed --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "-e without an argument is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-e"},
			wantErr:  "sed: option requires an argument -- 'e'\nTry 'sed --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "-f without an argument is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-f"},
			wantErr:  "sed: option requires an argument -- 'f'\nTry 'sed --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a missing script file is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-f", "missing"},
			wantErr:  "sed: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			// An empty -f option-argument is a file name, not an empty
			// script, so opening it fails.
			name:     "an empty -f file name is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-f", ""},
			wantErr:  "sed: : invalid argument\n",
			wantExit: 1,
		},
		{
			// An empty -e option-argument is an empty script.
			name:  "an empty -e script is allowed",
			stdin: strings.NewReader("a\n"),
			args:  []string{"-e", ""},
			want:  "a\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunFiles(t *testing.T) {
	tests := []testCase{
		{
			name: "files are read in order",
			fsys: stringsFS(map[string]string{"b.txt": "b\n", "a.txt": "a\n"}),
			args: []string{"d", "a.txt", "b.txt"},
			want: "",
		},
		{
			name: "line numbers count across files",
			fsys: stringsFS(map[string]string{"a.txt": "a\nb\n", "b.txt": "c\nd\n"}),
			args: []string{"3d", "a.txt", "b.txt"},
			want: "a\nb\nd\n",
		},
		{
			name: "$ addresses the last line of all files",
			fsys: stringsFS(map[string]string{"a.txt": "a\nb\n", "b.txt": "c\n"}),
			args: []string{"$d", "a.txt", "b.txt"},
			want: "a\nb\n",
		},
		{
			name: "an empty file contributes no lines",
			fsys: stringsFS(map[string]string{"a.txt": "", "b.txt": "b\n"}),
			args: []string{"1d", "a.txt", "b.txt"},
			want: "",
		},
		{
			name: "a missing last newline is kept",
			fsys: stringsFS(map[string]string{"a.txt": "a", "b.txt": "b\n"}),
			args: []string{"", "a.txt", "b.txt"},
			want: "ab\n",
		},
		{
			name:  "- is the standard input",
			fsys:  stringsFS(map[string]string{"a.txt": "a\n"}),
			stdin: strings.NewReader("s\n"),
			args:  []string{"d", "a.txt", "-", "a.txt"},
			want:  "",
		},
		{
			name: "the working directory is honored",
			fsys: stringsFS(map[string]string{"sub/a.txt": "a\n"}),
			dir:  "sub",
			args: []string{"d", "a.txt"},
			want: "",
		},
		{
			name:     "a missing file is an error",
			fsys:     stringsFS(map[string]string{"a.txt": "a\n"}),
			args:     []string{"d", "a.txt", "missing.txt"},
			want:     "",
			wantErr:  "sed: can't read missing.txt: file does not exist\n",
			wantExit: 2,
		},
		{
			// The lookahead that tells whether a line is the last one is
			// the first read to hit the missing file, so the lines read
			// before it must still be processed.
			name:     "lines read before a missing file are kept",
			fsys:     stringsFS(map[string]string{"a.txt": "a\nb\n"}),
			args:     []string{"p", "a.txt", "missing.txt"},
			want:     "a\na\nb\nb\n",
			wantErr:  "sed: can't read missing.txt: file does not exist\n",
			wantExit: 2,
		},
		{
			// A file that can't be opened doesn't keep the ones after it
			// from being processed, and the exit status is 2.
			name:     "the files after a missing one are still processed",
			fsys:     stringsFS(map[string]string{"a.txt": "a\n", "b.txt": "b\n"}),
			args:     []string{"p", "a.txt", "missing.txt", "b.txt"},
			want:     "a\na\nb\nb\n",
			wantErr:  "sed: can't read missing.txt: file does not exist\n",
			wantExit: 2,
		},
		{
			name: "w appends to a file",
			fsys: stringsFS(map[string]string{"in": "a\nb\n"}),
			args: []string{"-n", "1w out", "in"},
			want: "",
			wantFiles: map[string]string{
				"out": "a\n",
			},
		},
		{
			name: "w truncates an existing file",
			fsys: stringsFS(map[string]string{"in": "a\nb\n", "out": "OLD\n"}),
			args: []string{"-n", "1w out", "in"},
			wantFiles: map[string]string{
				"out": "a\n",
			},
		},
		{
			name: "two w commands may share a file",
			fsys: stringsFS(map[string]string{"in": "a\nb\n"}),
			args: []string{"-n", "1w out\n2w out", "in"},
			wantFiles: map[string]string{
				"out": "a\nb\n",
			},
		},
		{
			name: "w writes the whole pattern space",
			fsys: stringsFS(map[string]string{"in": "a\nb\n"}),
			args: []string{"-n", "N;w out", "in"},
			wantFiles: map[string]string{
				"out": "a\nb\n",
			},
		},
		{
			name: "a w file is created with the default mode",
			fsys: stringsFS(map[string]string{"in": "a\n"}),
			args: []string{"-n", "1w out", "in"},
			wantFiles: map[string]string{
				"out": "a\n",
			},
			wantModes: map[string]fs.FileMode{"out": 0o666},
		},
		{
			name:  "a w file respects the umask",
			fsys:  stringsFS(map[string]string{"in": "a\n"}),
			umask: 0o022,
			args:  []string{"-n", "1w out", "in"},
			wantFiles: map[string]string{
				"out": "a\n",
			},
			wantModes: map[string]fs.FileMode{"out": 0o644},
		},
		{
			name:     "a w file which can't be created is an error",
			fsys:     stringsFS(map[string]string{"in": "a\n"}),
			args:     []string{"-n", "w nodir/out", "in"},
			wantErr:  "sed: can't create nodir/out: file does not exist\n",
			wantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

// cancelReader cancels its context on the first read, simulating a context
// that is done while input is being read.
type cancelReader struct {
	cancel context.CancelFunc
}

func (r *cancelReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
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
		stdin := &cancelReader{cancel: cancel}

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
	tests := []testCase{
		// The T command branches when no substitution happened.
		{
			name:  "T branches without a substitution",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"s/b/X/\nTl\ns/./Y/\n:l"},
			want:  "a\nY\n",
		},
		{
			name:  "T doesn't branch after a substitution",
			stdin: strings.NewReader("a\n"),
			args:  []string{"s/a/X/\nTl\ns/./Y/\n:l"},
			want:  "Y\n",
		},
		{
			// The first T only takes the flag away, so the second one
			// finds no substitution and branches, skipping the d.
			name:  "T resets the substitution flag when it doesn't branch",
			stdin: strings.NewReader("a\n"),
			args:  []string{"s/a/x/;T done;T done;d;:done"},
			want:  "x\n",
		},
		{
			// The same flag is what the t looks at, so the T which took it
			// away keeps the t from branching too.
			name:  "t doesn't branch after a T took the flag away",
			stdin: strings.NewReader("a\n"),
			args:  []string{"s/a/x/\nTl\ntl\nd\n:l"},
			want:  "",
		},
		{
			name:     "T to an unknown label is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"Tx"},
			wantErr:  "sed: label not found: x\n",
			wantExit: 2,
		},
		{
			name:     "the T command is rejected with --posix",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"--posix", "Tl"},
			wantErr:  "sed: the T command is a GNU extension, not allowed with --posix\n",
			wantExit: 2,
		},
		// The Q command quits without writing the pattern space.
		{
			name:  "Q quits without writing the pattern space",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1Q"},
			want:  "",
		},
		{
			name:  "Q writes the text scheduled before it",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1a\\text\n1Q"},
			want:  "text\n",
		},
		{
			name:     "Q takes an exit status",
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"1Q5"},
			want:     "",
			wantExit: 5,
		},
		{
			name:     "an out of range exit status is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"q256"},
			wantErr:  "sed: exit status out of range\n",
			wantExit: 2,
		},
		{
			name:     "the Q command is rejected with --posix",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"--posix", "1Q"},
			wantErr:  "sed: the Q command is a GNU extension, not allowed with --posix\n",
			wantExit: 2,
		},
		// The z command empties the pattern space.
		{
			name:  "z empties the pattern space",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1z"},
			want:  "\nb\n",
		},
		{
			name:     "the z command is rejected with --posix",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"--posix", "z"},
			wantErr:  "sed: the z command is a GNU extension, not allowed with --posix\n",
			wantExit: 2,
		},
		// The F command writes the name of the current input file.
		{
			name: "F writes the name of the current file",
			fsys: stringsFS(map[string]string{"a.txt": "a\n", "b.txt": "b\n"}),
			args: []string{"F", "a.txt", "b.txt"},
			want: "a.txt\na\nb.txt\nb\n",
		},
		{
			name:  "F writes - for the standard input",
			stdin: strings.NewReader("a\n"),
			args:  []string{"F"},
			want:  "-\na\n",
		},
		{
			name:     "the F command is rejected with --posix",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"--posix", "F"},
			wantErr:  "sed: the F command is a GNU extension, not allowed with --posix\n",
			wantExit: 2,
		},
		// The W command writes the first line of the pattern space.
		{
			name: "W writes the first line of the pattern space",
			fsys: stringsFS(map[string]string{"in": "a\nb\nc\n"}),
			args: []string{"-n", "N;W out", "in"},
			want: "",
			wantFiles: map[string]string{
				"out": "a\n",
			},
		},
		{
			name: "W writes the first line of a joined pattern space",
			fsys: stringsFS(map[string]string{"in": "a\nb\nc\n"}),
			args: []string{"-n", "2N;3W out", "in"},
			wantFiles: map[string]string{
				"out": "b\n",
			},
		},
		{
			name:     "the W command is rejected with --posix",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"--posix", "W out"},
			wantErr:  "sed: the W command is a GNU extension, not allowed with --posix\n",
			wantExit: 2,
		},
		// The R command queues one line of a file each time it runs.
		{
			name: "R queues one line of the file per cycle",
			fsys: stringsFS(map[string]string{"r.txt": "l1\nl2\n", "in": "a\nb\nc\n"}),
			args: []string{"-n", "R r.txt", "in"},
			want: "l1\nl2\n",
		},
		{
			name: "R keeps reading where it left off",
			fsys: stringsFS(map[string]string{"r.txt": "l1\nl2\nl3\n", "in": "a\nb\nc\n"}),
			args: []string{"-n", "R r.txt", "in"},
			want: "l1\nl2\nl3\n",
		},
		{
			name: "R writes after the pattern space of the cycle",
			fsys: stringsFS(map[string]string{"r.txt": "l1\n", "in": "a\nb\n"}),
			args: []string{"R r.txt", "in"},
			want: "a\nl1\nb\n",
		},
		{
			name: "a missing R file has no lines",
			fsys: stringsFS(map[string]string{"in": "a\nb\n"}),
			args: []string{"R missing.txt", "in"},
			want: "a\nb\n",
		},
		{
			name:     "the R command is rejected with --posix",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"--posix", "R r.txt"},
			wantErr:  "sed: the R command is a GNU extension, not allowed with --posix\n",
			wantExit: 2,
		},
		// The e command runs a shell, which the runtime doesn't provide.
		{
			name:     "the e command is unknown",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"e"},
			wantErr:  "sed: unknown command: e\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunStdStreamNames(t *testing.T) {
	tests := []testCase{
		{
			name: "w writes to /dev/stdout",
			fsys: stringsFS(map[string]string{"in": "a\nb\n"}),
			args: []string{"-n", "1w /dev/stdout", "in"},
			want: "a\n",
		},
		{
			name:    "w writes to /dev/stderr",
			fsys:    stringsFS(map[string]string{"in": "a\nb\n"}),
			args:    []string{"-n", "1w /dev/stderr", "in"},
			wantErr: "a\n",
		},
		{
			name: "the w flag of s writes to /dev/stdout",
			fsys: stringsFS(map[string]string{"in": "a\nb\n"}),
			args: []string{"-n", "s/a/X/w /dev/stdout", "in"},
			want: "X\n",
		},
		{
			name: "W writes to /dev/stdout",
			fsys: stringsFS(map[string]string{"in": "a\nb\nc\n"}),
			args: []string{"-n", "N;W /dev/stdout", "in"},
			want: "a\n",
		},
		{
			name:  "r reads from /dev/stdin",
			fsys:  stringsFS(map[string]string{"in": "a\nb\n"}),
			stdin: strings.NewReader("EXTRA\n"),
			args:  []string{"1r /dev/stdin", "in"},
			want:  "a\nEXTRA\nb\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSeparate(t *testing.T) {
	tests := []testCase{
		{
			name: "-s restarts the line numbers for each file",
			fsys: stringsFS(map[string]string{"a.txt": "a\nb\n", "b.txt": "c\nd\n"}),
			args: []string{"-s", "=", "a.txt", "b.txt"},
			want: "1\na\n2\nb\n1\nc\n2\nd\n",
		},
		{
			name: "the line numbers continue without -s",
			fsys: stringsFS(map[string]string{"a.txt": "a\nb\n", "b.txt": "c\nd\n"}),
			args: []string{"=", "a.txt", "b.txt"},
			want: "1\na\n2\nb\n3\nc\n4\nd\n",
		},
		{
			name: "-s takes $ as the last line of each file",
			fsys: stringsFS(map[string]string{"a.txt": "a\nb\n", "b.txt": "c\nd\n"}),
			args: []string{"-s", "$d", "a.txt", "b.txt"},
			want: "a\nc\n",
		},
		{
			name: "-s restarts the ranges for each file",
			fsys: stringsFS(map[string]string{"one": "a\nb\nc\n", "two": "a\nb\n"}),
			args: []string{"-s", "/a/,/b/d", "one", "two"},
			want: "c\n",
		},
		{
			// GNU sed empties the hold space as well as the line counter
			// and the ranges when it starts each file.
			name: "-s empties the hold space for each file",
			fsys: stringsFS(map[string]string{"one": "a\n", "two": "b\n"}),
			args: []string{"-n", "-s", "/a/h\n$x\np", "one", "two"},
			want: "a\n\n",
		},
		{
			name: "the --separate long form restarts the line numbers",
			fsys: stringsFS(map[string]string{"a.txt": "a\nb\n", "b.txt": "c\nd\n"}),
			args: []string{"--separate", "=", "a.txt", "b.txt"},
			want: "1\na\n2\nb\n1\nc\n2\nd\n",
		},
		{
			// The n command which hits the end of one file doesn't end the
			// run: with -s, the next file still gets its own stream.
			name: "-s goes on after the end of a file",
			fsys: stringsFS(map[string]string{"one": "a\n", "two": "b\n"}),
			args: []string{"-s", "n;p", "one", "two"},
			want: "a\nb\n",
		},
		{
			// Without -s, a missing file stops the run; with it, each file
			// is its own stream, and the error only takes the status.
			name:     "-s reports a missing file and keeps going",
			fsys:     stringsFS(map[string]string{"a.txt": "a\n", "b.txt": "b\n"}),
			args:     []string{"-s", "p", "a.txt", "missing.txt", "b.txt"},
			want:     "a\na\nb\nb\n",
			wantErr:  "sed: can't read missing.txt: file does not exist\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunNullData(t *testing.T) {
	tests := []testCase{
		{
			name:  "-z takes NUL-delimited records",
			stdin: strings.NewReader("a\x00b\x00"),
			args:  []string{"-z", "s/a/X/"},
			want:  "X\x00b\x00",
		},
		{
			name:  "-z keeps a last record without a NUL",
			stdin: strings.NewReader("a\x00b"),
			args:  []string{"-z", "s/a/X/"},
			want:  "X\x00b",
		},
		{
			name:  "-z joins the records of N with a NUL",
			stdin: strings.NewReader("a\x00b\x00"),
			args:  []string{"-z", "N;p"},
			want:  "a\x00b\x00a\x00b\x00",
		},
		{
			name:  "-z counts the NUL-delimited records",
			stdin: strings.NewReader("a\x00b\x00c\x00"),
			args:  []string{"-z", "2d"},
			want:  "a\x00c\x00",
		},
		{
			name:  "-z lets the period match a newline of the record",
			stdin: strings.NewReader("a\nb\x00"),
			args:  []string{"-z", "s/a.b/X/"},
			want:  "X\x00",
		},
		{
			name:  "the --null-data long form takes NUL-delimited records",
			stdin: strings.NewReader("a\x00b\x00"),
			args:  []string{"--null-data", "s/a/X/"},
			want:  "X\x00b\x00",
		},
		{
			name:  "the --zero-terminated long form takes NUL-delimited records",
			stdin: strings.NewReader("a\x00b\x00"),
			args:  []string{"--zero-terminated", "s/a/X/"},
			want:  "X\x00b\x00",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLineLength(t *testing.T) {
	long := strings.Repeat("x", 12)
	tests := []testCase{
		{
			name:  "-l sets the wrap width of l",
			stdin: strings.NewReader(long + "\n"),
			args:  []string{"-l", "5", "l;d"},
			want:  "xxxxx\\\nxxxxx\\\nxx$\n",
		},
		{
			name:  "-l takes an attached width",
			stdin: strings.NewReader(long + "\n"),
			args:  []string{"-l5", "l;d"},
			want:  "xxxxx\\\nxxxxx\\\nxx$\n",
		},
		{
			name:  "the --line-length long form sets the wrap width",
			stdin: strings.NewReader(long + "\n"),
			args:  []string{"--line-length=5", "l;d"},
			want:  "xxxxx\\\nxxxxx\\\nxx$\n",
		},
		{
			name:  "a zero width leaves the output unfolded",
			stdin: strings.NewReader(long + "\n"),
			args:  []string{"-l", "0", "l;d"},
			want:  long + "$\n",
		},
		{
			name:  "the width of the l command wins over -l",
			stdin: strings.NewReader(long + "\n"),
			args:  []string{"-l", "5", "l10;d"},
			want:  "xxxxxxxxxx\\\nxx$\n",
		},
		{
			name:     "-l without a value is a usage error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-l"},
			wantErr:  "sed: option requires an argument -- 'l'\nTry 'sed --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a width which isn't a number is a usage error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-l", "x", "d"},
			wantErr:  "sed: invalid line length: x\nTry 'sed --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a negative width is a usage error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-l", "-1", "d"},
			wantErr:  "sed: invalid line length: -1\nTry 'sed --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "the --line-length long form rejects a bad value",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"--line-length=x", "d"},
			wantErr:  "sed: invalid line length: x\nTry 'sed --help' for more information.\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunGNUOptions(t *testing.T) {
	tests := []testCase{
		{
			name:  "the --expression long form gives the script",
			stdin: strings.NewReader("a\n"),
			args:  []string{"--expression", "s/a/X/"},
			want:  "X\n",
		},
		{
			name:  "the --expression long form takes an attached script",
			stdin: strings.NewReader("a\n"),
			args:  []string{"--expression=s/a/X/"},
			want:  "X\n",
		},
		{
			name:  "the --file long form reads the script from a file",
			fsys:  stringsFS(map[string]string{"scr": "s/a/X/\n"}),
			stdin: strings.NewReader("a\n"),
			args:  []string{"--file", "scr"},
			want:  "X\n",
		},
		{
			name:  "the --file long form takes an attached name",
			fsys:  stringsFS(map[string]string{"scr": "s/a/X/\n"}),
			stdin: strings.NewReader("a\n"),
			args:  []string{"--file=scr"},
			want:  "X\n",
		},
		{
			name:  "-f - reads the script from the standard input",
			fsys:  stringsFS(map[string]string{"in": "a\n"}),
			stdin: strings.NewReader("s/a/X/\n"),
			args:  []string{"-f", "-", "in"},
			want:  "X\n",
		},
		{
			name:  "-r takes extended regular expressions",
			stdin: strings.NewReader("aab\n"),
			args:  []string{"-r", "s/a+b/X/"},
			want:  "X\n",
		},
		{
			name:  "the --regexp-extended long form takes EREs",
			stdin: strings.NewReader("aab\n"),
			args:  []string{"--regexp-extended", "s/a+b/X/"},
			want:  "X\n",
		},
		{
			name:  "the --extended-regexp long form takes EREs",
			stdin: strings.NewReader("aab\n"),
			args:  []string{"--extended-regexp", "s/a+b/X/"},
			want:  "X\n",
		},
		{
			name:  "the --quiet long form suppresses the default output",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"--quiet", "p"},
			want:  "a\nb\n",
		},
		{
			name:  "-u is accepted",
			stdin: strings.NewReader("a\n"),
			args:  []string{"-u", "s/a/X/"},
			want:  "X\n",
		},
		{
			name:  "--posix accepts a POSIX script",
			stdin: strings.NewReader("a\n"),
			args:  []string{"--posix", "s/a/X/"},
			want:  "X\n",
		},
		{
			name:  "--follow-symlinks without -i is accepted",
			stdin: strings.NewReader("a\n"),
			args:  []string{"--follow-symlinks", "s/a/X/"},
			want:  "X\n",
		},
		// The --posix rejections of the GNU script syntax.
		{
			name:     "the I flag of s is rejected with --posix",
			stdin:    strings.NewReader("A\n"),
			args:     []string{"--posix", "s/a/X/I"},
			wantErr:  "sed: the I flag of the s command is a GNU extension, not allowed with --posix\n",
			wantExit: 2,
		},
		{
			// POSIX itself defines the lowercase i as the flag selecting
			// case-insensitive matching.
			name:  "the i flag of s is accepted with --posix",
			stdin: strings.NewReader("A\n"),
			args:  []string{"--posix", "s/a/X/i"},
			want:  "X\n",
		},
		{
			name:     "the M flag of s is rejected with --posix",
			stdin:    strings.NewReader("a\nb\n"),
			args:     []string{"--posix", "N;s/^b/X/M"},
			wantErr:  "sed: the M flag of the s command is a GNU extension, not allowed with --posix\n",
			wantExit: 2,
		},
		{
			name:     "a number with the g flag of s is rejected with --posix",
			stdin:    strings.NewReader("banana\n"),
			args:     []string{"--posix", "s/a/-/2g"},
			wantErr:  "sed: a number with the g flag of the s command is a GNU extension, not allowed with --posix\n",
			wantExit: 2,
		},
		{
			name:     "the M modifier of an address is rejected with --posix",
			stdin:    strings.NewReader("a\nb\nc\nd\n"),
			args:     []string{"--posix", "N;/^b/Md"},
			wantErr:  "sed: the M modifier of an address is a GNU extension, not allowed with --posix\n",
			wantExit: 2,
		},
		{
			name:     "the exit status of q is rejected with --posix",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"--posix", "1q5"},
			wantErr:  "sed: the exit status of the q command is a GNU extension, not allowed with --posix\n",
			wantExit: 2,
		},
		{
			// POSIX defines the numeric flag of s, only leaving its
			// combination with g unspecified, so --posix accepts it.
			name:  "a bare number flag is accepted with --posix",
			stdin: strings.NewReader("banana\n"),
			args:  []string{"--posix", "s/a/-/2"},
			want:  "ban-na\n",
		},
		{
			// POSIX itself says a script whose first two characters are #n
			// suppresses the default output, so --posix accepts it too.
			name:  "a #n script is accepted with --posix",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"--posix", "#n\np"},
			want:  "a\nb\n",
		},
		{
			name:     "the width of l is rejected with --posix",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"--posix", "l10"},
			wantErr:  "sed: the width of the l command is a GNU extension, not allowed with --posix\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunExitStatus(t *testing.T) {
	tests := []testCase{
		{
			// The q exit status is used when no input file failed.
			name:     "the exit status of q wins without a file error",
			fsys:     stringsFS(map[string]string{"a.txt": "a\n"}),
			args:     []string{"q5", "a.txt"},
			want:     "a\n",
			wantExit: 5,
		},
		{
			// An input file which can't be opened takes the status 2 over
			// the one q asked for, since the failure is reported last.
			name:     "a file open failure wins over the exit status of q",
			fsys:     stringsFS(map[string]string{"a.txt": "a\n"}),
			args:     []string{"q0", "a.txt", "missing.txt"},
			want:     "a\n",
			wantErr:  "sed: can't read missing.txt: file does not exist\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}
