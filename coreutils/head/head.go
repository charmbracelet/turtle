// Package head implements the POSIX head utility.
//
// It copies the first lines or bytes of each given file to the standard
// output, defaulting to the first 10 lines. The standard input is used when
// no operands are given, and also when an operand is "-". A negative count
// copies all but the last so many lines or bytes instead.
package head

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/sizeparse"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the head core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// headerMode says when the "==> name <==" headers are printed.
type headerMode int

const (
	headerMultiple headerMode = iota // only with multiple operands
	headerNever                      // -q
	headerAlways                     // -v
)

// Run writes the first lines or bytes of the given files to the standard
// output, in the same order the operands are given. The standard input is
// used when no operands are given, and also when an operand is "-".
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "head", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}

	inputs := make([]input.Input, 0, len(operands))
	for _, operand := range operands {
		inputs = append(inputs, input.New(operand))
	}
	if len(inputs) == 0 {
		inputs = append(inputs, input.Input{Stdin: true})
	}

	header := opts.headers(len(inputs) > 1)
	first := true

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
		r, err := input.Open(p.Runtime, ctx, in)
		if err != nil {
			p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
			ok = false
			continue
		}
		if header {
			p.writeHeader(in, !first)
			first = false
		}
		err = p.copy(r, opts)
		r.Close()
		if err == nil {
			continue
		}
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
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// copy copies the part of the input selected by the options: the first count
// lines or bytes, or all but the last count of them with a negative count.
func (p *Prog) copy(r io.Reader, opts options) error {
	if opts.elide {
		if opts.lines {
			return p.elideLines(r, opts.count, opts.delim())
		}
		return p.elideBytes(r, opts.count)
	}
	if opts.lines {
		return p.headLines(r, opts.count, opts.delim())
	}
	return p.headBytes(r, opts.count)
}

