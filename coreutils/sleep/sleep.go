// Package sleep implements the POSIX sleep utility.
package sleep

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
)

// Prog is the sleep core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run suspends execution for the number of seconds given in the time operand,
// returning the context error as-is when the context is done first.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	d, err := parseArgs(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Wait(ctx, d)
}

// parseArgs parses the options and the single time operand. sleep takes no
// options, so any argument starting with a single dash is an invalid option.
func parseArgs(args []string) (time.Duration, error) {
	operands, err := getopt.Set{}.Parse(args, func(opt rune, _ string) error {
		return getopt.InvalidOption(opt)
	})
	if err != nil {
		return 0, err
	}
	switch len(operands) {
	case 0:
		return 0, errors.New("missing operand")
	case 1:
		return parseSeconds(operands[0])
	default:
		return 0, fmt.Errorf("extra operand: %q", operands[1])
	}
}

// parseSeconds converts the time operand, a non-negative decimal integer of
// seconds, into a duration.
func parseSeconds(s string) (time.Duration, error) {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n > math.MaxInt64/uint64(time.Second) {
		return 0, fmt.Errorf("invalid number: '%s'", s)
	}
	return time.Duration(n) * time.Second, nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "sleep: "+format+"\n", args...)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
