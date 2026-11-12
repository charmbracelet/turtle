// Package numparse holds the ASCII digit predicates and the digits-only
// integer parsing shared by the utilities, so each of them keeps its own
// range checks, wording and overflow policy on top of them.
package numparse

import (
	"errors"
	"strconv"
	"strings"
)

// IsDigit reports whether c is a decimal digit.
func IsDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

// IsOctalDigit reports whether c is an octal digit.
func IsOctalDigit(c byte) bool {
	return '0' <= c && c <= '7'
}

// IsHexDigit reports whether c is a hexadecimal digit.
func IsHexDigit(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// DigitValue returns the decimal value of the digit c, or -1 when c isn't
// one.
func DigitValue(c byte) int {
	if !IsDigit(c) {
		return -1
	}
	return int(c - '0')
}

// AllDigits reports whether s holds one or more decimal digits.
func AllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !IsDigit(s[i]) {
			return false
		}
	}
	return true
}

// Unsigned parses a decimal integer made of digits only, preserving the
// strconv range errors.
func Unsigned(s string) (int64, error) {
	if !AllDigits(s) {
		return 0, errors.New("not a number")
	}
	return strconv.ParseInt(s, 10, 64)
}

// Signed parses an integer constant: an optional leading '-' followed by
// decimal digits, preserving the strconv range errors.
func Signed(s string) (int64, error) {
	if !AllDigits(strings.TrimPrefix(s, "-")) {
		return 0, errors.New("not an integer")
	}
	return strconv.ParseInt(s, 10, 64)
}
