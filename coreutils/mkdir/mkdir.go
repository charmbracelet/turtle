// Package mkdir implements the POSIX mkdir utility.
package mkdir

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/chmod"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the mkdir core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run creates the given directories, in the order given. With -p, missing
// parents are created too, with -m, new directories get the given mode, and
// with -v, a message is written to standard output for each directory created.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "mkdir", usage, optSet, args); done {
		return err
	}
	opts, dirs, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "mkdir", err)
	}
	if len(dirs) == 0 {
		stdopts.Errorf(p.Runtime, "mkdir", "missing operand")
		stdopts.TryHelp(p.Runtime, "mkdir")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "mkdir", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	var spec chmod.ModeSpec
	if opts.modeSet {
		if spec, err = chmod.ParseMode(opts.mode); err != nil {
			stdopts.Errorf(p.Runtime, "mkdir", "%v", err)
			stdopts.TryHelp(p.Runtime, "mkdir")
			return &coreutils.ExitError{Code: 1}
		}
	}

	ok := true
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := p.mkdir(dir, opts, spec); err != nil {
			stdopts.Errorf(p.Runtime, "mkdir", "%s: %v", dir, patherr.Unwrap(err))
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// mkdir creates a single directory and, with parents, any missing component
// above it. Existing directories are only allowed with parents. The -m mode
// only applies to the directory given itself, while parents get the default
// mode modified by u+wx. With verbose, every directory actually created is
// announced on the standard output.
func (p *Prog) mkdir(dir string, opts options, spec chmod.ModeSpec) error {
	comps := coreutils.SplitPath(p.FsName(dir))
	if len(comps) == 0 {
		comps = []string{"."}
	}
	disp := coreutils.SplitPath(path.Clean(filepath.ToSlash(dir)))

	umask := p.Umask.Perm()
	for i := range comps {
		name := coreutils.JoinPath(comps[:i+1])
		last := i == len(comps)-1

		fi, err := fs.Stat(p.FS, name)
		switch {
		case err == nil:
			if fi.IsDir() {
				if last && !opts.parents {
					return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrExist}
				}
				continue
			}
			if last {
				return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrExist}
			}
			return &fs.PathError{Op: "mkdir", Path: name, Err: errNotDir}
		case !errors.Is(err, fs.ErrNotExist):
			return err
		case !last && !opts.parents:
			return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrNotExist}
		}

		perm := (fs.FileMode(0o777) &^ umask) | 0o300
		if last {
			perm = 0o777 &^ umask
			if spec != nil {
				perm = spec(0o777, true, umask)
			}
		}
		if err := p.FS.Mkdir(name, perm); err != nil {
			return err
		}
		if opts.verbose {
			msg := name
			if j := i + len(disp) - len(comps); j >= 0 && j < len(disp) {
				msg = coreutils.JoinPath(disp[:j+1])
			}
			fmt.Fprintf(p.Stdout, "mkdir: created directory '%s'\n", msg)
		}
		if last && spec != nil {
			if err := p.FS.Chmod(name, perm); err != nil {
				return err
			}
		}
	}
	return nil
}

// errNotDir is returned when a path component names something other than a
// directory.
var errNotDir = errors.New("not a directory")

// options holds the -p and -v flags and the option-argument of the last -m
// option given.
type options struct {
	parents bool
	verbose bool
	modeSet bool
	mode    string
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--". The -m option takes an option-argument, either
// glued to the option or in the next argument, and the last one given is the
// one used.
// longOptions maps the long options of mkdir to the short ones they stand
// for.
var longOptions = map[string]rune{
	"mode":    'm',
	"parents": 'p',
	"verbose": 'v',
}

// optSet describes the options of mkdir.
var optSet = getopt.Set{Valued: "m", Long: longOptions}

// usage describes the options of mkdir.
const usage = `Usage: mkdir [OPTION]... DIRECTORY...
Create the DIRECTORY(ies), if they do not already exist.

  -m, --mode=MODE
         set the file mode bits of created directories to MODE
  -p, --parents
         make any missing parent directories for each argument
  -v, --verbose
         print a message for each created directory
` + stdopts.Tail

func parseFlags(args []string) (options, []string, error) {
	var opts options
	dirs, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'p':
			opts.parents = true
		case 'v':
			opts.verbose = true
		case 'm':
			opts.modeSet, opts.mode = true, value
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, err
	}
	return opts, dirs, nil
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
