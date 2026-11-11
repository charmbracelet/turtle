// Package pr implements the pr utility, with the POSIX surface and the GNU
// extensions.
//
// It paginates the given files into 66-line pages, each with a five-line
// header (the date, the centered file name and the page number, padded to the
// page width) and a five-line trailer of blank lines, unless suppressed by -t,
// -T or by a small -l. The text can be arranged in columns (-column, -a),
// merged from several files (-m), numbered (-n), offset (-o), double-spaced
// (-d), terminated with form feeds (-f, -F) and truncated to the page width
// (-w, -W). Form feeds in the input cause page breaks, unless -T is given.
package pr

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
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
	if done, err := stdopts.Check(p.Runtime, "pr", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "pr")
		return &coreutils.ExitError{Code: 1}
	}
	if err := opts.setup(operands); err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "pr")
		return &coreutils.ExitError{Code: 1}
	}
	if !opts.hasDateFmt {
		if _, ok := p.LookupEnv("POSIXLY_CORRECT"); ok {
			opts.dateFmt = posixDateFmt
		} else {
			opts.dateFmt = gnuDateFmt
		}
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

// Default values of the options, as specified by POSIX and GNU.
const (
	defColumns  = 1
	defLines    = 66
	defWidth    = 72
	defNumWidth = 5
	defGap      = 8
	headLen     = 5
	tailLen     = 5

	// The date formats of the header, per the -D option of GNU pr.
	gnuDateFmt   = "%Y-%m-%d %H:%M"
	posixDateFmt = "%b %e %H:%M %Y"
)

// options holds the parsed and settled command line options.
type options struct {
	page       int  // +FIRST or --pages: first page to output
	lastPage   int  // +FIRST:LAST or --pages: last page; 0 is unlimited
	columns    int  // -column: number of text columns
	columnsSet bool // -column or --columns was given
	merged     bool // -m given
	across     bool // -a given
	dspace     bool // -d given
	formfeed   bool // -f or -F given
	noHead     bool // -t, -T or a too small -l given
	omitPag    bool // -T given: also ignore input form feeds
	noDiag     bool // -r given
	oldW       bool // -w given, the old-style width
	oldS       bool // -s given, the old-style separator
	truncateW  bool // -W given: truncate lines even in single column mode

	header   string // -h: replaces the file name in the header
	hasHead  bool   // -h given
	lines    int    // -l: page length in lines
	width    int    // -w/-W: line width in columns
	offset   int    // -o: left margin in blanks
	number   bool   // -n given
	numChar  byte   // -n: separator after the number
	numWidth int    // -n: digits of the number
	firstNum int    // -N: first line number of the first printed line
	numSet   bool   // -N given
	expand   bool   // -e given or assumed: expand input tabs
	inChar   byte   // -e: input tab character
	inGap    int    // -e: tab stop distance
	contract bool   // -i given or assumed: contract blanks to tabs
	outChar  byte   // -i: output tab character
	outGap   int    // -i: tab stop distance

	sepStr   string // -s or -S: column separator string
	sepGiven bool   // -s with a character, or -S, was given
	sepSet   bool   // -S given: dominates -s

	showCtrl   bool   // -c: control characters in hat notation
	showEsc    bool   // -v: nonprinting characters in octal
	dateFmt    string // -D: date format of the header
	hasDateFmt bool   // -D given
	join       bool   // -J given

	// Settled during setup.
	cuts       []int  // columns where a blank run is split
	cols       int    // effective number of columns
	sep        string // effective column separator
	sepLen     int    // its length in bytes
	useSep     bool   // the separator was given explicitly
	truncate   bool   // lines are truncated to the column width
	colWidth   int    // content width of a text column
	numFieldW  int    // width of the line number field, or 0 when off
	textBudget int    // columns available to the text of a cell
	bodyLines  int    // content lines per page
	addOne     bool   // odd page length while double-spacing
}

// usage describes the options of pr.
const usage = `Usage: pr [OPTION]... [FILE]...
Paginate or columnate FILE(s) for printing.

By default, each page has a five-line header and a five-line trailer of blank
lines. The standard input is read when no FILE is given, or when a FILE is -.

  +FIRST_PAGE[:LAST_PAGE], --pages=FIRST_PAGE[:LAST_PAGE]
         begin output with page FIRST_PAGE, ending with LAST_PAGE
  -COLUMN, --columns=COLUMN
         produce COLUMN columns of output
  -a, --across
         print columns across rather than down, with -COLUMN
  -c, --show-control-chars
         print control characters in hat notation, other nonprinting
         characters as octal escapes
  -d, --double-space
         double space the output
  -D, --date-format=FORMAT
         format header dates with FORMAT, as date +FORMAT does
  -e[CHAR][GAP], --expand-tabs[=CHAR[GAP]]
         expand input tabs to spaces, or CHAR, to every GAPth column
  -f, -F, --form-feed
         use form feeds, not newlines, to separate pages
  -h, --header=HEADER
         use HEADER instead of the file name in the header
  -i[CHAR][GAP], --output-tabs[=CHAR[GAP]]
         replace runs of blanks in the output with tabs, or CHAR, every GAPth
         column
  -J, --join-lines
         merge full lines, without column alignment or truncation
  -l, --length=LINES
         page length of LINES lines (default 66); LINES of 10 or fewer omit
         the header and trailer
  -m, --merge
         merge all files, printing one per column
  -n[CHAR][WIDTH], --number-lines[=CHAR[WIDTH]]
         number lines with WIDTH digits (default 5), followed by CHAR (default
         TAB)
  -N, --first-line-number=NUMBER
         start counting the line numbers at NUMBER
  -o, --indent=MARGIN
         offset lines by MARGIN blanks (default 0)
  -p
         pause at the end of each page when writing to a terminal (ignored)
  -r, --no-file-warnings
         do not warn when a file cannot be opened
  -s[CHAR], --separator=CHAR
         separate columns with a single CHAR (default TAB), turning off
         truncation and column alignment
  -S[STRING], --sep-string[=STRING]
         separate columns with STRING, keeping truncation and alignment
  -t, --omit-header
         do not print the header and trailer, keeping input form feeds
  -T, --omit-pagination
         do not print the header and trailer, ignoring input form feeds
  -v, --show-nonprinting
         print nonprinting characters as octal escapes
  -w, --width=WIDTH
         page width of WIDTH columns (default 72) for multi-column output
  -W, --page-width=WIDTH
         page width of WIDTH columns, truncating even single-column output
      --help
         display this help and exit
      --version
         output version information and exit
`

// The long options which stand for no short option.
const (
	optPages   rune = 0x110000 + iota // --pages
	optColumns                        // --columns
)

// argKind tells how a long option takes its argument.
type argKind int

const (
	noArg  argKind = iota // --name
	reqArg                // --name=VALUE
	optArg                // --name[=VALUE]
)

// longOption describes one long option of pr.
type longOption struct {
	name string
	opt  rune
	arg  argKind
}

// longOptions lists the long options, with the short option each one stands
// for.
var longOptions = []longOption{
	{"pages", optPages, reqArg},
	{"columns", optColumns, reqArg},
	{"across", 'a', noArg},
	{"show-control-chars", 'c', noArg},
	{"double-space", 'd', noArg},
	{"date-format", 'D', reqArg},
	{"expand-tabs", 'e', optArg},
	{"form-feed", 'f', noArg},
	{"header", 'h', reqArg},
	{"output-tabs", 'i', optArg},
	{"join-lines", 'J', noArg},
	{"length", 'l', reqArg},
	{"merge", 'm', noArg},
	{"number-lines", 'n', optArg},
	{"first-line-number", 'N', reqArg},
	{"indent", 'o', reqArg},
	{"no-file-warnings", 'r', noArg},
	{"separator", 's', reqArg},
	{"sep-string", 'S', optArg},
	{"omit-header", 't', noArg},
	{"omit-pagination", 'T', noArg},
	{"show-nonprinting", 'v', noArg},
	{"width", 'w', reqArg},
	{"page-width", 'W', reqArg},
}

// longMap maps the long option names to the runes they stand for, for the
// --help and --version scan of stdopts.Check.
var longMap = func() map[string]rune {
	m := make(map[string]rune, len(longOptions))
	for _, lo := range longOptions {
		m[lo.name] = lo.opt
	}
	return m
}()

// optSet describes the options of pr for the standard option scan.
var optSet = getopt.Set{
	Valued:   "DhilNoWw",
	Optional: "einSs",
	Long:     longMap,
}

// parser holds the state of the command line scan.
type parser struct {
	opts options
	args []string
	i    int // current argument
}

// parseFlags parses the pr command line, which doesn't follow the Utility
// Syntax Guidelines: "+page" and "-column" take numbers without an option
// letter, and the arguments of -e, -i, -n, -s and -S must be attached to the
// option letter. Long options are accepted in the GNU form. Parsing stops at
// the first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	p := &parser{args: args}
	p.opts.numChar, p.opts.numWidth = '\t', defNumWidth
	p.opts.inChar, p.opts.outChar = '\t', '\t'
	p.opts.inGap, p.opts.outGap = defGap, defGap
	p.opts.width = defWidth
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
		switch {
		case strings.HasPrefix(arg, "--"):
			err = p.parseLong(arg)
		case arg[0] == '+':
			err = p.parsePage(arg)
		default:
			err = p.parseGroup(arg)
		}
		if err != nil {
			return p.opts, nil, err
		}
	}
	return p.opts, args[p.i:], nil
}

