package durationparse

import (
	"testing"
	"time"
)

type parseTest struct {
	name  string
	in    string
	want  time.Duration
	infin bool
	err   bool
}

func TestParseWithoutInfinity(t *testing.T) {
	tests := []parseTest{
		{name: "zero", in: "0", want: 0},
		{name: "seconds", in: "5", want: 5 * time.Second},
		{name: "a fraction", in: "1.5", want: 1500 * time.Millisecond},
		{name: "a bare fraction", in: ".5", want: 500 * time.Millisecond},
		{name: "the seconds unit", in: "60s", want: time.Minute},
		{name: "the minutes unit", in: "2m", want: 2 * time.Minute},
		{name: "the hours unit", in: "1h", want: time.Hour},
		{name: "the days unit", in: "1d", want: 24 * time.Hour},
		{name: "a hex float", in: "0x1p4", want: 16 * time.Second},
		{name: "the largest seconds", in: "9223372036", want: 9223372036 * time.Second},
		{name: "seconds just above the largest", in: "9223372036.854775", err: true},
		{name: "a fraction too small to hold", in: "1e-10", want: 0},
		{name: "an underflow reads as zero", in: "1e-400", want: 0},
		{name: "infinity", in: "inf", err: true},
		{name: "a negative infinity", in: "-inf", err: true},
		{name: "a not a number", in: "nan", err: true},
		{name: "a negative number", in: "-1", err: true},
		{name: "an overflow", in: "1e400", err: true},
		{name: "seconds too many", in: "106752d", err: true},
		{name: "a bare unit", in: "d", err: true},
		{name: "an empty operand", in: "", err: true},
		{name: "a syntax error", in: "1x", err: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			check(t, test, false)
		})
	}
}

func TestParseWithInfinity(t *testing.T) {
	tests := []parseTest{
		{name: "zero", in: "0", want: 0},
		{name: "seconds", in: "5", want: 5 * time.Second},
		{name: "the days unit", in: "1d", want: 24 * time.Hour},
		{name: "infinity", in: "inf", want: 0, infin: true},
		{name: "an uppercase infinity", in: "INF", want: 0, infin: true},
		{name: "an overflow", in: "1e400", want: 0, infin: true},
		{name: "seconds too many", in: "106752d", want: 0, infin: true},
		{name: "minutes too many", in: "1e10m", want: 0, infin: true},
		{name: "a fraction too small to hold", in: "1e-10", want: time.Nanosecond},
		{name: "an underflow", in: "1e-400", want: time.Nanosecond},
		{name: "an underflow with a unit", in: "1e-400m", want: time.Nanosecond},
		{name: "a hex underflow", in: "0xap-2000", want: time.Nanosecond},
		{name: "a zero with a unit", in: "0s", want: 0},
		{name: "a negative infinity", in: "-inf", err: true},
		{name: "a negative number", in: "-1", err: true},
		{name: "a negative underflow", in: "-1e-400", err: true},
		{name: "a not a number", in: "nan", err: true},
		{name: "an empty operand", in: "", err: true},
		{name: "a bare unit", in: "d", err: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			check(t, test, true)
		})
	}
}

func check(t *testing.T, test parseTest, infOK bool) {
	t.Helper()
	d, infin, err := Parse(test.in, infOK)
	if test.err {
		if err != ErrInvalid {
			t.Fatalf("Parse(%q, %v) error = %v, want %v", test.in, infOK, err, ErrInvalid)
		}
		return
	}
	if err != nil {
		t.Fatalf("Parse(%q, %v) error = %v, want none", test.in, infOK, err)
	}
	if d != test.want || infin != test.infin {
		t.Errorf("Parse(%q, %v) = %v, %v, want %v, %v",
			test.in, infOK, d, infin, test.want, test.infin)
	}
}
