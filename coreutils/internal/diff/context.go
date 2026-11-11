package diff

import (
	"fmt"
	"io"
	"strconv"
)

// contextStamp formats file times as the context headers write them.
const contextStamp = "Mon Jan _2 15:04:05 2006"

// Context writes the changes that turn a into b in the copied context
// format: file headers, then per group of changes the affected lines with
// context ones around them, shown for each file in turn.
func Context(w io.Writer, f1, f2 File, a, b []string, changes []Change, context int, o ...LineOpts) error {
	lo := lineOpts(o)
	if err := writeFileHeader(w, "***", f1, contextStamp); err != nil {
		return err
	}
	if err := writeFileHeader(w, "---", f2, contextStamp); err != nil {
		return err
	}
	for _, h := range hunks(a, b, changes, context) {
		if _, err := io.WriteString(w, "***************\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "*** %s ****\n", contextRange(h.aStart, h.aEnd)); err != nil {
			return err
		}
		ia := h.aStart
		same, changed, deleted, added := lo.spaceMark(" "), lo.spaceMark("!"), lo.spaceMark("-"), lo.spaceMark("+")
		for _, c := range h.changes {
			for ; ia < c.A0; ia++ {
				if err := writeLine(w, same, a[ia], lo); err != nil {
					return err
				}
			}
			// Lines that file2 replaces are changed; the others are just
			// deleted.
			mark := changed
			if c.B0 == c.B1 {
				mark = deleted
			}
			for ; ia < c.A1; ia++ {
				if err := writeLine(w, mark, a[ia], lo); err != nil {
					return err
				}
			}
		}
		for ; ia < h.aEnd; ia++ {
			if err := writeLine(w, same, a[ia], lo); err != nil {
				return err
			}
		}

		if _, err := fmt.Fprintf(w, "--- %s ----\n", contextRange(h.bStart, h.bEnd)); err != nil {
			return err
		}
		ib := h.bStart
		for _, c := range h.changes {
			for ; ib < c.B0; ib++ {
				if err := writeLine(w, same, b[ib], lo); err != nil {
					return err
				}
			}
			// Lines that replace ones of file1 are changed; the others are
			// just added.
			mark := changed
			if c.A0 == c.A1 {
				mark = added
			}
			for ; ib < c.B1; ib++ {
				if err := writeLine(w, mark, b[ib], lo); err != nil {
					return err
				}
			}
		}
		for ; ib < h.bEnd; ib++ {
			if err := writeLine(w, same, b[ib], lo); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeFileHeader writes one file header of the context and unified
// formats, with the file's name and time, or its label when it has one.
func writeFileHeader(w io.Writer, mark string, f File, stamp string) error {
	if f.Label != "" {
		_, err := io.WriteString(w, mark+" "+f.Label+"\n")
		return err
	}
	_, err := fmt.Fprintf(w, "%s %s\t%s\n", mark, f.Name, f.Time.Format(stamp))
	return err
}

// contextRange formats the 0-based line range [start, end) as written in
// the range headers of the context format: the number of the line before
// an empty range, a single number for one line, and the beginning and
// ending numbers for the rest.
func contextRange(start, end int) string {
	switch n := end - start; n {
	case 0:
		return strconv.Itoa(start)
	case 1:
		return strconv.Itoa(start + 1)
	default:
		return strconv.Itoa(start+1) + "," + strconv.Itoa(end)
	}
}
