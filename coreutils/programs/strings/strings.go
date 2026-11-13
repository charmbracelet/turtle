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

	"charm.land/turtle/coreutils"

	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/input"
	"charm.land/turtle/coreutils/internal/patherr"
	"charm.land/turtle/coreutils/internal/stdopts"
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
	if done, err := stdopts.Check(p.Runtime, "strings", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		stdopts.Errorf(p.Runtime, "strings", "%v", err)
		stdopts.TryHelp(p.Runtime, "strings")
		return &coreutils.ExitError{Code: 1}
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	for _, name := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		in := input.New(name)
		r, err := input.Open(p.Runtime, ctx, in)
		if err != nil {
			stdopts.Errorf(p.Runtime, "strings", "%s: %v", name, patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 1}
		}
		data, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			stdopts.Errorf(p.Runtime, "strings", "%s: %v", name, patherr.Unwrap(readErr))
			return &coreutils.ExitError{Code: 1}
		}
		if closeErr != nil {
			stdopts.Errorf(p.Runtime, "strings", "%s: %v", name, patherr.Unwrap(closeErr))
			return &coreutils.ExitError{Code: 1}
		}
		if err := p.writeStrings(data, in.Label(), opts); err != nil {
			stdopts.Errorf(p.Runtime, "strings", "write error: %v", err)
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	}
	return nil
}

func (p *Prog) writeStrings(data []byte, label string, opts options) error {
	var start int
	var count int
	flush := func(end int) error {
		if count >= opts.minLength {
			if opts.printFileNames {
				if _, err := fmt.Fprintf(p.Stdout, "%s: ", label); err != nil {
					return err
				}
			}
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

type options struct {
	minLength      int
	offset         byte
	printFileNames bool
}

// optSet describes the options of strings.
var optSet = getopt.Set{
	Valued: "nt",
	Long:   map[string]rune{"print-file-name": 'f'},
}

// usage describes the options of strings.
const usage = `Usage: strings [OPTION]... [FILE]...
Print the sequences of printable characters of at least 4 bytes each found
in the given files, or the standard input when no file is given.

  -a
         scan the whole file, not only the loaded data segments
  -f, --print-file-name
         print the file name before each string
  -n MIN
         print strings of at least MIN bytes (default 4)
  -o
         print the offsets in octal, like -t o
  -t RADIX
         print the offsets in the given radix: d (decimal), o (octal) or
         x (hexadecimal)
` + stdopts.Tail

func parseFlags(args []string) (options, []string, error) {
	opts := options{minLength: 4}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
		case 'f':
			opts.printFileNames = true
		case 'n':
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return errors.New("invalid minimum string length")
			}
			opts.minLength = n
		case 'o':
			opts.offset = 'o'
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
