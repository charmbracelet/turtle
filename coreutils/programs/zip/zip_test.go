package zip

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/zipcore"
	"charm.land/turtle/coreutils/programs/unzip"
	"charm.land/turtle/fsx"
)

// mod is the modification time of the fixtures which store one, on an even
// second so the 2-second granularity of the format keeps it exactly.
var mod = time.Date(2024, 3, 2, 1, 2, 4, 0, time.UTC)

// older and newer are the modification times around mod.
var (
	older = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
)

// testFS holds the fixtures used by the tests:
//
//	a.txt:     "alpha\n"
//	b.txt:     "beta\n", big enough to deflate
//	sub/c.txt: "gamma\n"
//	link:      a symbolic link to a.txt
func testFS() fsx.MemoryFS {
	return fsx.MemoryFS{
		"a.txt":     {Data: []byte("alpha\n"), Mode: 0o644, ModTime: mod},
		"b.txt":     {Data: []byte(strings.Repeat("bravo charlie delta\n", 30)), Mode: 0o644, ModTime: mod},
		"sub/c.txt": {Data: []byte("gamma\n"), Mode: 0o644, ModTime: mod},
		"link":      {Data: []byte("a.txt"), Mode: fs.ModeSymlink | 0o777, ModTime: mod},
	}
}

// result holds what a run of the program produced.
type result struct {
	stdout string
	stderr string
	exit   int
}

// run executes the program against the given arguments, returning what it
// wrote along with the file system it left behind.
func run(t *testing.T, fsys fsx.MemoryFS, stdin io.Reader, args ...string) (result, fsx.MemoryFS) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: &stdout,
		Stderr: &stderr,
		FS:     fsys,
	}
	err := New(rt).Run(t.Context(), args...)
	code, exit := 0, &coreutils.ExitError{}
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
	}
	if err != nil {
		code = exit.Code
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), exit: code}, fsys
}

// readArchive parses the archive a run left behind.
func readArchive(t *testing.T, fsys fsx.MemoryFS, name string) *zip.Reader {
	t.Helper()

	file, ok := fsys[name]
	if !ok {
		t.Fatalf("%s: missing archive", name)
	}
	zr, err := zip.NewReader(bytes.NewReader(file.Data), int64(len(file.Data)))
	if err != nil {
		t.Fatalf("NewReader(%s): %v", name, err)
	}
	return zr
}

// memberNames lists the names of the members of an archive.
func memberNames(zr *zip.Reader) []string {
	names := make([]string, 0, len(zr.File))
	for _, file := range zr.File {
		names = append(names, file.Name)
	}
	return names
}

// memberContent reads one member of an archive.
func memberContent(t *testing.T, zr *zip.Reader, name string) string {
	t.Helper()

	for _, file := range zr.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("Open(%s): %v", name, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read(%s): %v", name, err)
		}
		return string(data)
	}
	t.Fatalf("%s: missing member", name)
	return ""
}

