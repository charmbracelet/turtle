// Package charset implements the character set syntax shared by the
// utilities that translate characters, like tr and the y command of sed.
package charset

import (
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Kind is the kind of a set element.
type Kind uint8

// The kinds of elements a set can hold.
const (
	// Char is a single character, given by itself or an escape sequence.
	Char Kind = iota
	// Class is a character class, like the "[:alpha:]" of tr.
	Class
	// Equiv is an equivalence class, like the "[=e=]" of tr.
	Equiv
	// Repeat is a repetition, like the "[x*n]" of tr.
	Repeat
)

// Elem is one element of a set operand.
type Elem struct {
	Kind Kind

	// Char is the character of a Char, Equiv or Repeat element.
	Char rune

	// Class is the class name of a Class element.
	Class string

	// Repeat is how many times Char repeats in a Repeat element. A
	// negative count fills the set to the length of the one it is paired
	// with.
	Repeat int

	// Raw reports whether Char came from an octal sequence or an invalid
	// byte, in which case it is written as a single byte.
	Raw bool
}

// Member is a character of an expanded set.
type Member struct {
	Char rune

	// Raw reports whether Char should be written as a single byte.
	Raw bool
}

// Parse parses s as a set of characters, with the syntax of the tr operands:
// characters, backslash escapes and octal sequences, c-c ranges, character
// classes, equivalence classes and [x*n] repetitions.
func Parse(s string) ([]Elem, error) {
	var elems []Elem
	for i := 0; i < len(s); {
		elem, size, err := parseElem(s[i:])
		if err != nil {
			return nil, err
		}
		// A range needs two single character endpoints. A trailing dash, or
		// one followed by another construct, stays an ordinary character.
		if elem.Kind == Char && i+size < len(s) && s[i+size] == '-' && i+size+1 < len(s) {
			end, endSize, err := parseElem(s[i+size+1:])
			if err != nil {
				return nil, err
			}
			if end.Kind == Char {
				if end.Char < elem.Char {
					span := s[i : i+size+1+endSize]
					return nil, fmt.Errorf("range-endpoints of %q are in reverse collating sequence order", span)
				}
				elems = append(elems, rangeElems(elem, end)...)
				i += size + 1 + endSize
				continue
			}
		}
		elems = append(elems, elem)
		i += size
	}
	return elems, nil
}

// rangeElems expands the range between two character elements, inclusive.
func rangeElems(start, end Elem) []Elem {
	raw := start.Raw || end.Raw
	elems := make([]Elem, 0, end.Char-start.Char+1)
	for r := start.Char; ; r++ {
		elems = append(elems, Elem{Kind: Char, Char: r, Raw: raw})
		if r == end.Char {
			break
		}
	}
	return elems
}

// parseElem parses the element at the start of s, returning it with how many
// bytes it takes.
func parseElem(s string) (Elem, int, error) {
	if s[0] == '[' {
		if elem, size, ok, err := parseBracket(s); ok || err != nil {
			return elem, size, err
		}
	}
	r, raw, size := parseChar(s)
	return Elem{Kind: Char, Char: r, Raw: raw}, size, nil
}

// parseBracket parses a bracketed construct at the start of s, which begins
// with '['. When s doesn't hold one, it reports ok as false and consumes
// nothing. A construct with an invalid name, like an unknown character
// class, returns an error; one with an incomplete shape doesn't, as its
// characters stay ordinary.
func parseBracket(s string) (elem Elem, size int, ok bool, err error) {
	rest := s[1:]
	if strings.HasPrefix(rest, ":") {
		if end := strings.Index(rest, ":]"); end > 1 && isName(rest[1:end]) {
			name := rest[1:end]
			if _, valid := classPredicates[name]; !valid {
				return Elem{}, 0, false, fmt.Errorf("invalid character class %q", name)
			}
			return Elem{Kind: Class, Class: name}, 1 + end + 2, true, nil
		}
	}
	if strings.HasPrefix(rest, "=") {
		if char, size, ok := parseDelim(rest[1:], "=]"); ok {
			return Elem{Kind: Equiv, Char: char}, 1 + 1 + size + 2, true, nil
		}
	}
	if rest == "" {
		return Elem{}, 0, false, nil
	}
	char, raw, size := parseChar(rest)
	after := rest[size:]
	if strings.HasPrefix(after, "*") {
		after = after[1:]
		digits := 0
		for digits < len(after) && isDigit(after[digits]) {
			digits++
		}
		if after[digits:] == "]" {
			count, err := repeatCount(after[:digits])
			if err != nil {
				return Elem{}, 0, false, err
			}
			return Elem{Kind: Repeat, Char: char, Repeat: count, Raw: raw}, 1 + size + 1 + digits + 1, true, nil
		}
	}
	return Elem{}, 0, false, nil
}

// parseDelim parses the character at the start of s when it is followed by
// the given delimiter, reporting both.
func parseDelim(s, delim string) (rune, int, bool) {
	if s == "" {
		return 0, 0, false
	}
	r, _, size := parseChar(s)
	if strings.HasPrefix(s[size:], delim) {
		return r, size, true
	}
	return 0, 0, false
}

// repeatCount returns the count of a repetition from its digits, which are
// given after the '*'. An empty or zero count fills the set instead,
// reported as -1. A leading zero makes the digits an octal value.
func repeatCount(digits string) (int, error) {
	if digits == "" {
		return -1, nil
	}
	base := 10
	if digits[0] == '0' {
		base = 8
	}
	// The value is checked as it grows, so that a count with more digits
	// than the machine can hold can't wrap around and slip through.
	n := 0
	for i := 0; i < len(digits); i++ {
		d := int(digits[i] - '0')
		if d < 0 || d >= base {
			return 0, fmt.Errorf("invalid repetition count %q", digits)
		}
		if n > maxRepeat {
			return 0, fmt.Errorf("repetition count %q is too large", digits)
		}
		n = n*base + d
	}
	if n > maxRepeat {
		return 0, fmt.Errorf("repetition count %q is too large", digits)
	}
	if n == 0 {
		return -1, nil
	}
	return n, nil
}

// maxRepeat caps the count of a repetition, so that counts like
// [c*99999999999999999999] can't allocate without bound.
const maxRepeat = 1 << 20

// parseChar parses the character at the start of s, returning it with how
// many bytes it takes. The character can be given by an escape sequence,
// including octal ones, and the report raw tells whether it came from an
// octal sequence or an invalid byte.
func parseChar(s string) (r rune, raw bool, size int) {
	if s[0] == '\\' {
		if len(s) == 1 {
			return '\\', false, 1
		}
		if esc, ok := escapes[s[1]]; ok {
			return esc, false, 2
		}
		if isOctal(s[1]) {
			size = 2
			for size < 4 && size < len(s) && isOctal(s[size]) {
				size++
			}
			v := 0
			for _, c := range s[1:size] {
				v = v*8 + int(c-'0')
			}
			return rune(v), true, size
		}
		r, rsize := decodeRune(s[1:])
		if r < 0 {
			return rune(s[1]), true, 2
		}
		return r, false, 1 + rsize
	}
	r, rsize := decodeRune(s)
	if r < 0 {
		return rune(s[0]), true, 1
	}
	return r, false, rsize
}

// decodeRune decodes the first rune of s. An invalid encoding reports a
// negative rune, so that callers can tell it from a NUL.
func decodeRune(s string) (rune, int) {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size == 1 {
		return -1, 1
	}
	return r, size
}

var escapes = map[byte]rune{
	'a':  '\a',
	'b':  '\b',
	'f':  '\f',
	'n':  '\n',
	'r':  '\r',
	't':  '\t',
	'v':  '\v',
	'\\': '\\',
}

func isOctal(c byte) bool { return '0' <= c && c <= '7' }
func isDigit(c byte) bool { return '0' <= c && c <= '9' }

func isName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

// Expand returns the characters of e, in order. fillLen is how many more
// characters a repetition without a count must produce, or a negative number
// when they are not allowed.
func (e Elem) Expand(fillLen int) ([]Member, error) {
	switch e.Kind {
	case Char, Equiv:
		return []Member{{Char: e.Char, Raw: e.Raw}}, nil
	case Class:
		runes := classRunes(e.Class)
		members := make([]Member, len(runes))
		for i, r := range runes {
			members[i] = Member{Char: r}
		}
		return members, nil
	case Repeat:
		count := e.Repeat
		if count < 0 {
			if fillLen < 0 {
				return nil, fmt.Errorf("repetition of %q without a count", string(e.Char))
			}
			count = fillLen
		}
		members := make([]Member, count)
		for i := range members {
			members[i] = Member{Char: e.Char, Raw: e.Raw}
		}
		return members, nil
	}
	return nil, nil
}

// Expand expands elems to its characters, in order, filling repetitions
// without a count so the result has fillLen characters. A negative fillLen
// rejects them.
func Expand(elems []Elem, fillLen int) ([]Member, error) {
	var members []Member
	for _, e := range elems {
		expanded, err := e.Expand(fillLen - len(members))
		if err != nil {
			return nil, err
		}
		members = append(members, expanded...)
	}
	return members, nil
}

// CasePairs returns the characters of a case conversion, as the pairs of the
// toupper or tolower mappings of the current character set. When
// lowerInString1, from holds the lowercase characters and to their uppercase
// counterparts, the other way around otherwise. The characters match by
// position, so that a set can hold the first of each pair and another one
// the second.
func CasePairs(lowerInString1 bool) (from, to []rune) {
	classMutex.Lock()
	defer classMutex.Unlock()

	name := "upper"
	if lowerInString1 {
		name = "lower"
	}
	if pairs, ok := pairCache[name]; ok {
		return pairs.from, pairs.to
	}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if !utf8.ValidRune(r) {
			continue
		}
		var other rune
		if lowerInString1 {
			if !unicode.IsLower(r) {
				continue
			}
			other = unicode.ToUpper(r)
		} else {
			if !unicode.IsUpper(r) {
				continue
			}
			other = unicode.ToLower(r)
		}
		if other != r && utf8.ValidRune(other) {
			from = append(from, r)
			to = append(to, other)
		}
	}
	pairCache[name] = casePair{from: from, to: to}
	return from, to
}

