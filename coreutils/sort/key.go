package sort

import (
	"bytes"
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/snail/coreutils/internal/textutil"
)

// sortLine is an input line with the byte ranges of its keys.
type sortLine struct {
	data []byte
	keys []keyRange
}

// keyRange is a byte range of a line.
type keyRange struct {
	start, end int
}

// ordering holds the options that alter the comparison of a key.
type ordering struct {
	dictionary     bool
	foldCase       bool
	ignoreNonPrint bool
	numeric        bool
	reverse        bool
}

// compare returns the order of the keys a and b under the ordering: negative,
// zero or positive.
func (ord ordering) compare(a, b []byte) int {
	var c int
	if ord.numeric {
		c = compareNumeric(a, b)
	} else {
		c = bytes.Compare(ord.transform(a), ord.transform(b))
	}
	if ord.reverse {
		return -c
	}
	return c
}

// transform returns the key reduced to its significant characters, decoding
// it as UTF-8. Each byte that doesn't start a valid encoding is kept as is,
// unless -d or -i apply.
func (ord ordering) transform(key []byte) []byte {
	if !ord.dictionary && !ord.ignoreNonPrint && !ord.foldCase {
		return key
	}
	out := make([]byte, 0, len(key))
	for i := 0; i < len(key); {
		r, size := utf8.DecodeRune(key[i:])
		i += size
		if r == utf8.RuneError && size == 1 {
			// An invalid byte: not printable nor alphanumeric.
			if ord.dictionary || ord.ignoreNonPrint {
				continue
			}
			out = append(out, key[i-1])
			continue
		}
		if ord.dictionary && r != ' ' && r != '\t' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}
		if ord.ignoreNonPrint && !unicode.IsPrint(r) {
			continue
		}
		if ord.foldCase {
			r = unicode.ToUpper(r)
		}
		out = utf8.AppendRune(out, r)
	}
	return out
}

// union returns the combination of two orderings.
func (ord ordering) union(other ordering) ordering {
	return ordering{
		dictionary:     ord.dictionary || other.dictionary,
		foldCase:       ord.foldCase || other.foldCase,
		ignoreNonPrint: ord.ignoreNonPrint || other.ignoreNonPrint,
		numeric:        ord.numeric || other.numeric,
		reverse:        ord.reverse || other.reverse,
	}
}

// compareLines compares two lines, returning a negative, zero or positive
// value as the first orders before, with, or after the second. Lines whose
// keys compare equal fall back to the comparison of the whole lines, with all
// bytes significant and only -r in effect, unless -u was given.
func (opts options) compareLines(a, b sortLine) int {
	if len(opts.keys) == 0 {
		if c := opts.global.compare(a.data, b.data); c != 0 {
			return c
		}
	} else {
		for i := range opts.keys {
			ka := a.data[a.keys[i].start:a.keys[i].end]
			kb := b.data[b.keys[i].start:b.keys[i].end]
			if c := opts.keys[i].order.compare(ka, kb); c != 0 {
				return c
			}
		}
	}
	if opts.unique {
		return 0
	}
	c := bytes.Compare(a.data, b.data)
	if opts.global.reverse {
		return -c
	}
	return c
}

// equalKeys reports whether two lines have equal keys.
func (opts options) equalKeys(a, b sortLine) bool {
	if len(opts.keys) == 0 {
		return opts.global.compare(a.data, b.data) == 0
	}
	for i := range opts.keys {
		ka := a.data[a.keys[i].start:a.keys[i].end]
		kb := b.data[b.keys[i].start:b.keys[i].end]
		if opts.keys[i].order.compare(ka, kb) != 0 {
			return false
		}
	}
	return true
}

// newLine returns a line with the byte ranges of its keys computed.
func (opts options) newLine(data []byte) sortLine {
	line := sortLine{data: data}
	if len(opts.keys) > 0 {
		line.keys = make([]keyRange, len(opts.keys))
		for i := range opts.keys {
			line.keys[i] = opts.keys[i].extract(data, opts.sep)
		}
	}
	return line
}

