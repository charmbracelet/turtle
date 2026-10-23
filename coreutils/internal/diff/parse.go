package diff

import (
	"fmt"
	"strings"
)

// LineKind tells what a hunk line does: stay as context, get deleted, or
// get inserted.
type LineKind uint8

const (
	ContextLine LineKind = iota
	DeleteLine
	InsertLine
)

// HunkLine is one line of a hunk, without its trailing newline.
type HunkLine struct {
	Kind      LineKind
	Content   string
	NoNewline bool // the line ends its file without a trailing newline
}

// Hunk is one hunk of a patch: the old lines it replaces, with the context
// around them, and the new lines that take their place.
type Hunk struct {
	FromLine int        // 1-based first old line, or the one before an empty range
	ToLine   int        // 1-based first new line, or the one before an empty range
	Old      []HunkLine // context and deleted lines, in order
	New      []HunkLine // context and inserted lines, in order
}

// Patch is the set of hunks that apply to one file, with the names found in
// its headers.
type Patch struct {
	From  string // name of the file the changes came from
	To    string // name of the file to apply them to
	Index string // name of the Index: header, when the patch has one
	Hunks []Hunk
}

// ParseUnified parses patch data as unified diffs, returning one Patch per
// file. The lines around the patches, like the headers diff writes between
// them, are skipped.
func ParseUnified(data string) ([]Patch, error) {
	s := newScanner(stripIndent(data))
	var patches []Patch
	for {
		p, err := parseUnifiedPatch(s)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return patches, nil
		}
		patches = append(patches, *p)
	}
}

// parseUnifiedPatch parses the next patch of the scanner, returning nil at
// the end of the data.
func parseUnifiedPatch(s *scanner) (*Patch, error) {
	p := &Patch{}
	for {
		line, _, ok := s.peek(0)
		if !ok {
			return nil, nil
		}
		if strings.HasPrefix(line, "@@ -") {
			break
		}
		if strings.HasPrefix(line, "--- ") {
			if next, _, ok := s.peek(1); ok && strings.HasPrefix(next, "+++ ") {
				header, _, _ := s.next()
				p.From = headerName(header)
				to, _, _ := s.next()
				p.To = headerName(to)
				break
			}
		}
		if strings.HasPrefix(line, "Index: ") {
			p.Index = strings.TrimSpace(line[len("Index: "):])
		}
		s.next()
	}
	for {
		line, _, ok := s.peek(0)
		if !ok || !strings.HasPrefix(line, "@@ -") {
			break
		}
		h, err := parseUnifiedHunk(s)
		if err != nil {
			return nil, err
		}
		p.Hunks = append(p.Hunks, h)
	}
	// Lines that would belong to a hunk can't follow a complete one.
	if line, n, ok := s.peek(0); ok && isBodyLine(line, s) {
		return nil, malformed(n, "line beyond the hunk counts")
	}
	if len(p.Hunks) == 0 {
		_, n, _ := s.peek(0)
		return nil, malformed(n, "no hunks after the headers")
	}
	return p, nil
}

// isBodyLine reports whether the line would belong to a hunk body, which
// makes it an error outside one: a line starting with a space, a plus, a
// newline marker, or a dash that isn't the next file headers.
func isBodyLine(line string, s *scanner) bool {
	if line == "" {
		return false
	}
	switch line[0] {
	case ' ', '+', '\\':
		return true
	case '-':
		if !strings.HasPrefix(line, "--- ") {
			return true
		}
		next, _, ok := s.peek(1)
		return !ok || !strings.HasPrefix(next, "+++ ")
	default:
		return false
	}
}

// parseUnifiedHunk parses one hunk, with its "@@" header and body lines.
func parseUnifiedHunk(s *scanner) (Hunk, error) {
	var h Hunk
	line, n, _ := s.next()
	var oldCount, newCount int
	var err error
	h.FromLine, h.ToLine, oldCount, newCount, err = unifiedHeader(line, n)
	if err != nil {
		return h, err
	}
	var lastOld, lastNew bool
	for oldCount > 0 || newCount > 0 {
		body, n, ok := s.next()
		if !ok {
			return h, malformed(n, "hunk ends in the middle")
		}
		switch {
		case body == "" || body[0] == ' ':
			if oldCount == 0 || newCount == 0 {
				return h, malformed(n, "context line beyond the hunk counts")
			}
			content := ""
			if body != "" {
				content = body[1:]
			}
			h.Old = append(h.Old, HunkLine{Kind: ContextLine, Content: content})
			h.New = append(h.New, HunkLine{Kind: ContextLine, Content: content})
			oldCount, newCount = oldCount-1, newCount-1
			lastOld, lastNew = true, true
		case body[0] == '-':
			if oldCount == 0 {
				return h, malformed(n, "deleted line beyond the hunk counts")
			}
			h.Old = append(h.Old, HunkLine{Kind: DeleteLine, Content: body[1:]})
			oldCount--
			lastOld, lastNew = true, false
		case body[0] == '+':
			if newCount == 0 {
				return h, malformed(n, "inserted line beyond the hunk counts")
			}
			h.New = append(h.New, HunkLine{Kind: InsertLine, Content: body[1:]})
			newCount--
			lastOld, lastNew = false, true
		case body[0] == '\\':
			if err := markNoNewline(&h, lastOld, lastNew, n); err != nil {
				return h, err
			}
		default:
			return h, malformed(n, "unexpected line in the hunk")
		}
	}
	// The marker of the last line can trail the hunk.
	if line, n, ok := s.peek(0); ok && strings.HasPrefix(line, "\\") {
		s.next()
		if err := markNoNewline(&h, lastOld, lastNew, n); err != nil {
			return h, err
		}
	}
	return h, nil
}

