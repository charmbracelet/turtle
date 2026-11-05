// Package time implements the POSIX time utility.
//
// The real time of the run is measured with the runtime clock around the
// execution of the utility. The runtime has no way to report the user and
// system CPU times of a child process, so they are always written as zero.
//
// The default timing format is the same one -p selects, since the POSIX
// specification leaves the default implementation defined, and the GNU
// default reports statistics, like CPU and memory usage, which the runtime
// does not carry.
package time

import (
	"context"
	"errors"
	"fmt"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the time core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// optSet describes the options of time.
var optSet = getopt.Set{}

// usage describes the options of time.
const usage = `Usage: time [-p] utility [argument...]
Run the utility with the given arguments, and write timing statistics
about the run to the standard error.

  -p
         write the timing statistics in the POSIX format
      --help
         display this help and exit
      --version
         output version information and exit
`

// Run invokes the utility operand with the argument operands, and writes the
// timing statistics of the run to the standard error. The exit status of the
// utility becomes the exit status of time, and the 127, 126 and 125 statuses
// of a failed execution are passed along with a diagnostic. The context
// error is returned as-is when the caller's context is done.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "time", usage, optSet, args); done {
		return err
	}
	operands, err := parseFlags(args)
	if err != nil {
		return p.usageError(err)
	}
	if len(operands) == 0 {
		return p.usageError(errors.New("missing operand"))
	}
	return p.invoke(ctx, operands[0], operands[1:])
}

// usageError reports a usage error, followed by the standard hint, exiting
// with 1.
func (p *Prog) usageError(err error) error {
	p.errorf("%v", err)
	stdopts.TryHelp(p.Runtime, "time")
	return &coreutils.ExitError{Code: 1, Err: err}
}

// parseFlags parses the options and operands. The -p option needs no state,
// since the default format is the same one it selects. Option parsing stops
// at the first operand, the utility.
func parseFlags(args []string) ([]string, error) {
	return optSet.Parse(args, func(opt rune, _ string) error {
		if opt != 'p' {
			return getopt.InvalidOption(opt)
		}
		return nil
	})
}

// invoke runs the utility with the arguments in the working directory of the
// runtime, timing the run with the runtime clock. The statistics are written
// when the utility runs; a failed execution, which cannot be found, cannot
// be invoked or is terminated by a signal, gets only a diagnostic.
func (p *Prog) invoke(ctx context.Context, utility string, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Exec == nil {
		p.errorf("no command execution available")
		return &coreutils.ExitError{Code: 125}
	}
	start := p.Now()
	status, err := p.Exec(ctx, "", utility, args, p.EnvList())
	elapsed := p.Now().Sub(start)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: status, Err: err}
	}
	p.printStats(elapsed)
	if status != 0 {
		return &coreutils.ExitError{Code: status}
	}
	return nil
}

// printStats writes the timing statistics to the standard error, in the
// POSIX format, with the real seconds the run took and the user and system
// CPU seconds, which the runtime does not report. Two fractional digits are
// kept, as GNU time writes with -p, precise enough for the clock ticks of
// the usual systems.
func (p *Prog) printStats(elapsed time.Duration) {
	fmt.Fprintf(p.Stderr, "real %.2f\nuser %.2f\nsys %.2f\n",
		elapsed.Seconds(), 0.0, 0.0)
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "time: "+format+"\n", args...)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
