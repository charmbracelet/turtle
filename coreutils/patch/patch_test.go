package patch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/diff"
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

type testCase struct {
	name     string
	stdin    io.Reader
	fsys     fsx.MemFS
	dir      string
	args     []string
	want     string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) fsx.MemFS {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      test.stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		WorkingDir: test.dir,
	}
	if test.fsys != nil {
		rt.FS = test.fsys
	}
	err := New(rt).Run(t.Context(), test.args...)

	if code := exitCode(t, err); code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.want {
		t.Errorf("Run() stdout = %q, want %q", got, test.want)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
	return test.fsys
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
		name     string
		patch    string
		contents string
		args     []string
		want     string
		wantErr  string
		wantExit int
	}{
		{
			name:     "unified patch through the operand",
			patch:    unifiedPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n"),
			contents: "1\n2\n3\n",
			args:     []string{"f"},
			want:     "1\nx\n3\n",
			wantErr:  "patching file f\n",
		},
		{
			name:     "unified patch through the header names",
			patch:    unifiedPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n"),
			contents: "1\n2\n3\n",
			want:     "1\nx\n3\n",
			wantErr:  "patching file f\n",
		},
		{
			name:     "context patch",
			patch:    contextPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n"),
			contents: "1\n2\n3\n",
			args:     []string{"f"},
			want:     "1\nx\n3\n",
			wantErr:  "patching file f\n",
		},
		{
			name:     "forced unified format",
			patch:    unifiedPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n"),
			contents: "1\n2\n3\n",
			args:     []string{"-u", "f"},
			want:     "1\nx\n3\n",
			wantErr:  "patching file f\n",
		},
		{
			name:     "forced context format",
			patch:    contextPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n"),
			contents: "1\n2\n3\n",
			args:     []string{"-c", "f"},
			want:     "1\nx\n3\n",
			wantErr:  "patching file f\n",
		},
		{
			name:     "insertion and deletion",
			patch:    unifiedPatch("f", "f", "1\n2\n4\n5\n", "1\nx\n4\n"),
			contents: "1\n2\n4\n5\n",
			args:     []string{"f"},
			want:     "1\nx\n4\n",
			wantErr:  "patching file f\n",
		},
		{
			name:     "missing newline at end of file",
			patch:    unifiedPatch("f", "f", "a\nb", "a\nc"),
			contents: "a\nb",
			args:     []string{"f"},
			want:     "a\nc",
			wantErr:  "patching file f\n",
		},
		{
			name:     "newline added at end of file",
			patch:    unifiedPatch("f", "f", "a\nb", "a\nb\n"),
			contents: "a\nb",
			args:     []string{"f"},
			want:     "a\nb\n",
			wantErr:  "patching file f\n",
		},
		{
			name:     "patch from a file",
			patch:    unifiedPatch("a/f", "b/f", "1\n2\n3\n", "1\nx\n3\n"),
			contents: "1\n2\n3\n",
			args:     []string{"-i", "p.patch"},
			want:     "1\nx\n3\n",
			wantErr:  "patching file f\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fsys := fsx.MemFS{"f": {Data: []byte(test.contents)}}
			if strings.Contains(test.name, "from a file") {
				fsys["p.patch"] = &fsx.MemFile{Data: []byte(test.patch)}
			}
			runCase(t, testCase{
				name:     test.name,
				stdin:    strings.NewReader(test.patch),
				fsys:     fsys,
				args:     test.args,
				wantErr:  test.wantErr,
				wantExit: test.wantExit,
			})
			checkFile(t, fsys, "f", test.want)
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
	runCase(t, testCase{
		name:    "two files",
		stdin:   strings.NewReader(patch),
		fsys:    fsys,
		wantErr: "patching file f\npatching file g\n",
	})
	checkFile(t, fsys, "f", "1\nx\n3\n")
	checkFile(t, fsys, "g", "q\nz\n")
}

func TestRunSearch(t *testing.T) {
	t.Run("hunk moved by added lines", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("z\ny\n1\n2\n3\n4\n5\n")}}
		runCase(t, testCase{
			name: "offset",
			stdin: strings.NewReader(unifiedPatch("f", "f",
				"1\n2\n3\n4\n5\n", "1\nX\n3\n4\n5\n")),
			fsys:    fsys,
			args:    []string{"f"},
			wantErr: "patching file f\nHunk #1 succeeded at 3 (offset 2 lines).\n",
		})
		checkFile(t, fsys, "f", "z\ny\n1\nX\n3\n4\n5\n")
	})

	t.Run("hunk matching with fuzz", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n4\n9\n")}}
		runCase(t, testCase{
			name: "fuzz",
			stdin: strings.NewReader(unifiedPatch("f", "f",
				"1\n2\n3\n4\n5\n", "1\nX\n3\n4\n5\n")),
			fsys:    fsys,
			args:    []string{"f"},
			wantErr: "patching file f\nHunk #1 succeeded at 2 with fuzz 1 (offset 1 lines).\n",
		})
		checkFile(t, fsys, "f", "1\nX\n3\n4\n9\n")
	})

	t.Run("blank amounts ignored with -l", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2 \n3\n")}}
		runCase(t, testCase{
			name: "blank",
			stdin: strings.NewReader(unifiedPatch("f", "f",
				"1\n2\n3\n", "1\n2\n4\n")),
			fsys:    fsys,
			args:    []string{"-l", "f"},
			wantErr: "patching file f\n",
		})
		checkFile(t, fsys, "f", "1\n2\n4\n")
	})
}

