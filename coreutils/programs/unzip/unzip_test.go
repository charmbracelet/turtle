package unzip

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/fsx"
)

// mod is the modification time of the fixtures which store one, on an even
// second so the 2-second granularity of the format keeps it exactly.
var mod = time.Date(2024, 3, 2, 1, 2, 4, 0, time.UTC)

// older and newer are the modification times of the disk copies the
// freshening tests check against.
var (
	older = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
)

// entry is one member of the archives the tests build.
type entry struct {
	name   string
	data   string
	mode   fs.FileMode
	store  bool
	raw    bool
	method uint16
	flags  uint16
}

// file is the default entry mode and method.
func (e entry) fileMode() fs.FileMode {
	if e.mode == 0 {
		return 0o644
	}
	return e.mode
}

// zipData builds the contents of a ZIP archive holding entries.
func zipData(t *testing.T, entries ...entry) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, e := range entries {
		method := uint16(zip.Deflate)
		if e.store || e.raw || strings.HasSuffix(e.name, "/") {
			method = e.method
		}
		hdr := &zip.FileHeader{
			Name:     e.name,
			Method:   method,
			Modified: mod,
			Flags:    e.flags,
		}
		hdr.SetMode(e.fileMode())
		if e.raw {
			hdr.CRC32 = crc32.ChecksumIEEE([]byte(e.data))
			hdr.CompressedSize64 = uint64(len(e.data))
			hdr.UncompressedSize64 = uint64(len(e.data))
			fw, err := writer.CreateRaw(hdr)
			if err != nil {
				t.Fatalf("CreateRaw(%s): %v", e.name, err)
			}
			if _, err := fw.Write([]byte(e.data)); err != nil {
				t.Fatalf("write(%s): %v", e.name, err)
			}
			continue
		}
		fw, err := writer.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("CreateHeader(%s): %v", e.name, err)
		}
		if _, err := fw.Write([]byte(e.data)); err != nil {
			t.Fatalf("write(%s): %v", e.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	return buf.Bytes()
}

// zipFile is a file system entry holding a ZIP archive of entries.
func zipFile(t *testing.T, entries ...entry) *fsx.MemoryFile {
	t.Helper()

	return &fsx.MemoryFile{Data: zipData(t, entries...), Mode: 0o644, ModTime: mod}
}

// archive holds the members the extraction tests work on:
//
//	a.txt:   "alpha\n"
//	dir/     a directory entry
//	dir/b.c: "bravo\n"
//	link:    a symbolic link to a.txt
func archive(t *testing.T) *fsx.MemoryFile {
	t.Helper()

	// The link entry needs the standard writer, so its times are set.
	return zipFile(t,
		entry{name: "a.txt", data: "alpha\n"},
		entry{name: "dir/", mode: fs.ModeDir | 0o755},
		entry{name: "dir/b.c", data: "bravo\n", store: true},
		entry{name: "link", data: "a.txt", mode: fs.ModeSymlink | 0o777, store: true},
	)
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

// wantData checks the contents and modes of the files of a file system, and
// that the names given as absent hold nothing.
func wantData(t *testing.T, fsys fsx.MemoryFS, data map[string]string, absent ...string) {
	t.Helper()

	for name, want := range data {
		file, ok := fsys[name]
		if !ok {
			t.Errorf("%s: missing file", name)
			continue
		}
		if got := string(file.Data); got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
	for _, name := range absent {
		if _, ok := fsys[name]; ok {
			t.Errorf("%s: unexpected file", name)
		}
	}
}

func TestUnzipExtract(t *testing.T) {
	t.Run("extracts every member", func(t *testing.T) {
		res, fsys := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "a.zip")

		want := "Archive:  a.zip\n" +
			fmt.Sprintf("  inflating: %-25s\n", "a.txt") +
			"   creating: dir\n" +
			fmt.Sprintf(" extracting: %-25s\n", "dir/b.c") +
			fmt.Sprintf(" extracting: %-25s\n", "link")
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
		if res.stderr != "" || res.exit != 0 {
			t.Errorf("stderr = %q, exit = %d, want empty and 0", res.stderr, res.exit)
		}
		wantData(t, fsys, map[string]string{
			"a.txt":   "alpha\n",
			"dir/b.c": "bravo\n",
		})
		if link, ok := fsys["link"]; !ok || link.Mode&fs.ModeSymlink == 0 || string(link.Data) != "a.txt" {
			t.Errorf("link = %v, want a symbolic link to a.txt", link)
		}
	})

	t.Run("restores modes and times", func(t *testing.T) {
		res, fsys := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "a.zip")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		if got := fsys["a.txt"].Mode.Perm(); got != 0o644 {
			t.Errorf("a.txt mode = %o, want 644", got)
		}
		if got := fsys["a.txt"].ModTime; !got.Equal(mod) {
			t.Errorf("a.txt mtime = %v, want %v", got, mod)
		}
		if got := fsys["dir"].Mode.Perm(); got != 0o755 {
			t.Errorf("dir mode = %o, want 755", got)
		}
		if got := fsys["dir"].ModTime; !got.Equal(mod) {
			t.Errorf("dir mtime = %v, want %v", got, mod)
		}
	})

	t.Run("extracts into a directory", func(t *testing.T) {
		res, fsys := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "-d", "out", "a.zip")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		want := "Archive:  a.zip\n" +
			fmt.Sprintf("  inflating: %-25s\n", "out/a.txt") +
			"   creating: out/dir\n" +
			fmt.Sprintf(" extracting: %-25s\n", "out/dir/b.c") +
			fmt.Sprintf(" extracting: %-25s\n", "out/link")
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
		wantData(t, fsys, map[string]string{"out/a.txt": "alpha\n"}, "a.txt")
	})

	t.Run("junks paths", func(t *testing.T) {
		res, fsys := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "-j", "a.zip", "dir/b.c")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"b.c": "bravo\n"}, "dir/b.c")
	})

	t.Run("selects the members by pattern", func(t *testing.T) {
		res, fsys := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "a.zip", "*.c")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"dir/b.c": "bravo\n"}, "a.txt", "link")
	})

	t.Run("excludes the members by pattern", func(t *testing.T) {
		res, fsys := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "a.zip", "-x", "*.txt")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"dir/b.c": "bravo\n"}, "a.txt")
		if _, ok := fsys["link"]; !ok {
			t.Error("link: missing symbolic link")
		}
	})

	t.Run("a plain directory operand matches nothing", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "a.zip", "dir")

		if res.exit != 11 {
			t.Errorf("exit = %d, want 11", res.exit)
		}
		if got := res.stderr; got != "caution: filename not matched:  dir\n" {
			t.Errorf("stderr = %q, want the caution", got)
		}
	})

	t.Run("an unmatched member keeps the matched ones", func(t *testing.T) {
		res, fsys := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "a.zip", "nope", "dir/")

		if res.exit != 11 {
			t.Errorf("exit = %d, want 11", res.exit)
		}
		if got := res.stderr; got != "caution: filename not matched:  nope\n" {
			t.Errorf("stderr = %q, want the caution", got)
		}
		if _, ok := fsys["dir"]; !ok {
			t.Error("dir: missing directory")
		}
	})

	t.Run("quiet suppresses the messages", func(t *testing.T) {
		res, fsys := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "-q", "a.zip")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		if res.stdout != "" || res.stderr != "" {
			t.Errorf("stdout = %q, stderr = %q, want empty", res.stdout, res.stderr)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"})
	})

	t.Run("-qq suppresses the caution too", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "-qq", "a.zip", "nope")

		if res.exit != 11 {
			t.Errorf("exit = %d, want 11", res.exit)
		}
		if res.stderr != "" {
			t.Errorf("stderr = %q, want empty", res.stderr)
		}
	})
}

