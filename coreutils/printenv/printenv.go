// Package printenv implements the GNU printenv utility.
//
// With no operands it writes the environment, one "KEY=VALUE" entry per
// line, in the order the runtime enumerates it. With operands it writes
// only the value of each named variable.
package printenv

import (
	"context"
	"fmt"
	"io"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the printenv core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the environment to the standard output. Without operands every
// entry is written as given, as "KEY=VALUE". Each operand names a variable,
// and only its value is written; a missing variable makes the exit status 1,
// but the later operands are still processed. Write errors exit with 2.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "printenv", usage, optSet, args); done {
		return err
	}
	opts, names, err := parseArgs(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "printenv")
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	sep := "\n"
	if opts.zero {
		sep = "\x00"
	}
	if len(names) == 0 {
		return p.printEntries(ctx, p.EnvList(), sep)
	}
	return p.printValues(ctx, names, sep)
}

// printEntries writes each environment entry followed by sep.
func (p *Prog) printEntries(ctx context.Context, entries []string, sep string) error {
	if p.Stdout == nil {
		p.errorf("no standard output available")
		return &coreutils.ExitError{Code: 2}
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := io.WriteString(p.Stdout, entry+sep); err != nil {
			return &coreutils.ExitError{Code: 2, Err: err}
		}
	}
	return nil
}

// printValues writes the value of each named variable followed by sep,
// processing every operand even after a missing one, and reporting whether
// at least one of them was not set.
func (p *Prog) printValues(ctx context.Context, names []string, sep string) error {
	if p.Stdout == nil {
		p.errorf("no standard output available")
		return &coreutils.ExitError{Code: 2}
	}
	missing := false
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		value, ok := p.LookupEnv(name)
		if !ok {
			missing = true
			continue
		}
		if _, err := io.WriteString(p.Stdout, value+sep); err != nil {
			return &coreutils.ExitError{Code: 2, Err: err}
		}
	}
	if missing {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// errorf writes a diagnostic to the standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "printenv: "+format+"\n", args...)
}

// options holds the -0 flag.
type options struct {
	zero bool
}

// parseArgs parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseArgs(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case '0':
			opts.zero = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// longOptions maps the long options of printenv to the short ones they stand
// for.
var longOptions = map[string]rune{
	"null": '0',
}

// optSet describes the options of printenv.
var optSet = getopt.Set{Long: longOptions}

// usage describes the options of printenv.
const usage = `Usage: printenv [OPTION]... [VARIABLE]...
Print the values of the specified environment VARIABLE(s).

  -0, --null
         end each output line with NUL, not newline
      --help
         display this help and exit
      --version
         output version information and exit
`

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