func TestZipAdd(t *testing.T) {
	t.Run("adds a file", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "a.zip", "a.txt")

		if res.stdout != "  adding: a.txt (stored 0%)\n" {
			t.Errorf("stdout = %q, want the adding line", res.stdout)
		}
		if res.stderr != "" || res.exit != 0 {
			t.Errorf("stderr = %q, exit = %d, want empty and 0", res.stderr, res.exit)
		}
		zr := readArchive(t, fsys, "a.zip")
		if got := memberNames(zr); len(got) != 1 || got[0] != "a.txt" {
			t.Errorf("members = %v, want a.txt", got)
		}
		if got := memberContent(t, zr, "a.txt"); got != "alpha\n" {
			t.Errorf("a.txt = %q, want the file contents", got)
		}
		if got := zr.File[0].Mode().Perm(); got != 0o644 {
			t.Errorf("a.txt mode = %o, want 644", got)
		}
		if got := zr.File[0].Modified; !got.Equal(mod) {
			t.Errorf("a.txt mtime = %v, want %v", got, mod)
		}
	})

	t.Run("deflates the compressible files", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "a.zip", "b.txt")

		zr := readArchive(t, fsys, "a.zip")
		if zr.File[0].Method != zip.Deflate {
			t.Errorf("method = %d, want Deflate", zr.File[0].Method)
		}
		ratio := 100 - 100*int(zr.File[0].CompressedSize64)/len(strings.Repeat("bravo charlie delta\n", 30))
		want := fmt.Sprintf("  adding: b.txt (deflated %d%%)\n", ratio)
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
	})

	t.Run("-0 stores without compressing", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "-0", "a.zip", "b.txt")

		if res.stdout != "  adding: b.txt (stored 0%)\n" {
			t.Errorf("stdout = %q, want the stored line", res.stdout)
		}
		if got := readArchive(t, fsys, "a.zip").File[0].Method; got != zip.Store {
			t.Errorf("method = %d, want Store", got)
		}
	})

	t.Run("a directory without -r stores its entry alone", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "a.zip", "sub")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		if got := memberNames(readArchive(t, fsys, "a.zip")); len(got) != 1 || got[0] != "sub/" {
			t.Errorf("members = %v, want sub/", got)
		}
	})

	t.Run("-r walks the hierarchy", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "-r", "a.zip", "sub")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		got := memberNames(readArchive(t, fsys, "a.zip"))
		want := []string{"sub/", "sub/c.txt"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("members = %v, want %v", got, want)
		}
	})

	t.Run("-r from the working directory", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "-r", "a.zip", ".")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		got := memberNames(readArchive(t, fsys, "a.zip"))
		want := []string{"a.txt", "b.txt", "link", "sub/", "sub/c.txt"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("members = %v, want %v", got, want)
		}
	})

	t.Run("the archive is never packed into itself", func(t *testing.T) {
		res, _ := run(t, testFS(), nil, "-r", "a.zip", ".")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		res, fsys := run(t, testFS(), nil, "-r", "a.zip", ".")
		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		for _, name := range memberNames(readArchive(t, fsys, "a.zip")) {
			if name == "a.zip" {
				t.Error("a.zip: archive packed into itself")
			}
		}
	})

	t.Run("-D skips the directory entries", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "-rD", "a.zip", "sub")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		if got := memberNames(readArchive(t, fsys, "a.zip")); len(got) != 1 || got[0] != "sub/c.txt" {
			t.Errorf("members = %v, want sub/c.txt", got)
		}
	})

	t.Run("-j junks the paths", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "-rj", "a.zip", "sub")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		if got := memberNames(readArchive(t, fsys, "a.zip")); len(got) != 1 || got[0] != "c.txt" {
			t.Errorf("members = %v, want c.txt", got)
		}
	})

	t.Run("-q stays quiet", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "-q", "a.zip", "a.txt")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		if res.stdout != "" || res.stderr != "" {
			t.Errorf("stdout = %q, stderr = %q, want empty", res.stdout, res.stderr)
		}
		if got := memberNames(readArchive(t, fsys, "a.zip")); len(got) != 1 {
			t.Errorf("members = %v, want one", got)
		}
	})

	t.Run("the .zip suffix is appended to a bare name", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "noext", "a.txt")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		readArchive(t, fsys, "noext.zip")
	})

	t.Run("a name with an extension is kept as given", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "oth.ext", "a.txt")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		readArchive(t, fsys, "oth.ext")
	})

	t.Run("a symbolic link points at its target by default", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "a.zip", "link")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		zr := readArchive(t, fsys, "a.zip")
		if got := memberContent(t, zr, "link"); got != "alpha\n" {
			t.Errorf("link = %q, want the target contents", got)
		}
		if zr.File[0].Mode()&fs.ModeSymlink != 0 {
			t.Error("link mode = a symbolic link, want the target's")
		}
	})

	t.Run("-y stores the symbolic link", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "-y", "a.zip", "link")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		zr := readArchive(t, fsys, "a.zip")
		if got := memberContent(t, zr, "link"); got != "a.txt" {
			t.Errorf("link = %q, want the target name", got)
		}
		if zr.File[0].Mode()&fs.ModeSymlink == 0 {
			t.Error("link mode = a regular file, want a symbolic link")
		}
	})

	t.Run("-x excludes the matching files", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "-r", "a.zip", ".", "-x", "*.txt")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		got := memberNames(readArchive(t, fsys, "a.zip"))
		if strings.Join(got, ",") != "link,sub/" {
			t.Errorf("members = %v, want the unexcluded ones", got)
		}
	})

	t.Run("-i includes only the matching files", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "-r", "a.zip", ".", "-i", "*.txt")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		got := memberNames(readArchive(t, fsys, "a.zip"))
		if strings.Join(got, ",") != "a.txt,b.txt,sub/c.txt" {
			t.Errorf("members = %v, want the included ones", got)
		}
	})

	t.Run("-@ keeps the whitespace of the names", func(t *testing.T) {
		fsys := fsx.MemoryFS{"  spaced .txt": {Data: []byte("spaced\n"), Mode: 0o644, ModTime: mod}}
		res, fsys := run(t, fsys, strings.NewReader("  spaced .txt\r\n"), "-@", "a.zip")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		got := memberNames(readArchive(t, fsys, "a.zip"))
		if len(got) != 1 || got[0] != "  spaced .txt" {
			t.Errorf("members = %v, want the name as typed", got)
		}
	})

	t.Run("-v reports the sizes and the savings", func(t *testing.T) {
		res, _ := run(t, testFS(), nil, "-v", "a.zip", "a.txt", "b.txt")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		data := strings.Repeat("bravo charlie delta\n", 30)
		comp := deflateSize(t, data)
		ratio := deflateRatio(t, data)
		savings := 100 - 100*(6+comp)/(6+len(data))
		want := fmt.Sprintf("  adding: a.txt\t(in=6) (out=6) (stored 0%%)\n"+
			"  adding: b.txt\t(in=%d) (out=%d) (deflated %d%%)\n"+
			"total bytes=%d, compressed=%d -> %d%% savings\n",
			len(data), comp, ratio,
			6+len(data), 6+comp, savings)
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
	})

	t.Run("every level is accepted", func(t *testing.T) {
		for level := 0; level <= 9; level++ {
			t.Run(fmt.Sprintf("-%d", level), func(t *testing.T) {
				res, fsys := run(t, testFS(), nil, fmt.Sprintf("-%d", level), "a.zip", "b.txt")

				if res.exit != 0 {
					t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
				}
				zr := readArchive(t, fsys, "a.zip")
				want := uint16(zip.Deflate)
				if level == 0 {
					want = zip.Store
				}
				if got := zr.File[0].Method; got != want {
					t.Errorf("method = %d, want %d", got, want)
				}
				if got := memberContent(t, zr, "b.txt"); got != strings.Repeat("bravo charlie delta\n", 30) {
					t.Errorf("b.txt = %q, want the file contents", got)
				}
			})
		}
	})
}

