// Package tee implements the POSIX tee utility.
//
// It duplicates the standard input to the standard output and to each given
// file. Files are truncated before copying, unless -a is given to append to
// them.
package tee

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/fsx"
)

// Prog is the tee core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run duplicates the standard input to the standard output and to each file
// given as an operand. Writes that fail don't stop the other outputs, but they
// make the utility exit with a non-zero status.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "tee", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "tee")
		return &coreutils.ExitError{Code: 2}
	}
	if p.Stdin == nil {
		p.errorf("no standard input available")
		return &coreutils.ExitError{Code: 1}
	}

	outputs := []output{{w: p.Stdout, label: "standard output"}}
	ok := true
	for _, operand := range operands {
		if operand == "" {
			p.errorf("invalid zero-length file name")
			ok = false
			continue
		}
		f, err := p.openFile(operand, opts.append)
		if err != nil {
			p.errorf("%s: %v", operand, patherr.Unwrap(err))
			ok = false
			continue
		}
		defer f.Close()
		outputs = append(outputs, output{w: f, label: operand})
	}

	if err := p.copy(ctx, outputs); err != nil {
		return err
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// copy duplicates the standard input to all the given outputs, skipping the
// ones that already failed.
func (p *Prog) copy(ctx context.Context, outputs []output) error {
	in := ctxio.NewReader(ctx, p.Stdin)
	buf := make([]byte, 32*1024)
	ok := true
	for {
		n, err := in.Read(buf)
		if n > 0 {
			for i := range outputs {
				out := &outputs[i]
				if out.failed {
					continue
				}
				if _, werr := out.w.Write(buf[:n]); werr != nil {
					p.errorf("%s: %v", out.label, werr)
					out.failed = true
					ok = false
				}
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			p.errorf("standard input: %v", err)
			ok = false
			break
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// openFile opens an output file, truncating or appending to it.
func (p *Prog) openFile(name string, append bool) (fsx.File, error) {
	if p.FS == nil {
		return nil, errors.New("no file system available")
	}
	flag := os.O_WRONLY | os.O_CREATE
	if append {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	return p.FS.OpenFile(filepath.ToSlash(p.ResolvePath(name)), flag, 0o666&^p.Umask.Perm())
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "tee: "+format+"\n", args...)
}

// options holds the parsed command line options.
type options struct {
	append bool
}

// longOptions maps the long options of tee to the short ones they stand for.
var longOptions = map[string]rune{
	"append":            'a',
	"ignore-interrupts": 'i',
}

// optSet describes the options of tee.
var optSet = getopt.Set{Long: longOptions}

// usage describes the options of tee.
const usage = `Usage: tee [OPTION]... [FILE]...
Copy the standard input to the standard output and to each FILE.

  -a, --append              append to the given FILEs, do not overwrite
  -i, --ignore-interrupts   ignore interrupt signals
      --help     display this help and exit
      --version  output version information and exit
`

// parseFlags parses the command line options. The -i option is accepted and
// has no effect: signals aren't delivered to the utility in this context.
// Option parsing stops at the first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'a':
			opts.append = true
		case 'i':
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// output is a destination of the duplicated input: the standard output or one
// of the file operands.
type output struct {
	w      io.Writer
	label  string
	failed bool
}

var _ coreutils.Program = (*Prog)(nil)