func TestRunRejects(t *testing.T) {
	t.Run("failed hunk saved to the reject file", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n9\n3\n")}}
		runCase(t, testCase{
			name: "reject",
			stdin: strings.NewReader(unifiedPatch("f", "f",
				"1\n2\n3\n", "1\nX\n3\n")),
			fsys:     fsys,
			args:     []string{"f"},
			wantErr:  "patching file f\nHunk #1 FAILED at 1.\n1 out of 1 hunks FAILED -- saving rejects to file f.rej\n",
			wantExit: 1,
		})
		checkFile(t, fsys, "f", "1\n9\n3\n")
		checkFile(t, fsys, "f.rej", "@@ -1,3 +1,3 @@\n 1\n-2\n+X\n 3\n")
	})

	t.Run("context rejects keep the context format", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n9\n3\n")}}
		runCase(t, testCase{
			name: "context reject",
			stdin: strings.NewReader(contextPatch("f", "f",
				"1\n2\n3\n", "1\nX\n3\n")),
			fsys:     fsys,
			args:     []string{"-c", "f"},
			wantErr:  "patching file f\nHunk #1 FAILED at 1.\n1 out of 1 hunks FAILED -- saving rejects to file f.rej\n",
			wantExit: 1,
		})
		checkFile(t, fsys, "f.rej", "***************\n*** 1,3 ****\n  1\n- 2\n  3\n--- 1,3 ----\n  1\n+ X\n  3\n")
	})

	t.Run("reject file of the -r option", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n9\n3\n")}}
		runCase(t, testCase{
			name: "custom reject file",
			stdin: strings.NewReader(unifiedPatch("f", "f",
				"1\n2\n3\n", "1\nX\n3\n")),
			fsys:     fsys,
			args:     []string{"-r", "junk.rej", "f"},
			wantErr:  "patching file f\nHunk #1 FAILED at 1.\n1 out of 1 hunks FAILED -- saving rejects to file junk.rej\n",
			wantExit: 1,
		})
		checkFile(t, fsys, "junk.rej", "@@ -1,3 +1,3 @@\n 1\n-2\n+X\n 3\n")
	})
}

func TestRunReversed(t *testing.T) {
	patch := unifiedPatch("f", "f", "1\n2\n3\n", "1\nx\n3\n")

	t.Run("the -R option applies backwards", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\nx\n3\n")}}
		runCase(t, testCase{
			name:     "reverse",
			stdin:    strings.NewReader(patch),
			fsys:     fsys,
			args:     []string{"-R", "f"},
			wantErr:  "patching file f\n",
			wantExit: 0,
		})
		checkFile(t, fsys, "f", "1\n2\n3\n")
	})

	t.Run("an applied patch skipped without -N", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\nx\n3\n")}}
		runCase(t, testCase{
			name:     "skipped",
			stdin:    strings.NewReader(patch),
			fsys:     fsys,
			args:     []string{"f"},
			wantErr:  "Reversed (or previously applied) patch detected!  Assume -R? Skipping patch.\n",
			wantExit: 1,
		})
		checkFile(t, fsys, "f", "1\nx\n3\n")
	})

	t.Run("an applied patch skipped with -N", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\nx\n3\n")}}
		runCase(t, testCase{
			name:     "-N",
			stdin:    strings.NewReader(patch),
			fsys:     fsys,
			args:     []string{"-N", "f"},
			wantErr:  "Reversed (or previously applied) patch detected!  Skipping patch.\n",
			wantExit: 1,
		})
		checkFile(t, fsys, "f", "1\nx\n3\n")
	})

	t.Run("the prompt applying backwards", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\nx\n3\n")}, "p.patch": {Data: []byte(patch)}}
		runCase(t, testCase{
			name:     "prompt yes",
			stdin:    strings.NewReader("y\n"),
			fsys:     fsys,
			args:     []string{"-i", "p.patch", "f"},
			wantErr:  "Reversed (or previously applied) patch detected!  Assume -R? patching file f\n",
			wantExit: 0,
		})
		checkFile(t, fsys, "f", "1\n2\n3\n")
	})

	t.Run("the prompt refusing to apply backwards", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\nx\n3\n")}, "p.patch": {Data: []byte(patch)}}
		runCase(t, testCase{
			name:     "prompt no",
			stdin:    strings.NewReader("n\n"),
			fsys:     fsys,
			args:     []string{"-i", "p.patch", "f"},
			wantErr:  "Reversed (or previously applied) patch detected!  Assume -R? Skipping patch.\n",
			wantExit: 1,
		})
		checkFile(t, fsys, "f", "1\nx\n3\n")
	})
}

