// Package sort implements the POSIX sort utility.
//
// It sorts, merges or checks the lines of the given files, writing the result
// to the standard output or to the file given with -o. The standard input is
// used when no operands are given, and also when an operand is "-".
package sort

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/output"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/textutil"
)

// Prog is the sort core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run sorts, merges or checks the lines of the given files. A missing final
// newline of an input file is added, as the specification demands.
//
// The --random-source option is not supported: -R sorts by a hash of the
// keys seeded with a fixed value, so its permutation is deterministic.
//
// The exit status is 1 when -c detects disorder and 2 on errors. Reading is
// interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "sort", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "sort")
		return &coreutils.ExitError{Code: 2}
	}
	if opts.check != 0 && len(operands) > 1 {
		p.errorf("extra operand '%s' not allowed with -c", operands[1])
		stdopts.TryHelp(p.Runtime, "sort")
		return &coreutils.ExitError{Code: 2}
	}

	inputs := make([]input.Input, 0, len(operands))
	for _, operand := range operands {
		if operand == "" {
			p.errorf("invalid zero-length file name")
			return &coreutils.ExitError{Code: 2}
		}
		inputs = append(inputs, input.New(operand))
	}
	if len(inputs) == 0 {
		inputs = append(inputs, input.Input{Stdin: true})
	}

	if opts.check != 0 {
		return p.check(ctx, inputs[0], opts)
	}

	sets := make([][]sortLine, 0, len(inputs))
	for _, in := range inputs {
		lines, err := p.readLines(ctx, in, opts)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 2}
		}
		sets = append(sets, lines)
	}

	var lines []sortLine
	if opts.merge {
		lines = opts.mergeSets(sets)
	} else {
		lines = slices.Concat(sets...)
		slices.SortStableFunc(lines, opts.compareLines)
	}
	if opts.unique {
		lines = opts.uniqueLines(lines)
	}
	return p.writeLines(ctx, lines, opts)
}

// check verifies that the input is ordered as the options demand, reporting
// the first disordered line unless -C was given.
func (p *Prog) check(ctx context.Context, in input.Input, opts options) error {
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 2}
	}
	defer r.Close()

	name := in.Name
	if in.Stdin {
		name = "-"
	}

	delim := textutil.Delim(opts.zeroTerminated)
	term := []byte{delim}
	var prev sortLine
	havePrev := false
	number := 0
	err = textutil.Each(ctx, r, delim, func(line []byte) error {
		number++
		cur := opts.newLine(bytes.TrimSuffix(line, term))
		if havePrev {
			disorder := opts.compareLines(prev, cur) > 0
			if opts.unique && opts.equalKeys(prev, cur) {
				disorder = true
			}
			if disorder {
				if opts.check == 'c' {
					p.errorf("%s:%d: disorder: %s", name, number, cur.data)
				}
				return &coreutils.ExitError{Code: 1}
			}
		}
		prev = cur
		havePrev = true
		return nil
	})
	if err != nil {
		if _, ok := errors.AsType[*coreutils.ExitError](err); ok {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 2}
	}
	return nil
}

// readLines reads all the lines of an input, computing the key of each.
func (p *Prog) readLines(ctx context.Context, in input.Input, opts options) ([]sortLine, error) {
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var lines []sortLine
	delim := textutil.Delim(opts.zeroTerminated)
	term := []byte{delim}
	err = textutil.Each(ctx, r, delim, func(line []byte) error {
		lines = append(lines, opts.newLine(bytes.TrimSuffix(line, term)))
		return nil
	})
	return lines, err
}

// writeLines writes the sorted lines to the output file, or to the standard
// output when no -o option was given.
func (p *Prog) writeLines(ctx context.Context, lines []sortLine, opts options) error {
	out := io.Writer(p.Stdout)
	if opts.hasOutput {
		f, err := output.Create(p.Runtime, ctx, opts.output)
		if err != nil {
			p.errorf("%s: %v", opts.output, patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 2}
		}
		defer f.Close()
		out = f
	}
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeLine(out, line.data, textutil.Delim(opts.zeroTerminated)); err != nil {
			p.errorf("write error: %v", err)
			return &coreutils.ExitError{Code: 2, Err: err}
		}
	}
	return nil
}

// writeLine writes an item and its terminator to the output.
func writeLine(out io.Writer, data []byte, delim byte) error {
	line := make([]byte, 0, len(data)+1)
	line = append(line, data...)
	line = append(line, delim)
	if _, err := out.Write(line); err != nil {
		return &writeError{err: err}
	}
	return nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "sort: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	check          byte // 0, or 'c' or 'C'
	merge          bool
	unique         bool
	stable         bool
	zeroTerminated bool
	output         string
	hasOutput      bool
	sep            int // the -t separator byte, or -1 for blank separation
	leadingBlanks  bool
	global         ordering
	keys           []keydef
}

