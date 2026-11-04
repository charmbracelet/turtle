package grep

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/snail/coreutils/internal/bre"
	"charm.land/snail/coreutils/internal/ere"
)

// matcher reports whether a line is selected by the patterns, and where the
// patterns match it.
type matcher struct {
	re     *regexp.Regexp // the patterns, as one alternation
	invert bool
	words  bool
}

// newMatcher compiles the patterns as one alternation, as the options ask
// for.
func newMatcher(patterns []string, opts options) (*matcher, error) {
	var parts []string
	for _, pattern := range patterns {
		var translated string
		var err error
		switch opts.mode {
		case 'F':
			translated = regexp.QuoteMeta(pattern)
		case 'E':
			translated, err = ere.Translate(pattern)
		default:
			translated, err = bre.Translate(pattern)
		}
		if err != nil {
			return nil, err
		}
		parts = append(parts, translated)
	}
	body := strings.Join(parts, "|")
	if len(parts) > 1 {
		body = "(?:" + body + ")"
	}
	if opts.whole {
		body = `\A(?:` + body + `)\z`
	}
	if opts.ignoreCase {
		body = "(?i)" + body
	}
	re, err := regexp.Compile(body)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %v", err)
	}
	// -w has no effect when -x is also given.
	words := opts.words && !opts.whole
	return &matcher{re: re, invert: opts.invert, words: words}, nil
}

// matches reports whether line is selected.
func (m *matcher) matches(line []byte) bool {
	return m.matched(line) != m.invert
}

// matched reports whether the patterns match line, at whole words only when
// -w asks for it.
func (m *matcher) matched(line []byte) bool {
	if !m.words {
		return m.re.Match(line)
	}
	return len(m.find(line)) > 0
}

// find returns where the patterns match line, at whole words only when -w
// asks for it.
func (m *matcher) find(line []byte) [][]int {
	var out [][]int
	for _, loc := range m.re.FindAllIndex(line, -1) {
		if !m.words || isWordMatch(line, loc) {
			out = append(out, loc)
		}
	}
	return out
}

// isWordMatch reports whether a match begins and ends at the edge of a
// word, whose constituents are letters, digits and the underscore.
func isWordMatch(line []byte, loc []int) bool {
	if loc[0] > 0 && isWordByte(line[loc[0]-1]) {
		return false
	}
	if loc[1] < len(line) && isWordByte(line[loc[1]]) {
		return false
	}
	return true
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' ||
		b >= '0' && b <= '9' || b == '_'
}
