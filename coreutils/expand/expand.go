// Package expand implements the POSIX expand utility.
//
// It converts the tabs of the given files to the spaces needed to reach the
// next tab stop, writing the result to the standard output. The standard
// input is used when no operands are given, and also when an operand is "-".
// With -i, the tabs after the leading blanks of a line are kept as tabs.
package expand

import (
	"bytes"
	"context"
	"errors"
	"unicode/utf8"

	"charm.land/turtle/coreutils"

	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/input"
	"charm.land/turtle/coreutils/internal/patherr"
	"charm.land/turtle/coreutils/internal/stdopts"
	"charm.land/turtle/coreutils/internal/tabutil"
	"charm.land/turtle/coreutils/internal/textutil"
)

// Prog is the expand core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run converts the tabs of the given files to spaces and writes the result
// to the standard output. The column position of each character is counted
// from the start of the line: backspaces decrement it, down to zero, and
// each other character counts as one column.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "expand", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "expand", err)
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
			stdopts.Errorf(p.Runtime, "expand", "invalid zero-length file name")
			ok = false
			continue
		}
		if err := p.expand(ctx, in, opts); err != nil {
			var writeErr *writeError
			switch {
			case errors.As(err, &writeErr):
				stdopts.Errorf(p.Runtime, "expand", "write error: %v", writeErr.err)
				return &coreutils.ExitError{Code: 1, Err: writeErr.err}
			case ctx.Err() != nil:
				return ctx.Err()
			default:
				stdopts.Errorf(p.Runtime, "expand", "%s: %v", in.Label(), patherr.Unwrap(err))
				ok = false
			}
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// expand converts the tabs of a single input to spaces.
func (p *Prog) expand(ctx context.Context, in input.Input, opts options) error {
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		return err
	}
	defer r.Close()

	nl := []byte{'\n'}
	return textutil.Each(ctx, r, '\n', func(rec []byte) error {
		return p.expandLine(bytes.TrimSuffix(rec, nl), opts, bytes.HasSuffix(rec, nl))
	})
}

// expandLine writes a line with its tabs replaced by the spaces needed to
// reach the next tab stop. A tab past the last stop of a list becomes a
// single space. With opts.initial, only the tabs of the leading blanks of
// the line are converted: the first character which is neither a space nor
// a tab, a backspace included, keeps the remaining tabs of the line as tabs.
func (p *Prog) expandLine(line []byte, opts options, terminated bool) error {
	out := make([]byte, 0, len(line))
	col := 0
	convert := true
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRune(line[i:])
		if opts.initial {
			convert = convert && (r == ' ' || r == '\t')
		}
		if r == '\t' && convert {
			next, ok := opts.stops.Next(col)
			if !ok {
				next = col + 1
			}
			for ; col < next; col++ {
				out = append(out, ' ')
			}
		} else {
			out = append(out, line[i:i+size]...)
			if convert {
				if r == '\b' {
					if col > 0 {
						col--
					}
				} else {
					col++
				}
			}
		}
		i += size
	}
	if terminated {
		out = append(out, '\n')
	}
	if _, err := p.Stdout.Write(out); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// options holds the parsed command line options.
type options struct {
	stops   tabutil.Stops
	initial bool
}

// longOptions maps the long options of expand to the short ones they stand
// for.
var longOptions = map[string]rune{
	"initial": 'i',
	"tabs":    't',
}

// optSet describes the options of expand.
var optSet = getopt.Set{Valued: "t", Long: longOptions}

// usage describes the options of expand.
const usage = `Usage: expand [OPTION]... [FILE]...
Convert tabs in each FILE to spaces, writing to standard output.
With no FILE, or when FILE is -, read standard input.

  -i, --initial
         do not convert tabs after non blanks
  -t, --tabs=N
         have tabs N characters apart, not 8
  -t, --tabs=LIST
         use comma separated LIST of tab positions
` + stdopts.Tail

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{stops: tabutil.Default}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'i':
			opts.initial = true
		case 't':
			parsed, err := tabutil.Parse(value)
			if err != nil {
				return err
			}
			opts.stops = parsed
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
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