// parseLong applies one long option argument.
func (p *parser) parseLong(arg string) error {
	name, value, given := strings.Cut(arg[2:], "=")
	var lo *longOption
	for i := range longOptions {
		if longOptions[i].name == name {
			lo = &longOptions[i]
			break
		}
	}
	if lo == nil {
		return fmt.Errorf("unrecognized option '%s'", arg)
	}
	switch lo.arg {
	case reqArg:
		if !given {
			// The argument may be given in the next argument too, as in
			// "--length 12", the way the GNU long options work.
			if p.i+1 >= len(p.args) {
				return fmt.Errorf("option '%s' requires an argument", arg)
			}
			p.i++
			value = p.args[p.i]
		}
	case noArg:
		if given {
			return fmt.Errorf("option '%s' doesn't allow an argument", arg)
		}
	}
	switch lo.opt {
	case optPages:
		return p.setPage(value, arg)
	case optColumns:
		n, err := parseNumber(value)
		if err != nil || n < 1 {
			return fmt.Errorf("invalid number of columns: '%s'", value)
		}
		p.opts.columns, p.opts.columnsSet = n, true
		return nil
	case 'a', 'c', 'd', 'f', 'J', 'm', 'r', 't', 'T', 'v':
		return p.setFlag(lo.opt)
	default:
		return p.setOption(lo.opt, value)
	}
}

