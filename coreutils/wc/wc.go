// Package wc implements the POSIX wc utility.
//
// It reads one or more input files, or the standard input when no operands
// are given, and writes the number of <newline> characters, words and bytes
// of each input. A total line is written when more than one file is given.
//
// A word is a non-zero-length string of characters delimited by ASCII white
// space (space, tab, newline, vertical tab, form feed and carriage return).
// The -m option counts characters decoded as UTF-8, with each byte that is
// not part of a valid encoding counted as a single character.
package wc

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

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/getopt"
)

// Prog is the wc core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the number of <newline> characters, words and bytes of each
// input file to standard output. The standard input is used when no file
// operands are given, and also when an operand is "-".
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if !opts.any() {
		opts = options{lines: true, words: true, bytes: true}
	}

	inputs := make([]input, 0, len(operands))
	for _, operand := range operands {
		inputs = append(inputs, newInput(operand))
	}
	if len(inputs) == 0 {
		inputs = append(inputs, input{stdin: true})
	}

	var total counts
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
		got, err := p.count(ctx, in, opts)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			p.errorf("%s: %v", in.label(), unwrapPathError(err))
			ok = false
			continue
		}
		if err := p.write(got, in.name, opts); err != nil {
			return err
		}
		total.add(got)
	}
	if len(inputs) > 1 {
		if err := p.write(total, "total", opts); err != nil {
			return err
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// count reads in and returns its counts.
func (p *Prog) count(ctx context.Context, in input, opts options) (counts, error) {
	r, err := p.open(ctx, in)
	if err != nil {
		return counts{}, err
	}
	defer r.Close()

	var c counts
	if opts.chars {
		err = countChars(r, &c)
	} else {
		err = countBytes(r, &c)
	}
	return c, err
}

// open returns a reader for the input. Reads stop early when ctx is done. The
// standard input isn't owned by the caller, so it's wrapped on a no-op closer.
func (p *Prog) open(ctx context.Context, in input) (io.ReadCloser, error) {
	if in.stdin {
		if p.Stdin == nil {
			return nil, errors.New("no standard input available")
		}
		return ctxio.NewReader(ctx, p.Stdin), nil
	}
	if p.FS == nil {
		return nil, errors.New("no file system available")
	}
	f, err := p.FS.Open(filepath.ToSlash(p.ResolvePath(in.name)))
	if err != nil {
		return nil, err
	}
	return ctxio.ReadCloser{Ctx: ctx, RC: f}, nil
}

// write writes a line with the requested counts, followed by name when it's
// not empty.
func (p *Prog) write(c counts, name string, opts options) error {
	var b strings.Builder
	fields := 0
	add := func(v int) {
		if fields > 0 {
			b.WriteByte(' ')
		}
		fields++
		b.WriteString(strconv.Itoa(v))
	}
	if opts.lines {
		add(c.lines)
	}
	if opts.words {
		add(c.words)
	}
	if opts.chars {
		add(c.chars)
	}
	if opts.bytes {
		add(c.bytes)
	}
	if name != "" {
		if fields > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(name)
	}
	b.WriteByte('\n')
	if _, err := io.WriteString(p.Stdout, b.String()); err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "wc: "+format+"\n", args...)
}

// longOptions maps the long options of wc to the short ones they stand for.
var longOptions = map[string]rune{
	"bytes": 'c',
	"chars": 'm',
	"lines": 'l',
	"words": 'w',
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--". Following the POSIX description, -m replaces the
// byte count, discarding -c no matter the order the options are given.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := getopt.Set{Long: longOptions}.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'c':
			opts.bytes = true
		case 'l':
			opts.lines = true
		case 'm':
			opts.chars = true
		case 'w':
			opts.words = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	if opts.chars {
		opts.bytes = false
	}
	return opts, operands, nil
}

// input describes an input operand: a pathname or the standard input.
type input struct {
	name  string
	stdin bool
}

// newInput maps an operand to an input, reading "-" as the standard input.
func newInput(operand string) input {
	return input{name: operand, stdin: operand == "-"}
}

// label returns the name to use on diagnostic messages.
func (in input) label() string {
	if in.stdin && in.name == "" {
		return "standard input"
	}
	return in.name
}

// options holds which counts the -c, -l, -m and -w options select.
type options struct {
	lines, words, bytes, chars bool
}

// any reports whether any count is selected.
func (o options) any() bool {
	return o.lines || o.words || o.bytes || o.chars
}

// counts holds the counts of one input.
type counts struct {
	lines, words, bytes, chars int
}

// add adds the counts of another input to c.
func (c *counts) add(other counts) {
	c.lines += other.lines
	c.words += other.words
	c.bytes += other.bytes
	c.chars += other.chars
}

// countBytes counts lines, words and bytes while reading r.
func countBytes(r io.Reader, c *counts) error {
	buf := make([]byte, 32*1024)
	inWord := false
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] {
			if b == '\n' {
				c.lines++
			}
			if isSpace(b) {
				inWord = false
			} else if !inWord {
				inWord = true
				c.words++
			}
		}
		c.bytes += n
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// countChars counts lines, words and characters while reading r, decoding it
// as UTF-8. Each byte that does not start a valid encoding counts as one
// character.
func countChars(r io.Reader, c *counts) error {
	br := bufio.NewReader(r)
	inWord := false
	for {
		ru, _, err := br.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		c.chars++
		if ru == '\n' {
			c.lines++
		}
		if ru < utf8.RuneSelf && isSpace(byte(ru)) {
			inWord = false
		} else if !inWord {
			inWord = true
			c.words++
		}
	}
}

// isSpace reports whether c is an ASCII white space character, the set
// isspace considers in the POSIX locale.
func isSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	default:
		return false
	}
}

// unwrapPathError unwraps *fs.PathError so diagnostics don't repeat the file
// name.
func unwrapPathError(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