// sortOpt is the pseudo-option that stands for the --sort long option, which
// has no short form.
const sortOpt rune = 0

// longOptions maps the long options of sort to the short ones they stand for.
var longOptions = map[string]rune{
	"check":                 'c',
	"merge":                 'm',
	"unique":                'u',
	"output":                'o',
	"dictionary-order":      'd',
	"ignore-case":           'f',
	"ignore-nonprinting":    'i',
	"numeric-sort":          'n',
	"reverse":               'r',
	"ignore-leading-blanks": 'b',
	"field-separator":       't',
	"key":                   'k',
	"human-numeric-sort":    'h',
	"month-sort":            'M',
	"random-sort":           'R',
	"version-sort":          'V',
	"zero-terminated":       'z',
	"stable":                's',
	"sort":                  sortOpt,
}

// optSet describes the options of sort.
var optSet = getopt.Set{Valued: "kot" + string(sortOpt), Long: longOptions}

// usage describes the options of sort.
const usage = `Usage: sort [OPTION]... [FILE]...
Sort, merge, or compare all the lines from the given files.

With no FILE, or when FILE is -, read standard input.

  -b, --ignore-leading-blanks
         ignore leading blanks when finding sort keys
  -c, --check
         check whether the input is sorted
  -C
         like -c, but without printing a diagnostic
  -d, --dictionary-order
         consider only letters, digits and blanks
  -f, --ignore-case
         fold lowercase characters into uppercase
  -h, --human-numeric-sort
         compare human-numeric values, like 2K and 1G
  -i, --ignore-nonprinting
         ignore nonprinting characters
  -k, --key=KEYDEF
         sort by a key; KEYDEF gives the field and character positions, with the
         modifiers b, d, f, h, i, M, n, r, R and V
  -m, --merge
         merge the given sorted files as a group
  -M, --month-sort
         compare (unknown) < 'JAN' < ... < 'DEC'
  -n, --numeric-sort
         compare according to string numerical value
  -o, --output=FILE
         write the result to FILE instead of stdout
  -r, --reverse
         reverse the result of comparison
  -R, --random-sort
         sort by a hash of the input keys
  -s, --stable
         disable the last-resort comparison
  -t, --field-separator=SEP
         use SEP as the field separator
  -u, --unique
         output only the first of equal lines
  -V, --version-sort
         sort by version name and number
  -z, --zero-terminated
         delimit items with a zero byte, not a newline
      --sort=WORD
         sort by WORD: human-numeric, month, numeric, random, or version
      --help
         display this help and exit
      --version
         output version information and exit
`

// delimiter returns the byte that terminates an item: a zero byte with -z,
// and a newline otherwise.
func (opts options) delimiter() byte {
	if opts.zeroTerminated {
		return 0
	}
	return '\n'
}

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--". Once parsed, key fields without attached ordering
// modifiers inherit the global ordering options.
func parseFlags(args []string) (options, []string, error) {
	opts := options{sep: -1}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'b':
			opts.leadingBlanks = true
		case 'c':
			opts.check = 'c'
		case 'C':
			opts.check = 'C'
		case 'd':
			opts.global.dictionary = true
		case 'f':
			opts.global.foldCase = true
		case 'h':
			opts.global.kind = humanNumeric
		case 'i':
			opts.global.ignoreNonPrint = true
		case 'm':
			opts.merge = true
		case 'M':
			opts.global.kind = month
		case 'n':
			opts.global.kind = numeric
		case 'o':
			if value == "" {
				return errors.New("invalid zero-length file name")
			}
			opts.output = value
			opts.hasOutput = true
		case 'r':
			opts.global.reverse = true
		case 'R':
			opts.global.kind = random
		case 's':
			opts.stable = true
		case 't':
			if len(value) != 1 {
				return errors.New("the separator must be a single character")
			}
			opts.sep = int(value[0])
		case 'u':
			opts.unique = true
		case 'V':
			opts.global.kind = version
		case 'z':
			opts.zeroTerminated = true
		case 'k':
			key, err := parseKeydef(value)
			if err != nil {
				return err
			}
			opts.keys = append(opts.keys, key)
		case sortOpt:
			switch value {
			case "human-numeric":
				opts.global.kind = humanNumeric
			case "month":
				opts.global.kind = month
			case "numeric":
				opts.global.kind = numeric
			case "random":
				opts.global.kind = random
			case "version":
				opts.global.kind = version
			default:
				return fmt.Errorf("invalid argument '%s' for '--sort'", value)
			}
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	if opts.check != 0 && opts.merge {
		return options{}, nil, errors.New("options -c and -m are incompatible")
	}
	for i := range opts.keys {
		k := &opts.keys[i]
		if !k.hasOrder {
			k.order = opts.global
		}
		k.start.blank = k.start.blank || opts.leadingBlanks
		k.end.blank = k.end.blank || opts.leadingBlanks
	}
	return opts, operands, nil
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
