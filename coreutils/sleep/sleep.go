// Package sleep implements the sleep utility.
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
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the sleep core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run suspends execution for the sum of the durations given in the operands,
// returning the context error as-is when the context is done first.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "sleep", usage, getopt.Set{}, args); done {
		return err
	}
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

// parseArgs parses the options and the time operands. sleep takes no
// options, so any argument starting with a single dash is an invalid option.
func parseArgs(args []string) (time.Duration, error) {
	operands, err := getopt.Set{}.Parse(args, func(opt rune, _ string) error {
		return getopt.InvalidOption(opt)
	})
	if err != nil {
		return 0, err
	}
	if len(operands) == 0 {
		return 0, errors.New("missing operand")
	}
	return parseOperands(operands)
}

// parseOperands sums the duration operands into a single duration.
func parseOperands(operands []string) (time.Duration, error) {
	var total time.Duration
	for _, operand := range operands {
		d, err := parseOperand(operand)
		if err != nil {
			return 0, err
		}
		if d > 0 && total > math.MaxInt64-d {
			return 0, fmt.Errorf("invalid number: '%s'", operand)
		}
		total += d
	}
	return total, nil
}

// unitSeconds maps each unit suffix to its length in seconds.
var unitSeconds = map[byte]float64{
	's': 1,
	'm': 60,
	'h': 60 * 60,
	'd': 24 * 60 * 60,
}

// maxSeconds is the largest number of seconds representable as a duration.
const maxSeconds = float64(math.MaxInt64 / int64(time.Second))

// parseOperand converts a duration operand, a non-negative fractional number
// with an optional s, m, h or d unit suffix, into a duration.
func parseOperand(operand string) (time.Duration, error) {
	s := operand
	seconds := 1.0
	if len(s) > 0 {
		if mult, ok := unitSeconds[s[len(s)-1]]; ok {
			seconds = mult
			s = s[:len(s)-1]
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) || f > maxSeconds/seconds {
		return 0, fmt.Errorf("invalid number: '%s'", operand)
	}
	return time.Duration(f * seconds * float64(time.Second)), nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "sleep: "+format+"\n", args...)
}

// usage describes the options of sleep.
const usage = `Usage: sleep NUMBER[smhd]...
Pause for NUMBER seconds.  SUFFIX may be 's' for seconds (the default),
'm' for minutes, 'h' for hours or 'd' for days.  NUMBER need not be an
integer.  Given two or more arguments, pause for the amount of time
specified by the sum of their values.

      --help     display this help and exit
      --version  output version information and exit
`

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
