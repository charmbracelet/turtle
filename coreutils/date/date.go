// Package date implements the POSIX date utility.
//
// The GNU extensions for printing other times than the current one are
// supported too: -d/--date parses a date string, -f/--file parses one per
// line of a file, -r/--reference shows the modification time of a file, and
// -I/--iso-8601, -R/--rfc-email and --rfc-3339 pick fixed formats. The XSI
// settime operand is parsed to diagnose invalid dates, but it is never
// applied: the Runtime exposes a clock to read the current time, but no way
// to set it, so setting the date fails like a date running without the
// privileges to do so.
package date

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/gnudate"
	"charm.land/snail/coreutils/internal/input"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/strftime"
)

// defaultFormat is the output of date with no format operand, as the POSIX
// locale defines it.
const defaultFormat = "%a %b %e %H:%M:%S %Z %Y"

// Prog is the date core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the date and time to the standard output: the current one, the
// one described by the -d string, one per line of the -f file, or the
// modification time of the file given to -r, formatted by the +format operand
// or by one of the -I, -R and --rfc-3339 options when given.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "date", usage, optSet, args); done {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "date")
		return &coreutils.ExitError{Code: 1}
	}
	specified := 0
	for _, set := range []bool{opts.dateSet, opts.fileSet, opts.refSet} {
		if set {
			specified++
		}
	}
	if specified > 1 {
		p.errorf("the options to specify dates for printing are mutually exclusive")
		stdopts.TryHelp(p.Runtime, "date")
		return &coreutils.ExitError{Code: 1}
	}
	if len(operands) > 1 {
		p.errorf("extra operand '%s'", operands[1])
		stdopts.TryHelp(p.Runtime, "date")
		return &coreutils.ExitError{Code: 1}
	}

	loc := time.UTC
	if !opts.utc {
		loc = p.location()
	}
	now := p.Now()

	format, formatSet := opts.format, opts.format != ""
	if len(operands) == 1 {
		operand := operands[0]
		switch {
		case strings.HasPrefix(operand, "+"):
			if formatSet {
				p.errorf("multiple output formats specified")
				stdopts.TryHelp(p.Runtime, "date")
				return &coreutils.ExitError{Code: 1}
			}
			format = operand[1:]
			formatSet = true
		case specified > 0:
			p.errorf("the argument '%s' lacks a leading '+'; when using an option to specify date(s), any non-option argument must be a format string beginning with '+'", operand)
			stdopts.TryHelp(p.Runtime, "date")
			return &coreutils.ExitError{Code: 1}
		default:
			return p.setClock(operand, loc)
		}
	}
	if !formatSet {
		format = defaultFormat
	}

	if opts.fileSet {
		return p.datesFromFile(ctx, opts.file, format, loc, now)
	}

	t := now
	switch {
	case opts.dateSet:
		if t, err = gnudate.Parse(opts.dateStr, now, loc); err != nil {
			p.errorf("invalid date '%s'", opts.dateStr)
			return &coreutils.ExitError{Code: 1}
		}
	case opts.refSet:
		if t, err = p.referenceTime(opts.ref); err != nil {
			p.errorf("%v", err)
			return &coreutils.ExitError{Code: 1}
		}
	}
	return p.write(strftime.Format(format, t.In(loc)))
}

// setClock handles the XSI settime operand, mmddhhmm[[cc]yy]. It parses the
// operand to diagnose invalid dates, but since the Runtime has no way to set
// the system clock, it always fails in the end.
func (p *Prog) setClock(operand string, loc *time.Location) error {
	if _, err := parseSetTime(operand, p.Now(), loc); err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	p.errorf("cannot set date: setting the system clock is not supported")
	return &coreutils.ExitError{Code: 1}
}

