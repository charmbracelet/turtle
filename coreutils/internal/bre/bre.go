// Package bre compiles POSIX basic regular expressions.
//
// It translates a BRE pattern to the syntax understood by Go's regexp
// package, which is then compiled. Backreferences (\1 through \9) and
// multi-character collating symbols are not supported.
package bre

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/snail/coreutils/internal/bracket"
	"charm.land/snail/coreutils/internal/rex"
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

// Translate converts pattern into the syntax of Go's regexp package.
func Translate(pattern string) (string, error) {
	return translate(pattern)
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
			end := bracket.End(pattern, i)
			if end < 0 {
				return "", fmt.Errorf("unmatched [")
			}
			translated, err := bracket.Translate(pattern[i : end+1])
			if err != nil {
				return "", err
			}
			out.WriteString(translated)
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
				end, content, ok := rex.Interval(pattern, i+2, true)
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
			case rex.IsLetter(d):
				out.WriteByte('\\')
				out.WriteByte(d)
				atom = true
				anchor = false
			case rex.IsRegexpMeta(d):
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
