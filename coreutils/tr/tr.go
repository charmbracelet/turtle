// Package tr implements the POSIX tr utility.
//
// The input and the operands are handled as characters: the strings are
// decoded as UTF-8, and octal sequences as the characters with the same
// values, written as single bytes. Bytes which aren't valid UTF-8 are
// treated as characters with their own values, so that binary input keeps
// its bytes unless they are translated. The -c and -C options complement
// the same set: for deletions and squeezing it holds every character not
// given, while for translations it holds the byte values, from 0 to 255,
// which string1 doesn't give.
package tr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/charset"
	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/getopt"
)

// chunkSize is how many bytes of the input are read at a time, which is also
// how often the context is checked while reading.
const chunkSize = 32 * 1024

// Prog is the tr core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run translates the characters of the standard input as told by the
// operands, writing the result to the standard output.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	if err := checkOperands(opts, operands); err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	t, err := newTranslator(opts, operands)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	if p.Stdin == nil {
		p.errorf("no standard input available")
		return &coreutils.ExitError{Code: 1}
	}
	if err := p.copy(ctx, t); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// copy reads the standard input in chunks, translating each of them, and
// writes the result to the standard output. A character split across chunks
// is held until the next one completes it.
func (p *Prog) copy(ctx context.Context, t *translator) error {
	in := make([]byte, chunkSize)
	out := make([]byte, 0, chunkSize)
	var carry []byte
	r := ctxio.NewReader(ctx, p.Stdin)
	for {
		n, readErr := r.Read(in)
		if n > 0 {
			data := append(carry, in[:n]...)
			// At the end of the input every byte is final, even when it
			// doesn't form a valid character.
			hold := 0
			if readErr == nil {
				hold = partialTail(data)
			}
			out = t.translate(data[:len(data)-hold], out)
			carry = append(carry[:0], data[len(data)-hold:]...)
			if _, err := p.Stdout.Write(out); err != nil {
				return fmt.Errorf("write error: %w", err)
			}
			out = out[:0]
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return fmt.Errorf("read error: %w", readErr)
		}
	}
}

// partialTail returns how many of the trailing bytes of data may start a
// character which the next bytes of the input complete, so that they can be
// held until then.
func partialTail(data []byte) int {
	for i := max(0, len(data)-3); i < len(data); i++ {
		if partialRune(data[i:]) {
			return len(data) - i
		}
	}
	return 0
}

// partialRune reports whether tail holds the start of a character which the
// following bytes may complete.
func partialRune(tail []byte) bool {
	var want int
	switch c := tail[0]; {
	case c >= 0xc2 && c <= 0xdf:
		want = 2
	case c >= 0xe0 && c <= 0xef:
		want = 3
	case c >= 0xf0 && c <= 0xf4:
		want = 4
	default:
		return false
	}
	if len(tail) >= want {
		return false
	}
	for _, b := range tail[1:] {
		if b&0xc0 != 0x80 {
			return false
		}
	}
	return true
}

func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "tr: "+format+"\n", args...)
}

type options struct {
	complement bool
	delete     bool
	squeeze    bool
}

func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := (getopt.Set{}).Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'c', 'C':
			opts.complement = true
		case 'd':
			opts.delete = true
		case 's':
			opts.squeeze = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	return opts, operands, nil
}

// checkOperands checks the operand count against the options. Deletion
// takes one operand, or two when squeezing as well, while the other modes
// take one or two.
func checkOperands(opts options, operands []string) error {
	min, max := 2, 2
	switch {
	case opts.delete && opts.squeeze:
		min, max = 2, 2
	case opts.delete:
		min, max = 1, 1
	case opts.squeeze:
		min, max = 1, 2
	}
	switch {
	case len(operands) < min:
		if len(operands) == 0 {
			return errors.New("missing operand")
		}
		return fmt.Errorf("missing operand after %q", operands[len(operands)-1])
	case len(operands) > max:
		return fmt.Errorf("extra operand %q", operands[max])
	}
	return nil
}

