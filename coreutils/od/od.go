// Package od implements the od core utility, with its GNU extensions.
package od

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the od core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the input data in the requested representation.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	args = expandBare(args)
	if done, err := stdopts.Check(p.Runtime, "od", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "od")
		return &coreutils.ExitError{Code: 1}
	}
	if opts.stringMin >= 0 && len(opts.types) > 0 {
		p.errorf("no type may be specified when dumping strings")
		return &coreutils.ExitError{Code: 1}
	}
	files := traditionalOperands(operands, &opts)
	if opts.traditional && len(files) > 1 {
		p.errorf("extra operand '%s'", files[1])
		p.errorf("compatibility mode supports at most one file")
		stdopts.TryHelp(p.Runtime, "od")
		return &coreutils.ExitError{Code: 1}
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	var data []byte
	for _, name := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, err := input.Open(p.Runtime, ctx, input.New(name))
		if err != nil {
			p.errorf("%s: %v", name, patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 1}
		}
		part, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			p.errorf("%s: %v", name, patherr.Unwrap(readErr))
			return &coreutils.ExitError{Code: 1}
		}
		if closeErr != nil {
			p.errorf("%s: %v", name, patherr.Unwrap(closeErr))
			return &coreutils.ExitError{Code: 1}
		}
		data = append(data, part...)
	}
	if opts.skip > uint64(len(data)) {
		p.errorf("cannot skip past end of combined input")
		return &coreutils.ExitError{Code: 1}
	}
	start := int(opts.skip)
	data = data[start:]
	// The strings mode counts a string cut by -N as terminated, so record
	// whether the limit actually cuts the input short.
	atLimit := false
	if opts.count >= 0 {
		atLimit = int64(len(data)) >= opts.count
		if int64(len(data)) > opts.count {
			data = data[:opts.count]
		}
	}
	err = p.dump(ctx, data, uint64(start), opts, atLimit)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.errorf("write error: %v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// traditionalOperands applies the [+OFFSET[.][b]] and [+LABEL[.][b]]
// operands of the second and third forms to opts, and returns the files
// left. The options of the first form hide them, unless --traditional
// brings them back.
func traditionalOperands(operands []string, opts *options) []string {
	files := operands
	if opts.modern && !opts.traditional {
		return files
	}
	switch len(files) {
	case 1:
		// A lone operand is an offset only with --traditional or a
		// leading +, never with a bare digit, which stays a file.
		if opts.traditional || (files[0] != "" && files[0][0] == '+') {
			if offset, ok := parseOldOffset(files[0]); ok {
				opts.skip = offset
				files = nil
			}
		}
	case 2:
		last := files[1]
		if last != "" && (last[0] == '+' || isDigit(last[0]) || opts.traditional) {
			if offset, ok := parseOldOffset(last); ok {
				// With --traditional, an operand before the offset is
				// one itself, and the offset becomes the label.
				if first, ok := parseOldOffset(files[0]); opts.traditional && ok {
					opts.skip, opts.label, opts.labelSet = first, offset, true
					files = nil
				} else {
					opts.skip = offset
					files = files[:1]
				}
			}
		}
	case 3:
		if opts.traditional {
			offset, ok := parseOldOffset(files[1])
			label, ok2 := parseOldOffset(files[2])
			if ok && ok2 {
				opts.skip, opts.label, opts.labelSet = offset, label, true
				files = files[:1]
			}
		}
	}
	return files
}

func (p *Prog) dump(ctx context.Context, data []byte, base uint64, opts options, atLimit bool) error {
	if opts.stringMin >= 0 {
		return p.dumpStrings(ctx, data, base, opts, atLimit)
	}
	if len(data) == 0 {
		return p.writeAddress(base, '\n', opts)
	}
	specs, blockSize, ok := p.layout(opts)
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	var previous []byte
	starred := false
	for offset := 0; offset < len(data); offset += blockSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		block := data[offset:min(offset+blockSize, len(data))]
		nbytes := len(block)
		if nbytes < blockSize {
			padded := make([]byte, blockSize)
			copy(padded, block)
			block = padded
		}
		rows := make([]string, len(specs))
		for i, sp := range specs {
			rows[i] = renderType(block, nbytes, sp, opts.swap)
		}
		if !opts.all && nbytes == blockSize && previous != nil && bytes.Equal(previous, block) {
			if !starred {
				if _, err := fmt.Fprintln(p.Stdout, "*"); err != nil {
					return err
				}
				starred = true
			}
			continue
		}
		previous, starred = block, false
		lead := address(base+uint64(offset), 0, opts)
		indent := strings.Repeat(" ", len(lead))
		for i, row := range rows {
			prefix := indent
			if i == 0 {
				prefix = lead
			}
			if _, err := fmt.Fprintln(p.Stdout, prefix+row); err != nil {
				return err
			}
		}
	}
	return p.writeAddress(base+uint64(len(data)), '\n', opts)
}

