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
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
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
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}

	inputs := make([]input, 0, len(operands))
	for _, operand := range operands {
		inputs = append(inputs, input{name: operand, stdin: operand == "-"})
	}
	if len(inputs) == 0 {
		inputs = append(inputs, input{stdin: true})
	}

	ok := true
	for _, in := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !in.stdin && in.name == "" {
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
				p.errorf("%s: %v", in.label(), unwrapPathError(err))
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
func (p *Prog) fold(ctx context.Context, in input, opts options) error {
	r, err := p.open(ctx, in)
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

// open returns a reader for the input. Reads stop early when ctx is done. The
// standard input isn't owned by the caller, so it's wrapped on a no-op closer.
func (p *Prog) open(ctx context.Context, in input) (io.ReadCloser, error) {
	if in.stdin {
		if p.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return ctxReadCloser{ctx: ctx, rc: io.NopCloser(p.Stdin)}, nil
	}
	if p.FS == nil {
		return nil, errors.New("no file system available")
	}
	f, err := p.FS.Open(filepath.ToSlash(p.ResolvePath(in.name)))
	if err != nil {
		return nil, err
	}
	return ctxReadCloser{ctx: ctx, rc: f}, nil
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

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{width: 80}
	operands, err := getopt.Set{Valued: "w", Long: longOptions}.Parse(args, func(opt rune, value string) error {
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

// input describes an input operand: a pathname or the standard input.
type input struct {
	name  string
	stdin bool
}

// label returns the name to use on diagnostic messages.
func (in input) label() string {
	if in.stdin && in.name == "" {
		return "standard input"
	}
	return in.name
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
