package patch

import (
	"strings"
	"testing"

	"charm.land/snail/coreutils/internal/diff"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

// unifiedPatch builds the unified patch that turns a into b, like diff -u
// writes it.
func unifiedPatch(name1, name2, a, b string) string {
	return renderPatch(name1, name2, a, b, true)
}

// contextPatch builds the copied context patch that turns a into b, like
// diff -c writes it.
func contextPatch(name1, name2, a, b string) string {
	return renderPatch(name1, name2, a, b, false)
}

func renderPatch(name1, name2, a, b string, unified bool) string {
	la, lb := diff.SplitLines([]byte(a)), diff.SplitLines([]byte(b))
	changes := diff.Lines(la, lb, nil)
	var out strings.Builder
	if unified {
		diff.Unified(&out, diff.File{Name: name1}, diff.File{Name: name2}, la, lb, changes, 3)
	} else {
		diff.Context(&out, diff.File{Name: name1}, diff.File{Name: name2}, la, lb, changes, 3)
	}
	return out.String()
}

// checkFile asserts the contents of a file of the test file system.
func checkFile(t *testing.T, fsys fsx.MemFS, name, want string) {
	t.Helper()

	f, ok := fsys[name]
	if !ok {
		t.Errorf("file %s doesn't exist", name)
		return
	}
	if got := string(f.Data); got != want {
		t.Errorf("file %s = %q, want %q", name, got, want)
	}
}

// checkMissing asserts that no file of the given name exists.
func checkMissing(t *testing.T, fsys fsx.MemFS, name string) {
	t.Helper()

	if _, ok := fsys[name]; ok {
		t.Errorf("file %s exists, want missing", name)
	}
}

func TestRunApply(t *testing.T) {
	tests := []struct {
		Name     string
		Patch    string
		Contents string
		Args     []string
		Want     string
		WantErr  string
		WantExit int
	}{
		{
			Name:     "unified patch through the operand",
			Patch:    unifiedPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n"),
			Contents: "1\n2\n3\n",
			Args:     []string{"f"},
			Want:     "1\nx\n3\n",
			WantErr:  "patching file f\n",
		},
		{
			Name:     "unified patch through the header names",
			Patch:    unifiedPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n"),
			Contents: "1\n2\n3\n",
			Want:     "1\nx\n3\n",
			WantErr:  "patching file f\n",
		},
		{
			Name:     "context patch",
			Patch:    contextPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n"),
			Contents: "1\n2\n3\n",
			Args:     []string{"f"},
			Want:     "1\nx\n3\n",
			WantErr:  "patching file f\n",
		},
		{
			Name:     "forced unified format",
			Patch:    unifiedPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n"),
			Contents: "1\n2\n3\n",
			Args:     []string{"-u", "f"},
			Want:     "1\nx\n3\n",
			WantErr:  "patching file f\n",
		},
		{
			Name:     "forced context format",
			Patch:    contextPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n"),
			Contents: "1\n2\n3\n",
			Args:     []string{"-c", "f"},
			Want:     "1\nx\n3\n",
			WantErr:  "patching file f\n",
		},
		{
			Name:     "insertion and deletion",
			Patch:    unifiedPatch("f", "f", "1\n2\n4\n5\n", "1\nx\n4\n"),
			Contents: "1\n2\n4\n5\n",
			Args:     []string{"f"},
			Want:     "1\nx\n4\n",
			WantErr:  "patching file f\n",
		},
		{
			Name:     "missing newline at end of file",
			Patch:    unifiedPatch("f", "f", "a\nb", "a\nc"),
			Contents: "a\nb",
			Args:     []string{"f"},
			Want:     "a\nc",
			WantErr:  "patching file f\n",
		},
		{
			Name:     "newline added at end of file",
			Patch:    unifiedPatch("f", "f", "a\nb", "a\nb\n"),
			Contents: "a\nb",
			Args:     []string{"f"},
			Want:     "a\nb\n",
			WantErr:  "patching file f\n",
		},
		{
			Name:     "patch from a file",
			Patch:    unifiedPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n"),
			Contents: "1\n2\n3\n",
			Args:     []string{"-i", "p.patch"},
			Want:     "1\nx\n3\n",
			WantErr:  "patching file f\n",
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			fsys := fsx.MemFS{"f": {Data: []byte(test.Contents)}}
			if strings.Contains(test.Name, "from a file") {
				fsys["p.patch"] = &fsx.MemFile{Data: []byte(test.Patch)}
			}
			testutil.Once(t, New, testutil.Case{
				Name:     test.Name,
				Stdin:    strings.NewReader(test.Patch),
				FS:       fsys,
				Args:     test.Args,
				WantErr:  test.WantErr,
				WantExit: test.WantExit,
			})
			checkFile(t, fsys, "f", test.Want)
		})
	}
}

