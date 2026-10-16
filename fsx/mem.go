package fsx

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// MemFS is an in-memory implementation of FS, meant mostly to write tests of
// code that uses this package.
//
// Files are seeded with a map literal, like in testing/fstest.MapFS:
//
//	fsys := fsx.MemFS{
//		"file.txt": {Data: []byte("contents")},
//	}
//
// Directories are implicit: a name of the form "dir/file" makes "dir"
// statable as a directory.
//
// A symbolic link is an entry with the fs.ModeSymlink bit set and the link
// target as its data, and is followed by the read operations, like Open and
// Stat. Link makes two names share the same entry, so writes through either
// are seen by the other.
type MemFS map[string]*MemFile

// MemFile describes a file of a MemFS. Its access time starts as the
// modification time, and is otherwise only changed by Chtimes.
type MemFile struct {
	Data    []byte
	Mode    fs.FileMode
	ModTime time.Time

	atime time.Time
}

// Open implements fs.FS.
func (m MemFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	p, err := m.resolve(name, true)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	if f, ok := m[p]; ok {
		return &memOpenFile{entry: f, path: name, read: !f.Mode.IsDir()}, nil
	}
	if m.isDir(p) {
		return &memOpenFile{entry: &MemFile{Mode: fs.ModeDir | 0o755}, path: name}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// OpenFile implements WriteFS.
func (m MemFS) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	p, err := m.resolve(name, true)
	if err != nil && (!errors.Is(err, fs.ErrNotExist) || flag&os.O_CREATE == 0) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	f, ok := m[p]
	if !ok {
		switch {
		case flag&os.O_CREATE == 0:
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		case m == nil:
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
		case m.isDir(p):
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
		case !m.isDir(path.Dir(p)):
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		now := time.Now()
		f = &MemFile{Mode: perm.Perm(), ModTime: now, atime: now}
		m[p] = f
	}
	if f.Mode.IsDir() {
		if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
		}
		return &memOpenFile{entry: f, path: name}, nil
	}
	if flag&os.O_CREATE != 0 && flag&os.O_EXCL != 0 {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrExist}
	}
	if flag&os.O_TRUNC != 0 {
		f.Data = nil
		f.ModTime = time.Now()
	}
	return &memOpenFile{
		entry:  f,
		path:   name,
		read:   flag&os.O_WRONLY == 0,
		write:  flag&(os.O_WRONLY|os.O_RDWR) != 0,
		append: flag&os.O_APPEND != 0,
	}, nil
}

// Chtimes implements ChtimesFS. Implicit directories are ignored.
func (m MemFS) Chtimes(name string, atime, mtime time.Time) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "chtimes", Path: name, Err: fs.ErrInvalid}
	}
	f, ok := m[name]
	if !ok {
		if m.isDir(name) {
			return nil
		}
		return &fs.PathError{Op: "chtimes", Path: name, Err: fs.ErrNotExist}
	}
	f.atime, f.ModTime = atime, mtime
	return nil
}

// chmodBits are the mode bits that Chmod can change.
const chmodBits = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// Chmod implements ChmodFS. Only the permission, set-user-ID, set-group-ID and
// sticky bits are changed; the file type is kept. Chmoding an implicit
// directory makes it an explicit entry.
func (m MemFS) Chmod(name string, mode fs.FileMode) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "chmod", Path: name, Err: fs.ErrInvalid}
	}
	if m == nil {
		return &fs.PathError{Op: "chmod", Path: name, Err: fs.ErrInvalid}
	}
	f, ok := m[name]
	if !ok {
		if !m.isDir(name) {
			return &fs.PathError{Op: "chmod", Path: name, Err: fs.ErrNotExist}
		}
		f = &MemFile{Mode: fs.ModeDir}
		m[name] = f
	}
	f.Mode = f.Mode.Type() | mode&chmodBits
	return nil
}

