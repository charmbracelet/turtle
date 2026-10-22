// Package nl implements the POSIX nl utility.
//
// It numbers the lines of a file on the left, in terms of logical pages. A
// logical page consists of header, body and footer sections, each with its
// own numbering style, started by input lines holding nothing but the
// delimiter characters ("\:\:\:", "\:\:" and "\:" by default). Line
// numbering restarts at the start of each logical page, unless -p is given.
// When a line isn't numbered, both the line number and the separator are
// suppressed, as POSIX requires; delimiter lines are reproduced as empty
// lines.
package nl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/bre"
	"charm.land/snail/coreutils/internal/getopt"
)

// Prog is the nl core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the lines of the input to the standard output, adding line
// numbers according to the numbering style of the logical page section each
// line is in. It returns an exit status of 1 for any error.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	if len(operands) > 1 {
		p.errorf("extra operand '%s'", operands[1])
		return &coreutils.ExitError{Code: 1}
	}

	input := ""
	if len(operands) == 1 {
		input = operands[0]
	}
	r, err := p.open(ctx, input)
	if err != nil {
		p.errorf("%s: %v", label(input), unwrapPathError(err))
		return &coreutils.ExitError{Code: 1}
	}
	defer r.Close()

	if err := p.number(ctx, r, opts); err != nil {
		var writeErr *writeError
		switch {
		case errors.As(err, &writeErr):
			p.errorf("write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 1, Err: writeErr.err}
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, errOverflow):
			p.errorf("%v", err)
			return &coreutils.ExitError{Code: 1, Err: err}
		default:
			p.errorf("%s: %v", label(input), unwrapPathError(err))
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	}
	return nil
}

