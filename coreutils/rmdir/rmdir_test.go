package rmdir

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:   "removes an empty directory",
			FS:     fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Args:   []string{"d"},
			Absent: []string{"d"},
		},
		{
			Name:   "removes every directory in order",
			FS:     fsx.MemFS{"one": {Mode: fs.ModeDir}, "two": {Mode: fs.ModeDir}},
			Args:   []string{"one", "two"},
			Absent: []string{"one", "two"},
		},
		{
			Name:   "removes a subdirectory before its parent",
			FS:     fsx.MemFS{"a": {Mode: fs.ModeDir}, "a/b": {Mode: fs.ModeDir}},
			Args:   []string{"a/b", "a"},
			Absent: []string{"a", "a/b"},
		},
		{
			Name:     "a parent before the subdirectory is an error",
			FS:       fsx.MemFS{"a": {Mode: fs.ModeDir}, "a/b": {Mode: fs.ModeDir}},
			Args:     []string{"a", "a/b"},
			Present:  []string{"a"},
			Absent:   []string{"a/b"},
			WantErr:  "rmdir: a: directory not empty\n",
			WantExit: 1,
		},
		{
			Name:     "a missing directory is an error",
			FS:       fsx.MemFS{},
			Args:     []string{"missing"},
			WantErr:  "rmdir: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a file is an error",
			FS:       fsx.MemFS{"file.txt": {Data: []byte("hello")}},
			Args:     []string{"file.txt"},
			Present:  []string{"file.txt"},
			WantErr:  "rmdir: file.txt: not a directory\n",
			WantExit: 1,
		},
		{
			Name:     "a non-empty directory is an error",
			FS:       fsx.MemFS{"d/keep.txt": {Data: []byte("keep")}},
			Args:     []string{"d"},
			Present:  []string{"d/keep.txt"},
			WantErr:  "rmdir: d: directory not empty\n",
			WantExit: 1,
		},
		{
			Name:     "keeps removing the other directories after an error",
			FS:       fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Args:     []string{"missing", "d"},
			Absent:   []string{"d"},
			WantErr:  "rmdir: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:   "a trailing slash is dropped",
			FS:     fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Args:   []string{"d/"},
			Absent: []string{"d"},
		},
		{
			Name:   "redundant separators are dropped",
			FS:     fsx.MemFS{"a/d": {Mode: fs.ModeDir}},
			Args:   []string{"a//d"},
			Absent: []string{"a/d"},
		},
		{
			Name:   "a directory named like an option after a double dash",
			FS:     fsx.MemFS{"-p": {Mode: fs.ModeDir}},
			Args:   []string{"--", "-p"},
			Absent: []string{"-p"},
		},
		{
			Name:   "a directory named with a dash",
			FS:     fsx.MemFS{"-": {Mode: fs.ModeDir}},
			Args:   []string{"-"},
			Absent: []string{"-"},
		},
		{
			Name:     "the current directory cannot be removed",
			FS:       fsx.MemFS{},
			Args:     []string{"."},
			WantErr:  "rmdir: .: invalid argument\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunParents(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "removes every component of the path",
			FS: fsx.MemFS{
				"a":     {Mode: fs.ModeDir},
				"a/b":   {Mode: fs.ModeDir},
				"a/b/c": {Mode: fs.ModeDir},
			},
			Args:   []string{"-p", "a/b/c"},
			Absent: []string{"a", "a/b", "a/b/c"},
		},
		{
			Name: "stops at a non-empty directory",
			FS: fsx.MemFS{
				"a":          {Mode: fs.ModeDir},
				"a/keep.txt": {Data: []byte("keep")},
				"a/b":        {Mode: fs.ModeDir},
				"a/b/c":      {Mode: fs.ModeDir},
			},
			Args:     []string{"-p", "a/b/c"},
			Present:  []string{"a", "a/keep.txt"},
			Absent:   []string{"a/b", "a/b/c"},
			WantErr:  "rmdir: a: directory not empty\n",
			WantExit: 1,
		},
		{
			Name:   "a single component is only the directory itself",
			FS:     fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Args:   []string{"-p", "d"},
			Absent: []string{"d"},
		},
		{
			Name:     "an intermediate missing directory is an error",
			FS:       fsx.MemFS{"a": {Mode: fs.ModeDir}},
			Args:     []string{"-p", "a/b"},
			Present:  []string{"a"},
			WantErr:  "rmdir: a/b: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "keeps removing the other directories after an error",
			FS:       fsx.MemFS{"a": {Mode: fs.ModeDir}, "b": {Mode: fs.ModeDir}, "b/c": {Mode: fs.ModeDir}},
			Args:     []string{"-p", "a/b", "b/c"},
			Present:  []string{"a"},
			Absent:   []string{"b", "b/c"},
			WantErr:  "rmdir: a/b: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:   "repeated p options",
			FS:     fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Args:   []string{"-pp", "d"},
			Absent: []string{"d"},
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "parents removes every component of the path",
			FS: fsx.MemFS{
				"a":     {Mode: fs.ModeDir},
				"a/b":   {Mode: fs.ModeDir},
				"a/b/c": {Mode: fs.ModeDir},
			},
			Args:   []string{"--parents", "a/b/c"},
			Absent: []string{"a", "a/b", "a/b/c"},
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunIgnoreFailOnNonEmpty(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:    "a non-empty directory is ignored",
			FS:      fsx.MemFS{"d/keep.txt": {Data: []byte("keep")}},
			Args:    []string{"--ignore-fail-on-non-empty", "d"},
			Present: []string{"d/keep.txt"},
		},
		{
			Name: "a non-empty directory stops the parents quietly",
			FS: fsx.MemFS{
				"a":          {Mode: fs.ModeDir},
				"a/keep.txt": {Data: []byte("keep")},
				"a/b":        {Mode: fs.ModeDir},
				"a/b/c":      {Mode: fs.ModeDir},
			},
			Args:    []string{"--ignore-fail-on-non-empty", "-p", "a/b/c"},
			Present: []string{"a", "a/keep.txt"},
			Absent:  []string{"a/b", "a/b/c"},
		},
		{
			Name:     "a missing directory is still an error",
			FS:       fsx.MemFS{},
			Args:     []string{"--ignore-fail-on-non-empty", "missing"},
			WantErr:  "rmdir: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a file is still an error",
			FS:       fsx.MemFS{"file.txt": {Data: []byte("hello")}},
			Args:     []string{"--ignore-fail-on-non-empty", "file.txt"},
			Present:  []string{"file.txt"},
			WantErr:  "rmdir: file.txt: not a directory\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunVerbose(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:   "prints every removed directory",
			FS:     fsx.MemFS{"one": {Mode: fs.ModeDir}, "two": {Mode: fs.ModeDir}},
			Args:   []string{"-v", "one", "two"},
			Absent: []string{"one", "two"},
			Want:   "rmdir: removing directory, 'one'\nrmdir: removing directory, 'two'\n",
		},
		{
			Name: "prints every component of the path",
			FS: fsx.MemFS{
				"a":     {Mode: fs.ModeDir},
				"a/b":   {Mode: fs.ModeDir},
				"a/b/c": {Mode: fs.ModeDir},
			},
			Args:   []string{"--verbose", "-p", "a/b/c"},
			Absent: []string{"a", "a/b", "a/b/c"},
			Want:   "rmdir: removing directory, 'a/b/c'\nrmdir: removing directory, 'a/b'\nrmdir: removing directory, 'a'\n",
		},
		{
			Name:     "announces a directory which fails to be removed",
			FS:       fsx.MemFS{"d/keep.txt": {Data: []byte("keep")}},
			Args:     []string{"-v", "d"},
			Present:  []string{"d/keep.txt"},
			Want:     "rmdir: removing directory, 'd'\n",
			WantErr:  "rmdir: d: directory not empty\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "missing operand",
			Args:     []string{},
			WantErr:  "rmdir: missing operand\nTry 'rmdir --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option",
			FS:       fsx.MemFS{},
			Args:     []string{"-x"},
			WantErr:  "rmdir: invalid option -- 'x'\nTry 'rmdir --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unrecognized long option",
			FS:       fsx.MemFS{"d": {Mode: fs.ModeDir}},
			Args:     []string{"--bogus", "d"},
			Present:  []string{"d"},
			WantErr:  "rmdir: unrecognized option '--bogus'\nTry 'rmdir --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rt := &coreutils.Runtime{Stderr: io.Discard, FS: fsx.MemFS{}}
	err := New(rt).Run(ctx, "d")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "rmdir", usage)
}
