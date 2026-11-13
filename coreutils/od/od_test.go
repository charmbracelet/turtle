package od

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/fsx"
)

type odTest struct {
	name     string
	args     []string
	stdin    io.Reader
	files    fsx.MemFS
	want     string
	wantErr  string
	wantCode int
}

func runOdTests(t *testing.T, tests []odTest) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
		})
	}
}

func TestRun(t *testing.T) {
	runOdTests(t, []odTest{
		{
			name:  "default octal words and addresses",
			stdin: bytes.NewReader([]byte{1, 2, 3}),
			want:  "0000000 001001 000003\n0000003\n",
		},
		{
			name:  "byte octal type and no addresses",
			args:  []string{"-A", "n", "-t", "o1", "file"},
			files: fsx.MemFS{"file": {Data: []byte{1, 2, 255}}},
			want:  " 001 002 377\n",
		},
		{
			name:  "character escapes",
			args:  []string{"-A", "n", "-tc", "file"},
			files: fsx.MemFS{"file": {Data: []byte{'A', 0, '\n', 0x7f}}},
			want:  "   A  \\0  \\n 177\n",
		},
		{
			name:  "named characters",
			args:  []string{"-A", "n", "-ta", "file"},
			files: fsx.MemFS{"file": {Data: []byte{0, 32, 65}}},
			want:  " nul  sp   A\n",
		},
		{
			name:  "named characters ignore the high-order bit",
			args:  []string{"-A", "n", "-ta", "file"},
			files: fsx.MemFS{"file": {Data: []byte{0x80, 0xff, 'A' | 0x80}}},
			want:  " nul del   A\n",
		},
		{
			name:  "hexadecimal addresses are 6 digits wide",
			args:  []string{"-A", "x", "-t", "x2", "file"},
			files: fsx.MemFS{"file": {Data: []byte{0x34, 0x12, 0xff, 0xff}}},
			want:  "000000 1234 ffff\n000004\n",
		},
		{
			name:  "skip and count parse prefixes",
			args:  []string{"-A", "d", "-j", "0x1", "-N", "02", "-t", "o1", "file"},
			files: fsx.MemFS{"file": {Data: []byte{1, 2, 3, 4, 5}}},
			want:  "0000001 002 003\n0000003\n",
		},
		{
			name:     "skip past end is an error",
			args:     []string{"-j", "5", "file"},
			files:    fsx.MemFS{"file": {Data: []byte{1, 2}}},
			wantErr:  "od: cannot skip past end of combined input\n",
			wantCode: 1,
		},
		{
			name:     "invalid type is rejected",
			args:     []string{"-t", "q"},
			wantErr:  "od: invalid type string\nTry 'od --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "empty type is rejected",
			args:     []string{"-t", ""},
			wantErr:  "od: empty type string\nTry 'od --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "unknown option is rejected",
			args:     []string{"-q"},
			wantErr:  "od: invalid option -- 'q'\nTry 'od --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "-u is not an option",
			args:     []string{"-u"},
			wantErr:  "od: invalid option -- 'u'\nTry 'od --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "an address radix of more than one character is rejected",
			args:     []string{"-A", "xn"},
			wantErr:  "od: invalid address base\nTry 'od --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:     "unknown long option is rejected",
			args:     []string{"--bogus"},
			wantErr:  "od: unrecognized option '--bogus'\nTry 'od --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:  "long options match their short forms",
			args:  []string{"--address-radix", "n", "--format=o1", "--read-bytes", "2", "--skip-bytes", "1"},
			stdin: bytes.NewReader([]byte{1, 2, 255}),
			want:  " 002 377\n",
		},
		{
			name:  "the lines of several types are aligned",
			args:  []string{"-t", "o2x2"},
			stdin: bytes.NewReader([]byte{0x34, 0x12, 0xff, 0xff}),
			want:  "0000000 011064 177777\n          1234   ffff\n0000004\n",
		},
		{
			name:  "the types of several options accumulate",
			args:  []string{"-b", "-x"},
			stdin: bytes.NewReader([]byte{0x34, 0x12}),
			want:  "0000000 064 022\n           1234\n0000002\n",
		},
		{
			name:  "duplicate lines are elided",
			stdin: bytes.NewReader(bytes.Repeat([]byte{'A'}, 40)),
			want: "0000000 040501 040501 040501 040501 040501 040501 040501 040501\n" +
				"*\n0000040 040501 040501 040501 040501\n0000050\n",
		},
		{
			name:  "--output-duplicates keeps the duplicate lines",
			args:  []string{"--output-duplicates"},
			stdin: bytes.NewReader(bytes.Repeat([]byte{'A'}, 40)),
			want: "0000000 040501 040501 040501 040501 040501 040501 040501 040501\n" +
				"0000020 040501 040501 040501 040501 040501 040501 040501 040501\n" +
				"0000040 040501 040501 040501 040501\n0000050\n",
		},
		{
			name:  "an operand which looks like an option stays one",
			args:  []string{"a", "-w"},
			files: fsx.MemFS{"a": {Data: []byte{1}}, "-w": {Data: []byte{2}}},
			want:  "0000000 001001\n0000002\n",
		},
		{
			name:  "an operand after -- stays one",
			args:  []string{"--", "-w"},
			files: fsx.MemFS{"-w": {Data: []byte{1}}},
			want:  "0000000 000001\n0000001\n",
		},
	})
}

