// Package cat implements the POSIX cat utility.
//
// It reads the given files in sequence and writes their contents to the
// standard output in the same sequence. The standard input is used when no
// operands are given, and also when an operand is "-".
package cat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
)

// Prog is the cat core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the contents of the given files to the standard output, in the
// same order the operands are given. The standard input is used when no
// operands are given, and also when an operand is "-".
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	operands, err := parseFlags(args)
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
		if err := p.cat(ctx, in); err != nil {
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

// cat writes the contents of a single input to the standard output.
func (p *Prog) cat(ctx context.Context, in input) error {
	r, err := p.open(ctx, in)
	if err != nil {
		return err
	}
	defer r.Close()

	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := p.Stdout.Write(buf[:n]); werr != nil {
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
	fmt.Fprintf(p.Stderr, "cat: "+format+"\n", args...)
}

// parseFlags parses the operands. The -u option is accepted and has no effect:
// output is written as each byte is read, which is the behavior -u asks for.
// Option parsing stops at the first operand or at "--".
func parseFlags(args []string) ([]string, error) {
	var (
		operands []string
		parsing  = true
	)
	for _, arg := range args {
		switch {
		case !parsing || arg == "-" || !strings.HasPrefix(arg, "-"):
			parsing = false
			operands = append(operands, arg)
		case arg == "--":
			parsing = false
		case strings.HasPrefix(arg, "--"):
			return nil, fmt.Errorf("unrecognized option '%s'", arg)
		default:
			for i := 1; i < len(arg); i++ {
				if arg[i] != 'u' {
					return nil, fmt.Errorf("invalid option -- '%c'", arg[i])
				}
			}
		}
	}
	return operands, nil
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
