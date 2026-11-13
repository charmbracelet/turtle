package stat

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/fileinfo"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// ownerID and groupID are owner identifiers no system of the supported
// platforms has a name for, so the name conversions write UNKNOWN for.
const (
	ownerID = 54321
	groupID = 54322
)

// dirMode returns the mode of a directory with the given permissions.
func dirMode(perm fs.FileMode) fs.FileMode { return fs.ModeDir | perm }

// link returns a symbolic entry pointing at target.
func link(target string) *fsx.MemFile {
	return &fsx.MemFile{Data: []byte(target), Mode: fs.ModeSymlink | 0o777}
}

// modTime is the modification time the tests write their files with.
var modTime = time.Unix(1700000000, 500000000).UTC()

// utc is the environment the tests write the timestamps with, so the zone
// offsets they expect are the ones of the UTC zone.
func utc() map[string]string { return map[string]string{"TZ": "UTC"} }

// withFiles fills the cases without files of their own with the files of
// the table, so a table shares one file system.
func withFiles(tests []testutil.Case, files fsx.MemFS) []testutil.Case {
	for i := range tests {
		if tests[i].FS == nil {
			tests[i].FS = files
		}
	}
	return tests
}

func TestRunDefault(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name:    "writes the fields of a regular file",
			FS:      fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID}},
			Environ: testutil.Environ(utc()),
			Args:    []string{"file.txt"},
			Want: "  File: file.txt\n" +
				"  Size: 5         \tBlocks: 1          IO Block: 512    regular file\n" +
				"Device: 0,0\tInode: 0           Links: 1\n" +
				"Access: (0644/-rw-r--r--)  Uid: (54321/ UNKNOWN)   Gid: (54322/ UNKNOWN)\n" +
				"Access: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Modify: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Change: 2023-11-14 22:13:20.500000000 +0000\n" +
				" Birth: -\n",
		},
		{
			Name:    "writes the fields of a directory",
			FS:      fsx.MemFS{"dir": {Mode: dirMode(0o755), ModTime: modTime, Uid: ownerID, Gid: groupID}},
			Environ: testutil.Environ(utc()),
			Args:    []string{"dir"},
			Want: "  File: dir\n" +
				"  Size: 0         \tBlocks: 0          IO Block: 512    directory\n" +
				"Device: 0,0\tInode: 0           Links: 1\n" +
				"Access: (0755/drwxr-xr-x)  Uid: (54321/ UNKNOWN)   Gid: (54322/ UNKNOWN)\n" +
				"Access: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Modify: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Change: 2023-11-14 22:13:20.500000000 +0000\n" +
				" Birth: -\n",
		},
		{
			Name: "writes the fields of every operand, a newline each",
			FS: fsx.MemFS{
				"a.txt": {Data: []byte("a"), Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID},
				"b.txt": {Data: []byte("bb"), Mode: 0o600, ModTime: modTime, Uid: ownerID, Gid: groupID},
			},
			Environ: testutil.Environ(utc()),
			Args:    []string{"a.txt", "b.txt"},
			Want: "  File: a.txt\n" +
				"  Size: 1         \tBlocks: 1          IO Block: 512    regular file\n" +
				"Device: 0,0\tInode: 0           Links: 1\n" +
				"Access: (0644/-rw-r--r--)  Uid: (54321/ UNKNOWN)   Gid: (54322/ UNKNOWN)\n" +
				"Access: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Modify: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Change: 2023-11-14 22:13:20.500000000 +0000\n" +
				" Birth: -\n" +
				"  File: b.txt\n" +
				"  Size: 2         \tBlocks: 1          IO Block: 512    regular file\n" +
				"Device: 0,0\tInode: 0           Links: 1\n" +
				"Access: (0600/-rw-------)  Uid: (54321/ UNKNOWN)   Gid: (54322/ UNKNOWN)\n" +
				"Access: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Modify: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Change: 2023-11-14 22:13:20.500000000 +0000\n" +
				" Birth: -\n",
		},
		{
			Name: "resolves an operand against the working directory",
			FS: fsx.MemFS{
				"work/file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID},
			},
			Dir:     "work",
			Environ: testutil.Environ(utc()),
			Args:    []string{"file.txt"},
			Want: "  File: file.txt\n" +
				"  Size: 5         \tBlocks: 1          IO Block: 512    regular file\n" +
				"Device: 0,0\tInode: 0           Links: 1\n" +
				"Access: (0644/-rw-r--r--)  Uid: (54321/ UNKNOWN)   Gid: (54322/ UNKNOWN)\n" +
				"Access: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Modify: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Change: 2023-11-14 22:13:20.500000000 +0000\n" +
				" Birth: -\n",
		},
		{
			Name:    "resolves an operand with dot components",
			FS:      fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			Environ: testutil.Environ(utc()),
			Args:    []string{"-c", "%n %F", "./file.txt"},
			Want:    "./file.txt regular file\n",
		},
		{
			Name:    "resolves an operand naming a directory by its trailing separator",
			FS:      fsx.MemFS{"dir": {Mode: dirMode(0o755), ModTime: modTime}},
			Environ: testutil.Environ(utc()),
			Args:    []string{"-c", "%n %F", "dir/"},
			Want:    "dir/ directory\n",
		},
	})
}

