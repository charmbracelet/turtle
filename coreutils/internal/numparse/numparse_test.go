package numparse

import (
	"errors"
	"strconv"
	"testing"
)

func TestIsDigit(t *testing.T) {
	tests := []struct {
		name string
		in   byte
		want bool
	}{
		{name: "zero", in: '0', want: true},
		{name: "nine", in: '9', want: true},
		{name: "before zero", in: '/', want: false},
		{name: "after nine", in: ':', want: false},
		{name: "a letter", in: 'a', want: false},
		{name: "a high byte", in: 0xF0, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDigit(tt.in); got != tt.want {
				t.Errorf("IsDigit(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsOctalDigit(t *testing.T) {
	tests := []struct {
		name string
		in   byte
		want bool
	}{
		{name: "zero", in: '0', want: true},
		{name: "seven", in: '7', want: true},
		{name: "eight", in: '8', want: false},
		{name: "nine", in: '9', want: false},
		{name: "a letter", in: 'a', want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOctalDigit(tt.in); got != tt.want {
				t.Errorf("IsOctalDigit(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsHexDigit(t *testing.T) {
	tests := []struct {
		name string
		in   byte
		want bool
	}{
		{name: "zero", in: '0', want: true},
		{name: "nine", in: '9', want: true},
		{name: "lowercase a", in: 'a', want: true},
		{name: "lowercase f", in: 'f', want: true},
		{name: "uppercase A", in: 'A', want: true},
		{name: "uppercase F", in: 'F', want: true},
		{name: "after lowercase f", in: 'g', want: false},
		{name: "after uppercase F", in: 'G', want: false},
		{name: "before zero", in: '/', want: false},
		{name: "colon", in: ':', want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHexDigit(tt.in); got != tt.want {
				t.Errorf("IsHexDigit(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestDigitValue(t *testing.T) {
	tests := []struct {
		name string
		in   byte
		want int
	}{
		{name: "zero", in: '0', want: 0},
		{name: "nine", in: '9', want: 9},
		{name: "a letter", in: 'a', want: -1},
		{name: "a slash", in: '/', want: -1},
		{name: "a colon", in: ':', want: -1},
		{name: "a high byte", in: 0xFF, want: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DigitValue(tt.in); got != tt.want {
				t.Errorf("DigitValue(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestAllDigits(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "empty", in: "", want: false},
		{name: "one digit", in: "5", want: true},
		{name: "several digits", in: "4096", want: true},
		{name: "leading zeros", in: "007", want: true},
		{name: "a sign", in: "-5", want: false},
		{name: "a plus sign", in: "+5", want: false},
		{name: "a trailing letter", in: "5a", want: false},
		{name: "a space", in: "5 ", want: false},
		{name: "a high byte", in: "5\xff", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AllDigits(tt.in); got != tt.want {
				t.Errorf("AllDigits(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

type parseTest struct {
	name string
	in   string
	want int64
	// wantErr is the exact error message expected, and rangeErr reports a
	// strconv range error instead, so its wording stays out of the test.
	wantErr  string
	rangeErr bool
}

func TestUnsigned(t *testing.T) {
	groups := []struct {
		name  string
		tests []parseTest
	}{
		{
			name: "valid numbers",
			tests: []parseTest{
				{name: "zero", in: "0", want: 0},
				{name: "one", in: "1", want: 1},
				{name: "several digits", in: "123456", want: 123456},
				{name: "leading zeros", in: "007", want: 7},
				{name: "the largest int64", in: "9223372036854775807", want: 9223372036854775807},
			},
		},
		{
			name: "invalid numbers",
			tests: []parseTest{
				{name: "empty", in: "", wantErr: "not a number"},
				{name: "a sign", in: "-1", wantErr: "not a number"},
				{name: "a plus sign", in: "+1", wantErr: "not a number"},
				{name: "a letter", in: "1a", wantErr: "not a number"},
				{name: "a space", in: " 1", wantErr: "not a number"},
				{name: "a high byte", in: "1\xff", wantErr: "not a number"},
			},
		},
		{
			name: "range errors",
			tests: []parseTest{
				{name: "past the largest int64", in: "9223372036854775808", rangeErr: true},
				{name: "far past the largest int64", in: "999999999999999999999999", rangeErr: true},
			},
		},
	}
	for _, group := range groups {
		t.Run(group.name, func(t *testing.T) {
			for _, tt := range group.tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := Unsigned(tt.in)
					check(t, got, err, tt, "Unsigned")
				})
			}
		})
	}
}

func TestSigned(t *testing.T) {
	groups := []struct {
		name  string
		tests []parseTest
	}{
		{
			name: "valid integers",
			tests: []parseTest{
				{name: "zero", in: "0", want: 0},
				{name: "positive", in: "42", want: 42},
				{name: "negative", in: "-42", want: -42},
				{name: "negative zero", in: "-0", want: 0},
				{name: "leading zeros", in: "-007", want: -7},
				{name: "the smallest int64", in: "-9223372036854775808", want: -9223372036854775808},
				{name: "the largest int64", in: "9223372036854775807", want: 9223372036854775807},
			},
		},
		{
			name: "invalid integers",
			tests: []parseTest{
				{name: "empty", in: "", wantErr: "not an integer"},
				{name: "minus only", in: "-", wantErr: "not an integer"},
				{name: "a plus sign", in: "+1", wantErr: "not an integer"},
				{name: "two signs", in: "--1", wantErr: "not an integer"},
				{name: "a letter", in: "1a", wantErr: "not an integer"},
				{name: "a space", in: " 1", wantErr: "not an integer"},
			},
		},
		{
			name: "range errors",
			tests: []parseTest{
				{name: "past the largest int64", in: "9223372036854775808", rangeErr: true},
				{name: "past the smallest int64", in: "-9223372036854775809", rangeErr: true},
			},
		},
	}
	for _, group := range groups {
		t.Run(group.name, func(t *testing.T) {
			for _, tt := range group.tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := Signed(tt.in)
					check(t, got, err, tt, "Signed")
				})
			}
		})
	}
}

func check(t *testing.T, got int64, err error, tt parseTest, fn string) {
	t.Helper()
	switch {
	case tt.rangeErr:
		var numErr *strconv.NumError
		if !errors.As(err, &numErr) || numErr.Err != strconv.ErrRange {
			t.Errorf("%s(%q) error = %v, want a strconv range error", fn, tt.in, err)
		}
	case tt.wantErr != "":
		if err == nil || err.Error() != tt.wantErr {
			t.Errorf("%s(%q) error = %v, want %q", fn, tt.in, err, tt.wantErr)
		}
	case err != nil:
		t.Errorf("%s(%q) unexpected error: %v", fn, tt.in, err)
	case got != tt.want:
		t.Errorf("%s(%q) = %d, want %d", fn, tt.in, got, tt.want)
	}
}
