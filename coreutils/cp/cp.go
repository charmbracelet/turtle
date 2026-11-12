// Package cp implements the POSIX cp utility, plus the GNU -r, -a, -n, -u,
// -v, -t, -T, -d and --preserve options. Only the plain -u form is
// supported; the --update=WHEN form of GNU 9.3+ is out of scope, as is
// preserving links between source files.
//
// With -R, symbolic links in the source are copied as links, -L follows
// them all, -H follows only the command line operands, and -P or -d, as
// well as -a, copy them as links in every case. Outside -R, the default is
// to follow a symbolic link operand, as POSIX asks.
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

	"charm.land/snail/coreutils"

	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/filemode"
	"charm.land/snail/coreutils/internal/fsop"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
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
// destination named by the last operand or by -t: a directory to copy into,
// or the file to copy to when there's exactly one source. -T keeps the last
// operand from ever being taken as a directory.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "cp", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "cp", err)
	}
	sources, target, err := fsop.SplitOperands(operands, opts.targetDir, opts.targetDirSet, opts.noTarget)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "cp", err)
	}
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "cp", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if slices.Contains(operands, "") || (opts.targetDirSet && opts.targetDir == "") {
		stdopts.Errorf(p.Runtime, "cp", "invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}

	dst := p.FsPath(target)
	fi, err := fs.Stat(p.FS, dst)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		stdopts.Errorf(p.Runtime, "cp", "cannot stat '%s': %v", target, patherr.Unwrap(err))
		return &coreutils.ExitError{Code: 1}
	}
	targetIsDir := err == nil && fi.IsDir()
	slash := strings.HasSuffix(filepath.ToSlash(target), "/")
	if opts.targetDirSet && !targetIsDir {
		stdopts.Errorf(p.Runtime, "cp", "target '%s' is not a directory", target)
		return &coreutils.ExitError{Code: 1}
	}
	into := targetIsDir && !opts.noTarget
	if !targetIsDir && (len(sources) > 1 || (slash && !opts.recursive)) {
		stdopts.Errorf(p.Runtime, "cp", "target '%s' is not a directory", target)
		return &coreutils.ExitError{Code: 1}
	}

	ok := true
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		e := fsop.Pair{Src: p.FsPath(source), SrcName: source, Dst: dst, DstName: target}
		if into {
			base := coreutils.FsBase(source)
			e.Dst = path.Join(dst, base)
			e.DstName = path.Join(target, base)
		}
		err := p.copyEntry(ctx, e, opts, opts.links.operand(opts.recursive))
		switch {
		case err == nil, errors.Is(err, errSkipped):
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, errFailed):
			ok = false
		default:
			stdopts.Errorf(p.Runtime, "cp", "%v", err)
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// copyEntry copies a single file or symbolic link, or with -R the hierarchy
// rooted at a directory. follow tells whether a final symbolic link of the
// source is dereferenced, following the -H, -L, -P and -d policy. Errors are
// returned ready to be reported; errSkipped is returned when a copy was
// declined through -n, -u or -i, and errFailed when the failure, somewhere
// inside a hierarchy, was already reported.
func (p *Prog) copyEntry(ctx context.Context, e fsop.Pair, opts options, follow bool) error {
	return p.copyEntryVisited(ctx, e, opts, follow, nil)
}

// copyEntryVisited copies one entry, tracking the directories the walk of
// which it is part already entered, so cycles through symbolic links are
// caught.
func (p *Prog) copyEntryVisited(ctx context.Context, e fsop.Pair, opts options, follow bool, visited map[string]bool) error {
	fi, err := p.stat(e.Src, follow)
	if err != nil {
		return fmt.Errorf("cannot stat '%s': %v", e.SrcName, patherr.Unwrap(err))
	}
	switch {
	case fi.IsDir():
		if !opts.recursive {
			return fmt.Errorf("-R not specified; omitting directory '%s'", e.SrcName)
		}
		return p.copyDir(ctx, e, fi, opts, opts.links.tree(), visited)
	case e.Src == e.Dst:
		return fmt.Errorf("'%s' and '%s' are the same file", e.SrcName, e.DstName)
	case fi.Mode()&fs.ModeSymlink != 0:
		return p.copySymlink(e, fi, opts)
	default:
		return p.copyFile(ctx, e, fi, opts)
	}
}

// copyFile copies a regular file, applying the -n, -u and -i policies to an
// existing destination and preserving the requested attributes.
func (p *Prog) copyFile(ctx context.Context, e fsop.Pair, fi fs.FileInfo, opts options) error {
	if p.danglingLink(e.Dst) {
		return fmt.Errorf("not writing through dangling symlink '%s'", e.DstName)
	}
	dfi, err := fs.Stat(p.FS, e.Dst)
	switch {
	case err == nil && dfi.IsDir():
		return fmt.Errorf("cannot overwrite directory '%s' with non-directory '%s'", e.DstName, e.SrcName)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot stat '%s': %v", e.DstName, patherr.Unwrap(err))
	}
	if err == nil {
		switch {
		case opts.clobber == clobberSkip:
			return errSkipped
		case opts.update && !fi.ModTime().After(dfi.ModTime()):
			return errSkipped
		case opts.clobber == clobberPrompt && !fsop.Confirm(p.Runtime, "cp", "overwrite", e.DstName):
			return errSkipped
		}
	}

	perm := fi.Mode().Perm()
	if !opts.preserveMode {
		perm &^= p.Umask.Perm()
	}
	out, err := p.openDest(e.Dst, perm, opts.force)
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

	if opts.preserveMode {
		if err := p.FS.Chmod(e.Dst, fi.Mode()&filemode.AccessBits); err != nil {
			return fmt.Errorf("cannot set mode of '%s': %v", e.DstName, patherr.Unwrap(err))
		}
	}
	if err := p.preserve(e, fi, opts); err != nil {
		return err
	}
	if opts.verbose && p.Stdout != nil {
		fmt.Fprintf(p.Stdout, "'%s' -> '%s'\n", e.SrcName, e.DstName)
	}
	return nil
}

// copySymlink copies a symbolic link as itself, with the same target, after
// the -n, -u and -i policies were applied to an existing destination.
func (p *Prog) copySymlink(e fsop.Pair, fi fs.FileInfo, opts options) error {
	target, err := p.FS.ReadLink(e.Src)
	if err != nil {
		return fmt.Errorf("cannot read link '%s': %v", e.SrcName, patherr.Unwrap(err))
	}
	dfi, err := fs.Stat(p.FS, e.Dst)
	switch {
	case err == nil && dfi.IsDir():
		return fmt.Errorf("cannot overwrite directory '%s' with non-directory '%s'", e.DstName, e.SrcName)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot stat '%s': %v", e.DstName, patherr.Unwrap(err))
	}
	if err == nil {
		switch {
		case opts.clobber == clobberSkip:
			return errSkipped
		case opts.update && !fi.ModTime().After(dfi.ModTime()):
			return errSkipped
		case opts.clobber == clobberPrompt && !fsop.Confirm(p.Runtime, "cp", "overwrite", e.DstName):
			return errSkipped
		}
		if err := p.FS.Remove(e.Dst); err != nil {
			return fmt.Errorf("cannot remove '%s': %v", e.DstName, patherr.Unwrap(err))
		}
	}
	if err := p.FS.Symlink(target, e.Dst); err != nil {
		return fmt.Errorf("cannot create symbolic link '%s' to '%s': %v", e.DstName, target, patherr.Unwrap(err))
	}
	if opts.verbose && p.Stdout != nil {
		fmt.Fprintf(p.Stdout, "'%s' -> '%s'\n", e.SrcName, e.DstName)
	}
	return nil
}

// copyDir copies the hierarchy rooted at a directory. A missing destination
// is created with the source mode, and each entry that fails is reported
// while the remaining ones are still copied. follow tells whether the
// symbolic links inside the hierarchy are dereferenced.
func (p *Prog) copyDir(ctx context.Context, e fsop.Pair, fi fs.FileInfo, opts options, follow bool, visited map[string]bool) error {
	if insideSelf(fsop.ResolveSymlinks(p.FS, e.Src), fsop.ResolveSymlinks(p.FS, e.Dst)) {
		return fmt.Errorf("cannot copy a directory, '%s', into itself, '%s'", e.SrcName, e.DstName)
	}

	// A symbolic link dereferenced to a directory already being copied
	// would recurse forever: like GNU, it is refused.
	resolved := fsop.ResolveSymlinks(p.FS, e.Src)
	if visited == nil {
		visited = make(map[string]bool)
	}
	if visited[resolved] {
		return fmt.Errorf("cannot copy cyclic directory symlink '%s'", e.SrcName)
	}
	visited[resolved] = true
	defer delete(visited, resolved)

	dfi, err := fs.Stat(p.FS, e.Dst)
	created := false
	perm := dirPerm(fi, p.Umask.Perm(), opts.preserveMode)
	switch {
	case err == nil && dfi.IsDir():
	case err == nil:
		return fmt.Errorf("cannot overwrite non-directory '%s' with directory '%s'", e.DstName, e.SrcName)
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot stat '%s': %v", e.DstName, patherr.Unwrap(err))
	default:
		if err := p.FS.Mkdir(e.Dst, perm|0o700); err != nil {
			return fmt.Errorf("cannot create directory '%s': %v", e.DstName, patherr.Unwrap(err))
		}
		created = true
	}

	if opts.verbose && p.Stdout != nil {
		fmt.Fprintf(p.Stdout, "'%s' -> '%s'\n", e.SrcName, e.DstName)
	}

	entries, err := fs.ReadDir(p.FS, e.Src)
	if err != nil {
		return fmt.Errorf("cannot read directory '%s': %v", e.SrcName, patherr.Unwrap(err))
	}

	ok := true
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := p.copyEntryVisited(ctx, e.Child(entry.Name()), opts, follow, visited)
		switch {
		case err == nil, errors.Is(err, errSkipped):
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, errFailed):
			ok = false
		default:
			stdopts.Errorf(p.Runtime, "cp", "%v", err)
			ok = false
		}
	}

	// Only directories created by this copy get their final mode and times,
	// so that a pre-existing destination is left as it was. The owner bits
	// added to the created directory for traversal are dropped only when they
	// aren't part of the final mode: in the real file system the umask was
	// already applied by Mkdir, and changing the mode again would undo it.
	if created {
		if opts.preserveMode || perm|0o700 != perm {
			if err := p.FS.Chmod(e.Dst, perm); err != nil {
				return fmt.Errorf("cannot set mode of '%s': %v", e.DstName, patherr.Unwrap(err))
			}
		}
		if err := p.preserve(e, fi, opts); err != nil {
			return err
		}
	}
	if !ok {
		return errFailed
	}
	return nil
}