func TestUnzipOverwrite(t *testing.T) {
	existing := func() fsx.MemoryFS {
		return fsx.MemoryFS{
			"a.zip": archive(t),
			"a.txt": {Data: []byte("old\n"), Mode: 0o644, ModTime: mod},
		}
	}

	t.Run("-o overwrites without prompting", func(t *testing.T) {
		res, fsys := run(t, existing(), nil, "-o", "a.zip")

		if res.exit != 0 || res.stderr != "" {
			t.Fatalf("exit = %d, stderr = %q", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"})
	})

	t.Run("-n never overwrites", func(t *testing.T) {
		res, fsys := run(t, existing(), nil, "-n", "a.zip")

		if res.exit != 0 || res.stderr != "" {
			t.Fatalf("exit = %d, stderr = %q", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"a.txt": "old\n"})
	})

	t.Run("the prompt asks for each file", func(t *testing.T) {
		res, fsys := run(t, existing(), strings.NewReader("y\n"), "a.zip")

		want := "replace a.txt? [y]es, [n]o, [A]ll, [N]one, [r]ename: "
		if res.stderr != want {
			t.Errorf("stderr = %q, want the prompt %q", res.stderr, want)
		}
		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"})
	})

	t.Run("a no answer keeps the disk copy", func(t *testing.T) {
		res, fsys := run(t, existing(), strings.NewReader("n\n"), "a.zip")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
		wantData(t, fsys, map[string]string{"a.txt": "old\n"})
	})

	t.Run("an exhausted standard input answers none", func(t *testing.T) {
		res, fsys := run(t, existing(), strings.NewReader(""), "a.zip")

		want := "replace a.txt? [y]es, [n]o, [A]ll, [N]one, [r]ename: " +
			" NULL\n(EOF or read error, treating as \"[N]one\" ...)\n"
		if res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		wantData(t, fsys, map[string]string{"a.txt": "old\n"})
	})

	t.Run("an invalid answer is asked again", func(t *testing.T) {
		res, fsys := run(t, existing(), strings.NewReader("x\nn\n"), "a.zip")

		want := "replace a.txt? [y]es, [n]o, [A]ll, [N]one, [r]ename: " +
			"error:  invalid response [x]\n" +
			"replace a.txt? [y]es, [n]o, [A]ll, [N]one, [r]ename: "
		if res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		wantData(t, fsys, map[string]string{"a.txt": "old\n"})
	})

	t.Run("the A answer overwrites the files below", func(t *testing.T) {
		fsys := fsx.MemoryFS{
			"a.zip": zipFile(t, entry{name: "a.txt", data: "alpha\n"}, entry{name: "b.txt", data: "bravo\n"}),
			"a.txt": {Data: []byte("old\n"), Mode: 0o644},
			"b.txt": {Data: []byte("old\n"), Mode: 0o644},
		}
		res, fsys := run(t, fsys, strings.NewReader("A\n"), "a.zip")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n", "b.txt": "bravo\n"})
	})

	t.Run("the r answer extracts under a new name", func(t *testing.T) {
		res, fsys := run(t, existing(), strings.NewReader("r\nnew.txt\n"), "a.zip", "a.txt")

		want := "replace a.txt? [y]es, [n]o, [A]ll, [N]one, [r]ename: new name: "
		if res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		wantData(t, fsys, map[string]string{"a.txt": "old\n", "new.txt": "alpha\n"})
	})
	t.Run("-n never overwrites a link", func(t *testing.T) {
		fsys := fsx.MemoryFS{
			"a.zip": archive(t),
			"link":  {Data: []byte("old.txt"), Mode: fs.ModeSymlink | 0o777, ModTime: mod},
		}
		res, fsys := run(t, fsys, nil, "-n", "a.zip")

		if res.exit != 0 || res.stderr != "" {
			t.Fatalf("exit = %d, stderr = %q", res.exit, res.stderr)
		}
		if got := string(fsys["link"].Data); got != "old.txt" {
			t.Errorf("link = %q, want the disk copy kept", got)
		}
	})

	t.Run("the prompt asks for a link too", func(t *testing.T) {
		fsys := fsx.MemoryFS{
			"a.zip": archive(t),
			"link":  {Data: []byte("old.txt"), Mode: fs.ModeSymlink | 0o777, ModTime: mod},
		}
		res, fsys := run(t, fsys, strings.NewReader("y\n"), "a.zip", "link")

		want := "replace link? [y]es, [n]o, [A]ll, [N]one, [r]ename: "
		if res.stderr != want {
			t.Errorf("stderr = %q, want the prompt", res.stderr)
		}
		if got := string(fsys["link"].Data); got != "a.txt" {
			t.Errorf("link = %q, want the archived target", got)
		}
	})

	t.Run("a no answer keeps the disk link", func(t *testing.T) {
		fsys := fsx.MemoryFS{
			"a.zip": archive(t),
			"link":  {Data: []byte("old.txt"), Mode: fs.ModeSymlink | 0o777, ModTime: mod},
		}
		res, fsys := run(t, fsys, strings.NewReader("n\n"), "a.zip", "link")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
		if got := string(fsys["link"].Data); got != "old.txt" {
			t.Errorf("link = %q, want the disk copy kept", got)
		}
	})
}

func TestUnzipFreshen(t *testing.T) {
	freshFS := func(diskMod time.Time, diskData string) fsx.MemoryFS {
		return fsx.MemoryFS{
			"a.zip": archive(t),
			"a.txt": {Data: []byte(diskData), Mode: 0o644, ModTime: diskMod},
		}
	}

	t.Run("-f freshens the older disk copies", func(t *testing.T) {
		res, fsys := run(t, freshFS(older, "old\n"), nil, "-fo", "a.zip")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"}, "dir/b.c", "link")
	})

	t.Run("-f leaves the newer disk copies", func(t *testing.T) {
		res, fsys := run(t, freshFS(newer, "new\n"), nil, "-fo", "a.zip")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"a.txt": "new\n"}, "dir/b.c", "link")
	})

	t.Run("-u extracts the missing files too", func(t *testing.T) {
		fsys := fsx.MemoryFS{"a.zip": archive(t)}
		res, fsys := run(t, fsys, nil, "-uo", "a.zip")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n", "dir/b.c": "bravo\n"})
	})
}