// Mkdir implements MkdirFS. Implicit directories, which exist only because
// of nested entries, also make it fail with fs.ErrExist.
func (m MemFS) Mkdir(name string, perm fs.FileMode) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrInvalid}
	}
	if m == nil {
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrInvalid}
	}
	if _, ok := m[name]; ok || m.isDir(name) {
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrExist}
	}
	if parent := path.Dir(name); parent != "." {
		if f, ok := m[parent]; ok && !f.Mode.IsDir() {
			return &fs.PathError{Op: "mkdir", Path: name, Err: errNotDir}
		}
		if !m.isDir(parent) {
			return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrNotExist}
		}
	}
	now := time.Now()
	m[name] = &MemFile{Mode: fs.ModeDir | perm.Perm(), ModTime: now, atime: now}
	return nil
}

// Remove implements RemoveFS. Directories must be empty, which implicit ones,
// existing only because of nested entries, never are. A final symbolic link
// is removed instead of what it points to, as with unlink(2).
func (m MemFS) Remove(name string) error {
	if !fs.ValidPath(name) || name == "." {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrInvalid}
	}
	p, err := m.resolve(name, false)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return &fs.PathError{Op: "remove", Path: name, Err: err}
	}
	if _, ok := m[p]; !ok {
		if m.isDir(p) {
			return &fs.PathError{Op: "remove", Path: name, Err: ErrNotEmpty}
		}
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrNotExist}
	}
	for nested := range m {
		if strings.HasPrefix(nested, p+"/") {
			return &fs.PathError{Op: "remove", Path: name, Err: ErrNotEmpty}
		}
	}
	delete(m, p)
	return nil
}

// Rename implements RenameFS. The old name must exist, and the parent of the
// new name must be an existing directory. An existing destination is
// replaced, unless it's a directory receiving a non-directory, a non-empty
// directory, or the directory is the source itself, mirroring rename(2).
func (m MemFS) Rename(oldName, newName string) error {
	if !fs.ValidPath(oldName) || oldName == "." {
		return &fs.PathError{Op: "rename", Path: oldName, Err: fs.ErrInvalid}
	}
	if !fs.ValidPath(newName) || newName == "." {
		return &fs.PathError{Op: "rename", Path: newName, Err: fs.ErrInvalid}
	}
	if oldName == newName {
		return nil
	}
	if m == nil {
		return &fs.PathError{Op: "rename", Path: oldName, Err: fs.ErrNotExist}
	}
	if _, ok := m[oldName]; !ok && !m.isDir(oldName) {
		return &fs.PathError{Op: "rename", Path: oldName, Err: fs.ErrNotExist}
	}
	if parent := path.Dir(newName); parent != "." {
		if f, ok := m[parent]; ok && !f.Mode.IsDir() {
			return &fs.PathError{Op: "rename", Path: newName, Err: errNotDir}
		}
		if !m.isDir(parent) {
			return &fs.PathError{Op: "rename", Path: newName, Err: fs.ErrNotExist}
		}
	}
	oldIsDir := m.isDir(oldName)
	if oldIsDir && strings.HasPrefix(newName, oldName+"/") {
		return &fs.PathError{Op: "rename", Path: newName, Err: errInSelf}
	}

	if _, exists := m[newName]; exists || m.isDir(newName) {
		newIsDir := m.isDir(newName)
		switch {
		case newIsDir && !oldIsDir:
			return &fs.PathError{Op: "rename", Path: newName, Err: errIsDir}
		case !newIsDir && oldIsDir:
			return &fs.PathError{Op: "rename", Path: newName, Err: errNotDir}
		case newIsDir:
			for nested := range m {
				if strings.HasPrefix(nested, newName+"/") {
					return &fs.PathError{Op: "rename", Path: newName, Err: ErrNotEmpty}
				}
			}
		}
		delete(m, newName)
	}

	for name := range m {
		if name == oldName || strings.HasPrefix(name, oldName+"/") {
			m[newName+name[len(oldName):]] = m[name]
			delete(m, name)
		}
	}
	return nil
}

