// Package cp implements the POSIX cp utility.
//
// It copies files, and with -R whole file hierarchies. The -H, -L and -P
// options are accepted, but have no effect: symbolic links aren't handled
// specially, so a link is copied as the file it points to.
package cp

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
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/fsx"
)

// Prog is the cp core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run copies each source file, or with -R the hierarchy rooted at it, to the
// destination named by the last operand: a directory to copy into, or the
// file to copy to when there's exactly one source.
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
	if !targetIsDir && (len(sources) > 1 || (slash && !opts.recursive)) {
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
		err := p.copyEntry(ctx, e, opts)
		switch {
		case err == nil, errors.Is(err, errSkipped):
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, errFailed):
			ok = false
		default:
			p.errorf("%v", err)
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// copyEntry copies a single file, or with -R the hierarchy rooted at it.
// Errors are returned ready to be reported; errSkipped is returned when a
// copy was declined through -i, and errFailed when the failure, somewhere
// inside a hierarchy, was already reported.
func (p *Prog) copyEntry(ctx context.Context, e pair, opts options) error {
	fi, err := fs.Stat(p.FS, e.src)
	if err != nil {
		return fmt.Errorf("cannot stat '%s': %v", e.srcName, unwrapPathError(err))
	}
	switch {
	case fi.IsDir():
		if !opts.recursive {
			return fmt.Errorf("-R not specified; omitting directory '%s'", e.srcName)
		}
		return p.copyDir(ctx, e, fi, opts)
	case e.src == e.dst:
		return fmt.Errorf("'%s' and '%s' are the same file", e.srcName, e.dstName)
	default:
		return p.copyFile(ctx, e, fi, opts)
	}
}

// copyFile copies a regular file, prompting with -i before overwriting an
// existing file and, with -p, duplicating its mode and times.
func (p *Prog) copyFile(ctx context.Context, e pair, fi fs.FileInfo, opts options) error {
	dfi, err := fs.Stat(p.FS, e.dst)
	switch {
	case err == nil && dfi.IsDir():
		return fmt.Errorf("cannot overwrite directory '%s' with non-directory '%s'", e.dstName, e.srcName)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot stat '%s': %v", e.dstName, unwrapPathError(err))
	}
	if err == nil && opts.interactive && !p.confirm(e.dstName) {
		return errSkipped
	}

	perm := fi.Mode().Perm()
	if !opts.preserve {
		perm &^= p.Umask.Perm()
	}
	out, err := p.openDest(e.dst, perm, opts.force)
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

	if opts.preserve {
		if err := p.FS.Chmod(e.dst, fi.Mode()&chmodBits); err != nil {
			return fmt.Errorf("cannot set mode of '%s': %v", e.dstName, unwrapPathError(err))
		}
		if err := p.FS.Chtimes(e.dst, atime(fi), fi.ModTime()); err != nil {
			return fmt.Errorf("cannot set times of '%s': %v", e.dstName, unwrapPathError(err))
		}
	}
	return nil
}

// copyDir copies the hierarchy rooted at a directory. A missing destination
// is created with the source mode, and each entry that fails is reported
// while the remaining ones are still copied.
func (p *Prog) copyDir(ctx context.Context, e pair, fi fs.FileInfo, opts options) error {
	if insideSelf(e.src, e.dst) {
		return fmt.Errorf("cannot copy a directory, '%s', into itself, '%s'", e.srcName, e.dstName)
	}

	dfi, err := fs.Stat(p.FS, e.dst)
	created := false
	perm := dirPerm(fi, p.Umask.Perm(), opts.preserve)
	switch {
	case err == nil && dfi.IsDir():
	case err == nil:
		return fmt.Errorf("cannot overwrite non-directory '%s' with directory '%s'", e.dstName, e.srcName)
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot stat '%s': %v", e.dstName, unwrapPathError(err))
	default:
		if err := p.FS.Mkdir(e.dst, perm|0o700); err != nil {
			return fmt.Errorf("cannot create directory '%s': %v", e.dstName, unwrapPathError(err))
		}
		created = true
	}

	entries, err := fs.ReadDir(p.FS, e.src)
	if err != nil {
		return fmt.Errorf("cannot read directory '%s': %v", e.srcName, unwrapPathError(err))
	}

	ok := true
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := p.copyEntry(ctx, e.child(entry.Name()), opts)
		switch {
		case err == nil, errors.Is(err, errSkipped):
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, errFailed):
			ok = false
		default:
			p.errorf("%v", err)
			ok = false
		}
	}

	// Only directories created by this copy get their final mode and times,
	// so that a pre-existing destination is left as it was. The owner bits
	// added to the created directory for traversal are dropped only when they
	// aren't part of the final mode: in the real file system the umask was
	// already applied by Mkdir, and changing the mode again would undo it.
	if created {
		if opts.preserve || perm|0o700 != perm {
			if err := p.FS.Chmod(e.dst, perm); err != nil {
				return fmt.Errorf("cannot set mode of '%s': %v", e.dstName, unwrapPathError(err))
			}
		}
		if opts.preserve {
			if err := p.FS.Chtimes(e.dst, atime(fi), fi.ModTime()); err != nil {
				return fmt.Errorf("cannot set times of '%s': %v", e.dstName, unwrapPathError(err))
			}
		}
	}
	if !ok {
		return errFailed
	}
	return nil
}

