package diff

import (
	"io"
	"strconv"
	"strings"
)

// Normal writes the changes that turn a into b in the diff default format:
// an ed-like command line per change, such as "1,3c5,7", followed by the
// "< " lines of a and the "> " lines of b, split by "---".
func Normal(w io.Writer, a, b []string, changes []Change) error {
	for _, c := range changes {
		var cmd strings.Builder
		switch {
		case c.A0 == c.A1:
			cmd.WriteString(strconv.Itoa(c.A0))
			cmd.WriteString("a")
			writeRange(&cmd, c.B0+1, c.B1)
		case c.B0 == c.B1:
			writeRange(&cmd, c.A0+1, c.A1)
			cmd.WriteString("d")
			cmd.WriteString(strconv.Itoa(c.B0))
		default:
			writeRange(&cmd, c.A0+1, c.A1)
			cmd.WriteString("c")
			writeRange(&cmd, c.B0+1, c.B1)
		}
		cmd.WriteByte('\n')

		if _, err := io.WriteString(w, cmd.String()); err != nil {
			return err
		}
		if err := writeLines(w, "< ", a[c.A0:c.A1]); err != nil {
			return err
		}
		if c.A0 != c.A1 && c.B0 != c.B1 {
			if _, err := io.WriteString(w, "---\n"); err != nil {
				return err
			}
		}
		if err := writeLines(w, "> ", b[c.B0:c.B1]); err != nil {
			return err
		}
	}
	return nil
}

// writeRange writes the 1-based line range [first, last], as a single
// number when both ends meet.
func writeRange(b *strings.Builder, first, last int) {
	b.WriteString(strconv.Itoa(first))
	if last > first {
		b.WriteByte(',')
		b.WriteString(strconv.Itoa(last))
	}
}

// writeLines writes the lines prefixed with mark, flagging the last line of
// a file that lacks a trailing newline.
func writeLines(w io.Writer, mark string, lines []string) error {
	for _, line := range lines {
		if err := writeLine(w, mark, line); err != nil {
			return err
		}
	}
	return nil
}

// writeLine writes one line prefixed with mark, flagging the last line of a
// file that lacks a trailing newline.
func writeLine(w io.Writer, mark, line string) error {
	if _, err := io.WriteString(w, mark+line); err != nil {
		return err
	}
	if !strings.HasSuffix(line, "\n") {
		if _, err := io.WriteString(w, "\n\\ No newline at end of file\n"); err != nil {
			return err
		}
	}
	return nil
}
