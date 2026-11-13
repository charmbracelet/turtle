// Package printf implements the POSIX printf utility.
package printf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/numparse"
	"charm.land/turtle/coreutils/internal/printfmt"
)

// Prog is the printf core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the argument operands to standard output, under control of the
// format operand, which is reused as often as necessary to satisfy them.
func (p *Prog) Run(_ context.Context, args ...string) error {
	if len(args) == 0 {
		p.diag("missing format operand")
		return &coreutils.ExitError{Code: 1}
	}
	pr := &printer{p: p, args: args[1:]}
	pr.print(parseFormat(args[0]))
	if _, err := io.WriteString(p.Stdout, pr.out.String()); err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	switch {
	case pr.stopped:
		return nil
	case pr.failed:
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// diag writes a diagnostic message to standard error.
func (p *Prog) diag(msg string) {
	fmt.Fprintf(p.Stderr, "printf: %s\n", msg)
}

var (
	// formatEscapes takes the escapes of the format operand, and bEscapes
	// the ones of a b conversion argument.
	formatEscapes = printfmt.Escapes{Octal: printfmt.OctalPlain, Extended: true}
	bEscapes      = printfmt.Escapes{Octal: printfmt.OctalZero, Extended: true}
)

// printer renders a parsed format against the argument operands, reusing the
// format as often as necessary to satisfy them.
type printer struct {
	p    *Prog
	out  strings.Builder
	args []string

	numbered bool  // the format uses "%n$" conversions
	base     int64 // arguments consumed by the previous uses of the format
	next     int64 // next argument of an unnumbered conversion
	highest  int64 // highest numbered argument of the current use
	consumed bool  // whether the current use consumed any argument

	failed  bool // an error was diagnosed
	stopped bool // a '\c' escape stopped all further output
	halted  bool // an invalid directive stopped processing
}

func (pr *printer) print(segs []segment) {
	for _, seg := range segs {
		if seg.conv != nil && seg.conv.argnum > 0 {
			pr.numbered = true
			break
		}
	}
	for {
		pr.consumed = false
		for _, seg := range segs {
			if seg.conv == nil {
				stopped, err := formatEscapes.Expand(&pr.out, seg.text, -1)
				if err != nil {
					pr.escapeError(err)
					return
				}
				if stopped {
					pr.stopped = true
					return
				}
				continue
			}
			pr.writeConv(seg.conv)
			if pr.stopped || pr.halted {
				return
			}
		}
		if !pr.consumed {
			return
		}
		if pr.numbered {
			pr.base += pr.highest
			pr.highest = 0
			if pr.base >= int64(len(pr.args)) {
				return
			}
		} else if pr.next >= int64(len(pr.args)) {
			return
		}
	}
}

// writeConv writes a single conversion specification.
func (pr *printer) writeConv(c *conv) {
	if !validVerb(c.verb) {
		pr.p.diag(strconv.Quote(c.raw) + " invalid directive")
		pr.failed = true
		pr.halted = true
		return
	}
	i := pr.argIndex(c)
	pr.consumed = true
	arg, ok := "", i < int64(len(pr.args))
	if ok {
		arg = pr.args[i]
	} else {
		// A missing argument is a null string for the b, c and s
		// conversions and a zero for the numeric ones.
		if c.argnum > 0 {
			pr.p.diag(strconv.Quote(strconv.FormatInt(c.argnum, 10)) + " missing argument")
			pr.failed = true
		}
		if !isStringVerb(c.verb) {
			arg = "0"
		}
	}
	switch c.verb {
	case 's':
		pr.writePadded(truncate(arg, c.prec), c)
	case 'b':
		var b strings.Builder
		stopped, err := bEscapes.Expand(&b, arg, int(c.prec))
		if err != nil || stopped {
			// The bytes before the '\c' escape or the invalid one
			// were already converted.
			pr.out.WriteString(b.String())
			if err != nil {
				pr.escapeError(err)
				return
			}
			pr.stopped = true
			return
		}
		pr.writePadded(b.String(), c)
	case 'c':
		if arg != "" {
			pr.writePadded(arg[:1], c)
		}
	case 'f', 'e', 'E', 'g', 'G':
		pr.writeFloat(c, arg)
	default:
		pr.writeInt(c, arg)
	}
}

// argIndex returns the index of the argument operand to evaluate, updating
// the reuse state of the format.
func (pr *printer) argIndex(c *conv) int64 {
	switch {
	case c.argnum > 0:
		if c.argnum > pr.highest {
			pr.highest = c.argnum
		}
		return pr.base + c.argnum - 1
	case pr.numbered:
		i := pr.base + pr.highest
		pr.highest++
		return i
	default:
		i := pr.next
		pr.next++
		return i
	}
}

func (pr *printer) writePadded(s string, c *conv) {
	if pad := c.width - int64(len(s)); pad > 0 {
		spaces := strings.Repeat(" ", int(pad))
		if strings.IndexByte(c.flags, '-') >= 0 {
			s += spaces
		} else {
			s = spaces + s
		}
	}
	pr.out.WriteString(s)
}

func (pr *printer) writeInt(c *conv, arg string) {
	if c.verb == 'd' || c.verb == 'i' {
		v, kind := parseIntArg(arg)
		pr.report(arg, kind)
		fmt.Fprintf(&pr.out, directive(c, 'd', true), v)
		return
	}
	v, kind := parseUintArg(arg)
	pr.report(arg, kind)
	if v == 0 && (c.verb == 'x' || c.verb == 'X') {
		// The alternate form prefixes only nonzero results.
		c = withoutAltFlag(c)
	}
	fmt.Fprintf(&pr.out, directive(c, verbForUint(c.verb), true), v)
}

func (pr *printer) writeFloat(c *conv, arg string) {
	v, kind := parseFloatArg(arg)
	pr.report(arg, kind)
	if c.prec < 0 && (c.verb == 'g' || c.verb == 'G') {
		// The C default for the g conversions is six significant digits.
		cc := *c
		cc.prec = 6
		c = &cc
	}
	fmt.Fprintf(&pr.out, directive(c, c.verb, false), v)
}

// withoutAltFlag returns c without the '#' flag, for a zero value.
func withoutAltFlag(c *conv) *conv {
	if !strings.Contains(c.flags, "#") {
		return c
	}
	cc := *c
	cc.flags = strings.Replace(cc.flags, "#", "", 1)
	return &cc
}

// report diagnoses an argument that could not be completely converted,
// keeping the value accumulated at the time the error was detected.
func (pr *printer) report(arg string, kind convKind) {
	if kind == convOK {
		return
	}
	pr.failed = true
	var msg string
	switch kind {
	case convInvalid:
		msg = " expected numeric value"
	case convIncomplete:
		msg = " not completely converted"
	case convOverflow:
		msg = " arithmetic overflow"
	}
	pr.p.diag(strconv.Quote(arg) + msg)
}

// directive builds the Go format directive equivalent to a numeric
// conversion, dropping flags Go would reject.
func directive(c *conv, verb byte, intConv bool) string {
	var b strings.Builder
	b.WriteByte('%')
	for i := 0; i < len(c.flags); i++ {
		f := c.flags[i]
		switch {
		case f == '#' && verb == 'd', intConv && f == '0' && c.prec >= 0:
		default:
			b.WriteByte(f)
		}
	}
	if c.width > 0 {
		b.WriteString(strconv.FormatInt(c.width, 10))
	}
	if c.prec >= 0 {
		b.WriteByte('.')
		b.WriteString(strconv.FormatInt(c.prec, 10))
	}
	b.WriteByte(verb)
	return b.String()
}

// segment is a piece of the format operand: either literal text, possibly
// holding backslash escapes, or a conversion specification.
type segment struct {
	text string
	conv *conv
}

// conv is a conversion specification of the format operand.
type conv struct {
	verb   byte
	raw    string
	argnum int64
	flags  string
	width  int64
	prec   int64
}

// parseFormat splits format into literal text and conversion
// specifications.
func parseFormat(format string) []segment {
	var segs []segment
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			segs = append(segs, segment{text: lit.String()})
			lit.Reset()
		}
	}
	for i := 0; i < len(format); {
		switch {
		case format[i] != '%':
			lit.WriteByte(format[i])
			i++
		case i+1 < len(format) && format[i+1] == '%':
			lit.WriteByte('%')
			i += 2
		default:
			c, n := parseConv(format[i:])
			flush()
			segs = append(segs, segment{conv: &c})
			i += n
		}
	}
	flush()
	return segs
}