func TestRunWidth(t *testing.T) {
	repeat := func(s string, n int) string { return strings.Repeat(s, n) }
	stdin := func() io.Reader { return bytes.NewReader(bytes.Repeat([]byte{'A'}, 40)) }
	per32 := "0000000" + repeat(" 040501", 16) + "\n" + "0000040" + repeat(" 040501", 4) + "\n0000050\n"
	runOdTests(t, []odTest{
		{
			name:  "-w alone means 32 bytes and keeps the operand",
			args:  []string{"-w", "file"},
			files: fsx.MemFS{"file": {Data: bytes.Repeat([]byte{'A'}, 40)}},
			want:  per32,
		},
		{
			name:  "-w alone on standard input",
			args:  []string{"-A", "n", "-w"},
			stdin: stdin(),
			want:  repeat(" 040501", 16) + "\n" + repeat(" 040501", 4) + "\n",
		},
		{
			name:  "-w at the end of a cluster stays bare",
			args:  []string{"-vw"},
			stdin: stdin(),
			want:  per32,
		},
		{
			name:  "-w with an attached value",
			args:  []string{"-w8"},
			stdin: stdin(),
			want:  "0000000 040501 040501 040501 040501\n*\n0000050\n",
		},
		{
			name:  "--width with a value",
			args:  []string{"--width=8"},
			stdin: stdin(),
			want:  "0000000 040501 040501 040501 040501\n*\n0000050\n",
		},
		{
			name:  "--width alone means 32 bytes",
			args:  []string{"--width"},
			stdin: stdin(),
			want:  per32,
		},
		{
			name:     "a zero width is rejected",
			args:     []string{"-w0"},
			wantErr:  "od: invalid width '0'\nTry 'od --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:    "a width which is not a multiple warns and falls back",
			args:    []string{"-w3"},
			stdin:   bytes.NewReader([]byte{1, 2, 3, 4}),
			want:    "0000000 001001\n0000002 002003\n0000004\n",
			wantErr: "od: warning: invalid width 3; using 2 instead\n",
		},
	})
}

func TestRunEndian(t *testing.T) {
	runOdTests(t, []odTest{
		{
			name:  "big swaps the units",
			args:  []string{"-t", "x2", "--endian=big"},
			stdin: bytes.NewReader([]byte{0x34, 0x12}),
			want:  "0000000 3412\n0000002\n",
		},
		{
			name:  "little keeps them",
			args:  []string{"-t", "x2", "--endian=little"},
			stdin: bytes.NewReader([]byte{0x34, 0x12}),
			want:  "0000000 1234\n0000002\n",
		},
		{
			name:     "an invalid order is rejected",
			args:     []string{"--endian=middle"},
			stdin:    bytes.NewReader([]byte{1, 2}),
			wantErr:  "od: invalid --endian value 'middle'\nTry 'od --help' for more information.\n",
			wantCode: 1,
		},
		{
			name:  "--endian hides the offset operand",
			args:  []string{"--endian=little", "file", "10"},
			files: fsx.MemFS{"file": {Data: []byte{1, 2}}, "10": {Data: []byte{3, 4}}},
			want:  "0000000 001001 002003\n0000004\n",
		},
		{
			name:     "a missing order is rejected",
			args:     []string{"--endian"},
			wantErr:  "od: option '--endian' requires an argument\nTry 'od --help' for more information.\n",
			wantCode: 1,
		},
	})
}

