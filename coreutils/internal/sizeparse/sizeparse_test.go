package sizeparse

import "testing"

type parseTest struct {
	name    string
	in      string
	want    int64
	wantErr string
}

func TestParse(t *testing.T) {
	const (
		kib = 1 << 10
		mib = 1 << 20
		gib = 1 << 30
		tib = 1 << 40
		pib = 1 << 50
		eib = 1 << 60

		kb = 1000
		mb = kb * 1000
		gb = mb * 1000
		tb = gb * 1000
		pb = tb * 1000
		eb = pb * 1000
	)

	groups := []struct {
		name  string
		tests []parseTest
	}{
		{
			name: "plain numbers",
			tests: []parseTest{
				{name: "zero", in: "0", want: 0},
				{name: "one", in: "1", want: 1},
				{name: "a few digits", in: "4096", want: 4096},
				{name: "leading zeros", in: "007", want: 7},
				{name: "the largest int64", in: "9223372036854775807", want: 9223372036854775807},
				{name: "a number too large for an int64", in: "9223372036854775808", wantErr: "invalid size: '9223372036854775808'"},
				{name: "a number far too large", in: "999999999999999999999999", wantErr: "invalid size: '999999999999999999999999'"},
			},
		},
		{
			name: "block suffixes",
			tests: []parseTest{
				{name: "one block", in: "1b", want: 512},
				{name: "blocks", in: "3b", want: 1536},
				{name: "zero blocks", in: "0b", want: 0},
				{name: "one char", in: "1c", want: 1},
				{name: "chars", in: "42c", want: 42},
				{name: "one word", in: "1w", want: 2},
				{name: "words", in: "7w", want: 14},
				{name: "the largest int64 in chars", in: "9223372036854775807c", want: 9223372036854775807},
				{name: "an upper case B is no suffix", in: "1B", wantErr: "invalid size: '1B': invalid suffix: 'B'"},
				{name: "an upper case C is no suffix", in: "1C", wantErr: "invalid size: '1C': invalid suffix: 'C'"},
				{name: "an upper case W is no suffix", in: "1W", wantErr: "invalid size: '1W': invalid suffix: 'W'"},
			},
		},
		{
			name: "binary letters",
			tests: []parseTest{
				{name: "k", in: "1k", want: kib},
				{name: "upper case K", in: "1K", want: kib},
				{name: "k with a count", in: "4k", want: 4 * kib},
				{name: "m", in: "1m", want: mib},
				{name: "upper case M", in: "1M", want: mib},
				{name: "g", in: "1g", want: gib},
				{name: "upper case G", in: "1G", want: gib},
				{name: "t", in: "1t", want: tib},
				{name: "upper case T", in: "1T", want: tib},
				{name: "p", in: "1p", want: pib},
				{name: "upper case P", in: "1P", want: pib},
				{name: "e", in: "1e", want: eib},
				{name: "upper case E", in: "1E", want: eib},
				{name: "zero with a suffix", in: "0K", want: 0},
				{name: "the largest exact power", in: "7E", want: 7 * eib},
			},
		},
		{
			name: "decimal prefixes",
			tests: []parseTest{
				{name: "KB", in: "1KB", want: kb},
				{name: "kB", in: "1kB", want: kb},
				{name: "kb", in: "1kb", want: kb},
				{name: "Kb", in: "1Kb", want: kb},
				{name: "MB", in: "1MB", want: mb},
				{name: "mB", in: "1mB", want: mb},
				{name: "GB", in: "1GB", want: gb},
				{name: "gB", in: "1gB", want: gb},
				{name: "TB", in: "1TB", want: tb},
				{name: "PB", in: "1PB", want: pb},
				{name: "EB", in: "1EB", want: eb},
				{name: "EB with a count", in: "8EB", want: 8 * eb},
			},
		},
		{
			name: "binary prefixes",
			tests: []parseTest{
				{name: "KiB", in: "1KiB", want: kib},
				{name: "kiB", in: "1kiB", want: kib},
				{name: "kib", in: "1kib", want: kib},
				{name: "KIB", in: "1KIB", want: kib},
				{name: "MiB", in: "1MiB", want: mib},
				{name: "mib", in: "1mib", want: mib},
				{name: "GiB", in: "1GiB", want: gib},
				{name: "TiB", in: "1TiB", want: tib},
				{name: "PiB", in: "1PiB", want: pib},
				{name: "EiB", in: "1EiB", want: eib},
				{name: "EiB with a count", in: "7EiB", want: 7 * eib},
			},
		},
		{
			name: "suffixes too large for an int64",
			tests: []parseTest{
				{name: "Z", in: "1Z", wantErr: "invalid size: '1Z': suffix too large: 'Z'"},
				{name: "Y", in: "1Y", wantErr: "invalid size: '1Y': suffix too large: 'Y'"},
				{name: "R", in: "1R", wantErr: "invalid size: '1R': suffix too large: 'R'"},
				{name: "Q", in: "1Q", wantErr: "invalid size: '1Q': suffix too large: 'Q'"},
				{name: "z", in: "1z", wantErr: "invalid size: '1z': suffix too large: 'z'"},
				{name: "ZiB", in: "1ZiB", wantErr: "invalid size: '1ZiB': suffix too large: 'ZiB'"},
				{name: "QB", in: "1QB", wantErr: "invalid size: '1QB': suffix too large: 'QB'"},
				{name: "even for zero", in: "0Z", wantErr: "invalid size: '0Z': suffix too large: 'Z'"},
			},
		},
		{
			name: "products which do not fit in an int64",
			tests: []parseTest{
				{name: "eight EiB", in: "8E", wantErr: "size too large: '8E'"},
				{name: "ten EiB", in: "10EiB", wantErr: "size too large: '10EiB'"},
				{name: "a huge EB", in: "9223372036854776KB", wantErr: "size too large: '9223372036854776KB'"},
				{name: "a huge block count", in: "18014398509481984b", wantErr: "size too large: '18014398509481984b'"},
				{name: "a huge word count", in: "9223372036854775807w", wantErr: "size too large: '9223372036854775807w'"},
			},
		},
		{
			name: "sizes which just fit in an int64",
			tests: []parseTest{
				{name: "the largest KB product", in: "9223372036854775KB", want: 9223372036854775000},
				{name: "the largest block count", in: "18014398509481983b", want: 9223372036854775296},
				{name: "nine EB", in: "9EB", want: 9000000000000000000},
			},
		},
		{
			name: "invalid forms",
			tests: []parseTest{
				{name: "empty", in: "", wantErr: "invalid size: ''"},
				{name: "no digits", in: "x", wantErr: "invalid size: 'x'"},
				{name: "a suffix alone", in: "K", wantErr: "invalid size: 'K'"},
				{name: "a block suffix alone", in: "b", wantErr: "invalid size: 'b'"},
				{name: "a plus sign", in: "+5", wantErr: "invalid size: '+5'"},
				{name: "a minus sign", in: "-5", wantErr: "invalid size: '-5'"},
				{name: "leading blanks", in: " 5", wantErr: "invalid size: ' 5'"},
				{name: "trailing blanks", in: "5 ", wantErr: "invalid size: '5 ': invalid suffix: ' '"},
				{name: "a decimal number", in: "4.5", wantErr: "invalid size: '4.5': invalid suffix: '.5'"},
				{name: "a hexadecimal number", in: "0x10", wantErr: "invalid size: '0x10': invalid suffix: 'x10'"},
				{name: "an unknown letter", in: "1x", wantErr: "invalid size: '1x': invalid suffix: 'x'"},
				{name: "two letters which are no prefix", in: "1KbB", wantErr: "invalid size: '1KbB': invalid suffix: 'KbB'"},
				{name: "a repeated B", in: "1BB", wantErr: "invalid size: '1BB': invalid suffix: 'BB'"},
				{name: "an incomplete binary prefix", in: "1Ki", wantErr: "invalid size: '1Ki': invalid suffix: 'Ki'"},
				{name: "a binary prefix of the wrong letter", in: "1KBx", wantErr: "invalid size: '1KBx': invalid suffix: 'KBx'"},
				{name: "a decimal exponent", in: "1e5", wantErr: "invalid size: '1e5': invalid suffix: 'e5'"},
				{name: "four letters", in: "1KiBB", wantErr: "invalid size: '1KiBB': invalid suffix: 'KiBB'"},
			},
		},
	}

	for _, group := range groups {
		t.Run(group.name, func(t *testing.T) {
			for _, test := range group.tests {
				t.Run(test.name, func(t *testing.T) {
					got, err := Parse(test.in)
					if err != nil {
						if err.Error() != test.wantErr {
							t.Fatalf("Parse(%q) error = %q, want %q", test.in, err, test.wantErr)
						}
						return
					}
					if test.wantErr != "" {
						t.Fatalf("Parse(%q) error = nil, want %q", test.in, test.wantErr)
					}
					if got != test.want {
						t.Errorf("Parse(%q) = %d, want %d", test.in, got, test.want)
					}
				})
			}
		})
	}
}

