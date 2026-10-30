// Package unexpand implements the POSIX unexpand utility.
//
// It converts the blank runs of the given files to tabs, writing the result
// to the standard output. Only leading blanks are converted, unless -a or -t
// are given, or --first-only cancels them. The standard input is used when no
// operands are given, and also when an operand is "-".
package unexpand

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/tabutil"
)

// Prog is the unexpand core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run converts the blanks of the given files to tabs and writes the result to
// the standard output. The column position of each character is counted from
// the start of the line: backspaces decrement it, down to zero, and each
// other character counts as one column.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "unexpand", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "unexpand")
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
		if err := p.unexpand(ctx, in, opts); err != nil {
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

// unexpand converts the blanks of a single input to tabs.
func (p *Prog) unexpand(ctx context.Context, in input.Input, opts options) error {
	r, err := input.Open(p.Runtime, ctx, in)
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
			terminated := strings.HasSuffix(line, "\n")
			if cerr := p.unexpandLine([]byte(strings.TrimSuffix(line, "\n")), opts, terminated); cerr != nil {
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

// unexpandLine writes a line with its convertible blank runs replaced by the
// maximum number of tabs followed by the minimum number of spaces needed to
// fill the same column positions. A leading run is always convertible; the
// other runs need opts.all and at least two blanks.
func (p *Prog) unexpandLine(line []byte, opts options, terminated bool) error {
	out := make([]byte, 0, len(line))
	col := 0
	leading := true
	i := 0
	for i < len(line) {
		r, size := utf8.DecodeRune(line[i:])
		if r != ' ' && r != '\t' {
			leading = false
			if r == '\b' {
				out = append(out, '\b')
				if col > 0 {
					col--
				}
			} else {
				out = append(out, line[i:i+size]...)
				col++
			}
			i += size
			continue
		}

		// Scan the run of blanks, ending it before a tab past the last stop
		// of a list: no conversions happen there, so it's copied as is.
		end, endCol, blanks := i, col, 0
	runScan:
		for end < len(line) {
			r, size := utf8.DecodeRune(line[end:])
			switch {
			case r == ' ':
				endCol++
			case r == '\t':
				next, ok := opts.stops.Next(endCol)
				if !ok {
					break runScan
				}
				endCol = next
			default:
				break runScan
			}
			blanks++
			end += size
		}
		if end == i {
			out = append(out, '\t')
			col++
			i += size
			leading = false
			continue
		}
		if leading || (opts.all && blanks >= 2) {
			col = unexpandRun(&out, opts.stops, col, endCol)
		} else {
			out = append(out, line[i:end]...)
			col = endCol
		}
		leading = false
		i = end
	}
	if terminated {
		out = append(out, '\n')
	}
	if _, err := p.Stdout.Write(out); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// unexpandRun appends a run of blanks filling the columns [from, to) to out,
// as the maximum number of tabs followed by the minimum number of spaces. It
// returns the final column.
func unexpandRun(out *[]byte, stops tabutil.Stops, from, to int) int {
	pos := from
	for {
		next, ok := stops.Next(pos)
		if !ok || next > to {
			break
		}
		*out = append(*out, '\t')
		pos = next
	}
	for ; pos < to; pos++ {
		*out = append(*out, ' ')
	}
	return pos
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "unexpand: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	stops     tabutil.Stops
	all       bool
	firstOnly bool
}

// firstOnlyOpt is the pseudo short option --first-only stands for, as it has
// no short form of its own. It's above the range a byte can hold, so no short
// option can ever stand for it.
const firstOnlyOpt rune = 0x100

// longOptions maps the long options of unexpand to the short ones they stand
// for.
var longOptions = map[string]rune{
	"all":        'a',
	"tabs":       't',
	"first-only": firstOnlyOpt,
}

// optSet describes the options of unexpand.
var optSet = getopt.Set{Valued: "t", Long: longOptions}

// usage describes the options of unexpand.
const usage = `Usage: unexpand [OPTION]... [FILE]...
Convert blanks in each FILE to tabs, writing to standard output.
With no FILE, or when FILE is -, read standard input.

  -a, --all           convert all blanks, not just initial blanks
      --first-only    convert only leading sequences of blanks (overrides -a)
  -t, --tabs=N        have tabs N characters apart, not 8
  -t, --tabs=LIST     use comma separated LIST of tab positions
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags parses the command line options. Giving -t implies -a, as the
// conversions aren't limited to the leading blanks anymore, unless
// --first-only cancels it. Option parsing stops at the first operand or at
// "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{stops: tabutil.Default}
	tabsSet := false
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
			opts.all = true
		case 't':
			stops, err := tabutil.Parse(value)
			if err != nil {
				return err
			}
			opts.stops = stops
			tabsSet = true
		case firstOnlyOpt:
			opts.firstOnly = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	if tabsSet {
		opts.all = true
	}
	if opts.firstOnly {
		opts.all = false
	}
	return opts, operands, nil
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
