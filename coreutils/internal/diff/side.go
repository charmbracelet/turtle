package diff

import (
	"io"
	"strings"
)

// SideOpts configures the side-by-side output format of diff.
type SideOpts struct {
	// Width is the total output width in columns, 130 when zero.
	Width int

	// TabSize is the width of the tab stops, 8 when zero.
	TabSize int

	// ExpandTabs writes the tabs of the text as the spaces they stand
	// for, instead of keeping the tabs which fit.
	ExpandTabs bool

	// LeftColumn writes the common lines in the left column only, with
	// a '(' marking them.
	LeftColumn bool

	// SuppressCommon omits the common lines.
	SuppressCommon bool
}

// gutterMinimum is the least number of columns the gutter between the two
// columns takes, when the half line width is nonzero.
const gutterMinimum = 3

// Side writes the changes that turn a into b in the side-by-side format:
// the lines of both files in two columns around a gutter, with each line
// marked by how it compares, '|' for a changed pair, '<' and '>' for lines
// of one file only, '(' for the common lines of the left column only and
// ')' for the unpaired common lines of the right one.
func Side(w io.Writer, a, b []string, changes []Change, o SideOpts) error {
	s := &sideWriter{w: w, opts: o}
	s.hw, s.c2o = sideColumns(o)
	next0, next1 := 0, 0
	common := func(limit0, limit1 int) error {
		if !o.SuppressCommon && (next0 != limit0 || next1 != limit1) {
			var err error
			if !o.LeftColumn {
				for next0 < limit0 && next1 < limit1 && err == nil {
					err = s.line(&a[next0], ' ', &b[next1])
					next0, next1 = next0+1, next1+1
				}
				for next1 < limit1 && err == nil {
					err = s.line(nil, ')', &b[next1])
					next1++
				}
			}
			for next0 < limit0 && err == nil {
				err = s.line(&a[next0], '(', nil)
				next0++
			}
			if err != nil {
				return err
			}
		}
		next0, next1 = limit0, limit1
		return nil
	}
	for _, c := range changes {
		if err := common(c.A0, c.B0); err != nil {
			return err
		}
		i, j := c.A0, c.B0
		for i < c.A1 && j < c.B1 {
			if err := s.line(&a[i], '|', &b[j]); err != nil {
				return err
			}
			i, j = i+1, j+1
		}
		for ; j < c.B1; j++ {
			if err := s.line(nil, '>', &b[j]); err != nil {
				return err
			}
		}
		for ; i < c.A1; i++ {
			if err := s.line(&a[i], '<', nil); err != nil {
				return err
			}
		}
		next0, next1 = c.A1, c.B1
	}
	return common(len(a), len(b))
}

// sideColumns lays out the two columns of the side-by-side format over
// the output width: the width of a column of text, and the column the
// right one starts at. A zero width of text means the columns don't fit
// and only the marks come out.
func sideColumns(o SideOpts) (halfWidth, column2 int) {
	width, tabSize := o.Width, o.TabSize
	if width == 0 {
		width = 130
	}
	if tabSize == 0 {
		tabSize = 8
	}
	t := tabSize
	if o.ExpandTabs {
		t = 1
	}
	tg := t + gutterMinimum
	off := width/2 + tg/2 + (width & tg & 1)
	off -= off % t
	if off <= gutterMinimum || width <= off {
		return 0, width
	}
	return min(off-gutterMinimum, width-off), off
}

// sideWriter writes the lines of the side-by-side format over its layout.
type sideWriter struct {
	w    io.Writer
	opts SideOpts
	hw   int // width of a column of text
	c2o  int // column the right column of text starts at
}

// line writes one output line: the left text, the mark, then the right
// text, with the gutter between them.
func (s *sideWriter) line(left *string, sep byte, right *string) error {
	var b strings.Builder
	col, mid := 0, (s.hw+s.c2o-1)/2
	rightNewline := right != nil && strings.HasSuffix(*right, "\n")
	putNewline := false
	if left != nil {
		putNewline = strings.HasSuffix(*left, "\n")
		col = s.half(&b, *left, 0, s.hw)
	}
	if sep != ' ' {
		s.pad(&b, col, mid)
		if sep == '|' && putNewline != rightNewline {
			sep = '\\'
			if putNewline {
				sep = '/'
			}
		}
		b.WriteByte(sep)
		col = mid + 1
	}
	if right != nil {
		putNewline = putNewline || rightNewline
		if *right != "\n" {
			s.pad(&b, col, s.c2o)
			col = s.half(&b, *right, s.c2o, s.hw)
		}
	}
	if putNewline {
		b.WriteByte('\n')
	}
	if _, err := io.WriteString(s.w, b.String()); err != nil {
		return err
	}
	return nil
}

// half writes the text of one column of a line, truncated to bound
// columns, with the line starting at column indent. It returns the column
// the output reached.
func (s *sideWriter) half(b *strings.Builder, line string, indent, bound int) int {
	ts := s.opts.tabSize()
	if ts == 0 {
		ts = 8
	}
	in, out := 0, 0
	for i := range len(line) {
		switch c := line[i]; c {
		case '\n':
			return out
		case '\t':
			spaces := ts - in%ts
			if in == out {
				tabstop := out + spaces
				if s.opts.ExpandTabs {
					stop := min(tabstop, bound)
					b.WriteString(strings.Repeat(" ", stop-out))
					out = stop
				} else if tabstop < bound {
					b.WriteByte('\t')
					out = tabstop
				}
			}
			in += spaces
		case '\r':
			b.WriteByte(c)
			s.pad(b, 0, indent)
			in, out = 0, 0
		case '\b':
			if in > 0 {
				in--
			}
			if in < bound {
				if out <= in {
					b.WriteString(strings.Repeat(" ", in-out))
					out = in
				} else {
					b.WriteByte(c)
					out = in
				}
			}
		default:
			if in < bound {
				in++
				out = in
				b.WriteByte(c)
			} else {
				in++
			}
		}
	}
	return out
}

// pad writes tabs and spaces which advance the output from column from
// to column to.
func (s *sideWriter) pad(b *strings.Builder, from, to int) {
	ts := s.opts.tabSize()
	if ts == 0 {
		ts = 8
	}
	if !s.opts.ExpandTabs {
		for tab := from + ts - from%ts; tab <= to; tab += ts {
			b.WriteByte('\t')
			from = tab
		}
	}
	b.WriteString(strings.Repeat(" ", to-from))
}

// tabSize returns the width of a tab stop of the side-by-side layout.
func (o SideOpts) tabSize() int {
	if o.TabSize > 0 {
		return o.TabSize
	}
	return 8
}
