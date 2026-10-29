// Package pr implements the POSIX pr utility.
//
// It paginates the given files into 66-line pages, each with a five-line
// header (the date, the file name and the page number) and a five-line
// trailer of blank lines, unless suppressed by -t or by a small -l. The text
// can be arranged in columns (-column, -a), merged from several files (-m),
// numbered (-n), offset (-o), double-spaced (-d), and terminated with form
// feeds (-f, -F).
package pr

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"charm.land/snail/coreutils"
)

// Prog is the pr core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run paginates the operands to the standard output. It returns an exit
// status of 1 for any error, including a file that couldn't be opened, in
// which case processing of the remaining files continues.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	if err := opts.setup(operands); err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}

	pr := &printer{prog: p, opts: opts, w: bufio.NewWriter(p.Stdout)}
	err = pr.run(ctx, operands)

	var writeErr *writeError
	switch {
	case errors.As(err, &writeErr):
		p.errorf("write error: %v", writeErr.err)
		return &coreutils.ExitError{Code: 1, Err: writeErr.err}
	case err != nil:
		return err
	}
	if pr.err {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// Default values of the options, as specified by POSIX.
const (
	defColumns  = 1
	defLines    = 66
	defWidth    = 72
	defWidthSep = 512
	defNumWidth = 5
	defGap      = 8
	headLen     = 5
	tailLen     = 5
	sepLen      = 1 // width of a blank column separator
	dateLayout  = "Jan _2 15:04 2006"
)

// options holds the parsed and settled command line options.
type options struct {
	page       int  // +page: first page to output
	columns    int  // -column: number of text columns
	columnsSet bool // -column was given
	merged     bool // -m given
	across     bool // -a given
	dspace     bool // -d given
	formfeed   bool // -f or -F given
	noHead     bool // -t given, or -l too small
	noDiag     bool // -r given
	customW    bool // -w given

	header   string // -h: replaces the file name in the header
	hasHead  bool   // -h given
	sep      byte   // -s: column separator character
	hasSep   bool   // -s given
	lines    int    // -l: page length in lines
	width    int    // -w: line width in columns
	offset   int    // -o: left margin in blanks
	number   bool   // -n given
	numChar  byte   // -n: separator after the number
	numWidth int    // -n: digits of the number
	expand   bool   // -e given or assumed: expand input tabs
	inChar   byte   // -e: input tab character
	inGap    int    // -e: tab stop distance
	contract bool   // -i given or assumed: contract blanks to tabs
	outChar  byte   // -i: output tab character
	outGap   int    // -i: tab stop distance

	// Settled during setup.
	cols      int  // effective number of columns
	colWidth  int  // content width of a text column; 0 means no truncation
	bodyLines int  // content lines per page
	addOne    bool // odd page length while double-spacing
}

// numberWidth returns the width of the line number field, or 0 when line
// numbers are off.
func (o *options) numberWidth() int {
	if !o.number {
		return 0
	}
	return o.numWidth + 1
}

// pitch returns the width of a text column cell, including the separator
// that follows it.
func (o *options) pitch() int {
	return o.colWidth + sepLen
}

// parser holds the state of the command line scan.
type parser struct {
	opts options
	args []string
	i    int // current argument
}

// parseFlags parses the pr command line, which doesn't follow the Utility
// Syntax Guidelines: "+page" and "-column" take numbers without an option
// letter, and the arguments of -e, -i, -n and -s must be attached to the
// option letter. Parsing stops at the first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	p := &parser{args: args}
	p.opts.numChar, p.opts.numWidth = '\t', defNumWidth
	p.opts.inChar, p.opts.outChar, p.opts.sep = '\t', '\t', '\t'
	for p.i = 0; p.i < len(args); p.i++ {
		arg := args[p.i]
		if arg == "--" {
			p.i++
			break
		}
		if arg == "-" || arg == "" || arg[0] != '-' && arg[0] != '+' {
			break
		}
		var err error
		if arg[0] == '+' {
			err = p.parsePage(arg)
		} else {
			err = p.parseGroup(arg)
		}
		if err != nil {
			return p.opts, nil, err
		}
	}
	return p.opts, args[p.i:], nil
}

