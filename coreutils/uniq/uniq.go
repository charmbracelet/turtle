// Package uniq implements the POSIX uniq utility.
//
// It compares the adjacent lines of the input, writing one copy of each
// repeated group to the output. The input is the standard input when no
// input operand is given or it's "-"; the output is the standard output when
// no output operand is given or it's "-".
package uniq

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"unicode/utf8"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/textutil"
)

// Prog is the uniq core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run filters the adjacent repeated lines of the input, writing one copy of
// each group to the output, as selected by the options.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "uniq", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "uniq")
		return &coreutils.ExitError{Code: 2}
	}
	var inName, outName string
	switch len(operands) {
	case 0:
	case 1:
		inName = operands[0]
	case 2:
		inName, outName = operands[0], operands[1]
	default:
		p.errorf("extra operand '%s'", operands[2])
		stdopts.TryHelp(p.Runtime, "uniq")
		return &coreutils.ExitError{Code: 2}
	}
	if slices.Contains(operands, "") {
		p.errorf("invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}

	in := input.Input{Name: inName, Stdin: inName == "" || inName == "-"}
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1}
	}
	defer r.Close()

	out, err := p.openOutput(outName)
	if err != nil {
		p.errorf("%s: %v", outName, patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1}
	}

	if err := p.uniq(ctx, r, out, opts); err != nil {
		var writeErr *writeError
		switch {
		case errors.As(err, &writeErr):
			p.errorf("write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 1, Err: writeErr.err}
		case ctx.Err() != nil:
			return ctx.Err()
		default:
			p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 1}
		}
	}
	return nil
}

