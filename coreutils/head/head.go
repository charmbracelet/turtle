// Package head implements the POSIX head utility.
//
// It copies the first lines or bytes of each given file to the standard
// output, defaulting to the first 10 lines. The standard input is used when
// no operands are given, and also when an operand is "-".
package head

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strconv"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
)

// Prog is the head core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the first lines or bytes of the given files to the standard
// output, in the same order the operands are given. The standard input is
// used when no operands are given, and also when an operand is "-".
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
		inputs = append(inputs, newInput(operand))
	}
	if len(inputs) == 0 {
		inputs = append(inputs, input{stdin: true})
	}

	// A header precedes the output of each file, but only when there are
	// multiple operands, and only for files that are opened successfully.
	header := len(inputs) > 1
	first := true

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
		r, err := p.open(ctx, in)
		if err != nil {
			p.errorf("%s: %v", in.label(), unwrapPathError(err))
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
			p.errorf("%s: %v", in.label(), unwrapPathError(err))
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// copy copies the first opts.count lines or bytes of the input to the
// standard output.
func (p *Prog) copy(r io.Reader, opts options) error {
	if opts.lines {
		return p.headLines(r, opts.count)
	}
	return p.headBytes(r, opts.count)
}

// headLines copies the first count lines. A final line without a trailing
// <newline> counts as a line and is copied as is.
func (p *Prog) headLines(r io.Reader, count int) error {
	br := bufio.NewReader(r)
	for range count {
		line, err := br.ReadBytes('\n')
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
func (p *Prog) headBytes(r io.Reader, count int) error {
	_, err := io.CopyN(p.Stdout, r, int64(count))
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// writeHeader writes the header that precedes each file when there are
// multiple operands. The first header has no leading <newline>.
func (p *Prog) writeHeader(in input, leadingNewline bool) {
	if leadingNewline {
		fmt.Fprint(p.Stdout, "\n")
	}
	fmt.Fprintf(p.Stdout, "==> %s <==\n", in.label())
}

// open returns a reader for the input. Reads stop early when ctx is done.
// The standard input isn't owned by the caller, so it's wrapped on a no-op
// closer.
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
	fmt.Fprintf(p.Stderr, "head: "+format+"\n", args...)
}

// parseFlags parses the options and operands. The -c and -n options take a
// number option-argument, and the last one given wins. Option parsing stops
// at the first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var (
		opts     options
		countSet bool
	)
	operands, err := getopt.Set{Valued: "cn"}.Parse(args, func(opt rune, value string) error {
		if opt != 'c' && opt != 'n' {
			return getopt.InvalidOption(opt)
		}
		count, err := parseCount(value)
		if err != nil {
			return err
		}
		opts = options{lines: opt == 'n', count: count}
		countSet = true
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	if !countSet {
		opts = options{lines: true, count: 10}
	}
	return opts, operands, nil
}

// parseCount parses a non-negative decimal integer.
func parseCount(value string) (int, error) {
	if value == "" || value[0] == '+' || value[0] == '-' {
		return 0, fmt.Errorf("invalid number: '%s'", value)
	}
	count, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid number: '%s'", value)
	}
	return count, nil
}

// options holds how much of each input is copied.
type options struct {
	lines bool // -n instead of -c
	count int
}

// input describes an input operand: a pathname or the standard input.
type input struct {
	name  string
	stdin bool
}

// newInput maps an operand to an input, reading "-" as the standard input.
func newInput(operand string) input {
	return input{name: operand, stdin: operand == "-"}
}

// label returns the name to use on diagnostic messages.
func (in input) label() string {
	if in.stdin && in.name == "" {
		return "standard input"
	}
	return in.name
}

// ctxReadCloser stops read operations with the context error once ctx is done,
// so long inputs don't delay returning.
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

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
