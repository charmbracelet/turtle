// Package wc implements the POSIX wc utility.
//
// It reads one or more input files, or the standard input when no operands
// are given, and writes the number of <newline> characters, words and bytes
// of each input. A total line is written when more than one file is given,
// and the --total option controls when.
//
// A word is a non-zero-length string of characters delimited by ASCII white
// space (space, tab, newline, vertical tab, form feed and carriage return).
// The -m option counts characters decoded as UTF-8, with each byte that is
// not part of a valid encoding counted as a single character.
//
// The -c and -m options don't undo each other, following the GNU manual, so
// both the character and the byte counts are written when both are given.
//
// The -L option writes the length of the longest line. Like GNU wc in the C
// locale, the unit is the byte, not the UTF-8 character the -m option counts:
// every ASCII printable byte counts one column, a tab advances to the next
// multiple of 8 columns, a carriage return and a form feed also end a line,
// and a trailing line without a newline is counted. Bytes that aren't ASCII
// printable, like those of a multibyte character, count no columns. With
// more than one input, the maximum of the lengths, not their sum, is written.
package wc

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

// Prog is the wc core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the number of <newline> characters, words and bytes of each
// input file to standard output, or only the counts the options select. The
// standard input is used when no file operands are given, and also when an
// operand is "-".
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "wc", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if !opts.any() {
		opts.lines, opts.words, opts.bytes = true, true, true
	}

	inputs := make([]input.Input, 0, len(operands))
	for _, operand := range operands {
		inputs = append(inputs, input.New(operand))
	}
	if len(inputs) == 0 {
		inputs = append(inputs, input.Input{Stdin: true})
	}

	var total counts
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
		got, err := p.count(ctx, in, opts)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
			ok = false
			continue
		}
		if opts.total != totalOnly {
			if err := p.write(got, in.Name, opts); err != nil {
				return err
			}
		}
		total.add(got)
	}
	if p.writeTotal(opts, len(inputs)) {
		name := "total"
		if opts.total == totalOnly {
			name = ""
		}
		if err := p.write(total, name, opts); err != nil {
			return err
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// writeTotal reports whether the line with cumulative counts is written for
// inputs inputs.
func (p *Prog) writeTotal(opts options, inputs int) bool {
	switch opts.total {
	case totalAlways, totalOnly:
		return true
	case totalNever:
		return false
	default:
		return inputs > 1
	}
}

// count reads in and returns its counts.
func (p *Prog) count(ctx context.Context, in input.Input, opts options) (counts, error) {
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		return counts{}, err
	}
	defer r.Close()

	var s scanner
	if opts.chars {
		err = countChars(r, &s)
	} else {
		err = countBytes(r, &s)
	}
	return s.c, err
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
	if opts.maxLine {
		add(c.maxLength)
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

// optTotal is the rune the long-only --total option maps to. Having no short
// equivalent, it's kept above the bytes a group of short options can hold.
const optTotal rune = 0x100

// longOptions maps the long options of wc to the short ones they stand for.
var longOptions = map[string]rune{
	"bytes":           'c',
	"chars":           'm',
	"lines":           'l',
	"max-line-length": 'L',
	"total":           optTotal,
	"words":           'w',
}

// optSet describes the options of wc.
var optSet = getopt.Set{Valued: string(optTotal), Long: longOptions}

// usage describes the options of wc.
const usage = `Usage: wc [OPTION]... [FILE]...
Print newline, word, and byte counts for each FILE, and a total line if
more than one FILE is specified. A word is a non-zero-length sequence of
characters delimited by white space.

  -c, --bytes            print byte counts
  -m, --chars            print character counts
  -l, --lines            print newline counts
  -w, --words            print word counts
  -L, --max-line-length  print the maximum display width
      --total=WHEN       print a line with totals; WHEN can be: auto, always,
                         only, or never (default: auto)
      --help     display this help and exit
      --version  output version information and exit
`

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--". Following the GNU manual, options don't undo
// each other, so -c and -m together select both counts.
func parseFlags(args []string) (options, []string, error) {
	opts := options{total: totalAuto}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'c':
			opts.bytes = true
		case 'l':
			opts.lines = true
		case 'L':
			opts.maxLine = true
		case 'm':
			opts.chars = true
		case 'w':
			opts.words = true
		case optTotal:
			if err := opts.setTotal(value); err != nil {
				return err
			}
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

// A totalMode tells when the line with cumulative counts is written.
type totalMode int

const (
	totalAuto totalMode = iota
	totalAlways
	totalOnly
	totalNever
)

// setTotal applies the word --total stands for: when the line with cumulative
// counts is written.
func (o *options) setTotal(word string) error {
	switch word {
	case "auto":
		o.total = totalAuto
	case "always":
		o.total = totalAlways
	case "only":
		o.total = totalOnly
	case "never":
		o.total = totalNever
	default:
		return fmt.Errorf("invalid argument '%s' for '--total'", word)
	}
	return nil
}

// options holds which counts the -c, -l, -L, -m and -w options select, and
// when the total is written.
type options struct {
	lines, words, bytes, chars, maxLine bool
	total                               totalMode
}

// any reports whether any count is selected.
func (o options) any() bool {
	return o.lines || o.words || o.bytes || o.chars || o.maxLine
}

// counts holds the counts of one input.
type counts struct {
	lines, words, bytes, chars, maxLength int
}

// add adds the counts of another input to c. The maximum line length is the
// maximum of both, not the sum.
func (c *counts) add(other counts) {
	c.lines += other.lines
	c.words += other.words
	c.bytes += other.bytes
	c.chars += other.chars
	if other.maxLength > c.maxLength {
		c.maxLength = other.maxLength
	}
}

// scanner holds the counts of an input and the state of the loops feeding
// bytes or characters into it.
type scanner struct {
	c       counts
	inWord  bool
	lineLen int
}

// byte updates the counts with one ASCII byte of input.
func (s *scanner) byte(b byte) {
	if b == '\n' {
		s.c.lines++
	}
	if isSpace(b) {
		s.inWord = false
	} else if !s.inWord {
		s.inWord = true
		s.c.words++
	}
	switch b {
	case '\n', '\r', '\f':
		s.endLine()
	case '\t':
		s.lineLen += 8 - s.lineLen%8
	case '\v':
	default:
		if isPrint(b) {
			s.lineLen++
		}
	}
}

// rune updates the counts with one character decoded as UTF-8. Only ASCII
// contributes to the line length, as bytes beyond it count no columns in the
// C locale.
func (s *scanner) rune(ru rune) {
	if ru >= utf8.RuneSelf {
		if !s.inWord {
			s.inWord = true
			s.c.words++
		}
		return
	}
	s.byte(byte(ru))
}

// endLine folds the current line length into the maximum and resets it.
func (s *scanner) endLine() {
	if s.lineLen > s.c.maxLength {
		s.c.maxLength = s.lineLen
	}
	s.lineLen = 0
}

// eof folds the trailing line, unterminated or not, into the maximum line
// length.
func (s *scanner) eof() {
	s.endLine()
}

// countBytes counts lines, words, bytes and the longest line length while
// reading r.
func countBytes(r io.Reader, s *scanner) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] {
			s.byte(b)
			s.c.bytes++
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.eof()
				return nil
			}
			return err
		}
	}
}

// countChars counts lines, words, bytes, characters and the longest line
// length while reading r, decoding it as UTF-8. Each byte that does not start
// a valid encoding counts as one character.
func countChars(r io.Reader, s *scanner) error {
	br := bufio.NewReader(r)
	for {
		ru, size, err := br.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.eof()
				return nil
			}
			return err
		}
		s.rune(ru)
		s.c.chars++
		s.c.bytes += size
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

// isPrint reports whether c is an ASCII printable character, the set isprint
// considers in the C locale.
func isPrint(c byte) bool {
	return ' ' <= c && c <= '~'
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
