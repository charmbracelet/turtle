// Package comm implements the POSIX comm utility.
//
// It compares two sorted files line by line, writing the lines that are
// unique to each file and the lines present in both in up to three
// tab-separated columns. An operand of "-" means the standard input.
package comm

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"

	"charm.land/turtle/coreutils"

	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/input"
	"charm.land/turtle/coreutils/internal/patherr"
	"charm.land/turtle/coreutils/internal/stdopts"
	"charm.land/turtle/coreutils/internal/textutil"
)

// Prog is the comm core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run compares the sorted lines of two files, writing the columns selected by
// the options: lines unique to file1, lines unique to file2, and lines in
// both files.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "comm", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "comm", err)
	}
	if len(operands) < 2 {
		stdopts.Errorf(p.Runtime, "comm", "missing operand")
		stdopts.TryHelp(p.Runtime, "comm")
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) > 2 {
		stdopts.Errorf(p.Runtime, "comm", "extra operand '%s'", operands[2])
		stdopts.TryHelp(p.Runtime, "comm")
		return &coreutils.ExitError{Code: 2}
	}

	var ins [2]*file
	for i, name := range operands {
		in, err := p.openInput(ctx, name, opts.zero)
		if err != nil {
			stdopts.Errorf(p.Runtime, "comm", "%s: %v", name, patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 1}
		}
		ins[i] = in
		defer in.Close()
	}

	if err := p.comm(ctx, ins[0], ins[1], p.Stdout, opts); err != nil {
		var writeErr *writeError
		var readErr *input.ReadError
		switch {
		case errors.As(err, &writeErr):
			stdopts.Errorf(p.Runtime, "comm", "write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 1, Err: writeErr.err}
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.As(err, &readErr):
			stdopts.Errorf(p.Runtime, "comm", "%s: %v", readErr.Label, patherr.Unwrap(readErr.Err))
			return &coreutils.ExitError{Code: 1}
		default:
			return err
		}
	}
	return nil
}

// comm merges the lines of both inputs, writing the columns the options
// select to out.
func (p *Prog) comm(ctx context.Context, in1, in2 *file, out io.Writer, opts options) error {
	line1, ok1, err := in1.next()
	if err != nil {
		return err
	}
	line2, ok2, err := in2.next()
	if err != nil {
		return err
	}
	for ok1 || ok2 {
		if err := ctx.Err(); err != nil {
			return err
		}
		var order int
		switch {
		case !ok1:
			order = 1
		case !ok2:
			order = -1
		default:
			order = strings.Compare(line1, line2)
		}
		switch {
		case order == 0:
			if err := p.writeLine(out, line2, 3, opts); err != nil {
				return err
			}
			if line1, ok1, err = in1.next(); err != nil {
				return err
			}
			if line2, ok2, err = in2.next(); err != nil {
				return err
			}
		case order < 0:
			if err := p.writeLine(out, line1, 1, opts); err != nil {
				return err
			}
			if line1, ok1, err = in1.next(); err != nil {
				return err
			}
		default:
			if err := p.writeLine(out, line2, 2, opts); err != nil {
				return err
			}
			if line2, ok2, err = in2.next(); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeLine writes line, with the leading separators of its column, to out
// when the column isn't suppressed.
func (p *Prog) writeLine(out io.Writer, line string, column int, opts options) error {
	switch column {
	case 1:
		if opts.suppress1 {
			return nil
		}
	case 2:
		if opts.suppress2 {
			return nil
		}
		if !opts.suppress1 {
			line = opts.delimiter + line
		}
	case 3:
		if opts.suppress3 {
			return nil
		}
		if !opts.suppress1 {
			line = opts.delimiter + line
		}
		if !opts.suppress2 {
			line = opts.delimiter + line
		}
	}
	if _, err := io.WriteString(out, line); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// file is an input file read as items terminated by term, with its label for
// diagnostics.
type file struct {
	io.Closer
	reader *bufio.Reader
	label  string
	term   byte
}

// next reads the next item, keeping its terminator when it has one. It
// reports false once the input is exhausted.
func (in *file) next() (string, bool, error) {
	line, err := in.reader.ReadString(in.term)
	switch {
	case err == nil:
		return line, true, nil
	case errors.Is(err, io.EOF) && len(line) > 0:
		// A final item without its terminator: the output still ends
		// with one.
		return line + string(in.term), true, nil
	case errors.Is(err, io.EOF):
		return "", false, nil
	default:
		return "", false, &input.ReadError{Label: in.label, Err: err}
	}
}

// openInput returns an input for the given operand, read as items terminated
// by NUL when zero is set, or by a newline otherwise. Reads stop early when
// ctx is done. The standard input isn't owned by the caller, so it's wrapped
// on a no-op closer.
func (p *Prog) openInput(ctx context.Context, name string, zero bool) (*file, error) {
	rc, err := input.Open(p.Runtime, ctx, input.Input{Name: name, Stdin: name == "-"})
	if err != nil {
		return nil, err
	}
	term := textutil.Delim(zero)
	return &file{Closer: rc, reader: bufio.NewReader(rc), label: name, term: term}, nil
}

// optOutputDelimiter is the rune the long-only --output-delimiter option maps
// to. Having no short equivalent, it's kept above the bytes a group of short
// options can hold.
const optOutputDelimiter rune = 0x100

// longOptions maps the long options of comm to the short ones they stand for.
var longOptions = map[string]rune{
	"output-delimiter": optOutputDelimiter,
	"zero-terminated":  'z',
}

// optSet describes the options of comm.
var optSet = getopt.Set{
	Valued: string(optOutputDelimiter),
	Long:   longOptions,
}

// usage describes the options of comm.
const usage = `Usage: comm [OPTION]... FILE1 FILE2
Compare sorted files FILE1 and FILE2 line by line.

When FILE1 or FILE2 (not both) is -, read standard input.

  -1
         suppress column 1 (lines unique to FILE1)
  -2
         suppress column 2 (lines unique to FILE2)
  -3
         suppress column 3 (lines that appear in both)
      --output-delimiter=STR
         separate columns with STR
  -z, --zero-terminated
         line delimiter is NUL, not newline
` + stdopts.Tail

// options holds the parsed command line options. delimiter separates the
// columns, and an empty --output-delimiter value selects NUL.
type options struct {
	suppress1 bool
	suppress2 bool
	suppress3 bool
	delimiter string
	zero      bool
}

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{delimiter: "\t"}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case '1':
			opts.suppress1 = true
		case '2':
			opts.suppress2 = true
		case '3':
			opts.suppress3 = true
		case 'z':
			opts.zero = true
		case optOutputDelimiter:
			opts.delimiter = value
			if value == "" {
				opts.delimiter = "\x00"
			}
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

// writeError marks an error as coming from writing to the output.
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
