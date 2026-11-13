// Package tee implements the POSIX tee utility.
//
// It duplicates the standard input to the standard output and to each given
// file. Files are truncated before copying, unless -a is given to append to
// them. The -p and --output-error options select what an error on an output
// means for the run.
package tee

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"charm.land/turtle/coreutils"

	"charm.land/turtle/coreutils/internal/ctxio"
	"charm.land/turtle/coreutils/internal/getopt"
	outio "charm.land/turtle/coreutils/internal/output"
	"charm.land/turtle/coreutils/internal/patherr"
	"charm.land/turtle/coreutils/internal/stdopts"
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
// given as an operand. How an error on an output is handled follows the mode
// given to -p or --output-error: by default a failed write to standard output
// means a dead pipe and the copying stops, while any other error is only
// diagnosed, letting the remaining outputs through.
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "tee", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "tee", err)
	}
	if p.Stdin == nil {
		stdopts.Errorf(p.Runtime, "tee", "no standard input available")
		return &coreutils.ExitError{Code: 1}
	}

	outputs := []output{{w: p.Stdout, label: "standard output", pipe: isPipe(p.Stdout)}}
	ok := true
	for _, operand := range operands {
		if operand == "" {
			stdopts.Errorf(p.Runtime, "tee", "invalid zero-length file name")
			if opts.errMode.stopsOnOpen() {
				return &coreutils.ExitError{Code: 1}
			}
			ok = false
			continue
		}
		f, err := p.openFile(ctx, operand, opts.append)
		if err != nil {
			stdopts.Errorf(p.Runtime, "tee", "%s: %v", operand, patherr.Unwrap(err))
			if opts.errMode.stopsOnOpen() {
				return &coreutils.ExitError{Code: 1}
			}
			ok = false
			continue
		}
		defer f.Close()
		outputs = append(outputs, output{w: f, label: operand, pipe: isPipe(f)})
	}

	if err := p.copy(ctx, outputs, opts.errMode); err != nil {
		return err
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// copy duplicates the standard input to all the given outputs, following the
// error mode and stopping when it asks for it.
func (p *Prog) copy(ctx context.Context, outputs []output, mode errMode) error {
	c := copier{p: p, mode: mode}
	in := ctxio.NewReader(ctx, p.Stdin)
	buf := make([]byte, 32*1024)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			for i := range outputs {
				if c.stop {
					break
				}
				out := &outputs[i]
				if out.failed {
					continue
				}
				if _, werr := out.w.Write(buf[:n]); werr != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					out.failed = true
					c.writeError(outputs, out, werr)
				}
			}
		}
		if c.stop {
			break
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			stdopts.Errorf(p.Runtime, "tee", "standard input: %v", err)
			c.failed = true
			break
		}
	}
	if c.failed {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// openFile opens an output file, truncating or appending to it.
func (p *Prog) openFile(ctx context.Context, name string, append bool) (io.WriteCloser, error) {
	if append {
		return outio.Append(p.Runtime, ctx, name)
	}
	return outio.Create(p.Runtime, ctx, name)
}

// errMode is the behavior on output errors which --output-error selects.
type errMode int

const (
	// modeDefault, held when neither -p nor --output-error is given, stops
	// at a failed write to a pipe and diagnoses the errors of any other
	// output, as GNU does by dying of SIGPIPE.
	modeDefault errMode = iota
	// modeWarn diagnoses every output error and keeps writing.
	modeWarn
	// modeWarnNopipe diagnoses the errors of the outputs which aren't
	// pipes and keeps writing, stopping only when every output is gone.
	modeWarnNopipe
	// modeExit diagnoses an output error and stops at it, pipes included.
	modeExit
	// modeExitNopipe diagnoses an output error and stops at it, unless the
	// output is a pipe.
	modeExitNopipe
)

// stopsOnOpen reports whether a file which can't be opened ends the run. The
// nopipe modes only ignore the errors of pipes, and a file is never one.
func (m errMode) stopsOnOpen() bool {
	return m == modeExit || m == modeExitNopipe
}

// copier tracks what the errors of a run mean for its behavior.
type copier struct {
	p    *Prog
	mode errMode
	// failed reports whether some error asks for a non-zero exit status.
	failed bool
	// stop ends the copying before the input is exhausted.
	stop bool
}

// writeError accounts for a failed write to out, which is already dropped
// from the outputs of the run.
func (c *copier) writeError(outputs []output, out *output, err error) {
	if out.pipe && (c.mode == modeWarnNopipe || c.mode == modeExitNopipe) {
		// A broken pipe goes unnoticed: the point of the nopipe modes is
		// keeping the other outputs alive, and copying only ends when all
		// of them became broken pipes.
		c.stop = allFailed(outputs)
		return
	}
	if out.pipe && c.mode == modeDefault {
		c.failed = true
		c.stop = true
		return
	}
	stdopts.Errorf(c.p.Runtime, "tee", "%s: %v", out.label, err)
	c.failed = true
	if c.mode == modeExit || c.mode == modeExitNopipe {
		c.stop = true
	}
}

// allFailed reports whether no output is left to write to.
func allFailed(outputs []output) bool {
	for i := range outputs {
		if !outputs[i].failed {
			return false
		}
	}
	return true
}

// isPipe reports whether writing to w goes to a pipe. Only a real file or a
// standard stream answers for its status, so anything else, like a buffer,
// isn't a pipe.
func isPipe(w io.Writer) bool {
	f, ok := w.(interface{ Stat() (fs.FileInfo, error) })
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&fs.ModeNamedPipe != 0
}

// options holds the parsed command line options.
type options struct {
	append  bool
	errMode errMode
}

// longOptions maps the long options of tee to the short ones they stand for.
// The --output-error option shares the -p option, taking its mode only when
// attached to it.
var longOptions = map[string]rune{
	"append":            'a',
	"ignore-interrupts": 'i',
	"output-error":      'p',
}

// optSet describes the options of tee.
var optSet = getopt.Set{Attached: "p", Long: longOptions}

// usage describes the options of tee.
const usage = `Usage: tee [OPTION]... [FILE]...
Copy the standard input to the standard output and to each FILE.

  -a, --append
         append to the given FILEs, do not overwrite
  -i, --ignore-interrupts
         ignore interrupt signals
  -p
         operate in a more appropriate manner with pipes: same as
         --output-error=warn-nopipe
      --output-error[=MODE]
         on write errors: 'warn' diagnoses every output, 'warn-nopipe'
         (the default) diagnoses every output which isn't a pipe,
         'exit' and 'exit-nopipe' stop at the first error, the errors
         of pipes included and excluded
` + stdopts.Tail

// parseFlags parses the command line options. The -i option is accepted and
// has no effect: signals aren't delivered to the utility in this context. The
// -p option and the --output-error option, which stands for it, share the mode
// they select, with the last one given winning. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
			opts.append = true
		case 'i':
		case 'p':
			mode, err := parseErrMode(value)
			if err != nil {
				return err
			}
			opts.errMode = mode
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// parseErrMode returns the mode a --output-error option selects. The empty
// value, held by -p and by the long option given without a mode, is
// warn-nopipe.
func parseErrMode(value string) (errMode, error) {
	switch value {
	case "":
		return modeWarnNopipe, nil
	case "warn":
		return modeWarn, nil
	case "warn-nopipe":
		return modeWarnNopipe, nil
	case "exit":
		return modeExit, nil
	case "exit-nopipe":
		return modeExitNopipe, nil
	}
	return modeDefault, fmt.Errorf("invalid argument '%s' for '--output-error'", value)
}

// output is a destination of the duplicated input: the standard output or one
// of the file operands.
type output struct {
	w      io.Writer
	label  string
	pipe   bool
	failed bool
}

var _ coreutils.Program = (*Prog)(nil)