func TestRunFormatDirectives(t *testing.T) {
	files := fsx.MemFS{
		"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID},
		"dir":      {Mode: dirMode(0o755), ModTime: modTime, Uid: ownerID, Gid: groupID},
		"empty":    {ModTime: modTime},
		"sticky":   {Mode: fs.ModeSticky | 0o755, ModTime: modTime},
		"setuid":   {Mode: fs.ModeSetuid | fs.ModeSetgid | 0o750, ModTime: modTime},
		"big":      {Data: make([]byte, 1500), ModTime: modTime},
		"ln":       link("file.txt"),
	}
	tests := []testutil.Case{
		{Name: "octal permissions", Args: []string{"-c", "%a", "file.txt"}, Want: "644\n"},
		{Name: "octal permissions with special bits", Args: []string{"-c", "%a", "sticky"}, Want: "1755\n"},
		{Name: "octal permissions with setuid and setgid", Args: []string{"-c", "%a", "setuid"}, Want: "6750\n"},
		{Name: "octal permissions zero padded", Args: []string{"-c", "%06a", "file.txt"}, Want: "000644\n"},
		{Name: "octal permissions with a hash flag", Args: []string{"-c", "%#a", "file.txt"}, Want: "0644\n"},
		{Name: "octal permissions with a hash flag and a width", Args: []string{"-c", "%#07a", "file.txt"}, Want: "0000644\n"},
		{Name: "octal permissions left adjusted", Args: []string{"-c", "%-6a|", "file.txt"}, Want: "644   |\n"},
		{Name: "symbolic permissions", Args: []string{"-c", "%A", "file.txt"}, Want: "-rw-r--r--\n"},
		{Name: "symbolic permissions of a directory", Args: []string{"-c", "%A", "dir"}, Want: "drwxr-xr-x\n"},
		{Name: "symbolic permissions within a width", Args: []string{"-c", "%12A|", "file.txt"}, Want: "  -rw-r--r--|\n"},
		{Name: "symbolic permissions cut to the precision", Args: []string{"-c", "%10.10A", "file.txt"}, Want: "-rw-r--r--\n"},
		{Name: "block count", Args: []string{"-c", "%b", "file.txt"}, Want: "1\n"},
		{Name: "block count of a directory", Args: []string{"-c", "%b", "dir"}, Want: "0\n"},
		{Name: "block count measured by the size", Args: []string{"-c", "%b", "big"}, Want: "3\n"},
		{Name: "block size", Args: []string{"-c", "%B", "file.txt"}, Want: "512\n"},
		{Name: "device number in decimal", Args: []string{"-c", "%d", "file.txt"}, Want: "0\n"},
		{Name: "device number in hex", Args: []string{"-c", "%D", "file.txt"}, Want: "0\n"},
		{Name: "device major in decimal", Args: []string{"-c", "%Hd", "file.txt"}, Want: "0\n"},
		{Name: "device minor in decimal", Args: []string{"-c", "%Ld", "file.txt"}, Want: "0\n"},
		{Name: "raw mode in hex", Args: []string{"-c", "%f", "file.txt"}, Want: "81a4\n"},
		{Name: "raw mode of a symbolic link", Args: []string{"-c", "%f", "ln"}, Want: "a1ff\n"},
		{Name: "raw mode in hex with a hash flag", Args: []string{"-c", "%#f", "file.txt"}, Want: "0x81a4\n"},
		{Name: "file type", Args: []string{"-c", "%F", "file.txt"}, Want: "regular file\n"},
		{Name: "file type of a directory", Args: []string{"-c", "%F", "dir"}, Want: "directory\n"},
		{Name: "file type of a symbolic link", Args: []string{"-c", "%F", "ln"}, Want: "symbolic link\n"},
		{Name: "group id", Args: []string{"-c", "%g", "file.txt"}, Want: "54322\n"},
		{Name: "hard link count", Args: []string{"-c", "%h", "file.txt"}, Want: "1\n"},
		{Name: "inode number", Args: []string{"-c", "%i", "file.txt"}, Want: "0\n"},
		{Name: "file name", Args: []string{"-c", "%n", "file.txt"}, Want: "file.txt\n"},
		{Name: "quoted file name left alone", Args: []string{"-c", "%Qn", "file.txt"}, Want: "file.txt\n"},
		{Name: "quoted file name with spaces", Args: []string{"-c", "%Qn", "a b"}, FS: fsx.MemFS{"a b": {}}, Want: "'a b'\n"},
		{Name: "access time hint", Args: []string{"-c", "%o", "file.txt"}, Want: "512\n"},
		{Name: "size", Args: []string{"-c", "%s", "file.txt"}, Want: "5\n"},
		{Name: "size of an empty file", Args: []string{"-c", "%s", "empty"}, Want: "0\n"},
		{Name: "size within a width", Args: []string{"-c", "%4s|", "file.txt"}, Want: "   5|\n"},
		{Name: "size left adjusted", Args: []string{"-c", "%-4s|", "file.txt"}, Want: "5   |\n"},
		{
			Name: "access time", Args: []string{"-c", "%x", "file.txt"}, Environ: testutil.Environ(utc()),
			Want: "2023-11-14 22:13:20.500000000 +0000\n",
		},
		{
			Name: "access time in seconds", Args: []string{"-c", "%X", "file.txt"},
			Want: "1700000000\n",
		},
		{
			Name: "modification time", Args: []string{"-c", "%y", "file.txt"}, Environ: testutil.Environ(utc()),
			Want: "2023-11-14 22:13:20.500000000 +0000\n",
		},
		{
			Name: "modification time in seconds", Args: []string{"-c", "%Y", "file.txt"},
			Want: "1700000000\n",
		},
		{
			Name: "status change time", Args: []string{"-c", "%z", "file.txt"}, Environ: testutil.Environ(utc()),
			Want: "2023-11-14 22:13:20.500000000 +0000\n",
		},
		{
			Name: "status change time in seconds", Args: []string{"-c", "%Z", "file.txt"},
			Want: "1700000000\n",
		},
		{Name: "birth time unknown", Args: []string{"-c", "%w", "file.txt"}, Want: "-\n"},
		{Name: "birth time in seconds unknown", Args: []string{"-c", "%W", "file.txt"}, Want: "0\n"},
		{Name: "seconds zero padded", Args: []string{"-c", "%015Y", "file.txt"}, Want: "000001700000000\n"},
		{Name: "seconds space aligned", Args: []string{"-c", "%15Y|", "file.txt"}, Want: "     1700000000|\n"},
		{Name: "seconds left aligned", Args: []string{"-c", "%-15Y|", "file.txt"}, Want: "1700000000     |\n"},
		{Name: "seconds with a precision", Args: []string{"-c", "%.3Y", "file.txt"}, Want: "1700000000.500\n"},
		{Name: "seconds with a bare period", Args: []string{"-c", "%.Y", "file.txt"}, Want: "1700000000.500000000\n"},
		{
			Name: "seconds before the Epoch", FS: fsx.MemFS{"old": {ModTime: time.Unix(-2, 750000000)}},
			Args: []string{"-c", "%.3Y", "old"}, Want: "-1.250\n",
		},
		{
			Name: "whole seconds before the Epoch", FS: fsx.MemFS{"old": {ModTime: time.Unix(-3, 0)}},
			Args: []string{"-c", "%.3Y", "old"}, Want: "-3.000\n",
		},
		{
			Name: "fractional seconds before the Epoch are floored", FS: fsx.MemFS{"old": {ModTime: time.Unix(-2, 748100000)}},
			Args: []string{"-c", "%.1Y", "old"}, Want: "-1.3\n",
		},
		{
			Name: "a precision beyond the nanoseconds is zero padded", FS: fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			Args: []string{"-c", "%.20Y", "file.txt"},
			Want: "1700000000.50000000000000000000\n",
		},
		{
			Name: "a precision beyond the nanoseconds of a second before the Epoch", FS: fsx.MemFS{"old": {ModTime: time.Unix(-2, 748100000)}},
			Args: []string{"-c", "%.20Y", "old"}, Want: "-1.25190000000000000000\n",
		},
		{
			Name: "a precision beyond the nanoseconds is floored before the Epoch", FS: fsx.MemFS{"old": {ModTime: time.Unix(-2, 748100000)}},
			Args: []string{"-c", "%.25Y", "old"}, Want: "-1.2519000000000000000000000\n",
		},
		{
			Name: "an exact tenth before the Epoch is kept", FS: fsx.MemFS{"old": {ModTime: time.Unix(-2, 800000000)}},
			Args: []string{"-c", "%.1Y", "old"}, Want: "-1.2\n",
		},
		{
			Name: "a fraction of a second before the Epoch is floored", FS: fsx.MemFS{"old": {ModTime: time.Unix(-1, 250000000)}},
			Args: []string{"-c", "%.1Y", "old"}, Want: "-0.8\n",
		},
		{
			Name: "a second before the Epoch within a hundredth", FS: fsx.MemFS{"old": {ModTime: time.Unix(-1, 950000000)}},
			Args: []string{"-c", "%.2Y", "old"}, Want: "-0.05\n",
		},
		{
			Name: "seconds with a width and a precision", Args: []string{"-c", "[%22.3Y]", "file.txt"},
			Want: "[        1700000000.500]\n",
		},
		{
			Name: "seconds zero padded with a precision", Args: []string{"-c", "[%022.3Y]", "file.txt"},
			Want: "[000000001700000000.500]\n",
		},
		{
			Name: "time with a precision", Args: []string{"-c", "%.3x", "file.txt"}, Environ: testutil.Environ(utc()),
			Want: "2023-11-14 22:13:20.500 +0000\n",
		},
		{
			Name: "time without a fraction", Args: []string{"-c", "%.0x", "file.txt"}, Environ: testutil.Environ(utc()),
			Want: "2023-11-14 22:13:20 +0000\n",
		},
		{Name: "unknown directive", Args: []string{"-c", "%q|", "file.txt"}, Want: "?|\n"},
		{Name: "unknown directive with flags", Args: []string{"-c", "%5q|", "file.txt"}, Want: "?|\n"},
		{Name: "modifier without a pair", Args: []string{"-c", "%H|", "file.txt"}, Want: "?|\n"},
		{Name: "percent at the end", Args: []string{"-c", "a%", "file.txt"}, Want: "a%\n"},
		{Name: "percent signs kept", Args: []string{"-c", "a%%b|", "file.txt"}, Want: "a%b|\n"},
		{Name: "several fields on a line", Args: []string{"-c", "%d:%i", "file.txt", "dir"}, Want: "0:0\n0:0\n"},
	}
	testutil.Run(t, New, withFiles(tests, files))
}