func TestRunTypes(t *testing.T) {
	repeat := func(s string, n int) string { return strings.Repeat(s, n) }
	runOdTests(t, []odTest{
		{
			name:  "float",
			args:  []string{"-An", "-tfF"},
			stdin: bytes.NewReader([]byte{0, 0, 0x80, 0x3f}),
			want:  repeat(" ", 18) + "1\n",
		},
		{
			name:  "float by size",
			args:  []string{"-An", "-tf4"},
			stdin: bytes.NewReader([]byte{0, 0, 0x80, 0x3f}),
			want:  repeat(" ", 18) + "1\n",
		},
		{
			name:  "double",
			args:  []string{"-An", "-tfD"},
			stdin: bytes.NewReader([]byte{0, 0, 0, 0, 0, 0, 0xf0, 0x3f}),
			want:  repeat(" ", 36) + "1\n",
		},
		{
			name:  "float infinity",
			args:  []string{"-An", "-tfF"},
			stdin: bytes.NewReader([]byte{0, 0, 0x80, 0x7f}),
			want:  repeat(" ", 16) + "inf\n",
		},
		{
			name:  "float NaN",
			args:  []string{"-An", "-tfF"},
			stdin: bytes.NewReader([]byte{0, 0, 0xc0, 0x7f}),
			want:  repeat(" ", 16) + "nan\n",
		},
		{
			name:  "double negative infinity",
			args:  []string{"-An", "-tfD"},
			stdin: bytes.NewReader([]byte{0, 0, 0, 0, 0, 0, 0xf0, 0xff}),
			want:  repeat(" ", 33) + "-inf\n",
		},
		{
			name:  "half float infinity",
			args:  []string{"-An", "-tfH"},
			stdin: bytes.NewReader([]byte{0x00, 0x7c}),
			want:  repeat(" ", 16) + "inf\n",
		},
		{
			name:  "half float",
			args:  []string{"-An", "-tfH"},
			stdin: bytes.NewReader([]byte{0x00, 0x3c}),
			want:  repeat(" ", 18) + "1\n",
		},
		{
			name:  "half float of a longer value",
			args:  []string{"-An", "-tfH"},
			stdin: bytes.NewReader([]byte{0xc0, 0x3f}),
			want:  repeat(" ", 13) + "1.9375\n",
		},
		{
			name:  "bfloat",
			args:  []string{"-An", "-tfB"},
			stdin: bytes.NewReader([]byte{0xc0, 0x3f}),
			want:  repeat(" ", 16) + "1.5\n",
		},
		{
			name:  "long double",
			args:  []string{"-An", "-tfL"},
			stdin: bytes.NewReader([]byte{0, 0, 0, 0, 0, 0, 0, 0x80, 0xff, 0x3f, 0, 0, 0, 0, 0, 0}),
			want:  repeat(" ", 42) + "1\n",
		},
		{
			name:  "long double keeps its extended precision",
			args:  []string{"-An", "-tfL"},
			stdin: bytes.NewReader([]byte{0x35, 0xc2, 0x68, 0x21, 0xa2, 0xda, 0x0f, 0xc9, 0xff, 0x3f, 0, 0, 0, 0, 0, 0}),
			want:  repeat(" ", 22) + "1.5707963267948966193\n",
		},
		{
			name:  "negative long double zero",
			args:  []string{"-An", "-tfL"},
			stdin: bytes.NewReader([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0x00, 0x80, 0, 0, 0, 0, 0, 0}),
			want:  repeat(" ", 41) + "-0\n",
		},
		{
			name:  "long double infinity",
			args:  []string{"-An", "-tfL"},
			stdin: bytes.NewReader([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0x7f, 0, 0, 0, 0, 0, 0}),
			want:  repeat(" ", 40) + "inf\n",
		},
		{
			name:  "signed decimal of 4 bytes",
			args:  []string{"-An", "-td4"},
			stdin: bytes.NewReader([]byte{0xff, 0xff, 0xff, 0xff}),
			want:  repeat(" ", 10) + "-1\n",
		},
		{
			name:  "signed decimal of 8 bytes",
			args:  []string{"-An", "-td8"},
			stdin: bytes.NewReader(bytes.Repeat([]byte{0xff}, 8)),
			want:  repeat(" ", 20) + "-1\n",
		},
		{
			name:  "octal of 4 bytes",
			args:  []string{"-An", "-to4"},
			stdin: bytes.NewReader(bytes.Repeat([]byte{0xff}, 4)),
			want:  " 37777777777\n",
		},
		{
			name:  "unsigned decimal of 2 bytes",
			args:  []string{"-An", "-tu2"},
			stdin: bytes.NewReader([]byte{0xff, 0xff}),
			want:  " 65535\n",
		},
		{
			name:  "hexadecimal of 4 bytes",
			args:  []string{"-An", "-tx4"},
			stdin: bytes.NewReader([]byte{0x34, 0x12, 0x56, 0x78}),
			want:  " 78561234\n",
		},
		{
			name:  "trailing z appends the character column",
			args:  []string{"-An", "-tx1z"},
			stdin: bytes.NewReader([]byte("hi\n")),
			want:  " 68 69 0a" + repeat(" ", 39) + "  >hi.<\n",
		},
		{
			name:  "trailing z on a named character type",
			args:  []string{"-An", "-taz"},
			stdin: bytes.NewReader([]byte{'A', 0, '\n'}),
			want:  "   A nul  nl" + repeat(" ", 52) + "  >A..<\n",
		},
		{
			name:  "trailing z on the character type",
			args:  []string{"-An", "-tcz"},
			stdin: bytes.NewReader([]byte("hi\n")),
			want:  "   h   i  \\n" + repeat(" ", 52) + "  >hi.<\n",
		},
	})

	t.Run("invalid sizes", func(t *testing.T) {
		for _, test := range []struct{ arg, err string }{
			{"d16", "unsupported type size"},
			{"f3", "unsupported type size"},
			{"d0", "unsupported type size"},
			{"xC4", "invalid type string"},
		} {
			t.Run(test.arg, func(t *testing.T) {
				runOdTests(t, []odTest{{
					name:     test.arg,
					args:     []string{"-t", test.arg},
					stdin:    bytes.NewReader([]byte{1}),
					wantErr:  "od: " + test.err + "\nTry 'od --help' for more information.\n",
					wantCode: 1,
				}})
			})
		}
	})
}

