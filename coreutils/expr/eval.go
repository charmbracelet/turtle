package expr

import (
	"math"
	"math/bits"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/turtle/coreutils/internal/bre"
)

// value is the result of an expression or one of its operands: a string,
// which may take the form of an integer, or an integer produced by an
// arithmetic operation.
type value struct {
	s     string
	n     int64
	isInt bool
}

func strv(s string) value { return value{s: s} }

func intv(n int64) value { return value{n: n, isInt: true} }

// text returns the string form of v, converting an integer to decimal.
func (v value) text() string {
	if v.isInt {
		return strconv.FormatInt(v.n, 10)
	}
	return v.s
}

// isNull reports whether v is the null string or the number zero.
func isNull(v value) bool {
	if v.isInt {
		return v.n == 0
	}
	if v.s == "" {
		return true
	}
	digits := strings.TrimPrefix(v.s, "-")
	return digits != "" && strings.Trim(digits, "0") == ""
}

// looksLikeInteger reports whether s consists of an optional minus
// followed by digits, the form of an integer operand per POSIX.
func looksLikeInteger(s string) bool {
	s = strings.TrimPrefix(s, "-")
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// evaluate evaluates the whole expression, which must consume every token.
func evaluate(args []string) (value, error) {
	e := &evaluator{args: args}
	v, err := e.or(true)
	if err != nil {
		return value{}, err
	}
	if e.pos < len(e.args) {
		return value{}, syntaxErrf("syntax error: unexpected argument %s", quote(e.args[e.pos]))
	}
	return v, nil
}

// evaluator walks the tokens of the expression, from the lowest precedence
// level of | down to the atoms.
type evaluator struct {
	args []string
	pos  int
}

// next consumes the next token when it matches tok exactly.
func (e *evaluator) next(tok string) bool {
	if e.pos < len(e.args) && e.args[e.pos] == tok {
		e.pos++
		return true
	}
	return false
}

// requireArg fails when no token is left where after expects an operand.
func (e *evaluator) requireArg(after string) error {
	if e.pos >= len(e.args) {
		return syntaxErrf("syntax error: missing argument after %s", quote(after))
	}
	return nil
}

// or evaluates the | connectives: the first operand when it is neither
// null nor zero, the second one otherwise, and zero when both are.
func (e *evaluator) or(eval bool) (value, error) {
	l, err := e.and(eval)
	if err != nil {
		return value{}, err
	}
	for e.next("|") {
		r, err := e.and(eval && isNull(l))
		if err != nil {
			return value{}, err
		}
		if isNull(l) {
			l = r
			if isNull(l) {
				l = intv(0)
			}
		}
	}
	return l, nil
}

// and evaluates the & connectives: the first operand when neither operand
// of a & is null or zero, and zero otherwise.
func (e *evaluator) and(eval bool) (value, error) {
	l, err := e.rel(eval)
	if err != nil {
		return value{}, err
	}
	for e.next("&") {
		r, err := e.rel(eval && !isNull(l))
		if err != nil {
			return value{}, err
		}
		if isNull(l) || isNull(r) {
			l = intv(0)
		}
	}
	return l, nil
}

// rel evaluates the comparison operators, all left-associative: numeric
// when both operands take the form of integers, bytewise on their string
// forms otherwise.
func (e *evaluator) rel(eval bool) (value, error) {
	l, err := e.add(eval)
	if err != nil {
		return value{}, err
	}
	for {
		var want func(cmp int) bool
		switch {
		case e.next("<"):
			want = func(cmp int) bool { return cmp < 0 }
		case e.next("<="):
			want = func(cmp int) bool { return cmp <= 0 }
		case e.next("=") || e.next("=="):
			want = func(cmp int) bool { return cmp == 0 }
		case e.next("!="):
			want = func(cmp int) bool { return cmp != 0 }
		case e.next(">="):
			want = func(cmp int) bool { return cmp >= 0 }
		case e.next(">"):
			want = func(cmp int) bool { return cmp > 0 }
		default:
			return l, nil
		}
		r, err := e.add(eval)
		if err != nil {
			return value{}, err
		}
		if eval {
			l = intv(b2i(want(compare(l, r))))
		}
	}
}

// compare compares two values as the relational operators do: numerically
// when both hold integer-looking strings, bytewise otherwise.
func compare(l, r value) int {
	a, b := l.text(), r.text()
	if looksLikeInteger(a) && looksLikeInteger(b) {
		return strintcmp(a, b)
	}
	return strings.Compare(a, b)
}

// strintcmp compares integer strings as the numbers they denote, without
// converting them, so numbers beyond the 64-bit range compare correctly.
// The strings must look like integers.
func strintcmp(a, b string) int {
	// The magnitude of an integer string, ignoring its sign and leading
	// zeros: longer is greater, then the first differing digit decides.
	mag := func(a, b string) int {
		a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
		switch {
		case a == "" && b == "":
			return 0
		case len(a) != len(b):
			if len(a) < len(b) {
				return -1
			}
			return 1
		default:
			return strings.Compare(a, b)
		}
	}
	an, bn := strings.HasPrefix(a, "-"), strings.HasPrefix(b, "-")
	a, b = strings.TrimPrefix(a, "-"), strings.TrimPrefix(b, "-")
	switch {
	case an && bn:
		return -mag(a, b)
	case an || bn:
		if mag(a, "") != 0 || mag(b, "") != 0 {
			if an {
				return -1
			}
			return 1
		}
		return 0
	default:
		return mag(a, b)
	}
}

// add evaluates the + and - operators.
func (e *evaluator) add(eval bool) (value, error) {
	l, err := e.mul(eval)
	if err != nil {
		return value{}, err
	}
	for {
		op := addInt
		switch {
		case e.next("+"):
		case e.next("-"):
			op = subInt
		default:
			return l, nil
		}
		r, err := e.mul(eval)
		if err != nil {
			return value{}, err
		}
		if eval {
			if l, err = arith(l, r, op); err != nil {
				return value{}, err
			}
		}
	}
}

// mul evaluates the *, / and % operators.
func (e *evaluator) mul(eval bool) (value, error) {
	l, err := e.match(eval)
	if err != nil {
		return value{}, err
	}
	for {
		var op func(a, b int64) (int64, error)
		switch {
		case e.next("*"):
			op = mulInt
		case e.next("/"):
			op = divInt
		case e.next("%"):
			op = modInt
		default:
			return l, nil
		}
		r, err := e.match(eval)
		if err != nil {
			return value{}, err
		}
		if eval {
			if l, err = arith(l, r, op); err != nil {
				return value{}, err
			}
		}
	}
}

// arith coerces both operands to integers and applies op to them.
func arith(l, r value, op func(a, b int64) (int64, error)) (value, error) {
	a, err := toInt(l)
	if err != nil {
		return value{}, err
	}
	b, err := toInt(r)
	if err != nil {
		return value{}, err
	}
	n, err := op(a, b)
	if err != nil {
		return value{}, err
	}
	return intv(n), nil
}

// match evaluates the : operators, all left-associative.
func (e *evaluator) match(eval bool) (value, error) {
	l, err := e.keywords(eval)
	if err != nil {
		return value{}, err
	}
	for e.next(":") {
		r, err := e.keywords(eval)
		if err != nil {
			return value{}, err
		}
		if eval {
			if l, err = docolon(l, r); err != nil {
				return value{}, err
			}
		}
	}
	return l, nil
}

// keywords evaluates the string keywords and the GNU + token, which
// quotes the next one as a plain string.
func (e *evaluator) keywords(eval bool) (value, error) {
	if e.next("+") {
		if err := e.requireArg("+"); err != nil {
			return value{}, err
		}
		v := strv(e.args[e.pos])
		e.pos++
		return v, nil
	}
	if e.next("length") {
		r, err := e.keywords(eval)
		if err != nil {
			return value{}, err
		}
		return intv(int64(utf8.RuneCountInString(r.text()))), nil
	}
	if e.next("match") {
		l, err := e.keywords(eval)
		if err != nil {
			return value{}, err
		}
		r, err := e.keywords(eval)
		if err != nil {
			return value{}, err
		}
		if eval {
			return docolon(l, r)
		}
		return l, nil
	}
	if e.next("index") {
		l, err := e.keywords(eval)
		if err != nil {
			return value{}, err
		}
		r, err := e.keywords(eval)
		if err != nil {
			return value{}, err
		}
		return intv(strindex(l.text(), r.text())), nil
	}
	if e.next("substr") {
		l, err := e.keywords(eval)
		if err != nil {
			return value{}, err
		}
		i1, err := e.keywords(eval)
		if err != nil {
			return value{}, err
		}
		i2, err := e.keywords(eval)
		if err != nil {
			return value{}, err
		}
		pos, ok1 := toSize(i1)
		length, ok2 := toSize(i2)
		if !ok1 || !ok2 {
			return strv(""), nil
		}
		return strv(substr(l.text(), pos, length)), nil
	}
	return e.atom(eval)
}

// atom evaluates a parenthesized expression or a bare token, which is
// always taken as a string.
func (e *evaluator) atom(eval bool) (value, error) {
	if e.pos >= len(e.args) {
		if e.pos == 0 {
			return value{}, syntaxErrf("syntax error")
		}
		return value{}, syntaxErrf("syntax error: missing argument after %s", quote(e.args[e.pos-1]))
	}
	if e.next("(") {
		v, err := e.or(eval)
		if err != nil {
			return value{}, err
		}
		if e.pos >= len(e.args) {
			return value{}, syntaxErrf("syntax error: expecting ')' after %s", quote(e.args[e.pos-1]))
		}
		if !e.next(")") {
			return value{}, syntaxErrf("syntax error: expecting ')' instead of %s", quote(e.args[e.pos]))
		}
		return v, nil
	}
	if e.next(")") {
		return value{}, syntaxErrf("syntax error: unexpected ')'")
	}
	v := strv(e.args[e.pos])
	e.pos++
	return v, nil
}

// toInt coerces v to an integer, as GNU does: strings which take the form
// of an integer are converted, anything else is a non-integer argument.
// Numbers beyond the 64-bit range fail the evaluation.
func toInt(v value) (int64, error) {
	if v.isInt {
		return v.n, nil
	}
	s := v.s
	if !looksLikeInteger(s) {
		return 0, syntaxErrf("non-integer argument")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, rangeErrf(s)
	}
	return n, nil
}

// toSize coerces v to a size for substr: false when it doesn't take the
// form of an integer, saturating out of range values, whose extreme
// results GNU's wider arithmetic would produce anyway.
func toSize(v value) (int64, bool) {
	s := v.text()
	if !looksLikeInteger(s) {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		n = math.MaxInt64
		if strings.HasPrefix(s, "-") {
			n = math.MinInt64
		}
	}
	return n, true
}

// docolon matches string against pattern, a basic regular expression
// anchored at the beginning of the string. It returns the part matched by
// the first \( \) group when the pattern uses one, and the number of
// characters matched otherwise.
func docolon(s, pattern value) (value, error) {
	str, pat := s.text(), pattern.text()
	translated, err := bre.Translate(pat)
	if err != nil {
		return value{}, err
	}
	// The pattern is anchored at the beginning of the string, as if a ^
	// were prepended, and doesn't have to match it whole.
	re, err := regexp.Compile("^(?:" + translated + ")")
	if err != nil {
		return value{}, syntaxErrf("invalid regular expression: %v", err)
	}
	loc := re.FindStringSubmatchIndex(str)
	switch {
	case loc == nil || re.NumSubexp() > 0 && loc[2] < 0:
		// The match failed, or the group didn't contribute to it.
		if re.NumSubexp() > 0 {
			return strv(""), nil
		}
		return intv(0), nil
	case re.NumSubexp() > 0:
		return strv(str[loc[2]:loc[3]]), nil
	default:
		return intv(int64(utf8.RuneCountInString(str[:loc[1]]))), nil
	}
}

// strindex returns the 1-based position of the first character of charset
// found in s, or 0 when none is.
func strindex(s, charset string) int64 {
	if charset == "" {
		return 0
	}
	var idx int64
	for _, c := range s {
		idx++
		if strings.ContainsRune(charset, c) {
			return idx
		}
	}
	return 0
}

// substr returns at most length characters of s starting at the 1-based
// position pos. A non-positive position or length, or one past the end of
// the string, yields the null string.
func substr(s string, pos, length int64) string {
	if pos <= 0 || length <= 0 {
		return ""
	}
	runes := []rune(s)
	if pos > int64(len(runes)) {
		return ""
	}
	runes = runes[pos-1:]
	if length < int64(len(runes)) {
		runes = runes[:length]
	}
	return string(runes)
}

// addInt sums a and b, failing on an out of range result.
func addInt(a, b int64) (int64, error) {
	s := a + b
	if (b > 0 && s < a) || (b < 0 && s > a) {
		return 0, rangeErrf("")
	}
	return s, nil
}

// subInt subtracts b from a, failing on an out of range result.
func subInt(a, b int64) (int64, error) {
	d := a - b
	if (b > 0 && d > a) || (b < 0 && d < a) {
		return 0, rangeErrf("")
	}
	return d, nil
}

// mulInt multiplies a by b, failing on an out of range result. The product
// is computed from the magnitudes, since the unsigned product of the raw
// two's-complement words doesn't give the signed one.
func mulInt(a, b int64) (int64, error) {
	hi, lo := bits.Mul64(magnitude(a), magnitude(b))
	sneg := (a < 0) != (b < 0)
	switch {
	case hi == 0 && lo <= math.MaxInt64:
	case hi == 0 && lo == math.MaxInt64+1 && sneg:
		// The magnitude 2^63 only fits as a negative number.
		return math.MinInt64, nil
	default:
		return 0, rangeErrf("")
	}
	if sneg {
		return -int64(lo), nil
	}
	return int64(lo), nil
}

// magnitude gives the magnitude of a as an unsigned number, which holds it
// even for the lowest integer.
func magnitude(a int64) uint64 {
	if a < 0 {
		return uint64(-a)
	}
	return uint64(a)
}

// divInt divides a by b, truncating toward zero.
func divInt(a, b int64) (int64, error) {
	if b == 0 {
		return 0, syntaxErrf("division by zero")
	}
	if a == math.MinInt64 && b == -1 {
		return 0, rangeErrf("")
	}
	return a / b, nil
}

// modInt gives the remainder of dividing a by b, keeping the sign of a.
func modInt(a, b int64) (int64, error) {
	if b == 0 {
		return 0, syntaxErrf("division by zero")
	}
	return a % b, nil
}

// b2i gives 1 for a true relation and 0 for a false one.
func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
