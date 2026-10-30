// Package paste implements the POSIX paste utility.
//
// It merges the corresponding lines of the given files, or all the lines of
// each file with -s, writing one output line per input line group. The
// standard input is used for every "-" operand, read one line at a time,
// circularly.
package paste

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the paste core utility.
type Prog struct {
	*coreutils.Runtime

	stdin *bufio.Reader
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run merges the lines of the given files to the standard output. Without -s,
// when an input file can't be opened no output is written at all.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "paste", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "paste")
		if errors.Is(err, errTrailingBackslash) {
			return &coreutils.ExitError{Code: 1}
		}
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) == 0 {
		p.errorf("missing operand")
		stdopts.TryHelp(p.Runtime, "paste")
		return &coreutils.ExitError{Code: 2}
	}

	if opts.serial {
		return p.serial(ctx, opts, operands)
	}

	inputs := make([]*input, 0, len(operands))
	ok := true
	for _, operand := range operands {
		in := p.openInput(ctx, operand)
		if in.err != nil {
			p.openErrorf(in)
			ok = false
		}
		inputs = append(inputs, in)
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	defer func() {
		for _, in := range inputs {
			in.close()
		}
	}()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var line strings.Builder
		data := false
		for i, in := range inputs {
			if i > 0 {
				line.WriteString(opts.delims[(i-1)%len(opts.delims)])
			}
			part, err := in.readLine(opts.term())
			switch {
			case err != nil && ctx.Err() != nil:
				return ctx.Err()
			case err != nil:
				p.errorf("%s: %v", in.label(), patherr.Unwrap(err))
				ok = false
			case part != nil:
				data = true
				line.WriteString(*part)
			}
		}
		if !data {
			break
		}
		line.WriteByte(opts.term())
		if err := p.write(line.String()); err != nil {
			return err
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// serial concatenates all the lines of each input into one output line per
// input, in the order the operands are given.
func (p *Prog) serial(ctx context.Context, opts options, operands []string) error {
	ok := true
	for _, operand := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		in := p.openInput(ctx, operand)
		if in.err != nil {
			p.openErrorf(in)
			ok = false
			continue
		}
		var line strings.Builder
		delim := 0
		for i := 0; ; i++ {
			part, err := in.readLine(opts.term())
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				p.errorf("%s: %v", in.label(), patherr.Unwrap(err))
				ok = false
				break
			}
			if part == nil {
				break
			}
			if i > 0 {
				line.WriteString(opts.delims[delim])
				delim = (delim + 1) % len(opts.delims)
			}
			line.WriteString(*part)
		}
		in.close()
		line.WriteByte(opts.term())
		if err := p.write(line.String()); err != nil {
			return err
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// openInput returns an input reading from the operand: the standard input for
// "-", a file otherwise. The standard input is shared by every "-" operand.
func (p *Prog) openInput(ctx context.Context, operand string) *input {
	if operand == "-" {
		if p.stdin == nil {
			if p.Stdin == nil {
				return &input{operand: operand, err: errors.New("no standard input available")}
			}
			p.stdin = bufio.NewReader(ctxio.NewReader(ctx, p.Stdin))
		}
		return &input{operand: operand, r: p.stdin}
	}
	if operand == "" {
		return &input{operand: operand, err: errors.New("invalid zero-length file name")}
	}
	if p.FS == nil {
		return &input{operand: operand, err: errors.New("no file system available")}
	}
	f, err := p.FS.Open(filepath.ToSlash(p.ResolvePath(operand)))
	if err != nil {
		return &input{operand: operand, err: err}
	}
	return &input{
		operand: operand,
		r:       bufio.NewReader(ctxio.NewReader(ctx, f)),
		c:       f,
	}
}

// write writes an output line to the standard output.
func (p *Prog) write(line string) error {
	if _, err := io.WriteString(p.Stdout, line); err != nil {
		p.errorf("write error: %v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// openErrorf reports the error of an input which couldn't be opened.
func (p *Prog) openErrorf(in *input) {
	if in.operand == "" {
		p.errorf("%v", in.err)
		return
	}
	p.errorf("%s: %v", in.label(), patherr.Unwrap(in.err))
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "paste: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	serial bool
	zero   bool
	delims []string
}

// term returns the byte which terminates an item, both when reading the
// input and when writing the output.
func (o options) term() byte {
	if o.zero {
		return 0
	}
	return '\n'
}

// longOptions maps the long options of paste to the short ones they stand
// for.
var longOptions = map[string]rune{
	"delimiters":      'd',
	"serial":          's',
	"zero-terminated": 'z',
}

// optSet describes the options of paste.
var optSet = getopt.Set{Valued: "d", Long: longOptions}

// usage describes the options of paste.
const usage = `Usage: paste [OPTION]... [FILE]...
Write lines consisting of the sequentially corresponding lines from
each file, separated by TABs, to standard output.

With no FILE, or when FILE is -, read standard input.

  -d, --delimiters=LIST
         reuse characters from LIST instead of TABs
  -s, --serial
         paste one file at a time instead of in parallel
  -z, --zero-terminated
         line delimiter is NUL, not newline
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{delims: []string{"\t"}}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'd':
			var err error
			opts.delims, err = parseDelims(value)
			if err != nil {
				return fmt.Errorf("%w: %s", err, quoteDelims(value))
			}
		case 's':
			opts.serial = true
		case 'z':
			opts.zero = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// errTrailingBackslash marks a delimiter list which ends with an unescaped
// backslash.
var errTrailingBackslash = errors.New("delimiter list ends with an unescaped backslash")

// parseDelims parses a -d list into the delimiter elements. A backslash
// introduces the escapes "\b", "\f", "\n", "\r", "\t", "\v", "\\" and "\0"
// (the empty string); any other character following a backslash stands for
// itself. A list ending with an unescaped backslash is an error.
func parseDelims(list string) ([]string, error) {
	var delims []string
	for i := 0; i < len(list); i++ {
		if list[i] != '\\' {
			delims = append(delims, list[i:i+1])
			continue
		}
		if i+1 == len(list) {
			return nil, errTrailingBackslash
		}
		i++
		switch list[i] {
		case '0':
			delims = append(delims, "")
		case 'b':
			delims = append(delims, "\b")
		case 'f':
			delims = append(delims, "\f")
		case 'n':
			delims = append(delims, "\n")
		case 'r':
			delims = append(delims, "\r")
		case 't':
			delims = append(delims, "\t")
		case 'v':
			delims = append(delims, "\v")
		case '\\':
			delims = append(delims, "\\")
		default:
			delims = append(delims, list[i:i+1])
		}
	}
	if len(delims) == 0 {
		return []string{""}, nil
	}
	return delims, nil
}

// quoteDelims quotes a delimiter list for a diagnostic, in the C quoting
// style used by GNU paste.
func quoteDelims(list string) string {
	return "'" + strings.ReplaceAll(list, "'", `'\''`) + "'"
}

// input is a line source: an open file or the standard input.
type input struct {
	operand string
	r       *bufio.Reader
	c       io.Closer
	err     error
	eof     bool
}

// label returns the name to use on diagnostic messages.
func (in *input) label() string {
	if in.operand == "-" {
		return "standard input"
	}
	return in.operand
}

// readLine returns the next item without its trailing line delimiter, or nil
// when the input is exhausted. An exhausted input keeps returning nil.
func (in *input) readLine(sep byte) (*string, error) {
	if in.eof {
		return nil, nil
	}
	line, err := in.r.ReadString(sep)
	if err == nil {
		line = strings.TrimSuffix(line, string(sep))
		return &line, nil
	}
	if errors.Is(err, io.EOF) {
		in.eof = true
		if line != "" {
			return &line, nil
		}
		return nil, nil
	}
	in.eof = true
	return nil, err
}

// close closes the input file, if any.
func (in *input) close() {
	if in.c != nil {
		in.c.Close()
	}
}

var _ coreutils.Program = (*Prog)(nil)
