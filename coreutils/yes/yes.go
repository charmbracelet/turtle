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
	set, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(p.Stderr, "yes: %v\n", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	switch {
	case set.help:
		if _, err := fmt.Fprint(p.Stdout, usage); err != nil {
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	case set.version:
		if _, err := fmt.Fprint(p.Stdout, "yes (charm.land/snail/coreutils)\n"); err != nil {
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	default:
		return p.repeat(ctx, line(set.operands))
	}
	return nil
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

// The long options of yes map to runes above the range a single-byte short
// option can reach, so --help and --version stay long-only.
const (
	optHelp = 0x100 + iota
	optVersion
)

var longOptions = map[string]rune{
	"help":    optHelp,
	"version": optVersion,
}

// settings holds what a run of yes selects.
type settings struct {
	operands []string
	help     bool
	version  bool
}

// parseFlags parses the options and operands. Only --help and --version are
// options, and parsing stops at the first operand or at "--".
func parseFlags(args []string) (settings, error) {
	var set settings
	operands, err := getopt.Set{Long: longOptions}.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case optHelp:
			set.help = true
		case optVersion:
			set.version = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return settings{}, err
	}
	set.operands = operands
	return set, nil
}

// usage describes the options of yes.
const usage = `Usage: yes [STRING]...
  or:  yes OPTION

Repeatedly output a line with all specified STRING(s), or 'y'.

      --help     display this help and exit
      --version  output version information and exit
`

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
