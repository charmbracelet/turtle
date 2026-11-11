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
	"time"

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/filemode"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/fsx"
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
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "mv")
		return &coreutils.ExitError{Code: 2}
	}
	sources, target, err := splitOperands(operands, opts)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "mv")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if slices.Contains(operands, "") || (opts.targetDirSet && opts.targetDir == "") {
		p.errorf("invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}

	dst := p.FsPath(target)
	fi, err := fs.Stat(p.FS, dst)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		p.errorf("cannot stat '%s': %v", target, patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1}
	}
	targetIsDir := err == nil && fi.IsDir()
	slash := strings.HasSuffix(filepath.ToSlash(target), "/")
	if opts.targetDirSet && !targetIsDir {
		p.errorf("target '%s' is not a directory", target)
		return &coreutils.ExitError{Code: 1}
	}
	moveInto := targetIsDir && !opts.noTarget
	if !moveInto && (len(sources) > 1 || slash) {
		p.errorf("target '%s' is not a directory", target)
		return &coreutils.ExitError{Code: 1}
	}

	ok := true
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		e := pair{src: p.FsPath(source), srcName: source, dst: dst, dstName: target}
		if moveInto {
			base := coreutils.FsBase(source)
			e.dst = path.Join(dst, base)
			e.dstName = path.Join(target, base)
		}
		if err := p.moveEntry(ctx, e, opts); err != nil {
			switch {
			case errors.Is(err, errSkipped):
			case ctx.Err() != nil:
				return ctx.Err()
			default:
				p.errorf("%v", err)
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
func (p *Prog) moveEntry(ctx context.Context, e pair, opts options) error {
	fi, err := fs.Stat(p.FS, e.src)
	if err != nil {
		return fmt.Errorf("cannot stat '%s': %v", e.srcName, patherr.Unwrap(err))
	}
	if e.src == e.dst {
		return fmt.Errorf("'%s' and '%s' are the same file", e.srcName, e.dstName)
	}
	if fi.IsDir() && strings.HasPrefix(e.dst, e.src+"/") {
		return fmt.Errorf("cannot move a directory, '%s', into itself, '%s'", e.srcName, e.dstName)
	}

	dfi, err := fs.Stat(p.FS, e.dst)
	switch {
	case err == nil && dfi.IsDir() && !fi.IsDir():
		return fmt.Errorf("cannot overwrite directory '%s' with non-directory '%s'", e.dstName, e.srcName)
	case err == nil && !dfi.IsDir() && fi.IsDir():
		return fmt.Errorf("cannot overwrite non-directory '%s' with directory '%s'", e.dstName, e.srcName)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot stat '%s': %v", e.dstName, patherr.Unwrap(err))
	}
	if err == nil {
		switch {
		case opts.clobber == clobberSkip:
			return errSkipped
		case opts.update && !fi.IsDir() && !fi.ModTime().After(dfi.ModTime()):
			return errSkipped
		case opts.clobber == clobberPrompt && !p.confirm(e.dstName):
			return errSkipped
		}
	}

	err = p.FS.Rename(e.src, e.dst)
	switch {
	case err == nil:
	case crossDevice(err):
		if err := p.moveAcross(ctx, e, fi); err != nil {
			return err
		}
	default:
		return fmt.Errorf("cannot move '%s' to '%s': %v", e.srcName, e.dstName, unwrapRenameError(err))
	}
	if opts.verbose && p.Stdout != nil {
		fmt.Fprintf(p.Stdout, "renamed '%s' -> '%s'\n", e.srcName, e.dstName)
	}
	return nil
}

// moveAcross moves a file between file systems, where renaming doesn't work:
// the file, or hierarchy rooted at it, is duplicated with its mode and times
// over the destination, and the source is removed. An existing destination is
// replaced, except for a directory holding entries, which stays.
func (p *Prog) moveAcross(ctx context.Context, e pair, fi fs.FileInfo) error {
	dfi, err := fs.Stat(p.FS, e.dst)
	switch {
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot stat '%s': %v", e.dstName, patherr.Unwrap(err))
	case err == nil && dfi.IsDir():
		entries, err := fs.ReadDir(p.FS, e.dst)
		if err != nil {
			return fmt.Errorf("cannot read directory '%s': %v", e.dstName, patherr.Unwrap(err))
		}
		if len(entries) > 0 {
			return fmt.Errorf("cannot remove '%s': directory not empty", e.dstName)
		}
	}
	if err := p.copyEntry(ctx, e, fi); err != nil {
		return err
	}
	if err := p.removeTree(ctx, e.src); err != nil {
		return fmt.Errorf("cannot remove '%s': %v", e.srcName, patherr.Unwrap(err))
	}
	return nil
}

// copyEntry duplicates a file, or the hierarchy rooted at a directory, at the
// destination, with the mode and times of the source.
func (p *Prog) copyEntry(ctx context.Context, e pair, fi fs.FileInfo) error {
	if fi.IsDir() {
		return p.copyDir(ctx, e, fi)
	}
	return p.copyFile(ctx, e, fi)
}

// copyDir duplicates a directory hierarchy. The directory is created with
// traversal permissions, so its contents can be copied even from a
// restrictive source mode, and takes the source mode and times once it's
// done. An existing empty destination is kept and replaced.
func (p *Prog) copyDir(ctx context.Context, e pair, fi fs.FileInfo) error {
	if err := p.FS.Mkdir(e.dst, fi.Mode().Perm()|0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("cannot create directory '%s': %v", e.dstName, patherr.Unwrap(err))
	}
	entries, err := fs.ReadDir(p.FS, e.src)
	if err != nil {
		return fmt.Errorf("cannot read directory '%s': %v", e.srcName, patherr.Unwrap(err))
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		child := e.child(entry.Name())
		cfi, err := entry.Info()
		if err != nil {
			return fmt.Errorf("cannot stat '%s': %v", child.srcName, patherr.Unwrap(err))
		}
		if err := p.copyEntry(ctx, child, cfi); err != nil {
			return err
		}
	}
	if err := p.FS.Chmod(e.dst, fi.Mode()&filemode.AccessBits); err != nil {
		return fmt.Errorf("cannot set mode of '%s': %v", e.dstName, patherr.Unwrap(err))
	}
	if err := p.FS.Chtimes(e.dst, atime(fi), fi.ModTime()); err != nil {
		return fmt.Errorf("cannot set times of '%s': %v", e.dstName, patherr.Unwrap(err))
	}
	return nil
}

// copyFile duplicates a regular file with the mode and times of the source.
func (p *Prog) copyFile(ctx context.Context, e pair, fi fs.FileInfo) error {
	out, err := p.FS.OpenFile(e.dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return fmt.Errorf("cannot create '%s': %v", e.dstName, patherr.Unwrap(err))
	}
	in, err := p.FS.Open(e.src)
	if err != nil {
		out.Close()
		return fmt.Errorf("cannot open '%s': %v", e.srcName, patherr.Unwrap(err))
	}

	_, copyErr := io.Copy(out, ctxio.NewReader(ctx, in))
	inErr := in.Close()
	outErr := out.Close()
	switch {
	case copyErr != nil:
		return fmt.Errorf("cannot copy '%s' to '%s': %v", e.srcName, e.dstName, patherr.Unwrap(copyErr))
	case outErr != nil:
		return fmt.Errorf("cannot close '%s': %v", e.dstName, patherr.Unwrap(outErr))
	case inErr != nil:
		return fmt.Errorf("cannot close '%s': %v", e.srcName, patherr.Unwrap(inErr))
	}

	if err := p.FS.Chmod(e.dst, fi.Mode()&filemode.AccessBits); err != nil {
		return fmt.Errorf("cannot set mode of '%s': %v", e.dstName, patherr.Unwrap(err))
	}
	if err := p.FS.Chtimes(e.dst, atime(fi), fi.ModTime()); err != nil {
		return fmt.Errorf("cannot set times of '%s': %v", e.dstName, patherr.Unwrap(err))
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

// pair holds a source and its destination, both as the path used with the
// file system and as the name shown in diagnostics.
type pair struct {
	src, dst         string
	srcName, dstName string
}

// child returns the pair for an entry of the source directory.
func (e pair) child(name string) pair {
	return pair{
		src:     path.Join(e.src, name),
		dst:     path.Join(e.dst, name),
		srcName: e.srcName + "/" + name,
		dstName: e.dstName + "/" + name,
	}
}

// confirm asks on the standard error whether dst should be overwritten and
// reads the answer from the standard input. Only a response starting with
// 'y' or 'Y' is taken as affirmative.
func (p *Prog) confirm(dst string) bool {
	fmt.Fprintf(p.Stderr, "mv: overwrite '%s'? ", dst)
	if p.Stdin == nil {
		return false
	}
	var (
		buf    [1]byte
		answer byte
	)
	for {
		n, err := p.Stdin.Read(buf[:])
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			if answer == 0 {
				answer = buf[0]
			}
		}
		if err != nil {
			break
		}
	}
	return answer == 'y' || answer == 'Y'
}

// atime returns the access time of fi, falling back to its modification time.
func atime(fi fs.FileInfo) time.Time {
	if t, ok := fsx.Atime(fi); ok {
		return t
	}
	return fi.ModTime()
}

// errSkipped marks a move declined through -n, -u or -i, which isn't a
// failure.
var errSkipped = errors.New("skipped")

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "mv: "+format+"\n", args...)
}

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
      --help
         display this help and exit
      --version
         output version information and exit
`

// splitOperands splits the operands into the sources and the target,
// following the -t and -T options.
func splitOperands(operands []string, opts options) (sources []string, target string, err error) {
	if opts.targetDirSet {
		if len(operands) == 0 {
			return nil, "", errors.New("missing file operand")
		}
		return operands, opts.targetDir, nil
	}
	switch len(operands) {
	case 0:
		return nil, "", errors.New("missing file operand")
	case 1:
		return nil, "", fmt.Errorf("missing destination file operand after '%s'", operands[0])
	}
	if opts.noTarget {
		if len(operands) > 2 {
			return nil, "", fmt.Errorf("extra operand '%s'", operands[2])
		}
		return operands[:1], operands[1], nil
	}
	return operands[:len(operands)-1], operands[len(operands)-1], nil
}

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
