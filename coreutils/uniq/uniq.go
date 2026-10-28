// Package uniq implements the POSIX uniq utility.
//
// It compares the adjacent lines of the input, writing one copy of each
// repeated group to the output. The input is the standard input when no
// input operand is given or it's "-"; the output is the standard output when
// no output operand is given or it's "-".
package uniq

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/textutil"
)

// Prog is the uniq core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run filters the adjacent repeated lines of the input, writing one copy of
// each group to the output, as selected by the options.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	var inName, outName string
	switch len(operands) {
	case 0:
	case 1:
		inName = operands[0]
	case 2:
		inName, outName = operands[0], operands[1]
	default:
		p.errorf("extra operand '%s'", operands[2])
		return &coreutils.ExitError{Code: 2}
	}
	if slices.Contains(operands, "") {
		p.errorf("invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}

	in := input.Input{Name: inName, Stdin: inName == "" || inName == "-"}
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1}
	}
	defer r.Close()

	out, err := p.openOutput(outName)
	if err != nil {
		p.errorf("%s: %v", outName, patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1}
	}

	if err := p.uniq(ctx, r, out, opts); err != nil {
		var writeErr *writeError
		switch {
		case errors.As(err, &writeErr):
			p.errorf("write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 1, Err: writeErr.err}
		case ctx.Err() != nil:
			return ctx.Err()
		default:
			p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 1}
		}
	}
	return nil
}

// uniq copies the groups of adjacent equal lines of r to out, as the options
// demand.
func (p *Prog) uniq(ctx context.Context, r io.Reader, out io.Writer, opts options) error {
	br := bufio.NewReader(r)
	var (
		prevKey  []byte
		prevRaw  []byte
		count    int
		havePrev bool
	)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			raw := []byte(line)
			key := opts.key([]byte(strings.TrimSuffix(line, "\n")))
			if havePrev && bytes.Equal(prevKey, key) {
				count++
			} else {
				if havePrev {
					if werr := p.writeGroup(out, prevRaw, count, opts); werr != nil {
						return werr
					}
				}
				prevKey, prevRaw, count, havePrev = key, raw, 1, true
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}
	if havePrev {
		return p.writeGroup(out, prevRaw, count, opts)
	}
	return nil
}

// writeGroup writes the representative line of a group of count equal lines,
// as the options demand.
func (p *Prog) writeGroup(out io.Writer, raw []byte, count int, opts options) error {
	if opts.uniqueOnly && count > 1 || opts.repeatedOnly && count == 1 {
		return nil
	}
	if opts.count {
		if _, err := fmt.Fprintf(out, "%7d ", count); err != nil {
			return &writeError{err: err}
		}
	}
	if _, err := out.Write(raw); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// key returns the part of the line which takes part in comparisons: the line
// with the first opts.fields fields and opts.chars characters skipped.
func (opts options) key(line []byte) []byte {
	return textutil.SkipChars(textutil.SkipFields(line, opts.fields), opts.chars)
}

// openOutput returns a writer for the output operand. An empty name or "-"
// mean the standard output.
func (p *Prog) openOutput(name string) (io.Writer, error) {
	if name == "" || name == "-" {
		return p.Stdout, nil
	}
	if p.FS == nil {
		return nil, errors.New("no file system available")
	}
	return p.FS.OpenFile(filepath.ToSlash(p.ResolvePath(name)), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666&^p.Umask.Perm())
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "uniq: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	count        bool
	repeatedOnly bool
	uniqueOnly   bool
	fields       int
	chars        int
}

// longOptions maps the long options of uniq to the short ones they stand for.
var longOptions = map[string]rune{
	"count":       'c',
	"repeated":    'd',
	"unique":      'u',
	"skip-fields": 'f',
	"skip-chars":  's',
}

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := getopt.Set{Valued: "fs", Long: longOptions}.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'c':
			opts.count = true
		case 'd':
			opts.repeatedOnly = true
		case 'u':
			opts.uniqueOnly = true
		case 'f':
			n, err := parseSkip(value, "fields")
			if err != nil {
				return err
			}
			opts.fields = n
		case 's':
			n, err := parseSkip(value, "characters")
			if err != nil {
				return err
			}
			opts.chars = n
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	if opts.repeatedOnly && opts.uniqueOnly {
		return options{}, nil, errors.New("printing only duplicated and unique lines is not supported")
	}
	return opts, operands, nil
}

// parseSkip parses a non-negative decimal count of fields or characters to
// skip.
func parseSkip(s string, what string) (int, error) {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("invalid number of %s: '%s'", what, s)
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid number of %s: '%s'", what, s)
	}
	return n, nil
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
