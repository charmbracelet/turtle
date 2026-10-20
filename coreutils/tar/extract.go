package tar

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"charm.land/snail/fsx"
)

// extract restores the entries of the archive into the file system, filtered
// by the operands when given.
func (a *archiver) extract(ctx context.Context, operands []string) error {
	in, err := a.openReader()
	if err != nil {
		a.fail(a.archiveName(), err)
		return a.exit(1)
	}
	defer in.Close()
	tr, err := newTarReader(in)
	if err != nil {
		a.fail(a.archiveName(), err)
		return a.exit(1)
	}
	matched := matchTracker(operands)
	var dirs []dirFix
	var links []pendingLink
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			a.fail(a.archiveName(), errNotTar)
			break
		}
		name, err := a.extractName(hdr.Name)
		if err != nil {
			a.fail(hdr.Name, err)
			continue
		}
		if name == "" || !nameMatches(operands, hdr.Name) {
			continue
		}
		markMatched(matched, hdr.Name)
		if a.opts.interactive && !a.confirm("extract", hdr.Name) {
			continue
		}
		if a.opts.verbose {
			a.errorf("%s", hdr.Name)
		}
		if err := a.extractEntry(ctx, tr, name, hdr, &dirs, &links); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			a.fail(hdr.Name, err)
		}
	}
	a.retryLinks(links)
	for _, dir := range slices.Backward(dirs) {
		a.applyDirFix(dir)
	}
	a.reportMissing(operands, matched)
	return a.exit(1)
}

// extractName converts the name of an entry into a safe relative path:
// leading slashes are dropped and any ".." component refuses the entry, so
// an archive can't write outside the working directory. The empty name, for
// the root itself, is skipped.
func (a *archiver) extractName(name string) (string, error) {
	name = a.relativeName(name)
	if slices.Contains(strings.Split(name, "/"), "..") {
		return "", errors.New("Member name contains '..'")
	}
	name = path.Clean(name)
	if name == "." {
		return "", nil
	}
	return name, nil
}

// dirFix is the mode and modification time of an extracted directory,
// applied only after the whole archive is read so the permissions of a
// directory can't keep its own children out.
type dirFix struct {
	name  string
	mode  fs.FileMode
	mtime time.Time
}

// pendingLink is a hard link whose target didn't exist when it was
// extracted, retried after the whole archive is read.
type pendingLink struct {
	target, name string
	err          error
}

// extractEntry restores a single entry of the archive.
func (a *archiver) extractEntry(ctx context.Context, tr *tar.Reader, name string, hdr *tar.Header, dirs *[]dirFix, links *[]pendingLink) error {
	if a.rt.FS == nil {
		return errors.New("no file system available")
	}
	resolved := a.resolve(name)
	parent := path.Dir(resolved)
	switch hdr.Typeflag {
	case tar.TypeDir:
		mode := a.entryMode(hdr)
		if err := mkdirAll(a.rt.FS, resolved, mode|0o700); err != nil {
			return err
		}
		*dirs = append(*dirs, dirFix{resolved, mode, hdr.ModTime})
	case tar.TypeReg:
		if err := mkdirAll(a.rt.FS, parent, 0o777&^a.rt.Umask.Perm()); err != nil {
			return err
		}
		mode := a.entryMode(hdr)
		out, err := a.rt.FS.OpenFile(resolved, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode|0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, ctxReader{ctx: ctx, r: tr})
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if err := a.rt.FS.Chmod(resolved, mode); err != nil {
			return err
		}
		if err := a.restoreTimes(resolved, hdr); err != nil {
			return err
		}
	case tar.TypeSymlink:
		if err := mkdirAll(a.rt.FS, parent, 0o777&^a.rt.Umask.Perm()); err != nil {
			return err
		}
		if err := removeExisting(a.rt.FS, resolved); err != nil {
			return err
		}
		if err := a.rt.FS.Symlink(hdr.Linkname, resolved); err != nil {
			return err
		}
	case tar.TypeLink:
		target, err := a.extractName(hdr.Linkname)
		if err != nil {
			return err
		}
		if target == "" {
			return errors.New("empty hard link target")
		}
		if err := mkdirAll(a.rt.FS, parent, 0o777&^a.rt.Umask.Perm()); err != nil {
			return err
		}
		if err := removeExisting(a.rt.FS, resolved); err != nil {
			return err
		}
		if err := a.rt.FS.Link(a.resolve(target), resolved); err != nil {
			*links = append(*links, pendingLink{a.resolve(target), resolved, err})
		}
	case tar.TypeFifo, tar.TypeChar, tar.TypeBlock:
		return errors.New("cannot create special files")
	default:
		return fmt.Errorf("unsupported archive entry type %q", rune(hdr.Typeflag))
	}
	return nil
}

// retryLinks creates the hard links whose targets weren't extracted yet,
// reporting the ones which still fail.
func (a *archiver) retryLinks(links []pendingLink) {
	for len(links) > 0 {
		var pending []pendingLink
		progress := false
		for _, l := range links {
			if err := a.rt.FS.Link(l.target, l.name); err != nil {
				l.err = err
				pending = append(pending, l)
				continue
			}
			progress = true
		}
		if !progress {
			for _, l := range pending {
				a.fail(l.name, l.err)
			}
			return
		}
		links = pending
	}
}

// applyDirFix restores the mode and modification time of a directory.
func (a *archiver) applyDirFix(d dirFix) {
	if err := a.rt.FS.Chmod(d.name, d.mode); err != nil {
		a.fail(d.name, err)
		return
	}
	if a.opts.touch {
		return
	}
	if err := a.rt.FS.Chtimes(d.name, d.mtime, d.mtime); err != nil {
		a.fail(d.name, err)
	}
}

// restoreTimes sets the times of an extracted entry, unless -m was given.
func (a *archiver) restoreTimes(name string, hdr *tar.Header) error {
	if a.opts.touch {
		return nil
	}
	atime := hdr.AccessTime
	if atime.IsZero() {
		atime = hdr.ModTime
	}
	return a.rt.FS.Chtimes(name, atime, hdr.ModTime)
}

// entryMode returns the mode an extracted entry gets: exact with -p, or
// without the bits masked by the umask otherwise.
func (a *archiver) entryMode(hdr *tar.Header) fs.FileMode {
	mode := fs.FileMode(hdr.Mode) & fs.ModePerm
	if !a.opts.preserve {
		return mode &^ a.rt.Umask.Perm()
	}
	if hdr.Mode&0o4000 != 0 {
		mode |= fs.ModeSetuid
	}
	if hdr.Mode&0o2000 != 0 {
		mode |= fs.ModeSetgid
	}
	if hdr.Mode&0o1000 != 0 {
		mode |= fs.ModeSticky
	}
	return mode
}

// mkdirAll creates a directory and its missing parents, like mkdir -p.
func mkdirAll(fsys fsx.FS, name string, perm fs.FileMode) error {
	if name == "" || name == "." {
		return nil
	}
	fi, err := fsys.Lstat(name)
	if err == nil {
		if !fi.IsDir() {
			return errors.New("not a directory")
		}
		return nil
	}
	if err := mkdirAll(fsys, path.Dir(name), perm); err != nil {
		return err
	}
	if err := fsys.Mkdir(name, perm); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return nil
}

// removeExisting removes a path an extracted entry replaces.
func removeExisting(fsys fsx.FS, name string) error {
	if err := fsys.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
