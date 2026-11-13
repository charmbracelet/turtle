package dd

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"charm.land/turtle/coreutils/internal/sizeparse"
)

// options holds the operands of one dd invocation, as parsed.
type options struct {
	input, output string

	ibs, obs, cbs, bs int64
	twoBufs           bool

	conv   convSet
	iflags flagSet
	oflags flagSet
	status statusLevel

	skip, seek, count int64
	countSet          bool

	skipBlocks, skipBytes   int64
	seekBlocks, seekBytes   int64
	countBlocks, countBytes int64
	skipInBytes             bool
	seekInBytes             bool
	countInBytes            bool
}

// direct reports whether each input block is written as a single output
// block, as bs= copies do when no conversion transforms the data.
func (op *options) direct() bool { return !op.twoBufs }

// flagSet is the set of flags an iflag= or oflag= operand asks for.
type flagSet uint8

const (
	flagFullblock flagSet = 1 << iota
	flagAppend
)

func (f flagSet) has(g flagSet) bool { return f&g != 0 }

var flagSymbols = map[string]flagSet{
	"append":    flagAppend,
	"fullblock": flagFullblock,
}

// statusLevel is the amount of information status= asks to print.
type statusLevel uint8

const (
	statusDefault statusLevel = iota
	statusNone
	statusNoxfer
	statusProgress
)

var statusSymbols = map[string]statusLevel{
	"none":     statusNone,
	"noxfer":   statusNoxfer,
	"progress": statusProgress,
}

// parseOperands parses the name=value operands, in any order.
func parseOperands(args []string) (*options, error) {
	op := &options{}
	for _, arg := range args {
		name, val, ok := strings.Cut(arg, "=")
		if !ok {
			return nil, fmt.Errorf("unrecognized operand '%s'", arg)
		}
		var err error
		switch name {
		case "if":
			op.input = val
		case "of":
			op.output = val
		case "conv":
			op.conv, err = parseConversions(val)
		case "iflag":
			op.iflags, err = parseFlags(val, "input flag")
		case "oflag":
			op.oflags, err = parseFlags(val, "output flag")
		case "status":
			op.status, err = parseStatus(val)
		case "ibs", "obs", "bs", "cbs":
			err = op.setSize(name, val)
		case "skip", "iseek":
			op.skip, op.skipInBytes, err = parseCount(val)
		case "seek", "oseek":
			op.seek, op.seekInBytes, err = parseCount(val)
		case "count":
			op.count, op.countInBytes, err = parseCount(val)
			op.countSet = true
		default:
			err = fmt.Errorf("unrecognized operand '%s'", arg)
		}
		if err != nil {
			return nil, err
		}
	}
	return op, nil
}

// parseConversions parses a conv= list, the character set symbols counting
// the record conversions they imply.
func parseConversions(val string) (convSet, error) {
	var set convSet
	for sym := range strings.SplitSeq(val, ",") {
		f, ok := convSymbols[sym]
		if !ok {
			return 0, fmt.Errorf("invalid conversion '%s'", sym)
		}
		set |= f
	}
	return set, nil
}

// parseFlags parses an iflag= or oflag= list.
func parseFlags(val, what string) (flagSet, error) {
	var set flagSet
	for sym := range strings.SplitSeq(val, ",") {
		f, ok := flagSymbols[sym]
		if !ok {
			return 0, fmt.Errorf("invalid %s '%s'", what, sym)
		}
		set |= f
	}
	return set, nil
}

// parseStatus parses a status= level, the last one winning when more than
// one is given.
func parseStatus(val string) (statusLevel, error) {
	syms := strings.Split(val, ",")
	var level statusLevel
	for _, sym := range syms {
		l, ok := statusSymbols[sym]
		if !ok {
			return 0, fmt.Errorf("invalid status level '%s'", sym)
		}
		level = l
	}
	return level, nil
}

// setSize parses the value of a block size operand.
func (op *options) setSize(name, val string) error {
	n, _, err := parseExpr(val)
	if err != nil || n == 0 || n > maxBlock {
		return fmt.Errorf("invalid number '%s'", val)
	}
	switch name {
	case "ibs":
		op.ibs = n
	case "obs":
		op.obs = n
	case "bs":
		op.bs = n
	case "cbs":
		op.cbs = n
	}
	return nil
}

// parseCount parses the value of a skip=, seek= or count= operand, whose
// trailing 'B' makes it count bytes rather than blocks.
func parseCount(val string) (int64, bool, error) {
	n, inBytes, err := parseExpr(val)
	if err != nil {
		return 0, false, fmt.Errorf("invalid number '%s'", val)
	}
	return n, inBytes, nil
}

