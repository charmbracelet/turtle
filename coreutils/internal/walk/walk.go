// Package walk traverses the file hierarchies below given starting points,
// following symbolic links as its options ask for.
package walk

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"charm.land/snail/coreutils/internal/fileinfo"
)

// Options holds how a traversal walks the hierarchy below a starting point.
// The zero value never follows symbolic links and descends the whole tree.
type Options struct {
	// Follow tells how symbolic links are followed: 'P' or zero, the
	// default, never follows them; 'H' follows the starting points only;
	// 'L' follows every one. A link whose target can't be reached is
	// described as itself, unless the target doesn't exist, in which case
	// the link is.
	Follow byte

	// Depth visits the entries of a directory before the directory itself.
	// The -prune decision of the visit function is ignored then, as
	// descending happens first.
	Depth bool

	// MaxDepth limits how many levels of directories are descended below
	// each starting point. Negative, the default, means no limit, and zero
	// means the starting point alone.
	MaxDepth int

	// MinDepth skips the files at levels below it.
	MinDepth int

	// XDev doesn't descend into the directories of another file system.
	XDev bool

	// Mount skips the files of another file system entirely, and doesn't
	// descend into their directories either.
	Mount bool

	// OnError reports an error met on a file of the traversal. The error it
	// returns aborts the traversal, as the context cancellation does.
	OnError func(name string, err error) error

	// SkipEntry reports whether an entry of a directory, identified by the
	// type its directory read reports alone, is left out of the traversal:
	// it is neither stat'ed nor visited, and no error of it is reported.
	// Only the entries below a starting point go through it; the starting
	// point itself is always stat'ed as the options ask.
	//
	// When it's set, the traversal of directories follows the fs.WalkDir
	// flow on errors as well: a directory entry whose stat fails is still
	// entered, so the failure of reading it is reported too.
	SkipEntry func(d fs.DirEntry) bool
}

// VisitFunc is called for every file of a traversal. name is the pathname
// as the traversal builds it from the starting point, fsName the one the
// file system is asked for, and depth the number of levels below the
// starting point. Returning false keeps the traversal from descending into
// a directory.
type VisitFunc func(name, fsName string, fi fs.FileInfo, depth int) (descend bool, err error)

// Walk traverses the file hierarchy below root, which the file system knows
// as fsName, calling visit on every file as the options ask for. The
// starting point itself is visited too, at depth zero.
func Walk(ctx context.Context, fsys fs.FS, opts Options, root, fsName string, visit VisitFunc) error {
	fi, err := Stat(fsys, opts.Follow, fsName, true)
	if err != nil {
		return opts.onError(root, err)
	}
	st, _ := fileinfo.Stat(fi)
	w := &walker{fsys: fsys, opts: opts, visit: visit, loops: map[string]string{}, descend: true}
	return w.walk(ctx, root, fsName, fi, st.Dev, 0)
}

// Stat describes one file, following symbolic links as follow asks for.
// A link whose target doesn't exist is described as itself, while any other
// failure to reach the target, like a symlink loop, is reported.
func Stat(fsys fs.FS, follow byte, name string, operand bool) (fs.FileInfo, error) {
	if follow == 'L' || operand && follow == 'H' {
		fi, err := fs.Stat(fsys, name)
		if err == nil {
			return fi, nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			if lfi, lerr := fs.Lstat(fsys, name); lerr == nil {
				return lfi, nil
			}
		}
		return nil, err
	}
	return fs.Lstat(fsys, name)
}

// DirError reports whether err is the failure of reading the entries of
// the directory name, with the message of the diagnostic the walking
// utilities write for such a failure.
func DirError(name string, err error) (msg string, isDirErr bool) {
	perr, ok := errors.AsType[*fs.PathError](err)
	if !ok || perr.Op != "readdirent" && perr.Op != "readdir" {
		return "", false
	}
	return fmt.Sprintf("cannot read directory '%s': %v", name, perr.Err), true
}

// walker holds the state of one traversal.
type walker struct {
	fsys    fs.FS
	opts    Options
	visit   VisitFunc
	loops   map[string]string
	descend bool // the decision of the last visit
}

