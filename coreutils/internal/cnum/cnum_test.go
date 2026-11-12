package cnum

import "testing"

func TestScan(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		allowSign  bool
		wantDigits string
		wantBase   int
		wantNext   int
		wantOK     bool
	}{
		{name: "a decimal number", in: "42", wantDigits: "42", wantBase: 10, wantNext: 2, wantOK: true},
		{name: "a decimal number with a suffix", in: "42K", wantDigits: "42", wantBase: 10, wantNext: 2, wantOK: true},
		{name: "a leading zero makes octal", in: "010", wantDigits: "010", wantBase: 8, wantNext: 3, wantOK: true},
		{name: "a lone zero is decimal", in: "0", wantDigits: "0", wantBase: 10, wantNext: 1, wantOK: true},
		{name: "a zero and a suffix", in: "0K", wantDigits: "0", wantBase: 8, wantNext: 1, wantOK: true},
		{name: "an octal digit stops the number", in: "09", wantDigits: "0", wantBase: 8, wantNext: 1, wantOK: true},
		{name: "a lower case hex prefix", in: "0x1F", wantDigits: "1F", wantBase: 16, wantNext: 4, wantOK: true},
		{name: "an upper case hex prefix", in: "0X1f", wantDigits: "1f", wantBase: 16, wantNext: 4, wantOK: true},
		{name: "a hex number with a suffix", in: "0x10K", wantDigits: "10", wantBase: 16, wantNext: 4, wantOK: true},
		{name: "a hex prefix without digits", in: "0x", wantDigits: "", wantBase: 16, wantNext: 2, wantOK: false},
		{name: "a hex prefix and a suffix", in: "0xK", wantDigits: "", wantBase: 16, wantNext: 2, wantOK: false},
		{name: "a second hex prefix", in: "00x1", wantDigits: "00", wantBase: 8, wantNext: 2, wantOK: true},
		{name: "a suffix alone", in: "K", wantDigits: "", wantBase: 10, wantNext: 0, wantOK: false},
		{name: "an empty string", in: "", wantDigits: "", wantBase: 10, wantNext: 0, wantOK: false},
		{name: "a plus sign, when allowed", in: "+42", allowSign: true, wantDigits: "42", wantBase: 10, wantNext: 3, wantOK: true},
		{name: "a minus sign, when allowed", in: "-0x10", allowSign: true, wantDigits: "10", wantBase: 16, wantNext: 5, wantOK: true},
		{name: "a plus sign alone, when allowed", in: "+", allowSign: true, wantDigits: "", wantBase: 10, wantNext: 1, wantOK: false},
		{name: "a plus sign, when not allowed", in: "+42", wantDigits: "", wantBase: 10, wantNext: 0, wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			digits, base, next, ok := Scan(test.in, test.allowSign)
			if digits != test.wantDigits || base != test.wantBase ||
				next != test.wantNext || ok != test.wantOK {
				t.Errorf("Scan(%q, %v) = %q, %d, %d, %v, want %q, %d, %d, %v",
					test.in, test.allowSign, digits, base, next, ok,
					test.wantDigits, test.wantBase, test.wantNext, test.wantOK)
			}
		})
	}
}
