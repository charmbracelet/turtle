// Package install implements the GNU install utility, which copies files
// while setting their mode bits and, when possible, their owner and group.
// It's the tool Makefiles use to put programs in place.
//
// The default mode of installed files and directories is 'u=rwx,go=rx,a-s',
// which the current umask never masks. By default the ownership of the
// destination is left alone, instead of becoming the one of the process like
// GNU does, since the runtime carries no user or group identity. Out of
// scope are the --strip, --strip-program, -Z/--context, --preserve-context
// and --debug options, which need external programs or SELinux support the
// runtime doesn't have.
package install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/chmod"
	"charm.land/snail/coreutils/internal/backup"
	"charm.land/snail/coreutils/internal/ctxio"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/fsx"
)

// Prog is the install core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// settings holds what Run resolved out of the options, shared by every file
// and directory installed.
type settings struct {
	opts options

	// fileMode and dirMode are the mode bits installed files and directories
	// get. The umask never masks them.
	fileMode, dirMode fs.FileMode

	// uid and gid are the owner and group installed files get, or -1 to
	// leave the ownership alone.
	uid, gid int

	// bk describes the backups of overwritten files.
	bk backup.Options
}

// Run installs each source file, giving the destination the attributes the
// options ask for: the last operand names the destination, a file when
// there's exactly one source or a directory to copy into otherwise, -t and
// -T overruling that. With -d, the operands name the directories created
// instead.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "install", usage, optSet, args); done {
		return err
	}
	opts, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "install")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	bk, err := p.backupOptions(opts)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "install")
		return &coreutils.ExitError{Code: 2}
	}
	set, err := p.settings(opts, bk)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	if set.opts.compare && set.fileMode&specialBits != 0 {
		p.errorf("the --compare (-C) option is ignored when you specify a mode with non-permission bits")
	}
	if slices.Contains(operands, "") || (opts.targetDirSet && opts.targetDir == "") {
		p.errorf("invalid zero-length file name")
		return &coreutils.ExitError{Code: 1}
	}

	if opts.directory {
		if len(operands) == 0 {
			p.errorf("missing operand")
			stdopts.TryHelp(p.Runtime, "install")
			return &coreutils.ExitError{Code: 2}
		}
		ok := true
		for _, dir := range operands {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := p.installDir(dir, set); err != nil {
				p.errorf("%v", err)
				ok = false
			}
		}
		if !ok {
			return &coreutils.ExitError{Code: 1}
		}
		return nil
	}

	sources, target, err := splitOperands(operands, opts)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "install")
		return &coreutils.ExitError{Code: 2}
	}

	dst := p.resolve(target)
	into := false
	if opts.targetDirSet {
		if opts.createParents {
			if err := p.createTargetDir(target, set); err != nil {
				p.errorf("%v", err)
				return &coreutils.ExitError{Code: 1}
			}
		}
		fi, err := fs.Stat(p.FS, dst)
		switch {
		case err != nil:
			p.errorf("failed to access '%s': %v", target, patherr.Unwrap(err))
			return &coreutils.ExitError{Code: 1}
		case !fi.IsDir():
			p.errorf("failed to access '%s': not a directory", target)
			return &coreutils.ExitError{Code: 1}
		}
		into = true
	} else if fi, err := fs.Stat(p.FS, dst); err == nil && fi.IsDir() {
		into = !opts.noTarget
	}
	if !into && len(sources) > 1 {
		p.errorf("target '%s' is not a directory", target)
		return &coreutils.ExitError{Code: 1}
	}
	if opts.createParents && !opts.targetDirSet {
		if err := p.createParents(target, set); err != nil {
			p.errorf("%v", err)
			return &coreutils.ExitError{Code: 1}
		}
	}

	ok := true
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		e := pair{src: p.resolve(source), srcName: source, dst: dst, dstName: target}
		if into {
			base := path.Base(filepath.ToSlash(source))
			e.dst, e.dstName = path.Join(dst, base), path.Join(target, base)
		}
		if err := p.installFile(ctx, e, set); err != nil {
			switch {
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

// settings resolves the mode, owner and group of the files installed out of
// the options: the mode, given as an octal number or a symbolic mode as in
// chmod with 'a=' as the point of departure, defaults to 'u=rwx,go=rx,a-s',
// and the owner and group, given as names or numeric IDs, default to being
// left alone.
func (p *Prog) settings(opts options, bk backup.Options) (settings, error) {
	set := settings{opts: opts, bk: bk, uid: -1, gid: -1, fileMode: 0o755, dirMode: 0o755}
	if opts.modeSet {
		spec, err := chmod.ParseMode(opts.mode)
		if err != nil {
			return set, err
		}
		umask := p.Umask.Perm()
		set.fileMode = spec(0, false, umask)
		set.dirMode = spec(0, true, umask)
	}
	if opts.ownerSet {
		uid, err := parseOwner(opts.owner)
		if err != nil {
			return set, err
		}
		set.uid = uid
	}
	if opts.groupSet {
		gid, err := parseGroup(opts.group)
		if err != nil {
			return set, err
		}
		set.gid = gid
	}
	return set, nil
}

// installFile copies one source file to its destination, giving it the mode,
// owner, group and times the options ask for. A destination which exists is
// replaced, after a backup of it was made when asked, so a symbolic link
// never gets written through.
func (p *Prog) installFile(ctx context.Context, e pair, set settings) error {
	fi, err := fs.Stat(p.FS, e.src)
	if err != nil {
		return fmt.Errorf("cannot stat '%s': %v", e.srcName, patherr.Unwrap(err))
	}
	if fi.IsDir() {
		return fmt.Errorf("omitting directory '%s'", e.srcName)
	}
	if p.resolveSymlinks(e.src) == p.resolveSymlinks(e.dst) {
		return fmt.Errorf("'%s' and '%s' are the same file", e.srcName, e.dstName)
	}
	dfi, derr := p.FS.Lstat(e.dst)
	if derr != nil && !errors.Is(derr, fs.ErrNotExist) {
		return fmt.Errorf("cannot stat '%s': %v", e.dstName, patherr.Unwrap(derr))
	}
	destExists := derr == nil
	if destExists && dfi.IsDir() && set.bk.Method == backup.None {
		return fmt.Errorf("cannot overwrite directory '%s' with non-directory '%s'", e.dstName, e.srcName)
	}
	if set.opts.compare && destExists {
		up, err := p.upToDate(ctx, e, fi, dfi, set)
		if err != nil {
			return err
		}
		if up {
			if set.opts.preserveTimes {
				if err := p.FS.Chtimes(e.dst, atime(fi), fi.ModTime()); err != nil {
					return fmt.Errorf("cannot set timestamps for '%s': %v", e.dstName, patherr.Unwrap(err))
				}
			}
			return nil
		}
	}

	var backupName string
	switch {
	case !destExists:
	case set.bk.Method != backup.None:
		name, err := backup.Name(p.FS, e.dst, set.bk)
		if err != nil {
			return fmt.Errorf("cannot backup '%s': %v", e.dstName, patherr.Unwrap(err))
		}
		if name == e.src {
			return fmt.Errorf("backing up '%s' might destroy source; '%s' not copied", e.dstName, e.srcName)
		}
		if err := p.FS.Rename(e.dst, name); err != nil {
			return fmt.Errorf("cannot backup '%s': %v", e.dstName, patherr.Unwrap(err))
		}
		backupName = name
	default:
		if err := p.FS.Remove(e.dst); err != nil {
			return fmt.Errorf("cannot remove '%s': %v", e.dstName, patherr.Unwrap(err))
		}
		if set.opts.verbose && p.Stdout != nil {
			fmt.Fprintf(p.Stdout, "removed '%s'\n", e.dstName)
		}
	}

	// O_TRUNC matters only when the backup kept the destination itself, as
	// a suffix of "" asks: a shorter source would leave bytes behind.
	out, err := p.FS.OpenFile(e.dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, set.fileMode.Perm())
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
	if err := p.setAttrs(e, fi, set); err != nil {
		return err
	}
	if set.opts.verbose && p.Stdout != nil {
		if backupName != "" {
			fmt.Fprintf(p.Stdout, "'%s' -> '%s' (backup: '%s')\n", e.srcName, e.dstName, e.dstName+strings.TrimPrefix(backupName, e.dst))
		} else {
			fmt.Fprintf(p.Stdout, "'%s' -> '%s'\n", e.srcName, e.dstName)
		}
	}
	return nil
}

// setAttrs gives an installed file its mode, owner, group and, with -p, the
// times of the source file it came from. The mode is set after the ownership,
// so the special bits survive a change of owner.
func (p *Prog) setAttrs(e pair, fi fs.FileInfo, set settings) error {
	if runtime.GOOS != "windows" && (set.uid >= 0 || set.gid >= 0) {
		if err := p.chown(e.dst, set.uid, set.gid); err != nil {
			return fmt.Errorf("cannot change ownership of '%s': %v", e.dstName, patherr.Unwrap(err))
		}
	}
	if err := p.FS.Chmod(e.dst, set.fileMode&chmodBits); err != nil {
		return fmt.Errorf("cannot change permissions of '%s': %v", e.dstName, patherr.Unwrap(err))
	}
	if set.opts.preserveTimes {
		if err := p.FS.Chtimes(e.dst, atime(fi), fi.ModTime()); err != nil {
			return fmt.Errorf("cannot set timestamps for '%s': %v", e.dstName, patherr.Unwrap(err))
		}
	}
	return nil
}

// upToDate reports whether installing e would leave its destination as it
// is: the same content, mode, owner and group it has now. A context error
// is returned as is, so a cancelled comparison stops the installation.
func (p *Prog) upToDate(ctx context.Context, e pair, fi, dfi fs.FileInfo, set settings) (bool, error) {
	if !fi.Mode().IsRegular() || !dfi.Mode().IsRegular() {
		return false, nil
	}
	if set.fileMode&specialBits != 0 || fi.Mode()&specialBits != 0 || dfi.Mode()&specialBits != 0 {
		return false, nil
	}
	if dfi.Mode().Perm() != set.fileMode.Perm() || fi.Size() != dfi.Size() {
		return false, nil
	}
	if set.uid >= 0 {
		if uid, _, ok := fsx.Owner(dfi); !ok || uid != set.uid {
			return false, nil
		}
	}
	if set.gid >= 0 {
		if _, gid, ok := fsx.Owner(dfi); !ok || gid != set.gid {
			return false, nil
		}
	}
	return p.sameContent(ctx, e, fi.Size())
}

// sameContent reports whether the source and destination of e hold the same
// bytes. A cancelled context stops the comparison with its error.
func (p *Prog) sameContent(ctx context.Context, e pair, size int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	in, err := p.FS.Open(e.src)
	if err != nil {
		return false, nil
	}
	defer in.Close()
	out, err := p.FS.Open(e.dst)
	if err != nil {
		return false, nil
	}
	defer out.Close()
	var (
		left, right [32 * 1024]byte
		rest        = size
	)
	for rest > 0 {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		n, err := io.ReadFull(ctxio.NewReader(ctx, in), left[:min(int64(len(left)), rest)])
		if err != nil && ctx.Err() != nil {
			return false, ctx.Err()
		}
		if n == 0 {
			return false, nil
		}
		if _, err := io.ReadFull(ctxio.NewReader(ctx, out), right[:n]); err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return false, nil
		}
		if !bytes.Equal(left[:n], right[:n]) {
			return false, nil
		}
		rest -= int64(n)
	}
	// Extra bytes in the destination also make a difference.
	n, err := io.ReadFull(ctxio.NewReader(ctx, out), right[:1])
	switch {
	case ctx.Err() != nil:
		return false, ctx.Err()
	case n > 0:
		return false, nil
	}
	return true, nil
}

// installDir installs one directory of -d: the missing components above it
// are created with the default attributes, and the directory itself, created
// or not, gets the mode, owner and group the options ask for. Unlike mkdir
// -p, an existing directory isn't left alone.
func (p *Prog) installDir(dir string, set settings) error {
	comps := coreutils.SplitPath(filepath.ToSlash(p.ResolvePath(dir)))
	if len(comps) == 0 {
		comps = []string{"."}
	}
	disp := coreutils.SplitPath(path.Clean(filepath.ToSlash(dir)))
	for i := range comps {
		name := coreutils.JoinPath(comps[:i+1])
		last := i == len(comps)-1

		fi, err := fs.Stat(p.FS, name)
		switch {
		case err == nil && fi.IsDir():
			continue
		case err == nil:
			cause := fs.ErrExist
			if !last {
				cause = errNotDir
			}
			return fmt.Errorf("cannot create directory '%s': %v", dispPath(disp, i), cause)
		case !errors.Is(err, fs.ErrNotExist):
			return fmt.Errorf("cannot stat '%s': %v", dir, patherr.Unwrap(err))
		}
		perm := fs.FileMode(0o755)
		if last {
			perm = set.dirMode.Perm()
		}
		if err := p.FS.Mkdir(name, perm); err != nil {
			return fmt.Errorf("cannot create directory '%s': %v", dispPath(disp, i), patherr.Unwrap(err))
		}
		if set.opts.verbose {
			fmt.Fprintf(p.Stdout, "install: creating directory '%s'\n", dispPath(disp, i))
		}
	}
	return p.setDirAttrs(coreutils.JoinPath(comps), dispPath(disp, len(disp)-1), set)
}

// setDirAttrs gives a directory installed through -d or -D its mode, owner
// and group. The mode is set after the ownership, so the special bits
// survive a change of owner.
func (p *Prog) setDirAttrs(name, shown string, set settings) error {
	if runtime.GOOS != "windows" && (set.uid >= 0 || set.gid >= 0) {
		if err := p.chown(name, set.uid, set.gid); err != nil {
			return fmt.Errorf("cannot change owner and permissions of '%s': %v", shown, patherr.Unwrap(err))
		}
	}
	if err := p.FS.Chmod(name, set.dirMode&chmodBits); err != nil {
		return fmt.Errorf("cannot change owner and permissions of '%s': %v", shown, patherr.Unwrap(err))
	}
	return nil
}

// createTargetDir creates the target directory of -t, and the components
// above it, when -D asks for them. They all get the default attributes.
func (p *Prog) createTargetDir(dir string, set settings) error {
	return p.createHierarchy(dir, 0, set)
}

// createParents creates the components above the destination file of -D,
// with the default attributes.
func (p *Prog) createParents(dest string, set settings) error {
	return p.createHierarchy(dest, 1, set)
}

// createHierarchy creates the missing components of dir: all of them when
// skip is zero, or the ones above the last when it skips one, with the
// default attributes: mode 'u=rwx,go=rx', regardless of the current umask.
func (p *Prog) createHierarchy(dir string, skip int, set settings) error {
	comps := coreutils.SplitPath(filepath.ToSlash(p.ResolvePath(dir)))
	disp := coreutils.SplitPath(path.Clean(filepath.ToSlash(dir)))
	for i := range comps {
		if i >= len(comps)-skip {
			break
		}
		name := coreutils.JoinPath(comps[:i+1])
		fi, err := fs.Stat(p.FS, name)
		switch {
		case err == nil && fi.IsDir():
			continue
		case err == nil:
			if skip > 0 {
				return fmt.Errorf("cannot create directory '%s': %v", dispPath(disp, i), errNotDir)
			}
			return fmt.Errorf("failed to access '%s': %v", dir, errNotDir)
		case !errors.Is(err, fs.ErrNotExist):
			return fmt.Errorf("failed to access '%s': %v", dir, patherr.Unwrap(err))
		}
		if err := p.FS.Mkdir(name, 0o755); err != nil {
			return fmt.Errorf("cannot create directory '%s': %v", dispPath(disp, i), patherr.Unwrap(err))
		}
		if err := p.FS.Chmod(name, 0o755); err != nil {
			return fmt.Errorf("cannot set mode of '%s': %v", dispPath(disp, i), patherr.Unwrap(err))
		}
		if set.opts.verbose {
			fmt.Fprintf(p.Stdout, "install: creating directory '%s'\n", dispPath(disp, i))
		}
	}
	return nil
}

// dispPath returns the name shown in diagnostics for the i-th component of
// the display path disp, which mirrors the path used with the file system.
func dispPath(disp []string, i int) string {
	if i < 0 || i >= len(disp) {
		return coreutils.JoinPath(disp)
	}
	return coreutils.JoinPath(disp[:i+1])
}

// backupOptions resolves the backup method and suffix out of the options and
// the environment: the method comes from --backup or, without one, from
// VERSION_CONTROL, and the suffix from -S or SIMPLE_BACKUP_SUFFIX.
func (p *Prog) backupOptions(opts options) (backup.Options, error) {
	if !opts.backupSet {
		return backup.Options{Method: backup.None}, nil
	}
	method := backup.Existing
	if opts.backupArg != "" {
		m, err := backup.ParseMethod(opts.backupArg)
		if err != nil {
			return backup.Options{}, err
		}
		method = m
	} else if env, ok := p.LookupEnv("VERSION_CONTROL"); ok && env != "" {
		m, err := backup.ParseMethod(env)
		if err != nil {
			p.errorf("ignoring invalid value of environment variable VERSION_CONTROL")
		} else {
			method = m
		}
	}
	suffix := backup.DefaultSuffix
	switch {
	case opts.suffixSet:
		suffix = opts.suffix
	default:
		if env, ok := p.LookupEnv("SIMPLE_BACKUP_SUFFIX"); ok {
			suffix = env
		}
	}
	return backup.Options{Method: method, Suffix: suffix}, nil
}

// chown changes the owner and group of name, when the file system supports
// it, leaving the ownership alone otherwise.
func (p *Prog) chown(name string, uid, gid int) error {
	chowner, ok := p.FS.(fsx.ChownFS)
	if !ok {
		return nil
	}
	return chowner.Chown(name, uid, gid)
}

// parseOwner resolves an owner operand, a user name or a numeric user ID.
func parseOwner(s string) (int, error) {
	if n, err := strconv.Atoi(s); err == nil {
		return n, nil
	}
	u, err := user.Lookup(s)
	if err != nil {
		return 0, fmt.Errorf("invalid user '%s'", s)
	}
	n, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, fmt.Errorf("invalid user '%s'", s)
	}
	return n, nil
}

