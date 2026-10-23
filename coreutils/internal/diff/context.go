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
func Context(w io.Writer, f1, f2 File, a, b []string, changes []Change, context int) error {
	if _, err := fmt.Fprintf(w, "*** %s\t%s\n", f1.Name, f1.Time.Format(contextStamp)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "--- %s\t%s\n", f2.Name, f2.Time.Format(contextStamp)); err != nil {
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
		for _, c := range h.changes {
			for ; ia < c.A0; ia++ {
				if err := writeLine(w, "  ", a[ia]); err != nil {
					return err
				}
			}
			// Lines that file2 replaces are changed; the others are just
			// deleted.
			mark := "! "
			if c.B0 == c.B1 {
				mark = "- "
			}
			for ; ia < c.A1; ia++ {
				if err := writeLine(w, mark, a[ia]); err != nil {
					return err
				}
			}
		}
		for ; ia < h.aEnd; ia++ {
			if err := writeLine(w, "  ", a[ia]); err != nil {
				return err
			}
		}

		if _, err := fmt.Fprintf(w, "--- %s ----\n", contextRange(h.bStart, h.bEnd)); err != nil {
			return err
		}
		ib := h.bStart
		for _, c := range h.changes {
			for ; ib < c.B0; ib++ {
				if err := writeLine(w, "  ", b[ib]); err != nil {
					return err
				}
			}
			// Lines that replace ones of file1 are changed; the others are
			// just added.
			mark := "! "
			if c.A0 == c.A1 {
				mark = "+ "
			}
			for ; ib < c.B1; ib++ {
				if err := writeLine(w, mark, b[ib]); err != nil {
					return err
				}
			}
		}
		for ; ib < h.bEnd; ib++ {
			if err := writeLine(w, "  ", b[ib]); err != nil {
				return err
			}
		}
	}
	return nil
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
