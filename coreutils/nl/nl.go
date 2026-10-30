// Package nl implements the POSIX nl utility with the GNU extensions.
//
// It numbers the lines of the given files, treated as a single document, on
// the left, in terms of logical pages. A logical page consists of header,
// body and footer sections, each with its own numbering style, started by
// input lines holding nothing but the delimiter characters ("\:\:\:",
// "\:\:" and "\:" by default). Line numbering restarts at the start of each
// section, unless -p is given. When a line isn't numbered, a blank number
// field of the given width and separator takes its place; delimiter lines are
// reproduced as empty lines.
package nl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/bre"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the nl core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the lines of the given files to the standard output, adding line
// numbers according to the numbering style of the logical page section each
// line is in. The files are treated as a single document, so numbering isn't
// reset between them, and a file which can't be read is reported while the
// remaining ones are still processed. It returns an exit status of 1 for any
// error.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "nl", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "nl")
		return &coreutils.ExitError{Code: 1}
	}

	n := &numberer{opts: opts, out: bufio.NewWriter(p.Stdout)}
	n.reset()
	n.style = opts.body

	ok := true
	for _, in := range input.List(operands) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !in.Stdin && in.Name == "" {
			p.errorf("invalid zero-length file name")
			ok = false
			continue
		}
		if err := p.number(ctx, in, n); err != nil {
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
				p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
				ok = false
			}
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// number reads the lines of the input and writes them numbered, continuing
// the numbering state held by n.
func (p *Prog) number(ctx context.Context, in input.Input, n *numberer) error {
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		return err
	}
	defer r.Close()

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
		case body:
			n.style = n.opts.body
		case footer:
			n.style = n.opts.footer
		}
		if n.opts.restart {
			n.reset()
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
	} else if err := n.printNoLine(); err != nil {
		return err
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

// printNoLine writes a blank number field for an unnumbered line.
func (n *numberer) printNoLine() error {
	if _, err := fmt.Fprintf(n.out, "%*s%s", n.opts.width, "", n.opts.sep); err != nil {
		return &writeError{err: err}
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

// longOptions maps the long options of nl to the short ones they stand for.
var longOptions = map[string]rune{
	"body-numbering":       'b',
	"footer-numbering":     'f',
	"header-numbering":     'h',
	"join-blank-lines":     'l',
	"line-increment":       'i',
	"no-renumber":          'p',
	"number-format":        'n',
	"number-separator":     's',
	"number-width":         'w',
	"section-delimiter":    'd',
	"starting-line-number": 'v',
}

// optSet describes the options of nl.
var optSet = getopt.Set{Valued: "bdfhilnsvw", Long: longOptions}

// usage describes the options of nl.
const usage = `Usage: nl [OPTION]... [FILE]...
Number lines of files and write to standard output.

Each logical page has a header, body and footer section, delimited by input
lines holding only '\:\:\:', '\:\:' or '\:', and numbered in its own style.
The input files are treated as a single document.

  -b, --body-numbering=STYLE         use STYLE for numbering body lines
  -d, --section-delimiter=CC         use CC for logical page delimiters
  -f, --footer-numbering=STYLE       use STYLE for numbering footer lines
  -h, --header-numbering=STYLE       use STYLE for numbering header lines
  -i, --line-increment=NUMBER        line number increment at each line
  -l, --join-blank-lines=NUMBER      group of NUMBER empty lines counted as one
  -n, --number-format=FORMAT         insert line numbers according to FORMAT
  -p, --no-renumber                  do not reset line numbers at each section
  -s, --number-separator=STRING      add STRING after (possible) line number
  -v, --starting-line-number=NUMBER  first line number of each logical page
  -w, --number-width=NUMBER          use NUMBER columns for line numbers

STYLE is one of: a (all lines), t (nonempty lines), n (no lines) or pBRE
(number only lines matching the basic regular expression BRE). FORMAT is
one of: ln (left justified), rn (right justified) or rz (right justified,
with leading zeros).

      --help
         display this help and exit
      --version
         output version information and exit
`

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
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
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

var _ coreutils.Program = (*Prog)(nil)
