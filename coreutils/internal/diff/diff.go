// Package diff implements the line diffing shared by the diff and patch
// utilities: the line splitting, the blank comparison of diff's -b, and the
// Myers algorithm that computes the changes between two sequences of lines.
package diff

// Change replaces the lines [A0, A1) of one file with the lines [B0, B1) of
// the other. An empty a-side makes it an insertion, an empty b-side a
// deletion, and both non-empty a replacement.
type Change struct {
	A0, A1, B0, B1 int
}

// Lines returns the changes that turn a into b, in ascending order and
// without overlaps. key maps each line to the form it's compared in, and
// may be nil to compare lines as-is.
//
// The lines are as returned by SplitLines, keeping their trailing newline.
// The equal runs between changes always come from a, so a key that makes
// lines equal doesn't join lines that only look alike.
func Lines(a, b []string, key func(string) string) []Change {
	xa, xb := pack(a, b, key)
	e := &engine{}
	e.diff(xa, xb, 0, 0)
	return join(e.changes)
}

// Apply returns the result of applying the changes that turn a into b. It's
// meant to check scripts in tests: Apply(a, b, Lines(a, b, key)) equals b.
func Apply(a, b []string, changes []Change) []string {
	var out []string
	next := 0
	for _, c := range changes {
		out = append(out, a[next:c.A0]...)
		out = append(out, b[c.B0:c.B1]...)
		next = c.A1
	}
	return append(out, a[next:]...)
}

// join merges touching changes, so each spot where the files differ comes
// out as one change, like the delete and insert of a replaced line.
func join(changes []Change) []Change {
	out := changes[:0]
	for _, c := range changes {
		if n := len(out); n > 0 && out[n-1].A1 == c.A0 && out[n-1].B1 == c.B0 {
			out[n-1].A1 = c.A1
			out[n-1].B1 = c.B1
			continue
		}
		out = append(out, c)
	}
	return out
}

// pack maps the lines of both files to integer ids, so the engine compares
// them with array reads instead of string comparisons.
func pack(a, b []string, key func(string) string) ([]int, []int) {
	ids := make(map[string]int)
	xa := make([]int, len(a))
	for i, line := range a {
		xa[i] = id(ids, line, key)
	}
	xb := make([]int, len(b))
	for i, line := range b {
		xb[i] = id(ids, line, key)
	}
	return xa, xb
}

func id(ids map[string]int, line string, key func(string) string) int {
	if key != nil {
		line = key(line)
	}
	i, ok := ids[line]
	if !ok {
		i = len(ids)
		ids[line] = i
	}
	return i
}