// parseConv parses the conversion specification at the start of s, which
// begins with '%'. It always consumes at least the '%'.
func parseConv(s string) (conv, int) {
	spec, n := printfmt.ScanSpec(s, true)
	c := conv{
		argnum: spec.ArgNum,
		flags:  spec.Flags,
		width:  spec.Width,
		prec:   spec.Prec,
	}
	if n < len(s) {
		c.verb = s[n]
		n++
	}
	c.raw = s[:n]
	return c, n
}

func validVerb(v byte) bool {
	switch v {
	case 'b', 'c', 's', 'd', 'i', 'o', 'u', 'x', 'X', 'f', 'e', 'E', 'g', 'G':
		return true
	}
	return false
}

func isStringVerb(v byte) bool {
	return v == 'b' || v == 'c' || v == 's'
}

func verbForUint(v byte) byte {
	if v == 'u' {
		return 'd'
	}
	return v
}

func truncate(s string, n int64) string {
	if n >= 0 && int64(len(s)) > n {
		return s[:n]
	}
	return s
}

// escapeError diagnoses an invalid escape sequence, stopping all further
// processing like GNU printf does on a bad escape.
func (pr *printer) escapeError(err error) {
	pr.p.diag(err.Error())
	pr.failed = true
	pr.halted = true
}

