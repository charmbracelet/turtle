// Package touch implements the POSIX touch utility.
package touch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/fsx"
)

// Prog is the touch core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run sets the access and modification times of the given files, creating any
// that does not exist, unless -c is given.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, files, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if len(files) == 0 {
		p.errorf("missing file operand")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	atime, mtime, err := p.referenceTime(opts)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}

	ok := true
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := p.touch(file, opts, atime, mtime); err != nil {
			p.errorf("%s: %v", file, patherr.Unwrap(err))
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// referenceTime resolves the access and modification times to use, from the -d
// or -t time, from the -r ref_file times, or the current time.
func (p *Prog) referenceTime(opts options) (time.Time, time.Time, error) {
	loc := p.location()
	now := time.Now().In(loc)
	switch opts.source {
	case 'd':
		t, err := parseDateTime(opts.value, loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return t, t, nil
	case 'r':
		fi, err := fs.Stat(p.FS, filepath.ToSlash(p.ResolvePath(opts.value)))
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("%s: %v", opts.value, patherr.Unwrap(err))
		}
		mtime := fi.ModTime()
		atime := mtime
		if t, ok := fsx.Atime(fi); ok {
			atime = t
		}
		return atime, mtime, nil
	case 't':
		t, err := parseTime(opts.value, loc, now)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return t, t, nil
	default:
		return now, now, nil
	}
}

// location returns the time zone to interpret times with, honoring TZ.
func (p *Prog) location() *time.Location {
	if tz, ok := p.LookupEnv("TZ"); ok && tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.Local
}

// touch updates the times of a single file. Files that don't exist are created
// unless -c was given, and only the times selected by -a and -m are changed.
func (p *Prog) touch(name string, opts options, atime, mtime time.Time) error {
	path := filepath.ToSlash(p.ResolvePath(name))
	fi, err := fs.Stat(p.FS, path)
	switch {
	case errors.Is(err, fs.ErrNotExist) && opts.noCreate:
		return nil
	case errors.Is(err, fs.ErrNotExist):
		if err := p.create(path); err != nil {
			return err
		}
		if fi, err = fs.Stat(p.FS, path); err != nil {
			return err
		}
	case err != nil:
		return err
	}

	// The times not selected are kept from the file.
	newMtime := fi.ModTime()
	newAtime := newMtime
	if t, ok := fsx.Atime(fi); ok {
		newAtime = t
	}
	if opts.access {
		newAtime = atime
	}
	if opts.modify {
		newMtime = mtime
	}
	return p.FS.Chtimes(path, newAtime, newMtime)
}

// create creates an empty file with default permissions.
func (p *Prog) create(path string) error {
	f, err := p.FS.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	return f.Close()
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "touch: "+format+"\n", args...)
}

// options holds the -a, -c and -m flags, plus the option-argument of the last
// -d, -r or -t option given.
type options struct {
	access   bool
	modify   bool
	noCreate bool
	source   rune
	value    string
}

// longOptions maps the long options of touch to the short ones they stand
// for. The -a and -m options have no long form of their own: --time stands
// for them, taking the word which selects the time to change.
var longOptions = map[string]rune{
	"date":      'd',
	"no-create": 'c',
	"reference": 'r',
	"time":      optTime,
}

// optTime is the rune --time maps to. Having no short equivalent, it's kept
// above the bytes a group of short options can hold.
const optTime rune = 0x100

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--". The -d, -r and -t options take an option-argument,
// either glued to the option or in the next argument, and the last one given
// is the one used. The --time option takes the word which -a and -m each
// stand for.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	files, err := getopt.Set{Valued: "drt" + string(optTime), Long: longOptions}.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
			opts.access = true
		case 'c':
			opts.noCreate = true
		case 'm':
			opts.modify = true
		case 'd', 'r', 't':
			opts.source, opts.value = opt, value
		case optTime:
			switch value {
			case "atime", "access", "use":
				opts.access = true
			case "mtime", "modify":
				opts.modify = true
			default:
				return fmt.Errorf("invalid argument '%s' for '--time'", value)
			}
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	if !opts.access && !opts.modify {
		opts.access, opts.modify = true, true
	}
	return opts, files, nil
}

// parseTime parses a -t time of the form [[CC]YY]MMDDhhmm[.SS].
func parseTime(s string, loc *time.Location, now time.Time) (time.Time, error) {
	bad := fmt.Errorf("invalid date format %q", s)
	main, second, hasSecond := strings.Cut(s, ".")
	if !allDigits(main) {
		return time.Time{}, bad
	}

	var (
		year   = now.Year()
		prefix int
	)
	switch len(main) {
	case 12:
		prefix = 4
		year, _ = strconv.Atoi(main[:4])
	case 10:
		prefix = 2
		yy, _ := strconv.Atoi(main[:2])
		if yy >= 69 {
			year = 1900 + yy
		} else {
			year = 2000 + yy
		}
	case 8:
	default:
		return time.Time{}, bad
	}

	rest := main[prefix:]
	month := twoDigits(rest[0:2])
	day := twoDigits(rest[2:4])
	hour := twoDigits(rest[4:6])
	minute := twoDigits(rest[6:8])
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 {
		return time.Time{}, bad
	}
	sec := 0
	if hasSecond {
		if len(second) != 2 || !allDigits(second) {
			return time.Time{}, bad
		}
		if sec = twoDigits(second); sec > 60 {
			return time.Time{}, bad
		}
	}
	return time.Date(year, time.Month(month), day, hour, minute, sec, 0, loc), nil
}

// parseDateTime parses a -d date_time of the form
// YYYY-MM-DDThh:mm:SS[.frac][tz], where T can also be a space.
func parseDateTime(s string, loc *time.Location) (time.Time, error) {
	if strings.HasSuffix(s, "Z") {
		s, loc = s[:len(s)-1], time.UTC
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date format %q", s)
}

// twoDigits parses the decimal value of a two digit string.
func twoDigits(s string) int {
	return int(s[0]-'0')*10 + int(s[1]-'0')
}

// allDigits reports whether s is only made of decimal digits.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
