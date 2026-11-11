// Package time implements the POSIX time utility, with the format string
// extensions of the GNU one.
//
// The real time of the run is measured with the runtime clock around the
// execution of the utility. The user and system CPU times and the other
// resource counters come from the runtime execution-usage hook; the averaged
// memory sizes the hook does not carry, like the shared text and unshared
// data sizes, are always written as zero, as they are on the systems whose
// kernels do not fill them in.
//
// When the runtime cannot report the usage counters, every resource
// specifier but the elapsed time degrades to zero, and the built-in default
// format degrades to the three POSIX lines of real, user and system time.
//
// The percentage of the CPU is computed from the full durations the runtime
// carries, rather than from the millisecond-truncated ones of GNU time, so
// it keeps its precision on very short runs.
package time

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"math/bits"
	"os"
	"strings"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the time core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// The output format of the GNU time default, and the one the -p option
// selects.
const (
	defaultFormat = "%Uuser %Ssystem %Eelapsed %PCPU (%Xavgtext+%Davgdata " +
		"%Mmaxresident)k\n%Iinputs+%Ooutputs (%Fmajor+%Rminor)pagefaults %Wswaps"
	posixFormat = "real %e\nuser %U\nsys %S"
)

// The verbose format the -v option selects, the one of GNU time.
const verboseFormat = "\tCommand being timed: \"%C\"\n" +
	"\tUser time (seconds): %U\n" +
	"\tSystem time (seconds): %S\n" +
	"\tPercent of CPU this job got: %P\n" +
	"\tElapsed (wall clock) time (h:mm:ss or m:ss): %E\n" +
	"\tAverage shared text size (kbytes): %X\n" +
	"\tAverage unshared data size (kbytes): %D\n" +
	"\tAverage stack size (kbytes): %p\n" +
	"\tAverage total size (kbytes): %K\n" +
	"\tMaximum resident set size (kbytes): %M\n" +
	"\tAverage resident set size (kbytes): %t\n" +
	"\tMajor (requiring I/O) page faults: %F\n" +
	"\tMinor (reclaiming a frame) page faults: %R\n" +
	"\tVoluntary context switches: %w\n" +
	"\tInvoluntary context switches: %c\n" +
	"\tSwaps: %W\n" +
	"\tFile system inputs: %I\n" +
	"\tFile system outputs: %O\n" +
	"\tSocket messages sent: %s\n" +
	"\tSocket messages received: %r\n" +
	"\tSignals delivered: %k\n" +
	"\tPage size (bytes): %Z\n" +
	"\tExit status: %x"

// settings holds what a run of time selects.
type settings struct {
	append    bool   // -a
	quiet     bool   // -q
	verbose   bool   // -v
	outfile   string // -o
	outGiven  bool   // -o was given, even with an empty value
	format    string // -f, -p, or the TIME variable
	formatSet bool   // a format was given with -f or -p
	posix     bool   // -p was the last format-setting option
}

// longOptions maps the long options of time to the short ones they stand
// for. The --output-file spelling of -o is the one the GNU program accepts.
var longOptions = map[string]rune{
	"append":      'a',
	"format":      'f',
	"output":      'o',
	"output-file": 'o',
	"portability": 'p',
	"quiet":       'q',
	"verbose":     'v',
}

// optSet describes the options of time.
var optSet = getopt.Set{Valued: "fo", Long: longOptions}

// usage describes the options of time.
const usage = `Usage: time [OPTION]... COMMAND [ARG]...
Run COMMAND, then print system resource usage.

  -a, --append
         with -o FILE, append instead of overwriting
  -f, --format=FORMAT
         use the specified FORMAT instead of the default
  -o, --output=FILE
         write to FILE instead of the standard error
  -p, --portability
         write the timing statistics in the POSIX format
  -q, --quiet
         do not print information about abnormal program termination
         (non-zero exit codes or signals)
  -v, --verbose
         print all resource usage information instead of the default format
      --help
         display this help and exit
      --version
         output version information and exit
`

// Run invokes the utility operand with the argument operands, and writes the
// timing statistics of the run to the standard error, or to the file the -o
// option names. The exit status of the utility becomes the exit status of
// time, and the 127, 126 and 125 statuses of a failed execution are passed
// along with a diagnostic, without statistics. The context error is returned
// as-is when the caller's context is done.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "time", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return p.usageError(err)
	}
	if len(operands) == 0 {
		return p.usageError(errors.New("missing operand"))
	}
	p.chooseFormat(&opts)
	out, closeOut, err := p.openOutput(&opts)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 125, Err: err}
	}
	defer closeOut()
	return p.invoke(ctx, out, opts, operands[0], operands[1:])
}