// deflateSize returns the size data takes once deflated at the default
// level.
func deflateSize(t *testing.T, data string) int {
	t.Helper()

	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, zipcore.DefaultLevel)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(data))
	w.Close()
	return buf.Len()
}

// deflateRatio returns the compression percentage of data, as the report
// lines show it.
func deflateRatio(t *testing.T, data string) int {
	t.Helper()

	return 100 - 100*deflateSize(t, data)/len(data)
}

func TestZipUpdate(t *testing.T) {
	t.Run("a second add replaces the entry", func(t *testing.T) {
		fsys := testFS()
		res, fsys := run(t, fsys, nil, "a.zip", "a.txt")
		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		fsys["a.txt"].Data = []byte("changed\n")
		fsys["a.txt"].ModTime = newer
		res, fsys = run(t, fsys, nil, "a.zip", "a.txt")

		if res.stdout != "  updating: a.txt (stored 0%)\n" {
			t.Errorf("stdout = %q, want the updating line", res.stdout)
		}
		if got := memberContent(t, readArchive(t, fsys, "a.zip"), "a.txt"); got != "changed\n" {
			t.Errorf("a.txt = %q, want the new contents", got)
		}
	})

	t.Run("-f freshens the changed entries", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, nil, "a.zip", "a.txt")
		fsys["a.txt"].Data = []byte("changed\n")
		fsys["a.txt"].ModTime = newer
		res, fsys := run(t, fsys, nil, "-f", "a.zip", "a.txt")

		if res.stdout != "  freshening: a.txt (stored 0%)\n" {
			t.Errorf("stdout = %q, want the freshening line", res.stdout)
		}
		if got := memberContent(t, readArchive(t, fsys, "a.zip"), "a.txt"); got != "changed\n" {
			t.Errorf("a.txt = %q, want the new contents", got)
		}
	})

	t.Run("-f leaves the unchanged entries silently", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, nil, "a.zip", "a.txt")
		res, fsys := run(t, fsys, nil, "-f", "a.zip", "a.txt")

		if res.stdout != "" || res.stderr != "" {
			t.Errorf("stdout = %q, stderr = %q, want empty", res.stdout, res.stderr)
		}
		if res.exit != 12 {
			t.Errorf("exit = %d, want 12", res.exit)
		}
	})

	t.Run("-f doesn't add the missing entries", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, nil, "a.zip", "a.txt")
		res, fsys := run(t, fsys, nil, "-f", "a.zip", "b.txt")

		if res.exit != 12 {
			t.Errorf("exit = %d, want 12", res.exit)
		}
		got := memberNames(readArchive(t, fsys, "a.zip"))
		if strings.Join(got, ",") != "a.txt" {
			t.Errorf("members = %v, want a.txt alone", got)
		}
	})

	t.Run("-f on a missing archive warns", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "-f", "a.zip", "a.txt")

		if res.stderr != "\tzip warning: a.zip not found or empty\n" {
			t.Errorf("stderr = %q, want the warning", res.stderr)
		}
		if res.exit != 12 {
			t.Errorf("exit = %d, want 12", res.exit)
		}
		if _, ok := fsys["a.zip"]; ok {
			t.Error("a.zip: unexpected archive")
		}
	})

	t.Run("-u adds the missing files and updates the changed ones", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, nil, "a.zip", "a.txt")
		fsys["a.txt"].Data = []byte("changed\n")
		fsys["a.txt"].ModTime = newer
		res, fsys := run(t, fsys, nil, "-u", "a.zip", "a.txt", "b.txt")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		want := "  updating: a.txt (stored 0%)\n" +
			fmt.Sprintf("  adding: b.txt (deflated %d%%)\n", deflateRatio(t, string(fsys["b.txt"].Data)))
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
		zr := readArchive(t, fsys, "a.zip")
		if got := memberContent(t, zr, "a.txt"); got != "changed\n" {
			t.Errorf("a.txt = %q, want the new contents", got)
		}
		memberContent(t, zr, "b.txt")
	})

	t.Run("-u without inputs takes the entries of the archive", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, nil, "-r", "a.zip", ".")
		fsys["a.txt"].Data = []byte("changed\n")
		fsys["a.txt"].ModTime = newer
		res, fsys := run(t, fsys, nil, "-u", "a.zip")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		want := "  updating: a.txt (stored 0%)\n  updating: link (stored 0%)\n"
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
	})

	t.Run("an unchanged archive stays as it was", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, nil, "a.zip", "a.txt")
		before := string(fsys["a.zip"].Data)
		_, fsys = run(t, fsys, nil, "-f", "a.zip", "a.txt")
		after := string(fsys["a.zip"].Data)

		if before != after {
			t.Error("the archive was rewritten without a change")
		}
	})
}

