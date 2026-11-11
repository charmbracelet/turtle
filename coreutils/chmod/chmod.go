// Package chmod implements the POSIX chmod utility.
package chmod

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/filemode"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the chmod core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run changes the file mode bits of the given files, as specified by the mode
// operand, which is either an octal number or a symbolic mode expression, or
// by the mode of a reference file. With -R, directories have their whole
// hierarchy changed.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "chmod", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "chmod", err)
	}
	switch {
	case len(operands) == 0:
		stdopts.Errorf(p.Runtime, "chmod", "missing operand")
		stdopts.TryHelp(p.Runtime, "chmod")
		return &coreutils.ExitError{Code: 2}
	case p.FS == nil:
		stdopts.Errorf(p.Runtime, "chmod", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}

	spec, files, err := p.spec(opts, operands)
	if err != nil {
		return err
	}

	ok := true
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		failed, err := p.chmod(ctx, file, spec, opts)
		if err != nil {
			return err
		}
		ok = ok && !failed
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// spec builds the mode specification of a run, and returns the files it
// applies to. Without --reference, the first operand is the mode; with it,
// every operand is a file, and a mode operand is rejected as a usage error.
func (p *Prog) spec(opts options, operands []string) (ModeSpec, []string, error) {
	if opts.reference == nil {
		if len(operands) == 1 {
			stdopts.Errorf(p.Runtime, "chmod", "missing file operand")
			stdopts.TryHelp(p.Runtime, "chmod")
			return nil, nil, &coreutils.ExitError{Code: 2}
		}
		spec, err := ParseMode(operands[0])
		if err != nil {
			stdopts.Errorf(p.Runtime, "chmod", "%v", err)
			stdopts.TryHelp(p.Runtime, "chmod")
			return nil, nil, &coreutils.ExitError{Code: 1}
		}
		return spec, operands[1:], nil
	}
	// With --reference, every operand is a file.
	spec, err := p.referenceSpec(*opts.reference)
	if err != nil {
		stdopts.Errorf(p.Runtime, "chmod", "failed to get attributes of %s: %v", *opts.reference, patherr.Unwrap(err))
		return nil, nil, &coreutils.ExitError{Code: 1}
	}
	return spec, operands, nil
}

// referenceSpec returns a specification which uses the mode of the reference
// file. A symbolic link reference is followed.
func (p *Prog) referenceSpec(name string) (ModeSpec, error) {
	resolved := p.FsPath(name)
	fi, err := fs.Stat(p.FS, resolved)
	if err != nil {
		return nil, err
	}
	mode := fi.Mode() & filemode.AccessBits
	return func(fs.FileMode, bool, fs.FileMode) fs.FileMode { return mode }, nil
}

// chmod changes the mode of a single operand, reporting a diagnostic for every
// file it fails to change. It reports whether any file failed, and only
// returns an error when the run must be aborted.
func (p *Prog) chmod(ctx context.Context, name string, spec ModeSpec, opts options) (bool, error) {
	resolved := p.FsPath(name)
	umask := p.Umask.Perm()
	failed := false

	report := func(walked string, err error) {
		stdopts.Errorf(p.Runtime, "chmod", "%s: %v", display(name, resolved, walked), patherr.Unwrap(err))
		failed = true
	}

	if !opts.recursive {
		fi, err := fs.Stat(p.FS, resolved)
		if err == nil {
			err = p.apply(resolved, name, spec, umask, fi, opts.verbosity)
		}
		if err != nil {
			report(resolved, err)
		}
		return failed, nil
	}

	// The operand itself is followed, like -H does: a symbolic link to a
	// directory walks the tree it points at.
	root := p.resolveLink(resolved)

	err := fs.WalkDir(p.FS, root, func(walked string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			report(walked, err)
			if d == nil || d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// The symbolic links found below the operand are ignored.
		if walked != root && p.link(walked, d) {
			return nil
		}
		fi, err := d.Info()
		if err == nil {
			err = p.apply(walked, display(name, root, walked), spec, umask, fi, opts.verbosity)
		}
		if err != nil {
			report(walked, err)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return failed, nil
}

// lstatFS is the Lstat subset that the fsx file systems implement, which
// describes a name without following a final symbolic link.
type lstatFS interface {
	Lstat(name string) (fs.FileInfo, error)
}

// resolveLink follows the symbolic links at name, returning the path the
// last one points at. A name which is not a symbolic link, or a broken or
// looping chain, is returned as it is.
func (p *Prog) resolveLink(name string) string {
	lfs, ok := p.FS.(lstatFS)
	if !ok {
		return name
	}
	for range 40 {
		fi, err := lfs.Lstat(name)
		if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
			return name
		}
		target, err := p.FS.ReadLink(name)
		if err != nil {
			return name
		}
		name = path.Join(path.Dir(name), target)
	}
	return name
}

// link reports whether name is a symbolic link, without following it.
func (p *Prog) link(name string, d fs.DirEntry) bool {
	if fsys, ok := p.FS.(lstatFS); ok {
		if fi, err := fsys.Lstat(name); err == nil {
			return fi.Mode()&fs.ModeSymlink != 0
		}
	}
	return d.Type()&fs.ModeSymlink != 0
}

// apply changes the mode of the file at name, shown as given in diagnostics,
// and reports the change when the verbosity asks for it.
func (p *Prog) apply(name, shown string, spec ModeSpec, umask fs.FileMode, fi fs.FileInfo, verbosity int) error {
	old := fi.Mode() & filemode.AccessBits
	mode := spec(fi.Mode(), fi.IsDir(), umask)
	if err := p.FS.Chmod(name, mode); err != nil {
		return err
	}
	p.describe(shown, old, mode&filemode.AccessBits, verbosity)
	return nil
}

// describe reports how the mode of a file changed, or was retained.
func (p *Prog) describe(name string, old, mode fs.FileMode, verbosity int) {
	if verbosity == quiet || (verbosity == changesOnly && old == mode) {
		return
	}
	if old != mode {
		fmt.Fprintf(p.Stdout, "mode of '%s' changed from %04o (%s) to %04o (%s)\n",
			name, filemode.Octal(old), modeString(old), filemode.Octal(mode), modeString(mode))
		return
	}
	fmt.Fprintf(p.Stdout, "mode of '%s' retained as %04o (%s)\n",
		name, filemode.Octal(mode), modeString(mode))
}

// display returns the path to show in diagnostics: the operand as given, with
// any sub-path walked below it appended.
func display(operand, resolved, walked string) string {
	if walked == resolved {
		return operand
	}
	if rel, ok := strings.CutPrefix(walked, resolved+"/"); ok {
		return path.Join(operand, rel)
	}
	return path.Join(operand, walked)
}

// The verbosity levels of a run, selected by -c and -v.
const (
	quiet = iota
	changesOnly
	noisy
)

// longOptions maps the long options of chmod to the characters they stand
// for.
var longOptions = map[string]rune{
	"recursive": 'R',
	"changes":   'c',
	"verbose":   'v',
	"reference": optReference,
}

// optSet describes the options of chmod.
var optSet = getopt.Set{Valued: string(optReference), Long: longOptions}

// usage describes the options of chmod.
const usage = `Usage: chmod [OPTION]... {MODE | --reference=REF_FILE} FILE...
Change the access permissions of each FILE.

  -c, --changes
         verbosely describe the action for each file whose permissions actually
         change
  -R, --recursive
         recursively change permissions of directories and their contents
      --reference=REF_FILE
         use the mode of REF_FILE instead of a MODE operand
  -v, --verbose
         verbosely describe the action or non-action taken for every file
` + stdopts.Tail

// optReference is a rune which no short option can reach, so --reference
// stays long-only.
const optReference rune = 0x100 + iota

// options holds what a run of chmod selects.
type options struct {
	recursive bool
	// verbosity is one of quiet, changesOnly and noisy; the last of -c and
	// -v given wins.
	verbosity int
	// reference names the file whose mode is used instead of a mode operand,
	// or is nil without --reference.
	reference *string
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'R':
			opts.recursive = true
		case 'c':
			opts.verbosity = changesOnly
		case 'v':
			opts.verbosity = noisy
		case optReference:
			opts.reference = &value
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
