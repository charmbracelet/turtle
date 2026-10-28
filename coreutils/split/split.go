// Package split implements the POSIX split utility.
//
// It writes pieces of the input file to files named after a prefix and a
// letter suffix ("xaa", "xab", ... by default), either by line count or by
// byte count.
package split

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/fsx"
)

// Prog is the split core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run reads the input file and writes its pieces, returning an exit status
// of 2 for invalid usage and 1 for any processing error.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) > 2 {
		p.errorf("extra operand '%s'", operands[2])
		return &coreutils.ExitError{Code: 2}
	}
	name := "-"
	prefix := "x"
	if len(operands) >= 1 {
		name = operands[0]
	}
	if len(operands) == 2 {
		prefix = operands[1]
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	w := &pieceWriter{rt: p.Runtime, prefix: prefix, suffixLen: opts.suffixLen}
	if len(baseName(prefix))+opts.suffixLen > nameMax {
		p.errorf("%s: file name too long", w.nextName())
		return &coreutils.ExitError{Code: 1}
	}

	r, err := input.Open(p.Runtime, ctx, input.New(name))
	if err != nil {
		p.errorf("%s: %v", name, unwrapPathError(err))
		return &coreutils.ExitError{Code: 1}
	}
	defer r.Close()

	if opts.bytes >= 0 {
		err = splitBytes(ctx, r, w, opts.bytes)
	} else {
		err = splitLines(ctx, r, w, opts.lines)
	}
	if err != nil {
		var writeErr *writeError
		var readErr *readError
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, errSuffixesExhausted):
			p.errorf("%v", err)
		case errors.As(err, &writeErr):
			p.errorf("%s: %v", writeErr.name, unwrapPathError(writeErr.err))
		case errors.As(err, &readErr):
			p.errorf("%s: %v", name, unwrapPathError(readErr.err))
		default:
			p.errorf("%v", err)
		}
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// nameMax is the assumed NAME_MAX limit of the output directory.
const nameMax = 255

// pieceWriter creates the output files and keeps track of the suffixes.
type pieceWriter struct {
	rt        *coreutils.Runtime
	prefix    string
	suffixLen int
	count     int64
	file      fsx.File
	name      string
}

// errSuffixesExhausted signals that no more suffixes can be formed.
var errSuffixesExhausted = errors.New("output file suffixes exhausted")

// nextName returns the output name for the current count.
func (w *pieceWriter) nextName() string {
	suffix := make([]byte, w.suffixLen)
	n := w.count
	for i := w.suffixLen - 1; i >= 0; i-- {
		suffix[i] = byte('a' + n%26)
		n /= 26
	}
	return w.prefix + string(suffix)
}

// openNext starts a new piece, closing the previous one if any.
func (w *pieceWriter) openNext() error {
	if err := w.close(); err != nil {
		return err
	}
	if w.count >= intPow(26, w.suffixLen) {
		return errSuffixesExhausted
	}
	name := w.nextName()
	f, err := w.rt.FS.OpenFile(filepath.ToSlash(w.rt.ResolvePath(name)), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666&^w.rt.Umask.Perm())
	if err != nil {
		return &writeError{name: name, err: err}
	}
	w.file = f
	w.name = name
	w.count++
	return nil
}

// write appends data to the current piece, opening one first if needed.
func (w *pieceWriter) write(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if w.file == nil {
		if err := w.openNext(); err != nil {
			return err
		}
	}
	if _, err := w.file.Write(data); err != nil {
		return &writeError{name: w.name, err: err}
	}
	return nil
}

// close closes the current piece, if any, so the next write starts a new
// one.
func (w *pieceWriter) close() error {
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	if err != nil {
		return &writeError{name: w.name, err: err}
	}
	return nil
}

// splitLines writes pieces of at most n lines each.
func splitLines(ctx context.Context, r io.Reader, w *pieceWriter, n int64) error {
	br := bufio.NewReader(r)
	var count int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if count == n {
				if err := w.close(); err != nil {
					return err
				}
				count = 0
			}
			if err := w.write(line); err != nil {
				return err
			}
			count++
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return &readError{err: err}
		}
	}
	return w.close()
}

// splitBytes writes pieces of at most n bytes each.
func splitBytes(ctx context.Context, r io.Reader, w *pieceWriter, n int64) error {
	buf := make([]byte, 32*1024)
	var remaining int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		nr, err := r.Read(buf)
		data := buf[:nr]
		for len(data) > 0 {
			if remaining == 0 {
				if err := w.close(); err != nil {
					return err
				}
				remaining = n
			}
			chunk := min(int64(len(data)), remaining)
			if err := w.write(data[:chunk]); err != nil {
				return err
			}
			data = data[chunk:]
			remaining -= chunk
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return &readError{err: err}
		}
	}
	return w.close()
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "split: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	lines     int64
	bytes     int64
	suffixLen int
}

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{lines: 1000, bytes: -1, suffixLen: 2}
	var mode byte
	operands, err := (getopt.Set{Valued: "abl"}).Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
			n, err := strconv.ParseInt(value, 10, 0)
			if err != nil || n < 0 || !isDecimal(value) {
				return fmt.Errorf("invalid suffix length: '%s'", value)
			}
			opts.suffixLen = int(n)
		case 'b':
			if mode != 0 {
				return errors.New("cannot split in more than one way")
			}
			mode = 'b'
			n, err := parseBytes(value)
			if err != nil {
				return fmt.Errorf("invalid number of bytes: '%s'", value)
			}
			opts.bytes = n
		case 'l':
			if mode != 0 {
				return errors.New("cannot split in more than one way")
			}
			mode = 'l'
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 1 || !isDecimal(value) {
				return fmt.Errorf("invalid number of lines: '%s'", value)
			}
			opts.lines = n
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

// parseBytes parses the -b option-argument: a positive decimal integer with
// an optional 'k' (1024) or 'm' (1048576) multiplier suffix.
func parseBytes(value string) (int64, error) {
	multiplier := int64(1)
	if strings.HasSuffix(value, "k") {
		multiplier = 1024
		value = strings.TrimSuffix(value, "k")
	} else if strings.HasSuffix(value, "m") {
		multiplier = 1024 * 1024
		value = strings.TrimSuffix(value, "m")
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 1 || n > math.MaxInt64/multiplier || !isDecimal(value) {
		return 0, errors.New("invalid number")
	}
	return n * multiplier, nil
}

// isDecimal reports whether s holds only decimal digits.
func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// intPow returns base**exp.
func intPow(base, exp int) int64 {
	result := int64(1)
	for range exp {
		result *= int64(base)
	}
	return result
}

// baseName returns the last path element of prefix.
func baseName(prefix string) string {
	return path.Base(filepath.ToSlash(prefix))
}

// readError marks an error as coming from reading the input.
type readError struct {
	err error
}

// Error implements the error interface.
func (e *readError) Error() string {
	return e.err.Error()
}

// Unwrap implements the interface to make readError unwrapable.
func (e *readError) Unwrap() error {
	return e.err
}

// writeError marks an error as coming from writing to an output file.
type writeError struct {
	name string
	err  error
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