func TestRunStrings(t *testing.T) {
	runOdTests(t, []odTest{
		{
			name:  "NUL terminated strings",
			args:  []string{"-S", "3"},
			stdin: strings.NewReader("abc\x00hi\x00xy"),
			want:  "0000000 abc\n",
		},
		{
			name:  "-S alone defaults to 3",
			args:  []string{"-S"},
			stdin: strings.NewReader("abc\x00xy\x00"),
			want:  "0000000 abc\n",
		},
		{
			name:  "-S at the end of a cluster defaults to 3",
			args:  []string{"-vS"},
			stdin: strings.NewReader("abc\x00xy\x00"),
			want:  "0000000 abc\n",
		},
		{
			name:  "-S still takes the argument which follows it",
			args:  []string{"-S", "4"},
			stdin: strings.NewReader("abc\x00abcd\x00"),
			want:  "0000004 abcd\n",
		},
		{
			name:  "--strings defaults to 3",
			args:  []string{"--strings"},
			stdin: strings.NewReader("ab\x00abcd\x00"),
			want:  "0000003 abcd\n",
		},
		{
			name:  "the operand of --strings stays a file",
			args:  []string{"--strings", "file"},
			files: fsx.MemFS{"file": {Data: []byte("hello\x00")}},
			want:  "0000000 hello\n",
		},
		{
			name:  "the operand of -S is its argument",
			args:  []string{"-S", "4"},
			stdin: strings.NewReader("abc\x00abcd\x00"),
			want:  "0000004 abcd\n",
		},
		{
			name:  "a string cut by -N counts as terminated",
			args:  []string{"-S", "3", "-N", "5"},
			stdin: strings.NewReader("abcdef"),
			want:  "0000000 abcde\n",
		},
		{
			name:  "the input end does not terminate",
			args:  []string{"-S", "3"},
			stdin: strings.NewReader("abcdef"),
			want:  "",
		},
		{
			name:  "-N beyond the input does not terminate",
			args:  []string{"-S", "3", "-N", "10"},
			stdin: strings.NewReader("abc\x00def"),
			want:  "0000000 abc\n",
		},
		{
			name:  "the addresses follow -A and the skip",
			args:  []string{"-A", "d", "-S", "3"},
			stdin: strings.NewReader("xy\x00abc\x00"),
			want:  "0000003 abc\n",
		},
		{
			name:     "a type with strings is rejected",
			args:     []string{"-S", "3", "-t", "o1"},
			wantErr:  "od: no type may be specified when dumping strings\n",
			wantCode: 1,
		},
		{
			name:     "an invalid string length is rejected",
			args:     []string{"-S", "x"},
			wantErr:  "od: invalid string length\nTry 'od --help' for more information.\n",
			wantCode: 1,
		},
	})
}