func TestRunNames(t *testing.T) {
	patch := unifiedPatch("a/b/f", "c/b/f", "1\n2\n3\n", "1\nx\n3\n")

	t.Run("the basename by default", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}}
		runCase(t, testCase{
			name:    "basename",
			stdin:   strings.NewReader(patch),
			fsys:    fsys,
			wantErr: "patching file f\n",
		})
		checkFile(t, fsys, "f", "1\nx\n3\n")
	})

	t.Run("components stripped with -p", func(t *testing.T) {
		fsys := fsx.MemFS{"b/f": {Data: []byte("1\n2\n3\n")}}
		runCase(t, testCase{
			name:    "p1",
			stdin:   strings.NewReader(patch),
			fsys:    fsys,
			args:    []string{"-p1"},
			wantErr: "patching file b/f\n",
		})
		checkFile(t, fsys, "b/f", "1\nx\n3\n")
	})

	t.Run("the whole path with -p0", func(t *testing.T) {
		fsys := fsx.MemFS{"a/b/f": {Data: []byte("1\n2\n3\n")}}
		runCase(t, testCase{
			name:    "p0",
			stdin:   strings.NewReader(patch),
			fsys:    fsys,
			args:    []string{"-p0"},
			wantErr: "patching file a/b/f\n",
		})
		checkFile(t, fsys, "a/b/f", "1\nx\n3\n")
	})

	t.Run("the directory of the -d option", func(t *testing.T) {
		fsys := fsx.MemFS{"sub/f": {Data: []byte("1\n2\n3\n")}}
		runCase(t, testCase{
			name:    "d",
			stdin:   strings.NewReader(unifiedPatch("f", "f", "1\n2\n3\n", "1\nx\n3\n")),
			fsys:    fsys,
			args:    []string{"-d", "sub"},
			wantErr: "patching file f\n",
		})
		checkFile(t, fsys, "sub/f", "1\nx\n3\n")
	})

	t.Run("a missing file asked for interactively", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}, "p.patch": {Data: []byte(unifiedPatch("nope", "nope", "1\n2\n3\n", "1\nx\n3\n"))}}
		runCase(t, testCase{
			name:    "prompt for a name",
			stdin:   strings.NewReader("f\n"),
			fsys:    fsys,
			args:    []string{"-i", "p.patch"},
			wantErr: "File to patch: patching file f\n",
		})
		checkFile(t, fsys, "f", "1\nx\n3\n")
	})
}

func TestRunCreateDelete(t *testing.T) {
	t.Run("a new file from /dev/null", func(t *testing.T) {
		fsys := fsx.MemFS{}
		runCase(t, testCase{
			name: "creation",
			stdin: strings.NewReader("--- /dev/null\t2026-01-02 03:04:05 +0000\n" +
				"+++ new.txt\t2026-01-02 03:04:05 +0000\n" +
				"@@ -0,0 +1 @@\n+hello\n"),
			fsys:    fsys,
			wantErr: "patching file new.txt\n",
		})
		checkFile(t, fsys, "new.txt", "hello\n")
	})

	t.Run("a file taken away to /dev/null", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}}
		runCase(t, testCase{
			name: "deletion",
			stdin: strings.NewReader("--- f\t2026-01-02 03:04:05 +0000\n" +
				"+++ /dev/null\t2026-01-02 03:04:05 +0000\n" +
				"@@ -1,3 +0,0 @@\n-1\n-2\n-3\n"),
			fsys:    fsys,
			wantErr: "patching file f\n",
		})
		checkMissing(t, fsys, "f")
	})
}