// uniqueLines drops all but the first of each set of adjacent lines with
// equal keys.
func (opts options) uniqueLines(lines []sortLine) []sortLine {
	kept := lines[:0]
	for i, line := range lines {
		if i > 0 && opts.equalKeys(lines[i-1], line) {
			continue
		}
		kept = append(kept, line)
	}
	return kept
}

// mergeSets merges the presorted line sets into one ordered set.
func (opts options) mergeSets(sets [][]sortLine) []sortLine {
	var merged []sortLine
	idx := make([]int, len(sets))
	for {
		best := -1
		for i, set := range sets {
			if idx[i] == len(set) {
				continue
			}
			if best < 0 || opts.compareLines(set[idx[i]], sets[best][idx[best]]) < 0 {
				best = i
			}
		}
		if best < 0 {
			break
		}
		merged = append(merged, sets[best][idx[best]])
		idx[best]++
	}
	return merged
}

// keydef is a restricted sort key definition.
type keydef struct {
	start    keyPos
	end      keyPos
	hasEnd   bool
	order    ordering
	hasOrder bool
}

// keyPos is one end of a key definition: a 1-based field and character
// position, with the ordering modifiers attached to it. A zero character
// position of a key end means the last character of the field.
type keyPos struct {
	field  int
	char   int
	blank  bool
	ord    ordering
	hasOrd bool
}

// extract returns the byte range of the key in the line. The key is empty
// when its start falls beyond the end of the line or past its end.
func (k keydef) extract(line []byte, sep int) keyRange {
	start := fieldStart(line, k.start.field, sep)
	if k.start.blank {
		start = skipBlanks(line, start)
	}
	start = advanceChars(line, start, k.start.char-1)

	end := len(line)
	if k.hasEnd {
		if k.end.char == 0 {
			end = fieldEnd(line, k.end.field, sep)
		} else {
			end = fieldStart(line, k.end.field, sep)
			if k.end.blank {
				end = skipBlanks(line, end)
			}
			end = advanceChars(line, end, k.end.char)
		}
	}
	if start > end {
		start = end
	}
	return keyRange{start, end}
}

// fieldStart returns the offset where field f (1-based) starts: with a
// separator, one byte past its (f-1)-th occurrence; without one, the start of
// the blanks preceding the f-th run of non-blanks.
func fieldStart(line []byte, f int, sep int) int {
	off := 0
	for ; f > 1 && off < len(line); f-- {
		if sep >= 0 {
			i := bytes.IndexByte(line[off:], byte(sep))
			if i < 0 {
				return len(line)
			}
			off += i + 1
		} else {
			off = skipBlanks(line, off)
			off = skipNonBlanks(line, off)
		}
	}
	return off
}

// fieldEnd returns the offset one past the end of field f (1-based).
func fieldEnd(line []byte, f int, sep int) int {
	if sep >= 0 {
		off := fieldStart(line, f, sep)
		if i := bytes.IndexByte(line[off:], byte(sep)); i >= 0 {
			return off + i
		}
		return len(line)
	}
	off := 0
	for ; f > 0 && off < len(line); f-- {
		off = skipBlanks(line, off)
		off = skipNonBlanks(line, off)
	}
	return off
}

// skipBlanks returns the offset of the first non-blank at or after off.
func skipBlanks(line []byte, off int) int {
	for off < len(line) && textutil.IsBlank(line[off]) {
		off++
	}
	return off
}

// skipNonBlanks returns the offset of the first blank at or after off.
func skipNonBlanks(line []byte, off int) int {
	for off < len(line) && !textutil.IsBlank(line[off]) {
		off++
	}
	return off
}

// advanceChars returns the offset n characters after off, clamped to the end
// of the line. Each byte that doesn't start a valid UTF-8 encoding counts as
// one character.
func advanceChars(line []byte, off, n int) int {
	for ; n > 0 && off < len(line); n-- {
		_, size := utf8.DecodeRune(line[off:])
		off += size
	}
	return off
}