func TestZipDelete(t *testing.T) {
	t.Run("deletes the matching entries", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, nil, "a.zip", "a.txt", "b.txt")
		res, fsys := run(t, fsys, nil, "-d", "a.zip", "b.txt")

		if res.stdout != "deleting: b.txt\n" {
			t.Errorf("stdout = %q, want the deleting line", res.stdout)
		}
		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
		got := memberNames(readArchive(t, fsys, "a.zip"))
		if strings.Join(got, ",") != "a.txt" {
			t.Errorf("members = %v, want a.txt alone", got)
		}
	})

	t.Run("the patterns match", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, nil, "-r", "a.zip", "sub")
		res, fsys := run(t, fsys, nil, "-d", "a.zip", "*.txt")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		got := memberNames(readArchive(t, fsys, "a.zip"))
		if strings.Join(got, ",") != "sub/" {
			t.Errorf("members = %v, want sub/ alone", got)
		}
	})

	t.Run("nothing matched is nothing to do", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, nil, "a.zip", "a.txt")
		before := string(fsys["a.zip"].Data)
		res, fsys := run(t, fsys, nil, "-d", "a.zip", "nope")

		if res.exit != 12 {
			t.Errorf("exit = %d, want 12", res.exit)
		}
		if want := "\nzip error: Nothing to do! (a.zip)\n"; res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		if after := string(fsys["a.zip"].Data); before != after {
			t.Error("the archive was rewritten without a change")
		}
	})

	t.Run("deleting the last entry warns about the empty archive", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, nil, "a.zip", "a.txt")
		res, fsys := run(t, fsys, nil, "-d", "a.zip", "a.txt")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
		if want := "\tzip warning: zip file empty\n"; res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		if got := memberNames(readArchive(t, fsys, "a.zip")); len(got) != 0 {
			t.Errorf("members = %v, want none", got)
		}
	})
}

