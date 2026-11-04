package printfmt

import (
	"math"
	"strings"
	"testing"
)

func TestScanSpec(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		argNum bool
		want   Spec
		verb   byte
		n      int
	}{
		{
			name: "plain verb",
			s:    "%d",
			want: Spec{Width: -1, Prec: -1},
			verb: 'd',
			n:    1,
		},
		{
			name: "flags width and precision",
			s:    "%-+ 0#5.2d",
			want: Spec{Flags: "-+ 0#", Width: 5, Prec: 2, Raw: "-+ 0#5.2"},
			verb: 'd',
			n:    9,
		},
		{
			name: "precision without digits",
			s:    "%5.s",
			want: Spec{Width: 5, Prec: 0, Raw: "5."},
			verb: 's',
			n:    3,
		},
		{
			name:   "argument number",
			s:      "%2$d",
			argNum: true,
			want:   Spec{ArgNum: 2, Width: -1, Prec: -1, Raw: "2$"},
			verb:   'd',
			n:      3,
		},
		{
			name:   "digits without dollar are a width",
			s:      "%2d",
			argNum: true,
			want:   Spec{Width: 2, Prec: -1, Raw: "2"},
			verb:   'd',
			n:      2,
		},
		{
			name: "an absurd width saturates",
			s:    "%99999999999d",
			want: Spec{Width: math.MaxInt32, Prec: -1, Raw: "99999999999"},
			verb: 'd',
			n:    12,
		},
		{
			name: "format ends before the verb",
			s:    "%-5",
			want: Spec{Flags: "-", Width: 5, Prec: -1, Raw: "-5"},
			n:    3,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec, n := ScanSpec(test.s, test.argNum)
			if spec != test.want {
				t.Errorf("ScanSpec() spec = %+v, want %+v", spec, test.want)
			}
			verb := byte(0)
			if n < len(test.s) {
				verb = test.s[n]
			}
			if verb != test.verb {
				t.Errorf("ScanSpec() verb = %q, want %q", verb, test.verb)
			}
			if n != test.n {
				t.Errorf("ScanSpec() n = %d, want %d", n, test.n)
			}
		})
	}
}

func TestScanEscape(t *testing.T) {
	tests := []struct {
		name string
		s    string
		es   Escapes
		want Esc
		n    int
	}{
		{
			name: "simple escape",
			s:    "nab",
			want: Esc{Lit: "\n"},
			n:    1,
		},
		{
			name: "the stop escape",
			s:    "c",
			want: Esc{Stop: true},
			n:    1,
		},
		{
			name: "octal escape of one digit",
			s:    "0",
			es:   Escapes{Octal: OctalPlain},
			want: Esc{Lit: "\x00"},
			n:    1,
		},
		{
			name: "octal escape of three digits",
			s:    "101b",
			es:   Escapes{Octal: OctalPlain},
			want: Esc{Lit: "A"},
			n:    3,
		},
		{
			name: "an octal value beyond a byte is truncated",
			s:    "777",
			es:   Escapes{Octal: OctalPlain},
			want: Esc{Lit: "\xff"},
			n:    3,
		},
		{
			name: "octal escapes need a leading zero in the b mode",
			s:    "101",
			es:   Escapes{Octal: OctalZero},
			want: Esc{Lit: "\\1", Unknown: true},
			n:    1,
		},
		{
			name: "octal escapes take four digits in the b mode",
			s:    "0101b",
			es:   Escapes{Octal: OctalZero},
			want: Esc{Lit: "A"},
			n:    4,
		},
		{
			name: "an escape out of the set is unknown",
			s:    "e",
			es:   Escapes{Octal: OctalPlain},
			want: Esc{Lit: "\\e", Unknown: true},
			n:    1,
		},
		{
			name: "octal escapes need digits",
			s:    "8",
			es:   Escapes{Octal: OctalPlain},
			want: Esc{Lit: "\\8", Unknown: true},
			n:    1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			esc, n, err := ScanEscape(test.s, test.es)
			if err != nil {
				t.Fatalf("ScanEscape() error = %v", err)
			}
			if esc != test.want {
				t.Errorf("ScanEscape() = %+v, want %+v", esc, test.want)
			}
			if n != test.n {
				t.Errorf("ScanEscape() n = %d, want %d", n, test.n)
			}
		})
	}
}

func TestExpand(t *testing.T) {
	tests := []struct {
		name string
		s    string
		es   Escapes
		max  int
		want string
		stop bool
	}{
		{
			name: "plain text",
			s:    "abc",
			max:  -1,
			want: "abc",
		},
		{
			name: "a trailing backslash is kept",
			s:    "ab\\",
			max:  -1,
			want: "ab\\",
		},
		{
			name: "escapes",
			s:    "a\\tb\\n",
			max:  -1,
			want: "a\tb\n",
		},
		{
			name: "an unknown escape stands as it is",
			s:    "a\\zb",
			max:  -1,
			want: "a\\zb",
		},
		{
			name: "the stop escape",
			s:    "a\\cbb",
			max:  -1,
			want: "a",
			stop: true,
		},
		{
			name: "stops after max bytes",
			s:    "a\\tb\\tc",
			max:  3,
			want: "a\tb",
		},
		{
			name: "a unicode escape is cut at max bytes",
			s:    "\\u20ac",
			es:   Escapes{Extended: true},
			max:  2,
			want: "\u20ac"[:2],
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out strings.Builder
			stop, err := test.es.Expand(&out, test.s, test.max)
			if err != nil {
				t.Fatalf("Expand() error = %v", err)
			}
			if stop != test.stop {
				t.Errorf("Expand() stop = %v, want %v", stop, test.stop)
			}
			if got := out.String(); got != test.want {
				t.Errorf("Expand() = %q, want %q", got, test.want)
			}
		})
	}
}