func TestRunFormatDirectivesOfDevices(t *testing.T) {
	r := &renderer{prog: &Prog{Runtime: &coreutils.Runtime{}}, loc: time.UTC, style: quoteShellEscape}
	st := fileStat{fi: &fakeInfo{mode: fs.ModeDevice | 0o640}, rdev: 0x0207}
	tests := []struct {
		format string
		want   string
	}{
		{"%r", "519"},
		{"%R", "207"},
		{"%Hr", "2"},
		{"%Lr", "7"},
		{"%t", "2"},
		{"%T", "7"},
	}
	for _, test := range tests {
		t.Run(test.format, func(t *testing.T) {
			out, fail, err := r.render(test.format, &st, false, false)
			if err != nil || fail {
				t.Fatalf("render() = %q, %v, %v, want no failure", out, fail, err)
			}
			if out != test.want {
				t.Errorf("render() = %q, want %q", out, test.want)
			}
		})
	}
}

func TestRunDefaultOfDevices(t *testing.T) {
	r := &renderer{prog: &Prog{Runtime: &coreutils.Runtime{}}, loc: time.UTC, style: quoteShellEscape}
	st := fileStat{fi: &fakeInfo{mode: fs.ModeDevice | fs.ModeCharDevice | 0o644}, nlink: 1}
	out, fail, err := r.render(defaultFormatDevice, &st, false, false)
	if err != nil || fail {
		t.Fatalf("render() = %q, %v, %v, want no failure", out, fail, err)
	}
	if !strings.Contains(out, "Device type: 0,0\n") {
		t.Errorf("render() = %q, want a device type field", out)
	}
	if !strings.Contains(out, "Links: 1     Device type") {
		t.Errorf("render() = %q, want the link count of the device format", out)
	}
}

