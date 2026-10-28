// Package expand implements the POSIX expand utility.
//
// It converts the tabs of the given files to the spaces needed to reach the
// next tab stop, writing the result to the standard output. The standard
// input is used when no operands are given, and also when an operand is "-".
package expand

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/tabutil"
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
	stops, operands, err := parseFlags(args)
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
		if err := p.expand(ctx, in, stops); err != nil {
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

// expand converts the tabs of a single input to spaces.
func (p *Prog) expand(ctx context.Context, in input, stops tabutil.Stops) error {
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
			terminated := strings.HasSuffix(line, "\n")
			if cerr := p.expandLine([]byte(strings.TrimSuffix(line, "\n")), stops, terminated); cerr != nil {
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

// expandLine writes a line with its tabs replaced by the spaces needed to
// reach the next tab stop. A tab past the last stop of a list becomes a
// single space.
func (p *Prog) expandLine(line []byte, stops tabutil.Stops, terminated bool) error {
	out := make([]byte, 0, len(line))
	col := 0
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRune(line[i:])
		switch r {
		case '\t':
			next, ok := stops.Next(col)
			if !ok {
				next = col + 1
			}
			for ; col < next; col++ {
				out = append(out, ' ')
			}
		case '\b':
			out = append(out, '\b')
			if col > 0 {
				col--
			}
		default:
			out = append(out, line[i:i+size]...)
			col++
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

// open returns a reader for the input. Reads stop early when ctx is done. The
// standard input isn't owned by the caller, so it's wrapped on a no-op closer.
func (p *Prog) open(ctx context.Context, in input) (io.ReadCloser, error) {
	if in.stdin {
		if p.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return ctxio.NewReader(ctx, p.Stdin), nil
	}
	if p.FS == nil {
		return nil, errors.New("no file system available")
	}
	f, err := p.FS.Open(filepath.ToSlash(p.ResolvePath(in.name)))
	if err != nil {
		return nil, err
	}
	return ctxio.ReadCloser{Ctx: ctx, RC: f}, nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "expand: "+format+"\n", args...)
}

// longOptions maps the long options of expand to the short ones they stand
// for.
var longOptions = map[string]rune{
	"tabs": 't',
}

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (tabutil.Stops, []string, error) {
	stops := tabutil.Default
	operands, err := getopt.Set{Valued: "t", Long: longOptions}.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 't':
			parsed, err := tabutil.Parse(value)
			if err != nil {
				return err
			}
			stops = parsed
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return stops, operands, err
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
