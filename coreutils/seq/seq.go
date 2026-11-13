// Package seq implements the GNU seq utility.
//
// It prints the numbers from first to last by increment to the standard
// output, one per line by default. The operands may be integers or floating
// point numbers, and inf is supported.
package seq

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/stdopts"
)

// Prog is the seq core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run prints the sequence of numbers described by the operands, each one
// followed by the separator, and always terminates the output with a newline.
// An empty sequence writes nothing at all.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "seq", usage, optSet, args); done {
		return err
	}
	set, err := parseFlags(args)
	if err != nil {
		return p.usageErrorf("%v", err)
	}
	return p.run(ctx, set)
}

// run scans the operands and writes the sequence they describe.
func (p *Prog) run(ctx context.Context, set settings) error {
	seq, err := p.scanOperands(set)
	if err != nil {
		return err
	}
	f, err := newFormat(seq, set)
	if err != nil {
		stdopts.Errorf(p.Runtime, "seq", "%v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return p.print(ctx, seq, set, f)
}

// number is one of the operands of seq: its value, and the precision of its
// fixed point decimal representation.
type number struct {
	value float64
	prec  int
	fixed bool
}

// sequence holds the first, increment and last numbers.
type sequence struct {
	first, inc, last number
}

// scanOperands validates the operand count and parses the numbers. The
// operands are first, increment and last, in that order, with first and
// increment defaulting to 1.
func (p *Prog) scanOperands(set settings) (sequence, error) {
	ops := set.operands
	if len(ops) == 0 {
		return sequence{}, p.usageErrorf("missing operand")
	}
	if len(ops) > 3 {
		return sequence{}, p.usageErrorf("extra operand '%s'", ops[3])
	}
	if set.formatGiven && set.equalWidth {
		return sequence{}, p.usageErrorf("format string may not be specified when printing equal width strings")
	}

	seq := sequence{first: fixedOne, inc: fixedOne}
	nums := make([]*number, 0, 3)
	switch len(ops) {
	case 1:
		nums = append(nums, &seq.last)
	case 2:
		nums = append(nums, &seq.first, &seq.last)
	case 3:
		nums = append(nums, &seq.first, &seq.inc, &seq.last)
	}
	for i, num := range nums {
		n, err := scanArg(ops[i])
		if err != nil {
			return sequence{}, p.usageErrorf("%v", err)
		}
		*num = n
	}
	if seq.inc.value == 0 {
		return sequence{}, p.usageErrorf("invalid Zero increment value: '%s'", ops[1])
	}
	return seq, nil
}

// fixedOne is the number the first and increment operands default to.
var fixedOne = number{value: 1, fixed: true}

// scanArg parses a single operand, rejecting NaN and the underscores Go's
// parser accepts but C's doesn't.
func scanArg(arg string) (number, error) {
	if strings.ContainsRune(arg, '_') {
		return number{}, fmt.Errorf("invalid floating point argument: '%s'", arg)
	}
	value, err := strconv.ParseFloat(arg, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return number{}, fmt.Errorf("invalid floating point argument: '%s'", arg)
	}
	if math.IsNaN(value) {
		return number{}, fmt.Errorf("invalid 'not-a-number' argument: '%s'", arg)
	}
	prec, fixed := decimalPrecision(arg)
	return number{value: value, prec: prec, fixed: fixed}, nil
}

// decimalPrecision reports whether arg is a plain fixed point decimal number,
// and how many digits follow its decimal point.
func decimalPrecision(arg string) (int, bool) {
	s := strings.TrimLeft(arg, "+-")
	dot, digits := -1, 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			digits++
		case c == '.' && dot < 0:
			dot = i
		default:
			return 0, false
		}
	}
	if digits == 0 {
		return 0, false
	}
	if dot < 0 {
		return 0, true
	}
	return len(s) - dot - 1, true
}

