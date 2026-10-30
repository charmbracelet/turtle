// Package whoami implements the GNU whoami utility.
//
// The user name comes from the runtime environment, as the USER variable
// falling back to LOGNAME, since the runtime has no user database access.
package whoami

import (
	"context"
	"errors"
	"fmt"
	"io"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the whoami core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the effective user name to the standard output. It takes
// neither options nor operands of its own, and exits with status 1 when the
// user name cannot be found.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "whoami", usage, getopt.Set{}, args); done {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := parseArgs(args); err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "whoami")
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	name, ok := p.user()
	if !ok {
		err := errors.New("no user name: neither USER nor LOGNAME is set")
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	if _, err := io.WriteString(p.Stdout, name+"\n"); err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// parseArgs rejects every argument: whoami takes neither options nor
// operands.
func parseArgs(args []string) error {
	operands, err := getopt.Set{}.Parse(args, func(opt rune, _ string) error {
		return getopt.InvalidOption(opt)
	})
	if err != nil {
		return err
	}
	if len(operands) > 0 {
		return fmt.Errorf("unexpected operand: %q", operands[0])
	}
	return nil
}

// user returns the user name from the environment: USER, or LOGNAME when
// USER is unset or empty.
func (p *Prog) user() (string, bool) {
	for _, key := range []string{"USER", "LOGNAME"} {
		if name, ok := p.LookupEnv(key); ok && name != "" {
			return name, true
		}
	}
	return "", false
}

// errorf writes a diagnostic message to the standard error.
func (p *Prog) errorf(format string, args ...any) {
	if p.Stderr == nil {
		return
	}
	fmt.Fprintf(p.Stderr, "whoami: "+format+"\n", args...)
}

// usage describes the options of whoami.
const usage = `Usage: whoami [OPTION]...
Print the user name associated with the current effective user ID.

      --help
         display this help and exit
      --version
         output version information and exit
`

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
