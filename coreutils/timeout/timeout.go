// Package timeout implements the POSIX timeout utility, with the GNU
// extensions --foreground, --verbose and the 'inf' durations.
//
// This runtime has no way to signal a child process in the middle of an
// execution, so the time limit is enforced by canceling the context the
// execution runs with, which the real implementation turns into killing
// the child process. What the manuals call sending a signal maps to this
// cancellation.
package timeout

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/durationparse"
	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/stdopts"
)

// The signal numbers of the default and emergency signals.
const (
	sigTerm = 15
	sigKill = 9
)

// Prog is the timeout core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// settings holds what a run of timeout selects.
type settings struct {
	killAfter string // -k, the raw duration operand
	signal    string // -s
	preserve  bool   // -p, --preserve-status
	verbose   bool   // -v, --verbose
}

// longOptions maps the long options of timeout to the short ones they stand
// for. The -f option, --foreground, changes nothing here, since there are no
// process groups to keep out of the way in the first place.
var longOptions = map[string]rune{
	"kill-after":      'k',
	"signal":          's',
	"preserve-status": 'p',
	"foreground":      'f',
	"verbose":         'v',
}

// optSet describes the options of timeout.
var optSet = getopt.Set{Valued: "ks", Long: longOptions}

// usage describes the options of timeout.
const usage = `Usage: timeout [OPTION] DURATION COMMAND [ARG]...
Start COMMAND, and kill it if still running after DURATION.

  -f, --foreground
         when not running timeout from a shell prompt, allow COMMAND to use
         the foreground terminal and time out only COMMAND itself
  -k, --kill-after=DURATION
         also send a KILL signal if COMMAND is still running after DURATION
         since the first signal was sent
  -p, --preserve-status
         exit with the status of COMMAND, even when the time limit is reached
  -s, --signal=SIGNAL
         specify the signal to send on timeout, a name like HUP or a number
  -v, --verbose
         diagnose to standard error any signal sent upon timeout
` + stdopts.Tail

// Run executes the command operand under the time limit of the duration
// operand. When the limit elapses, the command is given up on and timeout
// exits with 124, or with the status of the command with --preserve-status.
// The 127, 126 and 125 statuses of a failed execution are passed along with
// a diagnostic. The context error is returned as-is when the caller's
// context is done.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "timeout", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return p.usageError(err)
	}
	if len(operands) == 0 {
		return p.usageError(errors.New("missing operand"))
	}
	if len(operands) == 1 {
		return p.usageError(errors.New("missing command"))
	}
	duration, infinite, err := parseDuration(operands[0])
	if err != nil {
		return p.usageError(err)
	}
	killAfter, err := parseKillAfter(opts.killAfter)
	if err != nil {
		return p.usageError(err)
	}
	sig := sigTerm
	if opts.signal != "" {
		if sig, err = parseSignal(opts.signal); err != nil {
			return p.usageError(err)
		}
	}
	return p.run(ctx, opts, duration, infinite, killAfter, sig, operands[1], operands[2:])
}