// number reads the lines of r and writes them numbered to the standard
// output.
func (p *Prog) number(ctx context.Context, r io.Reader, opts options) error {
	n := &numberer{opts: opts, out: bufio.NewWriter(p.Stdout)}
	n.reset()
	n.style = opts.body

	// fail flushes what was already produced before returning the error.
	fail := func(err error) error {
		n.out.Flush()
		return err
	}

	br := bufio.NewReader(r)
	for {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			terminated := line[len(line)-1] == '\n'
			content := line
			if terminated {
				content = line[:len(line)-1]
			}
			if cerr := n.line(content, terminated); cerr != nil {
				return fail(cerr)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fail(err)
		}
	}
	if err := n.out.Flush(); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// section is a logical page section.
type section int

const (
	text section = iota
	header
	body
	footer
)

// numberer holds the line numbering state.
type numberer struct {
	opts     options
	out      *bufio.Writer
	style    style
	lineNo   int64
	blanks   int64
	overflow bool
}

// reset restarts the line numbering.
func (n *numberer) reset() {
	n.lineNo = n.opts.start
	n.overflow = false
}

// line writes one input line: an empty line for a section delimiter, or the
// line itself, numbered when its section style says so.
func (n *numberer) line(content []byte, terminated bool) error {
	if sec, ok := n.section(content); ok {
		if err := n.out.WriteByte('\n'); err != nil {
			return &writeError{err: err}
		}
		switch sec {
		case header:
			n.style = n.opts.header
			if n.opts.restart {
				n.reset()
			}
		case body:
			n.style = n.opts.body
		case footer:
			n.style = n.opts.footer
		}
		return nil
	}

	numbered := false
	switch n.style.kind {
	case 'a':
		if len(content) > 0 {
			numbered = true
			n.blanks = 0
		} else {
			n.blanks++
			if n.blanks == n.opts.join {
				numbered = true
				n.blanks = 0
			}
		}
	case 't':
		numbered = len(content) > 0
	case 'p':
		numbered = n.style.re.Match(content)
	}
	if numbered {
		if err := n.printNo(); err != nil {
			return err
		}
	}
	if _, err := n.out.Write(content); err != nil {
		return &writeError{err: err}
	}
	if terminated {
		if err := n.out.WriteByte('\n'); err != nil {
			return &writeError{err: err}
		}
	}
	return nil
}

// section reports the section started by the given line content, when it
// holds nothing but delimiter characters.
func (n *numberer) section(content []byte) (section, bool) {
	del := n.opts.delim
	if del == "" {
		return text, false
	}
	switch string(content) {
	case del + del + del:
		return header, true
	case del + del:
		return body, true
	case del:
		return footer, true
	}
	return text, false
}

// printNo writes the current line number and the separator, and advances the
// line number.
func (n *numberer) printNo() error {
	if n.overflow {
		return errOverflow
	}
	format := "%*d%s"
	switch n.opts.format {
	case 'l':
		format = "%-*d%s"
	case 'z':
		format = "%0*d%s"
	}
	if _, err := fmt.Fprintf(n.out, format, n.opts.width, n.lineNo, n.opts.sep); err != nil {
		return &writeError{err: err}
	}
	sum := n.lineNo + n.opts.incr
	n.overflow = n.opts.incr > 0 && sum < n.lineNo || n.opts.incr < 0 && sum > n.lineNo
	n.lineNo = sum
	return nil
}

// errOverflow signals that the line number went past the int64 range.
var errOverflow = errors.New("line number overflow")

// open returns a reader for the input. Reads stop early when ctx is done. The
// standard input isn't owned by the caller, so it's wrapped on a no-op closer.
func (p *Prog) open(ctx context.Context, name string) (io.ReadCloser, error) {
	if name == "" || name == "-" {
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
	fmt.Fprintf(p.Stderr, "nl: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	body    style
	header  style
	footer  style
	delim   string
	incr    int64
	join    int64
	format  byte
	restart bool
	sep     string
	start   int64
	width   int
}

// style is the numbering style of a logical page section.
type style struct {
	kind byte
	re   *regexp.Regexp
}

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{
		body:    style{kind: 't'},
		header:  style{kind: 'n'},
		footer:  style{kind: 'n'},
		delim:   "\\:",
		incr:    1,
		join:    1,
		format:  'r',
		restart: true,
		sep:     "\t",
		start:   1,
		width:   6,
	}
	operands, err := getopt.Set{Valued: "bdfhilnsvw"}.Parse(args, func(opt rune, value string) error {
		var err error
		switch opt {
		case 'b':
			if opts.body, err = parseStyle(value, "body"); err != nil {
				return err
			}
		case 'd':
			switch len(value) {
			case 0:
				opts.delim = ""
			case 1:
				opts.delim = value + ":"
			default:
				opts.delim = value
			}
		case 'f':
			if opts.footer, err = parseStyle(value, "footer"); err != nil {
				return err
			}
		case 'h':
			if opts.header, err = parseStyle(value, "header"); err != nil {
				return err
			}
		case 'i':
			if opts.incr, err = parseNumber(value, "line number increment"); err != nil {
				return err
			}
		case 'l':
			if opts.join, err = parseNumber(value, "line number of blank lines"); err != nil {
				return err
			}
			if opts.join < 1 {
				return fmt.Errorf("invalid line number of blank lines: '%s'", value)
			}
		case 'n':
			switch value {
			case "ln":
				opts.format = 'l'
			case "rn":
				opts.format = 'r'
			case "rz":
				opts.format = 'z'
			default:
				return fmt.Errorf("invalid line numbering format: '%s'", value)
			}
		case 'p':
			opts.restart = false
		case 's':
			opts.sep = value
		case 'v':
			if opts.start, err = parseNumber(value, "starting line number"); err != nil {
				return err
			}
		case 'w':
			var width int64
			if width, err = parseNumber(value, "line number field width"); err != nil {
				return err
			}
			if width < 1 {
				return fmt.Errorf("invalid line number field width: '%s'", value)
			}
			opts.width = int(width)
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// parseStyle parses a numbering style: all lines, non-empty lines, no lines,
// or the lines matching a basic regular expression.
func parseStyle(value, section string) (style, error) {
	switch value {
	case "a", "t", "n":
		return style{kind: value[0]}, nil
	}
	if rest, ok := strings.CutPrefix(value, "p"); ok {
		re, err := bre.Compile(rest)
		if err != nil {
			return style{}, err
		}
		return style{kind: 'p', re: re}, nil
	}
	return style{}, fmt.Errorf("invalid %s numbering style: '%s'", section, value)
}

// parseNumber parses a decimal number.
func parseNumber(value, what string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: '%s'", what, value)
	}
	return n, nil
}

// label returns the name to use on diagnostic messages.
func label(name string) string {
	if name == "" {
		return "standard input"
	}
	return name
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
