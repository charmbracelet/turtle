package diff

import (
	"io"
	"strings"
)

// LineOpts adjusts the way the output formats write their lines, as the
// -t and -T options of diff ask.
type LineOpts struct {
	// ExpandTabs writes the tabs of the lines as the spaces they stand
	// for, so the output keeps its alignment when piped.
	ExpandTabs bool

	// InitialTab marks the lines with a tab after the flag of each one,
	// instead of a space, so tabbed lines stay aligned.
	InitialTab bool

	// TabSize is the width of the tab stops, 8 when zero.
	TabSize int
}

// lineOpts picks the options out of the variadic argument an output
// format takes, so callers with nothing to adjust pass none.
func lineOpts(o []LineOpts) LineOpts {
	if len(o) == 0 {
		return LineOpts{}
	}
	return o[0]
}

// tabSize returns the width of a tab stop.
func (o LineOpts) tabSize() int {
	if o.TabSize > 0 {
		return o.TabSize
	}
	return 8
}

// spaceMark returns the prefix of a flagged line: its flag and a space,
// or a tab with the -T of diff.
func (o LineOpts) spaceMark(flag string) string {
	if o.InitialTab {
		return flag + "\t"
	}
	return flag + " "
}

// bareMark returns the prefix of a flagged line in the unified format:
// the flag alone, or a tab with the -T of diff, which replaces the flag
// of the context lines, as their flag is a space already.
func (o LineOpts) bareMark(flag string) string {
	if o.InitialTab {
		if flag == " " {
			return "\t"
		}
		return flag + "\t"
	}
	return flag
}

// writeLine writes one line of output after its prefix, flagging the
// last line of a file that lacks a trailing newline.
func writeLine(w io.Writer, prefix, line string, o LineOpts) error {
	text := line
	if o.ExpandTabs {
		text = expandTabs(line, o.tabSize())
	}
	if _, err := io.WriteString(w, prefix+text); err != nil {
		return err
	}
	if !strings.HasSuffix(line, "\n") {
		if _, err := io.WriteString(w, "\n\\ No newline at end of file\n"); err != nil {
			return err
		}
	}
	return nil
}

// writePlain writes a line of output with no prefix, as the scripts do,
// flagging nothing: a line which ends without a newline simply keeps the
// output ended that way too.
func writePlain(w io.Writer, line string, o LineOpts) error {
	if o.ExpandTabs {
		line = expandTabs(line, o.tabSize())
	}
	_, err := io.WriteString(w, line)
	return err
}

// expandTabs replaces the tabs of a line with the spaces they stand for
// at the given tab stop width.
func expandTabs(line string, tabSize int) string {
	if !strings.ContainsAny(line, "\t\b\r") {
		return line
	}
	var out strings.Builder
	out.Grow(len(line) + tabSize)
	column := 0
	for i := range len(line) {
		switch c := line[i]; c {
		case '\t':
			spaces := tabSize - column%tabSize
			out.WriteString(strings.Repeat(" ", spaces))
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