// finalize resolves the operands against each other, checking the
// combinations POSIX and the GNU manual leave invalid.
func (op *options) finalize() error {
	if op.bs != 0 {
		op.ibs, op.obs = op.bs, op.bs
	}
	// Without bs=, or with a conversion that transforms the data, the
	// input is collected into full output blocks.
	op.twoBufs = op.bs == 0 ||
		op.conv.hasAny(convASCII, convEBCDIC, convIBM, convBlock, convUnblock,
			convLcase, convUcase, convSwab)
	if op.cbs == 0 {
		op.conv &^= convBlock | convUnblock
	}

	conv := op.conv
	if multiple(conv & (convASCII | convEBCDIC | convIBM)) {
		return fmt.Errorf("cannot combine any two of ascii, ebcdic and ibm")
	}
	if multiple(conv & (convBlock | convUnblock)) {
		return fmt.Errorf("cannot combine block and unblock")
	}
	if multiple(conv & (convLcase | convUcase)) {
		return fmt.Errorf("cannot combine lcase and ucase")
	}
	if multiple(conv & (convExcl | convNocreat)) {
		return fmt.Errorf("cannot combine excl and nocreat")
	}
	if op.oflags.has(flagFullblock) {
		return fmt.Errorf("invalid output flag 'fullblock'")
	}

	if op.ibs == 0 {
		op.ibs = defaultBlock
	}
	if op.obs == 0 {
		op.obs = defaultBlock
	}

	if op.skipInBytes {
		op.skipBlocks, op.skipBytes = op.skip/op.ibs, op.skip%op.ibs
	} else {
		op.skipBlocks = op.skip
	}
	if op.seekInBytes {
		op.seekBlocks, op.seekBytes = op.seek/op.obs, op.seek%op.obs
	} else {
		op.seekBlocks = op.seek
	}
	if op.countInBytes {
		op.countBlocks, op.countBytes = op.count/op.ibs, op.count%op.ibs
	} else {
		op.countBlocks = op.count
	}
	if !op.countSet {
		// No count= operand means every input block.
		op.countBlocks, op.countBytes = math.MaxInt64, 0
	}
	return nil
}

// multiple reports whether more than one bit of the set is on.
func multiple(set convSet) bool { return set&(set-1) != 0 }

// multiplier returns the multiplier a size suffix stands for, as 'b' for
// 512, 'k' for 1024, 'KB' for 1000 and 'KiB' for 1024 again. It also reports
// whether the suffix holds a 'B', which makes skip=, seek= and count= count
// bytes rather than blocks.
func multiplier(suffix string) (int64, bool, error) {
	if suffix == "B" {
		return 1, true, nil
	}
	mult, err := sizeparse.Multiplier(suffix)
	if err != nil {
		return 0, false, err
	}
	return mult, strings.ContainsRune(suffix, 'B'), nil
}

// parseExpr parses a size or count: one or more decimal numbers, each with
// an optional multiplier suffix, multiplied together across 'x' separators.
func parseExpr(s string) (int64, bool, error) {
	total := int64(1)
	inBytes := false
	for {
		part, rest, hasX := strings.Cut(s, "x")
		n, mult, b, err := parsePart(part)
		if err != nil {
			return 0, false, err
		}
		inBytes = inBytes || b
		if mult > 1 && n > math.MaxInt64/mult {
			return 0, false, fmt.Errorf("number too large")
		}
		if n*mult > math.MaxInt64/total {
			return 0, false, fmt.Errorf("number too large")
		}
		total *= n * mult
		if !hasX {
			return total, inBytes, nil
		}
		s = rest
	}
}

// parsePart parses one factor of a size or count: a decimal number with an
// optional multiplier suffix.
func parsePart(s string) (n, mult int64, inBytes bool, err error) {
	digits := s
	for digits != "" && digits[0] >= '0' && digits[0] <= '9' {
		digits = digits[1:]
	}
	if len(digits) == len(s) {
		return 0, 0, false, fmt.Errorf("invalid number")
	}
	n, err = strconv.ParseInt(s[:len(s)-len(digits)], 10, 64)
	if err != nil {
		return 0, 0, false, fmt.Errorf("invalid number")
	}
	mult, inBytes, err = multiplier(digits)
	if err != nil {
		return 0, 0, false, err
	}
	return n, mult, inBytes, nil
}
