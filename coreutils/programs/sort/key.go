package sort

import (
	"bytes"
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/turtle/coreutils/internal/textutil"
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
// sortKind is the comparison a key uses. The last kind given by an option
// wins: a later sort-type option replaces the earlier one.
type sortKind int

const (
	lexical sortKind = iota
	numeric
	humanNumeric
	month
	random
	version
)

type ordering struct {
	dictionary     bool
	foldCase       bool
	ignoreNonPrint bool
	kind           sortKind
	reverse        bool
}

// compare returns the order of the keys a and b under the ordering: negative,
// zero or positive.
func (ord ordering) compare(a, b []byte) int {
	var c int
	switch ord.kind {
	case numeric:
		c = compareNumeric(a, b)
	case humanNumeric:
		c = compareHumanNumeric(a, b)
	case month:
		c = compareMonth(a, b)
	case random:
		c = compareRandom(ord.transform(a), ord.transform(b))
	case version:
		c = compareVersion(a, b)
	default:
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

// union returns the combination of two orderings. The kind of the second
// one, usually a key's own, replaces the first's when it selects one.
func (ord ordering) union(other ordering) ordering {
	out := ordering{
		dictionary:     ord.dictionary || other.dictionary,
		foldCase:       ord.foldCase || other.foldCase,
		ignoreNonPrint: ord.ignoreNonPrint || other.ignoreNonPrint,
		kind:           ord.kind,
		reverse:        ord.reverse || other.reverse,
	}
	if other.kind != lexical {
		out.kind = other.kind
	}
	return out
}

// compareLines compares two lines, returning a negative, zero or positive
// value as the first orders before, with, or after the second. Lines whose
// keys compare equal fall back to the comparison of the whole lines, with all
// bytes significant and only -r in effect, unless -u or -s was given.
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
	if opts.unique || opts.stable {
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
		case 'h':
			pos.ord.kind = humanNumeric
		case 'i':
			pos.ord.ignoreNonPrint = true
		case 'M':
			pos.ord.kind = month
		case 'n':
			pos.ord.kind = numeric
		case 'R':
			pos.ord.kind = random
		case 'r':
			pos.ord.reverse = true
		case 'V':
			pos.ord.kind = version
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

// compareHumanNumeric compares the initial numbers of a and b followed by
// their optional SI suffixes, first by the order of magnitude of the unit
// and then by numeric value.
func compareHumanNumeric(a, b []byte) int {
	if c := cmp.Compare(findUnitOrder(a), findUnitOrder(b)); c != 0 {
		return c
	}
	return compareNumeric(a, b)
}

// findUnitOrder returns the order of magnitude of the unit following the
// initial number of b: negative numbers get negative orders, and zero or
// invalid numbers get zero.
func findUnitOrder(b []byte) int {
	i := 0
	for i < len(b) && textutil.IsBlank(b[i]) {
		i++
	}
	minus := false
	if i < len(b) && b[i] == '-' {
		minus = true
		i++
	}
	var maxDigit byte
	for i < len(b) && isDigit(b[i]) {
		if b[i] > maxDigit {
			maxDigit = b[i]
		}
		i++
	}
	if i < len(b) && b[i] == '.' {
		i++
		for i < len(b) && isDigit(b[i]) {
			if b[i] > maxDigit {
				maxDigit = b[i]
			}
			i++
		}
	}
	order := 0
	if maxDigit > '0' && i < len(b) {
		order = humanUnitOrder(b[i])
	}
	if minus {
		return -order
	}
	return order
}

// humanUnitOrder returns the order of magnitude of an SI suffix byte: 1 for
// k, 2 for M, and so on up to 10 for Q, and 0 for any other byte.
func humanUnitOrder(c byte) int {
	switch c {
	case 'k', 'K':
		return 1
	case 'm', 'M':
		return 2
	case 'g', 'G':
		return 3
	case 't', 'T':
		return 4
	case 'p', 'P':
		return 5
	case 'e', 'E':
		return 6
	case 'z', 'Z':
		return 7
	case 'y', 'Y':
		return 8
	case 'r', 'R':
		return 9
	case 'q', 'Q':
		return 10
	}
	return 0
}

// compareMonth compares the initial month names of a and b, case folded,
// with unknown months sorting low to known ones.
func compareMonth(a, b []byte) int {
	return cmp.Compare(monthValue(a), monthValue(b))
}

// monthNames holds the three-letter month abbreviations, from January on.
var monthNames = [12]string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}

// monthValue returns the 1-based month of the initial month name of b, and
// 0 when the name is unknown. A recognized abbreviation may be followed by
// any amount of characters, like in JANUARY.
func monthValue(b []byte) int {
	i := 0
	for i < len(b) && textutil.IsBlank(b[i]) {
		i++
	}
	if len(b)-i < 3 {
		return 0
	}
	prefix := strings.ToUpper(string(b[i : i+3]))
	for m, name := range monthNames {
		if prefix == name {
			return m + 1
		}
	}
	return 0
}

// randomSeed is the fixed seed of the random hash. The --random-source
// option is not supported, so the permutation is deterministic.
const randomSeed = 0x1B873593

// compareRandom compares the transformed keys a and b by their hash values,
// falling back to the key bytes in the unlikely event of a collision.
func compareRandom(a, b []byte) int {
	if c := cmp.Compare(hashKey(a), hashKey(b)); c != 0 {
		return c
	}
	return bytes.Compare(a, b)
}

// hashKey returns the FNV-1a hash of b, seeded by folding randomSeed into
// the offset basis.
func hashKey(b []byte) uint64 {
	h := uint64(14695981039346656037) ^ randomSeed
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}

// compareVersion compares a and b by the version sort ordering rules: from
// left to right, initial runs of non-digits are compared lexically, with
// letters before other bytes and tilde before anything, then initial runs
// of digits are compared by numeric value, until a difference is found or
// both strings are exhausted.
func compareVersion(a, b []byte) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		for (i < len(a) && !isDigit(a[i])) || (j < len(b) && !isDigit(b[j])) {
			ca, cb := versionOrder(a, i), versionOrder(b, j)
			if ca != cb {
				return ca - cb
			}
			i++
			j++
		}
		for i < len(a) && a[i] == '0' {
			i++
		}
		for j < len(b) && b[j] == '0' {
			j++
		}
		firstDiff := 0
		for i < len(a) && j < len(b) && isDigit(a[i]) && isDigit(b[j]) {
			if firstDiff == 0 {
				firstDiff = int(a[i]) - int(b[j])
			}
			i++
			j++
		}
		if i < len(a) && isDigit(a[i]) {
			return 1
		}
		if j < len(b) && isDigit(b[j]) {
			return -1
		}
		if firstDiff != 0 {
			return firstDiff
		}
	}
	return 0
}

// versionOrder returns the comparison value of the byte of s at pos, or of
// the end of the string, as the version sort ordering rules demand.
func versionOrder(s []byte, pos int) int {
	if pos == len(s) {
		return -1
	}
	c := s[pos]
	switch {
	case isDigit(c):
		return 0
	case isASCIILetter(c):
		return int(c)
	case c == '~':
		return -2
	default:
		return int(c) + 0x100
	}
}

// isDigit reports whether c is an ASCII digit.
func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// isASCIILetter reports whether c is an ASCII letter.
func isASCIILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