// usageError reports a usage error, followed by the standard hint, exiting
// with 1.
func (p *Prog) usageError(err error) error {
	p.errorf("%v", err)
	stdopts.TryHelp(p.Runtime, "time")
	return &coreutils.ExitError{Code: 1, Err: err}
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand, the utility.
func parseFlags(args []string) (settings, []string, error) {
	var opts settings
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
			opts.append = true
		case 'f':
			opts.format, opts.formatSet, opts.posix = value, true, false
		case 'o':
			opts.outfile, opts.outGiven = value, true
		case 'p':
			opts.format, opts.formatSet, opts.posix = posixFormat, true, true
		case 'q':
			opts.quiet = true
		case 'v':
			opts.verbose = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// chooseFormat selects the format string of the run. The verbose format
// wins over every other selection, then the -f format, then the TIME
// environment variable, and last the built-in default. The verbose format
// also takes the run out of the POSIX mode of the header of a non-zero
// status, as the GNU selection of the verbose format does.
func (p *Prog) chooseFormat(opts *settings) {
	switch {
	case opts.verbose:
		opts.format, opts.posix = verboseFormat, false
	case opts.formatSet:
	default:
		if env, ok := p.LookupEnv("TIME"); ok {
			opts.format = env
		} else {
			opts.format = defaultFormat
		}
	}
}

// openOutput returns the stream the statistics are written to, and a
// function closing it. It is the standard error unless -o was given, even
// with an empty value: an explicitly empty file name is an invalid path,
// failing the open as an empty fopen argument fails in GNU time.
func (p *Prog) openOutput(opts *settings) (io.Writer, func(), error) {
	if !opts.outGiven {
		return p.Stderr, func() {}, nil
	}
	if p.FS == nil {
		return nil, nil, errors.New("no file system available")
	}
	if opts.outfile == "" {
		return nil, nil, &fs.PathError{Op: "open", Path: "", Err: fs.ErrInvalid}
	}
	flag := os.O_WRONLY | os.O_CREATE
	if opts.append {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := p.FS.OpenFile(p.FsName(opts.outfile), flag, 0o666&^p.Umask.Perm())
	if err != nil {
		return nil, nil, err
	}
	return f, func() { f.Close() }, nil
}

// invoke runs the utility with the arguments in the working directory of the
// runtime, timing the run with the runtime clock and taking the resource
// counters from the usage reports bracketing it. The statistics are written
// when the utility runs; a failed execution, which cannot be found, cannot
// be invoked or is terminated by a signal, gets only a diagnostic.
func (p *Prog) invoke(ctx context.Context, out io.Writer, opts settings, utility string, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Exec == nil {
		p.errorf("no command execution available")
		return &coreutils.ExitError{Code: 125}
	}
	before, haveBefore := p.usage()
	start := p.Now()
	status, err := p.Exec(ctx, coreutils.ExecOpts{Name: utility, Args: args, Env: p.EnvList()})
	elapsed := p.Now().Sub(start)
	after, haveAfter := p.usage()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: status, Err: err}
	}
	st := stats{
		elapsed: elapsed,
		usage:   after.Sub(before),
		status:  status,
		command: command(utility, args),
	}
	if err := p.writeStats(out, opts, st, haveBefore && haveAfter); err != nil {
		p.errorf("write error: %v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	if status != 0 {
		return &coreutils.ExitError{Code: status}
	}
	return nil
}

// usage reports the cumulative resource usage of the executed children,
// reporting false when the runtime provides no reports.
func (p *Prog) usage() (coreutils.ExecUsage, bool) {
	if p.ExecUsage == nil {
		return coreutils.ExecUsage{}, false
	}
	return p.ExecUsage()
}

// command joins the utility and its arguments, as the %C specifier writes
// them.
func command(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + strings.Join(args, " ")
}

// writeStats writes the statistics to out, with the warning of a non-zero
// exit status ahead of them. The warning is written unless -q selects
// against it, or the run is in the POSIX mode, where the -p option was the
// last format-setting one, or the usage counters were unavailable and the
// built-in default was in effect, degrading it to the POSIX format.
func (p *Prog) writeStats(out io.Writer, opts settings, st stats, available bool) error {
	format, posixMode := opts.format, opts.posix
	if !available && format == defaultFormat {
		format, posixMode = posixFormat, true
	}
	var b strings.Builder
	if st.status != 0 && !opts.quiet && !posixMode {
		fmt.Fprintf(&b, "Command exited with non-zero status %d\n", st.status)
	}
	b.WriteString(st.summarize(format))
	_, err := io.WriteString(out, b.String())
	return err
}

// stats carries what the format specifiers report about one run.
type stats struct {
	elapsed time.Duration
	usage   coreutils.ExecUsage
	status  int
	command string
}

// summarize renders the statistics in the format, following the GNU format
// string: a backslash escapes the tab, newline and backslash characters, a
// percent sign introduces a resource specifier, and an unknown escape or
// specifier is written as a question mark before it. A trailing newline is
// always added.
func (s stats) summarize(format string) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		switch format[i] {
		case '%':
			i++
			if i >= len(format) {
				b.WriteByte('?')
				break
			}
			switch format[i] {
			case '%':
				b.WriteByte('%')
			case 'C':
				b.WriteString(s.command)
			case 'D', 'K', 'X', 'p', 't':
				// The averaged memory sizes are not carried by the
				// runtime usage reports.
				b.WriteByte('0')
			case 'E':
				s.writeElapsed(&b)
			case 'e':
				writeSeconds(&b, s.elapsed)
			case 'F':
				fmt.Fprintf(&b, "%d", s.usage.MajorFaults)
			case 'I':
				fmt.Fprintf(&b, "%d", s.usage.FSInputs)
			case 'M':
				fmt.Fprintf(&b, "%d", s.usage.MaxRSS/1024)
			case 'O':
				fmt.Fprintf(&b, "%d", s.usage.FSOutputs)
			case 'P':
				s.writePercent(&b)
			case 'R':
				fmt.Fprintf(&b, "%d", s.usage.MinorFaults)
			case 'S':
				writeSeconds(&b, s.usage.SysTime)
			case 'T':
				i++
				if i >= len(format) {
					b.WriteString("T=missing letter")
					break
				}
				switch format[i] {
				case 'n', 's':
					// The runtime reports no signal terminations
					// among the statistics.
				case 't':
					b.WriteString("normal")
				case 'x':
					fmt.Fprintf(&b, "%d", s.status)
				case 'o':
					if s.status == 0 {
						b.WriteString("ok")
					}
				default:
					b.WriteString("T?=unknown")
				}
			case 'U':
				writeSeconds(&b, s.usage.UserTime)
			case 'W':
				fmt.Fprintf(&b, "%d", s.usage.Swaps)
			case 'Z':
				fmt.Fprintf(&b, "%d", os.Getpagesize())
			case 'c':
				fmt.Fprintf(&b, "%d", s.usage.InvoluntarySwitches)
			case 'k':
				fmt.Fprintf(&b, "%d", s.usage.Signals)
			case 'r':
				fmt.Fprintf(&b, "%d", s.usage.SocketsReceived)
			case 's':
				fmt.Fprintf(&b, "%d", s.usage.SocketsSent)
			case 'w':
				fmt.Fprintf(&b, "%d", s.usage.VoluntarySwitches)
			case 'x':
				fmt.Fprintf(&b, "%d", s.status)
			default:
				b.WriteByte('?')
				b.WriteByte(format[i])
			}
		case '\\':
			i++
			if i >= len(format) {
				b.WriteString("?\\")
				break
			}
			switch format[i] {
			case 't':
				b.WriteByte('\t')
			case 'n':
				b.WriteByte('\n')
			case '\\':
				b.WriteByte('\\')
			default:
				b.WriteString("?\\")
				b.WriteByte(format[i])
			}
		default:
			b.WriteByte(format[i])
		}
	}
	b.WriteByte('\n')
	return b.String()
}