// convKind classifies the result of converting an argument operand to a
// numeric value.
type convKind int

const (
	convOK convKind = iota
	convInvalid
	convIncomplete
	convOverflow
)

// quotedArg evaluates a leading single- or double-quoted argument, whose
// value is the numeric value of the byte following the quote.
func quotedArg(arg string) (int64, convKind, bool) {
	if arg == "" || arg[0] != '\'' && arg[0] != '"' {
		return 0, convOK, false
	}
	if len(arg) < 2 {
		return 0, convInvalid, true
	}
	if len(arg) > 2 {
		return int64(arg[1]), convIncomplete, true
	}
	return int64(arg[1]), convOK, true
}

// parseIntArg evaluates arg as an unsuffixed C integer constant, with a
// leading sign or quote allowed.
func parseIntArg(arg string) (int64, convKind) {
	if v, kind, ok := quotedArg(arg); ok {
		return v, kind
	}
	s, base, complete := scanNumber(arg)
	if s == "" {
		return 0, convInvalid
	}
	v, err := strconv.ParseInt(s, base, 64)
	if err != nil {
		if strings.HasPrefix(s, "-") {
			return math.MinInt64, convOverflow
		}
		return math.MaxInt64, convOverflow
	}
	if !complete {
		return v, convIncomplete
	}
	return v, convOK
}

// parseUintArg evaluates arg like parseIntArg, with the strtoul semantics
// the o, u, x and X conversions require.
func parseUintArg(arg string) (uint64, convKind) {
	if v, kind, ok := quotedArg(arg); ok {
		return uint64(v), kind
	}
	s, base, complete := scanNumber(arg)
	if s == "" {
		return 0, convInvalid
	}
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	v, err := strconv.ParseUint(s, base, 64)
	if err != nil {
		return math.MaxUint64, convOverflow
	}
	if neg {
		v = -v
	}
	if !complete {
		return v, convIncomplete
	}
	return v, convOK
}

// parseFloatArg evaluates arg as if by strtod, using the longest prefix
// that converts.
func parseFloatArg(arg string) (float64, convKind) {
	for n := len(arg); n > 0; n-- {
		sub := arg[:n]
		// strtod accepts hexadecimal values without the 'p' exponent
		// that Go requires, so an exponent of zero is appended.
		if s, ok := withHexExponent(sub); ok {
			sub = s
		}
		v, err := strconv.ParseFloat(sub, 64)
		switch {
		case err == nil:
			if n == len(arg) {
				return v, convOK
			}
			return v, convIncomplete
		case errors.Is(err, strconv.ErrRange):
			return v, convOverflow
		}
	}
	return 0, convInvalid
}

// withHexExponent appends a binary exponent of zero to a hexadecimal value
// that has none, reporting whether it did.
func withHexExponent(sub string) (string, bool) {
	s := sub
	if s != "" && (s[0] == '+' || s[0] == '-') {
		s = s[1:]
	}
	if len(s) < 2 || s[0] != '0' || s[1] != 'x' && s[1] != 'X' {
		return "", false
	}
	if strings.ContainsAny(s, "pP") {
		return "", false
	}
	return sub + "p0", true
}

// scanNumber scans the longest prefix of s that converts like an
// unsuffixed C integer constant, with an optional leading sign. It returns
// it ready for strconv, along with its base and whether it covers the whole
// argument.
func scanNumber(s string) (num string, base int, complete bool) {
	i, sign := 0, ""
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		i, sign = 1, s[:1]
	}
	rest, n, base := s[i:], 0, 10
	switch {
	case strings.HasPrefix(rest, "0x"), strings.HasPrefix(rest, "0X"):
		if len(rest) > 2 && numparse.IsHexDigit(rest[2]) {
			n, base = 2, 16
			for n < len(rest) && numparse.IsHexDigit(rest[n]) {
				n++
			}
			return sign + rest[2:n], base, i+n == len(s)
		}
		// No hexadecimal digits: only the leading zero converts.
		n = 1
	case len(rest) > 0 && rest[0] == '0':
		n, base = 1, 8
		for n < len(rest) && numparse.IsOctalDigit(rest[n]) {
			n++
		}
	default:
		for n < len(rest) && numparse.IsDigit(rest[n]) {
			n++
		}
	}
	if n == 0 {
		return "", 0, false
	}
	return sign + rest[:n], base, i+n == len(s)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
