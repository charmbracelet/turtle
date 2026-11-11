package diff

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
)

// dirPair holds the identity of a compared directory pair, used to detect
// the loops that symbolic links to ancestors cause when comparing
// recursively.
type dirPair struct {
	fi1, fi2 os.FileInfo
}

// compareDirs compares the entries of both directory operands: names that
// exist in only one directory are reported, or compared against an empty
// file with the new file options; common subdirectories are compared with
// the recursive option or noted otherwise; and common regular files are
// compared after a header naming them.
func (p *Prog) compareDirs(ctx context.Context, d1, d2 operand, opts options, seen []dirPair) (bool, error) {
	if opts.format == ifdefFormat {
		return false, errIfdefDirs
	}
	fi1, fi2 := p.statDir(d1), p.statDir(d2)
	for _, v := range seen {
		if os.SameFile(v.fi1, fi1) && os.SameFile(v.fi2, fi2) {
			return false, &pathError{label: d1.name, err: errors.New("recursive directory loop detected")}
		}
	}
	seen = append(seen, dirPair{fi1: fi1, fi2: fi2})

	entries1, err := p.readDir(d1)
	if err != nil {
		return false, &pathError{label: d1.name, err: err}
	}
	entries2, err := p.readDir(d2)
	if err != nil {
		return false, &pathError{label: d2.name, err: err}
	}

	differ := false
	for _, name := range unionNames(entries1, entries2) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if opts.excluded(name) {
			continue
		}
		_, in1 := entries1[name]
		_, in2 := entries2[name]
		switch {
		case in1 && in2:
			d, err := p.compareEntry(ctx, d1, d2, name, opts, seen)
			if err != nil {
				return differ, err
			}
			differ = differ || d
		case in1:
			// Only in the first directory: with -N it's compared
			// against a missing second file, with -P only when the
			// missing one is the first.
			if opts.newFile {
				d, err := p.compareMissing(ctx, d1, d2, name, true, opts, seen)
				if err != nil {
					return differ, err
				}
				differ = differ || d
				continue
			}
			if err := p.write(fmt.Sprintf("Only in %s: %s\n", d1.name, name)); err != nil {
				return false, err
			}
			differ = true
		default:
			if opts.newFile || opts.unidirectional {
				d, err := p.compareMissing(ctx, d1, d2, name, false, opts, seen)
				if err != nil {
					return differ, err
				}
				differ = differ || d
				continue
			}
			if err := p.write(fmt.Sprintf("Only in %s: %s\n", d2.name, name)); err != nil {
				return false, err
			}
			differ = true
		}
	}
	return differ, nil
}

// compareMissing compares the entry name of the directory it's in against
// the file it would be in the other one, which the new file options treat
// as an empty file: a regular file against an empty one, and with the
// recursive option a directory against an empty one, so only its names
// come out as additions. Without it, a subdirectory is still only
// reported.
func (p *Prog) compareMissing(ctx context.Context, d1, d2 operand, name string, inFirst bool, opts options, seen []dirPair) (bool, error) {
	present, missing := d2, d1
	if inFirst {
		present, missing = d1, d2
	}
	sub := operand{
		name:    path.Join(missing.name, name),
		path:    path.Join(missing.path, name),
		missing: true,
	}
	fi, err := fs.Stat(p.FS, path.Join(present.path, name))
	if err != nil {
		return false, &pathError{label: path.Join(present.name, name), err: err}
	}
	if fi.IsDir() {
		if !opts.recursive {
			dir := d1
			if !inFirst {
				dir = d2
			}
			return true, p.write(fmt.Sprintf("Only in %s: %s\n", dir.name, name))
		}
		other := operand{name: path.Join(present.name, name), path: path.Join(present.path, name), isDir: true}
		if inFirst {
			return p.compareDirs(ctx, other, sub, opts, seen)
		}
		return p.compareDirs(ctx, sub, other, opts, seen)
	}
	pr := pair{
		name1:    path.Join(d1.name, name),
		path1:    path.Join(d1.path, name),
		missing1: !inFirst,
		name2:    path.Join(d2.name, name),
		path2:    path.Join(d2.path, name),
		missing2: inFirst,
	}
	pr.header = opts.header(path.Join(d1.name, name), path.Join(d2.name, name))
	return p.compareFiles(ctx, pr, opts)
}