// fakeInfo is a FileInfo whose fields the tests write.
type fakeInfo struct {
	mode fs.FileMode
	size int64
	sys  any
}

func (f *fakeInfo) Name() string       { return "dev" }
func (f *fakeInfo) Size() int64        { return f.size }
func (f *fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f *fakeInfo) ModTime() time.Time { return time.Time{} }
func (f *fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f *fakeInfo) Sys() any           { return f.sys }

func TestRunPrintf(t *testing.T) {
	files := fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}}
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "writes no trailing newline",
			FS:   files,
			Args: []string{"--printf", "%n", "file.txt"},
			Want: "file.txt",
		},
		{
			Name: "interprets the named escapes",
			FS:   files,
			Args: []string{"--printf", `%n\a\b\e\f\n\r\t\v\\%n`, "file.txt"},
			Want: "file.txt\a\b\x1b\f\n\r\t\v\\file.txt",
		},
		{
			Name: "interprets the octal and hexadecimal escapes",
			FS:   files,
			Args: []string{"--printf", `A\0101B\x42C%n`, "file.txt"},
			Want: "AABBCfile.txt",
		},
		{
			Name:     "warns about an unrecognized escape",
			FS:       files,
			Args:     []string{"--printf", `\q%n`, "file.txt"},
			Want:     "qfile.txt",
			WantErr:  "stat: warning: unrecognized escape '\\q'\n",
			WantExit: 0,
		},
		{
			Name:     "warns about a backslash at the end",
			FS:       files,
			Args:     []string{"--printf", `a\`, "file.txt"},
			Want:     "a\\",
			WantErr:  "stat: warning: backslash at end of format\n",
			WantExit: 0,
		},
		{
			Name: "keeps the backslashes of --format",
			FS:   files,
			Args: []string{"-c", `a\nb`, "file.txt"},
			Want: "a\\nb\n",
		},
		{
			Name: "newline terminates every use of --format",
			FS:   files,
			Args: []string{"-c", "%n", "file.txt", "file.txt"},
			Want: "file.txt\nfile.txt\n",
		},
		{
			Name: "the format of the last option wins",
			FS:   files,
			Args: []string{"-c", "%a", "--printf", "%s%n", "file.txt"},
			Want: "5file.txt",
		},
		{
			Name: "the format of the last option wins the other way",
			FS:   files,
			Args: []string{"--printf", "%s", "-c", "%a", "file.txt"},
			Want: "644\n",
		},
		{
			Name: "an empty --format writes an empty line",
			FS:   files,
			Args: []string{"-c", "", "file.txt", "file.txt"},
			Want: "\n\n",
		},
		{
			Name: "an empty --printf writes nothing",
			FS:   files,
			Args: []string{"--printf", "", "file.txt", "file.txt"},
			Want: "",
		},
		{
			Name: "an empty --printf keeps the last of the options",
			FS:   files,
			Args: []string{"-c", "%a", "--printf", "", "file.txt"},
			Want: "",
		},
	})
}

func TestRunTerse(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "writes one line of fields per file",
			FS:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID}},
			Args: []string{"-t", "file.txt"},
			Want: "file.txt 5 1 81a4 54321 54322 0 0 1 0 0 " +
				"1700000000 1700000000 1700000000 0 512\n",
		},
		{
			Name: "quotes names that need it",
			FS:   fsx.MemFS{"a b": {Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID}},
			Args: []string{"-t", "a b"},
			Want: "'a b' 0 0 81a4 54321 54322 0 0 1 0 0 " +
				"1700000000 1700000000 1700000000 0 512\n",
		},
		{
			Name: "gives way to an explicit format",
			FS:   fsx.MemFS{"file.txt": {Mode: 0o644, ModTime: modTime}},
			Args: []string{"-t", "-c", "%n", "file.txt"},
			Want: "file.txt\n",
		},
	})
}

func TestRunFileSystem(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "writes the file system fields it doesn't know as marks",
			FS:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			Args: []string{"-f", "file.txt"},
			Want: "  File: file.txt\n" +
				"    ID: ? Namelen: ? Type: ?\n" +
				"Block size: ? Fundamental block size: ?\n" +
				"Blocks: Total: ? Free: ? Available: ?\n" +
				"Inodes: Total: ? Free: ?\n",
		},
		{
			Name: "writes the terse fields of the file system",
			FS:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			Args: []string{"-f", "-t", "file.txt"},
			Want: "file.txt ? ? ? ? ? ? ? ? ? ?\n",
		},
		{
			Name: "reports the file name only when asked for",
			FS:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			Args: []string{"-f", "-c", "%n %s", "file.txt"},
			Want: "file.txt ?\n",
		},
		{
			Name: "follows a link to the file system it is on",
			FS:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}, "ln": link("file.txt")},
			Args: []string{"-f", "-c", "%n", "ln"},
			Want: "ln\n",
		},
		{
			Name:     "diagnoses a missing file",
			FS:       fsx.MemFS{},
			Args:     []string{"-f", "gone"},
			WantErr:  "stat: cannot read file system information for 'gone': file does not exist\n",
			WantExit: 1,
		},
	})
}

func TestRunSymlinks(t *testing.T) {
	files := fsx.MemFS{
		"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID},
		"dir":      {Mode: dirMode(0o755), ModTime: modTime, Uid: ownerID, Gid: groupID},
		"tofile":   link("file.txt"),
		"todir":    link("dir"),
		"broken":   link("gone"),
		"chain":    link("tofile"),
	}
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "reports the link itself by default",
			FS:   files,
			Args: []string{"-c", "%F %s %N", "tofile"},
			Want: "symbolic link 8 tofile -> file.txt\n",
		},
		{
			Name: "reports the file a link refers to with -L",
			FS:   files,
			Args: []string{"-L", "-c", "%F %s %N", "tofile"},
			Want: "regular file 5 tofile\n",
		},
		{
			Name: "reports a link to a directory with -L",
			FS:   files,
			Args: []string{"-L", "-c", "%F", "todir"},
			Want: "directory\n",
		},
		{
			Name: "reports the file a chain of links names with -L",
			FS:   files,
			Args: []string{"-L", "-c", "%F", "chain"},
			Want: "regular file\n",
		},
		{
			Name:     "diagnoses a broken link with -L",
			FS:       files,
			Args:     []string{"-L", "broken"},
			WantErr:  "stat: cannot stat 'broken': file does not exist\n",
			WantExit: 1,
		},
		{
			Name: "reports a broken link without -L",
			FS:   files,
			Args: []string{"-c", "%F %N", "broken"},
			Want: "symbolic link broken -> gone\n",
		},
		{
			Name: "quotes the link and its target",
			FS:   fsx.MemFS{"a b": link("c d")},
			Args: []string{"-c", "%N", "a b"},
			Want: "'a b' -> 'c d'\n",
		},
		{
			Name: "long option form of the dereference",
			FS:   files,
			Args: []string{"--dereference", "-c", "%F", "tofile"},
			Want: "regular file\n",
		},
	})
}

func TestRunQuoting(t *testing.T) {
	type entry struct {
		name  string
		style string
		files fsx.MemFS
		args  []string
		want  string
	}
	tests := []entry{
		{name: "literal", style: "literal", files: fsx.MemFS{"a b": {}}, args: []string{"-c", "%Qn", "a b"}, want: "a b\n"},
		{name: "shell", style: "shell", files: fsx.MemFS{"a b": {}}, args: []string{"-c", "%Qn", "a b"}, want: "'a b'\n"},
		{name: "shell leaves a plain name", style: "shell", files: fsx.MemFS{"ab": {}}, args: []string{"-c", "%Qn", "ab"}, want: "ab\n"},
		{name: "shell-always", style: "shell-always", files: fsx.MemFS{"ab": {}}, args: []string{"-c", "%Qn", "ab"}, want: "'ab'\n"},
		{name: "shell-escape leaves a plain name", style: "shell-escape", files: fsx.MemFS{"ab": {}}, args: []string{"-c", "%Qn", "ab"}, want: "ab\n"},
		{
			name: "shell-escape dollar quotes a control character", style: "shell-escape",
			files: fsx.MemFS{"a\tb": {}}, args: []string{"-c", "%Qn", "a\tb"}, want: "$'a\\tb'\n",
		},
		{name: "shell-escape-always", style: "shell-escape-always", files: fsx.MemFS{"ab": {}}, args: []string{"-c", "%Qn", "ab"}, want: "'ab'\n"},
		{name: "c", style: "c", files: fsx.MemFS{"a\tb": {}}, args: []string{"-c", "%Qn", "a\tb"}, want: `"a\tb"` + "\n"},
		{name: "c leaves a space alone", style: "c", files: fsx.MemFS{"a b": {}}, args: []string{"-c", "%Qn", "a b"}, want: `"a b"` + "\n"},
		{name: "c-maybe leaves a plain name", style: "c-maybe", files: fsx.MemFS{"ab": {}}, args: []string{"-c", "%Qn", "ab"}, want: "ab\n"},
		{name: "c-maybe quotes a control character", style: "c-maybe", files: fsx.MemFS{"a\tb": {}}, args: []string{"-c", "%Qn", "a\tb"}, want: `"a\tb"` + "\n"},
		{name: "escape", style: "escape", files: fsx.MemFS{"a\tb": {}}, args: []string{"-c", "%Qn", "a\tb"}, want: "a\\tb\n"},
		{name: "locale", style: "locale", files: fsx.MemFS{"a\tb": {}}, args: []string{"-c", "%Qn", "a\tb"}, want: "‘a\\tb’\n"},
		{name: "clocale", style: "clocale", files: fsx.MemFS{"a\tb": {}}, args: []string{"-c", "%Qn", "a\tb"}, want: "“a\\tb”\n"},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			testutil.Once(t, New, testutil.Case{
				Name:    test.name,
				FS:      test.files,
				Environ: testutil.Environ(map[string]string{"TZ": "UTC", "QUOTING_STYLE": test.style}),
				Args:    test.args,
				Want:    test.want,
			})
		})
	}
}

func TestRunQuotingDefault(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "quotes a control character by default",
			FS:   fsx.MemFS{"a\tb": {}},
			Args: []string{"-c", "%Qn", "a\tb"},
			Want: "$'a\\tb'\n",
		},
		{
			Name:     "warns about an invalid style and keeps the default",
			FS:       fsx.MemFS{"a b": {}},
			Environ:  testutil.Environ(map[string]string{"QUOTING_STYLE": "weird"}),
			Args:     []string{"-c", "%Qn", "a b"},
			Want:     "'a b'\n",
			WantErr:  "stat: ignoring invalid value of environment variable QUOTING_STYLE: weird\n",
			WantExit: 0,
		},
	})
}

func TestQuoteName(t *testing.T) {
	tests := []struct {
		style quotingStyle
		name  string
		want  string
	}{
		{quoteLiteral, "ab", "ab"},
		{quoteShell, "", "''"},
		{quoteShell, "a'b", `'a'\''b'`},
		{quoteShellEscape, "a\nb", `$'a\nb'`},
		{quoteShellEscape, "a b", `'a b'`},
		{quoteShellEscapeAlways, "ab", "'ab'"},
		{quoteC, "a\"b", `"a\"b"`},
		{quoteCMaybe, "ab", "ab"},
		{quoteEscape, "a b", "a b"},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			if got := quoteName(test.name, test.style); got != test.want {
				t.Errorf("quoteName(%q) = %q, want %q", test.name, got, test.want)
			}
		})
	}
}

