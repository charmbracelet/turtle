// Package textutil implements the line handling shared by the text processing
// utilities, like skipping the fields or characters of a line.
package textutil

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"unicode/utf8"
)

// Delim returns the record separator of an input: a NUL when zero is set, a
// newline otherwise.
func Delim(zero bool) byte {
	if zero {
		return 0
	}
	return '\n'
}

// FoldCase lowercases the ASCII letters of s, leaving every other byte as
// it is, so the result compares as the case-folded s would.
func FoldCase(s []byte) []byte {
	folded := make([]byte, len(s))
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		folded[i] = c
	}
	return folded
}

// Each reads the records of r, each including its trailing separator when
// present; the final record may lack one. fn is called once per record, in
// order, and its first error, like the context done error of ctx, stops the
// reading and is returned as it is.
func Each(ctx context.Context, r io.Reader, sep byte, fn func(rec []byte) error) error {
	br := bufio.NewReader(r)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rec, err := br.ReadBytes(sep)
		if len(rec) > 0 {
			if ferr := fn(rec); ferr != nil {
				return ferr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

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

// Fields splits line into its fields, with sep as the field separator. With
// a nil sep, a field is a maximal run of non-blank characters: leading
// blanks are ignored and a run of blanks counts as one separator. Otherwise
// every separator counts, so fields can be empty.
func Fields(line, sep []byte) [][]byte {
	if sep != nil {
		return bytes.Split(line, sep)
	}
	var fields [][]byte
	for i := 0; i < len(line); {
		for i < len(line) && IsBlank(line[i]) {
			i++
		}
		if i == len(line) {
			break
		}
		start := i
		for i < len(line) && !IsBlank(line[i]) {
			i++
		}
		fields = append(fields, line[start:i])
	}
	return fields
}
