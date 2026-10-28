// Package mkdir implements the POSIX mkdir utility.
package mkdir

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/chmod"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
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
// parents are created too, and with -m, new directories get the given mode.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, dirs, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if len(dirs) == 0 {
		p.errorf("missing operand")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	var spec chmod.ModeSpec
	if opts.modeSet {
		if spec, err = chmod.ParseMode(opts.mode); err != nil {
			p.errorf("%v", err)
			return &coreutils.ExitError{Code: 1}
		}
	}

	ok := true
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := p.mkdir(dir, opts.parents, spec); err != nil {
			p.errorf("%s: %v", dir, patherr.Unwrap(err))
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
// mode modified by u+wx.
func (p *Prog) mkdir(dir string, parents bool, spec chmod.ModeSpec) error {
	comps := coreutils.SplitPath(filepath.ToSlash(p.ResolvePath(dir)))
	if len(comps) == 0 {
		comps = []string{"."}
	}

	umask := p.Umask.Perm()
	for i := range comps {
		name := coreutils.JoinPath(comps[:i+1])
		last := i == len(comps)-1

		fi, err := fs.Stat(p.FS, name)
		switch {
		case err == nil:
			if fi.IsDir() {
				if last && !parents {
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
		case !last && !parents:
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

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "mkdir: "+format+"\n", args...)
}

// options holds the -p flag and the option-argument of the last -m option
// given.
type options struct {
	parents bool
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
}

func parseFlags(args []string) (options, []string, error) {
	var opts options
	dirs, err := getopt.Set{Valued: "m", Long: longOptions}.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'p':
			opts.parents = true
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