// walk visits one file and, when it's a directory that isn't pruned, the
// files below it.
func (w *walker) walk(ctx context.Context, name, fsName string, fi fs.FileInfo, rootDev uint64, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	st, stOK := fileinfo.Stat(fi)
	offRoot := stOK && st.Dev != rootDev
	// -mount doesn't act on the files of another file system, and with -xdev
	// it still applies, as both together behave like -mount alone.
	if w.opts.Mount && offRoot {
		return nil
	}
	descendable := fi.IsDir() && !(w.opts.XDev && offRoot) &&
		!(w.opts.MaxDepth >= 0 && depth+1 > w.opts.MaxDepth)
	if w.opts.Depth {
		// The entries of a directory come before the directory itself, and
		// no -prune keeps the traversal out of it.
		if descendable {
			if err := w.descendDir(ctx, name, fsName, fi, rootDev, depth); err != nil {
				return err
			}
		}
		w.descend = true
		return w.visitFile(name, fsName, fi, depth)
	}
	w.descend = true
	if err := w.visitFile(name, fsName, fi, depth); err != nil {
		return err
	}
	if !w.descend || !descendable {
		return nil
	}
	return w.descendDir(ctx, name, fsName, fi, rootDev, depth)
}

// visitFile calls the visit function of a file, unless its depth is below
// the minimum one.
func (w *walker) visitFile(name, fsName string, fi fs.FileInfo, depth int) error {
	if depth < w.opts.MinDepth {
		return nil
	}
	descend, err := w.visit(name, fsName, fi, depth)
	if err != nil {
		return err
	}
	w.descend = descend
	return nil
}

// descendDir walks the entries of one directory.
func (w *walker) descendDir(ctx context.Context, dir, fsDir string, fi fs.FileInfo, rootDev uint64, depth int) error {
	if w.opts.Follow == 'L' {
		// Only the directories of the current chain of ancestors count: a
		// directory met again once its ancestor is left is a revisit, not a
		// loop.
		if id, ok := DirID(fi); ok {
			if first, seen := w.loops[id]; seen {
				return w.opts.onError(dir, fmt.Errorf(
					"file system loop detected; '%s' is part of the same file system loop as '%s'",
					dir, first))
			}
			w.loops[id] = dir
			defer delete(w.loops, id)
		}
	}
	entries, err := fs.ReadDir(w.fsys, fsDir)
	if err != nil {
		return w.opts.onError(dir, err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		child := Join(dir, entry.Name())
		fsChild := path.Join(fsDir, entry.Name())
		if w.opts.SkipEntry != nil && w.opts.SkipEntry(entry) {
			continue
		}
		cfi, err := Stat(w.fsys, w.opts.Follow, fsChild, false)
		if err != nil {
			if err := w.opts.onError(child, err); err != nil {
				return err
			}
			if w.opts.SkipEntry != nil && entry.IsDir() {
				// As fs.WalkDir's traversal does, a directory entry whose
				// stat failed is still entered, so the failure of reading
				// it surfaces too.
				if _, err := fs.ReadDir(w.fsys, fsChild); err != nil {
					if err := w.opts.onError(child, err); err != nil {
						return err
					}
				}
			}
			continue
		}
		if err := w.walk(ctx, child, fsChild, cfi, rootDev, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// onError reports an error, reporting none when no handler is given.
func (o Options) onError(name string, err error) error {
	if o.OnError == nil {
		return nil
	}
	return o.OnError(name, err)
}

// Join concatenates a directory and an entry name, keeping a single slash
// between them no matter how the directory was given.
func Join(dir, name string) string {
	if len(dir) > 0 && dir[len(dir)-1] == '/' {
		return dir + name
	}
	return dir + "/" + name
}

// DirID returns what identifies a directory for loop detection, reporting
// false when the file system reports nothing to identify it with.
func DirID(fi fs.FileInfo) (string, bool) {
	st, ok := fileinfo.Stat(fi)
	if !ok || st.Dev == 0 && st.Ino == 0 {
		return "", false
	}
	return fmt.Sprintf("%d:%d", st.Dev, st.Ino), true
}

// ResolveLink follows the chain of symbolic links at name, returning the
// path the last one points at. A name which is not a symbolic link, or a
// broken or looping chain, is returned as it is.
func ResolveLink(fsys fs.ReadLinkFS, name string) string {
	for range 40 {
		fi, err := fsys.Lstat(name)
		if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
			return name
		}
		target, err := fsys.ReadLink(name)
		if err != nil {
			return name
		}
		name = path.Join(path.Dir(name), target)
	}
	return name
}