func TestZipMove(t *testing.T) {
	t.Run("moves the files into the archive", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "-m", "a.zip", "a.txt")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		if _, ok := fsys["a.txt"]; ok {
			t.Error("a.txt: still on the file system")
		}
		if got := memberContent(t, readArchive(t, fsys, "a.zip"), "a.txt"); got != "alpha\n" {
			t.Errorf("a.txt = %q, want the file contents", got)
		}
	})

	t.Run("moves a hierarchy, removing the emptied directories", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "-rm", "a.zip", "sub")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		if _, ok := fsys["sub/c.txt"]; ok {
			t.Error("sub/c.txt: still on the file system")
		}
		if _, ok := fsys["sub"]; ok {
			t.Error("sub: still on the file system")
		}
	})
}

func TestZipComment(t *testing.T) {
	t.Run("asks for the comment", func(t *testing.T) {
		res, fsys := run(t, testFS(), strings.NewReader("hello world\nsecond line\n.\n"), "-z", "a.zip", "a.txt")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		want := "  adding: a.txt (stored 0%)\n" +
			"enter new zip file comment (end with .):\n"
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
		if got := readArchive(t, fsys, "a.zip").Comment; got != "hello world\nsecond line" {
			t.Errorf("comment = %q, want the typed one", got)
		}
	})

	t.Run("shows the current comment first", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, strings.NewReader("one\n.\n"), "-z", "a.zip", "a.txt")
		res, fsys := run(t, fsys, strings.NewReader("two\n.\n"), "-z", "a.zip")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		want := "current zip file comment is:\none\n" +
			"enter new zip file comment (end with .):\n"
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
		if got := readArchive(t, fsys, "a.zip").Comment; got != "two" {
			t.Errorf("comment = %q, want two", got)
		}
		if got := memberContent(t, readArchive(t, fsys, "a.zip"), "a.txt"); got != "alpha\n" {
			t.Errorf("a.txt = %q, want the kept member", got)
		}
	})
}

func TestZipNothingToDo(t *testing.T) {
	t.Run("a missing input warns and does nothing", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "a.zip", "missing.txt")

		want := "\tzip warning: name not matched: missing.txt\n" +
			"\nzip error: Nothing to do! (a.zip)\n"
		if res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		if res.exit != 12 {
			t.Errorf("exit = %d, want 12", res.exit)
		}
		if _, ok := fsys["a.zip"]; ok {
			t.Error("a.zip: unexpected archive")
		}
	})

	t.Run("a warning with other inputs keeps the success", func(t *testing.T) {
		res, fsys := run(t, testFS(), nil, "a.zip", "a.txt", "missing.txt")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
		if want := "\tzip warning: name not matched: missing.txt\n"; res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		if got := memberNames(readArchive(t, fsys, "a.zip")); len(got) != 1 || got[0] != "a.txt" {
			t.Errorf("members = %v, want a.txt", got)
		}
	})

	t.Run("no inputs with an existing archive", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, nil, "a.zip", "a.txt")
		res, fsys := run(t, fsys, nil, "a.zip")

		if res.exit != 12 {
			t.Errorf("exit = %d, want 12", res.exit)
		}
		if want := "\nzip error: Nothing to do! (a.zip)\n"; res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
	})
}

// failOpen returns a file system whose Open refuses name, so an unreadable
// input is simulated.
type failOpen struct {
	fsx.FS
	name string
}

