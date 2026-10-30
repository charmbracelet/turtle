// Package nproc implements the GNU nproc utility.
package nproc

import (
	"context"
	"fmt"
	"runtime"
	"strconv"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the nproc core utility.
type Prog struct {
	*coreutils.Runtime

	// numCPU reports the number of processing units available to the
	// process. When nil, runtime.NumCPU is used.
	numCPU func() int
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run prints the number of processing units available to the process, as
// limited by the OMP_NUM_THREADS and OMP_THREAD_LIMIT environment variables
// unless --all is given, and reduced by --ignore. The result is guaranteed
// to be greater than zero.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "nproc", usage, optSet, args); done {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	opts, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	count := p.count(opts)
	if _, err := fmt.Fprintln(p.Stdout, count); err != nil {
		p.errorf("write error: %v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// count computes the number of processing units to print: the available
// count, raised to the OMP_NUM_THREADS minimum and lowered to the
// OMP_THREAD_LIMIT maximum unless --all ignores both, minus the number
// ignored, and never below one.
func (p *Prog) count(opts options) int {
	n := p.cpuCount()
	if !opts.all {
		if min, ok := p.envInt("OMP_NUM_THREADS"); ok && min > n {
			n = min
		}
		if limit, ok := p.envInt("OMP_THREAD_LIMIT"); ok && limit < n {
			n = limit
		}
	}
	if uint64(n) <= opts.ignore {
		return 1
	}
	return n - int(opts.ignore)
}

// cpuCount reports the number of processing units, honoring numCPU when the
// tests replaced it.
func (p *Prog) cpuCount() int {
	if p.numCPU != nil {
		return p.numCPU()
	}
	return runtime.NumCPU()
}

// envInt returns the value of an environment variable as a positive number,
// reporting whether it's set to one. Invalid values are ignored.
func (p *Prog) envInt(key string) (int, bool) {
	value, ok := p.LookupEnv(key)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// parseFlags parses the options. Any operand is a usage error.
func parseFlags(args []string) (options, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case optAll:
			opts.all = true
		case optIgnore:
			n, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid --ignore argument '%s'", value)
			}
			opts.ignore = n
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, err
	}
	if len(operands) > 0 {
		return options{}, fmt.Errorf("extra operand '%s'", operands[0])
	}
	return opts, nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "nproc: "+format+"\n", args...)
}

// options holds the --all flag and the --ignore count.
type options struct {
	all    bool
	ignore uint64
}

// The long options of nproc map to runes above the range a single-byte short
// option can reach, so --all and --ignore stay long-only, as in the GNU
// utility.
const (
	optAll rune = 0x100 + iota
	optIgnore
)

// longOptions maps the long options of nproc to the runes they stand for.
var longOptions = map[string]rune{
	"all":    optAll,
	"ignore": optIgnore,
}

// optSet describes the options of nproc.
var optSet = getopt.Set{Valued: string(optIgnore), Long: longOptions}

// usage describes the options of nproc.
const usage = `Usage: nproc [OPTION]...
Print the number of processing units available to the current process,
which may be less than the number of online processors.  If this information
is not accessible, then print the number of processors installed.  If the
OMP_NUM_THREADS or OMP_THREAD_LIMIT environment variables are set, then they
will determine the minimum and maximum returned value respectively.  The
result is guaranteed to be greater than zero.

      --all          print the number of installed processors
      --ignore=N     ignore up to N processors
      --help     display this help and exit
      --version  output version information and exit
`

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
