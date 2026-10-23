package diff

import (
	"strings"
)

// PatchFormat is the format a patch file is written in.
type PatchFormat uint8

const (
	// UnknownPatch marks data no patch was found in.
	UnknownPatch PatchFormat = iota
	// UnifiedPatch is the unified context format, the output of diff -u.
	UnifiedPatch
	// ContextPatch is the copied context format, the output of diff -c.
	ContextPatch
)

// DetectPatch reports the format the patch data seems to be in.
func DetectPatch(data string) PatchFormat {
	lines := strings.Split(stripIndent(data), "\n")
	for i := range lines {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "@@ -"):
			return UnifiedPatch
		case strings.HasPrefix(line, "********"):
			return ContextPatch
		case strings.HasPrefix(line, "--- ") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "+++ "):
			return UnifiedPatch
		case isFileHeader(line) && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "--- ") && !strings.HasSuffix(lines[i+1], "----"):
			return ContextPatch
		}
	}
	return UnknownPatch
}

// ParseContext parses patch data as copied context diffs, returning one
// Patch per file. The lines around the patches are skipped.
func ParseContext(data string) ([]Patch, error) {
	s := newScanner(stripIndent(data))
	var patches []Patch
	for {
		p, err := parseContextPatch(s)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return patches, nil
		}
		patches = append(patches, *p)
	}
}

// parseContextPatch parses the next patch of the scanner, returning nil at
// the end of the data.
func parseContextPatch(s *scanner) (*Patch, error) {
	p := &Patch{}
	for {
		line, _, ok := s.peek(0)
		if !ok {
			return nil, nil
		}
		if isFileHeader(line) {
			if next, _, ok := s.peek(1); ok && strings.HasPrefix(next, "--- ") && !strings.HasSuffix(next, "----") {
				from, _, _ := s.next()
				p.From = headerName(from)
				to, _, _ := s.next()
				p.To = headerName(to)
				break
			}
		}
		if strings.HasPrefix(line, "********") {
			break
		}
		if strings.HasPrefix(line, "Index: ") {
			p.Index = strings.TrimSpace(line[len("Index: "):])
		}
		s.next()
	}
	for {
		line, _, ok := s.peek(0)
		if !ok || !strings.HasPrefix(line, "********") {
			break
		}
		h, err := parseContextHunk(s)
		if err != nil {
			return nil, err
		}
		p.Hunks = append(p.Hunks, h)
	}
	if len(p.Hunks) == 0 {
		_, n, _ := s.peek(0)
		return nil, malformed(n, "no hunks after the headers")
	}
	return p, nil
}

// isFileHeader reports whether the line is the "*** name" header of a file
// patch, which the old range line of a hunk resembles.
func isFileHeader(line string) bool {
	return strings.HasPrefix(line, "*** ") && !strings.HasSuffix(line, "****")
}