func TestRunMultiFile(t *testing.T) {
	patch := unifiedPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n") +
		"diff -u a/g b/g\n" +
		unifiedPatch("a/g", "b/g", "q\nw\n", "q\nz\n")
	fsys := fsx.MemFS{
		"f": {Data: []byte("1\n2\n3\n")},
		"g": {Data: []byte("q\nw\n")},
	}
	testutil.Once(t, New, testutil.Case{
		Name:    "two files",
		Stdin:   strings.NewReader(patch),
		FS:      fsys,
		WantErr: "patching file f\npatching file g\n",
	})
	checkFile(t, fsys, "f", "1\nx\n3\n")
	checkFile(t, fsys, "g", "q\nz\n")
}

func TestRunSearch(t *testing.T) {
	t.Run("hunk moved by added lines", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("z\ny\n1\n2\n3\n4\n5\n")}}
		testutil.Once(t, New, testutil.Case{
			Name: "offset",
			Stdin: strings.NewReader(unifiedPatch("f", "f",
				"1\n2\n3\n4\n5\n", "1\nX\n3\n4\n5\n")),
			FS:      fsys,
			Args:    []string{"f"},
			WantErr: "patching file f\nHunk #1 succeeded at 3 (offset 2 lines).\n",
		})
		checkFile(t, fsys, "f", "z\ny\n1\nX\n3\n4\n5\n")
	})

	t.Run("hunk matching with fuzz", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n4\n9\n")}}
		testutil.Once(t, New, testutil.Case{
			Name: "fuzz",
			Stdin: strings.NewReader(unifiedPatch("f", "f",
				"1\n2\n3\n4\n5\n", "1\nX\n3\n4\n5\n")),
			FS:      fsys,
			Args:    []string{"f"},
			WantErr: "patching file f\nHunk #1 succeeded at 2 with fuzz 1 (offset 1 lines).\n",
		})
		checkFile(t, fsys, "f", "1\nX\n3\n4\n9\n")
	})

	t.Run("blank amounts ignored with -l", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2 \n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name: "blank",
			Stdin: strings.NewReader(unifiedPatch("f", "f",
				"1\n2\n3\n", "1\n2\n4\n")),
			FS:      fsys,
			Args:    []string{"-l", "f"},
			WantErr: "patching file f\n",
		})
		checkFile(t, fsys, "f", "1\n2\n4\n")
	})
}

func TestRunRejects(t *testing.T) {
	t.Run("failed hunk saved to the reject file", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n9\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name: "reject",
			Stdin: strings.NewReader(unifiedPatch("f", "f",
				"1\n2\n3\n", "1\nX\n3\n")),
			FS:       fsys,
			Args:     []string{"f"},
			WantErr:  "patching file f\nHunk #1 FAILED at 1.\n1 out of 1 hunks FAILED -- saving rejects to file f.rej\n",
			WantExit: 1,
		})
		checkFile(t, fsys, "f", "1\n9\n3\n")
		checkFile(t, fsys, "f.rej", "@@ -1,3 +1,3 @@\n 1\n-2\n+X\n 3\n")
	})

	t.Run("context rejects keep the context format", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n9\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name: "context reject",
			Stdin: strings.NewReader(contextPatch("f", "f",
				"1\n2\n3\n", "1\nX\n3\n")),
			FS:       fsys,
			Args:     []string{"-c", "f"},
			WantErr:  "patching file f\nHunk #1 FAILED at 1.\n1 out of 1 hunks FAILED -- saving rejects to file f.rej\n",
			WantExit: 1,
		})
		checkFile(t, fsys, "f.rej", "***************\n*** 1,3 ****\n  1\n- 2\n  3\n--- 1,3 ----\n  1\n+ X\n  3\n")
	})

	t.Run("reject file of the -r option", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n9\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name: "custom reject file",
			Stdin: strings.NewReader(unifiedPatch("f", "f",
				"1\n2\n3\n", "1\nX\n3\n")),
			FS:       fsys,
			Args:     []string{"-r", "junk.rej", "f"},
			WantErr:  "patching file f\nHunk #1 FAILED at 1.\n1 out of 1 hunks FAILED -- saving rejects to file junk.rej\n",
			WantExit: 1,
		})
		checkFile(t, fsys, "junk.rej", "@@ -1,3 +1,3 @@\n 1\n-2\n+X\n 3\n")
	})
}

