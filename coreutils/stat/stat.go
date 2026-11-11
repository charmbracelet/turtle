// Package stat implements the stat core utility, following the GNU manual.
//
// It writes the information the file system holds about each file given, as
// a default format or one built from the format directives of --format and
// --printf, and the file system status with -f.
//
// The file system abstraction used here carries no file system statistics,
// so -f writes every file system field but the name as a question mark, the
// way GNU writes the values a platform doesn't report. A file whose system
// information isn't available, like in a memory file system, is written with
// a device number, a file serial number and a device type of zero, a link
// count of one, no access-time hint beyond the 512-byte block size, and its
// blocks measured by its size. The time of the last status change a file
// system doesn't report is written as the modification time, and the birth
// time as a question mark for %w and zero for %W. The SELinux context and
// the mount point are written as a question mark, marking the file as
// failed the way GNU does.
//
// Timestamps are written in the zone TZ names, or the local zone when TZ is
// unset; the fixed-offset POSIX forms of TZ, like "EST5", aren't supported
// and fall back to the local zone.
package stat

import (
	"context"
	"fmt"
	"io/fs"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the stat core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// The long options which have no short equivalent, as runes above the bytes
// a group of short options can hold, keeping them out of the short syntax.
const (
	optPrintf rune = 0x100 + iota
	optCached
)

// longOptions maps the long options of stat to the short ones they stand
// for, or to the runes of the ones with no short equivalent.
var longOptions = map[string]rune{
	"cached":      optCached,
	"dereference": 'L',
	"file-system": 'f',
	"format":      'c',
	"printf":      optPrintf,
	"terse":       't',
}

// options are the options parsed from the command line.
type options struct {
	deref     bool   // -L: act on the files links refer to
	fs        bool   // -f: report the file system status
	terse     bool   // -t: terse form
	format    string // -c: the format of the output
	formatSet bool   // whether -c was given, even with an empty format
	printf    string // --printf: the format, with escapes taken
	printfSet bool   // whether --printf was given, even with an empty format
	cachedSet bool   // --cached given, whose value is only checked
}

// optSet describes the options of stat.
var optSet = getopt.Set{
	Valued: "c" + string(optPrintf) + string(optCached),
	Long:   longOptions,
}

// Run writes the status of each file named in args, as the options ask
// for. A file that can't be read is diagnosed and the run goes on, and the
// run reports failure when any file failed.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "stat", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "stat")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if len(operands) == 0 {
		p.errorf("missing operand")
		stdopts.TryHelp(p.Runtime, "stat")
		return &coreutils.ExitError{Code: 2}
	}
	r := p.newRenderer()
	format, escapes, given := chooseFormat(opts)
	failed := false
	for _, name := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "" {
			p.errorf("invalid zero-length file name")
			failed = true
			continue
		}
		st, ok := p.statOf(name, opts)
		if !ok {
			failed = true
			continue
		}
		text := format
		switch {
		case given:
		case opts.fs && opts.terse:
			text = terseFSFormat
		case opts.fs:
			text = fsFormat
		case opts.terse:
			text = terseFormat
		case isDevice(st.fi.Mode()):
			text = defaultFormatDevice
		default:
			text = defaultFormat
		}
		out, fail, err := r.render(text, &st, opts.fs, escapes)
		if err != nil {
			// The text before the invalid directive is written, the way
			// GNU leaves what it wrote before the directive fails it.
			fmt.Fprint(p.Stdout, out)
			p.errorf("%v", err)
			return &coreutils.ExitError{Code: 1}
		}
		if given && !escapes {
			// --format is newline-terminated, unlike --printf.
			out += "\n"
		}
		fmt.Fprint(p.Stdout, out)
		failed = failed || fail
	}
	if failed {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// parseFlags parses the options of stat out of args, returning the operands
// left after them.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'L':
			opts.deref = true
		case 'f':
			opts.fs, opts.deref = true, true
		case 't':
			opts.terse = true
		case 'c':
			opts.format, opts.formatSet, opts.printfSet = value, true, false
		case optPrintf:
			opts.printf, opts.printfSet, opts.formatSet = value, true, false
		case optCached:
			switch value {
			case "always", "never", "default":
			default:
				return fmt.Errorf("invalid argument '%s' for '--cached'", value)
			}
			opts.cachedSet = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

// chooseFormat returns the format a --printf or --format option names,
// with whether its backslash escapes are taken and whether one was given at
// all. An explicitly empty format is one that was given, not the default
// one. The formats left to the options choose themselves per file.
func chooseFormat(opts options) (format string, escapes, given bool) {
	switch {
	case opts.printfSet:
		return opts.printf, true, true
	case opts.formatSet:
		return opts.format, false, true
	case opts.terse:
		return terseFormat, false, false
	case opts.fs:
		return fsFormat, false, false
	default:
		return "", false, false
	}
}

// statOf gathers the information the file system holds about the file named
// by an operand, reporting failure when it can't be read.
func (p *Prog) statOf(name string, opts options) (fileStat, bool) {
	st := fileStat{name: name}
	path := p.FsPath(name)
	var fi fs.FileInfo
	var err error
	if opts.deref {
		fi, err = fs.Stat(p.FS, path)
	} else {
		fi, err = p.FS.Lstat(path)
	}
	if err != nil {
		if opts.fs {
			p.errorf("cannot read file system information for '%s': %v", name, patherr.Unwrap(err))
		} else {
			p.errorf("cannot stat '%s': %v", name, patherr.Unwrap(err))
		}
		return st, false
	}
	return p.newFileStat(name, path, fi, opts), true
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "stat: "+format+"\n", args...)
}

// usage describes the options of stat.
const usage = `Usage: stat [OPTION]... [FILE]...
Display file or file system status.

  -L, --dereference
         follow links
  -f, --file-system
         display file system status instead of file status
      --cached=MODE
         how attributes are cached; MODE is 'always', 'never' or
         'default', and the attribute caching of the file system
         is left alone here
  -c, --format=FORMAT
         use the specified FORMAT instead of the default;
         output a newline after each use of FORMAT
      --printf=FORMAT
         like --format, but interpret backslash escapes,
         and do not output a mandatory trailing newline;
         if you want a newline, include '\n' in FORMAT
  -t, --terse
         print the information in terse form

      --help
         display this help and exit
      --version
         output version information and exit
`
