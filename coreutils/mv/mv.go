// Package mv implements the POSIX mv utility.
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

// Run moves each source file to the destination named by the last operand: an
// existing directory to move into, or the new name of the single source.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	switch len(operands) {
	case 0:
		p.errorf("missing file operand")
		return &coreutils.ExitError{Code: 2}
	case 1:
		p.errorf("missing destination file operand after '%s'", operands[0])
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if slices.Contains(operands, "") {
		p.errorf("invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}

	sources, target := operands[:len(operands)-1], operands[len(operands)-1]

	dst := p.resolve(target)
	fi, err := fs.Stat(p.FS, dst)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		p.errorf("cannot stat '%s': %v", target, unwrapPathError(err))
		return &coreutils.ExitError{Code: 1}
	}
	targetIsDir := err == nil && fi.IsDir()
	slash := strings.HasSuffix(filepath.ToSlash(target), "/")
	if !targetIsDir && (len(sources) > 1 || slash) {
		p.errorf("target '%s' is not a directory", target)
		return &coreutils.ExitError{Code: 1}
	}

	ok := true
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		e := pair{src: p.resolve(source), srcName: source, dst: dst, dstName: target}
		if targetIsDir {
			base := baseName(source)
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
// through -i.
func (p *Prog) moveEntry(ctx context.Context, e pair, opts options) error {
	fi, err := fs.Stat(p.FS, e.src)
	if err != nil {
		return fmt.Errorf("cannot stat '%s': %v", e.srcName, unwrapPathError(err))
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
		return fmt.Errorf("cannot stat '%s': %v", e.dstName, unwrapPathError(err))
	}
	if err == nil && opts.interactive && !p.confirm(e.dstName) {
		return errSkipped
	}

	err = p.FS.Rename(e.src, e.dst)
	switch {
	case err == nil:
		return nil
	case crossDevice(err):
		return p.moveAcross(ctx, e, fi)
	default:
		return fmt.Errorf("cannot move '%s' to '%s': %v", e.srcName, e.dstName, unwrapRenameError(err))
	}
}

// moveAcross moves a file between file systems, where renaming doesn't work:
// the file, or hierarchy rooted at it, is duplicated with its mode and times
// over the destination, and the source is removed. An existing destination is
// replaced, except for a directory holding entries, which stays.
func (p *Prog) moveAcross(ctx context.Context, e pair, fi fs.FileInfo) error {
	dfi, err := fs.Stat(p.FS, e.dst)
	switch {
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot stat '%s': %v", e.dstName, unwrapPathError(err))
	case err == nil && dfi.IsDir():
		entries, err := fs.ReadDir(p.FS, e.dst)
		if err != nil {
			return fmt.Errorf("cannot read directory '%s': %v", e.dstName, unwrapPathError(err))
		}
		if len(entries) > 0 {
			return fmt.Errorf("cannot remove '%s': directory not empty", e.dstName)
		}
	}
	if err := p.copyEntry(ctx, e, fi); err != nil {
		return err
	}
	if err := p.removeTree(ctx, e.src); err != nil {
		return fmt.Errorf("cannot remove '%s': %v", e.srcName, unwrapPathError(err))
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
		return fmt.Errorf("cannot create directory '%s': %v", e.dstName, unwrapPathError(err))
	}
	entries, err := fs.ReadDir(p.FS, e.src)
	if err != nil {
		return fmt.Errorf("cannot read directory '%s': %v", e.srcName, unwrapPathError(err))
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		child := e.child(entry.Name())
		cfi, err := entry.Info()
		if err != nil {
			return fmt.Errorf("cannot stat '%s': %v", child.srcName, unwrapPathError(err))
		}
		if err := p.copyEntry(ctx, child, cfi); err != nil {
			return err
		}
	}
	if err := p.FS.Chmod(e.dst, fi.Mode()&chmodBits); err != nil {
		return fmt.Errorf("cannot set mode of '%s': %v", e.dstName, unwrapPathError(err))
	}
	if err := p.FS.Chtimes(e.dst, atime(fi), fi.ModTime()); err != nil {
		return fmt.Errorf("cannot set times of '%s': %v", e.dstName, unwrapPathError(err))
	}
	return nil
}

// copyFile duplicates a regular file with the mode and times of the source.
func (p *Prog) copyFile(ctx context.Context, e pair, fi fs.FileInfo) error {
	out, err := p.FS.OpenFile(e.dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return fmt.Errorf("cannot create '%s': %v", e.dstName, unwrapPathError(err))
	}
	in, err := p.FS.Open(e.src)
	if err != nil {
		out.Close()
		return fmt.Errorf("cannot open '%s': %v", e.srcName, unwrapPathError(err))
	}

	_, copyErr := io.Copy(out, ctxReader{ctx: ctx, r: in})
	inErr := in.Close()
	outErr := out.Close()
	switch {
	case copyErr != nil:
		return fmt.Errorf("cannot copy '%s' to '%s': %v", e.srcName, e.dstName, unwrapPathError(copyErr))
	case outErr != nil:
		return fmt.Errorf("cannot close '%s': %v", e.dstName, unwrapPathError(outErr))
	case inErr != nil:
		return fmt.Errorf("cannot close '%s': %v", e.srcName, unwrapPathError(inErr))
	}

	if err := p.FS.Chmod(e.dst, fi.Mode()&chmodBits); err != nil {
		return fmt.Errorf("cannot set mode of '%s': %v", e.dstName, unwrapPathError(err))
	}
	if err := p.FS.Chtimes(e.dst, atime(fi), fi.ModTime()); err != nil {
		return fmt.Errorf("cannot set times of '%s': %v", e.dstName, unwrapPathError(err))
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

// resolve converts an operand into the slash-separated path used with the
// file system, relative to the working directory.
func (p *Prog) resolve(name string) string {
	return path.Clean(filepath.ToSlash(p.ResolvePath(name)))
}

// baseName returns the last component of an operand as typed, so a trailing
// slash is ignored.
func baseName(name string) string {
	return path.Base(filepath.ToSlash(name))
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

// chmodBits are the mode bits that a duplicated entry keeps.
const chmodBits = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// errSkipped marks a move declined through -i, which isn't a failure.
var errSkipped = errors.New("skipped")

// ctxReader stops read operations with the context error once ctx is done.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

// Read implements io.Reader.
func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "mv: "+format+"\n", args...)
}

// unwrapPathError unwraps *fs.PathError so diagnostics don't repeat the file
// name.
func unwrapPathError(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}

// unwrapRenameError unwraps the errors of a rename, which *os.LinkError wraps
// with both names instead.
func unwrapRenameError(err error) error {
	if linkErr, ok := errors.AsType[*os.LinkError](err); ok {
		return linkErr.Err
	}
	return unwrapPathError(err)
}

// crossDevice reports whether err is a rename failing because the source and
// the destination are on different file systems, in which case mv duplicates
// the file and removes the source instead.
func crossDevice(err error) bool {
	return errors.Is(unwrapRenameError(err), exdev)
}

// options holds the -i flag; -f only cancels an earlier -i.
type options struct {
	interactive bool
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--", and the last of -f and -i wins.
func parseFlags(args []string) (options, []string, error) {
	var (
		opts     options
		operands []string
		parsing  = true
	)
	for i := range args {
		arg := args[i]
		switch {
		case !parsing || arg == "-" || !strings.HasPrefix(arg, "-"):
			parsing = false
			operands = append(operands, arg)
		case arg == "--":
			parsing = false
		case strings.HasPrefix(arg, "--"):
			return options{}, nil, fmt.Errorf("unrecognized option '%s'", arg)
		default:
			for j := 1; j < len(arg); j++ {
				switch c := arg[j]; c {
				case 'f':
					opts.interactive = false
				case 'i':
					opts.interactive = true
				default:
					return options{}, nil, fmt.Errorf("invalid option -- '%c'", c)
				}
			}
		}
	}
	return opts, operands, nil
}
