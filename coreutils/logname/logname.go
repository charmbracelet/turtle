// Package logname implements the POSIX logname utility.
//
// The login name comes from the LOGNAME environment variable, since the
// isolated runtime has no user database or login session to query.
package logname

import (
	"context"
	"errors"
	"fmt"
	"io"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/stdopts"
)

// Prog is the logname core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the user's login name to the standard output. It takes
// neither options nor operands of its own, and exits with status 1 when
// the login name cannot be found.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "logname", usage, getopt.Set{}, args); done {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := parseArgs(args); err != nil {
		stdopts.Errorf(p.Runtime, "logname", "%v", err)
		stdopts.TryHelp(p.Runtime, "logname")
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	name, ok := p.login()
	if !ok {
		err := errors.New("no login name")
		stdopts.Errorf(p.Runtime, "logname", "%v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	if _, err := io.WriteString(p.Stdout, name+"\n"); err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// parseArgs rejects every argument: logname takes neither options nor
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

// login returns the login name from the LOGNAME environment variable,
// treating an unset or empty value as absent.
func (p *Prog) login() (string, bool) {
	if name, ok := p.LookupEnv("LOGNAME"); ok && name != "" {
		return name, true
	}
	return "", false
}

// usage describes the options of logname.
const usage = `Usage: logname [OPTION]...
Print the user's login name.

` + stdopts.Tail

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
