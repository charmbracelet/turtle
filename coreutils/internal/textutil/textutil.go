// Package textutil implements the line handling shared by the text processing
// utilities, like skipping the fields or characters of a line.
package textutil

import "unicode/utf8"

// IsBlank reports whether c is a blank character: a space or a tab.
func IsBlank(c byte) bool {
	return c == ' ' || c == '\t'
}

// SkipFields returns the line with its first n fields skipped, where a field
// is a maximal run of blanks followed by a maximal run of non-blanks.
func SkipFields(line []byte, n int) []byte {
	for ; n > 0 && len(line) > 0; n-- {
		i := 0
		for i < len(line) && IsBlank(line[i]) {
			i++
		}
		for i < len(line) && !IsBlank(line[i]) {
			i++
		}
		line = line[i:]
	}
	return line
}

// SkipChars returns the line with its first n characters skipped, decoding it
// as UTF-8. Each byte that doesn't start a valid encoding counts as one
// character.
func SkipChars(line []byte, n int) []byte {
	for ; n > 0 && len(line) > 0; n-- {
		_, size := utf8.DecodeRune(line)
		line = line[size:]
	}
	return line
}