func TestRunReversed(t *testing.T) {
	patch := unifiedPatch("f", "f", "1\n2\n3\n", "1\nx\n3\n")

	t.Run("the -R option applies backwards", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\nx\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name:     "reverse",
			Stdin:    strings.NewReader(patch),
			FS:       fsys,
			Args:     []string{"-R", "f"},
			WantErr:  "patching file f\n",
			WantExit: 0,
		})
		checkFile(t, fsys, "f", "1\n2\n3\n")
	})

	t.Run("an applied patch skipped without -N", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\nx\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name:     "skipped",
			Stdin:    strings.NewReader(patch),
			FS:       fsys,
			Args:     []string{"f"},
			WantErr:  "Reversed (or previously applied) patch detected!  Assume -R? Skipping patch.\n",
			WantExit: 1,
		})
		checkFile(t, fsys, "f", "1\nx\n3\n")
	})

	t.Run("an applied patch skipped with -N", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\nx\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name:     "-N",
			Stdin:    strings.NewReader(patch),
			FS:       fsys,
			Args:     []string{"-N", "f"},
			WantErr:  "Reversed (or previously applied) patch detected!  Skipping patch.\n",
			WantExit: 1,
		})
		checkFile(t, fsys, "f", "1\nx\n3\n")
	})

	t.Run("the prompt applying backwards", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\nx\n3\n")}, "p.patch": {Data: []byte(patch)}}
		testutil.Once(t, New, testutil.Case{
			Name:     "prompt yes",
			Stdin:    strings.NewReader("y\n"),
			FS:       fsys,
			Args:     []string{"-i", "p.patch", "f"},
			WantErr:  "Reversed (or previously applied) patch detected!  Assume -R? patching file f\n",
			WantExit: 0,
		})
		checkFile(t, fsys, "f", "1\n2\n3\n")
	})

	t.Run("the prompt refusing to apply backwards", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\nx\n3\n")}, "p.patch": {Data: []byte(patch)}}
		testutil.Once(t, New, testutil.Case{
			Name:     "prompt no",
			Stdin:    strings.NewReader("n\n"),
			FS:       fsys,
			Args:     []string{"-i", "p.patch", "f"},
			WantErr:  "Reversed (or previously applied) patch detected!  Assume -R? Skipping patch.\n",
			WantExit: 1,
		})
		checkFile(t, fsys, "f", "1\nx\n3\n")
	})
}

func TestRunNames(t *testing.T) {
	patch := unifiedPatch("a/b/f", "c/b/f", "1\n2\n3\n", "1\nx\n3\n")

	t.Run("the basename by default", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name:    "basename",
			Stdin:   strings.NewReader(patch),
			FS:      fsys,
			WantErr: "patching file f\n",
		})
		checkFile(t, fsys, "f", "1\nx\n3\n")
	})

	t.Run("components stripped with -p", func(t *testing.T) {
		fsys := fsx.MemFS{"b/f": {Data: []byte("1\n2\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name:    "p1",
			Stdin:   strings.NewReader(patch),
			FS:      fsys,
			Args:    []string{"-p1"},
			WantErr: "patching file b/f\n",
		})
		checkFile(t, fsys, "b/f", "1\nx\n3\n")
	})

	t.Run("the whole path with -p0", func(t *testing.T) {
		fsys := fsx.MemFS{"a/b/f": {Data: []byte("1\n2\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name:    "p0",
			Stdin:   strings.NewReader(patch),
			FS:      fsys,
			Args:    []string{"-p0"},
			WantErr: "patching file a/b/f\n",
		})
		checkFile(t, fsys, "a/b/f", "1\nx\n3\n")
	})

	t.Run("the directory of the -d option", func(t *testing.T) {
		fsys := fsx.MemFS{"sub/f": {Data: []byte("1\n2\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name:    "d",
			Stdin:   strings.NewReader(unifiedPatch("f", "f", "1\n2\n3\n", "1\nx\n3\n")),
			FS:      fsys,
			Args:    []string{"-d", "sub"},
			WantErr: "patching file f\n",
		})
		checkFile(t, fsys, "sub/f", "1\nx\n3\n")
	})

	t.Run("a missing file asked for interactively", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}, "p.patch": {Data: []byte(unifiedPatch("nope", "nope", "1\n2\n3\n", "1\nx\n3\n"))}}
		testutil.Once(t, New, testutil.Case{
			Name:    "prompt for a name",
			Stdin:   strings.NewReader("f\n"),
			FS:      fsys,
			Args:    []string{"-i", "p.patch"},
			WantErr: "File to patch: patching file f\n",
		})
		checkFile(t, fsys, "f", "1\nx\n3\n")
	})
}

func TestRunCreateDelete(t *testing.T) {
	t.Run("a new file from /dev/null", func(t *testing.T) {
		fsys := fsx.MemFS{}
		testutil.Once(t, New, testutil.Case{
			Name: "creation",
			Stdin: strings.NewReader("--- /dev/null\t2026-01-02 03:04:05 +0000\n" +
				"+++ new.txt\t2026-01-02 03:04:05 +0000\n" +
				"@@ -0,0 +1 @@\n+hello\n"),
			FS:      fsys,
			WantErr: "patching file new.txt\n",
		})
		checkFile(t, fsys, "new.txt", "hello\n")
	})

	t.Run("a file taken away to /dev/null", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name: "deletion",
			Stdin: strings.NewReader("--- f\t2026-01-02 03:04:05 +0000\n" +
				"+++ /dev/null\t2026-01-02 03:04:05 +0000\n" +
				"@@ -1,3 +0,0 @@\n-1\n-2\n-3\n"),
			FS:      fsys,
			WantErr: "patching file f\n",
		})
		checkMissing(t, fsys, "f")
	})
}