type casePair struct {
	from, to []rune
}

var pairCache = map[string]casePair{}

// classRunes returns the characters of the named class, in ascending order.
func classRunes(name string) []rune {
	classMutex.Lock()
	defer classMutex.Unlock()

	if runes, ok := classCache[name]; ok {
		return runes
	}
	match := classPredicates[name]
	var runes []rune
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if utf8.ValidRune(r) && match(r) {
			runes = append(runes, r)
		}
	}
	classCache[name] = runes
	return runes
}

var (
	classMutex sync.Mutex
	classCache = map[string][]rune{}
)

// classPredicates holds the character classes of the current character set.
var classPredicates = map[string]func(rune) bool{
	"alnum": func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) },
	"alpha": unicode.IsLetter,
	"blank": func(r rune) bool { return r == ' ' || r == '\t' },
	"cntrl": unicode.IsControl,
	"digit": unicode.IsDigit,
	"graph": func(r rune) bool { return unicode.IsGraphic(r) && !unicode.IsSpace(r) },
	"lower": unicode.IsLower,
	"print": unicode.IsGraphic,
	"punct": func(r rune) bool {
		return unicode.IsGraphic(r) && !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r)
	},
	"space":  unicode.IsSpace,
	"upper":  unicode.IsUpper,
	"xdigit": func(r rune) bool { return strings.ContainsRune("0123456789abcdefABCDEF", r) },
}
