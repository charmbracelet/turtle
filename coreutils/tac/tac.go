// Package tac implements the GNU tac utility.
//
// It copies each file, or the standard input when no operand is given or the
// operand is "-", to the standard output with the order of the records
// reversed, each file being handled separately. A record ends at an
// occurrence of the separator, the <newline> by default, which is attached
// to the end of the record it follows, or to the beginning of the record it
// precedes with -b. The separator of the -s option interprets backslash
// escapes, and an empty separator means the NUL byte. With -r, the separator
// is a regular expression using the syntax of Go's regexp package (RE2), with
// the anchors matching at newlines like GNU tac does.
package tac

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"charm.land/turtle/coreutils"

	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/input"
	"charm.land/turtle/coreutils/internal/numparse"
	"charm.land/turtle/coreutils/internal/patherr"
	"charm.land/turtle/coreutils/internal/stdopts"
)

// Prog is the tac core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes each given file to the standard output, last record first. The
// standard input is used when no operands are given, and also when an
// operand is "-". Each file is reversed separately, and a file that fails
// doesn't stop the ones after it.
//
// Reading and writing are interrupted when ctx is done, and the context
// error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "tac", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "tac", err)
	}

	ok := true
	for _, in := range input.List(operands) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !in.Stdin && in.Name == "" {
			stdopts.Errorf(p.Runtime, "tac", "invalid zero-length file name")
			ok = false
			continue
		}
		if err := p.tac(ctx, in, opts); err != nil {
			var writeErr *writeError
			switch {
			case errors.As(err, &writeErr):
				stdopts.Errorf(p.Runtime, "tac", "write error: %v", writeErr.err)
				return &coreutils.ExitError{Code: 1, Err: writeErr.err}
			case ctx.Err() != nil:
				return ctx.Err()
			default:
				stdopts.Errorf(p.Runtime, "tac", "%s: %v", in.Label(), patherr.Unwrap(err))
				ok = false
			}
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// tac writes the records of a single input to the standard output, last
// record first.
func (p *Prog) tac(ctx context.Context, in input.Input, opts options) error {
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		return err
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	records := splitRecords(data, opts)
	for _, record := range slices.Backward(records) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := p.Stdout.Write(record); err != nil {
			return &writeError{err: err}
		}
	}
	return nil
}

// parseFlags parses the options and operands. The -s option takes the
// separator, interpreting its backslash escapes; an empty separator means
// the NUL byte, and is an error with -r, which compiles the separator as a
// regular expression. Option parsing stops at the first operand or at "--".
var longOptions = map[string]rune{
	"before":    'b',
	"regex":     'r',
	"separator": 's',
}

// optSet describes the options of tac.
var optSet = getopt.Set{Valued: "s", Long: longOptions}

// usage describes the options of tac.
const usage = `Usage: tac [OPTION]... [FILE]...
Concatenate and write files in reverse, last record first.

With no FILE, or when FILE is -, read standard input.

  -b, --before
         attach the separator to the record it precedes
  -r, --regex
         treat the separator as a regular expression
  -s, --separator=STRING
         use STRING as the record separator; empty means a zero byte
` + stdopts.Tail

func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'b':
			opts.before = true
		case 'r':
			opts.regex = true
		case 's':
			opts.separatorSet = true
			opts.separator = unescape(value)
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}

	switch {
	case !opts.separatorSet:
		opts.separator = "\n"
	case opts.separator == "" && opts.regex:
		return options{}, nil, errors.New("separator cannot be empty with -r")
	case opts.separator == "":
		opts.separator = "\x00"
	}
	if opts.regex {
		re, err := regexp.Compile(`(?m)` + opts.separator)
		if err != nil {
			return options{}, nil, fmt.Errorf("invalid separator: %v", err)
		}
		opts.re = re
	}
	return opts, operands, nil
}

// unescape interprets the backslash escapes of the separator: \a, \b, \f,
// \n, \r, \t, \v, \\, and octal escapes of up to three digits. Any other
// backslash sequence is left as is, so regular expression escapes survive.
func unescape(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch c := s[i]; c {
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case '\\':
			b.WriteByte('\\')
		case '0', '1', '2', '3', '4', '5', '6', '7':
			n := int(c - '0')
			for digits := 1; digits < 3 && i+1 < len(s) && numparse.IsOctalDigit(s[i+1]); digits++ {
				i++
				n = n*8 + int(s[i]-'0')
			}
			b.WriteByte(byte(n))
		default:
			b.WriteByte('\\')
			b.WriteByte(c)
		}
	}
	return b.String()
}

// splitRecords splits data into records at the separator occurrences. Each
// record holds the separator that ends it, or begins it with before. A
// final record without a separator is kept, and a trailing separator ends
// the last record instead of starting an empty one.
func splitRecords(data []byte, opts options) [][]byte {
	var records [][]byte
	prev := 0
	for _, m := range opts.matches(data) {
		end := m[1]
		if opts.before {
			end = m[0]
		}
		records = append(records, data[prev:end])
		prev = end
	}
	if prev < len(data) {
		records = append(records, data[prev:])
	}
	return records
}

// matches returns the separator occurrences of data, in file order. Fixed
// separators match right-aligned and without overlapping, like the GNU
// backward scan; regular expressions match leftmost, as Go's regexp package
// does.
func (o options) matches(data []byte) [][]int {
	if o.re != nil {
		return o.re.FindAllIndex(data, -1)
	}
	sep := []byte(o.separator)
	var matches [][]int
	for end := len(data); end > 0; {
		j := bytes.LastIndex(data[:end], sep)
		if j < 0 {
			break
		}
		matches = append(matches, []int{j, j + len(sep)})
		end = j
	}
	slices.Reverse(matches)
	return matches
}

// options holds how the records are separated.
type options struct {
	before       bool           // -b: attach the separator before the record
	regex        bool           // -r: the separator is a regular expression
	separatorSet bool           // -s given
	separator    string         // record separator, "\n" by default, "\x00" when empty
	re           *regexp.Regexp // separator compiled, with -r
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
