package diff

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// Ed writes the changes as an ed script that turns file1 into file2 when
// fed to ed with a trailing w command. The changes come from the end of the
// file to its beginning, so the line numbers of the edits still to apply
// stay valid.
func Ed(w io.Writer, b []string, changes []Change, o ...LineOpts) error {
	lo := lineOpts(o)
	for _, c := range slices.Backward(changes) {
		switch {
		case c.A0 == c.A1:
			if _, err := fmt.Fprintf(w, "%da\n", c.A0); err != nil {
				return err
			}
			if err := edText(w, b, c, c.A0+1, lo); err != nil {
				return err
			}
		case c.B0 == c.B1:
			if _, err := fmt.Fprintf(w, "%sd\n", edRange(c.A0, c.A1)); err != nil {
				return err
			}
		default:
			if _, err := fmt.Fprintf(w, "%sc\n", edRange(c.A0, c.A1)); err != nil {
				return err
			}
			if err := edText(w, b, c, c.A0+1, lo); err != nil {
				return err
			}
		}
	}
	return nil
}

// ForwardEd writes the changes in the alternative script form: like Ed,
// but from the beginning of the file to its end, with the command letter
// before the lines, which are then space separated. The result isn't meant
// to be fed to ed.
func ForwardEd(w io.Writer, b []string, changes []Change, o ...LineOpts) error {
	lo := lineOpts(o)
	for _, c := range changes {
		var cmd string
		switch {
		case c.A0 == c.A1:
			cmd = fmt.Sprintf("a%d", c.A0)
		case c.B0 == c.B1:
			cmd = "d" + forwardRange(c.A0, c.A1)
		default:
			cmd = "c" + forwardRange(c.A0, c.A1)
		}
		if _, err := io.WriteString(w, cmd+"\n"); err != nil {
			return err
		}
		if c.B0 != c.B1 {
			for _, line := range b[c.B0:c.B1] {
				if err := writeText(w, line, lo); err != nil {
					return err
				}
			}
			if _, err := io.WriteString(w, ".\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

// edText writes the lines of an ed a or c command, counting from start,
// with its closing period. A line consisting of a lone period, which would
// end the text, comes out as two periods that a substitute command then
// turns back into one.
func edText(w io.Writer, b []string, c Change, start int, lo LineOpts) error {
	var subs []int
	for i, j := c.B0, 0; i < c.B1; i, j = i+1, j+1 {
		line := b[i]
		if strings.TrimSuffix(line, "\n") == "." {
			subs = append(subs, start+j)
			line = "..\n"
		}
		if err := writeText(w, line, lo); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, ".\n"); err != nil {
		return err
	}
	for _, n := range subs {
		if _, err := fmt.Fprintf(w, "%ds/.//\n", n); err != nil {
			return err
		}
	}
	return nil
}

// writeText writes a line of ed script text, always ending in a newline.
func writeText(w io.Writer, line string, lo LineOpts) error {
	if lo.ExpandTabs {
		line = expandTabs(line, lo.tabSize())
	}
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	_, err := io.WriteString(w, line)
	return err
}

// edRange formats the 0-based line range [start, end) as the lines of an
// ed command.
func edRange(start, end int) string {
	if end-start == 1 {
		return strconv.Itoa(start + 1)
	}
	return strconv.Itoa(start+1) + "," + strconv.Itoa(end)
}

// forwardRange formats the range like edRange, but with the numbers space
// separated, as the forward script form writes them.
func forwardRange(start, end int) string {
	if end-start == 1 {
		return strconv.Itoa(start + 1)
	}
	return strconv.Itoa(start+1) + " " + strconv.Itoa(end)
}