func TestUnzipStdout(t *testing.T) {
	t.Run("-p writes the member data alone", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "-p", "a.zip", "a.txt")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		if res.stdout != "alpha\n" {
			t.Errorf("stdout = %q, want the member data", res.stdout)
		}
	})

	t.Run("-c prints the member names", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "-c", "a.zip", "a.txt")

		want := "Archive:  a.zip\n extracting: a.txt                    \nalpha\n"
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
	})
	t.Run("-p reports a bad CRC as corrupted data", func(t *testing.T) {
		data := zipData(t, entry{name: "a.txt", data: "alpha\n", store: true})
		data[bytes.Index(data, []byte("alpha\n"))] ^= 0xFF
		res, _ := run(t, fsx.MemoryFS{"a.zip": {Data: data, Mode: 0o644}}, nil, "-p", "a.zip")

		bad := []byte("alpha\n")
		bad[0] ^= 0xFF
		want := fmt.Sprintf("unzip: error:  bad CRC %08x  (should be %08x)\n",
			crc32.ChecksumIEEE(bad), crc32.ChecksumIEEE([]byte("alpha\n")))
		if res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		if res.exit != 2 {
			t.Errorf("exit = %d, want 2", res.exit)
		}
	})
}

func TestUnzipList(t *testing.T) {
	t.Run("the short listing", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "-l", "a.zip")

		want := "Archive:  a.zip\n" +
			"  Length      Date    Time    Name\n" +
			"---------  ---------- -----   ----\n" +
			fmt.Sprintf("%9d  03-02-2024 01:02   a.txt\n", len("alpha\n")) +
			fmt.Sprintf("%9d  03-02-2024 01:02   dir/\n", 0) +
			fmt.Sprintf("%9d  03-02-2024 01:02   dir/b.c\n", len("bravo\n")) +
			fmt.Sprintf("%9d  03-02-2024 01:02   link\n", len("a.txt")) +
			"---------                     -------\n" +
			"       17                     4 files\n"
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
	})

	t.Run("the members are selected by pattern", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "-l", "a.zip", "*.c")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		want := "Archive:  a.zip\n" +
			"  Length      Date    Time    Name\n" +
			"---------  ---------- -----   ----\n" +
			fmt.Sprintf("%9d  03-02-2024 01:02   dir/b.c\n", len("bravo\n")) +
			"---------                     -------\n" +
			"        6                     1 file\n"
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
	})

	t.Run("the verbose listing is selected too", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "-v", "a.zip", "*.c")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		if !strings.Contains(res.stdout, "dir/b.c") || strings.Contains(res.stdout, "a.txt") {
			t.Errorf("stdout = %q, want only dir/b.c", res.stdout)
		}
	})

	t.Run("an unmatched member exits 11", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "-l", "a.zip", "nope")

		if res.exit != 11 {
			t.Errorf("exit = %d, want 11", res.exit)
		}
	})

	t.Run("the singular total", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{"a.zip": zipFile(t, entry{name: "a.txt", data: "alpha\n"})}, nil, "-l", "a.zip")

		if !strings.HasSuffix(res.stdout, "        6                     1 file\n") {
			t.Errorf("stdout = %q, want the singular total", res.stdout)
		}
	})

	t.Run("the verbose listing", func(t *testing.T) {
		data := "alpha bravo charlie delta\n"
		big := zipFile(t,
			entry{name: "big.txt", data: data},
			entry{name: "dir/", mode: fs.ModeDir | 0o755},
		)
		res, _ := run(t, fsx.MemoryFS{"a.zip": big}, nil, "-v", "a.zip")

		size, comp := uint64(len(data)), compSize(t, data)
		want := "Archive:  a.zip\n" +
			" Length   Method    Size  Cmpr    Date    Time   CRC-32   Name\n" +
			"--------  ------  ------- ---- ---------- ----- --------  ----\n" +
			fmt.Sprintf("%8d  %-6s%9d %3d%% 03-02-2024 01:02 %08x  big.txt\n",
				size, "Defl:N", comp, percent(size, comp), crc32.ChecksumIEEE([]byte(data))) +
			fmt.Sprintf("%8d  %-6s%9d %3d%% 03-02-2024 01:02 %08x  dir/\n",
				uint64(0), "Stored", uint64(0), 0, crc32.ChecksumIEEE(nil)) +
			"--------          -------  ---                            -------\n" +
			fmt.Sprintf("%8d%17d %3d%%                            2 files\n", size, comp, percent(size, comp))
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
	})

	t.Run("the comment is shown", func(t *testing.T) {
		data := zipData(t, entry{name: "a.txt", data: "alpha\n"})
		data = withComment(t, data, "a comment")
		res, _ := run(t, fsx.MemoryFS{"a.zip": {Data: data, Mode: 0o644}}, nil, "-l", "a.zip")

		if !strings.Contains(res.stdout, "Archive:  a.zip\na comment\n") {
			t.Errorf("stdout = %q, want the comment", res.stdout)
		}
	})

	t.Run("-z prints only the comment", func(t *testing.T) {
		data := withComment(t, zipData(t, entry{name: "a.txt", data: "alpha\n"}), "a comment")
		res, _ := run(t, fsx.MemoryFS{"a.zip": {Data: data, Mode: 0o644}}, nil, "-z", "a.zip")

		if res.stdout != "Archive:  a.zip\na comment\n" {
			t.Errorf("stdout = %q, want the comment alone", res.stdout)
		}
	})

	t.Run("-z without a comment prints the header alone", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "-z", "a.zip")

		if res.stdout != "Archive:  a.zip\n" {
			t.Errorf("stdout = %q, want the header alone", res.stdout)
		}
	})
}

