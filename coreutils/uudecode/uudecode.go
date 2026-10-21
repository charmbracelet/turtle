// Package uudecode implements the POSIX uudecode utility.
//
// It scans the input for the output of uuencode, in the historical format or
// in the base64 one, and recreates the file it holds: on the pathname of the
// "begin" header, or on the standard output when the pathname is "-" or
// "/dev/stdout", or when -o says so.
package uudecode

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/chmod"
	"charm.land/snail/coreutils/internal/basex"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/uux"
)

// Prog is the uudecode core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run decodes the uuencode output held by the input file, recreating the
// file it encodes. The standard input is used when no operand is given, and
// also when the operand is "-".
//
// Reading is interrupted when ctx is done, and the context error is
// returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	outfile, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) > 1 {
		p.errorf("extra operand '%s'", operands[1])
		return &coreutils.ExitError{Code: 2}
	}

	in := newInput(operands)
	if !in.stdin && in.name == "" {
		p.errorf("invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}
	r, err := p.open(in)
	if err != nil {
		p.errorf("%s: %v", in.label(), unwrapPathError(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	defer r.Close()

	return p.decode(ctx, bufio.NewReader(r), in, outfile)
}

// decode scans the input for an uuencode output and recreates the file it
// holds.
func (p *Prog) decode(ctx context.Context, br *bufio.Reader, in input, outfile string) error {
	header, err := scanHeader(ctx, br)
	switch {
	case err == nil:
	case errors.Is(err, errNoBegin):
		p.errorf("%s: no 'begin' line", in.label())
		return &coreutils.ExitError{Code: 1, Err: err}
	case ctx.Err() != nil:
		return ctx.Err()
	default:
		p.errorf("%s: %v", in.label(), unwrapPathError(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}

	spec, err := chmod.ParseMode(header.Mode)
	if err != nil {
		p.errorf("%s: %v", in.label(), err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	mode := spec(0, false, 0)

	name := header.Name
	if outfile != "" {
		name = outfile
	}

	w, done, err := p.output(name, mode)
	if err != nil {
		p.errorf("%s: %v", name, unwrapPathError(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	err = p.decodeBody(ctx, br, w, header.Base64)
	if cerr := done(); err == nil {
		err = cerr
	}
	switch {
	case w.err != nil:
		p.errorf("%s: %v", name, w.err)
		return &coreutils.ExitError{Code: 1, Err: w.err}
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, errInvalidInput), errors.Is(err, basex.ErrInvalidInput), errors.Is(err, io.ErrUnexpectedEOF):
		p.errorf("%s: invalid input", in.label())
		return &coreutils.ExitError{Code: 1}
	default:
		p.errorf("%s: %v", in.label(), unwrapPathError(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
}

// decodeBody decodes the body which follows the header: lines of the
// historical encoding up to the terminator, or base64 up to a line holding
// "====". Empty lines are invalid, as is an "end" line without a terminator
// before it: POSIX requires the zero-length line first.
func (p *Prog) decodeBody(ctx context.Context, br *bufio.Reader, w io.Writer, base64 bool) error {
	if base64 {
		return basex.Base64.Decode(ctx, w, &bodyReader{br: br}, true)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := br.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		trimmed := trimLine(line)
		if trimmed == "" || trimmed == "end" {
			// POSIX requires a line holding only a space or backquote
			// before the end line, and no empty lines in between.
			return errInvalidInput
		}
		data, last, derr := uux.DecodeLine([]byte(trimmed))
		if derr != nil {
			return errInvalidInput
		}
		if len(data) > 0 {
			if _, werr := w.Write(data); werr != nil {
				return werr
			}
		}
		if last {
			return nil
		}
	}
}

// scanHeader reads lines from br until the "begin" line of an uuencode
// output shows up, skipping over any leading text.
func scanHeader(ctx context.Context, br *bufio.Reader) (uux.Header, error) {
	for {
		if err := ctx.Err(); err != nil {
			return uux.Header{}, err
		}
		line, err := br.ReadString('\n')
		if header, ok := uux.ParseHeader(trimLine(line)); ok {
			return header, nil
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if cerr := ctx.Err(); cerr != nil {
					return uux.Header{}, cerr
				}
				return uux.Header{}, errNoBegin
			}
			return uux.Header{}, err
		}
	}
}

// output returns the writer the decoded file is written to: the standard
// output when the pathname is one of the magic cookies, or a recreated file
// otherwise. The returned func closes the file, setting its mode bits, which
// POSIX demands not to be treated as an error when it fails.
func (p *Prog) output(name string, mode fs.FileMode) (*errWriter, func() error, error) {
	if name == "-" || name == "/dev/stdout" {
		if p.Stdout == nil {
			return nil, nil, errors.New("no standard output available")
		}
		return &errWriter{w: p.Stdout}, func() error { return nil }, nil
	}
	if p.FS == nil {
		return nil, nil, errors.New("no file system available")
	}
	path := filepath.ToSlash(p.ResolvePath(name))
	f, err := p.FS.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return nil, nil, err
	}
	done := func() error {
		err := f.Close()
		if err == nil {
			_ = p.FS.Chmod(path, mode)
		}
		return err
	}
	return &errWriter{w: f}, done, nil
}

// open returns a reader for the input.
func (p *Prog) open(in input) (io.ReadCloser, error) {
	if in.stdin {
		if p.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return io.NopCloser(p.Stdin), nil
	}
	if p.FS == nil {
		return nil, errors.New("no file system available")
	}
	return p.FS.Open(filepath.ToSlash(p.ResolvePath(in.name)))
}

// errorf writes a diagnostic message to the standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "uudecode: "+format+"\n", args...)
}

// errNoBegin reports an input which ends before any "begin" line.
var errNoBegin = errors.New("no 'begin' line")

// errInvalidInput reports a body line that can't be decoded.
var errInvalidInput = errors.New("invalid input")

// bodyReader reads the lines of a base64 body from br, ending the stream at
// a line holding "====". An end of the input before the terminator is
// reported as an unexpected EOF.
type bodyReader struct {
	br   *bufio.Reader
	line string
	done bool
}

// Read implements io.Reader.
func (r *bodyReader) Read(p []byte) (int, error) {
	for len(r.line) == 0 {
		if r.done {
			return 0, io.EOF
		}
		line, err := r.br.ReadString('\n')
		if trimLine(line) == "====" {
			r.done = true
			continue
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0, io.ErrUnexpectedEOF
			}
			return 0, err
		}
		r.line = line
	}
	n := copy(p, r.line)
	r.line = r.line[n:]
	return n, nil
}

// trimLine drops the newline which ends a line, accepting the CRLF ending
// of Windows too.
func trimLine(line string) string {
	return strings.TrimRight(line, "\r\n")
}

// longOptions maps the long options of uudecode to the short ones they
// stand for.
var longOptions = map[string]rune{
	"output-file": 'o',
}

// parseFlags parses the options and operands. The -o option takes a pathname
// option-argument, either attached or in the next argument, and the last one
// given wins. Option parsing stops at the first operand or at "--".
func parseFlags(args []string) (outfile string, operands []string, err error) {
	operands, err = getopt.Set{Valued: "o", Long: longOptions}.Parse(args, func(opt rune, value string) error {
		if opt != 'o' {
			return getopt.InvalidOption(opt)
		}
		outfile = value
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	return outfile, operands, nil
}

// input describes the input operand: a pathname or the standard input.
type input struct {
	name  string
	stdin bool
}

// newInput maps the operands to an input, reading "-" as the standard input.
func newInput(operands []string) input {
	if len(operands) == 0 {
		return input{stdin: true}
	}
	return input{name: operands[0], stdin: operands[0] == "-"}
}

// label returns the name to use on diagnostic messages.
func (in input) label() string {
	if in.stdin && in.name == "" {
		return "standard input"
	}
	return in.name
}

// errWriter records the first write error, so a read error isn't mistaken
// for one.
type errWriter struct {
	w   io.Writer
	err error
}

// Write implements io.Writer.
func (w *errWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if err != nil && w.err == nil {
		w.err = err
	}
	return n, err
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
