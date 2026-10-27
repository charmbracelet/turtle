// Package bre compiles POSIX basic regular expressions.
//
// It translates a BRE pattern to the syntax understood by Go's regexp
// package, which is then compiled. Backreferences (\1 through \9) and the
// collating symbols and equivalence classes of bracket expressions are not
// supported.
package bre

import (
	"fmt"
	"regexp"
	"strings"
)

// Compile parses pattern as a POSIX basic regular expression and returns the
// compiled Go regular expression.
func Compile(pattern string) (*regexp.Regexp, error) {
	translated, err := translate(pattern)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(translated)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %v", err)
	}
	return re, nil
}

// translate converts a BRE pattern to Go regexp syntax.
//
// In a BRE, ( ) { } + ? and | are ordinary characters, \( \) group, \{m,n\}
// is an interval expression, * quantifies the preceding atom (it's ordinary
// where no atom precedes it), and ^ and $ are anchors only at the edges of
// the pattern or of a subexpression.
func translate(pattern string) (string, error) {
	var out strings.Builder
	var groups int
	atom := false  // whether an atom precedes a possible * or \{...\}
	anchor := true // whether ^ is an anchor here
	for i := 0; i < len(pattern); {
		c := pattern[i]
		switch c {
		case '[':
			end := bracketEnd(pattern, i)
			if end < 0 {
				return "", fmt.Errorf("unmatched [")
			}
			out.WriteString(translateBracket(pattern[i : end+1]))
			i = end + 1
			atom = true
			anchor = false
		case '^':
			if anchor {
				out.WriteByte('^')
			} else {
				out.WriteString(`\^`)
				atom = true
			}
			i++
			anchor = false
		case '$':
			if i+1 == len(pattern) || (i+2 < len(pattern) && pattern[i+1] == '\\' && pattern[i+2] == ')') {
				out.WriteByte('$')
				atom = false
			} else {
				out.WriteString(`\$`)
				atom = true
			}
			i++
		case '*':
			if atom {
				out.WriteByte('*')
				atom = false
			} else {
				out.WriteString(`\*`)
			}
			i++
			anchor = false
		case '\\':
			if i+1 >= len(pattern) {
				out.WriteString(`\\`)
				i++
				atom = true
				anchor = false
				continue
			}
			d := pattern[i+1]
			switch {
			case d == '(':
				out.WriteByte('(')
				groups++
				atom = false
				anchor = true
			case d == ')':
				if groups == 0 {
					return "", fmt.Errorf("unmatched \\)")
				}
				out.WriteByte(')')
				groups--
				atom = true
				anchor = false
			case d == '{' && atom:
				end, content, ok := interval(pattern, i+2)
				if !ok {
					out.WriteString(`\{`)
					atom = true
					anchor = false
					break
				}
				out.WriteByte('{')
				out.WriteString(content)
				out.WriteByte('}')
				i = end
				atom = false
				anchor = false
			case d >= '1' && d <= '9':
				return "", fmt.Errorf("backreferences are not supported")
			case d == '{':
				out.WriteString(`\{`)
				atom = true
				anchor = false
			case d == '}':
				out.WriteString(`\}`)
				atom = true
				anchor = false
			case isLetter(d):
				out.WriteByte('\\')
				out.WriteByte(d)
				atom = true
				anchor = false
			case isRegexpMeta(d):
				out.WriteByte('\\')
				out.WriteByte(d)
				atom = true
				anchor = false
			default:
				out.WriteByte(d)
				atom = true
				anchor = false
			}
			i += 2
		case '.':
			out.WriteByte('.')
			i++
			atom = true
			anchor = false
		case '(', ')', '{', '}', '+', '?', '|':
			out.WriteByte('\\')
			out.WriteByte(c)
			i++
			atom = true
			anchor = false
		default:
			out.WriteByte(c)
			i++
			atom = true
			anchor = false
		}
	}
	if groups != 0 {
		return "", fmt.Errorf("unmatched \\(")
	}
	return out.String(), nil
}

// isLetter reports whether c is an ASCII letter or digit.
func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// isRegexpMeta reports whether c is a metacharacter in Go regexp syntax.
func isRegexpMeta(c byte) bool {
	switch c {
	case '.', '*', '+', '?', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
		return true
	}
	return false
}

// bracketEnd returns the index of the ] closing the bracket expression
// starting at i, or -1 when there's none. A ] right after the opening [ or a
// leading ^ is a member of the expression, and the ] closing a [:...:],
// [.x.] or [=x=] subexpression doesn't close the expression.
func bracketEnd(pattern string, i int) int {
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

// translateBracket copies a bracket expression, escaping the backslashes it
// holds, since they're ordinary characters in a POSIX bracket expression.
// Collating symbols ([. .]) and equivalence classes ([= =]) are left alone;
// Go's regexp package rejects them.
func translateBracket(expr string) string {
	return strings.ReplaceAll(expr, `\`, `\\`)
}

// interval parses a \{m\}, \{m,\} or \{m,n\} interval expression starting at
// i (right after the backslash and brace), returning the index just past the
// closing brace and the content between braces. ok is false when the
// expression isn't a valid interval, in which case it's ordinary text.
func interval(pattern string, i int) (end int, content string, ok bool) {
	j := i
	for j < len(pattern) && pattern[j] >= '0' && pattern[j] <= '9' {
		j++
	}
	if j == i {
		return 0, "", false
	}
	content = pattern[i:j]
	if j < len(pattern) && pattern[j] == ',' {
		k := j + 1
		for k < len(pattern) && pattern[k] >= '0' && pattern[k] <= '9' {
			k++
		}
		content = pattern[i:k]
		j = k
	}
	if j+1 >= len(pattern) || pattern[j] != '\\' || pattern[j+1] != '}' {
		return 0, "", false
	}
	return j + 2, content, true
}