// Link implements LinkFS. The new name shares the same entry as oldName, so
// writes through either name are seen by the other. A symbolic link oldName
// is linked as-is, without being followed, as with link(2).
func (m MemFS) Link(oldName, newName string) error {
	if !fs.ValidPath(oldName) || oldName == "." {
		return &fs.PathError{Op: "link", Path: oldName, Err: fs.ErrInvalid}
	}
	if !fs.ValidPath(newName) || newName == "." {
		return &fs.PathError{Op: "link", Path: newName, Err: fs.ErrInvalid}
	}
	old, err := m.resolve(oldName, false)
	if err != nil {
		return &fs.PathError{Op: "link", Path: oldName, Err: err}
	}
	new, err := m.resolve(newName, false)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return &fs.PathError{Op: "link", Path: newName, Err: err}
	}
	if m.isDir(old) {
		return &fs.PathError{Op: "link", Path: oldName, Err: fs.ErrPermission}
	}
	if _, ok := m[old]; !ok {
		return &fs.PathError{Op: "link", Path: oldName, Err: fs.ErrNotExist}
	}
	if _, ok := m[new]; ok || m.isDir(new) {
		return &fs.PathError{Op: "link", Path: newName, Err: fs.ErrExist}
	}
	if err := m.checkParent(new); err != nil {
		return &fs.PathError{Op: "link", Path: newName, Err: err}
	}
	m[new] = m[old]
	return nil
}

// Symlink implements SymlinkFS. oldName is stored as given and need not name
// an existing entry.
func (m MemFS) Symlink(oldName, newName string) error {
	if !fs.ValidPath(newName) || newName == "." {
		return &fs.PathError{Op: "symlink", Path: newName, Err: fs.ErrInvalid}
	}
	if m == nil {
		return &fs.PathError{Op: "symlink", Path: newName, Err: fs.ErrInvalid}
	}
	new, err := m.resolve(newName, false)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return &fs.PathError{Op: "symlink", Path: newName, Err: err}
	}
	if _, ok := m[new]; ok || m.isDir(new) {
		return &fs.PathError{Op: "symlink", Path: newName, Err: fs.ErrExist}
	}
	if err := m.checkParent(new); err != nil {
		return &fs.PathError{Op: "symlink", Path: newName, Err: err}
	}
	now := time.Now()
	m[new] = &MemFile{
		Data:    []byte(oldName),
		Mode:    fs.ModeSymlink | 0o777,
		ModTime: now,
		atime:   now,
	}
	return nil
}

// ReadLink implements fs.ReadLinkFS. It fails with fs.ErrInvalid when name
// isn't a symbolic link.
func (m MemFS) ReadLink(name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
	}
	p, err := m.resolve(name, false)
	if err != nil {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: err}
	}
	f, ok := m[p]
	if !ok {
		if m.isDir(p) {
			return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
		}
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrNotExist}
	}
	if f.Mode&fs.ModeSymlink == 0 {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
	}
	return string(f.Data), nil
}

// Lstat implements fs.ReadLinkFS. It describes the entry at name without
// following a final symbolic link, unlike Stat.
func (m MemFS) Lstat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrInvalid}
	}
	p, err := m.resolve(name, false)
	if err != nil {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: err}
	}
	if f, ok := m[p]; ok {
		return memFileInfo{entry: f, path: p}, nil
	}
	if m.isDir(p) {
		return memFileInfo{entry: &MemFile{Mode: fs.ModeDir | 0o755}, path: p}, nil
	}
	return nil, &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrNotExist}
}

// checkParent reports whether the parent of name is an existing directory.
func (m MemFS) checkParent(name string) error {
	parent := path.Dir(name)
	if parent == "." {
		return nil
	}
	if f, ok := m[parent]; ok && !f.Mode.IsDir() {
		return errNotDir
	}
	if !m.isDir(parent) {
		return fs.ErrNotExist
	}
	return nil
}

