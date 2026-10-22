// Package cmp implements the POSIX cmp utility.
//
// It compares two files byte by byte, writing the position of the first
// difference, or every differing byte with -l. An operand of "-" means the
// standard input.
package cmp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
)

// Prog is the cmp core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run compares two files byte by byte, returning an exit status of 1 when
// they differ and 2 when any other error happens.
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

	var readers [2]io.Reader
	for i, name := range operands {
		r, err := p.openInput(ctx, name)
		if err != nil {
			if !opts.silent {
				p.errorf("%s: %v", name, unwrapPathError(err))
			}
			return &coreutils.ExitError{Code: 2}
		}
		readers[i] = r
		defer r.Close()
	}

	status, err := p.compare(ctx, readers[0], readers[1], [2]string{operands[0], operands[1]}, opts)
	if err != nil {
		var writeErr *writeError
		var readErr *readError
		switch {
		case errors.As(err, &writeErr):
			p.errorf("write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 2, Err: writeErr.err}
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.As(err, &readErr):
			p.errorf("%s: %v", readErr.label, unwrapPathError(readErr.err))
			return &coreutils.ExitError{Code: 2}
		default:
			return err
		}
	}
	if status > 0 {
		return &coreutils.ExitError{Code: status}
	}
	return nil
}

// bufSize is the size of the blocks read from each file.
const bufSize = 32 * 1024

// compare compares both files block by block, writing the differences to the
// standard output as the options demand. It returns 1 when the files differ,
// and 0 when they are identical.
func (p *Prog) compare(ctx context.Context, r1, r2 io.Reader, names [2]string, opts options) (int, error) {
	br1 := bufio.NewReader(r1)
	br2 := bufio.NewReader(r2)
	buf1 := make([]byte, bufSize)
	buf2 := make([]byte, bufSize)
	var (
		byteNumber  int64 = 1
		lineNumber  int64 = 1
		atLineStart       = true
		differing         = false
	)
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		n1, err1 := io.ReadFull(br1, buf1)
		if err := readErr(err1); err != nil {
			return 0, &readError{label: names[0], err: err}
		}
		n2, err2 := io.ReadFull(br2, buf2)
		if err := readErr(err2); err != nil {
			return 0, &readError{label: names[1], err: err}
		}

		smaller := min(n1, n2)
		diff := firstDiff(buf1[:smaller], buf2[:smaller])
		if diff < smaller {
			if opts.silent {
				return 1, nil
			}
			if opts.list {
				differing = true
				for i := diff; i < smaller; i++ {
					if buf1[i] != buf2[i] {
						if err := p.writeDiff(byteNumber+int64(i), buf1[i], buf2[i]); err != nil {
							return 0, err
						}
					}
				}
			} else {
				lineNumber += countNewlines(buf1[:diff])
				if err := p.writeFirstDiff(names, byteNumber+int64(diff), lineNumber); err != nil {
					return 0, err
				}
				return 1, nil
			}
		}
		if !opts.list {
			lineNumber += countNewlines(buf1[:smaller])
			if smaller > 0 {
				atLineStart = buf1[smaller-1] == '\n'
			}
		}
		byteNumber += int64(smaller)

		if n1 != n2 {
			if !opts.silent {
				shorter := names[0]
				if n2 < n1 {
					shorter = names[1]
				}
				p.writeEOF(shorter, byteNumber-1, lineNumber, atLineStart, opts)
			}
			return 1, nil
		}
		if n1 < bufSize {
			if differing {
				return 1, nil
			}
			return 0, nil
		}
	}
}

// writeFirstDiff writes the message pointing at the first difference.
func (p *Prog) writeFirstDiff(names [2]string, byteNumber, lineNumber int64) error {
	_, err := fmt.Fprintf(p.Stdout, "%s %s differ: char %d, line %d\n",
		names[0], names[1], byteNumber, lineNumber)
	if err != nil {
		return &writeError{err: err}
	}
	return nil
}

// writeDiff writes the message of a differing byte for -l.
func (p *Prog) writeDiff(byteNumber int64, b1, b2 byte) error {
	_, err := fmt.Fprintf(p.Stdout, "%d %o %o\n", byteNumber, b1, b2)
	if err != nil {
		return &writeError{err: err}
	}
	return nil
}

// writeEOF writes the diagnostic of the file with the given name ending
// before the other, after compared bytes were already matched.
func (p *Prog) writeEOF(name string, compared, lineNumber int64, atLineStart bool, opts options) {
	switch {
	case compared == 0:
		p.errorf("EOF on '%s' which is empty", name)
	case opts.list:
		p.errorf("EOF on '%s' after byte %d", name, compared)
	case atLineStart:
		p.errorf("EOF on '%s' after byte %d, line %d", name, compared, lineNumber-1)
	default:
		p.errorf("EOF on '%s' after byte %d, in line %d", name, compared, lineNumber)
	}
}

// openInput returns a reader for the given operand. Reads stop early when ctx
// is done. The standard input isn't owned by the caller, so it's wrapped on a
// no-op closer.
func (p *Prog) openInput(ctx context.Context, name string) (io.ReadCloser, error) {
	if name == "-" {
		if p.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return ctxReadCloser{ctx: ctx, rc: io.NopCloser(p.Stdin)}, nil
	}
	if p.FS == nil {
		return nil, errors.New("no file system available")
	}
	f, err := p.FS.Open(filepath.ToSlash(p.ResolvePath(name)))
	if err != nil {
		return nil, err
	}
	return ctxReadCloser{ctx: ctx, rc: f}, nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "cmp: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	list   bool
	silent bool
}

// longOptions maps the long options of cmp to the short ones they stand for.
var longOptions = map[string]rune{
	"verbose": 'l',
	"quiet":   's',
	"silent":  's',
}

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	var mode byte
	operands, err := getopt.Set{Long: longOptions}.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'l':
			if mode != 0 && mode != 'l' {
				return errors.New("options -l and -s are incompatible")
			}
			mode = 'l'
			opts.list = true
		case 's':
			if mode != 0 && mode != 's' {
				return errors.New("options -l and -s are incompatible")
			}
			mode = 's'
			opts.silent = true
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

// readErr returns the error of a read result, or nil when it only signals
// the end of the input.
func readErr(err error) error {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return nil
	}
	return err
}

// firstDiff returns the offset of the first differing byte of b1 and b2, or
// their length when they are equal.
func firstDiff(b1, b2 []byte) int {
	if bytes.Equal(b1, b2) {
		return len(b1)
	}
	for i := range b1 {
		if b1[i] != b2[i] {
			return i
		}
	}
	return len(b1)
}

// countNewlines returns the number of newlines in b.
func countNewlines(b []byte) int64 {
	return int64(bytes.Count(b, []byte{'\n'}))
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
