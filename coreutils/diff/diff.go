// Package diff implements the POSIX diff utility.
//
// It compares two files line by line, writing the changes necessary to
// convert the first into the second. An operand of "-" means the standard
// input, and a directory operand makes diff compare the files of the same
// name inside it.
package diff

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strconv"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the diff core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run compares the operands line by line, writing the changes that turn
// each first file into its second. It returns an exit status of 1 when any
// comparison finds differences and 2 when any other error happens.
//
// Comparisons are interrupted when ctx is done, and the context error is
// returned as-is.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "diff", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "diff")
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) < 2 {
		p.errorf("missing operand")
		stdopts.TryHelp(p.Runtime, "diff")
		return &coreutils.ExitError{Code: 2}
	}
	if len(operands) > 2 {
		p.errorf("extra operand '%s'", operands[2])
		stdopts.TryHelp(p.Runtime, "diff")
		return &coreutils.ExitError{Code: 2}
	}

	differ, err := p.compare(ctx, filepath.ToSlash(operands[0]), filepath.ToSlash(operands[1]), opts)
	if err != nil {
		var writeErr *writeError
		var pathErr *pathError
		switch {
		case errors.As(err, &writeErr):
			p.errorf("write error: %v", writeErr.err)
			return &coreutils.ExitError{Code: 2, Err: writeErr.err}
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, errNoFS):
			p.errorf("%v", err)
			return &coreutils.ExitError{Code: 2}
		case errors.As(err, &pathErr):
			p.errorf("%s: %v", pathErr.label, patherr.Unwrap(pathErr.err))
			return &coreutils.ExitError{Code: 2, Err: pathErr.err}
		default:
			return err
		}
	}
	if differ {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// compare resolves both operands and compares them, reporting whether they
// differ.
func (p *Prog) compare(ctx context.Context, name1, name2 string, opts options) (bool, error) {
	op1, err := p.operand(name1)
	if err != nil {
		return false, err
	}
	op2, err := p.operand(name2)
	if err != nil {
		return false, err
	}

	switch {
	case op1.isDir && op2.isDir:
		return p.compareDirs(ctx, op1, op2, opts, nil)
	case op1.isDir:
		base := path.Base(op2.name)
		return p.compareFiles(ctx, pair{
			name1: path.Join(op1.name, base),
			path1: path.Join(op1.path, base),
			name2: op2.name,
			path2: op2.path,
		}, opts)
	case op2.isDir:
		base := path.Base(op1.name)
		return p.compareFiles(ctx, pair{
			name1: op1.name,
			path1: op1.path,
			name2: path.Join(op2.name, base),
			path2: path.Join(op2.path, base),
		}, opts)
	default:
		return p.compareFiles(ctx, pair{
			name1: op1.name,
			path1: op1.path,
			name2: op2.name,
			path2: op2.path,
		}, opts)
	}
}

// operand holds a resolved command line operand: its name as given, its
// path in the file system, empty for the standard input, and whether it
// names a directory.
type operand struct {
	name  string
	path  string
	isDir bool
}

// errNoFS marks the lack of a file system to reach paths through.
var errNoFS = errors.New("no file system available")

// operand resolves the given operand name.
func (p *Prog) operand(name string) (operand, error) {
	if name == "-" {
		return operand{name: name}, nil
	}
	if p.FS == nil {
		return operand{}, errNoFS
	}
	op := operand{name: name, path: filepath.ToSlash(p.ResolvePath(name))}
	fi, err := fs.Stat(p.FS, op.path)
	if err != nil {
		return operand{}, &pathError{label: name, err: err}
	}
	op.isDir = fi.IsDir()
	return op, nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "diff: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	blank     bool
	recursive bool
	format    format
	lines     int
}

// format is the output format of the changes.
type format uint8

const (
	normalFormat format = iota
	contextFormat
	unifiedFormat
	edFormat
	forwardEdFormat
)

// optSet describes the options of diff.
var optSet = getopt.Set{Valued: "CU"}

// usage describes the options of diff.
const usage = `Usage: diff [OPTION]... FILE1 FILE2
Compare FILE1 with FILE2 line by line, writing the changes which turn
FILE1 into FILE2. An operand of '-' means the standard input, and a
directory operand makes the files of the same name inside it compared.

  -b
         ignore changes in the amount of white space
  -r
         compare directories recursively
  -c
         output a copied context diff with 3 lines of context
  -C NUM
         output a copied context diff with NUM lines of context
  -e
         output an ed script
  -f
         output a forward ed script
  -u
         output a unified diff with 3 lines of context
  -U NUM
         output a unified diff with NUM lines of context
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags parses the command line options. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'b':
			opts.blank = true
		case 'r':
			opts.recursive = true
		case 'c':
			return opts.setStyle(contextFormat, 3)
		case 'e':
			return opts.setStyle(edFormat, 0)
		case 'f':
			return opts.setStyle(forwardEdFormat, 0)
		case 'u':
			return opts.setStyle(unifiedFormat, 3)
		case 'C':
			n, err := parseCount(value, 1)
			if err != nil {
				return err
			}
			return opts.setStyle(contextFormat, n)
		case 'U':
			n, err := parseCount(value, 0)
			if err != nil {
				return err
			}
			return opts.setStyle(unifiedFormat, n)
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

// setStyle picks the output format, rejecting a conflicting one.
func (o *options) setStyle(f format, lines int) error {
	if o.format != normalFormat && o.format != f {
		return errors.New("conflicting output style options")
	}
	o.format = f
	o.lines = lines
	return nil
}

// parseCount parses the option-argument of -C or -U as a line count of at
// least min lines.
func parseCount(value string, min int) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < min {
		return 0, fmt.Errorf("invalid context length '%s'", value)
	}
	return n, nil
}

var _ coreutils.Program = (*Prog)(nil)
