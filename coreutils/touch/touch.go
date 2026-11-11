// Package touch implements the POSIX touch utility.
package touch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/gnudate"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
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
// that does not exist, unless -c or -h is given. The -d, -r and -t options
// choose the times to set, and -h makes symbolic links themselves be acted
// on instead of the files they refer to.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "touch", usage, optSet, args); done {
		return err
	}
	opts, files, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "touch", err)
	}
	if len(files) == 0 {
		stdopts.Errorf(p.Runtime, "touch", "missing file operand")
		stdopts.TryHelp(p.Runtime, "touch")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "touch", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	atime, mtime, err := p.referenceTime(opts)
	if err != nil {
		stdopts.Errorf(p.Runtime, "touch", "%v", err)
		return &coreutils.ExitError{Code: 1}
	}

	ok := true
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := p.touch(file, opts, atime, mtime); err != nil {
			stdopts.Errorf(p.Runtime, "touch", "%s: %v", file, patherr.Unwrap(err))
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// referenceTime resolves the access and modification times to use, from the -d
// or -t time, from the -r ref_file times, or the current time. A -d value
// relative to a -r reference is resolved against its own times.
func (p *Prog) referenceTime(opts options) (time.Time, time.Time, error) {
	loc := p.location()
	now := p.Now().In(loc)

	var refAtime, refMtime time.Time
	if opts.refFile != "" {
		var err error
		if refAtime, refMtime, err = p.fileTimes(opts.refFile, opts.noDeref); err != nil {
			return time.Time{}, time.Time{}, err
		}
	}

	switch opts.source {
	case 'd':
		atime, mtime := now, now
		if opts.refFile != "" {
			atime, mtime = refAtime, refMtime
		}
		return flexTimes(opts.flex, opts, atime, mtime, loc)
	case 'r':
		// A -d value given before -r is still relative to the reference times.
		if opts.flex != "" {
			return flexTimes(opts.flex, opts, refAtime, refMtime, loc)
		}
		return refAtime, refMtime, nil
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

// flexTimes resolves a -d value against the base times: the access and
// modification times each get their own origin, as GNU does with -r.
func flexTimes(s string, opts options, atime, mtime time.Time, loc *time.Location) (time.Time, time.Time, error) {
	s = flexFraction(s)
	if opts.access {
		t, err := gnudate.Parse(s, atime, loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		atime = t
	}
	if opts.modify {
		t, err := gnudate.Parse(s, mtime, loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		mtime = t
	}
	return atime, mtime, nil
}

// flexFraction rewrites the standard format's "ss,frac" second fraction as
// "ss.frac", the form gnudate parses.
func flexFraction(s string) string {
	i := strings.LastIndex(s, ",")
	if i < 2 || s[i+1:] == "" || !allDigits(s[i+1:]) || !allDigits(s[i-2:i]) {
		return s
	}
	return s[:i] + "." + s[i+1:]
}

// fileTimes returns the access and modification times of a file, following
// or not following its final symbolic link as noDeref says.
func (p *Prog) fileTimes(name string, noDeref bool) (time.Time, time.Time, error) {
	path := p.FsName(name)
	fi, err := fs.Stat(p.FS, path)
	if noDeref {
		fi, err = p.FS.Lstat(path)
	}
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%s: %v", name, patherr.Unwrap(err))
	}
	mtime := fi.ModTime()
	atime := mtime
	if t, ok := fsx.Atime(fi); ok {
		atime = t
	}
	return atime, mtime, nil
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
// unless -c or -h was given, and only the times selected by -a and -m are
// changed. With -h the final symbolic link itself is the target; on a file
// system whose Chtimes follows links, like the OS one, it still acts on the
// file the link refers to.
func (p *Prog) touch(name string, opts options, atime, mtime time.Time) error {
	path := p.FsName(name)
	fi, err := fs.Stat(p.FS, path)
	if opts.noDeref {
		// Lstat describes the link itself, so a dangling one is not an error.
		fi, err = p.FS.Lstat(path)
	}
	switch {
	case errors.Is(err, fs.ErrNotExist) && opts.noDeref && !opts.noCreate:
		// -h never creates the missing file, but without -c it's still an error.
		return err
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

// options holds the -a, -c, -f, -h and -m flags, plus the option-argument of
// the last -d, -r or -t option given. The -d value is also kept when -r or -t
// follows it, as the origin for relative times, with refFile holding the -r
// reference file whenever it's given.
type options struct {
	access   bool
	modify   bool
	noCreate bool
	noDeref  bool
	source   rune
	value    string
	flex     string
	refFile  string
}

// longOptions maps the long options of touch to the short ones they stand
// for. The -a and -m options have no long form of their own: --time stands
// for them, taking the word which selects the time to change.
var longOptions = map[string]rune{
	"date":           'd',
	"no-create":      'c',
	"no-dereference": 'h',
	"reference":      'r',
	"time":           optTime,
}

// optTime is the rune --time maps to. Having no short equivalent, it's kept
// above the bytes a group of short options can hold.
const optTime rune = 0x100

// optSet describes the options of touch.
var optSet = getopt.Set{Valued: "drt" + string(optTime), Long: longOptions}

// usage describes the options of touch.
const usage = `Usage: touch [OPTION]... FILE...
Update the access and modification times of each FILE to the current time.

A FILE argument that does not exist is created, unless -c or -h is supplied.

  -a
         change only the access time
  -c, --no-create
         do not create any files
  -d, --date=STRING
         parse STRING and use it instead of current time
  -f
         (ignored)
  -h, --no-dereference
         affect each symbolic link instead of any referenced file
  -m
         change only the modification time
  -r, --reference=FILE
         use this file's times instead of current time
  -t STAMP
         use [[CC]YY]MMDDhhmm[.ss] instead of current time
      --time=WORD
         change the time WORD names: access, atime, use, modify or mtime
` + stdopts.Tail

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--". The -d, -r and -t options take an option-argument,
// either glued to the option or in the next argument, and the last one given
// is the one used. The --time option takes the word which -a and -m each
// stand for. The -f option is accepted and ignored, for BSD compatibility.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	files, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
			opts.access = true
		case 'c':
			opts.noCreate = true
		case 'f':
		case 'h':
			opts.noDeref = true
		case 'm':
			opts.modify = true
		case 'd':
			opts.source, opts.value, opts.flex = opt, value, value
		case 'r':
			opts.source, opts.value, opts.refFile = opt, value, value
		case 't':
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