// uniq copies the groups of adjacent equal items of r to out, as the options
// demand. Items are delimited by the byte the options pick.
func (p *Prog) uniq(ctx context.Context, r io.Reader, out io.Writer, opts options) error {
	br := bufio.NewReader(r)
	delim := opts.delimiter()
	term := []byte{delim}
	var (
		group    [][]byte
		prevKey  []byte
		havePrev bool
	)
	w := groupWriter{out: out, opts: opts}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := br.ReadString(delim)
		if len(item) > 0 {
			raw := []byte(item)
			key := opts.key(bytes.TrimSuffix(raw, term))
			if havePrev && bytes.Equal(prevKey, key) {
				group = append(group, raw)
			} else {
				if havePrev {
					if werr := w.write(group); werr != nil {
						return werr
					}
				}
				group, prevKey, havePrev = [][]byte{raw}, key, true
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}
	if havePrev {
		return w.write(group)
	}
	return nil
}

// groupWriter writes the groups of adjacent equal items to an output, as the
// options demand. It remembers whether a group was already written, to
// separate the ones -D delimits.
type groupWriter struct {
	out   io.Writer
	opts  options
	wrote bool
}

// write writes a group of count equal items, as the options demand.
func (w *groupWriter) write(group [][]byte) error {
	count := len(group)
	if w.opts.uniqueOnly && count > 1 || w.opts.repeatedOnly && count == 1 {
		return nil
	}
	if w.opts.allRepeated {
		return w.writeRepeated(group)
	}
	if w.opts.count {
		if _, err := fmt.Fprintf(w.out, "%7d ", count); err != nil {
			return &writeError{err: err}
		}
	}
	if _, err := w.out.Write(group[0]); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// writeRepeated writes every item of a group of repeated ones, delimited as
// the delimit method of the -D option asks. Groups of a single item are
// discarded.
func (w *groupWriter) writeRepeated(group [][]byte) error {
	if len(group) == 1 {
		return nil
	}
	if w.opts.delimit == delimitPrepend || w.opts.delimit == delimitSeparate && w.wrote {
		if _, err := w.out.Write([]byte{w.opts.delimiter()}); err != nil {
			return &writeError{err: err}
		}
	}
	for _, item := range group {
		if _, err := w.out.Write(item); err != nil {
			return &writeError{err: err}
		}
	}
	w.wrote = true
	return nil
}

// key returns the part of the line which takes part in comparisons: the line
// with the first opts.fields fields and opts.chars characters skipped, cut
// to the first opts.checkChars characters when set, and with the case of its
// ASCII letters folded when -i was given.
func (opts options) key(line []byte) []byte {
	key := textutil.SkipChars(opts.skipFields(line, opts.fields), opts.chars)
	if opts.checkChars >= 0 {
		key = limitChars(key, opts.checkChars)
	}
	if opts.ignoreCase {
		key = foldCase(key)
	}
	return key
}

// skipFields returns the line with its first n fields skipped. With
// NUL-terminated items, a newline separates fields as well.
func (opts options) skipFields(line []byte, n int) []byte {
	if !opts.zeroTerm {
		return textutil.SkipFields(line, n)
	}
	blank := func(c byte) bool {
		return textutil.IsBlank(c) || c == '\n'
	}
	for ; n > 0 && len(line) > 0; n-- {
		i := 0
		for i < len(line) && blank(line[i]) {
			i++
		}
		for i < len(line) && !blank(line[i]) {
			i++
		}
		line = line[i:]
	}
	return line
}

// delimiter returns the byte which terminates the items: a newline, or a NUL
// with -z.
func (opts options) delimiter() byte {
	if opts.zeroTerm {
		return 0
	}
	return '\n'
}

// limitChars returns s cut to its first n characters, decoding it as UTF-8.
func limitChars(s []byte, n int) []byte {
	i := 0
	for ; n > 0 && i < len(s); n-- {
		_, size := utf8.DecodeRune(s[i:])
		i += size
	}
	return s[:i]
}

// foldCase lowercases the ASCII letters of s.
func foldCase(s []byte) []byte {
	folded := make([]byte, len(s))
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		folded[i] = c
	}
	return folded
}

// openOutput returns a writer for the output operand. An empty name or "-"
// mean the standard output.
func (p *Prog) openOutput(name string) (io.Writer, error) {
	if name == "" || name == "-" {
		return p.Stdout, nil
	}
	if p.FS == nil {
		return nil, errors.New("no file system available")
	}
	return p.FS.OpenFile(filepath.ToSlash(p.ResolvePath(name)), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666&^p.Umask.Perm())
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "uniq: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	count        bool
	repeatedOnly bool
	uniqueOnly   bool
	allRepeated  bool
	delimit      delimitMethod
	fields       int
	chars        int
	checkChars   int // -1 when -w wasn't given
	ignoreCase   bool
	zeroTerm     bool
}

// delimitMethod is the way the groups of repeated items are delimited by the
// -D option.
type delimitMethod int

const (
	delimitNone     delimitMethod = iota // no delimiter
	delimitPrepend                       // a delimiter before each group
	delimitSeparate                      // a delimiter between groups
)

// optAllRepeated is the option character the --all-repeated long option maps
// to. It stands for no short option: -D takes its optional argument only
// when the long form carries it attached, as in --all-repeated=separate.
const optAllRepeated = '\x00'

// longOptions maps the long options of uniq to the short ones they stand for.
var longOptions = map[string]rune{
	"count":           'c',
	"repeated":        'd',
	"unique":          'u',
	"skip-fields":     'f',
	"skip-chars":      's',
	"ignore-case":     'i',
	"all-repeated":    optAllRepeated,
	"check-chars":     'w',
	"zero-terminated": 'z',
}

// optSet describes the options of uniq.
var optSet = getopt.Set{
	Valued:   "fsw",
	Optional: string(optAllRepeated),
	Long:     longOptions,
}

// usage describes the options of uniq.
const usage = `Usage: uniq [OPTION]... [INPUT [OUTPUT]]
Filter adjacent matching lines from INPUT (or standard input),
writing to OUTPUT (or standard output).

With no options, matching lines are merged to the first occurrence.

  -c, --count           prefix lines by the number of occurrences
  -d, --repeated        only print duplicate lines, one for each group
  -D, --all-repeated[=METHOD]
                        print all duplicate lines, delimiting the groups
                        with METHOD: none (default), prepend or separate
  -f, --skip-fields=N   avoid comparing the first N fields
  -i, --ignore-case     ignore differences in case when comparing
  -s, --skip-chars=N    avoid comparing the first N characters
  -u, --unique          only print unique lines
  -w, --check-chars=N   compare no more than N characters in lines
  -z, --zero-terminated line delimiter is NUL, not newline
      --help     display this help and exit
      --version  output version information and exit
`

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	opts := options{checkChars: -1}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'c':
			opts.count = true
		case 'd':
			opts.repeatedOnly = true
		case 'u':
			opts.uniqueOnly = true
		case 'D':
			opts.allRepeated = true
			opts.delimit = delimitNone
		case optAllRepeated:
			method, err := parseDelimit(value)
			if err != nil {
				return err
			}
			opts.allRepeated = true
			opts.delimit = method
		case 'i':
			opts.ignoreCase = true
		case 'z':
			opts.zeroTerm = true
		case 'f':
			n, err := parseSkip(value, "fields")
			if err != nil {
				return err
			}
			opts.fields = n
		case 's':
			n, err := parseSkip(value, "characters")
			if err != nil {
				return err
			}
			opts.chars = n
		case 'w':
			n, err := parseSkip(value, "characters to compare")
			if err != nil {
				return err
			}
			opts.checkChars = n
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	switch {
	case opts.repeatedOnly && opts.uniqueOnly:
		return options{}, nil, errors.New("printing only duplicated and unique lines is not supported")
	case opts.allRepeated && opts.uniqueOnly:
		return options{}, nil, errors.New("printing all duplicated and unique lines is not supported")
	case opts.count && opts.allRepeated:
		return options{}, nil, errors.New("printing all duplicated lines and repeat counts is meaningless")
	}
	return opts, operands, nil
}

// parseDelimit parses the delimit method of the --all-repeated option, where
// an empty value means none.
func parseDelimit(s string) (delimitMethod, error) {
	switch s {
	case "", "none":
		return delimitNone, nil
	case "prepend":
		return delimitPrepend, nil
	case "separate":
		return delimitSeparate, nil
	}
	return delimitNone, fmt.Errorf("invalid argument '%s' for '--all-repeated'", s)
}

// parseSkip parses a non-negative decimal count of fields or characters to
// skip.
func parseSkip(s string, what string) (int, error) {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("invalid number of %s: '%s'", what, s)
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid number of %s: '%s'", what, s)
	}
	return n, nil
}

// writeError marks an error as coming from writing to the output.
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
