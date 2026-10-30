// Package cat implements the GNU cat utility.
//
// It reads the given files in sequence and writes their contents to the
// standard output in the same sequence. The standard input is used when no
// operands are given, and also when an operand is "-". Display options can
// number lines, squeeze repeated blank ones and reveal control characters,
// tabs and line ends.
package cat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"unicode"
	"unicode/utf8"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
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
// operands are given, and also when an operand is "-". With the display
// options, lines are numbered, repeated blank lines are squeezed into one,
// and non-printing characters, tabs and line ends are made visible.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "cat", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "cat")
		return &coreutils.ExitError{Code: 2}
	}

	inputs := input.List(operands)
	pr := newPrinter(opts, p.Stdout)

	ok := true
	for _, in := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !in.Stdin && in.Name == "" {
			p.errorf("invalid zero-length file name")
			ok = false
			continue
		}
		if err := p.cat(ctx, in, pr); err != nil {
			var writeErr *writeError
			switch {
			case errors.As(err, &writeErr):
				p.errorf("write error: %v", writeErr.err)
				return &coreutils.ExitError{Code: 1, Err: writeErr.err}
			case ctx.Err() != nil:
				return ctx.Err()
			default:
				p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
				ok = false
			}
		}
	}
	if err := pr.flush(); err != nil {
		p.errorf("write error: %v", patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// cat writes the contents of a single input to the standard output. Without
// display options the bytes are copied as they are read; otherwise each line
// goes through the printer.
func (p *Prog) cat(ctx context.Context, in input.Input, pr *printer) error {
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		return err
	}
	defer r.Close()

	if !pr.opts.display() {
		return copyRaw(ctx, r, p.Stdout)
	}
	br := bufio.NewReader(r)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if werr := pr.line(line); werr != nil {
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

// copyRaw writes the bytes of r to w as they are read.
func copyRaw(ctx context.Context, r io.Reader, w io.Writer) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
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

// options holds the display options of cat.
type options struct {
	number          bool
	numberNonblank  bool
	squeeze         bool
	showNonprinting bool
	showEnds        bool
	showTabs        bool
}

// display reports whether any option transforms the output.
func (o options) display() bool {
	return o.number || o.numberNonblank || o.squeeze ||
		o.showNonprinting || o.showEnds || o.showTabs
}

// longOptions maps the long options of cat to the short ones they stand
// for.
var longOptions = map[string]rune{
	"number":           'n',
	"number-nonblank":  'b',
	"squeeze-blank":    's',
	"show-nonprinting": 'v',
	"show-ends":        'E',
	"show-tabs":        'T',
	"show-all":         'A',
}

// optSet describes the options of cat.
var optSet = getopt.Set{Long: longOptions}

// usage describes the options of cat.
const usage = `Usage: cat [OPTION]... [FILE]...
Concatenate FILE(s) to standard output.

With no FILE, or when FILE is -, read standard input.

  -A, --show-all           equivalent to -vET
  -b, --number-nonblank    number nonempty output lines, overrides -n
  -e                        equivalent to -vE
  -E, --show-ends          display $ at end of each line
  -n, --number             number all output lines, starting with 1
  -s, --squeeze-blank      suppress repeated adjacent blank lines
  -t                        equivalent to -vT
  -T, --show-tabs          display TAB characters as ^I
  -u                        (ignored)
  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB
      --help     display this help and exit
      --version  output version information and exit
`

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--". The -u option is accepted and has no effect:
// output is written as each byte is read, which is the behavior -u asks
// for. The -A, -e and -t options combine their letters' effects.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'n':
			opts.number = true
		case 'b':
			opts.numberNonblank = true
		case 's':
			opts.squeeze = true
		case 'v':
			opts.showNonprinting = true
		case 'E':
			opts.showEnds = true
		case 'T':
			opts.showTabs = true
		case 'A':
			opts.showNonprinting, opts.showEnds, opts.showTabs = true, true, true
		case 'e':
			opts.showNonprinting, opts.showEnds = true, true
		case 't':
			opts.showNonprinting, opts.showTabs = true, true
		case 'u':
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// printer writes the lines of the inputs with the display options applied.
// Its state spans the whole run, so numbering and blank-line squeezing carry
// over file boundaries, and an input which ends without a newline only ends
// its line when a following input (or the end of the run) continues it.
type printer struct {
	opts      options
	w         *bufio.Writer
	lineNo    int
	prevBlank bool
	pending   []byte
}

func newPrinter(opts options, w io.Writer) *printer {
	return &printer{opts: opts, w: bufio.NewWriter(w)}
}

// line writes a single line, which includes its trailing newline when it
// has one. A line without one is held back until more bytes continue it.
func (p *printer) line(line []byte) error {
	nl := len(line) > 0 && line[len(line)-1] == '\n'
	content := line
	if nl {
		content = line[:len(line)-1]
	}
	if p.pending != nil {
		content = append(p.pending, content...)
		p.pending = nil
	}
	if !nl {
		p.pending = content
		return nil
	}
	return p.writeLine(content, true)
}

// writeLine writes a complete line, whose terminator is written only when
// the input had one.
func (p *printer) writeLine(content []byte, nl bool) error {
	blank := len(content) == 0

	if p.opts.squeeze && blank && p.prevBlank {
		return nil
	}
	p.prevBlank = blank

	switch {
	case p.opts.numberNonblank:
		if !blank {
			p.lineNo++
			fmt.Fprintf(p.w, "%6d\t", p.lineNo)
		}
	case p.opts.number:
		p.lineNo++
		fmt.Fprintf(p.w, "%6d\t", p.lineNo)
	}

	p.shown(content)
	if p.opts.showEnds {
		p.w.WriteByte('$')
	}
	if nl {
		p.w.WriteByte('\n')
	}
	return nil
}

// shown writes the line content with the -v and -T transformations. Only
// printable characters are copied as they are: valid multibyte sequences of
// printable runes pass through, while control bytes, DEL and non-printing
// bytes are shown in the ^X and M-? notation.
func (p *printer) shown(content []byte) {
	for i := 0; i < len(content); {
		b := content[i]
		if b == '\t' && p.opts.showTabs {
			p.w.WriteString("^I")
			i++
			continue
		}
		if !p.opts.showNonprinting {
			p.w.WriteByte(b)
			i++
			continue
		}
		if b >= 0x80 {
			if r, size := utf8.DecodeRune(content[i:]); size > 1 && unicode.IsPrint(r) {
				p.w.Write(content[i : i+size])
				i += size
				continue
			}
			p.w.WriteString("M-")
			p.escape(b & 0x7f)
			i++
			continue
		}
		if b < 0x20 && b != '\t' || b == 0x7f {
			p.escape(b)
			i++
			continue
		}
		p.w.WriteByte(b)
		i++
	}
}

// escape writes a control byte in the caret notation.
func (p *printer) escape(b byte) {
	switch {
	case b < 0x20:
		p.w.WriteByte('^')
		p.w.WriteByte(b + '@')
	case b == 0x7f:
		p.w.WriteString("^?")
	default:
		p.w.WriteByte(b)
	}
}

func (p *printer) flush() error {
	if p.pending != nil {
		if err := p.writeLine(p.pending, false); err != nil {
			return err
		}
		p.pending = nil
	}
	return p.w.Flush()
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "cat: "+format+"\n", args...)
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

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