// openDest opens dst for writing, truncating it when it already exists. When
// that fails and force is set, dst is removed so a new file can be created.
func (p *Prog) openDest(dst string, perm fs.FileMode, force bool) (fsx.File, error) {
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	out, err := p.FS.OpenFile(dst, flag, perm)
	if err != nil && force {
		if rmErr := p.FS.Remove(dst); rmErr == nil {
			out, err = p.FS.OpenFile(dst, flag, perm)
		}
	}
	return out, err
}

// confirm asks on the standard error whether dst should be overwritten and
// reads the answer from the standard input. Only a response starting with
// 'y' or 'Y' is taken as affirmative.
func (p *Prog) confirm(dst string) bool {
	fmt.Fprintf(p.Stderr, "cp: overwrite '%s'? ", dst)
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

// baseName returns the last component of an operand as typed, so that a
// trailing "/." copies the contents of the directory, while a trailing
// slash alone is ignored.
func baseName(name string) string {
	return path.Base(filepath.ToSlash(name))
}

// insideSelf reports whether dst is src itself or inside it, so a directory
// is never copied into its own hierarchy.
func insideSelf(src, dst string) bool {
	if src == dst {
		return true
	}
	switch src {
	case ".":
		return !path.IsAbs(dst) && dst != ".." && !strings.HasPrefix(dst, "../")
	case "/":
		return strings.HasPrefix(dst, "/")
	}
	return strings.HasPrefix(dst, src+"/")
}

// dirPerm returns the permission bits a copied directory gets from fi.
func dirPerm(fi fs.FileInfo, umask fs.FileMode, preserve bool) fs.FileMode {
	perm := fi.Mode().Perm()
	if !preserve {
		perm &^= umask
	}
	return perm
}

// atime returns the access time of fi, falling back to its modification time.
func atime(fi fs.FileInfo) time.Time {
	if t, ok := fsx.Atime(fi); ok {
		return t
	}
	return fi.ModTime()
}

// chmodBits are the mode bits that -p duplicates.
const chmodBits = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// errSkipped marks a copy declined through -i, which isn't a failure.
var errSkipped = errors.New("skipped")

// errFailed marks a hierarchy whose failure was already reported.
var errFailed = errors.New("failed")

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
	fmt.Fprintf(p.Stderr, "cp: "+format+"\n", args...)
}

// unwrapPathError unwraps *fs.PathError so diagnostics don't repeat the file
// name.
func unwrapPathError(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}

// options holds the -f, -i, -p and -R flags.
type options struct {
	force       bool
	interactive bool
	preserve    bool
	recursive   bool
}

// longOptions maps the long options of cp to the short ones they stand for.
var longOptions = map[string]rune{
	"dereference":    'L',
	"force":          'f',
	"interactive":    'i',
	"no-dereference": 'P',
	"preserve":       'p',
	"recursive":      'R',
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := getopt.Set{Long: longOptions}.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'f':
			opts.force = true
		case 'i':
			opts.interactive = true
		case 'p':
			opts.preserve = true
		case 'R':
			opts.recursive = true
		case 'H', 'L', 'P':
			// Symbolic links aren't supported, so these have no effect.
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
