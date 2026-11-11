package diff

import (
	"fmt"
	"io"
)

// RCS writes the changes that turn a into b in the RCS format, which the
// Revision Control System reads: a deletion command "d start count" and an
// append command "a start count" followed by its lines, per change, from
// the beginning of the file to its end. A line without a trailing newline
// keeps the output without one too, which that format can express.
func RCS(w io.Writer, b []string, changes []Change, o ...LineOpts) error {
	lo := lineOpts(o)
	for _, c := range changes {
		if c.A0 < c.A1 {
			if _, err := fmt.Fprintf(w, "d%d %d\n", c.A0+1, c.A1-c.A0); err != nil {
				return err
			}
		}
		if c.B0 < c.B1 {
			if _, err := fmt.Fprintf(w, "a%d %d\n", c.A1, c.B1-c.B0); err != nil {
				return err
			}
			for _, line := range b[c.B0:c.B1] {
				if err := writePlain(w, line, lo); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
