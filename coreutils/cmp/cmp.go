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
	"strconv"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
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
	if done, err := stdopts.Check(p.Runtime, "cmp", usage, optSet, args); done {
		return err
	}
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
		r, err := input.Open(p.Runtime, ctx, input.New(name))
		if err != nil {
			if !opts.silent {
				p.errorf("%s: %v", name, patherr.Unwrap(err))
			}
			return &coreutils.ExitError{Code: 2}
		}
		readers[i] = r
		defer r.Close()
	}

	status, err := p.compare(ctx, readers[0], readers[1], [2]string{operands[0], operands[1]}, opts)
	if err != nil {
		var writeErr *writeError
		var readErr *input.ReadError
		switch {
		case errors.As(err, &writeErr):
			p.errorf("write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 2, Err: writeErr.err}
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.As(err, &readErr):
			p.errorf("%s: %v", readErr.Label, patherr.Unwrap(readErr.Err))
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
		// remaining counts the bytes still to be compared; it is negative when
		// the comparison is unlimited.
		remaining = opts.bytes
	)
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		size := bufSize
		if remaining >= 0 && remaining < int64(bufSize) {
			size = int(remaining)
		}
		remaining -= int64(size)

		n1, err1 := io.ReadFull(br1, buf1[:size])
		if err := readErr(err1); err != nil {
			return 0, &input.ReadError{Label: names[0], Err: err}
		}
		n2, err2 := io.ReadFull(br2, buf2[:size])
		if err := readErr(err2); err != nil {
			return 0, &input.ReadError{Label: names[1], Err: err}
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
		if n1 < size || remaining == 0 {
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

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "cmp: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	list   bool
	silent bool
	bytes  int64 // -n; negative when the comparison is unlimited
}

// longOptions maps the long options of cmp to the short ones they stand for.
var longOptions = map[string]rune{
	"bytes":   'n',
	"verbose": 'l',
	"quiet":   's',
	"silent":  's',
}

// optSet describes the options of cmp.
var optSet = getopt.Set{Valued: "n", Long: longOptions}

// usage describes the options of cmp.
const usage = `Usage: cmp [OPTION]... FILE1 FILE2
Compare two files byte by byte.

An operand of "-" is the standard input. An exit status of 1 means the
files differ, and 2 that an error happened.

  -l, --verbose          output byte numbers and values of all differing bytes
  -n, --bytes=N          compare at most N bytes
  -s, --quiet, --silent  suppress all normal output
      --help     display this help and exit
      --version  output version information and exit
`

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{bytes: -1}
	var mode byte
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
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
		case 'n':
			limit, err := parseLimit(value)
			if err != nil {
				return err
			}
			if opts.bytes < 0 || limit < opts.bytes {
				opts.bytes = limit
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

// parseLimit parses a non-negative decimal byte count.
func parseLimit(value string) (int64, error) {
	if value == "" || value[0] == '+' || value[0] == '-' {
		return 0, fmt.Errorf("invalid --bytes value '%s'", value)
	}
	limit, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid --bytes value '%s'", value)
	}
	return limit, nil
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