func TestParseNumber(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    int64
		wantNeg bool
		wantErr string
	}{
		{name: "no sign", in: "5", want: 5},
		{name: "zero", in: "0", want: 0},
		{name: "an explicit plus", in: "+5", want: 5},
		{name: "a minus", in: "-5", want: 5, wantNeg: true},
		{name: "minus zero", in: "-0", want: 0, wantNeg: true},
		{name: "a minus with a suffix", in: "-1K", want: 1024, wantNeg: true},
		{name: "a plus with a decimal prefix", in: "+2KB", want: 2000},
		{name: "a minus with a binary prefix", in: "-3MiB", want: 3 << 20, wantNeg: true},
		{name: "a minus sign alone", in: "-", wantErr: "invalid size: ''"},
		{name: "a plus sign alone", in: "+", wantErr: "invalid size: ''"},
		{name: "two minus signs", in: "--5", wantErr: "invalid size: '-5'"},
		{name: "a plus and a minus", in: "+-5", wantErr: "invalid size: '-5'"},
		{name: "a minus before an unknown suffix", in: "-1x", wantErr: "invalid size: '1x': invalid suffix: 'x'"},
		{name: "a minus before a huge number", in: "-9223372036854775808", wantErr: "invalid size: '9223372036854775808'"},
		{name: "a minus before a huge product", in: "-8E", wantErr: "size too large: '8E'"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, neg, err := ParseNumber(test.in)
			if err != nil {
				if err.Error() != test.wantErr {
					t.Fatalf("ParseNumber(%q) error = %q, want %q", test.in, err, test.wantErr)
				}
				return
			}
			if test.wantErr != "" {
				t.Fatalf("ParseNumber(%q) error = nil, want %q", test.in, test.wantErr)
			}
			if got != test.want || neg != test.wantNeg {
				t.Errorf("ParseNumber(%q) = %d, %v, want %d, %v", test.in, got, neg, test.want, test.wantNeg)
			}
		})
	}
}