func TestRunTraditional(t *testing.T) {
	digits := func(n int) []byte { // 0, 1, 2, ...
		data := make([]byte, n)
		for i := range data {
			data[i] = byte(i)
		}
		return data
	}
	at512 := make([]byte, 516)
	copy(at512[512:], []byte{8, 9, 10, 11})
	runOdTests(t, []odTest{
		{
			name:  "an offset with a plus",
			args:  []string{"file", "+10"},
			files: fsx.MemFS{"file": {Data: digits(12)}},
			want:  "0000010 004410 005412\n0000014\n",
		},
		{
			name:  "an offset with a digit",
			args:  []string{"file", "10"},
			files: fsx.MemFS{"file": {Data: digits(12)}},
			want:  "0000010 004410 005412\n0000014\n",
		},
		{
			name:  "a decimal offset with a dot",
			args:  []string{"file", "8."},
			files: fsx.MemFS{"file": {Data: digits(12)}},
			want:  "0000010 004410 005412\n0000014\n",
		},
		{
			name:  "a hexadecimal offset",
			args:  []string{"file", "+0x8"},
			files: fsx.MemFS{"file": {Data: digits(12)}},
			want:  "0000010 004410 005412\n0000014\n",
		},
		{
			name:  "an offset in blocks",
			args:  []string{"file", "1b"},
			files: fsx.MemFS{"file": {Data: at512}},
			want:  "0001000 004410 005412\n0001004\n",
		},
		{
			name:  "an offset alone reads standard input",
			args:  []string{"+10"},
			stdin: bytes.NewReader(digits(12)),
			want:  "0000010 004410 005412\n0000014\n",
		},
		{
			name:  "one numeric operand is a file",
			args:  []string{"10"},
			files: fsx.MemFS{"10": {Data: []byte{1, 2, 3}}},
			want:  "0000000 001001 000003\n0000003\n",
		},
		{
			name: "modern options hide the offset operand",
			args: []string{"-A", "n", "file", "10"},
			files: fsx.MemFS{
				"file": {Data: []byte{1, 2, 3, 4}},
				"10":   {Data: []byte{5, 6, 7, 8}},
			},
			want: " 001001 002003 003005 004007\n",
		},
		{
			name:  "a label follows the offset",
			args:  []string{"--traditional", "file", "10", "20"},
			files: fsx.MemFS{"file": {Data: digits(12)}},
			want:  "0000010 (0000020) 004410 005412\n0000014 (0000024)\n",
		},
		{
			name:  "an offset and a label alone read standard input",
			args:  []string{"--traditional", "10", "20"},
			stdin: bytes.NewReader(digits(12)),
			want:  "0000010 (0000020) 004410 005412\n0000014 (0000024)\n",
		},
		{
			name:  "a label with no addresses",
			args:  []string{"-A", "n", "--traditional", "10", "20"},
			stdin: bytes.NewReader(digits(12)),
			want:  "(0000020) 004410 005412\n(0000024)\n",
		},
		{
			name:  "the label sets the indent of the other type lines",
			args:  []string{"-t", "o2x2", "--traditional", "file", "10", "20"},
			files: fsx.MemFS{"file": {Data: digits(12)}},
			want: "0000010 (0000020) 004410 005412\n" +
				strings.Repeat(" ", 20) + "0908   0b0a\n" +
				"0000014 (0000024)\n",
		},
		{
			name:  "the label with no addresses sets the indent",
			args:  []string{"-A", "n", "-t", "o2x2", "--traditional", "10", "20"},
			stdin: bytes.NewReader(digits(12)),
			want: "(0000020) 004410 005412\n" +
				strings.Repeat(" ", 12) + "0908   0b0a\n" +
				"(0000024)\n",
		},
		{
			name:  "three operands without --traditional are files",
			args:  []string{"a", "b", "c"},
			files: fsx.MemFS{"a": {Data: []byte{1}}, "b": {Data: []byte{2}}, "c": {Data: []byte{3}}},
			want:  "0000000 001001 000003\n0000003\n",
		},
		{
			name:     "two files with --traditional are rejected",
			args:     []string{"--traditional", "f1", "f2"},
			files:    fsx.MemFS{"f1": {Data: []byte{1}}, "f2": {Data: []byte{2}}},
			wantErr:  "od: extra operand 'f2'\nod: compatibility mode supports at most one file\nTry 'od --help' for more information.\n",
			wantCode: 1,
		},
	})
}