// parsePage parses a "+page" argument.
func (p *parser) parsePage(arg string) error {
	digits := arg[1:]
	if !allDigits(digits) {
		bad := byte('+')
		if digits != "" {
			bad = digits[0]
		}
		return fmt.Errorf("invalid option -- '%c'", bad)
	}
	page, err := strconv.Atoi(digits)
	if err != nil || page < 1 {
		return errors.New("+page number must be 1 or more")
	}
	p.opts.page = page
	return nil
}

// parseGroup parses an argument of grouped pr options, which may start with
// the bare number of the -column option.
func (p *parser) parseGroup(arg string) error {
	for j := 1; j < len(arg); {
		if c := arg[j]; isDigit(c) {
			k := j
			for k < len(arg) && isDigit(arg[k]) {
				k++
			}
			n, err := strconv.Atoi(arg[j:k])
			if err != nil || n < 1 {
				return errors.New("-columns must be 1 or more")
			}
			p.opts.columns, p.opts.columnsSet = n, true
			j = k
			continue
		}
		next, err := p.parseOption(arg, j)
		if err != nil {
			return err
		}
		j = next
	}
	return nil
}

// parseOption applies a single option of the group starting at j, returning
// the index just after it in the group.
func (p *parser) parseOption(arg string, j int) (int, error) {
	opt := arg[j]
	rest := arg[j+1:]
	switch opt {
	case 'a':
		p.opts.across = true
	case 'd':
		p.opts.dspace = true
	case 'F', 'f':
		p.opts.formfeed = true
	case 'm':
		p.opts.merged = true
	case 'p':
		// Output is never a terminal for a library caller, so there's
		// nothing to pause for.
	case 'r':
		p.opts.noDiag = true
	case 't':
		p.opts.noHead = true
	case 'h', 'l', 'o', 'w':
		value := rest
		if value == "" {
			if p.i+1 >= len(p.args) {
				return 0, fmt.Errorf("option requires an argument -- '%c'", opt)
			}
			p.i++
			value = p.args[p.i]
		}
		return len(arg), p.setRequired(opt, value)
	case 'e', 'i', 'n', 's':
		return len(arg), p.setOptional(opt, rest)
	default:
		return 0, fmt.Errorf("invalid option -- '%c'", opt)
	}
	return j + 1, nil
}

// setRequired applies an option whose argument is mandatory: -h, -l, -o and
// -w.
func (p *parser) setRequired(opt byte, value string) error {
	switch opt {
	case 'h':
		p.opts.header, p.opts.hasHead = value, true
		return nil
	case 'l':
		n, err := parseNumber(value)
		if err != nil || n < 1 {
			return fmt.Errorf("number of lines must be 1 or more: '%s'", value)
		}
		p.opts.lines = n
		return nil
	case 'o':
		n, err := parseNumber(value)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid offset: '%s'", value)
		}
		p.opts.offset = n
		return nil
	case 'w':
		n, err := parseNumber(value)
		if err != nil || n < 1 {
			return fmt.Errorf("-w width must be 1 or more: '%s'", value)
		}
		p.opts.width, p.opts.customW = n, true
		return nil
	}
	return nil
}