// Open implements fs.FS, refusing the named file.
func (f failOpen) Open(name string) (fs.File, error) {
	if f.name == name {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return f.FS.Open(name)
}

func TestZipReadFailures(t *testing.T) {
	t.Run("a failed update keeps the old member", func(t *testing.T) {
		fsys := testFS()
		_, fsys = run(t, fsys, nil, "a.zip", "a.txt")
		fsys["a.txt"].Data = []byte("changed\n")
		fsys["a.txt"].ModTime = newer

		var stdout, stderr bytes.Buffer
		rt := &coreutils.Runtime{
			Stdout: &stdout,
			Stderr: &stderr,
			FS:     failOpen{FS: fsys, name: "a.txt"},
		}
		err := New(rt).Run(t.Context(), "a.zip", "a.txt")
		code := 0
		if exit := (&coreutils.ExitError{}); errors.As(err, &exit) {
			code = exit.Code
		} else if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if code != 18 {
			t.Errorf("exit = %d, want 18", code)
		}
		if got := stderr.String(); got != "\tzip warning: could not open for reading: a.txt\n" {
			t.Errorf("stderr = %q, want the read failure", got)
		}
		if got := memberContent(t, readArchive(t, fsys, "a.zip"), "a.txt"); got != "alpha\n" {
			t.Errorf("a.txt = %q, want the old contents kept", got)
		}
	})

	t.Run("only an unreadable input exits 18", func(t *testing.T) {
		fsys := testFS()
		var stdout, stderr bytes.Buffer
		rt := &coreutils.Runtime{
			Stdout: &stdout,
			Stderr: &stderr,
			FS:     failOpen{FS: fsys, name: "a.txt"},
		}
		err := New(rt).Run(t.Context(), "a.zip", "a.txt")
		code := 0
		if exit := (&coreutils.ExitError{}); errors.As(err, &exit) {
			code = exit.Code
		} else if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if code != 18 {
			t.Errorf("exit = %d, want 18", code)
		}
		if _, ok := fsys["a.zip"]; ok {
			t.Error("a.zip: unexpected archive")
		}
	})
}

func TestZipVerbose(t *testing.T) {
	t.Run("the summary of an empty file", func(t *testing.T) {
		fsys := fsx.MemoryFS{"empty.txt": {Data: nil, Mode: 0o644, ModTime: mod}}
		res, _ := run(t, fsys, nil, "-v", "a.zip", "empty.txt")

		want := "  adding: empty.txt\t(in=0) (out=0) (stored 0%)\n" +
			"total bytes=0, compressed=0 -> 0% savings\n"
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
	})
}

func TestZipRoundTrip(t *testing.T) {
	res, fsys := run(t, testFS(), nil, "-ry", "a.zip", ".")

	if res.exit != 0 {
		t.Fatalf("zip exit = %d (%s)", res.exit, res.stderr)
	}
	archive := fsys["a.zip"]
	out := fsx.MemoryFS{"a.zip": archive}
	var stdout, stderr bytes.Buffer
	rt := &coreutils.Runtime{Stdout: &stdout, Stderr: &stderr, FS: out}
	if err := unzip.New(rt).Run(t.Context(), "a.zip"); err != nil {
		t.Fatalf("unzip error = %v", err)
	}

	for name, want := range map[string]string{
		"a.txt":     "alpha\n",
		"b.txt":     strings.Repeat("bravo charlie delta\n", 30),
		"sub/c.txt": "gamma\n",
	} {
		if got := string(out[name].Data); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
		if got := out[name].Mode.Perm(); got != 0o644 {
			t.Errorf("%s mode = %o, want 644", name, got)
		}
		if got := out[name].ModTime; !got.Equal(mod) {
			t.Errorf("%s mtime = %v, want %v", name, got, mod)
		}
	}
	if link := out["link"]; link == nil || link.Mode&fs.ModeSymlink == 0 || string(link.Data) != "a.txt" {
		t.Errorf("link = %v, want a symbolic link to a.txt", link)
	}
	if _, ok := out["sub"]; !ok || out["sub"].Mode.Perm() != 0o755 {
		t.Errorf("sub = %v, want a directory", out["sub"])
	}
}

func TestZipContext(t *testing.T) {
	t.Run("the -@ reader reports the cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		rt := &coreutils.Runtime{Stdin: strings.NewReader("a.txt\nb.txt\n")}
		if _, err := stdinNames(ctx, rt, nil); !errors.Is(err, context.Canceled) {
			t.Errorf("stdinNames() error = %v, want %v", err, context.Canceled)
		}
	})

	t.Run("the -@ reader stops a blocked read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		release := make(chan struct{})
		var once sync.Once
		t.Cleanup(func() { once.Do(func() { close(release) }) })
		rt := &coreutils.Runtime{Stdin: blockedReader{release: release}}
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		done := make(chan error, 1)
		go func() {
			_, err := stdinNames(ctx, rt, nil)
			done <- err
		}()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("stdinNames() error = %v, want %v", err, context.Canceled)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("stdinNames() didn't return after the cancellation")
		}
		once.Do(func() { close(release) })
	})

	t.Run("the comment prompt reports the cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		var stdout bytes.Buffer
		a := &archiver{rt: &coreutils.Runtime{Stdin: strings.NewReader("hi\n.\n"), Stdout: &stdout}}
		if _, err := a.setComment(ctx, ""); !errors.Is(err, context.Canceled) {
			t.Errorf("setComment() error = %v, want %v", err, context.Canceled)
		}
	})

	t.Run("a cancelled -@ read returns the context error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		release := make(chan struct{})
		var once sync.Once
		t.Cleanup(func() { once.Do(func() { close(release) }) })

		var stdout, stderr bytes.Buffer
		rt := &coreutils.Runtime{
			Stdin:  blockedReader{release: release},
			Stdout: &stdout,
			Stderr: &stderr,
			FS:     testFS(),
		}
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		done := make(chan error, 1)
		go func() {
			done <- New(rt).Run(ctx, "-@", "a.zip")
		}()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Run() error = %v, want %v", err, context.Canceled)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run() didn't return after the cancellation")
		}
		once.Do(func() { close(release) })
	})
}