// dumpStrings writes the NUL terminated strings of at least the number of
// printable characters that -S asks for. A string cut short by -N counts as
// terminated, one ended by the input does not.
func (p *Prog) dumpStrings(ctx context.Context, data []byte, base uint64, opts options, atLimit bool) error {
	for i := 0; i < len(data); {
		if err := ctx.Err(); err != nil {
			return err
		}
		start := i
		for i < len(data) && isPrintable(data[i]) {
			i++
		}
		terminated := i < len(data) && data[i] == 0
		if i == len(data) && atLimit {
			terminated = true
		}
		if !terminated || i-start < opts.stringMin {
			if i < len(data) {
				i++ // step over the byte which ended the string
			}
			continue
		}
		if err := p.writeAddress(base+uint64(start), ' ', opts); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(p.Stdout, escapeString(data[start:i])); err != nil {
			return err
		}
		i++
	}
	return nil
}

// layout returns the output types of opts with the width GNU aligns their
// fields to, and the number of input bytes each output line covers. It
// reports whether the widths fit at all.
func (p *Prog) layout(opts options) ([]spec, int, bool) {
	lcm := 1
	for _, typ := range opts.types {
		lcm = lcmInt(lcm, typ.size)
	}
	blockSize := max(lcm, 16)
	if opts.width > 0 {
		switch {
		case opts.width%lcm == 0:
			blockSize = opts.width
		default:
			p.errorf("warning: invalid width %d; using %d instead", opts.width, lcm)
			blockSize = lcm
		}
	}
	specs := make([]spec, len(opts.types))
	lineWidth := 0
	for i, typ := range opts.types {
		fields := blockSize / typ.size
		if fields-1 > 0 && fields-1 > math.MaxInt/(fields-1) {
			p.errorf("width %d is too large", opts.width)
			return nil, 0, false
		}
		specs[i] = spec{outputType: typ, width: fieldWidth(typ)}
		if w := (specs[i].width + 1) * fields; w > lineWidth {
			lineWidth = w
		}
	}
	for i, sp := range specs {
		specs[i].pad = lineWidth - sp.width*(blockSize/sp.size)
	}
	return specs, blockSize, true
}

func lcmInt(a, b int) int {
	return a / gcdInt(a, b) * b
}