// parseGroup resolves a group operand, a group name or a numeric group ID.
func parseGroup(s string) (int, error) {
	if n, err := strconv.Atoi(s); err == nil {
		return n, nil
	}
	g, err := user.LookupGroup(s)
	if err != nil {
		return 0, fmt.Errorf("invalid group '%s'", s)
	}
	n, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, fmt.Errorf("invalid group '%s'", s)
	}
	return n, nil
}

// pair holds a source and its destination, both as the path used with the
// file system and as the name shown in diagnostics.
type pair struct {
	src, dst         string
	srcName, dstName string
}

// resolve converts an operand into the slash-separated path used with the
// file system, relative to the working directory.
func (p *Prog) resolve(name string) string {
	return path.Clean(filepath.ToSlash(p.ResolvePath(name)))
}

// resolveSymlinks resolves every component of name, following the symbolic
// links found on the way, and returns the canonical path it names. Broken
// links and loops give up and keep the remaining components as they are.
func (p *Prog) resolveSymlinks(name string) string {
	var resolved []string
	for comp := range strings.SplitSeq(path.Clean(name), "/") {
		switch comp {
		case "", ".":
			continue
		case "..":
			if len(resolved) > 0 {
				resolved = resolved[:len(resolved)-1]
			}
			continue
		}
		resolved = append(resolved, comp)
		for range 40 {
			link := path.Join(resolved...)
			fi, err := p.FS.Lstat(link)
			if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
				break
			}
			target, err := p.FS.ReadLink(link)
			if err != nil {
				break
			}
			resolved = resolved[:len(resolved)-1]
			for tc := range strings.SplitSeq(path.Clean(target), "/") {
				switch tc {
				case "", ".":
				case "..":
					if len(resolved) > 0 {
						resolved = resolved[:len(resolved)-1]
					}
				default:
					resolved = append(resolved, tc)
				}
			}
		}
	}
	if len(resolved) == 0 {
		return "."
	}
	return path.Join(resolved...)
}

