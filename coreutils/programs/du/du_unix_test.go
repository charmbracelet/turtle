//go:build unix && !aix && !hurd

package du

import (
	"io/fs"
	"syscall"
	"testing"

	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// statFS describes the files of a memory file system with the identity a
// real file system reports, so the tests can cover -x and the counting of
// files that occur more than once.
type statFS struct {
	fsx.MemoryFS
	stats map[*fsx.MemoryFile]*syscall.Stat_t
}

// statInfo is a FileInfo whose Sys value holds the identity injected.
type statInfo struct {
	fs.FileInfo
	st *syscall.Stat_t
}

// Sys returns the injected identity.
func (i statInfo) Sys() any { return i.st }

// wrap adds the identity of a file, when one was injected for it.
func (s statFS) wrap(fi fs.FileInfo) fs.FileInfo {
	if entry, ok := fi.Sys().(*fsx.MemoryFile); ok {
		if st, found := s.stats[entry]; found {
			return statInfo{FileInfo: fi, st: st}
		}
	}
	return fi
}

// Lstat describes the named file without following a final symbolic link,
// with the identity injected.
func (s statFS) Lstat(name string) (fs.FileInfo, error) {
	fi, err := s.MemoryFS.Lstat(name)
	if err != nil {
		return nil, err
	}
	return s.wrap(fi), nil
}

// Stat describes the named file, following symbolic links, with the identity
// injected.
func (s statFS) Stat(name string) (fs.FileInfo, error) {
	fi, err := fs.Stat(s.MemoryFS, name)
	if err != nil {
		return nil, err
	}
	return s.wrap(fi), nil
}

// hardLinked is a hierarchy whose file a is hard linked twice, identified as
// a real file system would.
func hardLinked() statFS {
	files := fsx.MemoryFS{"tree/a": {Data: make([]byte, 600)}}
	if err := files.Link("tree/a", "tree/b"); err != nil {
		panic(err)
	}
	files["tree/c"] = &fsx.MemoryFile{Data: make([]byte, 600)}
	return statFS{
		MemoryFS: files,
		stats: map[*fsx.MemoryFile]*syscall.Stat_t{
			files["tree/a"]: {Dev: 1, Ino: 5, Nlink: 2, Blocks: 2},
			files["tree/c"]: {Dev: 1, Ino: 6, Nlink: 1, Blocks: 2},
		},
	}
}

func TestRunHardLinks(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "a file with two links is counted once",
			FS:   hardLinked(),
			Args: []string{"tree"},
			Want: "4 tree\n",
		},
		{
			Name: "-a writes only the first occurrence",
			FS:   hardLinked(),
			Args: []string{"-a", "tree"},
			Want: "2 tree/a\n2 tree/c\n4 tree\n",
		},
		{
			Name: "the occurrences of an operand share the count",
			FS:   hardLinked(),
			Args: []string{"tree/a", "tree/b"},
			Want: "2 tree/a\n",
		},
		{
			Name: "-c totals the occurrences once",
			FS:   hardLinked(),
			Args: []string{"-c", "tree/a", "tree/b"},
			Want: "2 tree/a\n2 total\n",
		},
	})
}

func TestRunDevices(t *testing.T) {
	files := fsx.MemoryFS{
		"tree":       {Mode: fs.ModeDir | 0o755},
		"tree/a":     {Data: make([]byte, 600)},
		"tree/mnt":   {Mode: fs.ModeDir | 0o755},
		"tree/mnt/x": {Data: make([]byte, 2000)},
	}
	sys := statFS{
		MemoryFS: files,
		stats: map[*fsx.MemoryFile]*syscall.Stat_t{
			files["tree"]:       {Dev: 1, Ino: 2, Nlink: 3},
			files["tree/a"]:     {Dev: 1, Ino: 7, Nlink: 1, Blocks: 2},
			files["tree/mnt"]:   {Dev: 2, Ino: 3, Nlink: 2, Blocks: 1},
			files["tree/mnt/x"]: {Dev: 2, Ino: 8, Nlink: 1, Blocks: 4},
		},
	}
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "a hierarchy holding another file system",
			FS:   sys,
			Args: []string{"tree"},
			Want: "5 tree/mnt\n7 tree\n",
		},
		{
			Name: "-x skips the directories of another file system",
			FS:   sys,
			Args: []string{"-x", "tree"},
			Want: "1 tree/mnt\n3 tree\n",
		},
		{
			Name: "-x keeps the operands on their own file system",
			FS:   sys,
			Args: []string{"-x", "tree/mnt"},
			Want: "5 tree/mnt\n",
		},
	})
}