func gcdInt(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

type outputType struct {
	kind   byte // a, c, d, f, o, u or x
	size   int  // the bytes each unit takes from the input
	z      bool // append the column of printable characters
	bfloat bool // a 16-bit bfloat, rather than a half float
}

// spec is an output type with the widths GNU aligns the fields of a line to.
type spec struct {
	outputType
	width int // the columns of a rendered field
	pad   int // the spaces spread over the fields of a line
}

// fieldWidth returns the number of columns a value of typ needs at most,
// as GNU computes it for the alignment of the output.
func fieldWidth(typ outputType) int {
	switch typ.kind {
	case 'a', 'c':
		return 3
	case 'd':
		return (8*typ.size-1)*302/1000 + 2
	case 'o':
		return (8*typ.size + 2) / 3
	case 'u':
		return 8*typ.size*302/1000 + 1
	case 'x':
		return (8*typ.size + 3) / 4
	default: // 'f'
		switch typ.size {
		case 2, 4:
			return 18
		case 8:
			return 36
		default:
			return 42 // the 16-byte long double
		}
	}
}

// padAt returns the spaces a line of fields fields and pad spaces to spread
// over them accumulates before its field i.
func padAt(fields, i, pad int) int {
	return pad/fields*i + pad%fields*i/fields
}

// renderType renders the first nbytes bytes of block as one output line of
// the type sp. The bytes beyond them only pad the units which would
// otherwise be partial, the way zero bytes extend the last input line.
func renderType(block []byte, nbytes int, sp spec, swap bool) string {
	fields := len(block) / sp.size
	blank := (len(block) - nbytes) / sp.size
	var row strings.Builder
	remaining := sp.pad
	for i := fields; blank < i; i-- {
		next := padAt(fields, i-1, sp.pad)
		unit := block[(fields-i)*sp.size : (fields-i+1)*sp.size]
		if swap && sp.size > 1 {
			unit = reverseBytes(unit)
		}
		fmt.Fprintf(&row, "%*s", remaining-next+sp.width, renderUnit(unit, sp.outputType))
		remaining = next
	}
	if sp.z {
		row.WriteString(strings.Repeat(" ", blank*sp.width+padAt(fields, blank, sp.pad)))
		row.WriteString("  >")
		for _, c := range block[:nbytes] {
			if isPrintable(c) {
				row.WriteByte(c)
			} else {
				row.WriteByte('.')
			}
		}
		row.WriteByte('<')
	}
	return row.String()
}

func renderUnit(b []byte, typ outputType) string {
	switch typ.kind {
	case 'c':
		return renderCharacter(b[0])
	case 'a':
		return namedCharacter(b[0] & 0x7f)
	}
	if typ.size == 16 {
		return formatLongDouble(b)
	}
	value := binary.LittleEndian.Uint64(pad8(b))
	switch typ.kind {
	case 'o':
		return fmt.Sprintf("%0*o", fieldWidth(typ), value)
	case 'x':
		return fmt.Sprintf("%0*x", fieldWidth(typ), value)
	case 'u':
		return strconv.FormatUint(value, 10)
	case 'd':
		bits := uint(len(b) * 8)
		if bits < 64 && value&(uint64(1)<<(bits-1)) != 0 {
			value |= ^uint64(0) << bits
		}
		return strconv.FormatInt(int64(value), 10)
	default: // 'f'
		switch len(b) {
		case 2:
			if typ.bfloat {
				return formatFloat(float64(math.Float32frombits(uint32(value)<<16)), 32, 6)
			}
			return formatFloat(halfFloat(uint16(value)), 32, 6)
		case 4:
			return formatFloat(float64(math.Float32frombits(uint32(value))), 32, 6)
		default:
			return formatFloat(math.Float64frombits(value), 64, 15)
		}
	}
}

// formatFloat renders a float the way GNU od does: with the significant
// digits of the value, never fewer than the precision of its type, in a
// representation that reads back as the same value.
func formatFloat(x float64, bitSize, minPrec int) string {
	switch {
	case math.IsNaN(x):
		return "nan"
	case math.IsInf(x, 1):
		return "inf"
	case math.IsInf(x, -1):
		return "-inf"
	}
	for prec := minPrec; prec <= 20; prec++ {
		s := strconv.FormatFloat(x, 'g', prec, bitSize)
		if back, err := strconv.ParseFloat(s, bitSize); err != nil || back == x {
			return s
		}
	}
	return strconv.FormatFloat(x, 'g', 20, bitSize)
}

// halfFloat decodes an IEEE 754 half precision float.
func halfFloat(u uint16) float64 {
	sign := float64(1)
	if u&0x8000 != 0 {
		sign = -1
	}
	exp := int(u>>10) & 0x1f
	mant := uint64(u & 0x3ff)
	switch exp {
	case 0:
		return sign * math.Ldexp(float64(mant), -24)
	case 0x1f:
		if mant == 0 {
			return sign * math.Inf(1)
		}
		return math.NaN()
	}
	return sign * math.Ldexp(float64(0x400|mant), exp-25)
}

// formatLongDouble renders the x87 80-bit extended precision float of the
// first ten bytes of b, as GNU reads a 16-byte long double, with the
// significant digits of its whole 64-bit significand: the ones a float64
// conversion would round away.
func formatLongDouble(b []byte) string {
	mant := binary.LittleEndian.Uint64(b[:8])
	se := binary.LittleEndian.Uint16(b[8:10])
	sign := ""
	if se&0x8000 != 0 {
		sign = "-"
	}
	switch exp := int(se & 0x7fff); {
	case exp == 0x7fff:
		if mant&^(uint64(1)<<63) == 0 {
			return sign + "inf"
		}
		return "nan"
	case exp == 0 && mant == 0:
		return sign + "0"
	case exp == 0: // a denormal: no integer bit, the exponent at its least
		return sign + formatPrecise(mant, -16445)
	default:
		return sign + formatPrecise(mant, exp-16383-63)
	}
}

// formatPrecise renders mant × 2^exp the way GNU renders a long double:
// with as few significant digits as it takes to read the value back into
// one, and never fewer than the 18 digits a long double is good for.
func formatPrecise(mant uint64, exp int) string {
	f := new(big.Float).SetPrec(64).SetMantExp(
		new(big.Float).SetPrec(64).SetUint64(mant), exp)
	for prec := 18; prec <= 25; prec++ {
		s := f.Text('g', prec)
		back, _, err := big.ParseFloat(s, 10, 64, big.ToNearestEven)
		if err != nil || back.Cmp(f) == 0 {
			return s
		}
	}
	return f.Text('g', 25)
}

func pad8(b []byte) []byte {
	var padded [8]byte
	copy(padded[:], b)
	return padded[:]
}

func reverseBytes(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[len(b)-1-i]
	}
	return out
}

