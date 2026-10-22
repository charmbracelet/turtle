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
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
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
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) < 2 {
		p.errorf("missing operand")
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) > 2 {
		p.errorf("extra operand '%s'", operands[2])
		return &coreutils.ExitError{Code: 2}
	}

	var ins [2]*input
	for i, name := range operands {
		in, err := p.openInput(ctx, name)
		if err != nil {
			p.errorf("%s: %v", name, unwrapPathError(err))
			return &coreutils.ExitError{Code: 1}
		}
		ins[i] = in
		defer in.Close()
	}

	if err := p.comm(ctx, ins[0], ins[1], p.Stdout, opts); err != nil {
		var writeErr *writeError
		var readErr *readError
		switch {
		case errors.As(err, &writeErr):
			p.errorf("write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 1, Err: writeErr.err}
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.As(err, &readErr):
			p.errorf("%s: %v", readErr.label, unwrapPathError(readErr.err))
			return &coreutils.ExitError{Code: 1}
		default:
			return err
		}
	}
	return nil
}

// comm merges the lines of both inputs, writing the columns the options
// select to out.
func (p *Prog) comm(ctx context.Context, in1, in2 *input, out io.Writer, opts options) error {
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
			line = "\t" + line
		}
	case 3:
		if opts.suppress3 {
			return nil
		}
		if !opts.suppress1 {
			line = "\t" + line
		}
		if !opts.suppress2 {
			line = "\t" + line
		}
	}
	if _, err := io.WriteString(out, line); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// input is a line-oriented input file with its label for diagnostics.
type input struct {
	io.Closer
	reader *bufio.Reader
	label  string
}

// next reads the next line, keeping its trailing newline when it has one. It
// reports false once the input is exhausted.
func (in *input) next() (string, bool, error) {
	line, err := in.reader.ReadString('\n')
	switch {
	case err == nil:
		return line, true, nil
	case errors.Is(err, io.EOF) && len(line) > 0:
		return line, true, nil
	case errors.Is(err, io.EOF):
		return "", false, nil
	default:
		return "", false, &readError{label: in.label, err: err}
	}
}

// openInput returns an input for the given operand. Reads stop early when ctx
// is done. The standard input isn't owned by the caller, so it's wrapped on a
// no-op closer.
func (p *Prog) openInput(ctx context.Context, name string) (*input, error) {
	if name == "-" {
		if p.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		rc := ctxReadCloser{ctx: ctx, rc: io.NopCloser(p.Stdin)}
		return &input{Closer: rc, reader: bufio.NewReader(rc), label: name}, nil
	}
	if p.FS == nil {
		return nil, errors.New("no file system available")
	}
	f, err := p.FS.Open(filepath.ToSlash(p.ResolvePath(name)))
	if err != nil {
		return nil, err
	}
	rc := ctxReadCloser{ctx: ctx, rc: f}
	return &input{Closer: rc, reader: bufio.NewReader(rc), label: name}, nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "comm: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	suppress1 bool
	suppress2 bool
	suppress3 bool
}

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := getopt.Set{}.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case '1':
			opts.suppress1 = true
		case '2':
			opts.suppress2 = true
		case '3':
			opts.suppress3 = true
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

// readError marks an error as coming from reading the input file label.
type readError struct {
	label string
	err   error
}

// Error implements the error interface.
func (e *readError) Error() string {
	return e.err.Error()
}

// Unwrap implements the interface to make readError unwrapable.
func (e *readError) Unwrap() error {
	return e.err
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

// unwrapPathError unwraps *fs.PathError so diagnostics don't repeat the file
// name.
func unwrapPathError(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}

var _ coreutils.Program = (*Prog)(nil)
