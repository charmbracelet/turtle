// Package sizeparse parses GNU-style size operands shared by the utilities
// which take sizes with multiplicative suffixes, like the bytes of head, tail,
// split and od, or the block size of ls.
package sizeparse

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// suffixPowers maps each multiplier letter to the power it stands for, so
// that 'M' is the second power of 1024 and 'MB' the second of 1000.
var suffixPowers = map[byte]int{
	'k': 1, 'm': 2, 'g': 3, 't': 4, 'p': 5,
	'e': 6, 'z': 7, 'y': 8, 'r': 9, 'q': 10,
}

// binary holds the powers of 1024 which fit in an int64, decimal the powers
// of 1000, each keyed by their power.
var binary, decimal = powersOf(1024), powersOf(1000)

// powersOf returns the powers of base up to the largest which fits in an
// int64, keyed by their power.
func powersOf(base int64) map[int]int64 {
	mults := make(map[int]int64)
	mult := int64(1)
	for power := 1; mult <= math.MaxInt64/base; power++ {
		mult *= base
		mults[power] = mult
	}
	return mults
}

// Multiplier returns the multiplier a size suffix stands for, as 'b' for
// 512, 'K' for 1024, 'KB' for 1000 and 'KiB' for 1024 again. The letters of
// the decimal and binary prefixes may be given in any case, except that
// 'b', 'c' and 'w', which stand for 512, 1 and 2, are only recognized in
// lower case.
func Multiplier(suffix string) (int64, error) {
	switch suffix {
	case "":
		return 1, nil
	case "b":
		return 512, nil
	case "c":
		return 1, nil
	case "w":
		return 2, nil
	}
	s := strings.ToLower(suffix)
	var mults map[int]int64
	var base int64
	switch len(s) {
	case 1:
		base, mults = 1024, binary
	case 2:
		if s[1] != 'b' {
			return 0, fmt.Errorf("invalid suffix: '%s'", suffix)
		}
		base, mults = 1000, decimal
	case 3:
		if s[1] != 'i' || s[2] != 'b' {
			return 0, fmt.Errorf("invalid suffix: '%s'", suffix)
		}
		base, mults = 1024, binary
	default:
		return 0, fmt.Errorf("invalid suffix: '%s'", suffix)
	}
	power, ok := Power(s[0], base)
	if power == 0 {
		return 0, fmt.Errorf("invalid suffix: '%s'", suffix)
	}
	if !ok {
		return 0, fmt.Errorf("suffix too large: '%s'", suffix)
	}
	return mults[power], nil
}

// Power returns the power of base a size suffix letter stands for, as 1
// for 'K' or 'k' in base 1024, and reports whether the multiplier
// base**power holds in an int64. The letter is case-insensitive, as GNU
// takes it. A power of 0 means the letter is not a size suffix letter.
func Power(letter byte, base int64) (int, bool) {
	power, ok := suffixPowers[lower(letter)]
	if !ok {
		return 0, false
	}
	mult := int64(1)
	for range power {
		if mult > math.MaxInt64/base {
			return power, false
		}
		mult *= base
	}
	return power, true
}

// lower returns the lower case of an ASCII letter.
func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// Parse parses a non-negative size: a decimal integer optionally followed by
// a multiplicative suffix, as "10K" for 10240 or "1MB" for 1000000. The
// returned error names the operand which is not a valid size.
func Parse(s string) (int64, error) {
	digits := s
	for digits != "" && digits[0] >= '0' && digits[0] <= '9' {
		digits = digits[1:]
	}
	n, err := strconv.ParseInt(s[:len(s)-len(digits)], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size: '%s'", s)
	}
	mult, err := Multiplier(digits)
	if err != nil {
		return 0, fmt.Errorf("invalid size: '%s': %w", s, err)
	}
	if mult > 1 && n > math.MaxInt64/mult {
		return 0, fmt.Errorf("size too large: '%s'", s)
	}
	return n * mult, nil
}

// ParseNumber parses a size with an optional leading '+' or '-', which has
// no meaning of its own: it returns the magnitude of the size and whether
// the sign was '-'. Utilities keep the interpretation of the sign, as it
// differs from one to the next.
func ParseNumber(s string) (n int64, neg bool, err error) {
	if strings.HasPrefix(s, "-") {
		neg, s = true, s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	n, err = Parse(s)
	if err != nil {
		return 0, false, err
	}
	return n, neg, nil
}