// identified is a hierarchy of two files and a subdirectory holding one,
// whose entries are identified the way a real file system does.
func identified() statFS {
	files := fsx.MemoryFS{
		"tree":       {Mode: fs.ModeDir | 0o755},
		"tree/a":     {Data: make([]byte, 600)},
		"tree/b":     {Data: make([]byte, 100)},
		"tree/sub":   {Mode: fs.ModeDir | 0o755},
		"tree/sub/c": {Data: make([]byte, 2000)},
	}
	return statFS{
		MemoryFS: files,
		stats: map[*fsx.MemoryFile]*syscall.Stat_t{
			files["tree"]:       {Dev: 1, Ino: 2, Nlink: 2},
			files["tree/a"]:     {Dev: 1, Ino: 7, Nlink: 1, Blocks: 2},
			files["tree/b"]:     {Dev: 1, Ino: 9, Nlink: 1, Blocks: 1},
			files["tree/sub"]:   {Dev: 1, Ino: 3, Nlink: 2},
			files["tree/sub/c"]: {Dev: 1, Ino: 8, Nlink: 1, Blocks: 4},
		},
	}
}

// reachable holds two symbolic links to the same directory, for the -L
// traversal of a directory that occurs twice in one hierarchy.
func reachable() statFS {
	files := fsx.MemoryFS{
		"d":       {Mode: fs.ModeDir | 0o755},
		"d/p1":    link("../point"),
		"d/p2":    link("../point"),
		"point":   {Mode: fs.ModeDir | 0o755},
		"point/x": {Data: make([]byte, 2000)},
	}
	return statFS{
		MemoryFS: files,
		stats: map[*fsx.MemoryFile]*syscall.Stat_t{
			files["d"]:       {Dev: 1, Ino: 11, Nlink: 2},
			files["point"]:   {Dev: 1, Ino: 12, Nlink: 2},
			files["point/x"]: {Dev: 1, Ino: 13, Nlink: 1, Blocks: 4},
		},
	}
}

func TestRunDuplicateDirectories(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "a repeated directory operand is written once",
			FS:   identified(),
			Args: []string{"tree", "tree"},
			Want: "4 tree/sub\n7 tree\n",
		},
		{
			Name: "an operand below another one is written once",
			FS:   identified(),
			Args: []string{"tree", "tree/sub"},
			Want: "4 tree/sub\n7 tree\n",
		},
		{
			Name: "a repeated operand is counted once in the total",
			FS:   identified(),
			Args: []string{"-c", "tree", "tree"},
			Want: "4 tree/sub\n7 tree\n7 total\n",
		},
		{
			Name: "-a writes the hierarchy of repeated operands once",
			FS:   identified(),
			Args: []string{"-a", "tree", "tree"},
			Want: "2 tree/a\n1 tree/b\n4 tree/sub/c\n4 tree/sub\n7 tree\n",
		},
		{
			Name: "-s summarizes a repeated operand once",
			FS:   identified(),
			Args: []string{"-s", "tree", "tree/sub"},
			Want: "7 tree\n",
		},
		{
			Name: "-L counts a directory reached by two links once",
			FS:   reachable(),
			Args: []string{"-L", "d"},
			Want: "4 d/p1\n4 d\n",
		},
		{
			Name: "-L writes the files below a repeated directory once",
			FS:   reachable(),
			Args: []string{"-L", "-a", "d"},
			Want: "4 d/p1/x\n4 d/p1\n4 d\n",
		},
	})
}