// newTranslator builds the transformations told by the options and operands.
func newTranslator(opts options, operands []string) (*translator, error) {
	elems1, err := charset.Parse(operands[0])
	if err != nil {
		return nil, err
	}
	var elems2 []charset.Elem
	if len(operands) > 1 {
		if elems2, err = charset.Parse(operands[1]); err != nil {
			return nil, err
		}
	}
	for _, e := range elems1 {
		if e.Kind == charset.Repeat {
			return nil, errors.New("repetitions can only be used in string2")
		}
	}

	t := &translator{}
	switch {
	case opts.delete && opts.squeeze:
		if t.del, err = newSet(elems1, opts.complement); err != nil {
			return nil, err
		}
		var squeeze []charset.Member
		if squeeze, err = charset.Expand(elems2, -1); err != nil {
			return nil, err
		}
		t.squeeze = setOf(squeeze)
	case opts.delete:
		if t.del, err = newSet(elems1, opts.complement); err != nil {
			return nil, err
		}
	case opts.squeeze && len(operands) == 1:
		if t.squeeze, err = newSet(elems1, opts.complement); err != nil {
			return nil, err
		}
	default:
		conversions, err := caseConversions(elems1, elems2)
		if err != nil {
			return nil, err
		}
		members1, members2, plain, err := expandPair(elems1, elems2, conversions, opts.complement)
		if err != nil {
			return nil, err
		}
		t.replace = mapOf(members1, members2)
		if opts.complement {
			t.complSet = setOf(plain)
			var last *charset.Member
			if len(members2) > 0 {
				last = &members2[len(members2)-1]
			}
			t.complTo = last
		}
		if opts.squeeze {
			t.squeeze = setOf(members2)
		}
	}
	return t, nil
}

// translator holds the transformations applied to the input: deletions,
// translations and squeezing.
type translator struct {
	// replace maps the characters of string1 to their counterparts of
	// string2, or is nil when only deleting or squeezing.
	replace map[rune]charset.Member

	// complSet and complTo translate the characters beyond the byte values
	// of a complemented translation, which have no position to pair with
	// string2: when such a character isn't in string1, it becomes the last
	// character of string2, the one that pads it.
	complSet set
	complTo  *charset.Member

	del     set
	squeeze set

	// last is the last character written, used to squeeze repetitions.
	last    rune
	written bool
}

// translate transforms data, appending the result to out and returning it.
func (t *translator) translate(data, out []byte) []byte {
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		key := r
		if r == utf8.RuneError && size == 1 {
			key = rune(data[i])
		}
		if t.del.has(key) {
			i += size
			continue
		}
		char, raw, ok := key, false, false
		if t.replace != nil {
			if m, found := t.replace[key]; found {
				char, raw, ok = m.Char, m.Raw, true
			} else if t.complTo != nil && key > 0xff && !t.complSet.has(key) {
				char, raw, ok = t.complTo.Char, t.complTo.Raw, true
			}
		}
		if t.squeeze.has(char) && t.written && t.last == char {
			i += size
			continue
		}
		if ok {
			var buf [4]byte
			n := 1
			if raw {
				buf[0] = byte(char)
			} else {
				n = utf8.EncodeRune(buf[:], char)
			}
			out = append(out, buf[:n]...)
		} else {
			out = append(out, data[i:i+size]...)
		}
		t.last, t.written = char, true
		i += size
	}
	return out
}

// set holds the characters of a set, or the ones it doesn't hold when it is
// complemented.
type set struct {
	members map[rune]bool
	compl   bool
}

func (s set) has(r rune) bool {
	return s.members[r] != s.compl
}

// setOf builds the set of the given characters.
func setOf(members []charset.Member) set {
	m := make(map[rune]bool, len(members))
	for _, member := range members {
		m[member.Char] = true
	}
	return set{members: m}
}

