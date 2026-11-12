// Package join implements the POSIX join utility.
//
// The join fields are compared in the bytewise collating sequence of the C
// locale, the same one the sort utility of this project uses, so files
// sorted by it match: characters which only a locale's LC_COLLATE makes
// collate equally stay different. The -i option ignores the case of ASCII
// letters in the join fields, the way sort -f does in the C locale.
package join

import (
	"bufio"
	"bytes"
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
	"charm.land/snail/coreutils/internal/numparse"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/textutil"
)

// Prog is the join core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run joins the lines of the two files with equal join fields, writing the
// result to the standard output.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "join", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		stdopts.Errorf(p.Runtime, "join", "%v", err)
		stdopts.TryHelp(p.Runtime, "join")
		return &coreutils.ExitError{Code: 1}
	}
	if err := checkOperands(operands); err != nil {
		stdopts.Errorf(p.Runtime, "join", "%v", err)
		stdopts.TryHelp(p.Runtime, "join")
		return &coreutils.ExitError{Code: 1}
	}
	r1, err := input.Open(p.Runtime, ctx, input.New(operands[0]))
	if err != nil {
		stdopts.Errorf(p.Runtime, "join", "%s: %v", operands[0], patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1}
	}
	r2, err := input.Open(p.Runtime, ctx, input.New(operands[1]))
	if err != nil {
		r1.Close()
		stdopts.Errorf(p.Runtime, "join", "%s: %v", operands[1], patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1}
	}
	err = p.joinFiles(ctx, file{name: operands[0], r: newLineReader(r1)}, file{name: operands[1], r: newLineReader(r2)}, opts)
	for _, closeErr := range []error{r1.Close(), r2.Close()} {
		if err == nil {
			err = closeErr
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		stdopts.Errorf(p.Runtime, "join", "%v", patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// joinFiles merges the files on their join fields, writing the pairs with
// equal fields, and the unpaired lines the options select.
func (p *Prog) joinFiles(ctx context.Context, f1, f2 file, opts options) error {
	line1, ok1, err := f1.next()
	if err != nil {
		return f1.wrap(err)
	}
	line2, ok2, err := f2.next()
	if err != nil {
		return f2.wrap(err)
	}
	for ok1 && ok2 {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch compareFields(field(line1, opts.key1, opts.sep), field(line2, opts.key2, opts.sep), opts.ignoreCase) {
		case -1:
			if opts.unpaired1 {
				if err := p.writeLine(line1, nil, opts); err != nil {
					return err
				}
			}
			if line1, ok1, err = f1.next(); err != nil {
				return f1.wrap(err)
			}
		case 1:
			if opts.unpaired2 {
				if err := p.writeLine(nil, line2, opts); err != nil {
					return err
				}
			}
			if line2, ok2, err = f2.next(); err != nil {
				return f2.wrap(err)
			}
		default:
			group1, err := f1.group(line1, opts.key1, opts.sep, opts.ignoreCase)
			if err != nil {
				return f1.wrap(err)
			}
			line1, ok1 = f1.line, f1.ok
			group2, err := f2.group(line2, opts.key2, opts.sep, opts.ignoreCase)
			if err != nil {
				return f2.wrap(err)
			}
			line2, ok2 = f2.line, f2.ok
			if opts.paired {
				// The combinations of the groups are written without
				// reading anything, so the context is checked as they
				// are emitted.
				for _, l1 := range group1 {
					for _, l2 := range group2 {
						if err := ctx.Err(); err != nil {
							return err
						}
						if err := p.writeLine(l1, l2, opts); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	// The lines left when a file ends can come from the reader's buffer,
	// without any reads in between, so the context is checked here as
	// well.
	for ok1 {
		if err := ctx.Err(); err != nil {
			return err
		}
		if opts.unpaired1 {
			if err := p.writeLine(line1, nil, opts); err != nil {
				return err
			}
		}
		if line1, ok1, err = f1.next(); err != nil {
			return f1.wrap(err)
		}
	}
	for ok2 {
		if err := ctx.Err(); err != nil {
			return err
		}
		if opts.unpaired2 {
			if err := p.writeLine(nil, line2, opts); err != nil {
				return err
			}
		}
		if line2, ok2, err = f2.next(); err != nil {
			return f2.wrap(err)
		}
	}
	return nil
}

// writeLine writes the join of the two lines, either of which can be nil
// when the other is unpaired.
func (p *Prog) writeLine(line1, line2 []byte, opts options) error {
	var out []byte
	if opts.selectors != nil {
		for i, sel := range opts.selectors {
			if i > 0 {
				out = append(out, opts.outputSep()...)
			}
			var value []byte
			switch sel.file {
			case 0:
				if line1 != nil {
					value = field(line1, opts.key1, opts.sep)
				} else {
					value = field(line2, opts.key2, opts.sep)
				}
			case 1:
				value = field(line1, sel.field, opts.sep)
			case 2:
				value = field(line2, sel.field, opts.sep)
			}
			if len(value) == 0 && opts.emptyGiven {
				value = []byte(opts.empty)
			}
			out = append(out, value...)
		}
	} else {
		var key []byte
		if line1 != nil {
			key = field(line1, opts.key1, opts.sep)
		} else {
			key = field(line2, opts.key2, opts.sep)
		}
		out = append(out, key...)
		for _, f := range otherFields(line1, opts.key1, opts.sep) {
			out = append(out, opts.outputSep()...)
			out = append(out, f...)
		}
		for _, f := range otherFields(line2, opts.key2, opts.sep) {
			out = append(out, opts.outputSep()...)
			out = append(out, f...)
		}
	}
	out = append(out, '\n')
	if _, err := p.Stdout.Write(out); err != nil {
		return fmt.Errorf("write error: %w", err)
	}
	return nil
}

// field returns the nth field of the line, or nil when it has none.
func field(line []byte, n int, sep []byte) []byte {
	fields := textutil.Fields(line, sep)
	if n < 1 || n > len(fields) {
		return nil
	}
	return fields[n-1]
}

// compareFields compares two join fields, folding the case of ASCII
// letters first when the -i option was given.
func compareFields(a, b []byte, ignoreCase bool) int {
	if ignoreCase {
		a, b = textutil.FoldCase(a), textutil.FoldCase(b)
	}
	return bytes.Compare(a, b)
}

// otherFields returns the fields of the line besides its join field, in
// their original order.
func otherFields(line []byte, key int, sep []byte) [][]byte {
	if line == nil {
		return nil
	}
	fields := textutil.Fields(line, sep)
	rest := make([][]byte, 0, len(fields))
	for i, f := range fields {
		if i != key-1 {
			rest = append(rest, f)
		}
	}
	return rest
}

// file is an input file, holding the line a group was cut from.
type file struct {
	name string
	r    *lineReader
	line []byte
	ok   bool
}

// next returns the next line of the file, without its newline, reporting
// whether one was read. A last line without a newline counts as well.
func (f file) next() ([]byte, bool, error) {
	return f.r.next()
}

// group returns the lines of the file whose join field equals the one of
// the given line, which is the first of them. The lines that follow the
// group are left in f.line, or mark the end of the file in f.ok.
func (f *file) group(line []byte, key int, sep []byte, ignoreCase bool) ([][]byte, error) {
	group := [][]byte{line}
	want := field(line, key, sep)
	for {
		next, ok, err := f.next()
		if err != nil {
			return nil, err
		}
		f.line, f.ok = next, ok
		if !ok || compareFields(field(next, key, sep), want, ignoreCase) != 0 {
			return group, nil
		}
		group = append(group, next)
	}
}

func (f file) wrap(err error) error {
	return fmt.Errorf("%s: %w", f.name, err)
}

// lineReader reads a file line by line.
type lineReader struct {
	r *bufio.Reader
}

func newLineReader(r io.Reader) *lineReader {
	return &lineReader{r: bufio.NewReader(r)}
}

func (r *lineReader) next() ([]byte, bool, error) {
	line, err := r.r.ReadBytes('\n')
	switch err {
	case nil:
		return line[:len(line)-1], true, nil
	case io.EOF:
		if len(line) == 0 {
			return nil, false, nil
		}
		return line, true, nil
	default:
		return nil, false, err
	}
}

type options struct {
	sep        []byte
	unpaired1  bool
	unpaired2  bool
	paired     bool
	ignoreCase bool
	empty      string
	emptyGiven bool
	selectors  []selector
	key1, key2 int
}

// selector is an output field: a field of a file, or the join field.
type selector struct {
	file  int // 1, 2, or 0 for the join field
	field int // 1-based field number
}

// outputSep returns the output field separator: a space by default, or the
// one given by -t.
func (opts options) outputSep() []byte {
	if opts.sep != nil {
		return opts.sep
	}
	return []byte{' '}
}

// longOptions maps the long options of join to the short ones they stand
// for.
var longOptions = map[string]rune{
	"ignore-case": 'i',
}

// optSet describes the options of join.
var optSet = getopt.Set{Valued: "12aeotv", Long: longOptions}

// usage describes the options of join.
const usage = `Usage: join [OPTION]... FILE1 FILE2
For each pair of input lines with identical join fields, write them to
standard output. The default join field is the first, separated by
blanks. When FILE1 or FILE2 (not both) is -, read standard input.

  -a FILenum
         also print unpairable lines from file FILenum
  -e EMPTY
         replace missing input fields with EMPTY
  -i, --ignore-case
         ignore differences in case when comparing fields
  -1 FIELD
         join on this FIELD of file 1
  -2 FIELD
         join on this FIELD of file 2
  -o FORMAT
         obey FORMAT while constructing output line
  -t CHAR
         use CHAR as input and output field separator
  -v FILenum
         like -a FILenum, but suppress joined output lines
` + stdopts.Tail

func parseFlags(args []string) (options, []string, error) {
	opts := options{paired: true, key1: 1, key2: 1}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case '1':
			key, err := parseField(value)
			if err != nil {
				return err
			}
			opts.key1 = key
		case '2':
			key, err := parseField(value)
			if err != nil {
				return err
			}
			opts.key2 = key
		case 'a', 'v':
			switch value {
			case "1":
				opts.unpaired1 = true
			case "2":
				opts.unpaired2 = true
			default:
				return fmt.Errorf("invalid file number %q", value)
			}
			if opt == 'v' {
				opts.paired = false
			}
		case 'i':
			opts.ignoreCase = true
		case 'e':
			opts.empty, opts.emptyGiven = value, true
		case 'o':
			sels, err := parseSelectors(value)
			if err != nil {
				return err
			}
			opts.selectors = sels
		case 't':
			if value == "" {
				return errors.New("empty separator")
			}
			// The separator is a single character, which keeps all the bytes
			// of its encoding.
			if utf8.RuneCountInString(value) != 1 {
				return fmt.Errorf("separator %q is not a single character", value)
			}
			opts.sep = []byte(value)
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

func checkOperands(operands []string) error {
	switch {
	case len(operands) < 2:
		if len(operands) == 0 {
			return errors.New("missing operand")
		}
		return fmt.Errorf("missing operand after %q", operands[0])
	case len(operands) > 2:
		return fmt.Errorf("extra operand %q", operands[2])
	case operands[0] == "-" && operands[1] == "-":
		return errors.New("both files are the standard input")
	}
	return nil
}

// parseField parses a join field number: digits only, from 1 on, so that
// signs and anything else a decimal number could carry is rejected.
func parseField(value string) (int, error) {
	if !numparse.AllDigits(value) {
		return 0, fmt.Errorf("invalid field number %q", value)
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid field number %q", value)
	}
	return n, nil
}

// parseSelectors parses an -o list, whose elements are separated by commas
// or blanks: a field of a file, like 1.2, or 0 for the join field. An empty
// element, like the one an extra comma leaves, is invalid.
func parseSelectors(list string) ([]selector, error) {
	var sels []selector
	empty := false
	for commaPart := range strings.SplitSeq(list, ",") {
		parts := strings.FieldsFunc(commaPart, func(r rune) bool {
			// Only the blanks of the list syntax separate elements, so
			// that other whitespace stays part of one and is rejected.
			return r == ' ' || r == '\t'
		})
		if len(parts) == 0 {
			empty = true
			continue
		}
		for _, elem := range parts {
			sel, err := parseSelector(elem)
			if err != nil {
				return nil, err
			}
			sels = append(sels, sel)
		}
	}
	switch {
	case sels == nil:
		return nil, errors.New("empty field list")
	case empty:
		return nil, errors.New("empty field specifier")
	}
	return sels, nil
}

func parseSelector(elem string) (selector, error) {
	if elem == "0" {
		return selector{}, nil
	}
	file, field, ok := strings.Cut(elem, ".")
	if !ok {
		return selector{}, fmt.Errorf("invalid field specifier %q", elem)
	}
	f, err := strconv.Atoi(file)
	if err != nil || !numparse.AllDigits(file) || (f != 1 && f != 2) {
		return selector{}, fmt.Errorf("invalid file number in %q", elem)
	}
	n, err := parseField(field)
	if err != nil {
		return selector{}, fmt.Errorf("invalid field number in %q", elem)
	}
	return selector{file: f, field: n}, nil
}

var _ coreutils.Program = (*Prog)(nil)
