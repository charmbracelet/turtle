// Package glob compiles shell filename expansion patterns, as the pattern
// arguments of find and the file selection options of grep take them, into
// regular expressions.
package glob

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/turtle/coreutils/internal/bracket"
)

// Compile compiles a pattern into a regular expression that reports whether
// a string matches the whole pattern. Unlike a filename expansion pattern, a
// '*' or '?' matches a slash, and a leading dot is not special. A backslash
// quotes a wildcard or another backslash.
func Compile(pattern string, ignoreCase bool) (*regexp.Regexp, error) {
	var out strings.Builder
	if ignoreCase {
		out.WriteString("(?i)")
	}
	out.WriteString(`\A(?:`)
	for i := 0; i < len(pattern); {
		switch c := pattern[i]; c {
		case '*':
			out.WriteString(".*")
			i++
		case '?':
			out.WriteByte('.')
			i++
		case '[':
			end := bracket.End(pattern, i)
			if end < 0 {
				out.WriteString(`\[`)
				i++
				continue
			}
			t, err := bracket.Translate(pattern[i : end+1])
			if err != nil {
				return nil, fmt.Errorf("invalid pattern '%s'", pattern)
			}
			out.WriteString(t)
			i = end + 1
		case '\\':
			if i+1 < len(pattern) {
				out.WriteString(regexp.QuoteMeta(pattern[i+1 : i+2]))
				i += 2
			} else {
				out.WriteString(`\\`)
				i++
			}
		default:
			out.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
			i++
		}
	}
	out.WriteString(`)\z`)
	re, err := regexp.Compile(out.String())
	if err != nil {
		return nil, fmt.Errorf("invalid pattern '%s'", pattern)
	}
	return re, nil
}
