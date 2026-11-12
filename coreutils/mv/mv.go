// Package mv implements the POSIX mv utility, plus the GNU -n, -u, -v, -t
// and -T options. Only the plain -u form is supported; the --update=WHEN
// form of GNU 9.3+ is out of scope.
//
// Files are moved by renaming them, falling back to duplicating and removing
// the source when the rename fails because the two names are on different
// file systems.
//
// A destination is only confirmed with -i: detecting that the standard input
// is a terminal isn't supported, so an unwritable destination is reported
// through the error of the failed move instead of a prompt.
package mv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/filemode"
	"charm.land/snail/coreutils/internal/fsop"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the mv core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run moves each source file to the destination named by the last operand:
// an existing directory to move into, or the new name of the single source.
// The -t option names the destination directory instead, and -T makes the
// last operand a plain destination name.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "mv", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "mv", err)
	}
	sources, target, err := fsop.SplitOperands(operands, opts.targetDir, opts.targetDirSet, opts.noTarget)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "mv", err)
	}
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "mv", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if slices.Contains(operands, "") || (opts.targetDirSet && opts.targetDir == "") {
		stdopts.Errorf(p.Runtime, "mv", "invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}

	dst := p.FsPath(target)
	fi, err := fs.Stat(p.FS, dst)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		stdopts.Errorf(p.Runtime, "mv", "cannot stat '%s': %v", target, patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1}
	}
	targetIsDir := err == nil && fi.IsDir()
	slash := strings.HasSuffix(filepath.ToSlash(target), "/")
	if opts.targetDirSet && !targetIsDir {
		stdopts.Errorf(p.Runtime, "mv", "target '%s' is not a directory", target)
		return &coreutils.ExitError{Code: 1}
	}
	moveInto := targetIsDir && !opts.noTarget
	if !moveInto && (len(sources) > 1 || slash) {
		stdopts.Errorf(p.Runtime, "mv", "target '%s' is not a directory", target)
		return &coreutils.ExitError{Code: 1}
	}

	ok := true
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		e := fsop.Pair{Src: p.FsPath(source), SrcName: source, Dst: dst, DstName: target}
		if moveInto {
			base := coreutils.FsBase(source)
			e.Dst = path.Join(dst, base)
			e.DstName = path.Join(target, base)
		}
		if err := p.moveEntry(ctx, e, opts); err != nil {
			switch {
			case errors.Is(err, errSkipped):
			case ctx.Err() != nil:
				return ctx.Err()
			default:
				stdopts.Errorf(p.Runtime, "mv", "%v", err)
				ok = false
			}
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// moveEntry moves a single source to its destination. Errors are returned
// ready to be reported; errSkipped is returned when a move was declined
// through -n, -u or -i.
func (p *Prog) moveEntry(ctx context.Context, e fsop.Pair, opts options) error {
	fi, err := fs.Stat(p.FS, e.Src)
	if err != nil {
		return fmt.Errorf("cannot stat '%s': %v", e.SrcName, patherr.Unwrap(err))
	}
	if e.Src == e.Dst {
		return fmt.Errorf("'%s' and '%s' are the same file", e.SrcName, e.DstName)
	}
	if fi.IsDir() && strings.HasPrefix(e.Dst, e.Src+"/") {
		return fmt.Errorf("cannot move a directory, '%s', into itself, '%s'", e.SrcName, e.DstName)
	}

	dfi, err := fs.Stat(p.FS, e.Dst)
	switch {
	case err == nil && dfi.IsDir() && !fi.IsDir():
		return fmt.Errorf("cannot overwrite directory '%s' with non-directory '%s'", e.DstName, e.SrcName)
	case err == nil && !dfi.IsDir() && fi.IsDir():
		return fmt.Errorf("cannot overwrite non-directory '%s' with directory '%s'", e.DstName, e.SrcName)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot stat '%s': %v", e.DstName, patherr.Unwrap(err))
	}
	if err == nil {
		switch {
		case opts.clobber == clobberSkip:
			return errSkipped
		case opts.update && !fi.IsDir() && !fi.ModTime().After(dfi.ModTime()):
			return errSkipped
		case opts.clobber == clobberPrompt && !fsop.Confirm(p.Runtime, "mv", "overwrite", e.DstName):
			return errSkipped
		}
	}

	err = p.FS.Rename(e.Src, e.Dst)
	switch {
	case err == nil:
	case crossDevice(err):
		if err := p.moveAcross(ctx, e, fi); err != nil {
			return err
		}
	default:
		return fmt.Errorf("cannot move '%s' to '%s': %v", e.SrcName, e.DstName, unwrapRenameError(err))
	}
	if opts.verbose && p.Stdout != nil {
		fmt.Fprintf(p.Stdout, "renamed '%s' -> '%s'\n", e.SrcName, e.DstName)
	}
	return nil
}

// moveAcross moves a file between file systems, where renaming doesn't work:
// the file, or hierarchy rooted at it, is duplicated with its mode and times
// over the destination, and the source is removed. An existing destination is
// replaced, except for a directory holding entries, which stays.
func (p *Prog) moveAcross(ctx context.Context, e fsop.Pair, fi fs.FileInfo) error {
	dfi, err := fs.Stat(p.FS, e.Dst)
	switch {
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot stat '%s': %v", e.DstName, patherr.Unwrap(err))
	case err == nil && dfi.IsDir():
		entries, err := fs.ReadDir(p.FS, e.Dst)
		if err != nil {
			return fmt.Errorf("cannot read directory '%s': %v", e.DstName, patherr.Unwrap(err))
		}
		if len(entries) > 0 {
			return fmt.Errorf("cannot remove '%s': directory not empty", e.DstName)
		}
	}
	if err := p.copyEntry(ctx, e, fi); err != nil {
		return err
	}
	if err := p.removeTree(ctx, e.Src); err != nil {
		return fmt.Errorf("cannot remove '%s': %v", e.SrcName, patherr.Unwrap(err))
	}
	return nil
}

// copyEntry duplicates a file, or the hierarchy rooted at a directory, at the
// destination, with the mode and times of the source.
func (p *Prog) copyEntry(ctx context.Context, e fsop.Pair, fi fs.FileInfo) error {
	if fi.IsDir() {
		return p.copyDir(ctx, e, fi)
	}
	return p.copyFile(ctx, e, fi)
}

// copyDir duplicates a directory hierarchy. The directory is created with
// traversal permissions, so its contents can be copied even from a
// restrictive source mode, and takes the source mode and times once it's
// done. An existing empty destination is kept and replaced.
func (p *Prog) copyDir(ctx context.Context, e fsop.Pair, fi fs.FileInfo) error {
	if err := p.FS.Mkdir(e.Dst, fi.Mode().Perm()|0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("cannot create directory '%s': %v", e.DstName, patherr.Unwrap(err))
	}
	entries, err := fs.ReadDir(p.FS, e.Src)
	if err != nil {
		return fmt.Errorf("cannot read directory '%s': %v", e.SrcName, patherr.Unwrap(err))
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		child := e.Child(entry.Name())
		cfi, err := entry.Info()
		if err != nil {
			return fmt.Errorf("cannot stat '%s': %v", child.SrcName, patherr.Unwrap(err))
		}
		if err := p.copyEntry(ctx, child, cfi); err != nil {
			return err
		}
	}
	if err := p.FS.Chmod(e.Dst, fi.Mode()&filemode.AccessBits); err != nil {
		return fmt.Errorf("cannot set mode of '%s': %v", e.DstName, patherr.Unwrap(err))
	}
	if err := p.FS.Chtimes(e.Dst, fsop.Atime(fi), fi.ModTime()); err != nil {
		return fmt.Errorf("cannot set times of '%s': %v", e.DstName, patherr.Unwrap(err))
	}
	return nil
}

// copyFile duplicates a regular file with the mode and times of the source.
func (p *Prog) copyFile(ctx context.Context, e fsop.Pair, fi fs.FileInfo) error {
	out, err := p.FS.OpenFile(e.Dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return fmt.Errorf("cannot create '%s': %v", e.DstName, patherr.Unwrap(err))
	}
	in, err := p.FS.Open(e.Src)
	if err != nil {
		out.Close()
		return fmt.Errorf("cannot open '%s': %v", e.SrcName, patherr.Unwrap(err))
	}

	_, copyErr := io.Copy(out, ctxio.NewReader(ctx, in))
	inErr := in.Close()
	outErr := out.Close()
	switch {
	case copyErr != nil:
		return fmt.Errorf("cannot copy '%s' to '%s': %v", e.SrcName, e.DstName, patherr.Unwrap(copyErr))
	case outErr != nil:
		return fmt.Errorf("cannot close '%s': %v", e.DstName, patherr.Unwrap(outErr))
	case inErr != nil:
		return fmt.Errorf("cannot close '%s': %v", e.SrcName, patherr.Unwrap(inErr))
	}

	if err := p.FS.Chmod(e.Dst, fi.Mode()&filemode.AccessBits); err != nil {
		return fmt.Errorf("cannot set mode of '%s': %v", e.DstName, patherr.Unwrap(err))
	}
	if err := p.FS.Chtimes(e.Dst, fsop.Atime(fi), fi.ModTime()); err != nil {
		return fmt.Errorf("cannot set times of '%s': %v", e.DstName, patherr.Unwrap(err))
	}
	return nil
}

// removeTree removes the file or hierarchy rooted at name, deepest entries
// first. An entry that vanished on its own, as an emptied implicit directory
// does, is taken as removed.
func (p *Prog) removeTree(ctx context.Context, name string) error {
	if fi, err := fs.Stat(p.FS, name); err == nil && fi.IsDir() {
		entries, err := fs.ReadDir(p.FS, name)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := p.removeTree(ctx, path.Join(name, entry.Name())); err != nil {
				return err
			}
		}
	}
	if err := p.FS.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// errSkipped marks a move declined through -n, -u or -i, which isn't a
// failure.
var errSkipped = errors.New("skipped")

// unwrapRenameError unwraps the errors of a rename, which *os.LinkError wraps
// with both names instead.
func unwrapRenameError(err error) error {
	if linkErr, ok := errors.AsType[*os.LinkError](err); ok {
		return linkErr.Err
	}
	return patherr.Unwrap(err)
}

// crossDevice reports whether err is a rename failing because the source and
// the destination are on different file systems, in which case mv duplicates
// the file and removes the source instead.
func crossDevice(err error) bool {
	return errors.Is(unwrapRenameError(err), exdev)
}

// options holds the flags of mv. For -f, -i and -n, only the last one given
// takes effect, and -u is ignored when -n is in effect.
type options struct {
	clobber      clobber
	update       bool
	verbose      bool
	targetDir    string
	targetDirSet bool
	noTarget     bool
}

// clobber is the policy for an existing destination, set by the last of
// -f, -i and -n.
type clobber int

const (
	clobberOverwrite clobber = iota // -f, and the default
	clobberPrompt                   // -i
	clobberSkip                     // -n
)

// longOptions maps the long options of mv to the short ones they stand for.
var longOptions = map[string]rune{
	"force":               'f',
	"interactive":         'i',
	"no-clobber":          'n',
	"no-target-directory": 'T',
	"target-directory":    't',
	"update":              'u',
	"verbose":             'v',
}

// optSet describes the options of mv.
var optSet = getopt.Set{Long: longOptions, Valued: "t"}

// usage describes the options of mv.
const usage = `Usage: mv [OPTION]... [-T] SOURCE DEST
  or:  mv [OPTION]... SOURCE... DIRECTORY
  or:  mv [OPTION]... -t DIRECTORY SOURCE...
Rename SOURCE to DEST, or move SOURCE(s) to DIRECTORY.

  -f, --force
         do not prompt before overwriting
  -i, --interactive
         prompt before overwrite
  -n, --no-clobber
         do not overwrite an existing file
  -u, --update
         move only when the SOURCE file is newer than the destination file or
         when the destination file is missing
  -v, --verbose
         explain what is being done
  -T, --no-target-directory
         treat DEST as a normal file
  -t, --target-directory=DIRECTORY
         move all SOURCE arguments into DIRECTORY
` + stdopts.Tail

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--", the last of -f, -i and -n wins, and -t can't be
// combined with -T.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'f':
			opts.clobber = clobberOverwrite
		case 'i':
			opts.clobber = clobberPrompt
		case 'n':
			opts.clobber = clobberSkip
		case 'u':
			opts.update = true
		case 'v':
			opts.verbose = true
		case 't':
			opts.targetDir, opts.targetDirSet = value, true
		case 'T':
			opts.noTarget = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	switch {
	case err != nil:
		return options{}, nil, err
	case opts.targetDirSet && opts.noTarget:
		return options{}, nil, errors.New("cannot combine --target-directory (-t) and --no-target-directory (-T)")
	}
	return opts, operands, nil
}