// datesFromFile writes the date described by each line of the -f file, like
// one -d option per line. A line which fails to parse is reported and makes
// the program fail, after the valid lines before it were written.
func (p *Prog) datesFromFile(ctx context.Context, name, format string, loc *time.Location, now time.Time) error {
	in := input.New(name)
	r, err := input.Open(p.Runtime, ctx, in)
	if err != nil {
		p.errorf("%s: %v", in.Label(), patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	defer r.Close()

	ok := true
	br := bufio.NewReader(r)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, rerr := br.ReadString('\n')
		if rerr != nil && !errors.Is(rerr, io.EOF) {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			p.errorf("%s: %v", in.Label(), patherr.Unwrap(rerr))
			return &coreutils.ExitError{Code: 1, Err: rerr}
		}
		if line != "" {
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			t, err := gnudate.Parse(line, now, loc)
			if err != nil {
				p.errorf("invalid date '%s'", line)
				ok = false
			} else if werr := p.write(strftime.Format(format, t.In(loc))); werr != nil {
				return werr
			}
		}
		if rerr != nil {
			break
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// referenceTime returns the modification time of the file given to -r.
func (p *Prog) referenceTime(name string) (time.Time, error) {
	if p.FS == nil {
		return time.Time{}, errors.New("no file system available")
	}
	if name == "" {
		// An empty -r argument names no file, and resolving it against the
		// working directory would stat the directory itself.
		return time.Time{}, fmt.Errorf("failed to get timestamp of '': file does not exist")
	}
	fi, err := fs.Stat(p.FS, p.FsName(name))
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get timestamp of '%s': %v", name, patherr.Unwrap(err))
	}
	return fi.ModTime(), nil
}

// location returns the time zone to display the time in, honoring TZ. The
// zone name forms, like "America/New_York", are looked up in the system
// database, and the fixed-offset POSIX forms, like "UTC0" or "EST5", are
// parsed here. Values with DST rules, like "EST5EDT", are not supported and
// fall back to the local zone, like unknown ones.
func (p *Prog) location() *time.Location {
	if tz, ok := p.LookupEnv("TZ"); ok && tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
		if loc := parseTZ(tz); loc != nil {
			return loc
		}
	}
	return time.Local
}

// parseTZ parses the fixed-offset POSIX form of TZ, std[+-]hh[:mm[:ss]],
// like "UTC0" or "EST5", returning nil for anything else. The sign of the
// POSIX offset is inverted from the usual one: a positive value is west of
// Greenwich.
func parseTZ(tz string) *time.Location {
	// The zone name: three or more letters.
	i := 0
	for i < len(tz) && isLetter(tz[i]) {
		i++
	}
	if i < 3 {
		return nil
	}
	name, s := tz[:i], tz[i:]
	if s == "" {
		return nil
	}

	east := false
	if s[0] == '+' || s[0] == '-' {
		east = s[0] == '-'
		s = s[1:]
	}
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return nil
	}
	secs := 0
	for j, part := range parts {
		if part == "" || len(part) > 2 || !allDigits(part) {
			return nil
		}
		n := atoi(part)
		if j > 0 && n > 59 || j == 0 && n > 24 {
			return nil
		}
		secs += n * []int{3600, 60, 1}[j]
	}
	if secs/3600 == 24 && secs%3600 != 0 {
		return nil
	}
	if !east {
		secs = -secs
	}
	return time.FixedZone(name, secs)
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// write appends the newline the specification requires and writes the result
// to the standard output.
func (p *Prog) write(s string) error {
	if _, err := io.WriteString(p.Stdout, s+"\n"); err != nil {
		p.errorf("write error: %v", err)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// errorf writes a diagnostic message to the standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "date: "+format+"\n", args...)
}

// options holds the -u flag, whether -d, -f or -r were given with their
// option-arguments, and the format an option picked, empty when none did.
type options struct {
	utc     bool
	refSet  bool
	ref     string
	dateSet bool
	dateStr string
	fileSet bool
	file    string
	format  string
}

// rfc3339Opt is the pseudo option --rfc-3339 stands for, which has no short
// form. Its value is beyond any character a short option can be.
const rfc3339Opt = rune('\u0100')

// longOptions maps the long options of date to the options they stand for.
var longOptions = map[string]rune{
	"date":      'd',
	"file":      'f',
	"iso-8601":  'I',
	"reference": 'r',
	"rfc-822":   'R',
	"rfc-2822":  'R',
	"rfc-3339":  rfc3339Opt,
	"rfc-email": 'R',
	"universal": 'u',
	"utc":       'u',
}

// isoFormats maps the timespec of -I/--iso-8601 to the format it stands for,
// with "auto" as the default one, printing just the date.
var isoFormats = map[string]string{
	"auto":    "%Y-%m-%d",
	"date":    "%Y-%m-%d",
	"hours":   "%Y-%m-%dT%H%:z",
	"minutes": "%Y-%m-%dT%H:%M%:z",
	"seconds": "%Y-%m-%dT%H:%M:%S%:z",
	"ns":      "%Y-%m-%dT%H:%M:%S,%N%:z",
}

// rfc3339Formats maps the timespec of --rfc-3339 to the format it stands
// for, like the ISO 8601 one but with a space and a period.
var rfc3339Formats = map[string]string{
	"date":    "%Y-%m-%d",
	"seconds": "%Y-%m-%d %H:%M:%S%:z",
	"ns":      "%Y-%m-%d %H:%M:%S.%N%:z",
}

// rfcEmailFormat is the format of -R/--rfc-email, from the Internet email
// standards RFC 5322, 2822 and 822.
const rfcEmailFormat = "%a, %d %b %Y %H:%M:%S %z"

// optSet describes the options of date.
var optSet = getopt.Set{
	Valued:   "dfr" + string(rfc3339Opt),
	Optional: "I",
	Long:     longOptions,
}

// usage describes the options of date.
const usage = `Usage: date [OPTION]... [+FORMAT]
  or:  date [OPTION]... MMDDhhmm[[CC]YY][.ss]
Print or set the system date and time.

  -d, --date=STRING
         display the time described by STRING, not 'now'
  -f, --file=DATEFILE
         like --date, once for each line of DATEFILE
  -I[FMT], --iso-8601[=FMT]
         output an ISO 8601 date, FMT='date' (default), 'hours', 'minutes',
         'seconds' or 'ns'
  -r, --reference=FILE
         display the last modification time of FILE
  -R, --rfc-email
         output date and time in RFC 5322 format
      --rfc-3339=FMT
         output date and time in RFC 3339 format, FMT='date', 'seconds' or 'ns'
  -u, --utc, --universal
         print or set Coordinated Universal Time (UTC)
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--". The -d, -f and -r options take an
// option-argument, either glued to the option or in the next argument, and
// -I takes an optional one. Repeating -d keeps only the last value.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	// The pseudo option of --rfc-3339 requires an argument too, so it joins
	// the valued characters, where no short option can ever match it.
	operands, err := optSet.Parse(bareISO(args), func(opt rune, value string) error {
		switch opt {
		case 'd':
			opts.dateSet = true
			opts.dateStr = value
		case 'f':
			opts.fileSet = true
			opts.file = value
		case 'I':
			if value == "" {
				value = "auto"
			}
			format, ok := isoFormats[value]
			if !ok {
				return fmt.Errorf("invalid argument '%s' for '--iso-8601'", value)
			}
			opts.format = format
		case 'r':
			opts.refSet = true
			opts.ref = value
		case 'R':
			opts.format = rfcEmailFormat
		case 'u':
			opts.utc = true
		case rfc3339Opt:
			format, ok := rfc3339Formats[value]
			if !ok {
				return fmt.Errorf("invalid argument '%s' for '--rfc-3339'", value)
			}
			opts.format = format
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// bareISO rewrites the short -I options which end their group, like "-I" or
// "-uI", into the long one: the getopt package takes the argument of a short
// optional option from the next argument, while the timespec of -I is
// optional and only the one glued to the option counts.
func bareISO(args []string) []string {
	out := make([]string, 0, len(args)+1)
	for _, arg := range args {
		prefix, ok := splitISO(arg)
		if !ok {
			out = append(out, arg)
			continue
		}
		if prefix != "-" {
			out = append(out, prefix)
		}
		out = append(out, "--iso-8601")
	}
	return out
}

// splitISO tells whether arg is a group of short options ending in -I, with
// no option before it swallowing the rest of the group, returning the group
// without the trailing I.
func splitISO(arg string) (string, bool) {
	if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
		return "", false
	}
	for j := 1; j < len(arg); j++ {
		switch {
		case strings.IndexByte("dfr", arg[j]) >= 0:
			// A valued option: the rest of the group is its argument.
			return "", false
		case arg[j] == 'I' && j < len(arg)-1:
			// The timespec is glued to the option, like in -Iseconds.
			return "", false
		case arg[j] == 'I':
			return arg[:len(arg)-1], true
		}
	}
	return "", false
}

// parseSetTime parses the XSI settime operand, mmddhhmm[[cc]yy], as the time
// to set the system clock to. A two-digit year is expanded per the
// specification, and the current year is the default when it is omitted.
func parseSetTime(operand string, now time.Time, loc *time.Location) (time.Time, error) {
	invalid := func() (time.Time, error) {
		return time.Time{}, fmt.Errorf("invalid date '%s'", operand)
	}
	switch len(operand) {
	case 8, 10, 12:
	default:
		return invalid()
	}
	for _, c := range operand {
		if c < '0' || c > '9' {
			return invalid()
		}
	}

	year := now.Year()
	switch len(operand) {
	case 12:
		year = atoi(operand[8:12])
	case 10:
		if yy := atoi(operand[8:10]); yy >= 69 {
			year = 1900 + yy
		} else {
			year = 2000 + yy
		}
	}

	month, day := atoi(operand[0:2]), atoi(operand[2:4])
	hour, min := atoi(operand[4:6]), atoi(operand[6:8])
	if month < 1 || month > 12 || hour > 23 || min > 59 {
		return invalid()
	}
	t := time.Date(year, time.Month(month), day, hour, min, 0, 0, loc)
	// A day beyond the month's length is normalized into the next one, so
	// comparing it back tells whether the date was valid.
	if t.Month() != time.Month(month) || t.Day() != day {
		return invalid()
	}
	return t, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

var _ coreutils.Program = (*Prog)(nil)
