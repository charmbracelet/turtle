// Package nice implements the POSIX nice utility.
//
// The niceness adjustment is parsed and validated as the POSIX and GNU
// specifications define, but it is never applied: the runtime cannot change
// the scheduling priority, so the utility operand runs with the niceness it
// would have anyway.
package nice

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the nice core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// settings holds what a run of nice selects.
type settings struct {
	adjustment string // -n or the obsolete form, as given
	given      bool   // an adjustment was given
}

// longOptions maps the long options of nice to the short ones they stand
// for.
var longOptions = map[string]rune{
	"adjustment": 'n',
}

// optSet describes the options of nice.
var optSet = getopt.Set{Valued: "n", Long: longOptions}

// usage describes the options of nice.
const usage = `Usage: nice [OPTION] [COMMAND [ARG]...]
Run COMMAND with an adjusted niceness, which affects process scheduling.

  -n, --adjustment=N
         add integer N to the niceness (default 10)
` + stdopts.Tail

// Run invokes the utility operand with the argument operands, after the
// niceness adjustment is parsed and validated. The adjustment is accepted
// but never applied, since the runtime cannot change the scheduling
// priority. The exit status of the utility becomes the exit status of
// nice, and the 127, 126 and 125 statuses of a failed execution are passed
// along with a diagnostic. The context error is returned as-is when the
// caller's context is done.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	// The standard options are looked for only among the options of nice:
	// after the utility name they are the operands of the utility, and the
	// obsolescent adjustments are not options at all.
	if done, err := stdopts.Check(p.Runtime, "nice", usage, optSet, checkArgs(args)); done {
		return err
	}
	set, operands, err := parseFlags(args)
	if err != nil {
		return p.usageError(err)
	}
	if set.given {
		if err := validateAdjustment(set.adjustment); err != nil {
			return p.usageError(err)
		}
		if len(operands) == 0 {
			return p.usageError(errors.New("a command must be given with an adjustment"))
		}
	}
	if len(operands) == 0 {
		return p.usageError(errors.New("missing operand"))
	}
	return p.invoke(ctx, operands[0], operands[1:])
}

// usageError reports a usage error, followed by the standard hint, exiting
// with 125, the status GNU nice uses when it fails.
func (p *Prog) usageError(err error) error {
	stdopts.Errorf(p.Runtime, "nice", "%v", err)
	stdopts.TryHelp(p.Runtime, "nice")
	return &coreutils.ExitError{Code: 125, Err: err}
}

// parseFlags parses the options and operands the way GNU processes them,
// in argument order: an obsolescent adjustment, then the options with
// their values, over and over, with the last adjustment given winning. A
// dash argument which is neither an integer nor an option of nice is the
// utility name, and the arguments after it are its operands.
func parseFlags(args []string) (settings, []string, error) {
	var set settings
	for len(args) > 0 {
		if incr, ok := obsolete(args[0]); ok {
			set.adjustment, set.given = incr, true
			args = args[1:]
			continue
		}
		opts, rest, operands := scanOpts(args)
		_, err := optSet.Parse(opts, func(opt rune, value string) error {
			switch opt {
			case 'n':
				set.adjustment, set.given = value, true
			default:
				return getopt.InvalidOption(opt)
			}
			return nil
		})
		if err != nil {
			return set, nil, err
		}
		if operands {
			return set, rest, nil
		}
		args = rest
	}
	return set, nil, nil
}

// scanOpts returns the next run of arguments holding the options of nice
// and their values, and the ones left after them. The run stops before an
// obsolescent adjustment, which the caller takes, or at the utility name
// or at "--", when the arguments left are the operands.
func scanOpts(args []string) (opts, rest []string, operands bool) {
	for i := 0; i < len(args); i++ {
		s := args[i]
		if s == "--" {
			return args[:i], args[i+1:], true
		}
		if s == "-" || !strings.HasPrefix(s, "-") {
			return args[:i], args[i:], true
		}
		if _, ok := obsolete(s); ok {
			return args[:i], args[i:], false
		}
		if !optForm(s) {
			return args[:i], args[i:], true
		}
		if valuedNext(s) && i+1 < len(args) {
			i++
		}
	}
	return args, nil, false
}

// checkArgs returns the arguments the standard options are looked for in:
// the options of nice with their values, up to the utility name, without
// the obsolescent adjustments. The values are kept, since a --help which
// follows a lone -n is its value, as GNU's option parsing takes it.
func checkArgs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		s := args[i]
		if s == "--" || s == "-" || !strings.HasPrefix(s, "-") {
			return out
		}
		if _, ok := obsolete(s); ok {
			continue
		}
		if !optForm(s) {
			return out
		}
		out = append(out, s)
		if valuedNext(s) && i+1 < len(args) {
			out = append(out, args[i+1])
			i++
		}
	}
	return out
}

// optForm reports whether arg starts an option of nice, which the option
// parsing takes: the -n one, with its value attached or not, or a long
// one.
func optForm(arg string) bool {
	return strings.HasPrefix(arg, "-n") || strings.HasPrefix(arg, "--")
}

// valuedNext reports whether the option in arg takes its value from the
// next argument: a lone -n or --adjustment, without one attached.
func valuedNext(arg string) bool {
	return arg == "-n" || arg == "--adjustment"
}

// obsolete reports the adjustment of the obsolescent -adjustment syntax
// held in arg: a dash and an integer after it. An integer too large to
// hold still makes one, since GNU reports it as an invalid adjustment
// rather than taking the argument as a utility name.
func obsolete(arg string) (string, bool) {
	if len(arg) < 2 || arg[0] != '-' {
		return "", false
	}
	s := arg[1:]
	if !intSyntax(s) {
		return "", false
	}
	return s, true
}

// intSyntax reports whether s has the syntax of a decimal integer, even
// when its value is too large to hold.
func intSyntax(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	var numErr *strconv.NumError
	return err == nil || (errors.As(err, &numErr) && numErr.Err == strconv.ErrRange)
}

// validateAdjustment checks that the adjustment is a valid integer, as GNU
// reports on one that is not, or one too large to hold.
func validateAdjustment(s string) error {
	if _, err := strconv.ParseInt(s, 10, 64); err != nil {
		return fmt.Errorf("invalid adjustment %q", s)
	}
	return nil
}

// invoke runs the utility with the arguments in the working directory of
// the runtime, with the environment of the runtime. The exit status of the
// utility becomes the exit status of nice, and the 127, 126 or 125 status
// the execution returns is passed along with a diagnostic.
func (p *Prog) invoke(ctx context.Context, utility string, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Exec == nil {
		stdopts.Errorf(p.Runtime, "nice", "no command execution available")
		return &coreutils.ExitError{Code: 125}
	}
	status, err := p.Exec(ctx, coreutils.ExecOpts{Name: utility, Args: args, Env: p.EnvList()})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		stdopts.Errorf(p.Runtime, "nice", "%v", err)
		return &coreutils.ExitError{Code: status, Err: err}
	}
	if status != 0 {
		return &coreutils.ExitError{Code: status}
	}
	return nil
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
