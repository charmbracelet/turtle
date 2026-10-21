// Package cut implements the POSIX cut utility.
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
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
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
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}

	inputs := make([]input, 0, len(operands))
	for _, operand := range operands {
		inputs = append(inputs, input{name: operand, stdin: operand == "-"})
	}
	if len(inputs) == 0 {
		inputs = append(inputs, input{stdin: true})
	}

	ok := true
	for _, in := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !in.stdin && in.name == "" {
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
				p.errorf("%s: %v", in.label(), unwrapPathError(err))
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
func (p *Prog) cut(ctx context.Context, in input, opts options) error {
	r, err := p.open(ctx, in)
	if err != nil {
		return err
	}
	defer r.Close()

	br := bufio.NewReader(r)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			if cerr := p.cutLine([]byte(strings.TrimSuffix(line, "\n")), opts); cerr != nil {
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
	out = append(out, '\n')
	if _, err := p.Stdout.Write(out); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// open returns a reader for the input. Reads stop early when ctx is done. The
// standard input isn't owned by the caller, so it's wrapped on a no-op closer.
func (p *Prog) open(ctx context.Context, in input) (io.ReadCloser, error) {
	if in.stdin {
		if p.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return ctxReadCloser{ctx: ctx, rc: io.NopCloser(p.Stdin)}, nil
	}
	if p.FS == nil {
		return nil, errors.New("no file system available")
	}
	f, err := p.FS.Open(filepath.ToSlash(p.ResolvePath(in.name)))
	if err != nil {
		return nil, err
	}
	return ctxReadCloser{ctx: ctx, rc: f}, nil
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
	mode     mode
	spans    []span
	delim    byte
	noSplit  bool
	suppress bool
}

// longOptions maps the long options of cut to the short ones they stand for.
var longOptions = map[string]rune{
	"bytes":          'b',
	"characters":     'c',
	"delimiter":      'd',
	"fields":         'f',
	"only-delimited": 's',
}

// parseFlags parses the options and operands, checking that exactly one list
// option is given and that the other options combine with it. Option parsing
// stops at the first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{delim: '\t'}
	var (
		listSeen rune
		delimSet bool
		nSet     bool
		sSet     bool
	)
	operands, err := getopt.Set{Valued: "bcdf", Long: longOptions}.Parse(args, func(opt rune, value string) error {
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

// cutBytes returns the selected bytes of the line.
func (opts options) cutBytes(line []byte) []byte {
	spans := opts.spans
	if opts.noSplit {
		spans = charSpans(line, spans)
	}
	var out []byte
	for _, sp := range spans {
		high := sp.high
		if high == 0 || high > len(line) {
			high = len(line)
		}
		if sp.low <= high {
			out = append(out, line[sp.low-1:high]...)
		}
	}
	return out
}

// cutChars returns the selected characters of the line.
func (opts options) cutChars(line []byte) []byte {
	bounds := charBounds(line)
	var out []byte
	for _, sp := range opts.spans {
		high := sp.high
		if high == 0 || high > len(bounds) {
			high = len(bounds)
		}
		for i := sp.low; i <= high; i++ {
			end := len(line)
			if i < len(bounds) {
				end = bounds[i]
			}
			out = append(out, line[bounds[i-1]:end]...)
		}
	}
	return out
}

// cutFields returns the selected fields of the line, joined by the delimiter.
// Lines without a delimiter are returned as-is, unless -s suppresses them.
func (opts options) cutFields(line []byte) ([]byte, bool) {
	if !bytes.Contains(line, []byte{opts.delim}) {
		return line, !opts.suppress
	}
	fields := bytes.Split(line, []byte{opts.delim})
	var out []byte
	wrote := false
	for _, sp := range opts.spans {
		high := sp.high
		if high == 0 || high > len(fields) {
			high = len(fields)
		}
		for i := sp.low; i <= high; i++ {
			if wrote {
				out = append(out, opts.delim)
			}
			out = append(out, fields[i-1]...)
			wrote = true
		}
	}
	return out, true
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

// input describes an input operand: a pathname or the standard input.
type input struct {
	name  string
	stdin bool
}

// label returns the name to use on diagnostic messages.
func (in input) label() string {
	if in.stdin && in.name == "" {
		return "standard input"
	}
	return in.name
}

// ctxReadCloser stops read operations with the context error once ctx is
// done, so long inputs don't delay returning.
type ctxReadCloser struct {
	ctx context.Context
	rc  io.ReadCloser
}

// Read implements io.Reader.
func (c ctxReadCloser) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.rc.Read(p)
}

// Close implements io.Closer.
func (c ctxReadCloser) Close() error {
	return c.rc.Close()
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

// unwrapPathError unwraps *fs.PathError so diagnostics don't repeat the file
// name.
func unwrapPathError(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}

var _ coreutils.Program = (*Prog)(nil)