func TestRunSizeSuffixes(t *testing.T) {
	data := make([]byte, 1030)
	data[512], data[513], data[514], data[515] = 1, 2, 3, 4
	data[1024], data[1025] = 1, 2
	runOdTests(t, []odTest{
		{
			name:  "b multiplies by 512",
			args:  []string{"-A", "d", "-j", "1b", "-N", "4", "-t", "o1"},
			stdin: bytes.NewReader(data),
			want:  "0000512 001 002 003 004\n0000516\n",
		},
		{
			name:  "K multiplies by 1024",
			args:  []string{"-A", "d", "-j", "1K", "-N", "2", "-t", "o1"},
			stdin: bytes.NewReader(data),
			want:  "0001024 001 002\n0001026\n",
		},
		{
			name:  "KiB multiplies by 1024 too",
			args:  []string{"-A", "d", "-j", "1KiB", "-N", "2", "-t", "o1"},
			stdin: bytes.NewReader(data),
			want:  "0001024 001 002\n0001026\n",
		},
		{
			name:  "KB multiplies by 1000",
			args:  []string{"-A", "d", "-j", "1KB", "-N", "4", "-t", "o1"},
			stdin: bytes.NewReader(data),
			want:  "0001000 000 000 000 000\n0001004\n",
		},
	})
}

func TestRunAddressWidth(t *testing.T) {
	data := make([]byte, 10000002)
	data[10000000], data[10000001] = 1, 2
	runOdTests(t, []odTest{
		{
			name:  "addresses widen past their width",
			args:  []string{"-A", "d", "-j", "10000000", "-N", "2", "-t", "o1"},
			stdin: bytes.NewReader(data),
			want:  "10000000 001 002\n10000002\n",
		},
	})
}

func TestShorthands(t *testing.T) {
	data := []byte{'A', 0, 255, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 0x7f, 32, 33}
	for _, test := range []struct{ short, format string }{
		{"-a", "-t a"},
		{"-b", "-t o1"},
		{"-c", "-t c"},
		{"-d", "-t u2"},
		{"-f", "-t fF"},
		{"-i", "-t dI"},
		{"-l", "-t dL"},
		{"-o", "-t o2"},
		{"-s", "-t d2"},
		{"-x", "-t x2"},
	} {
		t.Run(test.short, func(t *testing.T) {
			dump := func(args ...string) string {
				stdout := new(bytes.Buffer)
				runtime := &coreutils.Runtime{Stdin: bytes.NewReader(data), Stdout: stdout, Stderr: io.Discard}
				if err := New(runtime).Run(t.Context(), args...); err != nil {
					t.Fatalf("Run(%v) error = %v", args, err)
				}
				return stdout.String()
			}
			if got, want := dump(test.short), dump(strings.Fields(test.format)...); got != want {
				t.Errorf("od %s output = %q, want %q", test.short, got, want)
			}
		})
	}
}

func TestRunCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	prog := New(&coreutils.Runtime{Stdin: bytes.NewReader([]byte{1}), Stdout: io.Discard, Stderr: io.Discard})
	if err := prog.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "od (charm.land/turtle/coreutils) " + coreutils.Version + "\nhttps://charm.land/turtle\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