// usageError reports a usage error, followed by the standard hint, exiting
// with 1.
func (p *Prog) usageError(err error) error {
	stdopts.Errorf(p.Runtime, "timeout", "%v", err)
	stdopts.TryHelp(p.Runtime, "timeout")
	return &coreutils.ExitError{Code: 1, Err: err}
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand, the duration.
func parseFlags(args []string) (settings, []string, error) {
	var set settings
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'f':
		case 'k':
			set.killAfter = value
		case 'p':
			set.preserve = true
		case 's':
			set.signal = value
		case 'v':
			set.verbose = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return set, operands, err
}

// parseDuration converts a duration operand, a non-negative floating point
// number with an optional s, m, h or d unit suffix, as strtod would read it,
// into a duration. The infinity "inf" also reads, and, like a duration of 0
// or one too large to hold, never elapses; a fraction too small to hold
// still elapses, at once.
func parseDuration(operand string) (time.Duration, bool, error) {
	d, infinite, err := durationparse.Parse(operand, true)
	if err != nil {
		return 0, false, fmt.Errorf("invalid time interval '%s'", operand)
	}
	return d, infinite, nil
}

// parseKillAfter converts the -k duration. Like a duration of 0, an
// infinite one gives no grace period.
func parseKillAfter(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, infinite, err := parseDuration(s)
	if err != nil {
		return 0, err
	}
	if infinite {
		return 0, nil
	}
	return d, nil
}

// signalNames lists the signals which -s accepts, with the numbers the usual
// systems give them.
var signalNames = []struct {
	name string
	num  int
}{
	{"HUP", 1},
	{"INT", 2},
	{"QUIT", 3},
	{"ILL", 4},
	{"TRAP", 5},
	{"ABRT", 6},
	{"BUS", 7},
	{"FPE", 8},
	{"KILL", 9},
	{"USR1", 10},
	{"SEGV", 11},
	{"USR2", 12},
	{"PIPE", 13},
	{"ALRM", 14},
	{"TERM", 15},
	{"CHLD", 17},
	{"CONT", 18},
	{"STOP", 19},
	{"TSTP", 20},
	{"TTIN", 21},
	{"TTOU", 22},
	{"URG", 23},
	{"XCPU", 24},
	{"XFSZ", 25},
	{"VTALRM", 26},
	{"PROF", 27},
	{"WINCH", 28},
	{"SYS", 31},
}

// maxSignalNumber is the largest signal number accepted by -s, a pragmatic
// stand-in for NSIG.
const maxSignalNumber = 64

// parseSignal converts a -s signal specification, a case-independent name
// with or without the SIG prefix, or a number, into a signal number.
func parseSignal(spec string) (int, error) {
	if spec == "" {
		return 0, fmt.Errorf("invalid signal spec '%s'", spec)
	}
	if n, err := strconv.Atoi(spec); err == nil {
		if n > 0 && n <= maxSignalNumber {
			return n, nil
		}
		return 0, fmt.Errorf("invalid signal spec '%s'", spec)
	}
	name := strings.TrimPrefix(strings.ToUpper(spec), "SIG")
	for _, sig := range signalNames {
		if sig.name == name {
			return sig.num, nil
		}
	}
	return 0, fmt.Errorf("invalid signal spec '%s'", spec)
}

// signalName names a signal for the verbose messages.
func signalName(sig int) string {
	for _, s := range signalNames {
		if s.num == sig {
			return s.name
		}
	}
	return strconv.Itoa(sig)
}

// execResult is the outcome of running the command.
type execResult struct {
	status int
	err    error
}

// run executes the command, giving up on it when the duration elapses, or
// when the caller's context is done, since the only way to signal the child
// is canceling the context its execution runs with.
func (p *Prog) run(ctx context.Context, opts settings, duration time.Duration, infinite bool, killAfter time.Duration, sig int, command string, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Exec == nil {
		stdopts.Errorf(p.Runtime, "timeout", "no command execution available")
		return &coreutils.ExitError{Code: 125}
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan execResult, 1)
	go func() {
		status, err := p.Exec(child, coreutils.ExecOpts{Name: command, Args: args, Env: p.EnvList()})
		done <- execResult{status, err}
	}()

	if infinite || duration == 0 {
		// A duration of 0 enforces no time limit.
		select {
		case res := <-done:
			return p.report(ctx, command, res, false, opts.preserve, sig)
		case <-ctx.Done():
			cancel()
			<-done
			return ctx.Err()
		}
	}

	alarm := make(chan error, 1)
	waitCtx, stopWait := context.WithCancel(ctx)
	defer stopWait()
	go func() { alarm <- p.Wait(waitCtx, duration) }()

	select {
	case res := <-done:
		return p.report(ctx, command, res, false, opts.preserve, sig)
	case err := <-alarm:
		if err != nil {
			// The caller's context is done: give up on the child too,
			// which the cancellation of its context stops.
			cancel()
			return err
		}
	}

	// The time limit elapsed. Canceling the child is the closest this
	// runtime gets to sending it the signal, and the execution reports it
	// as terminated by a signal, unless it managed to exit in the race.
	if opts.verbose {
		stdopts.Errorf(p.Runtime, "timeout", "sending signal %s to command '%s'", signalName(sig), command)
	}
	cancel()

	var res execResult
	if killAfter > 0 {
		grace := make(chan error, 1)
		graceCtx, stopGrace := context.WithCancel(ctx)
		defer stopGrace()
		go func() { grace <- p.Wait(graceCtx, killAfter) }()
		select {
		case res = <-done:
		case err := <-grace:
			if err != nil {
				return err
			}
			if opts.verbose {
				stdopts.Errorf(p.Runtime, "timeout", "sending signal %s to command '%s'", signalName(sigKill), command)
			}
			// The KILL settles the status of the run, so there is no
			// need to wait for the child to go.
			return &coreutils.ExitError{Code: 128 + sigKill}
		}
	} else {
		res = <-done
	}
	return p.report(ctx, command, res, true, opts.preserve, sig)
}

// report computes the exit status of a finished execution. When the time
// limit elapsed, the failed execution reports the command as killed by the
// cancellation, and the status of dying from the signal sent is mimicked
// with --preserve-status.
func (p *Prog) report(ctx context.Context, command string, res execResult, timedOut, preserve bool, sig int) error {
	if timedOut {
		if res.err != nil && (res.status == 126 || res.status == 127) {
			stdopts.Errorf(p.Runtime, "timeout", "%s: %v", command, res.err)
			return &coreutils.ExitError{Code: res.status, Err: res.err}
		}
		if preserve {
			if res.err == nil {
				return &coreutils.ExitError{Code: res.status}
			}
			return &coreutils.ExitError{Code: 128 + sig}
		}
		return &coreutils.ExitError{Code: 124}
	}
	if res.err == nil {
		if res.status != 0 {
			return &coreutils.ExitError{Code: res.status}
		}
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stdopts.Errorf(p.Runtime, "timeout", "%s: %v", command, res.err)
	return &coreutils.ExitError{Code: res.status, Err: res.err}
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
