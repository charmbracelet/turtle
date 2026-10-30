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

// multiplier returns the multiplier a size suffix stands for, as 'b' for 512,
// 'K' for 1024, 'KB' for 1000 and 'KiB' for 1024 again. The letters of the
// decimal and binary prefixes may be given in any case, except that 'b', 'c'
// and 'w', which stand for 512, 1 and 2, are only recognized in lower case.
func multiplier(suffix string) (int64, error) {
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
	power, ok := suffixPowers[s[0]]
	if !ok {
		return 0, fmt.Errorf("invalid suffix: '%s'", suffix)
	}
	var mults map[int]int64
	switch len(s) {
	case 1:
		mults = binary
	case 2:
		if s[1] != 'b' {
			return 0, fmt.Errorf("invalid suffix: '%s'", suffix)
		}
		mults = decimal
	case 3:
		if s[1] != 'i' || s[2] != 'b' {
			return 0, fmt.Errorf("invalid suffix: '%s'", suffix)
		}
		mults = binary
	default:
		return 0, fmt.Errorf("invalid suffix: '%s'", suffix)
	}
	mult, ok := mults[power]
	if !ok {
		return 0, fmt.Errorf("suffix too large: '%s'", suffix)
	}
	return mult, nil
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
	mult, err := multiplier(digits)
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