// blockedReader delivers nothing until its release channel closes.
type blockedReader struct {
	release <-chan struct{}
}

// Read implements io.Reader, blocking until the release.
func (b blockedReader) Read([]byte) (int, error) {
	<-b.release
	return 0, io.EOF
}

func TestZipUsage(t *testing.T) {
	t.Run("a bare invocation prints the usage screen", func(t *testing.T) {
		res, _ := run(t, nil, nil)

		if !strings.HasPrefix(res.stdout, "Usage: zip ") {
			t.Errorf("stdout = %q, want the usage screen", res.stdout)
		}
		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
	})

	t.Run("-v alone prints the version", func(t *testing.T) {
		res, _ := run(t, nil, nil, "-v")

		if !strings.HasPrefix(res.stdout, "zip (charm.land/turtle/coreutils)") {
			t.Errorf("stdout = %q, want the version text", res.stdout)
		}
	})

	t.Run("an unknown option is a usage error", func(t *testing.T) {
		res, _ := run(t, nil, nil, "-e", "a.zip")

		want := "zip: invalid option -- 'e'\nTry 'zip --help' for more information.\n"
		if res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		if res.exit != 2 {
			t.Errorf("exit = %d, want 2", res.exit)
		}
	})

	t.Run("an unknown long option is a usage error", func(t *testing.T) {
		res, _ := run(t, nil, nil, "--split-size", "10m", "a.zip")

		want := "zip: unrecognized option '--split-size'\nTry 'zip --help' for more information.\n"
		if res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		if res.exit != 2 {
			t.Errorf("exit = %d, want 2", res.exit)
		}
	})

	t.Run("options without an archive print the usage screen", func(t *testing.T) {
		res, _ := run(t, nil, nil, "-q")

		if !strings.HasPrefix(res.stdout, "Usage: zip ") {
			t.Errorf("stdout = %q, want the usage screen", res.stdout)
		}
		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
	})

	t.Run("no file system available", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		rt := &coreutils.Runtime{Stdout: &stdout, Stderr: &stderr}
		err := New(rt).Run(t.Context(), "a.zip", "a.txt")
		code, exit := 0, &coreutils.ExitError{}
		if !errors.As(err, &exit) {
			t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
		}
		code = exit.Code

		if code != 15 {
			t.Errorf("exit = %d, want 15", code)
		}
		if want := "zip: no file system available\n"; stderr.String() != want {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
	})

	t.Run("--help and --version", func(t *testing.T) {
		for _, test := range []struct {
			args []string
			want string
		}{
			{[]string{"--help"}, "Usage: zip "},
			{[]string{"--version"}, "zip (charm.land/turtle/coreutils)"},
		} {
			res, _ := run(t, nil, nil, test.args...)
			if !strings.HasPrefix(res.stdout, test.want) {
				t.Errorf("%v: stdout = %q, want %q", test.args, res.stdout, test.want)
			}
			if res.exit != 0 {
				t.Errorf("%v: exit = %d, want 0", test.args, res.exit)
			}
		}
	})
}
