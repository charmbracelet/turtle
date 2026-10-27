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
			name:     "text without a backslash is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"1atext"},
			wantErr:  "sed: text must start with \\\n",
			wantExit: 2,
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
			// The d belongs to the label of b, so it doesn't run: the
			// branch targets the : command bearing the whole label.
			name:  "the label of b extends to the end of a line",
			stdin: strings.NewReader("a\nb\n"),
			args:  []string{"1!b end;d\ns/./X/\n:end;d"},
			want:  "X\nb\n",
		},
		{
			// The d belongs to the label of :, so it doesn't run.
			name:  "the label of : extends to the end of a line",
			stdin: strings.NewReader("a\n"),
			args:  []string{":end;d\ns/./X/"},
			want:  "X\n",
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
			wantErr:  "sed: missing script\n",
			wantExit: 2,
		},
		{
			name:     "no arguments is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{},
			wantErr:  "sed: missing script\n",
			wantExit: 2,
		},
		{
			name:     "an unknown option is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-z", "d"},
			wantErr:  "sed: invalid option -- 'z'\n",
			wantExit: 2,
		},
		{
			name:     "-e without an argument is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-e"},
			wantErr:  "sed: option requires an argument -- 'e'\n",
			wantExit: 2,
		},
		{
			name:     "-f without an argument is an error",
			stdin:    strings.NewReader("a\n"),
			args:     []string{"-f"},
			wantErr:  "sed: option requires an argument -- 'f'\n",
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
			wantExit: 1,
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
			wantExit: 1,
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