// address renders a file offset as od writes it: in the radix of -A, or
// not at all with -A n, with the pseudo address of --traditional in
// parentheses, and the byte c after it when c is not zero.
func address(offset uint64, c byte, opts options) string {
	if opts.address == 'n' && !opts.labelSet {
		return ""
	}
	base := opts.address
	if base == 'n' {
		base = 'o' // GNU writes the pseudo address alone, in octal
	}
	var b strings.Builder
	if opts.address != 'n' {
		b.WriteString(formatAddress(offset, opts.address))
	}
	if opts.labelSet {
		if opts.address != 'n' {
			b.WriteByte(' ')
		}
		b.WriteString("(")
		b.WriteString(formatAddress(uint64(int64(offset)+opts.pseudoOffset()), base))
		b.WriteString(")")
	}
	if c != 0 {
		b.WriteByte(c)
	}
	return b.String()
}

// pseudoOffset returns the distance from a file offset to the pseudo
// address it is written with.
func (opts options) pseudoOffset() int64 {
	return int64(opts.label) - int64(opts.skip)
}

// writeAddress writes a file offset, with the byte c after it, unless the
// output has no addresses at all.
func (p *Prog) writeAddress(offset uint64, c byte, opts options) error {
	if s := address(offset, c, opts); s != "" {
		if _, err := io.WriteString(p.Stdout, s); err != nil {
			return err
		}
	}
	return nil
}

func formatAddress(offset uint64, base byte) string {
	switch base {
	case 'd':
		return fmt.Sprintf("%07d", offset)
	case 'x':
		return fmt.Sprintf("%06x", offset)
	default:
		return fmt.Sprintf("%07o", offset)
	}
}

func isPrintable(c byte) bool {
	return c >= 0x20 && c <= 0x7e
}

func renderCharacter(b byte) string {
	switch b {
	case 0:
		return "\\0"
	case '\\':
		return "\\\\"
	case '\a':
		return "\\a"
	case '\b':
		return "\\b"
	case '\f':
		return "\\f"
	case '\n':
		return "\\n"
	case '\r':
		return "\\r"
	case '\t':
		return "\\t"
	case '\v':
		return "\\v"
	default:
		if isPrintable(b) {
			return string(b)
		}
		return fmt.Sprintf("%03o", b)
	}
}

// escapeString renders the characters of a string of the -S output, with
// the backslash escapes the C language has for them.
func escapeString(b []byte) string {
	var out strings.Builder
	for _, c := range b {
		switch c {
		case '\a':
			out.WriteString("\\a")
		case '\b':
			out.WriteString("\\b")
		case '\f':
			out.WriteString("\\f")
		case '\n':
			out.WriteString("\\n")
		case '\r':
			out.WriteString("\\r")
		case '\t':
			out.WriteString("\\t")
		case '\v':
			out.WriteString("\\v")
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}

var characterNames = [...]string{
	"nul", "soh", "stx", "etx", "eot", "enq", "ack", "bel",
	"bs", "ht", "nl", "vt", "ff", "cr", "so", "si",
	"dle", "dc1", "dc2", "dc3", "dc4", "nak", "syn", "etb",
	"can", "em", "sub", "esc", "fs", "gs", "rs", "us",
	"sp",
}

func namedCharacter(b byte) string {
	if b <= 32 {
		return characterNames[b]
	}
	if b == 127 {
		return "del"
	}
	return string(b)
}

func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "od: "+format+"\n", args...)
}

var _ coreutils.Program = (*Prog)(nil)
