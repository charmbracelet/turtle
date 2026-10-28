// Package strings implements the POSIX strings utility.
package strings

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode"
	"unicode/utf8"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
)

// Prog is the strings core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes printable strings found in the input files.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	for _, name := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, err := input.Open(p.Runtime, ctx, input.New(name))
		if err != nil {
			p.errorf("%s: %v", name, patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 1}
		}
		data, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			p.errorf("%s: %v", name, patherr.Unwrap(readErr))
			return &coreutils.ExitError{Code: 1}
		}
		if closeErr != nil {
			p.errorf("%s: %v", name, patherr.Unwrap(closeErr))
			return &coreutils.ExitError{Code: 1}
		}
		if err := p.writeStrings(data, opts); err != nil {
			p.errorf("write error: %v", err)
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	}
	return nil
}

func (p *Prog) writeStrings(data []byte, opts options) error {
	var start int
	var count int
	flush := func(end int) error {
		if count >= opts.minLength {
			if opts.offset != 0 {
				if _, err := fmt.Fprintf(p.Stdout, "%s ", formatOffset(uint64(start), opts.offset)); err != nil {
					return err
				}
			}
			if _, err := p.Stdout.Write(data[start:end]); err != nil {
				return err
			}
			if _, err := io.WriteString(p.Stdout, "\n"); err != nil {
				return err
			}
		}
		count = 0
		return nil
	}
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r != utf8.RuneError && unicode.IsPrint(r) {
			if count == 0 {
				start = i
			}
			count++
			i += size
			continue
		}
		if err := flush(i); err != nil {
			return err
		}
		i += size
	}
	return flush(len(data))
}

func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "strings: "+format+"\n", args...)
}

type options struct {
	minLength int
	offset    byte
}

func parseFlags(args []string) (options, []string, error) {
	opts := options{minLength: 4}
	operands, err := (getopt.Set{Valued: "nt"}).Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
		case 'n':
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return errors.New("invalid minimum string length")
			}
			opts.minLength = n
		case 't':
			if len(value) != 1 || (value[0] != 'd' && value[0] != 'o' && value[0] != 'x') {
				return errors.New("invalid format type")
			}
			opts.offset = value[0]
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

func formatOffset(offset uint64, format byte) string {
	switch format {
	case 'd':
		return strconv.FormatUint(offset, 10)
	case 'o':
		return strconv.FormatUint(offset, 8)
	default:
		return strconv.FormatUint(offset, 16)
	}
}

var _ coreutils.Program = (*Prog)(nil)
