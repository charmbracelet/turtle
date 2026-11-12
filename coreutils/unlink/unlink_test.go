package unlink

import (
	"io/fs"
	"testing"

	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

func TestRun(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := []testutil.Case{
			{
				Name:   "removes a file",
				FS:     fsx.MemFS{"a.txt": {Data: []byte("hello")}},
				Args:   []string{"a.txt"},
				Absent: []string{"a.txt"},
			},
			{
				Name: "removes a symbolic link itself",
				FS: fsx.MemFS{
					"a.txt":  {Data: []byte("hello")},
					"b.link": {Data: []byte("a.txt"), Mode: fs.ModeSymlink},
				},
				Args:    []string{"b.link"},
				Absent:  []string{"b.link"},
				Present: []string{"a.txt"},
			},
			{
				Name: "removes a symbolic link to a directory itself",
				FS: fsx.MemFS{
					"d/a.txt": {Data: []byte("hello")},
					"link":    {Data: []byte("d"), Mode: fs.ModeSymlink},
				},
				Args:    []string{"link"},
				Absent:  []string{"link"},
				Present: []string{"d/a.txt"},
			},
			{
				Name:   "resolves the operand against the working directory",
				FS:     fsx.MemFS{"d/a.txt": {Data: []byte("hello")}},
				Dir:    "d",
				Args:   []string{"a.txt"},
				Absent: []string{"d/a.txt"},
			},
			{
				Name:   "an operand starting with a dash is a file",
				FS:     fsx.MemFS{"-f.txt": {Data: []byte("hello")}},
				Args:   []string{"-f.txt"},
				Absent: []string{"-f.txt"},
			},
		}
		testutil.Run(t, New, tests)
	})

	t.Run("failures", func(t *testing.T) {
		tests := []testutil.Case{
			{
				Name:     "a directory is not removed",
				FS:       fsx.MemFS{"d/a.txt": {Data: []byte("hello")}},
				Args:     []string{"d"},
				Present:  []string{"d/a.txt"},
				WantErr:  "unlink: d: is a directory\n",
				WantExit: 1,
			},
			{
				Name:     "an empty directory is not removed either",
				FS:       fsx.MemFS{"d": {Mode: fs.ModeDir}},
				Args:     []string{"d"},
				Present:  []string{"d"},
				WantErr:  "unlink: d: is a directory\n",
				WantExit: 1,
			},
			{
				Name:     "a trailing slash makes the operand a directory reference",
				FS:       fsx.MemFS{"a.txt": {Data: []byte("hello")}},
				Args:     []string{"a.txt/"},
				Present:  []string{"a.txt"},
				WantErr:  "unlink: a.txt/: invalid argument\n",
				WantExit: 1,
			},
			{
				Name: "a dot-dot isn't resolved lexically away from a symlink",
				FS: fsx.MemFS{
					"a.txt": {Data: []byte("hello")},
					"s":     {Data: []byte("d"), Mode: fs.ModeSymlink},
				},
				Args:     []string{"s/../a.txt"},
				Present:  []string{"a.txt"},
				WantErr:  "unlink: s/../a.txt: invalid argument\n",
				WantExit: 1,
			},
			{
				Name:     "a missing file is an error",
				FS:       fsx.MemFS{},
				Args:     []string{"missing"},
				WantErr:  "unlink: missing: file does not exist\n",
				WantExit: 1,
			},
			{
				Name:     "a missing operand starting with a dash is an error",
				FS:       fsx.MemFS{},
				Args:     []string{"-f"},
				WantErr:  "unlink: -f: file does not exist\n",
				WantExit: 1,
			},
			{
				Name:     "a double dash is an operand",
				FS:       fsx.MemFS{},
				Args:     []string{"--"},
				WantErr:  "unlink: --: file does not exist\n",
				WantExit: 1,
			},
			{
				Name:     "an empty operand is an error",
				FS:       fsx.MemFS{},
				Args:     []string{""},
				WantErr:  "unlink: invalid zero-length file name\n",
				WantExit: 1,
			},
			{
				Name:     "no operand is an error",
				FS:       fsx.MemFS{},
				Args:     nil,
				WantErr:  "unlink: missing operand\nTry 'unlink --help' for more information.\n",
				WantExit: 2,
			},
			{
				Name:     "an extra operand is an error",
				FS:       fsx.MemFS{"a.txt": {Data: []byte("a")}, "b.txt": {Data: []byte("b")}},
				Args:     []string{"a.txt", "b.txt"},
				Present:  []string{"a.txt", "b.txt"},
				WantErr:  "unlink: extra operand 'b.txt'\nTry 'unlink --help' for more information.\n",
				WantExit: 2,
			},
			{
				Name:     "an extra operand after a double dash is an error",
				FS:       fsx.MemFS{"a.txt": {Data: []byte("a")}},
				Args:     []string{"--", "a.txt"},
				Present:  []string{"a.txt"},
				WantErr:  "unlink: extra operand 'a.txt'\nTry 'unlink --help' for more information.\n",
				WantExit: 2,
			},
		}
		testutil.Run(t, New, tests)
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "unlink", usage)
}
