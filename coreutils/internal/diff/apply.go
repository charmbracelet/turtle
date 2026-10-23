package diff

import (
	"slices"
	"strings"
)

// Applier applies the hunks of a patch to the lines of a file, searching
// for their places when the file has moved them around.
type Applier struct {
	// Blank makes sequences of blanks match any sequence of blanks, as with
	// patch's -l.
	Blank bool
}

// Outcome tells what happened to one hunk.
type Outcome struct {
	Offset  int // how far the hunk landed from its stated place
	Fuzz    int // context lines each end gave up to match
	Failed  bool
	Already bool // the changes of the hunk are in the file already
}

// Apply tries each hunk at its stated place, then scanning around it, then
// giving up context lines, and finally checks whether its changes are
// already in the file. It returns the new lines and the outcome of every
// hunk, leaving failed hunks unapplied.
func (a Applier) Apply(lines []string, hunks []Hunk) ([]string, []Outcome) {
	out := slices.Clone(lines)
	outcomes := make([]Outcome, len(hunks))
	offset := 0
	for i, h := range hunks {
		pos, fuzz, ok := a.find(out, h, offset)
		if !ok {
			if a.appliedAlready(out, h) {
				outcomes[i].Already = true
			} else {
				outcomes[i].Failed = true
			}
			continue
		}
		offset = pos - statedPos(h)
		out = a.splice(out, pos, h, fuzz)
		outcomes[i].Offset = offset
		outcomes[i].Fuzz = fuzz
	}
	return out, outcomes
}

// find searches the position of the hunk: its stated place, moved by the
// offset the previous hunk landed at, first, then scanning around it,
// giving up context lines between the rounds.
func (a Applier) find(lines []string, h Hunk, offset int) (pos, fuzz int, ok bool) {
	stated := statedPos(h) + offset
	for f := range fuzzLimit(h) + 1 {
		if a.matches(lines, stated, h, f) {
			return stated, f, true
		}
		for d := 1; d <= len(lines); d++ {
			if a.matches(lines, stated+d, h, f) {
				return stated + d, f, true
			}
			if a.matches(lines, stated-d, h, f) {
				return stated - d, f, true
			}
		}
	}
	return 0, 0, false
}

// statedPos returns the 0-based position the hunk says its old lines sit at,
// which is the insertion point of a hunk that replaces nothing.
func statedPos(h Hunk) int {
	if len(h.Old) == 0 {
		return h.FromLine
	}
	return h.FromLine - 1
}

// matches reports whether the old lines of the hunk, minus fuzz lines at
// each end, match the file lines at pos.
func (a Applier) matches(lines []string, pos int, h Hunk, fuzz int) bool {
	old := h.Old[fuzz : len(h.Old)-fuzz]
	if pos < 0 || pos+len(old) > len(lines) {
		return false
	}
	for i, l := range old {
		if !a.equal(l.Content, lines[pos+i]) {
			return false
		}
	}
	return true
}

// appliedAlready reports whether the new lines of the hunk are in the file
// at their stated place, as an already applied patch leaves them.
func (a Applier) appliedAlready(lines []string, h Hunk) bool {
	if len(h.New) == 0 {
		return false
	}
	pos := h.ToLine - 1
	if pos < 0 || pos+len(h.New) > len(lines) {
		return false
	}
	for i, l := range h.New {
		if !a.equal(l.Content, lines[pos+i]) {
			return false
		}
	}
	return true
}

// splice replaces the old lines the hunk matched at pos with its new ones.
func (a Applier) splice(lines []string, pos int, h Hunk, fuzz int) []string {
	old := h.Old[fuzz : len(h.Old)-fuzz]
	out := make([]string, 0, len(lines)-(len(old)-len(h.New))+len(h.New))
	out = append(out, lines[:pos]...)
	for _, l := range h.New[fuzz : len(h.New)-fuzz] {
		line := l.Content
		if !l.NoNewline {
			line += "\n"
		}
		out = append(out, line)
	}
	return append(out, lines[pos+len(old):]...)
}

// equal compares a hunk line, which holds no newline, with a file line.
func (a Applier) equal(want, line string) bool {
	line = strings.TrimSuffix(line, "\n")
	if a.Blank {
		return BlankKey(want) == BlankKey(line)
	}
	return want == line
}

// fuzzLimit returns how many context lines each end of the hunk can give
// up: the context runs of its edges, with at least one old line left to
// match.
func fuzzLimit(h Hunk) int {
	if len(h.Old) == 0 {
		return 0
	}
	lead := 0
	for lead < len(h.Old) && lead < len(h.New) &&
		h.Old[lead].Kind == ContextLine && h.New[lead].Kind == ContextLine {
		lead++
	}
	trail := 0
	for trail < len(h.Old) && trail < len(h.New) &&
		h.Old[len(h.Old)-1-trail].Kind == ContextLine && h.New[len(h.New)-1-trail].Kind == ContextLine {
		trail++
	}
	return min(lead, trail, (len(h.Old)-1)/2)
}

// Reverse swaps the sides of every hunk of the patch, so it applies the
// changes backwards.
func Reverse(p Patch) Patch {
	r := Patch{From: p.To, To: p.From, Index: p.Index}
	for _, h := range p.Hunks {
		r.Hunks = append(r.Hunks, Hunk{
			FromLine: h.ToLine,
			ToLine:   h.FromLine,
			Old:      h.New,
			New:      h.Old,
		})
	}
	return r
}