// writeElapsed writes the elapsed time in [hours:]minutes:seconds, with two
// fractional digits on the seconds below an hour, as GNU time writes.
func (s stats) writeElapsed(b *strings.Builder) {
	secs := int64(s.elapsed / time.Second)
	if secs >= 3600 {
		fmt.Fprintf(b, "%d:%02d:%02d", secs/3600, secs%3600/60, secs%60)
		return
	}
	fmt.Fprintf(b, "%d:%02d.%02d", secs/60, secs%60,
		int64((s.elapsed%time.Second)/(10*time.Millisecond)))
}

// writeSeconds writes a duration as seconds with two fractional digits,
// truncating the extra precision as GNU time does.
func writeSeconds(b *strings.Builder, d time.Duration) {
	fmt.Fprintf(b, "%d.%02d", int64(d/time.Second),
		int64((d%time.Second)/(10*time.Millisecond)))
}

// writePercent writes the share of the CPU the run got, the user and system
// CPU times divided by the elapsed time, floored to a whole percent. Unlike
// GNU time, which computes the ratio from millisecond-truncated values and
// only falls back to microseconds, the ratio here comes from the full
// durations the runtime carries, so very short runs keep their precision;
// a question mark is written only when the elapsed time is zero.
func (s stats) writePercent(b *strings.Builder) {
	if s.elapsed == 0 {
		b.WriteString("?%")
		return
	}
	cpu := addDurations(s.usage.UserTime, s.usage.SysTime)
	fmt.Fprintf(b, "%d%%", percentOf(cpu, s.elapsed))
}

// addDurations adds two durations, saturating at the largest one when the
// sum overflows, as the user and system times of a very long run could.
func addDurations(a, b time.Duration) time.Duration {
	sum := a + b
	if sum < a {
		return time.Duration(math.MaxInt64)
	}
	return sum
}

// percentOf computes floor(cpu*100/elapsed) by 128-bit division, free of
// the overflows of the naive product, saturating at the largest percent
// when even the quotient overflows, which the real clocks cannot reach.
// The durations are never negative here.
func percentOf(cpu, elapsed time.Duration) uint64 {
	hi, lo := bits.Mul64(uint64(cpu), 100)
	if hi >= uint64(elapsed) {
		return math.MaxUint64
	}
	quotient, _ := bits.Div64(hi, lo, uint64(elapsed))
	return quotient
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "time: "+format+"\n", args...)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