// unifiedHeader parses a "@@ -l[,s] +l[,s] @@" line into the hunk line
// numbers, with the line counts as the length of each side.
func unifiedHeader(line string, n int) (from, to int, oldCount, newCount int, err error) {
	rest := line[4:] // after "@@ -"
	from, rest, err = rangeNum(rest, n)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	oldCount = 1
	if rest != "" && rest[0] == ',' {
		oldCount, rest, err = rangeNum(rest[1:], n)
		if err != nil {
			return 0, 0, 0, 0, err
		}
	}
	if !strings.HasPrefix(rest, " +") {
		return 0, 0, 0, 0, malformed(n, "missing new range in the hunk header")
	}
	rest = rest[2:]
	to, rest, err = rangeNum(rest, n)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	newCount = 1
	if rest != "" && rest[0] == ',' {
		newCount, rest, err = rangeNum(rest[1:], n)
		if err != nil {
			return 0, 0, 0, 0, err
		}
	}
	if rest != "" && !strings.HasPrefix(rest, " @@") {
		return 0, 0, 0, 0, malformed(n, "trailing text in the hunk header")
	}
	return from, to, oldCount, newCount, nil
}

// rangeNum parses a decimal number and returns what follows it.
func rangeNum(s string, n int) (num int, rest string, err error) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, "", malformed(n, "missing line number")
	}
	if i > 12 {
		return 0, "", malformed(n, "line number too large")
	}
	num = int(parseDigits(s[:i]))
	return num, s[i:], nil
}

func parseDigits(s string) int64 {
	var v int64
	for i := 0; i < len(s); i++ {
		v = v*10 + int64(s[i]-'0')
	}
	return v
}

// markNoNewline flags the line before a "\ No newline at end of file"
// marker, which is the last line of one side or both.
func markNoNewline(h *Hunk, lastOld, lastNew bool, n int) error {
	switch {
	case lastOld && lastNew:
		h.Old[len(h.Old)-1].NoNewline = true
		h.New[len(h.New)-1].NoNewline = true
	case lastOld:
		h.Old[len(h.Old)-1].NoNewline = true
	case lastNew:
		h.New[len(h.New)-1].NoNewline = true
	default:
		return malformed(n, "marker without a line before it")
	}
	return nil
}

// headerName extracts the file name of a "--- " or "*** " header line,
// which the timestamp follows, separated by a tab.
func headerName(line string) string {
	if len(line) < 4 {
		return ""
	}
	rest := line[4:]
	if before, _, ok := strings.Cut(rest, "\t"); ok {
		return before
	}
	return rest
}

// malformed builds the error of a patch that can't be parsed.
func malformed(n int, reason string) error {
	return fmt.Errorf("malformed patch at line %d: %s", n, reason)
}

// scanner walks the lines of patch data.
type scanner struct {
	lines []string
	pos   int
}

func newScanner(data string) *scanner {
	lines := strings.Split(data, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // the split of the final newline
	}
	return &scanner{lines: lines}
}

// peek returns the line i positions ahead, without consuming it, with its
// 1-based number.
func (s *scanner) peek(i int) (string, int, bool) {
	if s.pos+i >= len(s.lines) {
		return "", s.pos + i + 1, false
	}
	return s.lines[s.pos+i], s.pos + i + 1, true
}

// next consumes and returns the next line, with its 1-based number.
func (s *scanner) next() (string, int, bool) {
	if s.pos >= len(s.lines) {
		return "", s.pos + 1, false
	}
	line := s.lines[s.pos]
	s.pos++
	return line, s.pos, true
}

// stripIndent removes the leading blanks shared by every line of the patch
// data, as patch files nested in other text end up with.
func stripIndent(data string) string {
	lines := strings.Split(data, "\n")
	n := -1
	for _, line := range lines {
		b := 0
		for b < len(line) && (line[b] == ' ' || line[b] == '\t') {
			b++
		}
		if b == len(line) {
			continue // a blank line has nothing to strip
		}
		if n < 0 || b < n {
			n = b
		}
	}
	if n <= 0 {
		return data
	}
	for i, line := range lines {
		lines[i] = line[min(n, len(line)):]
	}
	return strings.Join(lines, "\n")
}
