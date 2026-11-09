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

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/fileinfo"
	"charm.land/snail/fsx"
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

// testCase is one run of stat, with the files of the memory file system it
// works on and the standard streams its output is read from.
type testCase struct {
	name     string
	files    fsx.MemFS
	sys      fsx.FS
	wd       string
	env      map[string]string
	args     []string
	wantOut  string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout:     stdout,
		Stderr:     stderr,
		WorkingDir: test.wd,
		Environ:    func() []string { return environ(test.env) },
	}
	switch {
	case test.sys != nil:
		rt.FS = test.sys
	case test.files != nil:
		rt.FS = test.files
	}
	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.wantOut {
		t.Errorf("Run() stdout = %q, want %q", got, test.wantOut)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
}

func runCases(t *testing.T, tests []testCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

// withFiles fills the cases without files of their own with the files of
// the table, so a table shares one file system.
func withFiles(tests []testCase, files fsx.MemFS) []testCase {
	for i := range tests {
		if tests[i].files == nil {
			tests[i].files = files
		}
	}
	return tests
}

// environ returns the test environment as a list of KEY=VALUE strings.
func environ(values map[string]string) []string {
	entries := make([]string, 0, len(values))
	for name, value := range values {
		entries = append(entries, name+"="+value)
	}
	return entries
}

func exitCode(t *testing.T, err error) int {
	t.Helper()

	if err == nil {
		return 0
	}
	var exitErr *coreutils.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
	}
	return exitErr.Code
}