// atime returns the access time of fi, falling back to its modification time.
func atime(fi fs.FileInfo) time.Time {
	if t, ok := fsx.Atime(fi); ok {
		return t
	}
	return fi.ModTime()
}

// chmodBits are the mode bits that Chmod changes, and specialBits the ones
// beyond the plain permissions.
const (
	chmodBits   = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky
	specialBits = fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky
)

// errNotDir is reported when a path component names something other than a
// directory.
var errNotDir = errors.New("not a directory")

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "install: "+format+"\n", args...)
}

// options holds the flags of install.
type options struct {
	compare       bool
	createParents bool
	directory     bool
	preserveTimes bool
	verbose       bool
	targetDirSet  bool
	targetDir     string
	noTarget      bool
	modeSet       bool
	mode          string
	ownerSet      bool
	owner         string
	groupSet      bool
	group         string
	backupSet     bool
	backupArg     string
	suffixSet     bool
	suffix        string
}

// longOptions maps the long options of install to the short ones they stand
// for.
var longOptions = map[string]rune{
	"backup":              'b',
	"compare":             'C',
	"directory":           'd',
	"group":               'g',
	"mode":                'm',
	"no-target-directory": 'T',
	"owner":               'o',
	"preserve-timestamps": 'p',
	"suffix":              'S',
	"target-directory":    't',
	"verbose":             'v',
}

