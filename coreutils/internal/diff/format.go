package diff

import "time"

// File describes one of the compared files for the headers of the context
// and unified formats: its name and modification time, which is the current
// time for the standard input.
type File struct {
	Name string
	Time time.Time
}

// hunk holds the changes of one group, which sit close enough to share
// their context lines, and the line ranges the group covers in each file,
// context included.
type hunk struct {
	changes      []Change
	aStart, aEnd int
	bStart, bEnd int
}

// hunks groups the changes that sit within 2*context equal lines of each
// other, and bounds each group with its context lines, clipped to the
// files.
func hunks(a, b []string, changes []Change, context int) []hunk {
	var hs []hunk
	for _, c := range changes {
		if n := len(hs); n > 0 && c.A0-hs[n-1].changes[len(hs[n-1].changes)-1].A1 <= 2*context {
			h := &hs[n-1]
			h.changes = append(h.changes, c)
			h.aEnd = min(len(a), c.A1+context)
			h.bEnd = min(len(b), c.B1+context)
			continue
		}
		hs = append(hs, hunk{
			changes: []Change{c},
			aStart:  max(0, c.A0-context),
			aEnd:    min(len(a), c.A1+context),
			bStart:  max(0, c.B0-context),
			bEnd:    min(len(b), c.B1+context),
		})
	}
	return hs
}