// compareEntry compares the entries of both directories that share a name,
// following symbolic links to their targets.
func (p *Prog) compareEntry(ctx context.Context, d1, d2 operand, name string, opts options, seen []dirPair) (bool, error) {
	t1, err := p.fileType(path.Join(d1.path, name))
	if err != nil {
		return false, &pathError{label: path.Join(d1.name, name), err: err}
	}
	t2, err := p.fileType(path.Join(d2.path, name))
	if err != nil {
		return false, &pathError{label: path.Join(d2.name, name), err: err}
	}

	switch {
	case t1.IsDir() && t2.IsDir():
		sub1 := operand{name: path.Join(d1.name, name), path: path.Join(d1.path, name), isDir: true}
		sub2 := operand{name: path.Join(d2.name, name), path: path.Join(d2.path, name), isDir: true}
		if opts.recursive {
			return p.compareDirs(ctx, sub1, sub2, opts, seen)
		}
		return false, p.write(fmt.Sprintf("Common subdirectories: %s and %s\n", sub1.name, sub2.name))
	case t1.IsRegular() && t2.IsRegular():
		return p.compareFiles(ctx, pair{
			name1:  path.Join(d1.name, name),
			path1:  path.Join(d1.path, name),
			name2:  path.Join(d2.name, name),
			path2:  path.Join(d2.path, name),
			header: opts.header(path.Join(d1.name, name), path.Join(d2.name, name)),
		}, opts)
	case t1 == t2:
		// File types diff can't compare, like FIFOs: nothing to report.
		return false, nil
	default:
		return true, p.write(fmt.Sprintf("File %s is a %s while file %s is a %s\n",
			path.Join(d1.name, name), typeName(t1), path.Join(d2.name, name), typeName(t2)))
	}
}

// statDir stats the directory operand, returning nil when it can't be
// reached, in which case loop detection just can't tell.
func (p *Prog) statDir(d operand) os.FileInfo {
	if d.missing {
		return nil
	}
	fi, err := fs.Stat(p.FS, d.path)
	if err != nil {
		return nil
	}
	return fi
}

// readDir lists the entries of the directory operand, with a missing
// directory holding none, as the new file options compare it.
func (p *Prog) readDir(d operand) (map[string]fs.DirEntry, error) {
	if d.missing {
		return map[string]fs.DirEntry{}, nil
	}
	entries, err := fs.ReadDir(p.FS, d.path)
	if err != nil {
		return nil, err
	}
	m := make(map[string]fs.DirEntry, len(entries))
	for _, e := range entries {
		m[e.Name()] = e
	}
	return m, nil
}

// fileType classifies the file at the path by its type bits, following
// symbolic links.
func (p *Prog) fileType(path string) (fs.FileMode, error) {
	fi, err := fs.Stat(p.FS, path)
	if err != nil {
		return 0, err
	}
	return fi.Mode().Type(), nil
}

// header returns the line written before the changes of files compared
// under directories, holding the command line as it was given.
func (o options) header(name1, name2 string) string {
	var b strings.Builder
	b.WriteString(o.cmdline)
	fmt.Fprintf(&b, " %s %s", name1, name2)
	return b.String()
}

// unionNames returns the sorted names of the entries of both directories.
func unionNames(e1, e2 map[string]fs.DirEntry) []string {
	names := make([]string, 0, len(e1)+len(e2))
	for name := range e1 {
		names = append(names, name)
	}
	for name := range e2 {
		if _, ok := e1[name]; !ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// typeName names a file type for the messages of directory comparisons.
func typeName(t fs.FileMode) string {
	switch {
	case t.IsDir():
		return "directory"
	case t.IsRegular():
		return "regular file"
	case t&fs.ModeCharDevice != 0:
		return "character device"
	case t&fs.ModeDevice != 0:
		return "block device"
	case t&fs.ModeNamedPipe != 0:
		return "fifo"
	case t&fs.ModeSocket != 0:
		return "socket"
	default:
		return "irregular file"
	}
}
