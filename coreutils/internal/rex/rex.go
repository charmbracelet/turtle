// Package rex holds the helpers shared by the basic and extended regular
// expression translators.
package rex

// IsLetter reports whether c is an ASCII letter or digit.
func IsLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// IsRegexpMeta reports whether c is a metacharacter in Go regexp syntax.
func IsRegexpMeta(c byte) bool {
	switch c {
	case '.', '*', '+', '?', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
		return true
	}
	return false
}

// Interval parses a {m}, {m,} or {m,n} interval expression starting at i
// (right after the opening brace), returning the index just past the closing
// brace and the content between braces. The closing brace is a \} when
// escapedClose, as in a BRE. ok is false when the expression isn't a valid
// interval, in which case it's ordinary text.
func Interval(pattern string, i int, escapedClose bool) (end int, content string, ok bool) {
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
	if escapedClose {
		if j+1 >= len(pattern) || pattern[j] != '\\' || pattern[j+1] != '}' {
			return 0, "", false
		}
		return j + 2, content, true
	}
	if j >= len(pattern) || pattern[j] != '}' {
		return 0, "", false
	}
	return j + 1, content, true
}