// errNotDir is returned when an operation expects a directory but the entry is
// not one.
var errNotDir = errors.New("not a directory")

// errIsDir is returned when an operation replaces a directory with a
// non-directory.
var errIsDir = errors.New("is a directory")

// errInSelf is returned when a rename moves a directory into itself.
var errInSelf = errors.New("cannot move directory into itself")

// errLoop is returned when following symbolic links doesn't reach an entry,
// like the ELOOP error of the real file systems.
var errLoop = errors.New("too many levels of symbolic links")

// maxSymlinks is the number of symbolic links that resolve follows before
// giving up.
const maxSymlinks = 40

// resolve returns the path of the entry that name designates, following
// symbolic links in every component, the last one only when followLast is
// true. When a link dangles, or an entry is missing, the path where the
// lookup stopped is returned together with an fs.ErrNotExist error, so that
// writers can create the target of a dangling link.
func (m MemFS) resolve(name string, followLast bool) (string, error) {
	dir, rest := ".", name
	links := 0
	for rest != "" {
		comp, tail, _ := strings.Cut(rest, "/")
		rest = tail
		if comp == "" || comp == "." {
			continue
		}
		cur := path.Join(dir, comp)
		f, ok := m[cur]
		switch {
		case ok && f.Mode&fs.ModeSymlink != 0:
			if rest == "" && !followLast {
				return cur, nil
			}
			if links++; links > maxSymlinks {
				return cur, errLoop
			}
			target := string(f.Data)
			if after, ok0 := strings.CutPrefix(target, "/"); ok0 {
				dir, target = ".", after
			}
			if rest == "" {
				rest = target
			} else {
				rest = strings.TrimSuffix(target, "/") + "/" + rest
			}
		case ok && rest == "":
			return cur, nil
		case ok && !f.Mode.IsDir():
			return cur + "/" + rest, errNotDir
		case ok:
			dir = cur
		case m.isDir(cur) && rest == "":
			return cur, nil
		case m.isDir(cur):
			dir = cur
		case rest == "":
			return cur, fs.ErrNotExist
		default:
			return cur + "/" + rest, fs.ErrNotExist
		}
	}
	return dir, nil
}

