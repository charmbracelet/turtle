package diff

import (
	"fmt"
	"io"
	"strconv"
)

// unifiedStamp formats file times as the unified headers write them.
const unifiedStamp = "2006-01-02 15:04:05 -0700"

// Unified writes the changes that turn a into b in the unified context
// format: file headers, then per group of changes the affected lines and
// the context around them, interleaved and taken from a.
func Unified(w io.Writer, f1, f2 File, a, b []string, changes []Change, context int, o ...LineOpts) error {
	lo := lineOpts(o)
	if err := writeFileHeader(w, "---", f1, unifiedStamp); err != nil {
		return err
	}
	if err := writeFileHeader(w, "+++", f2, unifiedStamp); err != nil {
		return err
	}
	for _, h := range hunks(a, b, changes, context) {
		if _, err := fmt.Fprintf(w, "@@ -%s +%s @@\n",
			unifiedRange(h.aStart, h.aEnd), unifiedRange(h.bStart, h.bEnd)); err != nil {
			return err
		}
		same, deleted, added := lo.bareMark(" "), lo.bareMark("-"), lo.bareMark("+")
		ia, ib := h.aStart, h.bStart
		for _, c := range h.changes {
			for ; ia < c.A0; ia, ib = ia+1, ib+1 {
				if err := writeLine(w, same, a[ia], lo); err != nil {
					return err
				}
			}
			for ; ia < c.A1; ia++ {
				if err := writeLine(w, deleted, a[ia], lo); err != nil {
					return err
				}
			}
			for ; ib < c.B1; ib++ {
				if err := writeLine(w, added, b[ib], lo); err != nil {
					return err
				}
			}
		}
		for ; ia < h.aEnd; ia, ib = ia+1, ib+1 {
			if err := writeLine(w, same, a[ia], lo); err != nil {
				return err
			}
		}
	}
	return nil
}

// unifiedRange formats the 0-based line range [start, end) as written in
// the hunk headers of the unified format: a single number for one line,
// and a number and count pair otherwise, with the number of the line
// before an empty range.
func unifiedRange(start, end int) string {
	switch n := end - start; n {
	case 0:
		return strconv.Itoa(start) + ",0"
	case 1:
		return strconv.Itoa(start + 1)
	default:
		return strconv.Itoa(start+1) + "," + strconv.Itoa(n)
	}
}
