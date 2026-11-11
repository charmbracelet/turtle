package diff

import "strings"

// KeyOpts describes a line comparison which ignores some of the contents
// of lines, as the white space and case options of diff ask.
type KeyOpts struct {
	IgnoreCase          bool
	IgnoreAllSpace      bool
	IgnoreSpaceChange   bool
	IgnoreTabExpansion  bool
	IgnoreTrailingSpace bool

	// TabSize is the width of the tab stops of IgnoreTabExpansion,
	// 8 when zero.
	TabSize int
}

// Key returns the function which maps lines to the form they're compared
// in, or nil when the options don't ask anything of the comparison.
//
// A line which ends without a newline, which can only be the last one of
// a file, keeps that ending in its key: it compares equal only to another
// line without one, so a missing final newline stays a difference even
// when the contents of the lines are compared loosely. The exceptions are
// the comparisons which already ignore the blanks at the end of a line
// (-Z, -b and -w), where GNU diff treats a missing newline like one more
// trailing blank.
func (o KeyOpts) Key() func(string) string {
	if o == (KeyOpts{}) {
		return nil
	}
	keep := o.keepNewline()
	return func(line string) string {
		key := o.canon(line)
		if keep && strings.HasSuffix(line, "\n") {
			// The newline can't appear anywhere else in the key, so
			// it marks a complete line on its own.
			key += "\n"
		}
		return key
	}
}

// Blank reports whether a line holds nothing but what the comparison
// ignores, which is what the -B option of diff takes as a blank line.
func (o KeyOpts) Blank(line string) bool {
	return o.canon(line) == ""
}

// canon maps a line to the form its contents are compared in, without
// the ending which tells a complete line from an incomplete one.
func (o KeyOpts) canon(line string) string {
	s := strings.TrimSuffix(line, "\n")
	switch {
	case o.IgnoreAllSpace:
		s = dropBlank(s)
	case o.IgnoreSpaceChange:
		s = collapseBlank(s)
	case o.IgnoreTabExpansion || o.IgnoreTrailingSpace:
		s = expandTrailBlank(s, o.IgnoreTabExpansion, o.IgnoreTrailingSpace, o.tabSize())
	}
	if o.IgnoreCase {
		s = lower(s)
	}
	return s
}

// keepNewline reports whether a missing final newline stays a difference
// of the comparison, which it does unless its blanks at the end of a line
// are already ignored.
func (o KeyOpts) keepNewline() bool {
	return !o.IgnoreAllSpace && !o.IgnoreSpaceChange && !o.IgnoreTrailingSpace
}

// tabSize returns the width of the tab stops of the comparison.
func (o KeyOpts) tabSize() int {
	if o.TabSize > 0 {
		return o.TabSize
	}
	return 8
}

// blank holds the horizontal white space characters the comparisons of
// KeyOpts treat alike.
const blank = " \t\v\f\r"

func isBlankByte(c byte) bool {
	return strings.IndexByte(blank, c) >= 0
}

// dropBlank removes every blank of a line.
func dropBlank(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for i := range len(s) {
		if !isBlankByte(s[i]) {
			out.WriteByte(s[i])
		}
	}
	return out.String()
}

// collapseBlank turns every run of blanks into a single space and drops
// the trailing blanks of a line, so lines only differing in the amount
// of blanks compare equal.
func collapseBlank(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	pending := false
	for i := range len(s) {
		if isBlankByte(s[i]) {
			pending = true
			continue
		}
		if pending {
			out.WriteByte(' ')
			pending = false
		}
		out.WriteByte(s[i])
	}
	return out.String()
}

// expandTrailBlank expands the tabs of a line to the spaces they stand
// for and drops its trailing blanks, as the -E and -Z options of diff
// compare them.
func expandTrailBlank(s string, tab, trail bool, tabSize int) string {
	if trail {
		s = strings.TrimRight(s, blank)
	}
	if !tab || !strings.ContainsAny(s, "\t\b\r") {
		return s
	}
	var out strings.Builder
	out.Grow(len(s) + 8)
	column := 0
	for i := range len(s) {
		switch c := s[i]; c {
		case '\t':
			spaces := tabSize - column%tabSize
			for range spaces {
				out.WriteByte(' ')
			}
			column += spaces
		case '\b':
			if column > 0 {
				column--
			}
			out.WriteByte(c)
		case '\r':
			column = 0
			out.WriteByte(c)
		default:
			column++
			out.WriteByte(c)
		}
	}
	return out.String()
}

// lower lowercases the ASCII letters of a line.
func lower(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for i := range len(s) {
		if 'A' <= s[i] && s[i] <= 'Z' {
			out.WriteByte(s[i] + 'a' - 'A')
		} else {
			out.WriteByte(s[i])
		}
	}
	return out.String()
}
