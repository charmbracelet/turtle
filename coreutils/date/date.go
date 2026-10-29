// Package date implements the POSIX date utility.
//
// The -r option is a GNU extension: it displays the modification time of a
// file instead of the current time. The XSI settime operand is parsed to
// diagnose invalid dates, but it is never applied: the Runtime exposes a
// clock to read the current time, but no way to set it, so setting the date
// fails like a date running without the privileges to do so.
package date

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
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

// Run writes the date and time to the standard output: the current one, or
// the modification time of the file given to -r, formatted by the +format
// operand when given.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	if len(operands) > 1 {
		p.errorf("extra operand '%s'", operands[1])
		return &coreutils.ExitError{Code: 1}
	}

	loc := time.UTC
	if !opts.utc {
		loc = p.location()
	}

	if len(operands) == 1 && !strings.HasPrefix(operands[0], "+") {
		return p.setClock(operands[0], loc)
	}

	t := p.Now()
	if opts.refSet {
		if t, err = p.referenceTime(opts.ref); err != nil {
			p.errorf("%v", err)
			return &coreutils.ExitError{Code: 1}
		}
	}

	format := defaultFormat
	if len(operands) == 1 {
		format = operands[0][1:]
	}
	return p.write(strftime(format, t.In(loc)))
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
	fi, err := fs.Stat(p.FS, filepath.ToSlash(p.ResolvePath(name)))
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

// options holds the -u flag, and whether -r was given, with the
// option-argument it took.
type options struct {
	utc    bool
	refSet bool
	ref    string
}

// longOptions maps the long options of date to the short ones they stand for.
var longOptions = map[string]rune{
	"reference": 'r',
	"universal": 'u',
	"utc":       'u',
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--". The -r option takes an option-argument, either
// glued to the option or in the next argument.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := getopt.Set{Valued: "r", Long: longOptions}.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'r':
			opts.refSet = true
			opts.ref = value
		case 'u':
			opts.utc = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
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