// parseContextHunk parses one hunk, between "***************" separators.
func parseContextHunk(s *scanner) (Hunk, error) {
	var h Hunk
	s.next() // the separator

	line, n, ok := s.next()
	if !ok || !strings.HasPrefix(line, "*** ") {
		return h, malformed(n, "missing old range in the hunk")
	}
	from, wantOld, err := parseContextRange(line, n)
	if err != nil {
		return h, err
	}
	h.FromLine = from

	var body string
	var lastOld bool
	for {
		body, n, ok = s.next()
		if !ok {
			return h, malformed(n, "hunk ends in the middle")
		}
		if isNewRange(body) {
			break
		}
		// Every line of a context hunk holds a marker and a blank before
		// its content, so shorter ones are malformed.
		if len(body) < 2 {
			return h, malformed(n, "truncated line in the old part of the hunk")
		}
		switch {
		case body[0] == ' ':
			h.Old = append(h.Old, HunkLine{Kind: ContextLine, Content: body[2:]})
			lastOld = true
		case body[0] == '-' || body[0] == '!':
			h.Old = append(h.Old, HunkLine{Kind: DeleteLine, Content: body[2:]})
			lastOld = true
		case body[0] == '\\':
			if !lastOld {
				return h, malformed(n, "marker without a line before it")
			}
			h.Old[len(h.Old)-1].NoNewline = true
		default:
			return h, malformed(n, "unexpected line in the old part of the hunk")
		}
	}
	if wantOld >= 0 && len(h.Old) != wantOld {
		return h, malformed(n, "old part line count doesn't match its range")
	}

	to, wantNew, err := parseContextRange(body, n)
	if err != nil {
		return h, err
	}
	h.ToLine = to
	var lastNew bool
	for {
		body, n, ok := s.peek(0)
		if !ok || wantNew >= 0 && len(h.New) >= wantNew {
			break
		}
		if body == "" || body[0] != ' ' && body[0] != '+' && body[0] != '!' && body[0] != '\\' {
			break
		}
		s.next()
		if len(body) < 2 {
			return h, malformed(n, "truncated line in the new part of the hunk")
		}
		switch {
		case body[0] == ' ':
			h.New = append(h.New, HunkLine{Kind: ContextLine, Content: body[2:]})
			lastNew = true
		case body[0] == '+' || body[0] == '!':
			h.New = append(h.New, HunkLine{Kind: InsertLine, Content: body[2:]})
			lastNew = true
		case body[0] == '\\':
			if !lastNew {
				return h, malformed(n, "marker without a line before it")
			}
			h.New[len(h.New)-1].NoNewline = true
		}
	}
	if wantNew >= 0 && len(h.New) != wantNew {
		return h, malformed(n, "new part line count doesn't match its range")
	}
	// The marker of the last line can trail the new part.
	if line, n, ok := s.peek(0); ok && strings.HasPrefix(line, "\\") {
		s.next()
		if !lastNew {
			return h, malformed(n, "marker without a line before it")
		}
		h.New[len(h.New)-1].NoNewline = true
	}
	return h, nil
}

// isNewRange reports whether the line is the "--- N,M ----" range that ends
// the old part of a context hunk.
func isNewRange(line string) bool {
	if !strings.HasPrefix(line, "--- ") || !strings.HasSuffix(line, "----") {
		return false
	}
	return isRangeNums(strings.TrimSpace(line[4 : len(line)-4]))
}

// parseContextRange parses the "N" or "N,M" range of a hunk header line, with
// its star or dash padding, giving the start line and its count, which is
// unknown when the range has the single number form.
func parseContextRange(line string, n int) (start, count int, err error) {
	if len(line) < 4 || (line[:4] != "*** " && line[:4] != "--- ") {
		return 0, 0, malformed(n, "invalid range in the hunk header")
	}
	rest := strings.TrimSpace(line[4:])
	if strings.HasSuffix(rest, "****") || strings.HasSuffix(rest, "----") {
		rest = strings.TrimSpace(rest[:len(rest)-4])
	}
	start, rest, err = rangeNum(rest, n)
	if err != nil {
		return 0, 0, err
	}
	if rest == "" {
		return start, -1, nil
	}
	if rest[0] != ',' {
		return 0, 0, malformed(n, "invalid range in the hunk header")
	}
	end, rest, err := rangeNum(rest[1:], n)
	if err != nil {
		return 0, 0, err
	}
	if rest != "" {
		return 0, 0, malformed(n, "invalid range in the hunk header")
	}
	return start, end - start + 1, nil
}

// isRangeNums reports whether s is a "N" or "N,M" pair of line numbers.
func isRangeNums(s string) bool {
	_, rest, err := rangeNum(s, 0)
	if err != nil {
		return false
	}
	if rest == "" {
		return true
	}
	if rest[0] != ',' {
		return false
	}
	_, rest, err = rangeNum(rest[1:], 0)
	return err == nil && rest == ""
}
