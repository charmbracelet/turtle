// Package shuf implements the GNU shuf utility.
//
// It writes a random permutation of its input lines, which come from the
// standard input, a file operand, the command line operands with -e, or a
// numeric range with -i. There's no POSIX specification for it.
package shuf

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/output"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/textutil"
)

// Prog is the shuf core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes a random permutation of the input lines to the standard output
// or to the file given with -o. With -r and no -n it repeats forever,
// stopping when ctx is done, which returns the context error. The
// --random-source file is read as its bytes are needed, and running out of
// them fails, as GNU does. The exit status is 1 on errors.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "shuf", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "shuf")
		return &coreutils.ExitError{Code: 1}
	}
	return p.run(ctx, opts, operands)
}

// run checks the usage of the operands, gathers the input lines and writes
// the output.
func (p *Prog) run(ctx context.Context, opts options, operands []string) error {
	if err := p.checkOperands(opts, operands); err != nil {
		return err
	}
	g, err := p.newRNG(ctx, opts)
	if err != nil {
		return err
	}
	defer g.Close()

	count := noCount
	if opts.hasCount {
		count = opts.count
	}
	var lines []string
	var lo, n uint64
	if count != 0 {
		// Like GNU, -n 0 doesn't read the input at all, so a missing
		// file isn't reported.
		switch {
		case opts.echo:
			lines = operands
			n = uint64(len(operands))
		case opts.inputRange:
			lo = opts.lo
			n = opts.hi - opts.lo + 1
		default:
			lines, err = p.readLines(ctx, opts, operands)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				p.errorf("%s: %v", inputLabel(operands), patherr.Unwrap(err))
				return &coreutils.ExitError{Code: 1, Err: err}
			}
			n = uint64(len(lines))
		}
	}
	return p.write(ctx, g, opts, count, lines, lo, n)
}

// checkOperands reports the invalid uses of the operands: -e with -i, a
// range with an operand, and more than one file operand.
func (p *Prog) checkOperands(opts options, operands []string) error {
	if opts.echo && opts.inputRange {
		return p.usageErrorf("cannot combine -e and -i options")
	}
	if opts.inputRange {
		if len(operands) > 0 {
			return p.usageErrorf("extra operand '%s'", operands[0])
		}
	} else if !opts.echo && len(operands) > 1 {
		return p.usageErrorf("extra operand '%s'", operands[1])
	}
	return nil
}

// newRNG builds the source of random bytes: the --random-source file when
// one is given, or the system's entropy. Reads from the source stop early
// when ctx is done.
func (p *Prog) newRNG(ctx context.Context, opts options) (*rng, error) {
	if !opts.hasRandomSource {
		return newRNG(&ctxReader{ctx: ctx, r: rand.Reader}, "getrandom", nil), nil
	}
	if p.FS == nil {
		p.errorf("%s: no file system available", opts.randomSource)
		return nil, &coreutils.ExitError{Code: 1}
	}
	f, err := p.FS.Open(p.FsName(opts.randomSource))
	if err != nil {
		p.errorf("%s: %v", opts.randomSource, patherr.Unwrap(err))
		return nil, &coreutils.ExitError{Code: 1, Err: err}
	}
	return newRNG(&ctxReader{ctx: ctx, r: f}, opts.randomSource, f), nil
}

// readLines reads every line of the input, which is the standard input or
// the one file operand. A missing final delimiter is accepted, as GNU does.
func (p *Prog) readLines(ctx context.Context, opts options, operands []string) ([]string, error) {
	in := input.Input{Stdin: true}
	if len(operands) == 1 {
		in = input.New(operands[0])
	}
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	delim := textutil.Delim(opts.zeroTerminated)
	var lines []string
	err = textutil.Each(ctx, r, delim, func(rec []byte) error {
		lines = append(lines, string(bytes.TrimSuffix(rec, []byte{delim})))
		return nil
	})
	return lines, err
}