// parseKeydef parses a keydef option-argument:
// field_start[type][,field_end[type]].
func parseKeydef(s string) (keydef, error) {
	start, end, hasEnd := strings.Cut(s, ",")
	sp, ok := parseKeyPos(start, true)
	if !ok {
		return keydef{}, fmt.Errorf("invalid key definition: '%s'", s)
	}
	k := keydef{start: sp, hasEnd: hasEnd}
	if hasEnd {
		ep, ok := parseKeyPos(end, false)
		if !ok {
			return keydef{}, fmt.Errorf("invalid key definition: '%s'", s)
		}
		k.end = ep
	}
	k.order = k.start.ord.union(k.end.ord)
	k.hasOrder = k.start.hasOrd || k.end.hasOrd
	return k, nil
}

// parseKeyPos parses one end of a key definition: a field number, an optional
// character position after a dot, and the attached ordering modifiers.
func parseKeyPos(s string, start bool) (keyPos, bool) {
	var pos keyPos
	field, i := scanInt(s, 0)
	if field < 1 {
		return keyPos{}, false
	}
	pos.field = field
	pos.char = 1
	if !start {
		pos.char = 0
	}
	if i < len(s) && s[i] == '.' {
		var char int
		char, i = scanInt(s, i+1)
		if char < pos.char {
			return keyPos{}, false
		}
		pos.char = char
	}
	for ; i < len(s); i++ {
		switch s[i] {
		case 'b':
			pos.blank = true
		case 'd':
			pos.ord.dictionary = true
		case 'f':
			pos.ord.foldCase = true
		case 'i':
			pos.ord.ignoreNonPrint = true
		case 'n':
			pos.ord.numeric = true
		case 'r':
			pos.ord.reverse = true
		default:
			return keyPos{}, false
		}
		if s[i] != 'b' {
			pos.hasOrd = true
		}
	}
	return pos, true
}

// scanInt scans a decimal number from s starting at i, returning its value
// and the offset past it.
func scanInt(s string, i int) (int, int) {
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	n, err := strconv.Atoi(s[start:i])
	if err != nil {
		return 0, i
	}
	return n, i
}

// number is a parsed numeric string: optional blanks, an optional '-', and
// digits with an optional radix.
type number struct {
	negative bool
	whole    string // digits before the radix, without leading zeros
	frac     string // digits after the radix, without trailing zeros
}

// parseNumber parses the initial numeric string of b.
func parseNumber(b []byte) number {
	i := 0
	for i < len(b) && textutil.IsBlank(b[i]) {
		i++
	}
	var n number
	if i < len(b) && b[i] == '-' {
		n.negative = true
		i++
	}
	start := i
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		i++
	}
	n.whole = strings.TrimLeft(string(b[start:i]), "0")
	if i < len(b) && b[i] == '.' {
		i++
		start = i
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
		n.frac = strings.TrimRight(string(b[start:i]), "0")
	}
	return n
}

// sign returns -1, 0 or 1 as the number is negative, zero or positive. A zero
// value has no sign, no matter how it was written.
func (n number) sign() int {
	if n.whole == "" && n.frac == "" {
		return 0
	}
	if n.negative {
		return -1
	}
	return 1
}

// compareMagnitude compares the absolute values of two numbers.
func compareMagnitude(a, b number) int {
	if c := cmp.Compare(len(a.whole), len(b.whole)); c != 0 {
		return c
	}
	if c := strings.Compare(a.whole, b.whole); c != 0 {
		return c
	}
	return strings.Compare(a.frac, b.frac)
}

// compareNumeric compares the initial numeric strings of a and b by
// arithmetic value.
func compareNumeric(a, b []byte) int {
	na, nb := parseNumber(a), parseNumber(b)
	sa, sb := na.sign(), nb.sign()
	if sa != sb {
		return cmp.Compare(sa, sb)
	}
	if sa == 0 {
		return 0
	}
	c := compareMagnitude(na, nb)
	if sa < 0 {
		return -c
	}
	return c
}
