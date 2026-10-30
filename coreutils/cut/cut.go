// Package cut implements the cut utility: the POSIX specification plus the
// main GNU extensions.
//
// It cuts out selected bytes, characters, or delimiter-separated fields of
// each line of the given files. The standard input is used when no operands
// are given, and also when an operand is "-".
package cut

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the cut core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run cuts the selected portions of each line of the given files and writes
// them to the standard output. Lines are decoded as UTF-8; each byte that
// does not start a valid encoding counts as one character.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "cut", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "cut")
		return &coreutils.ExitError{Code: 2}
	}

	inputs := make([]input.Input, 0, len(operands))
	for _, operand := range operands {
		inputs = append(inputs, input.New(operand))
	}
	if len(inputs) == 0 {
		inputs = append(inputs, input.Input{Stdin: true})
	}

	ok := true
	for _, in := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !in.Stdin && in.Name == "" {
			p.errorf("invalid zero-length file name")
			ok = false
			continue
		}
		if err := p.cut(ctx, in, opts); err != nil {
			var writeErr *writeError
			switch {
			case errors.As(err, &writeErr):
				p.errorf("write error: %v", writeErr.err)
				return &coreutils.ExitError{Code: 1, Err: writeErr.err}
			case ctx.Err() != nil:
				return ctx.Err()
			default:
				p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
				ok = false
			}
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// cut cuts the selected portions of each line of a single input.
func (p *Prog) cut(ctx context.Context, in input.Input, opts options) error {
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		return err
	}
	defer r.Close()

	br := bufio.NewReader(r)
	term := byte('\n')
	if opts.zero {
		term = 0
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := br.ReadBytes(term)
		if len(line) > 0 {
			if cerr := p.cutLine(bytes.TrimSuffix(line, []byte{term}), opts); cerr != nil {
				return cerr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// cutLine writes the selected portions of a line to the standard output.
func (p *Prog) cutLine(line []byte, opts options) error {
	var out []byte
	switch opts.mode {
	case modeFields:
		fields, ok := opts.cutFields(line)
		if !ok {
			return nil
		}
		out = fields
	case modeChars:
		out = opts.cutChars(line)
	default:
		out = opts.cutBytes(line)
	}
	term := byte('\n')
	if opts.zero {
		term = 0
	}
	out = append(out, term)
	if _, err := p.Stdout.Write(out); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "cut: "+format+"\n", args...)
}

// mode is the kind of position a list selects.
type mode int

const (
	modeBytes mode = iota
	modeChars
	modeFields
)

// options holds the parsed command line options.
type options struct {
	mode       mode
	spans      []span
	delim      byte
	outDelim   []byte
	noSplit    bool
	suppress   bool
	complement bool
	zero       bool
}

// longOptions maps the long options of cut to the short ones they stand for.
var longOptions = map[string]rune{
	"bytes":            'b',
	"characters":       'c',
	"complement":       complementOpt,
	"delimiter":        'd',
	"fields":           'f',
	"only-delimited":   's',
	"output-delimiter": 'O',
	"zero-terminated":  'z',
}

// complementOpt is the pseudo short option --complement stands for, as it
// has no short form.
const complementOpt rune = 0x100

// optSet describes the options of cut.
var optSet = getopt.Set{Valued: "bcdfO", Long: longOptions}

// usage describes the options of cut.
const usage = `Usage: cut OPTION... [FILE]...
Print selected parts of lines from each FILE to standard output.

With no FILE, or when FILE is -, read standard input.

  -b, --bytes=LIST        select only these bytes
  -c, --characters=LIST   select only these characters
  -d, --delimiter=DELIM   use DELIM instead of TAB for field delimiter
  -f, --fields=LIST       select only these fields;  also print any line
                            that contains no delimiter character, unless
                            the -s option is specified
  -n                      with -b, do not split multi-byte characters
      --complement        complement the set of selected bytes, characters
                            or fields
  -s, --only-delimited    do not print lines not containing delimiters
  -O, --output-delimiter=STRING
                            use STRING as the output delimiter
  -z, --zero-terminated   line delimiter is NUL, not newline
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags parses the options and operands, checking that exactly one list
// option is given and that the other options combine with it. Option parsing
// stops at the first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{delim: '\t'}
	var (
		listSeen    rune
		delimSet    bool
		outDelimSet bool
		nSet        bool
		sSet        bool
	)
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'b', 'c', 'f':
			if listSeen != 0 && listSeen != opt {
				return errors.New("only one of -b, -c or -f may be specified")
			}
			listSeen = opt
			spans, err := parseList(value)
			if err != nil {
				return err
			}
			opts.spans = spans
			switch opt {
			case 'c':
				opts.mode = modeChars
			case 'f':
				opts.mode = modeFields
			}
		case 'd':
			if len(value) != 1 {
				return errors.New("the delimiter must be a single character")
			}
			opts.delim = value[0]
			delimSet = true
		case 'n':
			opts.noSplit = true
			nSet = true
		case 's':
			opts.suppress = true
			sSet = true
		case 'O':
			opts.outDelim = []byte(value)
			outDelimSet = true
		case 'z':
			opts.zero = true
		case complementOpt:
			opts.complement = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	if listSeen == 0 {
		return options{}, nil, errors.New("you must specify a list of bytes, characters, or fields")
	}
	if opts.mode != modeFields && delimSet {
		return options{}, nil, errors.New("the delimiter is only meaningful with the -f option")
	}
	if opts.mode != modeFields && sSet {
		return options{}, nil, errors.New("the -s option is only meaningful with the -f option")
	}
	if opts.mode != modeBytes && nSet {
		return options{}, nil, errors.New("the -n option is only meaningful with the -b option")
	}
	if !outDelimSet && opts.mode == modeFields {
		opts.outDelim = []byte{opts.delim}
	} else if outDelimSet && len(opts.outDelim) == 0 {
		opts.outDelim = []byte{0}
	}
	return opts, operands, nil
}

// span is an inclusive range of 1-based positions. A zero high stands for
// "to the end of the line".
type span struct {
	low, high int
}

// parseList parses a comma or blank separated list of positive numbers and
// ranges, merging them into sorted disjoint spans.
func parseList(list string) ([]span, error) {
	var spans []span
	for elem := range strings.SplitSeq(strings.NewReplacer(" ", ",", "\t", ",").Replace(list), ",") {
		sp, err := parseSpan(elem)
		if err != nil {
			return nil, err
		}
		spans = append(spans, sp)
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].low < spans[j].low })
	merged := spans[:0]
	for _, sp := range spans {
		last := len(merged) - 1
		if last >= 0 && (merged[last].high == 0 || sp.low <= merged[last].high+1) {
			if merged[last].high != 0 && (sp.high == 0 || sp.high > merged[last].high) {
				merged[last].high = sp.high
			}
			continue
		}
		merged = append(merged, sp)
	}
	return merged, nil
}

// parseSpan parses one list element: a number, low-high, -high or low-.
func parseSpan(elem string) (span, error) {
	low, high, dash := strings.Cut(elem, "-")
	switch {
	case !dash:
		n, err := parsePosition(elem)
		return span{n, n}, err
	case low == "" && high == "":
		return span{}, fmt.Errorf("invalid list: '%s'", elem)
	case low == "":
		n, err := parsePosition(high)
		return span{1, n}, err
	case high == "":
		n, err := parsePosition(low)
		return span{n, 0}, err
	default:
		lo, err := parsePosition(low)
		if err != nil {
			return span{}, err
		}
		hi, err := parsePosition(high)
		if err != nil {
			return span{}, err
		}
		if lo > hi {
			return span{}, fmt.Errorf("invalid decreasing range: '%s'", elem)
		}
		return span{lo, hi}, nil
	}
}

// parsePosition parses a positive decimal position.
func parsePosition(s string) (int, error) {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("invalid position: '%s'", s)
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid position: '%s'", s)
	}
	return n, nil
}

// cutBytes returns the selected bytes of the line, separated by the output
// delimiter between the written ranges.
func (opts options) cutBytes(line []byte) []byte {
	spans := opts.spans
	if opts.noSplit {
		spans = charSpans(line, spans)
	}
	if opts.complement {
		spans = complementSpans(spans)
	}
	var out []byte
	wrote := false
	for _, sp := range spans {
		high := sp.high
		if high == 0 || high > len(line) {
			high = len(line)
		}
		if sp.low > high {
			continue
		}
		if wrote {
			out = append(out, opts.outDelim...)
		}
		out = append(out, line[sp.low-1:high]...)
		wrote = true
	}
	return out
}

// cutChars returns the selected characters of the line, separated by the
// output delimiter between the written ranges.
func (opts options) cutChars(line []byte) []byte {
	bounds := charBounds(line)
	spans := opts.spans
	if opts.complement {
		spans = complementSpans(spans)
	}
	var out []byte
	wrote := false
	for _, sp := range spans {
		high := sp.high
		if high == 0 || high > len(bounds) {
			high = len(bounds)
		}
		if sp.low > high {
			continue
		}
		if wrote {
			out = append(out, opts.outDelim...)
		}
		for i := sp.low; i <= high; i++ {
			end := len(line)
			if i < len(bounds) {
				end = bounds[i]
			}
			out = append(out, line[bounds[i-1]:end]...)
		}
		wrote = true
	}
	return out
}

// cutFields returns the selected fields of the line, joined by the output
// delimiter. Lines without a delimiter are returned as-is, unless -s
// suppresses them.
func (opts options) cutFields(line []byte) ([]byte, bool) {
	if !bytes.Contains(line, []byte{opts.delim}) {
		return line, !opts.suppress
	}
	fields := bytes.Split(line, []byte{opts.delim})
	spans := opts.spans
	if opts.complement {
		spans = complementSpans(spans)
	}
	var out []byte
	wrote := false
	for _, sp := range spans {
		high := sp.high
		if high == 0 || high > len(fields) {
			high = len(fields)
		}
		for i := sp.low; i <= high; i++ {
			if wrote {
				out = append(out, opts.outDelim...)
			}
			out = append(out, fields[i-1]...)
			wrote = true
		}
	}
	return out, true
}

// complementSpans returns the gaps the spans leave in the positions of a
// line, in order: the span from the start up to the first one, the ones
// between them, and a final one to the end of the line.
func complementSpans(spans []span) []span {
	var gaps []span
	next := 1
	for _, sp := range spans {
		if sp.low > next {
			gaps = append(gaps, span{next, sp.low - 1})
		}
		if sp.high == 0 {
			return gaps
		}
		next = sp.high + 1
	}
	return append(gaps, span{next, 0})
}

// charBounds returns the byte offsets where the characters of the line
// start, decoding it as UTF-8. Each byte that does not start a valid encoding
// counts as one character.
func charBounds(line []byte) []int {
	bounds := []int{0}
	for i := 0; i < len(line); {
		_, size := utf8.DecodeRune(line[i:])
		i += size
		if i < len(line) {
			bounds = append(bounds, i)
		}
	}
	return bounds
}

// charSpans adjusts the spans to the character boundaries of the line, as -n
// requires: lows move back to the start of their character and highs move
// back to the end of the previous character. Empty results are dropped.
func charSpans(line []byte, spans []span) []span {
	bounds := charBounds(line)
	adjusted := make([]span, 0, len(spans))
	for _, sp := range spans {
		if sp.low > len(line) {
			continue
		}
		high := sp.high
		if high == 0 || high > len(line) {
			high = len(line)
		}
		low := bounds[boundIndex(bounds, sp.low-1)] + 1
		if !isCharEnd(bounds, high, len(line)) {
			high = bounds[boundIndex(bounds, high-1)]
		}
		if high == 0 || low > high {
			continue
		}
		adjusted = append(adjusted, span{low, high})
	}
	return adjusted
}

// boundIndex returns the index of the greatest bound less than or equal to
// the byte offset.
func boundIndex(bounds []int, offset int) int {
	return sort.SearchInts(bounds, offset+1) - 1
}

// isCharEnd reports whether the 1-based byte position ends a character.
func isCharEnd(bounds []int, pos, length int) bool {
	if pos == length {
		return true
	}
	i := sort.SearchInts(bounds, pos)
	return i < len(bounds) && bounds[i] == pos
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
