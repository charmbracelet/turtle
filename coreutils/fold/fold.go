// Package fold implements the POSIX fold utility.
//
// It folds the lines of the given files, breaking them with newlines so that
// no output line exceeds the given width, counted in bytes with -b or in
// column positions otherwise. The standard input is used when no operands are
// given, and also when an operand is "-".
package fold

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the fold core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run folds the lines of the given files and writes them to the standard
// output. In column mode, backspaces decrement the column, down to zero,
// carriage returns reset it to zero, and tabs advance it to the next multiple
// of 8; each other character counts as one column. A line is never broken in
// the middle of a character.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "fold", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "fold")
		return &coreutils.ExitError{Code: 2}
	}

	inputs := make([]input.Input, 0, len(operands))
	for _, operand := range operands {
		inputs = append(inputs, input.New(operand))
	}
	if len(inputs) == 0 {
		inputs = append(inputs, input.Input{Stdin: true})
	}

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
		if err := p.fold(ctx, in, opts); err != nil {
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
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// fold folds the lines of a single input.
func (p *Prog) fold(ctx context.Context, in input.Input, opts options) error {
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		return err
	}
	defer r.Close()

	br := bufio.NewReader(r)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			terminated := strings.HasSuffix(line, "\n")
			if cerr := p.foldLine([]byte(strings.TrimSuffix(line, "\n")), opts, terminated); cerr != nil {
				return cerr
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

// foldLine writes a line broken into segments of at most the given width.
func (p *Prog) foldLine(line []byte, opts options, terminated bool) error {
	var seg []byte
	col := 0
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRune(line[i:])
		// Place the character, breaking the line as needed. It's retried on
		// the new line, as its width may depend on the column it lands on.
		for {
			next := adjustColumn(col, r, size, opts.bytes)
			if next <= opts.width {
				col = next
				break
			}
			if opts.spaces {
				if end, ok := lastBlank(seg); ok {
					// Break after the last blank of the segment.
					if err := p.writeSegment(seg[:end], true); err != nil {
						return err
					}
					seg = append(seg[:0], seg[end:]...)
					col = segmentColumns(seg, opts.bytes)
					continue
				}
			}
			if len(seg) == 0 {
				// A character wider than the whole line is never split.
				col = next
				break
			}
			if err := p.writeSegment(seg, true); err != nil {
				return err
			}
			seg = seg[:0]
			col = 0
		}
		seg = append(seg, line[i:i+size]...)
		i += size
	}
	return p.writeSegment(seg, terminated)
}

// adjustColumn returns the column the cursor lands on after the character,
// from column col. In bytes mode, each byte of the character counts.
func adjustColumn(col int, r rune, size int, countBytes bool) int {
	if countBytes {
		return col + size
	}
	switch r {
	case '\b':
		if col > 0 {
			col--
		}
		return col
	case '\r':
		return 0
	case '\t':
		return col + 8 - col%8
	default:
		return col + 1
	}
}

// segmentColumns returns the column a segment leaves the cursor on.
func segmentColumns(seg []byte, countBytes bool) int {
	col := 0
	for i := 0; i < len(seg); {
		r, size := utf8.DecodeRune(seg[i:])
		col = adjustColumn(col, r, size, countBytes)
		i += size
	}
	return col
}

// lastBlank returns the offset just past the last blank of a segment,
// reporting whether it has one.
func lastBlank(seg []byte) (int, bool) {
	end := 0
	for i, c := range seg {
		if c == ' ' || c == '\t' {
			end = i + 1
		}
	}
	return end, end != 0
}

// writeSegment writes a segment to the standard output, followed by a newline
// when it ends a line.
func (p *Prog) writeSegment(seg []byte, newline bool) error {
	out := make([]byte, len(seg), len(seg)+1)
	copy(out, seg)
	if newline {
		out = append(out, '\n')
	}
	if _, err := p.Stdout.Write(out); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "fold: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	bytes  bool
	spaces bool
	width  int
}

// longOptions maps the long options of fold to the short ones they stand for.
var longOptions = map[string]rune{
	"bytes":  'b',
	"spaces": 's',
	"width":  'w',
}

// optSet describes the options of fold.
var optSet = getopt.Set{Valued: "w", Long: longOptions}

// usage describes the options of fold.
const usage = `Usage: fold [OPTION]... [FILE]...
Wrap input lines to fit in specified width.

Writes each FILE ('-' means standard input), or standard input if none are
given, to standard output, breaking lines so that no output line is wider
than the limit, 80 columns by default.

  -b, --bytes         count bytes rather than columns
  -s, --spaces        break at word boundaries, after the last blank
  -w, --width=WIDTH   use WIDTH columns instead of 80
      --help     display this help and exit
      --version  output version information and exit
`

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{width: 80}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'b':
			opts.bytes = true
		case 's':
			opts.spaces = true
		case 'w':
			width, err := parseWidth(value)
			if err != nil {
				return err
			}
			opts.width = width
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// parseWidth parses a positive decimal width.
func parseWidth(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid width: '%s'", s)
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