// setFlag applies a long option which takes no argument.
func (p *parser) setFlag(opt rune) error {
	_, err := p.parseOption(string(opt), 0)
	return err
}

// setOption applies a long option with its attached argument.
func (p *parser) setOption(opt rune, value string) error {
	switch opt {
	case 'h', 'l', 'o', 'w', 'N', 'D', 'W':
		return p.setRequired(byte(opt), value)
	case 'e', 'i', 'n', 's', 'S':
		return p.setOptional(byte(opt), value)
	}
	return getopt.InvalidOption(opt)
}

// parsePage parses a "+first[:last]" argument.
func (p *parser) parsePage(arg string) error {
	return p.setPage(arg[1:], arg)
}

// setPage applies a first[:last] page range, as the "+page" operand and the
// --pages option do.
func (p *parser) setPage(value, arg string) error {
	first, last, hasLast := strings.Cut(value, ":")
	if !allDigits(first) {
		bad := byte('+')
		if first != "" {
			bad = first[0]
		}
		return fmt.Errorf("invalid option -- '%c'", bad)
	}
	page, err := strconv.Atoi(first)
	if err != nil || page < 1 {
		return fmt.Errorf("invalid page range '%s'", value)
	}
	p.opts.page = page
	if !hasLast {
		return nil
	}
	if !allDigits(last) {
		return fmt.Errorf("invalid page range '%s'", value)
	}
	lastPage, err := strconv.Atoi(last)
	if err != nil || lastPage < page {
		return fmt.Errorf("invalid page range '%s'", value)
	}
	p.opts.lastPage = lastPage
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
	rest := ""
	if j+1 < len(arg) {
		rest = arg[j+1:]
	}
	switch opt {
	case 'a':
		p.opts.across = true
	case 'c':
		p.opts.showCtrl = true
	case 'd':
		p.opts.dspace = true
	case 'F', 'f':
		p.opts.formfeed = true
	case 'J':
		p.opts.join = true
	case 'm':
		p.opts.merged = true
	case 'p':
		// Output is never a terminal for a library caller, so there's
		// nothing to pause for.
	case 'r':
		p.opts.noDiag = true
	case 't':
		p.opts.noHead = true
	case 'T':
		p.opts.noHead = true
		p.opts.omitPag = true
	case 'v':
		p.opts.showEsc = true
	case 'h', 'l', 'o', 'w', 'N', 'D', 'W':
		value := rest
		if value == "" {
			if p.i+1 >= len(p.args) {
				return 0, fmt.Errorf("option requires an argument -- '%c'", opt)
			}
			p.i++
			value = p.args[p.i]
		}
		return len(arg), p.setRequired(opt, value)
	case 'e', 'i', 'n', 's', 'S':
		return len(arg), p.setOptional(opt, rest)
	default:
		return 0, fmt.Errorf("invalid option -- '%c'", opt)
	}
	return j + 1, nil
}