func TestRunBackupOutput(t *testing.T) {
	patch := unifiedPatch("f", "f", "1\n2\n3\n", "1\nx\n3\n")

	t.Run("backup with -b", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}}
		runCase(t, testCase{
			name:    "backup",
			stdin:   strings.NewReader(patch),
			fsys:    fsys,
			args:    []string{"-b", "f"},
			wantErr: "patching file f\n",
		})
		checkFile(t, fsys, "f.orig", "1\n2\n3\n")
		checkFile(t, fsys, "f", "1\nx\n3\n")
	})

	t.Run("output to another file with -o", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}}
		runCase(t, testCase{
			name:    "output",
			stdin:   strings.NewReader(patch),
			fsys:    fsys,
			args:    []string{"-o", "out.txt", "f"},
			wantErr: "patching file f\n",
		})
		checkFile(t, fsys, "out.txt", "1\nx\n3\n")
		checkFile(t, fsys, "f", "1\n2\n3\n")
	})

	t.Run("the backup of an output file with -b and -o", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}, "out.txt": {Data: []byte("old\n")}}
		runCase(t, testCase{
			name:    "output backup",
			stdin:   strings.NewReader(patch),
			fsys:    fsys,
			args:    []string{"-b", "-o", "out.txt", "f"},
			wantErr: "patching file f\n",
		})
		checkFile(t, fsys, "out.txt.orig", "old\n")
		checkFile(t, fsys, "out.txt", "1\nx\n3\n")
	})

	t.Run("versions of one file concatenated with -o", func(t *testing.T) {
		fsys := fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}}
		patches := unifiedPatch("f", "f", "1\n2\n3\n", "1\nx\n3\n") +
			unifiedPatch("f", "f", "1\nx\n3\n", "1\ny\n3\n")
		runCase(t, testCase{
			name:    "concatenated",
			stdin:   strings.NewReader(patches),
			fsys:    fsys,
			args:    []string{"-o", "out.txt", "f"},
			wantErr: "patching file f\npatching file f\n",
		})
		checkFile(t, fsys, "out.txt", "1\nx\n3\n1\ny\n3\n")
	})
}

func TestRunErrors(t *testing.T) {
	patch := unifiedPatch("f", "f", "1\n2\n3\n", "1\nx\n3\n")
	tests := []testCase{
		{
			name:     "garbage input",
			stdin:    strings.NewReader("just some text\n"),
			fsys:     fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			args:     []string{"f"},
			wantErr:  "patch: only garbage was found in the patch input\n",
			wantExit: 2,
		},
		{
			name:     "missing file to patch",
			stdin:    strings.NewReader(patch),
			fsys:     fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			args:     []string{"nope"},
			wantErr:  "patch: can't find file to patch: nope\n",
			wantExit: 2,
		},
		{
			name:     "extra operand",
			stdin:    strings.NewReader(patch),
			fsys:     fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			args:     []string{"f", "g"},
			wantErr:  "patch: extra operand 'g'\n",
			wantExit: 2,
		},
		{
			name:     "invalid option",
			stdin:    strings.NewReader(patch),
			fsys:     fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			args:     []string{"-x", "f"},
			wantErr:  "patch: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "conflicting formats",
			stdin:    strings.NewReader(patch),
			fsys:     fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			args:     []string{"-c", "-u", "f"},
			wantErr:  "patch: options -c and -u are incompatible\n",
			wantExit: 2,
		},
		{
			name:     "invalid -p argument",
			stdin:    strings.NewReader(patch),
			fsys:     fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			args:     []string{"-p", "x", "f"},
			wantErr:  "patch: invalid -p argument 'x'\n",
			wantExit: 2,
		},
		{
			name:     "malformed patch",
			stdin:    strings.NewReader("@@ -x +1 @@\n"),
			fsys:     fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
			args:     []string{"f"},
			wantErr:  "patch: malformed patch at line 1: missing line number\n",
			wantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		runCancelled(t, ctx, strings.NewReader("patch\n"))
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runCancelled(t, ctx, &cancelReader{cancel: cancel})
	})
}

// cancelReader cancels its context on the first read, simulating a context
// that is done while input is being read.
type cancelReader struct {
	cancel context.CancelFunc
}

func (r *cancelReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
}

// runCancelled runs the utility with a context that is either already done
// or becomes done while reading, and checks that the context error is
// returned without any output.
func runCancelled(t *testing.T, ctx context.Context, stdin io.Reader) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsx.MemFS{"f": {Data: []byte("1\n2\n3\n")}},
	}
	err := New(rt).Run(ctx, "f")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
	}
}

func TestHelpVersion(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"help", []string{"--help"}, usage},
		{
			"version",
			[]string{"--version"},
			"patch (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, testCase{name: test.name, args: test.args, want: test.want})
		})
	}
}