// setOptional applies an option whose argument must be attached and is
// optional: -e, -i, -n and -s.
func (p *parser) setOptional(opt byte, value string) error {
	switch opt {
	case 'e':
		gap, err := parseGap(value, &p.opts.inChar, opt)
		if err != nil {
			return err
		}
		p.opts.expand, p.opts.inGap = true, gap
		return nil
	case 'i':
		gap, err := parseGap(value, &p.opts.outChar, opt)
		if err != nil {
			return err
		}
		p.opts.contract, p.opts.outGap = true, gap
		return nil
	case 'n':
		sep, width := byte('\t'), defNumWidth
		if value != "" && !isDigit(value[0]) {
			sep = value[0]
			value = value[1:]
		}
		if value != "" {
			n, err := parseNumber(value)
			if err != nil || n < 1 {
				return fmt.Errorf("-n width must be 1 or more: '%s'", value)
			}
			width = n
		}
		p.opts.number, p.opts.numChar, p.opts.numWidth = true, sep, width
		return nil
	case 's':
		p.opts.hasSep = true
		if value == "" {
			return nil
		}
		if len(value) != 1 {
			return fmt.Errorf("invalid value for -s: '%s'", value)
		}
		p.opts.sep = value[0]
		return nil
	}
	return nil
}

// parseGap parses the "[char][gap]" argument of -e and -i.
func parseGap(value string, char *byte, opt byte) (int, error) {
	gap := defGap
	if value != "" && !isDigit(value[0]) {
		*char = value[0]
		value = value[1:]
	}
	if value != "" {
		n, err := parseNumber(value)
		if err != nil {
			return 0, fmt.Errorf("invalid value for -%c: '%s'", opt, value)
		}
		if n == 0 {
			n = defGap
		}
		gap = n
	}
	return gap, nil
}

// setup settles the option values, as POSIX and the historical practice
// require.
func (o *options) setup(operands []string) error {
	if o.page == 0 {
		o.page = 1
	}
	if o.lines == 0 {
		o.lines = defLines
	}
	cols := o.columns
	if cols == 0 {
		if o.merged {
			if cols = len(operands); cols <= 1 {
				cols, o.merged = defColumns, false
			}
		} else {
			cols = defColumns
		}
	} else if o.merged {
		return errors.New("-m cannot be used with multiple columns")
	}
	if o.across && cols == 1 {
		return errors.New("-a flag requires multiple columns")
	}
	if o.across && o.merged {
		return errors.New("-m cannot be used with -a")
	}
	if !o.customW {
		if o.hasSep {
			o.width = defWidthSep
		} else {
			o.width = defWidth
		}
	}

	// Tab processing is assumed for multiple column output.
	if cols > 1 || o.merged {
		if !o.expand {
			o.expand, o.inChar, o.inGap = true, '\t', defGap
		}
		if !o.contract {
			o.contract, o.outChar, o.outGap = true, '\t', defGap
		}
	}
	o.cols = cols
	if cols > 1 && !o.merged {
		// One blank separator follows each column but the last.
		o.colWidth = (o.width + sepLen - cols*(o.numberWidth()+sepLen)) / cols
		if o.colWidth < 1 {
			return errors.New("page width too narrow")
		}
	}

	if o.lines <= headLen+tailLen {
		o.noHead = true
	} else if !o.noHead {
		o.lines -= headLen + tailLen
	}
	o.bodyLines = o.lines
	if o.dspace {
		if o.bodyLines == 1 {
			o.dspace = false
		} else {
			o.addOne = o.bodyLines%2 == 1
			o.bodyLines /= 2
		}
	}
	return nil
}

// parseNumber parses a decimal number, allowing a leading sign.
func parseNumber(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, errors.New("not a number")
	}
	return n, nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

// location returns the time zone of the header dates, honoring TZ.
func (p *Prog) location() *time.Location {
	if tz, ok := p.LookupEnv("TZ"); ok && tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.Local
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "pr: "+format+"\n", args...)
}

// writeError marks an error as coming from writing to the standard output.
type writeError struct {
	err error
}

// Error implements the error interface.
func (e *writeError) Error() string {
	return e.err.Error()
}

// Unwrap implements the interface to make writeError unwrapable.
func (e *writeError) Unwrap() error {
	return e.err
}

var _ coreutils.Program = (*Prog)(nil)