// write reports the emptiness of a repeated input, opens the output file
// after the input was read, so a file can be shuffled in place, and writes.
func (p *Prog) write(ctx context.Context, g *rng, opts options, count uint64, lines []string, lo, n uint64) error {
	if opts.repeat && count != 0 && n == 0 {
		p.errorf("no lines to repeat")
		return &coreutils.ExitError{Code: 1}
	}

	out := io.Writer(p.Stdout)
	if opts.hasOutput {
		f, err := output.Create(p.Runtime, ctx, opts.output)
		if err != nil {
			p.errorf("%s: %v", opts.output, patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 1, Err: err}
		}
		defer f.Close()
		out = f
	}
	w := bufio.NewWriter(out)

	var err error
	if opts.repeat {
		err = p.writeRepeated(ctx, w, g, opts, count, lines, lo, n)
	} else {
		err = p.writePermuted(ctx, w, g, opts, count, lines, lo, n)
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if srcErr, ok := errors.AsType[*sourceError](err); ok {
			p.errorf("%s: %v", srcErr.label, srcErr.err)
			return &coreutils.ExitError{Code: 1, Err: err}
		}
		if errors.Is(err, errTooManyItems) {
			p.errorf("%v", err)
			return &coreutils.ExitError{Code: 1, Err: err}
		}
		p.errorf("write error: %v", patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	if err := w.Flush(); err != nil {
		p.errorf("write error: %v", patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// writePermuted writes the first count lines of a random permutation of the
// input.
func (p *Prog) writePermuted(ctx context.Context, w *bufio.Writer, g *rng, opts options, count uint64, lines []string, lo, n uint64) error {
	delim := textutil.Delim(opts.zeroTerminated)
	if opts.inputRange {
		idx, err := sampleIndices(ctx, g, n, min(count, n))
		if err != nil {
			return err
		}
		for _, i := range idx {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "%d%c", lo+i, delim); err != nil {
				return err
			}
		}
		return nil
	}
	if err := shuffle(ctx, g, lines); err != nil {
		return err
	}
	for i, line := range lines {
		if uint64(i) >= count {
			break
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeItem(w, line, delim); err != nil {
			return err
		}
	}
	return nil
}

// writeRepeated writes count lines chosen at random from the whole input,
// with replacement, or forever when count is unbounded.
func (p *Prog) writeRepeated(ctx context.Context, w *bufio.Writer, g *rng, opts options, count uint64, lines []string, lo, n uint64) error {
	delim := textutil.Delim(opts.zeroTerminated)
	for range count {
		if err := ctx.Err(); err != nil {
			return err
		}
		j, err := g.below(n)
		if err != nil {
			return err
		}
		if opts.inputRange {
			if _, err := fmt.Fprintf(w, "%d%c", lo+j, delim); err != nil {
				return err
			}
			continue
		}
		if err := writeItem(w, lines[j], delim); err != nil {
			return err
		}
	}
	return nil
}

// writeItem writes one output item and its delimiter.
func writeItem(w *bufio.Writer, item string, delim byte) error {
	if _, err := w.WriteString(item); err != nil {
		return err
	}
	return w.WriteByte(delim)
}

// usageErrorf reports a usage error, followed by the hint, and exits with
// status 1.
func (p *Prog) usageErrorf(format string, args ...any) error {
	p.errorf(format, args...)
	stdopts.TryHelp(p.Runtime, "shuf")
	return &coreutils.ExitError{Code: 1}
}

// errorf writes a diagnostic message to the standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "shuf: "+format+"\n", args...)
}

// inputLabel returns the name of the input for diagnostics.
func inputLabel(operands []string) string {
	if len(operands) == 0 {
		return "standard input"
	}
	return operands[0]
}

// noCount is the unbounded head count, the default.
const noCount = uint64(math.MaxUint64)

// options holds the parsed command line options.
type options struct {
	echo            bool
	inputRange      bool
	lo, hi          uint64
	hasCount        bool
	count           uint64
	hasOutput       bool
	output          string
	hasRandomSource bool
	randomSource    string
	repeat          bool
	zeroTerminated  bool
}

// randomOpt is the pseudo-option that stands for --random-source, which has
// no short form.
const randomOpt rune = 0

// longOptions maps the long options of shuf to the short ones they stand for.
var longOptions = map[string]rune{
	"echo":            'e',
	"input-range":     'i',
	"head-count":      'n',
	"output":          'o',
	"random-source":   randomOpt,
	"repeat":          'r',
	"zero-terminated": 'z',
}

// optSet describes the options of shuf.
var optSet = getopt.Set{Valued: "ino" + string(randomOpt), Long: longOptions}

// usage describes the options of shuf.
const usage = `Usage: shuf [OPTION]... [FILE]
  or:  shuf -e [OPTION]... [ARG]...
  or:  shuf -i LO-HI [OPTION]...
Write a random permutation of the input lines to standard output.

With no FILE, or when FILE is -, read standard input.

  -e, --echo
         treat each ARG as an input line
  -i, --input-range=LO-HI
         treat each number LO through HI as an input line
  -n, --head-count=COUNT
         output at most COUNT lines
  -o, --output=FILE
         write result to FILE instead of standard output
      --random-source=FILE
         get random bytes from FILE
  -r, --repeat
         output lines can be repeated
  -z, --zero-terminated
         delimit items with a zero byte, not a newline
      --help
         display this help and exit
      --version
         output version information and exit
`

// delim returns the byte that terminates an item: a zero byte with -z, and a
// newline otherwise.
func (opts options) delim() byte {
	if opts.zeroTerminated {
		return 0
	}
	return '\n'
}

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'e':
			opts.echo = true
		case 'i':
			if opts.inputRange {
				return errors.New("multiple -i options specified")
			}
			lo, hi, err := parseRange(value)
			if err != nil {
				return err
			}
			opts.inputRange = true
			opts.lo, opts.hi = lo, hi
		case 'n':
			count, err := parseCount(value)
			if err != nil {
				return err
			}
			// Like GNU, several -n options keep the smallest count.
			if opts.hasCount {
				opts.count = min(opts.count, count)
			} else {
				opts.count = count
			}
			opts.hasCount = true
		case 'o':
			if opts.hasOutput && opts.output != value {
				return errors.New("multiple output files specified")
			}
			opts.output, opts.hasOutput = value, true
		case 'r':
			opts.repeat = true
		case 'z':
			opts.zeroTerminated = true
		case randomOpt:
			if opts.hasRandomSource && opts.randomSource != value {
				return errors.New("multiple random sources specified")
			}
			opts.randomSource, opts.hasRandomSource = value, true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	return opts, operands, nil
}

// parseRange parses a -i value, which must be two unsigned decimal integers
// separated by a dash, with LO at most HI.
func parseRange(value string) (uint64, uint64, error) {
	lo, hi, ok := strings.Cut(value, "-")
	if !ok {
		return 0, 0, fmt.Errorf("invalid input range '%s'", value)
	}
	l, err := strconv.ParseUint(lo, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid input range '%s'", value)
	}
	h, err := strconv.ParseUint(hi, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid input range '%s'", value)
	}
	if l > h || h-l == math.MaxUint64 {
		return 0, 0, fmt.Errorf("invalid input range '%s'", value)
	}
	return l, h, nil
}

// parseCount parses a -n value, accepting a value too large to represent, as
// GNU does, since it means no limit.
func parseCount(value string) (uint64, error) {
	count, err := strconv.ParseUint(value, 10, 64)
	if errors.Is(err, strconv.ErrRange) {
		return noCount, nil
	}
	if err != nil {
		return 0, fmt.Errorf("invalid line count: '%s'", value)
	}
	return count, nil
}

var _ coreutils.Program = (*Prog)(nil)