// preserve duplicates the requested times and owner of the source entry at
// the destination.
func (p *Prog) preserve(e fsop.Pair, fi fs.FileInfo, opts options) error {
	if opts.preserveTimes {
		if err := p.FS.Chtimes(e.Dst, fsop.Atime(fi), fi.ModTime()); err != nil {
			return fmt.Errorf("cannot set times of '%s': %v", e.DstName, patherr.Unwrap(err))
		}
	}
	if opts.preserveOwnership {
		chowner, ok := p.FS.(fsx.ChownFS)
		if !ok {
			return nil
		}
		uid, gid, ok := fsx.Owner(fi)
		if !ok {
			return nil
		}
		if err := chowner.Chown(e.Dst, uid, gid); err != nil {
			return fmt.Errorf("cannot set owner of '%s': %v", e.DstName, patherr.Unwrap(err))
		}
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

// stat describes name, following a final symbolic link only when asked.
func (p *Prog) stat(name string, follow bool) (fs.FileInfo, error) {
	if follow {
		return fs.Stat(p.FS, name)
	}
	return p.FS.Lstat(name)
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
func dirPerm(fi fs.FileInfo, umask fs.FileMode, preserveMode bool) fs.FileMode {
	perm := fi.Mode().Perm()
	if !preserveMode {
		perm &^= umask
	}
	return perm
}

// errSkipped marks a copy declined through -n, -u or -i, which isn't a
// failure.
var errSkipped = errors.New("skipped")

// errFailed marks a hierarchy whose failure was already reported.
var errFailed = errors.New("failed")

// options holds the flags of cp. For -i and -n, only the last one given
// takes effect, and -u is ignored when -n is in effect.
type options struct {
	force             bool
	clobber           clobber
	update            bool
	verbose           bool
	recursive         bool
	links             links
	preserveMode      bool
	preserveTimes     bool
	preserveOwnership bool
	targetDir         string
	targetDirSet      bool
	noTarget          bool
}

// clobber is the policy for an existing destination, set by the last of
// -i and -n.
type clobber int

const (
	clobberOverwrite clobber = iota // the default
	clobberPrompt                   // -i
	clobberSkip                     // -n
)

// links is the policy for the symbolic links of the source, set by the last
// of -H, -L, -P, -d and -a.
type links int

const (
	linksDefault links = iota // follow operands only without -R
	linksCopy                 // -P, -d and -a: links are copied as links
	linksOperand              // -H: operands are followed, tree links are not
	linksFollow               // -L: links are always followed
)

// operand reports whether a symbolic link named on the command line is
// dereferenced, when recursion is in effect or not.
func (l links) operand(recursive bool) bool {
	switch l {
	case linksCopy:
		return false
	case linksFollow, linksOperand:
		return true
	default:
		return !recursive
	}
}

// tree reports whether the symbolic links found while copying a hierarchy
// are dereferenced.
func (l links) tree() bool {
	return l == linksFollow
}

// optPreserve and optNoPreserve are runes which no short option can reach,
// so --preserve and --no-preserve stay long-only.
const (
	optPreserve rune = 0x100 + iota
	optNoPreserve
)

// longOptions maps the long options of cp to the short ones they stand for.
var longOptions = map[string]rune{
	"archive":             'a',
	"dereference":         'L',
	"force":               'f',
	"interactive":         'i',
	"no-clobber":          'n',
	"no-dereference":      'P',
	"no-preserve":         optNoPreserve,
	"no-target-directory": 'T',
	"preserve":            optPreserve,
	"recursive":           'R',
	"target-directory":    't',
	"update":              'u',
	"verbose":             'v',
}

// optSet describes the options of cp.
var optSet = getopt.Set{
	Valued:   "t" + string(optNoPreserve),
	Optional: string(optPreserve),
	Long:     longOptions,
}

// usage describes the options of cp.
const usage = `Usage: cp [OPTION]... [-T] SOURCE DEST
  or:  cp [OPTION]... SOURCE... DIRECTORY
  or:  cp [OPTION]... -t DIRECTORY SOURCE...
Copy SOURCE to DEST, or multiple SOURCE(s) to DIRECTORY.

  -a, --archive
         same as -dR --preserve=all
  -d
         same as --no-dereference --preserve=links
  -f, --force
         if an existing destination file cannot be opened, remove it and try
         again
  -H
         follow command-line symbolic links in SOURCE
  -i, --interactive
         prompt before overwrite
  -L, --dereference
         always follow symbolic links in SOURCE
  -n, --no-clobber
         do not overwrite an existing file
  -P, --no-dereference
         never follow symbolic links in SOURCE
  -p
         same as --preserve=mode,ownership,timestamps
      --no-preserve=ATTR_LIST
         don't preserve the specified attributes
      --preserve[=ATTR_LIST]
         preserve the specified attributes (default: mode,ownership,timestamps)
  -R, -r, --recursive
         copy directories recursively
  -t, --target-directory=DIRECTORY
         copy all SOURCE arguments into DIRECTORY
  -T, --no-target-directory
         treat DEST as a normal file
  -u, --update
         copy only when the SOURCE file is newer than the destination file or is
         missing
  -v, --verbose
         explain what is being done
` + stdopts.Tail

// setPreserve applies a --preserve or --no-preserve attribute list, where an
// empty list selects the default attributes.
func (o *options) setPreserve(list string, enable bool) error {
	if list == "" {
		o.preserveMode = enable
		o.preserveTimes = enable
		if enable {
			// The default of --preserve is GNU's -a set, ownership
			// included.
			o.preserveOwnership = true
		}
		return nil
	}
	for name := range strings.SplitSeq(list, ",") {
		switch name {
		case "all":
			o.preserveMode = enable
			o.preserveTimes = enable
			o.preserveOwnership = enable
		case "mode":
			o.preserveMode = enable
		case "timestamps":
			o.preserveTimes = enable
		case "ownership":
			o.preserveOwnership = enable
		case "links":
			// Links between source files aren't preserved.
		default:
			return fmt.Errorf("invalid attribute list: '%s'", list)
		}
	}
	return nil
}

// danglingLink reports whether name is a symbolic link which points at
// nothing: writing through it would create a file somewhere unexpected.
func (p *Prog) danglingLink(name string) bool {
	fi, err := p.FS.Lstat(name)
	if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		return false
	}
	_, err = fs.Stat(p.FS, name)
	return errors.Is(err, fs.ErrNotExist)
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--", the last of -i and -n wins, as does the last of
// -H, -L, -P, -d and -a, and -t can't be combined with -T.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
			opts.recursive = true
			opts.links = linksCopy
			opts.preserveMode = true
			opts.preserveTimes = true
			opts.preserveOwnership = true
		case 'f':
			opts.force = true
		case 'i':
			opts.clobber = clobberPrompt
		case 'n':
			opts.clobber = clobberSkip
		case 'p':
			opts.preserveMode = true
			opts.preserveTimes = true
			opts.preserveOwnership = true
		case 'r', 'R':
			opts.recursive = true
		case 'H':
			opts.links = linksOperand
		case 'L':
			opts.links = linksFollow
		case 'P', 'd':
			opts.links = linksCopy
		case 't':
			opts.targetDir, opts.targetDirSet = value, true
		case 'T':
			opts.noTarget = true
		case 'u':
			opts.update = true
		case 'v':
			opts.verbose = true
		case optPreserve:
			return opts.setPreserve(value, true)
		case optNoPreserve:
			if value == "" {
				return fmt.Errorf("ambiguous argument '' for '--no-preserve'")
			}
			return opts.setPreserve(value, false)
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
