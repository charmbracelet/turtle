// Package ere compiles POSIX extended regular expressions.
//
// It translates an ERE pattern to the syntax understood by Go's regexp
// package, which is then compiled. Backreferences and multi-character
// collating symbols are not supported.
package ere

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/turtle/coreutils/internal/bracket"
	"charm.land/turtle/coreutils/internal/rex"
)

// Compile parses pattern as a POSIX extended regular expression and returns
// the compiled Go regular expression.
func Compile(pattern string) (*regexp.Regexp, error) {
	translated, err := Translate(pattern)
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

// translate converts an ERE pattern to Go regexp syntax, which is mostly
// the same syntax. The differences handled here: {m,n} interval expressions
// are errors rather than literals when invalid, since Go accepts a lone { as
// an ordinary character, and a backslash before an ordinary character, which
// POSIX leaves undefined, is kept so the escapes shared with the BREs, like
// \b and \w, still work.
func translate(pattern string) (string, error) {
	var out strings.Builder
	atom := false // whether an atom precedes a possible repetition
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
		case '{':
			if !atom {
				out.WriteString(`\{`)
				i++
				continue
			}
			end, content, ok := rex.Interval(pattern, i+1, false)
			if !ok {
				return "", fmt.Errorf("unmatched \\{")
			}
			out.WriteByte('{')
			out.WriteString(content)
			out.WriteByte('}')
			i = end
			atom = false
		case '\\':
			if i+1 >= len(pattern) {
				out.WriteString(`\\`)
				i++
				atom = true
				continue
			}
			d := pattern[i+1]
			if d >= '1' && d <= '9' {
				return "", fmt.Errorf("backreferences are not supported")
			}
			if rex.IsRegexpMeta(d) || rex.IsLetter(d) {
				out.WriteByte('\\')
				out.WriteByte(d)
			} else {
				out.WriteByte(d)
			}
			i += 2
			atom = true
		default:
			out.WriteByte(c)
			i++
			switch c {
			case '*', '+', '?', '|', '(':
				atom = false
			default:
				atom = true
			}
		}
	}
	return out.String(), nil
}
