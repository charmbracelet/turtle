// Package readlink implements the GNU readlink utility.
//
// Without -f, -e or -m, it prints the target of each symbolic link operand
// as stored. Otherwise it canonicalizes the operands like realpath does,
// through the realpath package.
package readlink

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/realpath"
)

// Prog is the readlink core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run prints the value of each operand: its link target, or its
// canonicalized absolute name when -f, -e or -m asks. All operands are
// processed, and the exit status is 1 when any of them fails.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "readlink", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	switch {
	case err != nil:
		return p.usageError(err)
	case len(operands) == 0:
		return p.usageError(errors.New("missing operand"))
	case slices.Contains(operands, ""):
		p.errorf("invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}

	// Most errors are suppressed, unless -v asks for them, or -q leaves
	// the default of POSIXLY_CORRECT stand, which is set even when empty.
	_, posixlyCorrect := p.LookupEnv("POSIXLY_CORRECT")
	verbose := opts.verbose || (!opts.explicit && posixlyCorrect)

	delims := "\n"
	if opts.zero {
		delims = "\x00"
	}
	switch {
	case !opts.noNewline:
	case len(operands) == 1:
		delims = ""
	default:
		p.errorf("warning: -n specified with more than one operand; not printing delimiters")
		delims = ""
	}

	ok := true
	for _, operand := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		out, err := p.value(ctx, operand, opts)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if verbose {
				p.errorf("%s: %v", operand, err)
			}
			ok = false
			continue
		}
		if _, err := fmt.Fprintf(p.Stdout, "%s%s", out, delims); err != nil {
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// value returns the output for one operand: the target of the symbolic
// link it names, or its canonicalized name in canonicalize mode.
func (p *Prog) value(ctx context.Context, operand string, opts options) (string, error) {
	if !opts.canonical {
		name := p.FsPath(operand)
		fi, err := p.FS.Lstat(name)
		if err != nil {
			return "", patherr.Unwrap(err)
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			return "", errNotSymlink
		}
		target, err := p.FS.ReadLink(name)
		if err != nil {
			return "", patherr.Unwrap(err)
		}
		return target, nil
	}
	return realpath.Canonicalize(ctx, p.FS, p.FsAbs(operand), realpath.Options{Mode: opts.mode})
}

// options holds the readlink flags. mode starts as AllButLast, and -f, -e
// and -m each turn canonicalize on and replace whichever mode came before.
// The explicit and verbose fields keep the last of -q and -v standing.
type options struct {
	canonical bool
	mode      realpath.Mode
	noNewline bool
	explicit  bool
	verbose   bool
	zero      bool
}

// longOptions maps the long options of readlink to the characters they
// stand for.
var longOptions = map[string]rune{
	"canonicalize":          'f',
	"canonicalize-existing": 'e',
	"canonicalize-missing":  'm',
	"no-newline":            'n',
	"quiet":                 'q',
	"silent":                'q',
	"verbose":               'v',
	"zero":                  'z',
}

// optSet describes the options of readlink.
var optSet = getopt.Set{Long: longOptions}

// usage describes the options of readlink.
const usage = `Usage: readlink [OPTION]... FILE...
Print value of a symbolic link or canonical file name.

  -f, --canonicalize
         canonicalize by following every symbolic link of every component of
         the given name recursively; all but the last component must exist
  -e, --canonicalize-existing
         canonicalize by following every symbolic link of every component of
         the given name recursively; all components must exist
  -m, --canonicalize-missing
         canonicalize by following every symbolic link of every component of
         the given name recursively; missing components are treated as
         directories
  -n, --no-newline
         do not print the output delimiter, when a single file is specified
  -q, --quiet
  -s, --silent
         suppress most error messages
  -v, --verbose
         report error messages
  -z, --zero
         end each output line with NUL, not newline
      --help
         display this help and exit
      --version
         output version information and exit
`

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'f':
			opts.canonical, opts.mode = true, realpath.AllButLast
		case 'e':
			opts.canonical, opts.mode = true, realpath.Existing
		case 'm':
			opts.canonical, opts.mode = true, realpath.Missing
		case 'n':
			opts.noNewline = true
		case 'q', 's':
			opts.explicit, opts.verbose = true, false
		case 'v':
			opts.explicit, opts.verbose = true, true
		case 'z':
			opts.zero = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	return opts, operands, nil
}

// usageError reports a usage error, with the hint that follows them.
func (p *Prog) usageError(err error) error {
	p.errorf("%v", err)
	stdopts.TryHelp(p.Runtime, "readlink")
	return &coreutils.ExitError{Code: 1, Err: err}
}

// errNotSymlink is returned when an operand names no symbolic link.
var errNotSymlink = errors.New("not a symbolic link")

// errorf writes a diagnostic message to the standard error.
func (p *Prog) errorf(format string, args ...any) {
	if p.Stderr != nil {
		fmt.Fprintf(p.Stderr, "readlink: "+format+"\n", args...)
	}
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