// optSet describes the options of install. The -b option takes its argument
// only when attached to its long form, as in --backup=none.
var optSet = getopt.Set{
	Valued:   "Sgmot",
	Attached: "b",
	Long:     longOptions,
}

// usage describes the options of install.
const usage = `Usage: install [OPTION]... [-T] SOURCE DEST
  or:  install [OPTION]... SOURCE... DIRECTORY
  or:  install [OPTION]... -t DIRECTORY SOURCE...
  or:  install [OPTION]... -d DIRECTORY...
In the first three forms, copy SOURCE to DEST, or multiple SOURCE(s) to the
existing DIRECTORY, while setting permission modes and owner/group. In the
4th form, create all components of the given DIRECTORY(ies).

  -b, --backup[=METHOD]
         make a backup of each existing destination file
  -C, --compare
         compare each pair of source and destination files, and when neither
         the content, mode, owner nor group would change, leave the
         destination alone
  -c
         ignored
  -D
         create all leading components of DEST
  -d, --directory
         treat all arguments as directory names; create all components of the
         specified directories
  -g, --group=GROUP
         set group ownership, instead of the process's current group
  -m, --mode=MODE
         set permission mode (as in chmod), instead of rwxr-xr-x
  -o, --owner=OWNER
         set ownership (super-user only), instead of the process's current
         owner
  -p, --preserve-timestamps
         apply access/modification times of SOURCE files to corresponding
         destination files
  -S, --suffix=SUFFIX
         override the usual backup suffix
  -t, --target-directory=DIRECTORY
         copy all SOURCE arguments into DIRECTORY
  -T, --no-target-directory
         treat DEST as a normal file
  -v, --verbose
         print the name of each file as it is installed
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
// first operand or at "--", -t can't be combined with -T, with -d, nor
// given twice.
func parseFlags(args []string) (options, []string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'b':
			opts.backupSet = true
			opts.backupArg = value
		case 'C':
			opts.compare = true
		case 'c':
			// Ignored, for compatibility with old Unix versions.
		case 'D':
			opts.createParents = true
		case 'd':
			opts.directory = true
		case 'g':
			opts.groupSet, opts.group = true, value
		case 'm':
			opts.modeSet, opts.mode = true, value
		case 'o':
			opts.ownerSet, opts.owner = true, value
		case 'p':
			opts.preserveTimes = true
		case 'S':
			opts.suffixSet, opts.suffix = true, value
		case 't':
			if opts.targetDirSet {
				return errors.New("multiple target directories specified")
			}
			opts.targetDir, opts.targetDirSet = value, true
		case 'T':
			opts.noTarget = true
		case 'v':
			opts.verbose = true
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
	case opts.targetDirSet && opts.directory:
		return options{}, nil, errors.New("target directory not allowed when installing a directory")
	}
	return opts, operands, nil
}
