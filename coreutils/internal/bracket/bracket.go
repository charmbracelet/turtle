// Package bracket translates the POSIX bracket expressions shared by the
// basic and extended regular expressions.
package bracket

import (
	"fmt"
	"strings"
)

// End returns the index of the ] closing the bracket expression starting
// at i, or -1 when there's none. A ] right after the opening [ or a
// leading ^ is a member of the expression, and the ] closing a [:...:],
// [.x.] or [=x=] subexpression doesn't close the expression.
func End(pattern string, i int) int {
	j := i + 1
	if j < len(pattern) && pattern[j] == '^' {
		j++
	}
	if j < len(pattern) && pattern[j] == ']' {
		j++
	}
	for ; j < len(pattern); j++ {
		if pattern[j] == ']' {
			return j
		}
		if end := subEnd(pattern, j); end >= 0 {
			j = end
		}
	}
	return -1
}

// subEnd returns the index of the ] closing the [:...:], [.x.] or [=x=]
// subexpression starting at j, or -1 when the bracket at j doesn't open one.
func subEnd(pattern string, j int) int {
	if j+1 >= len(pattern) {
		return -1
	}
	var marker byte
	switch pattern[j+1] {
	case ':', '.', '=':
		marker = pattern[j+1]
	default:
		return -1
	}
	for k := j + 2; k+1 < len(pattern); k++ {
		if pattern[k] == marker && pattern[k+1] == ']' {
			return k + 1
		}
	}
	return -1
}

// Translate converts a bracket expression into the syntax of Go's regexp
// package: backslashes are ordinary characters there, and so they're
// escaped, while the collating symbols and equivalence classes of a single
// character stand for that character. A multi-character collating symbol
// can't be expressed in that syntax and is an error.
func Translate(expr string) (string, error) {
	var out strings.Builder
	out.WriteByte(expr[0])
	j := 1
	if j < len(expr)-1 && expr[j] == '^' {
		out.WriteByte('^')
		j++
	}
	if j < len(expr)-1 && expr[j] == ']' {
		out.WriteString(`\]`)
		j++
	}
	for ; j < len(expr)-1; j++ {
		end := subEnd(expr, j)
		if end < 0 {
			writeMember(&out, expr[j])
			continue
		}
		if expr[j+1] == ':' {
			out.WriteString(expr[j : end+1])
			j = end
			continue
		}
		if end-j != 4 {
			return "", fmt.Errorf("invalid collating element %s", expr[j:end+1])
		}
		writeMember(&out, expr[j+2])
		j = end
	}
	out.WriteByte(']')
	return out.String(), nil
}

// writeMember appends one character of a bracket expression to out, escaping
// it when the syntax of Go's regexp package gives it a meaning, as it does
// for a backslash or a closing bracket.
func writeMember(out *strings.Builder, c byte) {
	switch c {
	case '\\', ']':
		out.WriteByte('\\')
		out.WriteByte(c)
	default:
		out.WriteByte(c)
	}
}