func TestRunDefault(t *testing.T) {
	runCases(t, []testCase{
		{
			name:  "writes the fields of a regular file",
			files: fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID}},
			env:   utc(),
			args:  []string{"file.txt"},
			wantOut: "  File: file.txt\n" +
				"  Size: 5         \tBlocks: 1          IO Block: 512    regular file\n" +
				"Device: 0,0\tInode: 0           Links: 1\n" +
				"Access: (0644/-rw-r--r--)  Uid: (54321/ UNKNOWN)   Gid: (54322/ UNKNOWN)\n" +
				"Access: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Modify: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Change: 2023-11-14 22:13:20.500000000 +0000\n" +
				" Birth: -\n",
		},
		{
			name:  "writes the fields of a directory",
			files: fsx.MemFS{"dir": {Mode: dirMode(0o755), ModTime: modTime, Uid: ownerID, Gid: groupID}},
			env:   utc(),
			args:  []string{"dir"},
			wantOut: "  File: dir\n" +
				"  Size: 0         \tBlocks: 0          IO Block: 512    directory\n" +
				"Device: 0,0\tInode: 0           Links: 1\n" +
				"Access: (0755/drwxr-xr-x)  Uid: (54321/ UNKNOWN)   Gid: (54322/ UNKNOWN)\n" +
				"Access: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Modify: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Change: 2023-11-14 22:13:20.500000000 +0000\n" +
				" Birth: -\n",
		},
		{
			name: "writes the fields of every operand, a newline each",
			files: fsx.MemFS{
				"a.txt": {Data: []byte("a"), Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID},
				"b.txt": {Data: []byte("bb"), Mode: 0o600, ModTime: modTime, Uid: ownerID, Gid: groupID},
			},
			env:  utc(),
			args: []string{"a.txt", "b.txt"},
			wantOut: "  File: a.txt\n" +
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
			name: "resolves an operand against the working directory",
			files: fsx.MemFS{
				"work/file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID},
			},
			wd:   "work",
			env:  utc(),
			args: []string{"file.txt"},
			wantOut: "  File: file.txt\n" +
				"  Size: 5         \tBlocks: 1          IO Block: 512    regular file\n" +
				"Device: 0,0\tInode: 0           Links: 1\n" +
				"Access: (0644/-rw-r--r--)  Uid: (54321/ UNKNOWN)   Gid: (54322/ UNKNOWN)\n" +
				"Access: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Modify: 2023-11-14 22:13:20.500000000 +0000\n" +
				"Change: 2023-11-14 22:13:20.500000000 +0000\n" +
				" Birth: -\n",
		},
		{
			name:    "resolves an operand with dot components",
			files:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			env:     utc(),
			args:    []string{"-c", "%n %F", "./file.txt"},
			wantOut: "./file.txt regular file\n",
		},
		{
			name:    "resolves an operand naming a directory by its trailing separator",
			files:   fsx.MemFS{"dir": {Mode: dirMode(0o755), ModTime: modTime}},
			env:     utc(),
			args:    []string{"-c", "%n %F", "dir/"},
			wantOut: "dir/ directory\n",
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
	tests := []testCase{
		{name: "octal permissions", args: []string{"-c", "%a", "file.txt"}, wantOut: "644\n"},
		{name: "octal permissions with special bits", args: []string{"-c", "%a", "sticky"}, wantOut: "1755\n"},
		{name: "octal permissions with setuid and setgid", args: []string{"-c", "%a", "setuid"}, wantOut: "6750\n"},
		{name: "octal permissions zero padded", args: []string{"-c", "%06a", "file.txt"}, wantOut: "000644\n"},
		{name: "octal permissions with a hash flag", args: []string{"-c", "%#a", "file.txt"}, wantOut: "0644\n"},
		{name: "octal permissions with a hash flag and a width", args: []string{"-c", "%#07a", "file.txt"}, wantOut: "0000644\n"},
		{name: "octal permissions left adjusted", args: []string{"-c", "%-6a|", "file.txt"}, wantOut: "644   |\n"},
		{name: "symbolic permissions", args: []string{"-c", "%A", "file.txt"}, wantOut: "-rw-r--r--\n"},
		{name: "symbolic permissions of a directory", args: []string{"-c", "%A", "dir"}, wantOut: "drwxr-xr-x\n"},
		{name: "symbolic permissions within a width", args: []string{"-c", "%12A|", "file.txt"}, wantOut: "  -rw-r--r--|\n"},
		{name: "symbolic permissions cut to the precision", args: []string{"-c", "%10.10A", "file.txt"}, wantOut: "-rw-r--r--\n"},
		{name: "block count", args: []string{"-c", "%b", "file.txt"}, wantOut: "1\n"},
		{name: "block count of a directory", args: []string{"-c", "%b", "dir"}, wantOut: "0\n"},
		{name: "block count measured by the size", args: []string{"-c", "%b", "big"}, wantOut: "3\n"},
		{name: "block size", args: []string{"-c", "%B", "file.txt"}, wantOut: "512\n"},
		{name: "device number in decimal", args: []string{"-c", "%d", "file.txt"}, wantOut: "0\n"},
		{name: "device number in hex", args: []string{"-c", "%D", "file.txt"}, wantOut: "0\n"},
		{name: "device major in decimal", args: []string{"-c", "%Hd", "file.txt"}, wantOut: "0\n"},
		{name: "device minor in decimal", args: []string{"-c", "%Ld", "file.txt"}, wantOut: "0\n"},
		{name: "raw mode in hex", args: []string{"-c", "%f", "file.txt"}, wantOut: "81a4\n"},
		{name: "raw mode of a symbolic link", args: []string{"-c", "%f", "ln"}, wantOut: "a1ff\n"},
		{name: "raw mode in hex with a hash flag", args: []string{"-c", "%#f", "file.txt"}, wantOut: "0x81a4\n"},
		{name: "file type", args: []string{"-c", "%F", "file.txt"}, wantOut: "regular file\n"},
		{name: "file type of a directory", args: []string{"-c", "%F", "dir"}, wantOut: "directory\n"},
		{name: "file type of a symbolic link", args: []string{"-c", "%F", "ln"}, wantOut: "symbolic link\n"},
		{name: "group id", args: []string{"-c", "%g", "file.txt"}, wantOut: "54322\n"},
		{name: "hard link count", args: []string{"-c", "%h", "file.txt"}, wantOut: "1\n"},
		{name: "inode number", args: []string{"-c", "%i", "file.txt"}, wantOut: "0\n"},
		{name: "file name", args: []string{"-c", "%n", "file.txt"}, wantOut: "file.txt\n"},
		{name: "quoted file name left alone", args: []string{"-c", "%Qn", "file.txt"}, wantOut: "file.txt\n"},
		{name: "quoted file name with spaces", args: []string{"-c", "%Qn", "a b"}, files: fsx.MemFS{"a b": {}}, wantOut: "'a b'\n"},
		{name: "access time hint", args: []string{"-c", "%o", "file.txt"}, wantOut: "512\n"},
		{name: "size", args: []string{"-c", "%s", "file.txt"}, wantOut: "5\n"},
		{name: "size of an empty file", args: []string{"-c", "%s", "empty"}, wantOut: "0\n"},
		{name: "size within a width", args: []string{"-c", "%4s|", "file.txt"}, wantOut: "   5|\n"},
		{name: "size left adjusted", args: []string{"-c", "%-4s|", "file.txt"}, wantOut: "5   |\n"},
		{
			name: "access time", args: []string{"-c", "%x", "file.txt"}, env: utc(),
			wantOut: "2023-11-14 22:13:20.500000000 +0000\n",
		},
		{
			name: "access time in seconds", args: []string{"-c", "%X", "file.txt"},
			wantOut: "1700000000\n",
		},
		{
			name: "modification time", args: []string{"-c", "%y", "file.txt"}, env: utc(),
			wantOut: "2023-11-14 22:13:20.500000000 +0000\n",
		},
		{
			name: "modification time in seconds", args: []string{"-c", "%Y", "file.txt"},
			wantOut: "1700000000\n",
		},
		{
			name: "status change time", args: []string{"-c", "%z", "file.txt"}, env: utc(),
			wantOut: "2023-11-14 22:13:20.500000000 +0000\n",
		},
		{
			name: "status change time in seconds", args: []string{"-c", "%Z", "file.txt"},
			wantOut: "1700000000\n",
		},
		{name: "birth time unknown", args: []string{"-c", "%w", "file.txt"}, wantOut: "-\n"},
		{name: "birth time in seconds unknown", args: []string{"-c", "%W", "file.txt"}, wantOut: "0\n"},
		{name: "seconds zero padded", args: []string{"-c", "%015Y", "file.txt"}, wantOut: "000001700000000\n"},
		{name: "seconds space aligned", args: []string{"-c", "%15Y|", "file.txt"}, wantOut: "     1700000000|\n"},
		{name: "seconds left aligned", args: []string{"-c", "%-15Y|", "file.txt"}, wantOut: "1700000000     |\n"},
		{name: "seconds with a precision", args: []string{"-c", "%.3Y", "file.txt"}, wantOut: "1700000000.500\n"},
		{name: "seconds with a bare period", args: []string{"-c", "%.Y", "file.txt"}, wantOut: "1700000000.500000000\n"},
		{
			name: "seconds before the Epoch", files: fsx.MemFS{"old": {ModTime: time.Unix(-2, 750000000)}},
			args: []string{"-c", "%.3Y", "old"}, wantOut: "-1.250\n",
		},
		{
			name: "whole seconds before the Epoch", files: fsx.MemFS{"old": {ModTime: time.Unix(-3, 0)}},
			args: []string{"-c", "%.3Y", "old"}, wantOut: "-3.000\n",
		},
		{
			name: "fractional seconds before the Epoch are floored", files: fsx.MemFS{"old": {ModTime: time.Unix(-2, 748100000)}},
			args: []string{"-c", "%.1Y", "old"}, wantOut: "-1.3\n",
		},
		{
			name: "a precision beyond the nanoseconds is zero padded", files: fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			args:    []string{"-c", "%.20Y", "file.txt"},
			wantOut: "1700000000.50000000000000000000\n",
		},
		{
			name: "a precision beyond the nanoseconds of a second before the Epoch", files: fsx.MemFS{"old": {ModTime: time.Unix(-2, 748100000)}},
			args: []string{"-c", "%.20Y", "old"}, wantOut: "-1.25190000000000000000\n",
		},
		{
			name: "a precision beyond the nanoseconds is floored before the Epoch", files: fsx.MemFS{"old": {ModTime: time.Unix(-2, 748100000)}},
			args: []string{"-c", "%.25Y", "old"}, wantOut: "-1.2519000000000000000000000\n",
		},
		{
			name: "an exact tenth before the Epoch is kept", files: fsx.MemFS{"old": {ModTime: time.Unix(-2, 800000000)}},
			args: []string{"-c", "%.1Y", "old"}, wantOut: "-1.2\n",
		},
		{
			name: "a fraction of a second before the Epoch is floored", files: fsx.MemFS{"old": {ModTime: time.Unix(-1, 250000000)}},
			args: []string{"-c", "%.1Y", "old"}, wantOut: "-0.8\n",
		},
		{
			name: "a second before the Epoch within a hundredth", files: fsx.MemFS{"old": {ModTime: time.Unix(-1, 950000000)}},
			args: []string{"-c", "%.2Y", "old"}, wantOut: "-0.05\n",
		},
		{
			name: "seconds with a width and a precision", args: []string{"-c", "[%22.3Y]", "file.txt"},
			wantOut: "[        1700000000.500]\n",
		},
		{
			name: "seconds zero padded with a precision", args: []string{"-c", "[%022.3Y]", "file.txt"},
			wantOut: "[000000001700000000.500]\n",
		},
		{
			name: "time with a precision", args: []string{"-c", "%.3x", "file.txt"}, env: utc(),
			wantOut: "2023-11-14 22:13:20.500 +0000\n",
		},
		{
			name: "time without a fraction", args: []string{"-c", "%.0x", "file.txt"}, env: utc(),
			wantOut: "2023-11-14 22:13:20 +0000\n",
		},
		{name: "unknown directive", args: []string{"-c", "%q|", "file.txt"}, wantOut: "?|\n"},
		{name: "unknown directive with flags", args: []string{"-c", "%5q|", "file.txt"}, wantOut: "?|\n"},
		{name: "modifier without a pair", args: []string{"-c", "%H|", "file.txt"}, wantOut: "?|\n"},
		{name: "percent at the end", args: []string{"-c", "a%", "file.txt"}, wantOut: "a%\n"},
		{name: "percent signs kept", args: []string{"-c", "a%%b|", "file.txt"}, wantOut: "a%b|\n"},
		{name: "several fields on a line", args: []string{"-c", "%d:%i", "file.txt", "dir"}, wantOut: "0:0\n0:0\n"},
	}
	runCases(t, withFiles(tests, files))
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
	runCases(t, []testCase{
		{
			name:    "writes no trailing newline",
			files:   files,
			args:    []string{"--printf", "%n", "file.txt"},
			wantOut: "file.txt",
		},
		{
			name:    "interprets the named escapes",
			files:   files,
			args:    []string{"--printf", `%n\a\b\e\f\n\r\t\v\\%n`, "file.txt"},
			wantOut: "file.txt\a\b\x1b\f\n\r\t\v\\file.txt",
		},
		{
			name:    "interprets the octal and hexadecimal escapes",
			files:   files,
			args:    []string{"--printf", `A\0101B\x42C%n`, "file.txt"},
			wantOut: "AABBCfile.txt",
		},
		{
			name:     "warns about an unrecognized escape",
			files:    files,
			args:     []string{"--printf", `\q%n`, "file.txt"},
			wantOut:  "qfile.txt",
			wantErr:  "stat: warning: unrecognized escape '\\q'\n",
			wantExit: 0,
		},
		{
			name:     "warns about a backslash at the end",
			files:    files,
			args:     []string{"--printf", `a\`, "file.txt"},
			wantOut:  "a\\",
			wantErr:  "stat: warning: backslash at end of format\n",
			wantExit: 0,
		},
		{
			name:    "keeps the backslashes of --format",
			files:   files,
			args:    []string{"-c", `a\nb`, "file.txt"},
			wantOut: "a\\nb\n",
		},
		{
			name:    "newline terminates every use of --format",
			files:   files,
			args:    []string{"-c", "%n", "file.txt", "file.txt"},
			wantOut: "file.txt\nfile.txt\n",
		},
		{
			name:    "the format of the last option wins",
			files:   files,
			args:    []string{"-c", "%a", "--printf", "%s%n", "file.txt"},
			wantOut: "5file.txt",
		},
		{
			name:    "the format of the last option wins the other way",
			files:   files,
			args:    []string{"--printf", "%s", "-c", "%a", "file.txt"},
			wantOut: "644\n",
		},
		{
			name:    "an empty --format writes an empty line",
			files:   files,
			args:    []string{"-c", "", "file.txt", "file.txt"},
			wantOut: "\n\n",
		},
		{
			name:    "an empty --printf writes nothing",
			files:   files,
			args:    []string{"--printf", "", "file.txt", "file.txt"},
			wantOut: "",
		},
		{
			name:    "an empty --printf keeps the last of the options",
			files:   files,
			args:    []string{"-c", "%a", "--printf", "", "file.txt"},
			wantOut: "",
		},
	})
}

func TestRunTerse(t *testing.T) {
	runCases(t, []testCase{
		{
			name:  "writes one line of fields per file",
			files: fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID}},
			args:  []string{"-t", "file.txt"},
			wantOut: "file.txt 5 1 81a4 54321 54322 0 0 1 0 0 " +
				"1700000000 1700000000 1700000000 0 512\n",
		},
		{
			name:  "quotes names that need it",
			files: fsx.MemFS{"a b": {Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID}},
			args:  []string{"-t", "a b"},
			wantOut: "'a b' 0 0 81a4 54321 54322 0 0 1 0 0 " +
				"1700000000 1700000000 1700000000 0 512\n",
		},
		{
			name:    "gives way to an explicit format",
			files:   fsx.MemFS{"file.txt": {Mode: 0o644, ModTime: modTime}},
			args:    []string{"-t", "-c", "%n", "file.txt"},
			wantOut: "file.txt\n",
		},
	})
}

func TestRunFileSystem(t *testing.T) {
	runCases(t, []testCase{
		{
			name:  "writes the file system fields it doesn't know as marks",
			files: fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			args:  []string{"-f", "file.txt"},
			wantOut: "  File: file.txt\n" +
				"    ID: ? Namelen: ? Type: ?\n" +
				"Block size: ? Fundamental block size: ?\n" +
				"Blocks: Total: ? Free: ? Available: ?\n" +
				"Inodes: Total: ? Free: ?\n",
		},
		{
			name:    "writes the terse fields of the file system",
			files:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			args:    []string{"-f", "-t", "file.txt"},
			wantOut: "file.txt ? ? ? ? ? ? ? ? ? ?\n",
		},
		{
			name:    "reports the file name only when asked for",
			files:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			args:    []string{"-f", "-c", "%n %s", "file.txt"},
			wantOut: "file.txt ?\n",
		},
		{
			name:    "follows a link to the file system it is on",
			files:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}, "ln": link("file.txt")},
			args:    []string{"-f", "-c", "%n", "ln"},
			wantOut: "ln\n",
		},
		{
			name:     "diagnoses a missing file",
			files:    fsx.MemFS{},
			args:     []string{"-f", "gone"},
			wantErr:  "stat: cannot read file system information for 'gone': file does not exist\n",
			wantExit: 1,
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
	runCases(t, []testCase{
		{
			name:    "reports the link itself by default",
			files:   files,
			args:    []string{"-c", "%F %s %N", "tofile"},
			wantOut: "symbolic link 8 tofile -> file.txt\n",
		},
		{
			name:    "reports the file a link refers to with -L",
			files:   files,
			args:    []string{"-L", "-c", "%F %s %N", "tofile"},
			wantOut: "regular file 5 tofile\n",
		},
		{
			name:    "reports a link to a directory with -L",
			files:   files,
			args:    []string{"-L", "-c", "%F", "todir"},
			wantOut: "directory\n",
		},
		{
			name:    "reports the file a chain of links names with -L",
			files:   files,
			args:    []string{"-L", "-c", "%F", "chain"},
			wantOut: "regular file\n",
		},
		{
			name:     "diagnoses a broken link with -L",
			files:    files,
			args:     []string{"-L", "broken"},
			wantErr:  "stat: cannot stat 'broken': file does not exist\n",
			wantExit: 1,
		},
		{
			name:    "reports a broken link without -L",
			files:   files,
			args:    []string{"-c", "%F %N", "broken"},
			wantOut: "symbolic link broken -> gone\n",
		},
		{
			name:    "quotes the link and its target",
			files:   fsx.MemFS{"a b": link("c d")},
			args:    []string{"-c", "%N", "a b"},
			wantOut: "'a b' -> 'c d'\n",
		},
		{
			name:    "long option form of the dereference",
			files:   files,
			args:    []string{"--dereference", "-c", "%F", "tofile"},
			wantOut: "regular file\n",
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
			runCase(t, testCase{
				name:    test.name,
				files:   test.files,
				env:     map[string]string{"TZ": "UTC", "QUOTING_STYLE": test.style},
				args:    test.args,
				wantOut: test.want,
			})
		})
	}
}

func TestRunQuotingDefault(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "quotes a control character by default",
			files:   fsx.MemFS{"a\tb": {}},
			args:    []string{"-c", "%Qn", "a\tb"},
			wantOut: "$'a\\tb'\n",
		},
		{
			name:     "warns about an invalid style and keeps the default",
			files:    fsx.MemFS{"a b": {}},
			env:      map[string]string{"QUOTING_STYLE": "weird"},
			args:     []string{"-c", "%Qn", "a b"},
			wantOut:  "'a b'\n",
			wantErr:  "stat: ignoring invalid value of environment variable QUOTING_STYLE: weird\n",
			wantExit: 0,
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
	runCases(t, []testCase{
		{
			name:     "needs an operand",
			files:    fsx.MemFS{"file.txt": {}},
			wantErr:  "stat: missing operand\nTry 'stat --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "rejects an unknown short option",
			files:    fsx.MemFS{"file.txt": {}},
			args:     []string{"-x", "file.txt"},
			wantErr:  "stat: invalid option -- 'x'\nTry 'stat --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "rejects an unknown option in a group",
			files:    fsx.MemFS{"file.txt": {}},
			args:     []string{"-tx", "file.txt"},
			wantErr:  "stat: invalid option -- 'x'\nTry 'stat --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "rejects an unknown long option",
			files:    fsx.MemFS{"file.txt": {}},
			args:     []string{"--frobnicate", "file.txt"},
			wantErr:  "stat: unrecognized option '--frobnicate'\nTry 'stat --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "needs the argument of --format",
			files:    fsx.MemFS{"file.txt": {}},
			args:     []string{"--format"},
			wantErr:  "stat: option '--format' requires an argument\nTry 'stat --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "needs the argument of -c",
			files:    fsx.MemFS{"file.txt": {}},
			args:     []string{"-c"},
			wantErr:  "stat: option requires an argument -- 'c'\nTry 'stat --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "rejects an invalid --cached value",
			files:    fsx.MemFS{"file.txt": {}},
			args:     []string{"--cached=sometimes", "file.txt"},
			wantErr:  "stat: invalid argument 'sometimes' for '--cached'\nTry 'stat --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "takes a fatal invalid directive with flags",
			files:    fsx.MemFS{"file.txt": {}},
			args:     []string{"-c", "%5%", "file.txt"},
			wantErr:  "stat: '%5%': invalid directive\n",
			wantExit: 1,
		},
		{
			name:     "writes the text before an invalid directive",
			files:    fsx.MemFS{"file.txt": {}},
			args:     []string{"-c", "abc%5%", "file.txt"},
			wantOut:  "abc",
			wantErr:  "stat: '%5%': invalid directive\n",
			wantExit: 1,
		},
		{
			name:     "diagnoses a directive the format ends within",
			files:    fsx.MemFS{"file.txt": {}},
			args:     []string{"-c", "%5", "file.txt"},
			wantErr:  "stat: '%5': invalid directive\n",
			wantExit: 1,
		},
		{
			name:     "rejects an argument given to a flag option",
			files:    fsx.MemFS{"file.txt": {}},
			args:     []string{"--terse=1", "file.txt"},
			wantErr:  "stat: option '--terse=1' doesn't allow an argument\nTry 'stat --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "diagnoses a missing file and goes on",
			files:    fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			args:     []string{"-c", "%n", "gone", "file.txt"},
			wantOut:  "file.txt\n",
			wantErr:  "stat: cannot stat 'gone': file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "reports an invalid zero-length file name",
			files:    fsx.MemFS{"file.txt": {}},
			args:     []string{""},
			wantErr:  "stat: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "reports no file system available",
			args:     []string{"file.txt"},
			wantErr:  "stat: no file system available\n",
			wantExit: 1,
		},
	})
}

func TestRunCachedOption(t *testing.T) {
	files := fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}}
	runCases(t, []testCase{
		{
			name:    "accepts the modes of --cached",
			files:   files,
			args:    []string{"--cached=always", "--cached=never", "--cached=default", "-c", "%n", "file.txt"},
			wantOut: "file.txt\n",
		},
		{
			name:    "ends the options at --",
			files:   fsx.MemFS{"-weird": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			args:    []string{"-c", "%n", "--", "-weird"},
			wantOut: "-weird\n",
		},
	})
}

func TestRunStdOptions(t *testing.T) {
	runCases(t, []testCase{
		{
			name:     "writes the usage for --help",
			args:     []string{"--help"},
			wantOut:  usage,
			wantExit: 0,
		},
		{
			name:     "writes the usage for the short form of --help",
			args:     []string{"-L", "--help"},
			wantOut:  usage,
			wantExit: 0,
		},
		{
			name:     "writes the version for --version",
			args:     []string{"--version"},
			wantOut:  "stat (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n",
			wantExit: 0,
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
	runCases(t, []testCase{
		{
			name:    "writes the timestamps in the zone TZ names",
			files:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			env:     map[string]string{"TZ": "America/New_York"},
			args:    []string{"-c", "%x", "file.txt"},
			wantOut: "2023-11-14 17:13:20.500000000 -0500\n",
		},
		{
			name:    "falls back to the local zone for an unknown TZ",
			files:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			env:     map[string]string{"TZ": "Nowhere/Bad"},
			args:    []string{"-c", "%Y", "file.txt"},
			wantOut: "1700000000\n",
		},
	})
}

func TestRunUsersAndGroups(t *testing.T) {
	runCases(t, []testCase{
		{
			name:    "reports UNKNOWN for an owner without a name",
			files:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime, Uid: ownerID, Gid: groupID}},
			args:    []string{"-c", "%U %G", "file.txt"},
			wantOut: "UNKNOWN UNKNOWN\n",
		},
		{
			name:  "reports the owner a system names",
			files: fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime, Uid: 0}},
			args: func() []string {
				return []string{"-c", "%u %U", "file.txt"}
			}(),
			wantOut: func() string {
				if runtime.GOOS == "windows" {
					return "0 UNKNOWN\n"
				}
				return "0 root\n"
			}(),
		},
		{
			name:    "reports the owners of a file without owner information as zero",
			files:   fsx.MemFS{"file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			args:    []string{"-c", "%u %g", "file.txt"},
			wantOut: "0 0\n",
		},
	})
}

func TestRunRealFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCases(t, []testCase{
		{
			name:    "reports the size and type of a real file",
			sys:     fsx.OsFS(""),
			wd:      dir,
			env:     utc(),
			args:    []string{"-c", "%n %s %F", "file.txt"},
			wantOut: "file.txt 5 regular file\n",
		},
	})
	if runtime.GOOS != "windows" {
		runCases(t, []testCase{
			{
				name: "reports the inode and device of a real file",
				sys:  fsx.OsFS(""),
				wd:   dir,
				env:  utc(),
				args: []string{"-c", "%i %D %d", "file.txt"},
				wantOut: func() string {
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
	runCases(t, []testCase{
		{
			name:    "takes the operands with Windows separators",
			files:   fsx.MemFS{"dir/file.txt": {Data: []byte("hello"), Mode: 0o644, ModTime: modTime}},
			env:     utc(),
			args:    []string{"-c", "%n %s", `dir\file.txt`},
			wantOut: `dir\file.txt 5` + "\n",
		},
	})
}