// headLines copies the first count items separated by delim. A final item
// without a trailing delimiter counts as an item and is copied as is.
func (p *Prog) headLines(r io.Reader, count int64, delim byte) error {
	br := bufio.NewReader(r)
	for range count {
		line, err := br.ReadBytes(delim)
		if len(line) > 0 {
			if _, werr := p.Stdout.Write(line); werr != nil {
				return &writeError{err: werr}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
	return nil
}

// headBytes copies the first count bytes.
func (p *Prog) headBytes(r io.Reader, count int64) error {
	_, err := io.CopyN(p.Stdout, r, count)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// elideLines writes all but the last count items separated by delim. A final
// item without a trailing delimiter counts as an item.
func (p *Prog) elideLines(r io.Reader, count int64, delim byte) error {
	if count <= 0 {
		return p.copyAll(r)
	}
	br := bufio.NewReader(r)
	var last [][]byte
	for {
		line, err := br.ReadBytes(delim)
		if len(line) > 0 {
			last = append(last, line)
			if int64(len(last)) > count {
				if _, werr := p.Stdout.Write(last[0]); werr != nil {
					return &writeError{err: werr}
				}
				last = append(last[:0], last[1:]...)
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

// elideBytes writes all but the last count bytes, keeping a window of the
// most recent ones and only writing what falls out of it.
func (p *Prog) elideBytes(r io.Reader, count int64) error {
	if count <= 0 {
		return p.copyAll(r)
	}
	const chunk = 32 * 1024
	buf := make([]byte, 0, min(count, chunk))
	tmp := make([]byte, chunk)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if int64(len(buf)) > count {
			cut := int64(len(buf)) - count
			if _, werr := p.Stdout.Write(buf[:cut]); werr != nil {
				return &writeError{err: werr}
			}
			buf = append(buf[:0], buf[cut:]...)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// copyAll copies the whole input to the standard output.
func (p *Prog) copyAll(r io.Reader) error {
	_, err := io.Copy(p.Stdout, r)
	if err != nil {
		return err
	}
	return nil
}

// writeHeader writes the header that precedes each file when there are
// multiple operands. The first header has no leading <newline>.
func (p *Prog) writeHeader(in input.Input, leadingNewline bool) {
	if leadingNewline {
		fmt.Fprint(p.Stdout, "\n")
	}
	fmt.Fprintf(p.Stdout, "==> %s <==\n", in.Label())
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "head: "+format+"\n", args...)
}

// options holds how much of each input is copied and whether the headers
// are printed.
type options struct {
	lines bool // -n instead of -c
	count int64
	elide bool // a negative count: all but the last count items
	zero  bool // -z: items terminated with a NUL byte

	countSet bool
	header   headerMode
}

// delim returns the byte separating the items counted by -n.
func (o options) delim() byte {
	if o.zero {
		return 0
	}
	return '\n'
}

// headers reports whether the "==> name <==" headers are printed for the
// operands given: never with -q, always with -v, otherwise only with
// multiple operands.
func (o options) headers(multiple bool) bool {
	switch o.header {
	case headerNever:
		return false
	case headerAlways:
		return true
	default:
		return multiple
	}
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

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)

// parseFlags parses the options and operands, including the obsolete option
// syntax, which is recognized before the options below. The -c and -n
// options take a number option-argument, and the last one given wins. Option
// parsing stops at the first operand or at "--".
//
// longOptions maps the long options of head to the short ones they stand
// for.
var longOptions = map[string]rune{
	"bytes":           'c',
	"lines":           'n',
	"quiet":           'q',
	"silent":          'q',
	"verbose":         'v',
	"zero-terminated": 'z',
}

// optSet describes the options of head.
var optSet = getopt.Set{Valued: "cn", Long: longOptions}

// usage describes the options of head.
const usage = `Usage: head [OPTION]... [FILE]...
Print the first 10 lines of each FILE to standard output.
With more than one FILE, precede each with a header giving the file name.

With no FILE, or when FILE is -, read standard input.

  -c, --bytes=[-]NUM     print the first NUM bytes of each file; with a
                         leading '-', print all but the last NUM bytes of
                         each file
  -n, --lines=[-]NUM     print the first NUM lines instead of the first 10;
                         with a leading '-', print all but the last NUM lines
                         of each file
  -q, --quiet, --silent  never print headers giving file names
  -v, --verbose          always print headers giving file names
  -z, --zero-terminated  line delimiter is NUL, not newline
      --help     display this help and exit
      --version  output version information and exit
`

func parseFlags(args []string) (options, []string, error) {
	var opts options
	args, err := parseObsolete(args, &opts)
	if err != nil {
		return options{}, nil, err
	}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'q':
			opts.header = headerNever
		case 'v':
			opts.header = headerAlways
		case 'z':
			opts.zero = true
		case 'c', 'n':
			count, elide, err := parseCount(value, opt == 'n')
			if err != nil {
				return err
			}
			opts.lines = opt == 'n'
			opts.count = count
			opts.elide = elide
			opts.countSet = true
		default:
			if isDigit(byte(opt)) {
				return fmt.Errorf("invalid trailing option -- %c", opt)
			}
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	if !opts.countSet {
		opts.lines = true
		opts.count = 10
	}
	return opts, operands, nil
}

// parseObsolete parses the obsolete option syntax "-[num][bkm][cqv]", like
// in "-5" for the first five lines or "-5bqv" for the first 2560 bytes with
// no headers. It is recognized only when given as the first argument, before
// the options parsed above, and returns the arguments without it. A plain
// number counts lines, like -n.
//
// The number is a decimal integer optionally followed by a size letter
// standing for a multiplier as in -c, and the option letters set the same
// options as their normal counterparts.
func parseObsolete(args []string, opts *options) ([]string, error) {
	if len(args) == 0 {
		return args, nil
	}
	arg := args[0]
	if len(arg) < 2 || arg[0] != '-' || !isDigit(arg[1]) {
		return args, nil
	}
	digits := arg[1:]
	letters := digits
	for letters != "" && isDigit(letters[0]) {
		letters = letters[1:]
	}
	digits = digits[:len(digits)-len(letters)]

	lines := true
	var mult byte
	for i := range letters {
		switch c := letters[i]; c {
		case 'c':
			lines = false
			mult = 0
		case 'b', 'k', 'm':
			lines = false
			mult = c
		case 'l':
			lines = true
		case 'q':
			opts.header = headerNever
		case 'v':
			opts.header = headerAlways
		case 'z':
			opts.zero = true
		default:
			return nil, fmt.Errorf("invalid trailing option -- %c", c)
		}
	}

	value := digits
	if mult != 0 {
		value += string(mult)
	}
	count, err := sizeparse.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("invalid number of %s: '%s'", unit(lines), value)
	}
	opts.lines = lines
	opts.count = count
	opts.countSet = true
	return args[1:], nil
}

// parseCount parses the number option-argument of -c and -n: a decimal
// integer optionally followed by a multiplier suffix, and with an optional
// sign. A leading '-' elides the last count items from the end of the input
// instead of copying the first count; a leading '+' is accepted and has no
// effect. lines selects the unit of the diagnostic message.
func parseCount(value string, lines bool) (count int64, elide bool, err error) {
	n, neg, err := sizeparse.ParseNumber(value)
	if err != nil {
		return 0, false, fmt.Errorf("invalid number of %s: '%s'", unit(lines), trimSign(value))
	}
	return n, neg, nil
}

// trimSign removes a leading '+' or '-' from value, for the diagnostic
// message which GNU reports without the sign.
func trimSign(value string) string {
	if value != "" && (value[0] == '+' || value[0] == '-') {
		return value[1:]
	}
	return value
}

// unit returns "lines" or "bytes", for the diagnostic message.
func unit(lines bool) string {
	if lines {
		return "lines"
	}
	return "bytes"
}

func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}