// format renders the numbers of a sequence: the -f format when given, or the
// default one derived from the operands.
type format struct {
	// before, conv and after hold a -f format split around its single
	// conversion, with conv in its Go form. conv is empty for the default
	// format.
	before, conv, after string

	// hex reports whether conv renders a hexadecimal float, whose exponent
	// Go prints with two digits where C prints one.
	hex bool

	// prec is the precision of the default %.pf format, or -1 for %g.
	prec int
}

// newFormat builds the format of a sequence. The default format is %.pf with
// the precision the first and increment operands need when all of the
// operands use a fixed point decimal representation, and %g otherwise.
func newFormat(seq sequence, set settings) (format, error) {
	if set.formatGiven {
		before, conv, after, hex, err := translateFormat(set.format)
		if err != nil {
			return format{}, err
		}
		return format{before: before, conv: conv, after: after, hex: hex}, nil
	}
	if seq.first.fixed && seq.inc.fixed && seq.last.fixed {
		return format{prec: max(seq.first.prec, seq.inc.prec)}, nil
	}
	return format{prec: -1}, nil
}

// render writes value as a single number, the way C's printf renders it under
// the format.
func (f format) render(value float64) string {
	if f.conv != "" {
		s := fmt.Sprintf(f.conv, value)
		if f.hex {
			s = trimHexExp(s)
		}
		return f.before + s + f.after
	}
	if math.IsInf(value, 0) {
		if value < 0 {
			return "-inf"
		}
		return "inf"
	}
	if f.prec < 0 {
		return strconv.FormatFloat(value, 'g', 6, 64)
	}
	return strconv.FormatFloat(value, 'f', f.prec, 64)
}

// width returns the width -w pads the numbers to: the length of the longest
// of the first and last numbers, as rendered.
func (f format) width(seq sequence) int {
	w := len(f.render(seq.first.value))
	if l := len(f.render(seq.last.value)); l > w {
		return l
	}
	return w
}

// pad zero-pads s to the given width, with the zeros after a leading '-'.
func pad(s string, width int) string {
	if n := width - len(s); n > 0 {
		if strings.HasPrefix(s, "-") {
			return "-" + strings.Repeat("0", n) + s[1:]
		}
		return strings.Repeat("0", n) + s
	}
	return s
}

// trimHexExp rewrites the exponent of the hexadecimal float s the way C's
// printf prints it, with a single digit at the minimum: 0x1p+00 becomes
// 0x1p+0 and 0x1p-04 becomes 0x1p-4.
func trimHexExp(s string) string {
	i := strings.IndexAny(s, "pP")
	if i < 0 {
		return s
	}
	mant, exp := s[:i+1], s[i+1:]
	sign := ""
	if len(exp) > 0 && (exp[0] == '+' || exp[0] == '-') {
		sign, exp = exp[:1], exp[1:]
	}
	exp = strings.TrimLeft(exp, "0")
	if exp == "" {
		exp = "0"
	}
	return mant + sign + exp
}

// print writes the numbers of the sequence, separated by sep and followed by
// a newline. An empty sequence writes nothing at all, and printing stops with
// the context error when ctx is done, which the infinite sequences need.
func (p *Prog) print(ctx context.Context, seq sequence, set settings, f format) error {
	if seq.empty() {
		return nil
	}
	w := 0
	if set.equalWidth {
		w = f.width(seq)
	}
	out := bufio.NewWriter(p.Stdout)

	x := seq.first.value
	for i := 1; ; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if i > 1 {
			if _, err := out.WriteString(set.separator); err != nil {
				return p.writeError(err)
			}
		}
		s := f.render(x)
		if set.equalWidth {
			if _, err := out.WriteString(pad(s, w)); err != nil {
				return p.writeError(err)
			}
		} else if _, err := out.WriteString(s); err != nil {
			return p.writeError(err)
		}

		next := seq.first.value + float64(i)*seq.inc.value
		if seq.past(next) {
			// The last value may still belong to the sequence once rounded
			// to the precision of the format, like 0.3 in "seq 0 0.1 0.3".
			if extra := extraNumber(f, next, s, seq.last.value); extra != "" {
				if set.equalWidth {
					extra = pad(extra, w)
				}
				if _, err := out.WriteString(set.separator); err != nil {
					return p.writeError(err)
				}
				if _, err := out.WriteString(extra); err != nil {
					return p.writeError(err)
				}
			}
			break
		}
		x = next
	}
	if _, err := out.WriteString("\n"); err != nil {
		return p.writeError(err)
	}
	if err := out.Flush(); err != nil {
		return p.writeError(err)
	}
	return nil
}

