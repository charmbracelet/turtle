// Package rmdir implements the POSIX rmdir utility.
package rmdir

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"syscall"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/fsx"
)

// Prog is the rmdir core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run removes the given directories, in the order given. With -p, the
// directories above each operand are removed too, as long as they become
// empty. Non-empty failures are silently ignored when asked to.
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

	ok := true
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		failed, err := p.rmdir(dir, opts)
		if err != nil {
			p.errorf("%s: %v", failed, patherr.Unwrap(err))
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// rmdir removes a single directory and, with parents, the directories above
// it, stopping at the first error. A failure of a non-empty directory is
// ignored when asked to, as if the directory had been removed. It returns the
// path of the directory that failed to be removed, if any.
func (p *Prog) rmdir(dir string, opts options) (string, error) {
	comps := coreutils.SplitPath(dir)
	if len(comps) == 0 {
		comps = []string{"."}
	}
	for i := len(comps); i > 0; i-- {
		name := coreutils.JoinPath(comps[:i])
		display := name
		if i == len(comps) {
			display = dir
		}
		if opts.verbose {
			fmt.Fprintf(p.Stdout, "rmdir: removing directory, '%s'\n", display)
		}
		resolved := filepath.ToSlash(p.ResolvePath(name))
		if err := p.remove(resolved); err != nil {
			if opts.ignoreFailOnNonEmpty && p.nonEmpty(resolved, err) {
				return "", nil
			}
			return display, err
		}
		if !opts.parents {
			break
		}
	}
	return "", nil
}

// remove removes a single directory, which must be empty.
func (p *Prog) remove(name string) error {
	fi, err := fs.Stat(p.FS, name)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return &fs.PathError{Op: "rmdir", Path: name, Err: errNotDir}
	}
	return p.FS.Remove(name)
}

// errNotDir is returned when an operand names something other than a
// directory.
var errNotDir = errors.New("not a directory")

// nonEmpty reports whether err comes from removing a directory which holds
// entries. The errno check covers the common platforms, and reading the
// directory covers the rest, including failures which may or may not be
// caused by a non-empty directory.
func (p *Prog) nonEmpty(name string, err error) bool {
	if errors.Is(err, fsx.ErrNotEmpty) ||
		errors.Is(err, syscall.ENOTEMPTY) ||
		errors.Is(err, syscall.EEXIST) {
		return true
	}
	entries, readErr := fs.ReadDir(p.FS, name)
	return readErr == nil && len(entries) > 0
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "rmdir: "+format+"\n", args...)
}

// optIgnoreFail is the pseudo option which --ignore-fail-on-non-empty
// stands for, having no short form.
const optIgnoreFail = 0x100

// options holds the -p, --ignore-fail-on-non-empty and -v flags.
type options struct {
	parents              bool
	ignoreFailOnNonEmpty bool
	verbose              bool
}

// longOptions maps the long options of rmdir to the short ones they stand
// for.
var longOptions = map[string]rune{
	"ignore-fail-on-non-empty": optIgnoreFail,
	"parents":                  'p',
	"verbose":                  'v',
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	dirs, err := getopt.Set{Long: longOptions}.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'p':
			opts.parents = true
		case 'v':
			opts.verbose = true
		case optIgnoreFail:
			opts.ignoreFailOnNonEmpty = true
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