// compSize returns the size data takes once deflated at the default level.
func compSize(t *testing.T, data string) uint64 {
	t.Helper()

	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, -1)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(data))
	w.Close()
	return uint64(buf.Len())
}

// withComment appends a central-directory comment to an archive by
// rebuilding it with the standard writer.
func withComment(t *testing.T, data []byte, comment string) []byte {
	t.Helper()

	zr, size := bytes.NewReader(data), int64(len(data))
	reader, err := zip.NewReader(zr, size)
	if err != nil {
		t.Fatalf("NewReader(): %v", err)
	}
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	writer.SetComment(comment)
	for _, file := range reader.File {
		if err := writer.Copy(file); err != nil {
			t.Fatalf("Copy(%s): %v", file.Name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	return buf.Bytes()
}

func TestUnzipTest(t *testing.T) {
	t.Run("reports the members and the summary", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "-t", "a.zip")

		want := "Archive:  a.zip\n" +
			fmt.Sprintf("    testing: %-25s OK\n", "a.txt") +
			fmt.Sprintf("    testing: %-25s OK\n", "dir/") +
			fmt.Sprintf("    testing: %-25s OK\n", "dir/b.c") +
			fmt.Sprintf("    testing: %-25s OK\n", "link") +
			"No errors detected in compressed data of a.zip.\n"
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
	})

	t.Run("a bad CRC is reported", func(t *testing.T) {
		data := zipData(t, entry{name: "a.txt", data: "alpha\n", store: true})
		data[bytes.Index(data, []byte("alpha\n"))] ^= 0xFF
		bad := []byte("alpha\n")
		bad[0] ^= 0xFF
		res, _ := run(t, fsx.MemoryFS{"a.zip": {Data: data, Mode: 0o644}}, nil, "-t", "a.zip")

		want := "Archive:  a.zip\n" +
			fmt.Sprintf("    testing: %-25s bad CRC %08x  (should be %08x)\n",
				"a.txt", crc32.ChecksumIEEE(bad), crc32.ChecksumIEEE([]byte("alpha\n"))) +
			"At least one error was detected in a.zip.\n"
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
		if res.exit != 2 {
			t.Errorf("exit = %d, want 2", res.exit)
		}
	})

	t.Run("an encrypted member is refused", func(t *testing.T) {
		data := zipData(t, entry{name: "a.txt", data: "alpha\n", raw: true, flags: 0x1})
		res, _ := run(t, fsx.MemoryFS{"a.zip": {Data: data, Mode: 0o644}}, nil, "-t", "a.zip")

		want := "Archive:  a.zip\n" +
			fmt.Sprintf("    testing: %-25s %s\n", "a.txt", "unsupported encrypted member") +
			"At least one error was detected in a.zip.\n"
		if res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
		if res.exit != 81 {
			t.Errorf("exit = %d, want 81", res.exit)
		}
	})

	t.Run("an unknown method is refused", func(t *testing.T) {
		data := zipData(t, entry{name: "a.txt", data: "alpha\n", store: true})
		data[bytes.Index(data, []byte("PK\x01\x02"))+10] = 99
		res, _ := run(t, fsx.MemoryFS{"a.zip": {Data: data, Mode: 0o644}}, nil, "-t", "a.zip")

		if res.exit != 81 {
			t.Errorf("exit = %d, want 81", res.exit)
		}
		if !strings.Contains(res.stdout, "unknown compression method 99") {
			t.Errorf("stdout = %q, want the method report", res.stdout)
		}
	})
}

func TestUnzipErrors(t *testing.T) {
	t.Run("a missing archive", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{}, nil, "missing.zip")

		want := "unzip:  cannot find or open missing.zip, missing.zip.zip or missing.zip.ZIP.\n"
		if res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		if res.exit != 9 {
			t.Errorf("exit = %d, want 9", res.exit)
		}
	})

	t.Run("a corrupt archive", func(t *testing.T) {
		res, _ := run(t, fsx.MemoryFS{"bad.zip": {Data: []byte("garbage"), Mode: 0o644}}, nil, "bad.zip")

		want := "Archive:  bad.zip\n" +
			"  End-of-central-directory signature not found.  Either this file is not\n" +
			"  a zipfile, or it constitutes one disk of a multi-part archive.  In the\n" +
			"  latter case the central directory and zipfile comment will be found on\n" +
			"  the last disk(s) of this archive.\n" +
			"unzip:  cannot find zipfile directory in one of bad.zip or\n" +
			"        bad.zip.zip, and cannot find bad.zip.ZIP, period.\n"
		if res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		if res.exit != 9 {
			t.Errorf("exit = %d, want 9", res.exit)
		}
	})

	t.Run("the zip suffix is tried", func(t *testing.T) {
		res, fsys := run(t, fsx.MemoryFS{"a.zip": archive(t)}, nil, "a")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"})
	})

	t.Run("a member escaping the directory is refused", func(t *testing.T) {
		data := zipData(t, entry{name: "../evil", data: "boom\n"})
		res, fsys := run(t, fsx.MemoryFS{"a.zip": {Data: data, Mode: 0o644}}, nil, "a.zip")

		if res.exit != 1 {
			t.Errorf("exit = %d, want 1", res.exit)
		}
		if want := "unzip: ../evil: skipped \"../\" path component(s)\n"; res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		wantData(t, fsys, nil, "evil", "../evil")
	})

	t.Run("every leading slash is dropped", func(t *testing.T) {
		data := zipData(t, entry{name: "//tmp/target", data: "boom\n"})
		res, fsys := run(t, fsx.MemoryFS{"a.zip": {Data: data, Mode: 0o644}}, nil, "a.zip")

		if res.exit != 0 {
			t.Fatalf("exit = %d (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"tmp/target": "boom\n"}, "target")
	})
}

func TestUnzipContext(t *testing.T) {
	t.Run("the prompt reader reports the cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		r := &runner{rt: &coreutils.Runtime{Stdin: strings.NewReader("y\n")}}
		if _, err := r.readLine(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("readLine() error = %v, want %v", err, context.Canceled)
		}
	})

	t.Run("the replace prompt returns the cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		var stderr bytes.Buffer
		r := &runner{rt: &coreutils.Runtime{Stdin: strings.NewReader("y\n"), Stderr: &stderr}}
		if _, _, err := r.replace(ctx, "a.txt"); !errors.Is(err, context.Canceled) {
			t.Errorf("replace() error = %v, want %v", err, context.Canceled)
		}
	})

	t.Run("the replace prompt stops a blocked read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		release := make(chan struct{})
		var once sync.Once
		t.Cleanup(func() { once.Do(func() { close(release) }) })

		var stderr bytes.Buffer
		r := &runner{rt: &coreutils.Runtime{Stdin: blockedReader{release: release}, Stderr: &stderr}}
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		done := make(chan error, 1)
		go func() {
			_, _, err := r.replace(ctx, "a.txt")
			done <- err
		}()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("replace() error = %v, want %v", err, context.Canceled)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("replace() didn't return after the cancellation")
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

func TestUnzipUsage(t *testing.T) {
	t.Run("a bare invocation prints the usage screen", func(t *testing.T) {
		res, _ := run(t, nil, nil)

		if !strings.HasPrefix(res.stdout, "Usage: unzip ") {
			t.Errorf("stdout = %q, want the usage screen", res.stdout)
		}
		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
	})

	t.Run("-v alone prints the version", func(t *testing.T) {
		res, _ := run(t, nil, nil, "-v")

		if !strings.HasPrefix(res.stdout, "unzip (charm.land/turtle/coreutils)") {
			t.Errorf("stdout = %q, want the version text", res.stdout)
		}
	})

	t.Run("an unknown option is a usage error", func(t *testing.T) {
		res, _ := run(t, nil, nil, "-Z", "a.zip")

		want := "unzip: invalid option -- 'Z'\nTry 'unzip --help' for more information.\n"
		if res.stderr != want {
			t.Errorf("stderr = %q, want %q", res.stderr, want)
		}
		if res.exit != 2 {
			t.Errorf("exit = %d, want 2", res.exit)
		}
	})

	t.Run("--help and --version", func(t *testing.T) {
		for _, test := range []struct {
			args []string
			want string
		}{
			{[]string{"--help"}, "Usage: unzip "},
			{[]string{"--version"}, "unzip (charm.land/turtle/coreutils)"},
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