func TestRunErrors(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name:     "needs an operand",
			FS:       fsx.MemFS{"file.txt": {}},
			WantErr:  "stat: missing operand\nTry 'stat --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "rejects an unknown short option",
			FS:       fsx.MemFS{"file.txt": {}},
			Args:     []string{"-x", "file.txt"},
			WantErr:  "stat: invalid option -- 'x'\nTry 'stat --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "rejects an unknown option in a group",
			FS:       fsx.MemFS{"file.txt": {}},
			Args:     []string{"-tx", "file.txt"},
			WantErr:  "stat: invalid option -- 'x'\nTry 'stat --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "rejects an unknown long option",
			FS:       fsx.MemFS{"file.txt": {}},
			Args:     []string{"--frobnicate", "file.txt"},
			WantErr:  "stat: unrecognized option '--frobnicate'\nTry 'stat --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "needs the argument of --format",
			FS:       fsx.MemFS{"file.txt": {}},
			Args:     []string{"--format"},
			WantErr:  "stat: option '--format' requires an argument\nTry 'stat --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "needs the argument of -c",
			FS:       fsx.MemFS{"file.txt": {}},
			Args:     []string{"-c"},
			WantErr:  "stat: option requires an argument -- 'c'\nTry 'stat --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "rejects an invalid --cached value",
			FS:       fsx.MemFS{"file.txt": {}},
			Args:     []string{"--cached=sometimes", "file.txt"},
			WantErr:  "stat: invalid argument 'sometimes' for '--cached'\nTry 'stat --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "takes a fatal invalid directive with flags",
			FS:       fsx.MemFS{"file.txt": {}},
			Args:     []string{"-c", "%5%", "file.txt"},
			WantErr:  "stat: '%5%': invalid directive\n",
			WantExit: 1,
		},
		{
			Name:     "writes the text before an invalid directive",
			FS:       fsx.MemFS{"file.txt": {}},
			Args:     []string{"-c", "abc%5%", "file.txt"},
			Want:     "abc",
			WantErr:  "stat: '%5%': invalid directive\n",
			WantExit: 1,
		},
		{
			Name:     "diagnoses a directive the format ends within",
			FS:       fsx.MemFS{"file.txt": {}},
			Args:     []string{"-c", "%5", "file.txt"},
			WantErr:  "stat: '%5': invalid directive\n",
			WantExit: 1,
		},
		{
			Name:     "rejects an argument given to a flag option",
			FS:       fsx.MemFS{"file.txt": {}},
			Args:     []string{"--terse=1", "file.txt"},
			WantErr:  "stat: option '--terse=1' doesn't allow an argument\nTry 'stat --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "diagnoses a missing file and goes on",
			FS:       fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			Args:     []string{"-c", "%n", "gone", "file.txt"},
			Want:     "file.txt\n",
			WantErr:  "stat: cannot stat 'gone': file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "reports an invalid zero-length file name",
			FS:       fsx.MemFS{"file.txt": {}},
			Args:     []string{""},
			WantErr:  "stat: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "reports no file system available",
			Args:     []string{"file.txt"},
			WantErr:  "stat: no file system available\n",
			WantExit: 1,
		},
	})
}

