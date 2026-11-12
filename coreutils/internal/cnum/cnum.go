// Package cnum scans the C-style integers of the core utility operands,
// with the base of the digits detected the way C takes it: hexadecimal
// with a 0x or 0X prefix, octal with a leading 0, and decimal otherwise.
package cnum

import "strings"

// Scan scans the integer at the start of s, returning the digits of its
// number without the base prefix, the base they are in, and the offset in
// s of the first byte after the number. ok reports whether the number has
// at least one digit, as a lone "0x" prefix or a plain suffix letter
// doesn't. When allowSign is set, a leading '+' or '-' is skipped and
// counted in the offset.
func Scan(s string, allowSign bool) (digits string, base, next int, ok bool) {
	i := 0
	if allowSign && len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		i++
	}
	base = 10
	if strings.HasPrefix(s[i:], "0x") || strings.HasPrefix(s[i:], "0X") {
		base, i = 16, i+2
	} else if i+1 < len(s) && s[i] == '0' {
		base = 8
	}
	start := i
	for i < len(s) && digitValue(s[i]) < base {
		i++
	}
	return s[start:i], base, i, i > start
}

func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 99
}
