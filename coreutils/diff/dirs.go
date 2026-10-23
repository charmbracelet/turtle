package diff

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// compareDirs compares the entries of both directory operands: names that
// exist in only one directory are reported, common subdirectories are
// noted, and common regular files are compared after a header naming them.
func (p *Prog) compareDirs(ctx context.Context, d1, d2 operand, opts options) (bool, error) {
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
		if _, ok := entries2[name]; !ok {
			if err := p.write(fmt.Sprintf("Only in %s: %s\n", d1.name, name)); err != nil {
				return false, err
			}
			differ = true
			continue
		}
		if _, ok := entries1[name]; !ok {
			if err := p.write(fmt.Sprintf("Only in %s: %s\n", d2.name, name)); err != nil {
				return false, err
			}
			differ = true
			continue
		}
		d, err := p.compareEntry(ctx, d1, d2, name, opts)
		if err != nil {
			return differ, err
		}
		differ = differ || d
	}
	return differ, nil
}

// compareEntry compares the entries of both directories that share a name,
// following symbolic links to their targets.
func (p *Prog) compareEntry(ctx context.Context, d1, d2 operand, name string, opts options) (bool, error) {
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

// readDir lists the entries of the directory operand.
func (p *Prog) readDir(d operand) (map[string]fs.DirEntry, error) {
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
// under directories, holding the options as they'd be given on the command
// line.
func (o options) header(name1, name2 string) string {
	var b strings.Builder
	b.WriteString("diff")
	if o.blank {
		b.WriteString(" -b")
	}
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