// empty reports whether the sequence holds no numbers at all.
func (s sequence) empty() bool {
	return s.past(s.first.value)
}

// past reports whether value is beyond the last number of the sequence.
func (s sequence) past(value float64) bool {
	if s.inc.value < 0 {
		return value < s.last.value
	}
	return value > s.last.value
}

// extraNumber returns the rendering of the first value beyond the sequence
// when it rounds back to the last number and differs from prev, making it the
// closing number of the sequence.
func extraNumber(f format, value float64, prev string, last float64) string {
	s := f.render(value)
	parsed, err := strconv.ParseFloat(s, 64)
	if err != nil || parsed != last || s == prev {
		return ""
	}
	return s
}

// write writes s to the standard output, with a write failure exiting with
// status 1.
func (p *Prog) write(s string) error {
	if _, err := io.WriteString(p.Stdout, s); err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// writeError reports a failure writing to the standard output.
func (p *Prog) writeError(err error) error {
	stdopts.Errorf(p.Runtime, "seq", "write error: %v", err)
	return &coreutils.ExitError{Code: 1, Err: err}
}

// usageErrorf writes a diagnostic message to the standard error, followed by
// a hint to the help, and returns the exit status it asks for.
func (p *Prog) usageErrorf(f string, args ...any) *coreutils.ExitError {
	stdopts.Errorf(p.Runtime, "seq", f, args...)
	stdopts.TryHelp(p.Runtime, "seq")
	return &coreutils.ExitError{Code: 1}
}

var longOptions = map[string]rune{
	"format":      'f',
	"separator":   's',
	"equal-width": 'w',
}

// optSet describes the options of seq.
var optSet = getopt.Set{Valued: "fs", Long: longOptions}

// settings holds what a run of seq selects.
type settings struct {
	format      string
	separator   string
	equalWidth  bool
	operands    []string
	formatGiven bool
}

// parseFlags parses the options and operands. The -f and -s options take an
// option-argument, either attached or in the next argument, and the last one
// given wins. Option parsing stops at the first operand, at "--", and at the
// first argument that looks like a negative number, which conflicts with the
// options.
func parseFlags(args []string) (settings, error) {
	set := settings{separator: "\n"}
	opts, rest := splitOperands(args)
	operands, err := optSet.Parse(opts, func(opt rune, value string) error {
		switch opt {
		case 'f':
			set.format, set.formatGiven = value, true
		case 's':
			set.separator = value
		case 'w':
			set.equalWidth = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return settings{}, err
	}
	set.operands = append(operands, rest...)
	return set, nil
}

// splitOperands cuts args at the first argument that looks like a negative
// number, since such a number conflicts with the options. The arguments taken
// as the value of an option are skipped, as they may look like one.
func splitOperands(args []string) (opts, operands []string) {
	for i := 0; i < len(args); i++ {
		if takesNextValue(args[i]) {
			i++
			continue
		}
		if isNegativeNumber(args[i]) {
			return args[:i], args[i:]
		}
	}
	return args, nil
}

// isNegativeNumber reports whether arg starts like a negative number, as
// "-9e5" and "-.5" do, but "-inf" and "-w" don't.
func isNegativeNumber(arg string) bool {
	return len(arg) > 1 && arg[0] == '-' && (isDigit(arg[1]) || arg[1] == '.')
}

// takesNextValue reports whether arg is an option which takes its value from
// the next argument, either alone or at the end of a group.
func takesNextValue(arg string) bool {
	if arg == "-f" || arg == "-s" || arg == "--format" || arg == "--separator" {
		return true
	}
	last := len(arg) - 1
	if len(arg) < 3 || arg[0] != '-' || arg[1] == '-' || (arg[last] != 'f' && arg[last] != 's') {
		return false
	}
	for i := 1; i < last; i++ {
		if arg[i] != 'w' {
			return false
		}
	}
	return true
}

// convs are the conversion letters a -f format accepts, and convMap maps them
// to the Go ones, since Go's fmt has no %a, %A or %F.
const convs = "aefgAEFG"

var convMap = map[byte]byte{
	'a': 'x', 'A': 'X', 'F': 'f',
}

// translateFormat validates a -f format, which must hold exactly one of the
// printf floating point conversions and any number of %% ones, and returns
// the parts of its equivalent for Go's fmt: the literal text around the
// conversion, the Go conversion itself, and whether the conversion renders a
// hexadecimal float.
func translateFormat(f string) (before, conv, after string, hex bool, err error) {
	var b, c, a strings.Builder
	out := &b
	directives := 0
	for i := 0; i < len(f); i++ {
		ch := f[i]
		if ch != '%' {
			out.WriteByte(ch)
			continue
		}
		if i+1 < len(f) && f[i+1] == '%' {
			out.WriteByte('%')
			i++
			continue
		}
		if directives == 1 {
			out = &a
		} else {
			out = &c
		}
		out.WriteByte('%')
		j := i + 1
		for ; j < len(f) && strings.IndexByte("-+#0 '", f[j]) >= 0; j++ {
			// Go's fmt doesn't take the ' flag, which C locales leave as a
			// no-op anyway.
			if f[j] != '\'' {
				out.WriteByte(f[j])
			}
		}
		for ; j < len(f) && isDigit(f[j]); j++ {
			out.WriteByte(f[j])
		}
		given := false
		if j < len(f) && f[j] == '.' {
			given = true
			out.WriteByte('.')
			for j++; j < len(f) && isDigit(f[j]); j++ {
				out.WriteByte(f[j])
			}
		}
		if j >= len(f) {
			return "", "", "", false, fmt.Errorf("format '%s' ends in %%", f)
		}
		letter := f[j]
		if strings.IndexByte(convs, letter) < 0 {
			return "", "", "", false, fmt.Errorf("format '%s' has unknown %%%c directive", f, letter)
		}
		directives++
		// Go's %g only matches C's with an explicit precision, which is
		// six significant digits by default.
		if (letter == 'g' || letter == 'G') && !given {
			out.WriteString(".6")
		}
		if letter == 'a' || letter == 'A' {
			hex = true
		}
		if mapped, ok := convMap[letter]; ok {
			letter = mapped
		}
		out.WriteByte(letter)
		out = &a
		i = j
	}
	switch {
	case directives == 0:
		return "", "", "", false, fmt.Errorf("format '%s' has no %% directive", f)
	case directives > 1:
		return "", "", "", false, fmt.Errorf("format '%s' has too many %% directives", f)
	}
	return b.String(), c.String(), a.String(), hex, nil
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// usage describes the options of seq.
const usage = `Usage: seq [OPTION]... LAST
  or:  seq [OPTION]... FIRST LAST
  or:  seq [OPTION]... FIRST INCREMENT LAST
Print numbers from FIRST to LAST, in steps of INCREMENT.

  -f, --format=FORMAT
         use printf style floating-point FORMAT
  -s, --separator=STRING
         use STRING to separate numbers (default: \n)
  -w, --equal-width
         equalize width by padding with leading zeros
` + stdopts.Tail

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
