// Package yes implements the GNU yes utility.
//
// It repeatedly writes the given operands, separated by single spaces and
// followed by a newline, to the standard output until a write fails or the
// context is done. With no operands, it repeats "y".
package yes

import (
	"context"
	"fmt"
	"io"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the yes core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run repeatedly writes the operands, separated by single spaces and followed
// by a newline, to the standard output, until a write fails or ctx is done.
// With no operands, it writes "y". A lone --help or --version is handled
// before any output, and a failing write makes it exit with status 1.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "yes", usage, getopt.Set{}, args); done {
		return err
	}
	operands, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(p.Stderr, "yes: %v\n", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return p.repeat(ctx, line(operands))
}

// repeat writes line to the standard output until it fails or ctx is done,
// returning the context error as-is when it's done.
func (p *Prog) repeat(ctx context.Context, line string) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := io.WriteString(p.Stdout, line); err != nil {
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	}
}

// line builds the line to repeat: the operands separated by single spaces, or
// "y" when there are none.
func line(operands []string) string {
	if len(operands) == 0 {
		return "y\n"
	}
	return strings.Join(operands, " ") + "\n"
}

// parseFlags parses the operands. Only the standard options exist, and
// parsing stops at the first operand or at "--".
func parseFlags(args []string) ([]string, error) {
	operands, err := getopt.Set{}.Parse(args, func(opt rune, _ string) error {
		return getopt.InvalidOption(opt)
	})
	if err != nil {
		return nil, err
	}
	return operands, nil
}

// usage describes the options of yes.
const usage = `Usage: yes [STRING]...
  or:  yes OPTION

Repeatedly output a line with all specified STRING(s), or 'y'.

` + stdopts.Tail

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