// setRequired applies an option whose argument is mandatory: -h, -l, -o, -w,
// -N, -D and -W.
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
		p.opts.oldW = true
		// -W dominates -w: once it was given, -w changes nothing.
		if !p.opts.truncateW {
			p.opts.width = n
		}
		return nil
	case 'W':
		n, err := parseNumber(value)
		if err != nil || n < 1 {
			return fmt.Errorf("-W width must be 1 or more: '%s'", value)
		}
		p.opts.truncateW = true
		p.opts.oldW = false
		p.opts.width = n
		return nil
	case 'N':
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid starting line number: '%s'", value)
		}
		p.opts.firstNum, p.opts.numSet = n, true
		return nil
	case 'D':
		p.opts.dateFmt, p.opts.hasDateFmt = value, true
		return nil
	}
	return nil
}

// setOptional applies an option whose argument must be attached and is
// optional: -e, -i, -n, -s and -S.
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
		p.opts.oldS = true
		if value == "" {
			return nil
		}
		if len(value) != 1 {
			return fmt.Errorf("invalid value for -s: '%s'", value)
		}
		if !p.opts.sepSet {
			p.opts.sepStr, p.opts.sepGiven = value, true
		}
		return nil
	case 'S':
		// -S dominates a previous -s, and a later -s is ignored.
		p.opts.sepSet = true
		p.opts.useSep = true
		p.opts.sepStr, p.opts.sepGiven = value, true
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
// require, following the option translation of GNU pr: the old-style -w and
// -s options are turned into the new -W and -J/-S ones.
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

	// -W truncates the lines even in single column mode.
	if o.truncateW {
		o.truncate = true
	}

	// Translate the old-style -w and -s, as GNU pr does. With a column
	// option, -w keeps the lines truncated and -s keeps only its separator;
	// without one, both turn off truncation and column alignment.
	columned := o.merged || o.columnsSet
	switch {
	case o.oldW:
		if columned {
			o.truncate = true
			if o.oldS {
				o.useSep = true
			}
		} else {
			o.join = true
		}
	case !o.sepSet && o.oldS && columned:
		if !o.truncate {
			o.join = true
			if o.sepGiven {
				o.useSep = true
			}
		} else {
			o.useSep = true
		}
	}

	// Settle the columns, the separator and the truncation.
	o.cols = cols
	if cols > 1 {
		if !o.useSep {
			if o.join {
				o.sep = "\t"
			} else {
				o.sep = " "
			}
			o.sepLen = 1
			o.useSep = true
		} else {
			o.sep, o.sepLen = o.sepStr, len(o.sepStr)
			// A single TAB separator is pointless with column alignment.
			if !o.join && o.sepLen == 1 && o.sep == "\t" {
				o.sep = " "
			}
		}
		o.truncate = true
		// Tab processing is assumed for multiple column output.
		if !(o.sepLen == 1 && o.sep == "\t") {
			o.expand = true
		}
		o.contract = true
	} else {
		o.sep, o.sepLen = o.sepStr, len(o.sepStr)
	}
	if o.join {
		o.truncate = false
	}

	if o.number {
		if o.numChar == '\t' {
			// A TAB number separator is expanded to the next tab stop.
			o.numFieldW = o.numWidth + defGap - o.numWidth%defGap
		} else {
			o.numFieldW = o.numWidth + 1
		}
	}
	if o.truncate {
		used := 0
		if o.merged && o.number {
			// The number of a merged line stands outside the columns.
			used = o.numFieldW
		}
		o.colWidth = (o.width - used - (o.cols-1)*o.sepLen) / o.cols
		if o.colWidth < 1 {
			return errors.New("page width too narrow")
		}
		o.textBudget = o.colWidth
		if o.number && !o.merged {
			o.textBudget -= o.numFieldW
		}
		// A blank run ends at the margin and at each column separator.
		o.cuts = append(o.cuts, o.offset)
		for k := 1; k < o.cols; k++ {
			o.cuts = append(o.cuts, o.offset+k*(o.colWidth+o.sepLen))
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