// newSet expands elems and builds its set, complemented or not.
func newSet(elems []charset.Elem, complement bool) (set, error) {
	members, err := charset.Expand(elems, -1)
	if err != nil {
		return set{}, err
	}
	s := setOf(members)
	s.compl = complement
	return s, nil
}

// expandPair expands the operands of a translation, returning the arrays of
// string1 and string2, and the characters string1 holds before a
// complement. Case conversions replace the classes that form them, and
// string2 is padded with its last character.
func expandPair(elems1, elems2 []charset.Elem, conversions map[int]bool, complement bool) ([]charset.Member, []charset.Member, []charset.Member, error) {
	var members1, plain []charset.Member
	for i, e := range elems1 {
		var expanded []charset.Member
		if lower, ok := conversions[i]; ok {
			from, _ := charset.CasePairs(lower)
			expanded = members(from)
		} else {
			var err error
			if expanded, err = e.Expand(-1); err != nil {
				return nil, nil, nil, err
			}
		}
		// The characters of the case conversions belong to string1 as
		// well, so a complement must leave them out.
		plain = append(plain, expanded...)
		members1 = append(members1, expanded...)
	}
	if complement {
		members1 = complementOf(plain)
	}
	var members2 []charset.Member
	for i, e := range elems2 {
		var expanded []charset.Member
		if lower, ok := conversions[i]; ok {
			_, to := charset.CasePairs(lower)
			expanded = members(to)
		} else {
			var err error
			if expanded, err = e.Expand(len(members1) - len(members2)); err != nil {
				return nil, nil, nil, err
			}
		}
		members2 = append(members2, expanded...)
	}
	if len(members2) == 0 && len(members1) > 0 {
		return nil, nil, nil, errors.New("when translating, string2 must be non-empty")
	}
	if last := len(members2) - 1; last >= 0 {
		for len(members2) < len(members1) {
			members2 = append(members2, members2[last])
		}
	}
	return members1, members2, plain, nil
}

// members builds the characters of the given runes.
func members(runes []rune) []charset.Member {
	m := make([]charset.Member, len(runes))
	for i, r := range runes {
		m[i] = charset.Member{Char: r}
	}
	return m
}

// complementOf returns the byte values which the given characters don't
// hold, in ascending order.
func complementOf(members []charset.Member) []charset.Member {
	inSet := setOf(members)
	var out []charset.Member
	for v := range rune(0x100) {
		if !inSet.has(v) {
			out = append(out, charset.Member{Char: v})
		}
	}
	return out
}

// mapOf pairs the characters of the two arrays, mapping each one of members1
// to the one at the same position of members2.
func mapOf(members1, members2 []charset.Member) map[rune]charset.Member {
	replace := make(map[rune]charset.Member, len(members1))
	for i, m := range members1 {
		if i < len(members2) {
			replace[m.Char] = members2[i]
		}
	}
	return replace
}

// caseConversions returns the element indexes where a class of string1 and
// the class of string2 at the same position form a case conversion, keyed by
// whether string1 holds the lowercase characters. The classes of string2
// are only valid there, as are its equivalence classes with -d and -s.
func caseConversions(elems1, elems2 []charset.Elem) (map[int]bool, error) {
	var conversions map[int]bool
	for i, e := range elems2 {
		switch e.Kind {
		case charset.Equiv:
			return nil, errors.New("equivalence classes in string2 are only valid with -d and -s")
		case charset.Class:
			counter, ok := caseComplement[e.Class]
			if !ok || i >= len(elems1) || elems1[i].Kind != charset.Class || elems1[i].Class != counter {
				return nil, errors.New("string2 classes must be lower or upper, matching the one in string1")
			}
			if conversions == nil {
				conversions = map[int]bool{}
			}
			conversions[i] = e.Class == "upper"
		}
	}
	return conversions, nil
}

var caseComplement = map[string]string{
	"lower": "upper",
	"upper": "lower",
}

var _ coreutils.Program = (*Prog)(nil)