// ReadDir implements fs.ReadDirFS, so that directory listings and helpers from
// io/fs, like fs.WalkDir, work with a MemFS. Entries are sorted by name.
func (m MemFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	p, err := m.resolve(name, true)
	if err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
	}
	if f, ok := m[p]; ok && !f.Mode.IsDir() {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: errNotDir}
	}
	if !m.isDir(p) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	prefix := p + "/"
	if p == "." {
		prefix = ""
	}
	seen := make(map[string]bool)
	var entries []fs.DirEntry
	for nested := range m {
		rest, ok := strings.CutPrefix(nested, prefix)
		if !ok || rest == "" {
			continue
		}
		base, _, _ := strings.Cut(rest, "/")
		if seen[base] {
			continue
		}
		seen[base] = true

		child := path.Join(p, base)
		f, ok := m[child]
		if !ok {
			f = &MemFile{Mode: fs.ModeDir | 0o755}
		}
		entries = append(entries, memDirEntry{info: memFileInfo{entry: f, path: child}})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// isDir reports whether name is a directory in m, either because it holds an
// entry marked as one or because it's implied by a nested name.
func (m MemFS) isDir(name string) bool {
	if name == "." {
		return true
	}
	if f, ok := m[name]; ok {
		return f.Mode.IsDir()
	}
	for nested := range m {
		if strings.HasPrefix(nested, name+"/") {
			return true
		}
	}
	return false
}

// memOpenFile is an open file of a MemFS.
type memOpenFile struct {
	entry  *MemFile
	path   string
	offset int
	read   bool
	write  bool
	append bool
	closed bool
}

// Stat implements fs.File.
func (f *memOpenFile) Stat() (fs.FileInfo, error) {
	if f.closed {
		return nil, f.err("stat", fs.ErrClosed)
	}
	return memFileInfo{entry: f.entry, path: f.path}, nil
}

// Read implements fs.File.
func (f *memOpenFile) Read(p []byte) (int, error) {
	switch {
	case f.closed:
		return 0, f.err("read", fs.ErrClosed)
	case !f.read:
		return 0, f.err("read", fs.ErrInvalid)
	case f.offset >= len(f.entry.Data):
		return 0, io.EOF
	}
	n := copy(p, f.entry.Data[f.offset:])
	f.offset += n
	return n, nil
}

// Write implements io.Writer.
func (f *memOpenFile) Write(p []byte) (int, error) {
	switch {
	case f.closed:
		return 0, f.err("write", fs.ErrClosed)
	case !f.write:
		return 0, f.err("write", fs.ErrInvalid)
	}
	if f.append {
		f.offset = len(f.entry.Data)
	}
	end := f.offset + len(p)
	if end > len(f.entry.Data) {
		data := make([]byte, end)
		copy(data, f.entry.Data)
		f.entry.Data = data
	}
	copy(f.entry.Data[f.offset:], p)
	f.offset = end
	f.entry.ModTime = time.Now()
	return len(p), nil
}

// Close implements fs.File.
func (f *memOpenFile) Close() error {
	if f.closed {
		return f.err("close", fs.ErrClosed)
	}
	f.closed = true
	return nil
}

// Seek implements io.Seeker.
func (f *memOpenFile) Seek(offset int64, whence int) (int64, error) {
	if f.closed {
		return 0, f.err("seek", fs.ErrClosed)
	}
	var base int64
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = int64(f.offset)
	case io.SeekEnd:
		base = int64(len(f.entry.Data))
	default:
		return 0, f.err("seek", fs.ErrInvalid)
	}
	pos := base + offset
	if pos < 0 {
		return 0, f.err("seek", fs.ErrInvalid)
	}
	f.offset = int(pos)
	return pos, nil
}

// err returns a path error for an operation on f.
func (f *memOpenFile) err(op string, err error) error {
	return &fs.PathError{Op: op, Path: f.path, Err: err}
}

// memFileInfo implements fs.FileInfo for a MemFS file.
type memFileInfo struct {
	entry *MemFile
	path  string
}

func (fi memFileInfo) Name() string       { return path.Base(fi.path) }
func (fi memFileInfo) Size() int64        { return int64(len(fi.entry.Data)) }
func (fi memFileInfo) Mode() fs.FileMode  { return fi.entry.Mode }
func (fi memFileInfo) ModTime() time.Time { return fi.entry.ModTime }
func (fi memFileInfo) IsDir() bool        { return fi.entry.Mode.IsDir() }
func (fi memFileInfo) Sys() any           { return fi.entry }

// memDirEntry implements fs.DirEntry for a MemFS entry.
type memDirEntry struct {
	info memFileInfo
}

func (e memDirEntry) Name() string               { return e.info.Name() }
func (e memDirEntry) IsDir() bool                { return e.info.IsDir() }
func (e memDirEntry) Type() fs.FileMode          { return e.info.Mode().Type() }
func (e memDirEntry) Info() (fs.FileInfo, error) { return e.info, nil }

var (
	_ FS            = MemFS{}
	_ fs.FS         = MemFS{}
	_ fs.ReadDirFS  = MemFS{}
	_ fs.ReadLinkFS = MemFS{}
	_ File          = (*memOpenFile)(nil)
	_ SeekFile      = (*memOpenFile)(nil)
	_ fs.File       = (*memOpenFile)(nil)
	_ fs.DirEntry   = memDirEntry{}
)