func TestRunBackupOutput(t *testing.T) {
	patch := unifiedPatch("f", "f", "1\n2\n3\n", "1\nx\n3\n")

	t.Run("backup with -b", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name:    "backup",
			Stdin:   strings.NewReader(patch),
			FS:      fsys,
			Args:    []string{"-b", "f"},
			WantErr: "patching file f\n",
		})
		checkFile(t, fsys, "f.orig", "1\n2\n3\n")
		checkFile(t, fsys, "f", "1\nx\n3\n")
	})

	t.Run("output to another file with -o", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}}
		testutil.Once(t, New, testutil.Case{
			Name:    "output",
			Stdin:   strings.NewReader(patch),
			FS:      fsys,
			Args:    []string{"-o", "out.txt", "f"},
			WantErr: "patching file f\n",
		})
		checkFile(t, fsys, "out.txt", "1\nx\n3\n")
		checkFile(t, fsys, "f", "1\n2\n3\n")
	})

	t.Run("the backup of an output file with -b and -o", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}, "out.txt": {Data: []byte("old\n")}}
		testutil.Once(t, New, testutil.Case{
			Name:    "output backup",
			Stdin:   strings.NewReader(patch),
			FS:      fsys,
			Args:    []string{"-b", "-o", "out.txt", "f"},
			WantErr: "patching file f\n",
		})
		checkFile(t, fsys, "out.txt.orig", "old\n")
		checkFile(t, fsys, "out.txt", "1\nx\n3\n")
	})

	t.Run("versions of one file concatenated with -o", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}}
		patches := unifiedPatch("f", "f", "1\n2\n3\n", "1\nx\n3\n") +
			unifiedPatch("f", "f", "1\nx\n3\n", "1\ny\n3\n")
		testutil.Once(t, New, testutil.Case{
			Name:    "concatenated",
			Stdin:   strings.NewReader(patches),
			FS:      fsys,
			Args:    []string{"-o", "out.txt", "f"},
			WantErr: "patching file f\npatching file f\n",
		})
		checkFile(t, fsys, "out.txt", "1\nx\n3\n1\ny\n3\n")
	})
}

func TestRunErrors(t *testing.T) {
	patch := unifiedPatch("f", "f", "1\n2\n3\n", "1\nx\n3\n")
	tests := []testutil.Case{
		{
			Name:     "garbage input",
			Stdin:    strings.NewReader("just some text\n"),
			FS:       fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			Args:     []string{"f"},
			WantErr:  "patch: only garbage was found in the patch input\n",
			WantExit: 2,
		},
		{
			Name:     "missing file to patch",
			Stdin:    strings.NewReader(patch),
			FS:       fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			Args:     []string{"nope"},
			WantErr:  "patch: can't find file to patch: nope\n",
			WantExit: 2,
		},
		{
			Name:     "extra operand",
			Stdin:    strings.NewReader(patch),
			FS:       fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			Args:     []string{"f", "g"},
			WantErr:  "patch: extra operand 'g'\nTry 'patch --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option",
			Stdin:    strings.NewReader(patch),
			FS:       fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			Args:     []string{"-x", "f"},
			WantErr:  "patch: invalid option -- 'x'\nTry 'patch --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "conflicting formats",
			Stdin:    strings.NewReader(patch),
			FS:       fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			Args:     []string{"-c", "-u", "f"},
			WantErr:  "patch: options -c and -u are incompatible\nTry 'patch --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid -p argument",
			Stdin:    strings.NewReader(patch),
			FS:       fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			Args:     []string{"-p", "x", "f"},
			WantErr:  "patch: invalid -p argument 'x'\nTry 'patch --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "malformed patch",
			Stdin:    strings.NewReader("@@ -x +1 @@\n"),
			FS:       fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			Args:     []string{"f"},
			WantErr:  "patch: malformed patch at line 1: missing line number\n",
			WantExit: 2,
		},
	}
	testutil.Run(t, New, tests)
}

func TestRunContext(t *testing.T) {
	fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}}

	t.Run("cancelled before running", func(t *testing.T) {
		testutil.Cancelled(t, New, strings.NewReader("patch\n"), fsys, "f")
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		testutil.Cancelled(t, New, &testutil.CancelReader{}, fsys, "f")
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "patch", usage)
}
