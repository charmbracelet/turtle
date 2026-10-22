// Package sort implements the POSIX sort utility.
//
// It sorts, merges or checks the lines of the given files, writing the result
// to the standard output or to the file given with -o. The standard input is
// used when no operands are given, and also when an operand is "-".
package sort

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/fsx"
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
// The exit status is 1 when -c detects disorder and 2 on errors. Reading is
// interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if opts.check != 0 && len(operands) > 1 {
		p.errorf("extra operand '%s' not allowed with -c", operands[1])
		return &coreutils.ExitError{Code: 2}
	}

	inputs := make([]input, 0, len(operands))
	for _, operand := range operands {
		if operand == "" {
			p.errorf("invalid zero-length file name")
			return &coreutils.ExitError{Code: 2}
		}
		inputs = append(inputs, input{name: operand, stdin: operand == "-"})
	}
	if len(inputs) == 0 {
		inputs = append(inputs, input{stdin: true})
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
			p.errorf("%s: %v", in.label(), unwrapPathError(err))
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
func (p *Prog) check(ctx context.Context, in input, opts options) error {
	r, err := p.open(ctx, in)
	if err != nil {
		p.errorf("%s: %v", in.label(), unwrapPathError(err))
		return &coreutils.ExitError{Code: 2}
	}
	defer r.Close()

	name := in.name
	if in.stdin {
		name = "-"
	}

	br := bufio.NewReader(r)
	var prev sortLine
	havePrev := false
	number := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			number++
			cur := opts.newLine([]byte(strings.TrimSuffix(line, "\n")))
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
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			p.errorf("%s: %v", in.label(), unwrapPathError(err))
			return &coreutils.ExitError{Code: 2}
		}
	}
}

// readLines reads all the lines of an input, computing the key of each.
func (p *Prog) readLines(ctx context.Context, in input, opts options) ([]sortLine, error) {
	r, err := p.open(ctx, in)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var lines []sortLine
	br := bufio.NewReader(r)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			lines = append(lines, opts.newLine([]byte(strings.TrimSuffix(line, "\n"))))
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return lines, nil
			}
			return nil, err
		}
	}
}

// writeLines writes the sorted lines to the output file, or to the standard
// output when no -o option was given.
func (p *Prog) writeLines(ctx context.Context, lines []sortLine, opts options) error {
	out := io.Writer(p.Stdout)
	if opts.hasOutput {
		f, err := p.openOutput(opts.output)
		if err != nil {
			p.errorf("%s: %v", opts.output, unwrapPathError(err))
			return &coreutils.ExitError{Code: 2}
		}
		defer f.Close()
		out = f
	}
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeLine(out, line.data); err != nil {
			p.errorf("write error: %v", err)
			return &coreutils.ExitError{Code: 2, Err: err}
		}
	}
	return nil
}

// writeLine writes a line and its newline to the output.
func writeLine(out io.Writer, data []byte) error {
	line := make([]byte, 0, len(data)+1)
	line = append(line, data...)
	line = append(line, '\n')
	if _, err := out.Write(line); err != nil {
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

// openOutput creates the output file.
func (p *Prog) openOutput(name string) (fsx.File, error) {
	if p.FS == nil {
		return nil, errors.New("no file system available")
	}
	return p.FS.OpenFile(filepath.ToSlash(p.ResolvePath(name)), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666&^p.Umask.Perm())
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "sort: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	check         byte // 0, or 'c' or 'C'
	merge         bool
	unique        bool
	output        string
	hasOutput     bool
	sep           int // the -t separator byte, or -1 for blank separation
	leadingBlanks bool
	global        ordering
	keys          []keydef
}

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
}

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--". Once parsed, key fields without attached ordering
// modifiers inherit the global ordering options.
func parseFlags(args []string) (options, []string, error) {
	opts := options{sep: -1}
	operands, err := getopt.Set{Valued: "kot", Long: longOptions}.Parse(args, func(opt rune, value string) error {
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
		case 'i':
			opts.global.ignoreNonPrint = true
		case 'm':
			opts.merge = true
		case 'n':
			opts.global.numeric = true
		case 'o':
			if value == "" {
				return errors.New("invalid zero-length file name")
			}
			opts.output = value
			opts.hasOutput = true
		case 'r':
			opts.global.reverse = true
		case 'u':
			opts.unique = true
		case 't':
			if len(value) != 1 {
				return errors.New("the separator must be a single character")
			}
			opts.sep = int(value[0])
		case 'k':
			key, err := parseKeydef(value)
			if err != nil {
				return err
			}
			opts.keys = append(opts.keys, key)
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

// unwrapPathError unwraps *fs.PathError so diagnostics don't repeat the file
// name.
func unwrapPathError(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}

var _ coreutils.Program = (*Prog)(nil)
