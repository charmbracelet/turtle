package diff

import (
	"io"
	"strings"
)

// Ifdef writes the changes that turn a into b in the merged if-then-else
// format of diff's -D option: the whole contents of both files, with each
// differing group of lines wrapped in C preprocessor conditionals on the
// given name. A line without a trailing newline ends completed, as the
// format can't express otherwise.
func Ifdef(w io.Writer, name string, a, b []string, changes []Change, o ...LineOpts) error {
	lo := lineOpts(o)
	ia, ib := 0, 0
	for _, c := range changes {
		for ; ia < c.A0; ia, ib = ia+1, ib+1 {
			if err := writeMerged(w, a[ia], lo); err != nil {
				return err
			}
		}
		switch {
		case c.A0 == c.A1:
			if err := wrap(w, "#ifdef "+name+"\n", b[c.B0:c.B1], "#endif /* "+name+" */\n", lo); err != nil {
				return err
			}
		case c.B0 == c.B1:
			if err := wrap(w, "#ifndef "+name+"\n", a[c.A0:c.A1], "#endif /* ! "+name+" */\n", lo); err != nil {
				return err
			}
		default:
			if err := wrap(w, "#ifndef "+name+"\n", a[c.A0:c.A1], "#else /* "+name+" */\n", lo); err != nil {
				return err
			}
			if err := wrap(w, "", b[c.B0:c.B1], "#endif /* "+name+" */\n", lo); err != nil {
				return err
			}
		}
		ia, ib = c.A1, c.B1
	}
	for ; ia < len(a); ia, ib = ia+1, ib+1 {
		if err := writeMerged(w, a[ia], lo); err != nil {
			return err
		}
	}
	return nil
}

// wrap writes a group of lines between an opening and a closing line.
func wrap(w io.Writer, open string, lines []string, close string, lo LineOpts) error {
	if open != "" {
		if _, err := io.WriteString(w, open); err != nil {
			return err
		}
	}
	for _, line := range lines {
		if err := writeMerged(w, line, lo); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, close); err != nil {
		return err
	}
	return nil
}

// writeMerged writes one line of the merged output, completing a line
// which ends without a newline.
func writeMerged(w io.Writer, line string, lo LineOpts) error {
	if lo.ExpandTabs {
		line = expandTabs(line, lo.tabSize())
	}
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	_, err := io.WriteString(w, line)
	return err
}
