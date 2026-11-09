//go:build unix && !aix && !hurd

package du

import (
	"io/fs"
	"syscall"
	"testing"

	"charm.land/snail/fsx"
)

// statFS describes the files of a memory file system with the identity a
// real file system reports, so the tests can cover -x and the counting of
// files that occur more than once.
type statFS struct {
	fsx.MemFS
	stats map[*fsx.MemFile]*syscall.Stat_t
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
	if entry, ok := fi.Sys().(*fsx.MemFile); ok {
		if st, found := s.stats[entry]; found {
			return statInfo{FileInfo: fi, st: st}
		}
	}
	return fi
}

// Lstat describes the named file without following a final symbolic link,
// with the identity injected.
func (s statFS) Lstat(name string) (fs.FileInfo, error) {
	fi, err := s.MemFS.Lstat(name)
	if err != nil {
		return nil, err
	}
	return s.wrap(fi), nil
}

// Stat describes the named file, following symbolic links, with the identity
// injected.
func (s statFS) Stat(name string) (fs.FileInfo, error) {
	fi, err := fs.Stat(s.MemFS, name)
	if err != nil {
		return nil, err
	}
	return s.wrap(fi), nil
}

// hardLinked is a hierarchy whose file a is hard linked twice, identified as
// a real file system would.
func hardLinked() statFS {
	files := fsx.MemFS{"tree/a": {Data: make([]byte, 600)}}
	if err := files.Link("tree/a", "tree/b"); err != nil {
		panic(err)
	}
	files["tree/c"] = &fsx.MemFile{Data: make([]byte, 600)}
	return statFS{
		MemFS: files,
		stats: map[*fsx.MemFile]*syscall.Stat_t{
			files["tree/a"]: {Dev: 1, Ino: 5, Nlink: 2, Blocks: 2},
			files["tree/c"]: {Dev: 1, Ino: 6, Nlink: 1, Blocks: 2},
		},
	}
}

func TestRunHardLinks(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "a file with two links is counted once",
			sys:     hardLinked(),
			args:    []string{"tree"},
			wantOut: "4 tree\n",
		},
		{
			name:    "-a writes only the first occurrence",
			sys:     hardLinked(),
			args:    []string{"-a", "tree"},
			wantOut: "2 tree/a\n2 tree/c\n4 tree\n",
		},
		{
			name:    "the occurrences of an operand share the count",
			sys:     hardLinked(),
			args:    []string{"tree/a", "tree/b"},
			wantOut: "2 tree/a\n",
		},
		{
			name:    "-c totals the occurrences once",
			sys:     hardLinked(),
			args:    []string{"-c", "tree/a", "tree/b"},
			wantOut: "2 tree/a\n2 total\n",
		},
	})
}

func TestRunDevices(t *testing.T) {
	files := fsx.MemFS{
		"tree":       {Mode: fs.ModeDir | 0o755},
		"tree/a":     {Data: make([]byte, 600)},
		"tree/mnt":   {Mode: fs.ModeDir | 0o755},
		"tree/mnt/x": {Data: make([]byte, 2000)},
	}
	sys := statFS{
		MemFS: files,
		stats: map[*fsx.MemFile]*syscall.Stat_t{
			files["tree"]:       {Dev: 1, Ino: 2, Nlink: 3},
			files["tree/a"]:     {Dev: 1, Ino: 7, Nlink: 1, Blocks: 2},
			files["tree/mnt"]:   {Dev: 2, Ino: 3, Nlink: 2, Blocks: 1},
			files["tree/mnt/x"]: {Dev: 2, Ino: 8, Nlink: 1, Blocks: 4},
		},
	}
	runCases(t, []testCase{
		{
			name:    "a hierarchy holding another file system",
			sys:     sys,
			args:    []string{"tree"},
			wantOut: "5 tree/mnt\n7 tree\n",
		},
		{
			name:    "-x skips the directories of another file system",
			sys:     sys,
			args:    []string{"-x", "tree"},
			wantOut: "1 tree/mnt\n3 tree\n",
		},
		{
			name:    "-x keeps the operands on their own file system",
			sys:     sys,
			args:    []string{"-x", "tree/mnt"},
			wantOut: "5 tree/mnt\n",
		},
	})
}

// identified is a hierarchy of two files and a subdirectory holding one,
// whose entries are identified the way a real file system does.
func identified() statFS {
	files := fsx.MemFS{
		"tree":       {Mode: fs.ModeDir | 0o755},
		"tree/a":     {Data: make([]byte, 600)},
		"tree/b":     {Data: make([]byte, 100)},
		"tree/sub":   {Mode: fs.ModeDir | 0o755},
		"tree/sub/c": {Data: make([]byte, 2000)},
	}
	return statFS{
		MemFS: files,
		stats: map[*fsx.MemFile]*syscall.Stat_t{
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
	files := fsx.MemFS{
		"d":       {Mode: fs.ModeDir | 0o755},
		"d/p1":    link("../point"),
		"d/p2":    link("../point"),
		"point":   {Mode: fs.ModeDir | 0o755},
		"point/x": {Data: make([]byte, 2000)},
	}
	return statFS{
		MemFS: files,
		stats: map[*fsx.MemFile]*syscall.Stat_t{
			files["d"]:       {Dev: 1, Ino: 11, Nlink: 2},
			files["point"]:   {Dev: 1, Ino: 12, Nlink: 2},
			files["point/x"]: {Dev: 1, Ino: 13, Nlink: 1, Blocks: 4},
		},
	}
}

func TestRunDuplicateDirectories(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "a repeated directory operand is written once",
			sys:     identified(),
			args:    []string{"tree", "tree"},
			wantOut: "4 tree/sub\n7 tree\n",
		},
		{
			name:    "an operand below another one is written once",
			sys:     identified(),
			args:    []string{"tree", "tree/sub"},
			wantOut: "4 tree/sub\n7 tree\n",
		},
		{
			name:    "a repeated operand is counted once in the total",
			sys:     identified(),
			args:    []string{"-c", "tree", "tree"},
			wantOut: "4 tree/sub\n7 tree\n7 total\n",
		},
		{
			name:    "-a writes the hierarchy of repeated operands once",
			sys:     identified(),
			args:    []string{"-a", "tree", "tree"},
			wantOut: "2 tree/a\n1 tree/b\n4 tree/sub/c\n4 tree/sub\n7 tree\n",
		},
		{
			name:    "-s summarizes a repeated operand once",
			sys:     identified(),
			args:    []string{"-s", "tree", "tree/sub"},
			wantOut: "7 tree\n",
		},
		{
			name:    "-L counts a directory reached by two links once",
			sys:     reachable(),
			args:    []string{"-L", "d"},
			wantOut: "4 d/p1\n4 d\n",
		},
		{
			name:    "-L writes the files below a repeated directory once",
			sys:     reachable(),
			args:    []string{"-L", "-a", "d"},
			wantOut: "4 d/p1/x\n4 d/p1\n4 d\n",
		},
	})
}