func TestRunCachedOption(t *testing.T) {
	files := fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}}
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "accepts the modes of --cached",
			FS:   files,
			Args: []string{"--cached=always", "--cached=never", "--cached=default", "-c", "%n", "file.txt"},
			Want: "file.txt\n",
		},
		{
			Name: "ends the options at --",
			FS:   fsx.MemFS{"-weird": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			Args: []string{"-c", "%n", "--", "-weird"},
			Want: "-weird\n",
		},
	})
}

func TestRunStdOptions(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name:     "writes the usage for --help",
			Args:     []string{"--help"},
			Want:     usage,
			WantExit: 0,
		},
		{
			Name:     "writes the usage for the short form of --help",
			Args:     []string{"-L", "--help"},
			Want:     usage,
			WantExit: 0,
		},
		{
			Name:     "writes the version for --version",
			Args:     []string{"--version"},
			Want:     "stat (charm.land/turtle/coreutils) " + coreutils.Version + "\nhttps://charm.land/turtle\n",
			WantExit: 0,
		},
	})
}

func TestRunContext(t *testing.T) {
	rt := &coreutils.Runtime{
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
		FS:     fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := New(rt).Run(ctx, "file.txt"); !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRunTimezone(t *testing.T) {
	if _, err := time.LoadLocation("America/New_York"); err != nil {
		t.Skip("the system doesn't have a time zone database")
	}
	testutil.Run(t, New, []testutil.Case{
		{
			Name:    "writes the timestamps in the zone TZ names",
			FS:      fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			Environ: testutil.Environ(map[string]string{"TZ": "America/New_York"}),
			Args:    []string{"-c", "%x", "file.txt"},
			Want:    "2023-11-14 17:13:20.500000000 -0500\n",
		},
		{
			Name:    "falls back to the local zone for an unknown TZ",
			FS:      fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			Environ: testutil.Environ(map[string]string{"TZ": "Nowhere/Bad"}),
			Args:    []string{"-c", "%Y", "file.txt"},
			Want:    "1700000000\n",
		},
	})
}

func TestRunUsersAndGroups(t *testing.T) {
	testutil.Run(t, New, []testutil.Case{
		{
			Name: "reports UNKNOWN for an owner without a name",
			FS:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID}},
			Args: []string{"-c", "%U %G", "file.txt"},
			Want: "UNKNOWN UNKNOWN\n",
		},
		{
			Name: "reports the owner a system names",
			FS:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime, Uid: 0}},
			Args: func() []string {
				return []string{"-c", "%u %U", "file.txt"}
			}(),
			Want: func() string {
				if runtime.GOOS == "windows" {
					return "0 UNKNOWN\n"
				}
				return "0 root\n"
			}(),
		},
		{
			Name: "reports the owners of a file without owner information as zero",
			FS:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			Args: []string{"-c", "%u %g", "file.txt"},
			Want: "0 0\n",
		},
	})
}

func TestRunRealFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Run(t, New, []testutil.Case{
		{
			Name:    "reports the size and type of a real file",
			FS:      fsx.OsFS(""),
			Dir:     dir,
			Environ: testutil.Environ(utc()),
			Args:    []string{"-c", "%n %s %F", "file.txt"},
			Want:    "file.txt 5 regular file\n",
		},
	})
	if runtime.GOOS != "windows" {
		testutil.Run(t, New, []testutil.Case{
			{
				Name:    "reports the inode and device of a real file",
				FS:      fsx.OsFS(""),
				Dir:     dir,
				Environ: testutil.Environ(utc()),
				Args:    []string{"-c", "%i %D %d", "file.txt"},
				Want: func() string {
					st, err := os.Stat(path)
					if err != nil {
						t.Fatal(err)
					}
					info, ok := fileinfo.Stat(st)
					if !ok {
						t.Skip("the system doesn't report inode numbers")
					}
					return strconv.FormatUint(info.Ino, 10) + " " +
						strconv.FormatUint(info.Dev, 16) + " " +
						strconv.FormatUint(info.Dev, 10) + "\n"
				}(),
			},
		})
	}
}

func TestRunWindowsOperands(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the tests of the Windows operands run on Windows alone")
	}
	testutil.Run(t, New, []testutil.Case{
		{
			Name:    "takes the operands with Windows separators",
			FS:      fsx.MemFS{"dir/file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			Environ: testutil.Environ(utc()),
			Args:    []string{"-c", "%n %s", `dir\file.txt`},
			Want:    `dir\file.txt 5` + "\n",
		},
	})
}
