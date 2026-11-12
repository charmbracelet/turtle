// Package rm implements the POSIX rm utility.
//
// Files are removed with the file system's Remove operation, and with -r or
// -R the whole hierarchy rooted at an operand is removed, reporting each
// entry that can't be removed while the remaining ones are still processed.
//
// A removal is only confirmed with -i: detecting that the standard input is
// a terminal isn't supported, so write-protected files are removed without
// prompting. Symbolic links aren't supported by the file systems used here
// either, so an operand naming one is handled as its target.
package rm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/fsop"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the rm core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run removes the given files, in the order given. With -r or -R the whole
// hierarchy rooted at each operand is removed, with -d empty directories are
// removed like rmdir would, and with -i each removal is confirmed first. An
// operand ending in dot or dot-dot, or naming a file system root, is always
// refused.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "rm", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "rm", err)
	}
	if len(operands) == 0 {
		if opts.force {
			return nil
		}
		stdopts.Errorf(p.Runtime, "rm", "missing operand")
		stdopts.TryHelp(p.Runtime, "rm")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "rm", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}

	ok := true
	for _, operand := range operands {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := p.removeOperand(ctx, operand, opts)
		switch {
		case err == nil, errors.Is(err, errSkipped):
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, errFailed):
			ok = false
		default:
			stdopts.Errorf(p.Runtime, "rm", "%v", err)
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// removeOperand removes a single operand. Errors are returned ready to be
// reported; errSkipped is returned when a removal was declined through -i,
// and errFailed when the failure, somewhere inside a hierarchy, was already
// reported.
func (p *Prog) removeOperand(ctx context.Context, operand string, opts options) error {
	if operand == "" {
		return errors.New("invalid zero-length file name")
	}
	if err := protectedErr(operand); err != nil {
		return fmt.Errorf("%s: %v", operand, err)
	}
	return p.removeEntry(ctx, p.FsPath(operand), operand, opts)
}

// removeEntry removes the entry named by name, which display shows in
// diagnostics and prompts. A directory is descended into with -r or -R, and
// only an empty one is removed with -d alone.
func (p *Prog) removeEntry(ctx context.Context, name, display string, opts options) error {
	fi, err := fs.Stat(p.FS, name)
	if err != nil {
		if opts.force && errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("%s: %v", display, patherr.Unwrap(err))
	}
	if fi.IsDir() {
		switch {
		case opts.recursive:
			return p.removeTree(ctx, name, display, opts)
		case !opts.empty:
			return fmt.Errorf("%s: %v", display, errIsDir)
		}
	}
	if opts.interactive && !fsop.Confirm(p.Runtime, "rm", "remove", display) {
		return errSkipped
	}
	return p.removeOne(name, display, opts)
}

// removeTree removes the hierarchy rooted at a directory, deepest entries
// first. Each entry that fails is reported while the remaining ones are
// still removed, but the directory itself is left behind when that happens.
func (p *Prog) removeTree(ctx context.Context, name, display string, opts options) error {
	entries, err := fs.ReadDir(p.FS, name)
	if err != nil {
		return fmt.Errorf("%s: %v", display, patherr.Unwrap(err))
	}
	if len(entries) > 0 && opts.interactive && !fsop.Confirm(p.Runtime, "rm", "descend into directory", display) {
		return errSkipped
	}

	ok := true
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		child := entry.Name()
		err := p.removeEntry(ctx, path.Join(name, child), path.Join(display, child), opts)
		switch {
		case err == nil, errors.Is(err, errSkipped):
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, errFailed):
			ok = false
		default:
			stdopts.Errorf(p.Runtime, "rm", "%v", err)
			ok = false
		}
	}
	if !ok {
		return errFailed
	}
	if opts.interactive && !fsop.Confirm(p.Runtime, "rm", "remove", display) {
		return errSkipped
	}
	return p.removeOne(name, display, opts)
}

// removeOne removes a single entry, which must be a file or an empty
// directory. An entry that vanished on its own, as an emptied implicit
// directory of a memory file system does, is taken as removed.
func (p *Prog) removeOne(name, display string, opts options) error {
	err := p.FS.Remove(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s: %v", display, patherr.Unwrap(err))
	}
	if opts.verbose {
		fmt.Fprintf(p.Stdout, "removed '%s'\n", display)
	}
	return nil
}

// protectedErr returns the refusal to remove an operand ending in dot or
// dot-dot, or naming a file system root.
func protectedErr(operand string) error {
	switch path.Base(filepath.ToSlash(operand)) {
	case ".", "..":
		return errDotDot
	case "/":
		return errRoot
	}
	if clean := filepath.Clean(operand); filepath.Dir(clean) == clean {
		return errRoot
	}
	return nil
}

// errDotDot and errRoot mark operands that rm refuses to remove.
var (
	errDotDot = errors.New("refusing to remove '.' or '..' directory")
	errRoot   = errors.New("refusing to remove root directory")
)

// errIsDir is returned when a directory operand is given without -r, -R or
// -d.
var errIsDir = errors.New("is a directory")

// errSkipped marks a removal declined through -i, which isn't a failure.
var errSkipped = errors.New("skipped")

// errFailed marks a hierarchy whose failure was already reported.
var errFailed = errors.New("failed")

// options holds the -d, -f, -i, -r, -R and -v flags.
type options struct {
	empty       bool
	force       bool
	interactive bool
	recursive   bool
	verbose     bool
}

// longOptions maps the long options of rm to the short ones they stand for.
var longOptions = map[string]rune{
	"dir":         'd',
	"force":       'f',
	"interactive": 'i',
	"recursive":   'r',
	"verbose":     'v',
}

// optSet describes the options of rm.
var optSet = getopt.Set{Long: longOptions}

// usage describes the options of rm.
const usage = `Usage: rm [OPTION]... [FILE]...
Remove (unlink) the FILE(s).

  -f, --force
         ignore nonexistent files and arguments, never prompt
  -i, --interactive
         prompt before every removal
  -r, -R, --recursive
         remove directories and their contents recursively
  -d, --dir
         remove empty directories
  -v, --verbose
         explain what is being done
` + stdopts.Tail

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--", and the last of -f and -i wins.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'd':
			opts.empty = true
		case 'f':
			opts.force, opts.interactive = true, false
		case 'i':
			opts.force, opts.interactive = false, true
		case 'r', 'R':
			opts.recursive = true
		case 'v':
			opts.verbose = true
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

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
