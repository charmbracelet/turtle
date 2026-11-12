package gzip

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"runtime"
	"strings"
	"testing"
	"time"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

// testFS holds the fixtures used by the tests:
//
//	a.txt:     "alpha\n"
//	b.txt:     "beta\n"
//	sub/c.txt: "gamma\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":     {Data: []byte("alpha\n"), Mode: 0o644},
		"b.txt":     {Data: []byte("beta\n"), Mode: 0o644},
		"sub/c.txt": {Data: []byte("gamma\n"), Mode: 0o644},
	}
}

// mtime is the modification time of the fixtures which store one.
var mtime = time.Date(2024, 3, 2, 1, 2, 3, 0, time.UTC)

// gz builds the contents of a gzip file holding data, storing name and mod in
// its header.
func gz(t *testing.T, name string, mod time.Time, data string) string {
	t.Helper()

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	writer.Name = name
	writer.ModTime = mod
	if _, err := writer.Write([]byte(data)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.String()
}

// gzFile is a file system entry holding a gzip file of the given contents.
func gzFile(t *testing.T, data string) *fsx.MemFile {
	t.Helper()

	return &fsx.MemFile{Data: []byte(gz(t, "", mtime, data)), Mode: 0o644}
}

// result holds what a run of a program produced.
type result struct {
	stdout string
	stderr string
	exit   int
}

// run executes the program built by new against the given arguments, returning
// what it wrote along with the file system it left behind.
// environ converts a map to a list of KEY=VALUE strings.
func environ(env map[string]string) []string {
	entries := make([]string, 0, len(env))
	for name, value := range env {
		entries = append(entries, name+"="+value)
	}
	return entries
}

func run(t *testing.T, new coreutils.NewFunc, fsys fsx.MemFS, stdin io.Reader, env map[string]string, args ...string) (result, fsx.MemFS) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	rt := &coreutils.Runtime{
		Stdin:   stdin,
		Stdout:  &stdout,
		Stderr:  &stderr,
		FS:      fsys,
		Environ: func() []string { return environ(env) },
	}

	err := new(rt).Run(t.Context(), args...)
	code := 0
	var exitErr *coreutils.ExitError
	if err != nil {
		if !errors.As(err, &exitErr) {
			t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
		}
		code = exitErr.Code
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), exit: code}, fsys
}

// wantData checks the contents of the files of a file system, and that the
// names given as absent hold nothing.
func wantData(t *testing.T, fsys fsx.MemFS, data map[string]string, absent ...string) {
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

// names lists the names of a file system, for failure messages.
func names(fsys fsx.MemFS) []string {
	list := make([]string, 0, len(fsys))
	for name := range fsys {
		list = append(list, name)
	}
	return list
}

func TestGzipCompresses(t *testing.T) {
	t.Run("replaces each operand", func(t *testing.T) {
		res, fsys := run(t, New, testFS(), nil, nil, "a.txt", "b.txt")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, nil, "a.txt", "b.txt")
		for _, name := range []string{"a.txt.gz", "b.txt.gz"} {
			if _, ok := fsys[name]; !ok {
				t.Errorf("%s: missing file, have %v", name, names(fsys))
			}
		}
	})

	t.Run("round-trips through gunzip", func(t *testing.T) {
		_, fsys := run(t, New, testFS(), nil, nil, "a.txt")
		res, fsys := run(t, Gunzip, fsys, nil, nil, "a.txt.gz")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"}, "a.txt.gz")
	})

	t.Run("keeps the mode, times and owner", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a.txt": {Data: []byte("alpha\n"), Mode: 0o640, ModTime: mtime, Uid: 100, Gid: 200},
		}

		_, fsys = run(t, New, fsys, nil, nil, "a.txt")

		file := fsys["a.txt.gz"]
		if file == nil {
			t.Fatal("a.txt.gz: missing file")
		}
		if file.Mode != 0o640 {
			t.Errorf("Mode = %o, want 640", file.Mode)
		}
		if !file.ModTime.Equal(mtime) {
			t.Errorf("ModTime = %v, want %v", file.ModTime, mtime)
		}
		if file.Uid != 100 || file.Gid != 200 {
			t.Errorf("Uid, Gid = %d, %d, want 100, 200", file.Uid, file.Gid)
		}
	})

	t.Run("stores the name and modification time", func(t *testing.T) {
		fsys := fsx.MemFS{"a.txt": {Data: []byte("alpha\n"), Mode: 0o644, ModTime: mtime}}

		_, fsys = run(t, New, fsys, nil, nil, "a.txt")

		header := readHeader(t, fsys, "a.txt.gz")
		if header.Name != "a.txt" {
			t.Errorf("stored Name = %q, want a.txt", header.Name)
		}
		if !header.ModTime.Equal(mtime) {
			t.Errorf("stored ModTime = %v, want %v", header.ModTime, mtime)
		}
	})
}

func TestGzipOptions(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"-c leaves the file alone", []string{"-c", "a.txt"}},
		{"--stdout leaves the file alone", []string{"--stdout", "a.txt"}},
		{"--to-stdout leaves the file alone", []string{"--to-stdout", "a.txt"}},
		{"-k keeps the input", []string{"-k", "a.txt"}},
		{"--keep keeps the input", []string{"--keep", "a.txt"}},
		{"-n leaves the name out", []string{"-n", "a.txt"}},
		{"--no-name leaves the name out", []string{"--no-name", "a.txt"}},
		{"-1 compresses fast", []string{"-1", "a.txt"}},
		{"-9 compresses best", []string{"-9", "a.txt"}},
		{"-0 stores without compressing", []string{"-0", "a.txt"}},
		{"--fast compresses fast", []string{"--fast", "a.txt"}},
		{"--best compresses best", []string{"--best", "a.txt"}},
		{"-q is quiet", []string{"-q", "a.txt"}},
		{"--quiet is quiet", []string{"--quiet", "a.txt"}},
		{"-S uses another suffix", []string{"-S", ".zip", "a.txt"}},
		{"--suffix uses another suffix", []string{"--suffix=.zip", "a.txt"}},
		{"-v is verbose", []string{"-v", "a.txt"}},
		{"--verbose is verbose", []string{"--verbose", "a.txt"}},
		{"-f forces it", []string{"-f", "a.txt"}},
		{"options are clustered", []string{"-kv", "a.txt"}},
		{"an operand named like an option is taken as a file", []string{"--", "b.txt"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, fsys := run(t, New, testFS(), nil, nil, test.args...)

			if res.exit != 0 {
				t.Fatalf("exit = %d, want 0 (%s)", res.exit, res.stderr)
			}
			for _, name := range []string{"a.txt", "b.txt"} {
				file, ok := fsys[name]
				if !ok {
					continue
				}
				if got, want := string(file.Data), map[string]string{"a.txt": "alpha\n", "b.txt": "beta\n"}[name]; got != want {
					t.Errorf("%s: got %q, want the original contents", name, got)
				}
			}
		})
	}

	t.Run("-c writes the compressed data out", func(t *testing.T) {
		fsys := fsx.MemFS{"a.txt": {Data: []byte("alpha\n"), Mode: 0o644}}
		res, fsys := run(t, New, fsys, nil, nil, "-c", "a.txt")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		if got := gunzipAll(t, res.stdout); got != "alpha\n" {
			t.Errorf("stdout = %q, want alpha\\n", got)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"})
	})

	t.Run("-k keeps the input file", func(t *testing.T) {
		_, fsys := run(t, New, testFS(), nil, nil, "-k", "a.txt")

		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"})
		if _, ok := fsys["a.txt.gz"]; !ok {
			t.Error("a.txt.gz: missing file")
		}
	})

	t.Run("-S names the output with another suffix", func(t *testing.T) {
		_, fsys := run(t, New, testFS(), nil, nil, "-S", ".zip", "a.txt")

		wantData(t, fsys, nil, "a.txt")
		if _, ok := fsys["a.txt.zip"]; !ok {
			t.Errorf("files = %v, want a.txt.zip", names(fsys))
		}
	})

	t.Run("-v reports the ratio and the output", func(t *testing.T) {
		res, _ := run(t, New, testFS(), nil, nil, "-v", "a.txt")

		if !strings.HasPrefix(res.stderr, "a.txt: ") || !strings.Contains(res.stderr, "-- replaced with a.txt.gz") {
			t.Errorf("stderr = %q, want it to report a.txt and its replacement", res.stderr)
		}
	})

	t.Run("-n leaves the name and time out of the file", func(t *testing.T) {
		fsys := fsx.MemFS{"a.txt": {Data: []byte("alpha\n"), Mode: 0o644, ModTime: mtime}}

		_, fsys = run(t, New, fsys, nil, nil, "-n", "a.txt")

		header := readHeader(t, fsys, "a.txt.gz")
		if header.Name != "" {
			t.Errorf("stored Name = %q, want none", header.Name)
		}
		if !header.ModTime.IsZero() {
			t.Errorf("stored ModTime = %v, want the zero time", header.ModTime)
		}
	})

	t.Run("an operand which already has a suffix is left alone", func(t *testing.T) {
		fsys := fsx.MemFS{"a.txt.gz": {Data: []byte("data"), Mode: 0o644}}

		res, fsys := run(t, New, fsys, nil, nil, "a.txt.gz")

		if res.exit != 2 {
			t.Errorf("exit = %d, want 2 (%s)", res.exit, res.stderr)
		}
		if !strings.Contains(res.stderr, "already has .gz suffix") {
			t.Errorf("stderr = %q, want it to mention the suffix", res.stderr)
		}
		if _, ok := fsys["a.txt.gz.gz"]; ok {
			t.Error("a.txt.gz.gz: unexpected double-compressed output")
		}
		wantData(t, fsys, map[string]string{"a.txt.gz": "data"})
	})

	t.Run("-f compresses an operand which already has a suffix", func(t *testing.T) {
		fsys := fsx.MemFS{"a.txt.gz": {Data: []byte(strings.Repeat("alpha ", 30)), Mode: 0o644}}

		res, fsys := run(t, New, fsys, nil, nil, "-f", "a.txt.gz")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, nil, "a.txt.gz")
	})

	t.Run("-q hides the diagnostics", func(t *testing.T) {
		res, _ := run(t, New, testFS(), nil, nil, "-q", "missing.txt")

		if res.exit != 1 {
			t.Errorf("exit = %d, want 1", res.exit)
		}
		if res.stderr != "" {
			t.Errorf("stderr = %q, want none", res.stderr)
		}
	})
}

func TestGzipUsage(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"an unknown option", []string{"-Z", "a.txt"}, "gzip: invalid option -- 'Z'\nTry 'gzip --help' for more information.\n"},
		{"an unknown long option", []string{"--zest", "a.txt"}, "gzip: unrecognized option '--zest'\nTry 'gzip --help' for more information.\n"},
		{"a long option with a value it doesn't take", []string{"--force=yes"}, "gzip: option '--force=yes' doesn't allow an argument\nTry 'gzip --help' for more information.\n"},
		{"-S without an argument", []string{"-S"}, "gzip: option requires an argument -- 'S'\nTry 'gzip --help' for more information.\n"},
		{"--suffix without an argument", []string{"--suffix"}, "gzip: option '--suffix' requires an argument\nTry 'gzip --help' for more information.\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, fsys := run(t, New, testFS(), nil, nil, test.args...)

			if res.exit != 1 {
				t.Errorf("exit = %d, want 1", res.exit)
			}
			if res.stderr != test.wantErr {
				t.Errorf("stderr = %q, want %q", res.stderr, test.wantErr)
			}
			wantData(t, fsys, map[string]string{"a.txt": "alpha\n"})
		})
	}
}

func TestGzipErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		fsys    fsx.MemFS
		wantErr string
	}{
		{
			name:    "a missing operand",
			args:    []string{"missing.txt"},
			fsys:    testFS(),
			wantErr: "missing.txt",
		},
		{
			name:    "a directory operand",
			args:    []string{"sub"},
			fsys:    testFS(),
			wantErr: "is a directory",
		},
		{
			name:    "a missing decompression operand",
			args:    []string{"-d", "missing.txt"},
			fsys:    fsx.MemFS{},
			wantErr: "file does not exist",
		},
		{
			name:    "data which isn't compressed",
			args:    []string{"-d", "a.txt"},
			fsys:    testFS(),
			wantErr: "unknown suffix",
		},
		{
			name:    "a .Z file isn't supported yet",
			args:    []string{"-d", "a.txt.Z"},
			fsys:    fsx.MemFS{"a.txt.Z": {Data: []byte{0x1f, 0x9d, 0x48, 0x00}, Mode: 0o644}},
			wantErr: "unsupported compression format",
		},
		{
			name:    "a name too long for the suffix",
			args:    []string{strings.Repeat("x", 253)},
			fsys:    fsx.MemFS{strings.Repeat("x", 253): {Data: []byte("alpha\n"), Mode: 0o644}},
			wantErr: "invalid name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, _ := run(t, New, test.fsys, nil, nil, test.args...)

			if res.exit != 1 {
				t.Errorf("exit = %d, want 1", res.exit)
			}
			if !strings.Contains(res.stderr, test.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", res.stderr, test.wantErr)
			}
		})
	}

	t.Run("a bad operand doesn't stop the others", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a.txt": testFS()["a.txt"],
			"b.gz":  {Data: []byte("no magic here"), Mode: 0o644},
		}

		res, fsys := run(t, New, fsys, nil, nil, "missing.txt", "a.txt", "b.gz")

		if res.exit != 1 {
			t.Errorf("exit = %d, want 1", res.exit)
		}
		if _, ok := fsys["a.txt.gz"]; !ok {
			t.Errorf("files = %v, want a.txt.gz", names(fsys))
		}
	})

	t.Run("restoring a bad file leaves it alone", func(t *testing.T) {
		fsys := fsx.MemFS{"a.gz": {Data: []byte("no magic here"), Mode: 0o644}}

		res, fsys := run(t, Gunzip, fsys, nil, nil, "a.gz")

		if res.exit != 1 {
			t.Errorf("exit = %d, want 1", res.exit)
		}
		wantData(t, fsys, map[string]string{"a.gz": "no magic here"}, "a")
	})
}

func TestGzipStdin(t *testing.T) {
	t.Run("no operands compresses the standard input", func(t *testing.T) {
		res, _ := run(t, New, testFS(), strings.NewReader("alpha\n"), nil)

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		if got := gunzipAll(t, res.stdout); got != "alpha\n" {
			t.Errorf("round-trip = %q, want alpha\\n", got)
		}
	})

	t.Run("a dash operand reads the standard input", func(t *testing.T) {
		res, fsys := run(t, New, testFS(), strings.NewReader("alpha\n"), nil, "-")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		if got := gunzipAll(t, res.stdout); got != "alpha\n" {
			t.Errorf("round-trip = %q, want alpha\\n", got)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"})
	})

	t.Run("the standard input is restored", func(t *testing.T) {
		stdin := strings.NewReader(gz(t, "", mtime, "alpha\n"))

		res, _ := run(t, Gunzip, testFS(), stdin, nil)

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		if res.stdout != "alpha\n" {
			t.Errorf("stdout = %q, want alpha\\n", res.stdout)
		}
	})

	t.Run("operands are read in sequence", func(t *testing.T) {
		stdin := strings.NewReader(gz(t, "", mtime, "beta\n"))

		res, _ := run(t, Zcat, testFS(), stdin, nil, "-")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		if res.stdout != "beta\n" {
			t.Errorf("stdout = %q, want beta\\n", res.stdout)
		}
	})
}

func TestGunzipSuffixes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{"gzip suffix", "a.txt.gz", "a.txt"},
		{"short gzip suffix", "a.txt.z", "a.txt"},
		{"compress suffix", "a.txt.Z", "a.txt"},
		{"tar archive", "a.tar.gz", "a.tar"},
		{"compressed tar", "a.tgz", "a.tar"},
		{"tar with a short suffix", "a.tar.z", "a.tar"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fsys := fsx.MemFS{test.in: gzFile(t, "alpha\n")}

			res, fsys := run(t, Gunzip, fsys, nil, nil, test.in)

			if res.exit != 0 {
				t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
			}
			wantData(t, fsys, map[string]string{test.out: "alpha\n"}, test.in)
		})
	}

	t.Run("the suffix is added to find the input", func(t *testing.T) {
		fsys := fsx.MemFS{"a.txt.gz": gzFile(t, "alpha\n")}

		res, fsys := run(t, Gunzip, fsys, nil, nil, "a.txt")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"}, "a.txt.gz")
	})

	t.Run("-N restores the stored name in the same directory", func(t *testing.T) {
		fsys := fsx.MemFS{"sub/a.gz": {Data: []byte(gz(t, "named.txt", mtime, "alpha\n")), Mode: 0o644}}

		res, fsys := run(t, Gunzip, fsys, nil, nil, "-N", "sub/a.gz")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"sub/named.txt": "alpha\n"}, "sub/a.gz")
	})

	t.Run("-N refuses a stored name equal to the compressed input", func(t *testing.T) {
		fsys := fsx.MemFS{"a.gz": {Data: []byte(gz(t, "a.gz", mtime, "alpha\n")), Mode: 0o644}}

		res, fsys := run(t, Gunzip, fsys, nil, nil, "-N", "a.gz")

		if res.exit != 1 {
			t.Errorf("exit = %d, want 1", res.exit)
		}
		wantData(t, fsys, map[string]string{"a.gz": string(fsys["a.gz"].Data)})
	})

	t.Run("-N refuses a path traversal name", func(t *testing.T) {
		fsys := fsx.MemFS{"sub/a.gz": {Data: []byte(gz(t, "../evil", mtime, "alpha\n")), Mode: 0o644}}

		res, fsys := run(t, Gunzip, fsys, nil, nil, "-N", "sub/a.gz")

		if res.exit != 1 {
			t.Errorf("exit = %d, want 1", res.exit)
		}
		wantData(t, fsys, map[string]string{"sub/a.gz": string(fsys["sub/a.gz"].Data)})
		if _, ok := fsys["evil"]; ok {
			t.Error("evil: unexpected file")
		}
	})

	t.Run("-N refuses names containing a newline", func(t *testing.T) {
		fsys := fsx.MemFS{"a.gz": {Data: []byte(gz(t, "bad\nname", mtime, "alpha\n")), Mode: 0o644}}

		res, fsys := run(t, Gunzip, fsys, nil, nil, "-N", "a.gz")

		if res.exit != 1 {
			t.Errorf("exit = %d, want 1", res.exit)
		}
		wantData(t, fsys, map[string]string{"a.gz": string(fsys["a.gz"].Data)})
	})
}

func TestZcat(t *testing.T) {
	t.Run("concatenates operands", func(t *testing.T) {
		fsys := fsx.MemFS{"a.txt.gz": gzFile(t, "alpha\n"), "b.txt.gz": gzFile(t, "beta\n")}

		res, fsys := run(t, Zcat, fsys, nil, nil, "a.txt.gz", "b.txt.gz")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		if res.stdout != "alpha\nbeta\n" {
			t.Errorf("stdout = %q, want alpha\\nbeta\\n", res.stdout)
		}
		if len(fsys) != 2 {
			t.Errorf("files = %v, want both inputs kept", names(fsys))
		}
	})

	t.Run("gzcat does the same", func(t *testing.T) {
		res, _ := run(t, Gzcat, fsx.MemFS{"a.txt.gz": gzFile(t, "alpha\n")}, nil, nil, "a.txt.gz")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		if res.stdout != "alpha\n" {
			t.Errorf("stdout = %q, want alpha\\n", res.stdout)
		}
	})

	t.Run("members are read in sequence", func(t *testing.T) {
		multi := gz(t, "", mtime, "alpha\n") + gz(t, "", mtime, "more\n")
		fsys := fsx.MemFS{"a.gz": {Data: []byte(multi), Mode: 0o644}}

		res, _ := run(t, Zcat, fsys, nil, nil, "a.gz")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		if res.stdout != "alpha\nmore\n" {
			t.Errorf("stdout = %q, want alpha\\nmore\\n", res.stdout)
		}
	})

	t.Run("-f -c passes a plain operand through", func(t *testing.T) {
		fsys := fsx.MemFS{"plain": {Data: []byte("alpha\n"), Mode: 0o644}}
		res, _ := run(t, Zcat, fsys, nil, nil, "-f", "plain")
		if res.exit != 0 || res.stdout != "alpha\n" {
			t.Errorf("exit = %d, stdout = %q, stderr = %q", res.exit, res.stdout, res.stderr)
		}
	})

	t.Run("-f -c reports an operand which doesn't exist", func(t *testing.T) {
		res, _ := run(t, Zcat, fsx.MemFS{}, nil, nil, "-f", "missing.txt")
		if res.exit != 1 || !strings.Contains(res.stderr, "file does not exist") {
			t.Errorf("exit = %d, stderr = %q, want a missing-file error", res.exit, res.stderr)
		}
	})

	t.Run("-f -c passes through an operand that has a non-regular type", func(t *testing.T) {
		fsys := fsx.MemFS{
			"plain": {Data: []byte("plain"), Mode: 0o644},
			"link":  {Data: []byte("plain"), Mode: fs.ModeSymlink | 0o777},
		}
		res, _ := run(t, Zcat, fsys, nil, nil, "-f", "link")
		if res.exit != 0 || res.stdout != "plain" {
			t.Errorf("exit = %d, stdout = %q, stderr = %q", res.exit, res.stdout, res.stderr)
		}
	})

	t.Run("data which isn't compressed fails without -f", func(t *testing.T) {
		res, _ := run(t, Zcat, testFS(), nil, nil, "a.txt")

		if res.exit != 1 {
			t.Errorf("exit = %d, want 1", res.exit)
		}
		if !strings.Contains(res.stderr, "format not recognized") {
			t.Errorf("stderr = %q, want it to mention the format", res.stderr)
		}
	})
}

func TestOverwrite(t *testing.T) {
	t.Run("an existing output is not overwritten", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a.txt":    {Data: []byte("alpha\n"), Mode: 0o644},
			"a.txt.gz": {Data: []byte("keep me"), Mode: 0o644},
		}

		res, fsys := run(t, New, fsys, nil, nil, "a.txt")

		if res.exit != 1 {
			t.Errorf("exit = %d, want 1", res.exit)
		}
		if !strings.Contains(res.stderr, "file exists") {
			t.Errorf("stderr = %q, want it to mention the existing file", res.stderr)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n", "a.txt.gz": "keep me"})
	})

	t.Run("-f overwrites it", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a.txt":    {Data: []byte("alpha\n"), Mode: 0o644},
			"a.txt.gz": {Data: []byte("keep me"), Mode: 0o644},
		}

		res, fsys := run(t, New, fsys, nil, nil, "-f", "a.txt")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, nil, "a.txt")
		if gunzipAll(t, string(fsys["a.txt.gz"].Data)) != "alpha\n" {
			t.Error("a.txt.gz: want the compressed input")
		}
	})

	t.Run("gunzip -f overwrites the restored file", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a.txt":    {Data: []byte("old\n"), Mode: 0o644},
			"a.txt.gz": gzFile(t, "alpha\n"),
		}

		res, fsys := run(t, Gunzip, fsys, nil, nil, "-f", "a.txt.gz")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"}, "a.txt.gz")
	})

	t.Run("gunzip without -f leaves both files alone", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a.txt":    {Data: []byte("old\n"), Mode: 0o644},
			"a.txt.gz": gzFile(t, "alpha\n"),
		}

		res, fsys := run(t, Gunzip, fsys, nil, nil, "a.txt.gz")

		if res.exit != 1 {
			t.Errorf("exit = %d, want 1", res.exit)
		}
		wantData(t, fsys, map[string]string{"a.txt": "old\n"})
	})
}

func TestGzipTest(t *testing.T) {
	good := gz(t, "", mtime, "alpha\n")
	tests := []struct {
		name string
		data string
		exit int
	}{
		{"intact", good, 0},
		{"truncated", good[:12], 1},
		{"corrupt trailer", good[:len(good)-3], 1},
		{"not compressed", "alpha\n", 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fsys := fsx.MemFS{"a.gz": {Data: []byte(test.data), Mode: 0o644}}

			res, fsys := run(t, New, fsys, nil, nil, "-t", "a.gz")

			if res.exit != test.exit {
				t.Errorf("exit = %d, want %d (%s)", res.exit, test.exit, res.stderr)
			}
			if res.stdout != "" {
				t.Errorf("stdout = %q, want none", res.stdout)
			}
			wantData(t, fsys, map[string]string{"a.gz": test.data})
		})
	}

	t.Run("-tv reports the file", func(t *testing.T) {
		fsys := fsx.MemFS{"a.gz": gzFile(t, "alpha\n")}

		res, _ := run(t, New, fsys, nil, nil, "-tv", "a.gz")

		if !strings.Contains(res.stderr, "a.gz: OK") {
			t.Errorf("stderr = %q, want it to report the file", res.stderr)
		}
	})
}

func TestGzipSymlinks(t *testing.T) {
	t.Run("compression refuses a symbolic link without -f", func(t *testing.T) {
		fsys := fsx.MemFS{
			"target.txt": {Data: []byte("alpha\n"), Mode: 0o644},
			"link.txt":   {Data: []byte("target.txt"), Mode: fs.ModeSymlink | 0o777},
		}

		res, fsys := run(t, New, fsys, nil, nil, "link.txt")

		if res.exit != 1 {
			t.Errorf("exit = %d, want 1 (%s)", res.exit, res.stderr)
		}
		if _, ok := fsys["link.txt"]; !ok {
			t.Error("link.txt: want the link kept")
		}
		if _, ok := fsys["link.txt.gz"]; ok {
			t.Error("link.txt.gz: unexpected compressed output")
		}
	})

	t.Run("compression follows a symbolic link with -f", func(t *testing.T) {
		fsys := fsx.MemFS{
			"target.txt": {Data: []byte("alpha\n"), Mode: 0o644},
			"link.txt":   {Data: []byte("target.txt"), Mode: fs.ModeSymlink | 0o777},
		}

		res, fsys := run(t, New, fsys, nil, nil, "-f", "link.txt")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		if _, ok := fsys["link.txt"]; ok {
			t.Error("link.txt: want the link replaced by its target")
		}
		if _, ok := fsys["link.txt.gz"]; !ok {
			t.Error("link.txt.gz: want the compressed target")
		}
	})

	t.Run("restoring a symbolic link fails without -f", func(t *testing.T) {
		fsys := fsx.MemFS{
			"a.txt.gz": gzFile(t, "alpha\n"),
			"link.gz":  {Data: []byte("a.txt.gz"), Mode: fs.ModeSymlink | 0o777},
		}

		res, _ := run(t, Gunzip, fsys, nil, nil, "link.gz")

		if res.exit != 1 || !strings.Contains(res.stderr, "not a regular file") {
			t.Errorf("exit = %d, stderr = %q, want a refusal", res.exit, res.stderr)
		}
	})
}

func TestGzipRecursive(t *testing.T) {
	t.Run("-r walks into directories", func(t *testing.T) {
		fsys := fsx.MemFS{
			"sub/c.txt":    {Data: []byte("gamma\n"), Mode: 0o644},
			"sub/deep.txt": {Data: []byte("deep\n"), Mode: 0o644},
		}

		res, fsys := run(t, New, fsys, nil, nil, "-r", "sub")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, nil, "sub/c.txt", "sub/deep.txt")
		for _, name := range []string{"sub/c.txt.gz", "sub/deep.txt.gz"} {
			if _, ok := fsys[name]; !ok {
				t.Errorf("files = %v, want %s", names(fsys), name)
			}
		}
	})

	t.Run("-r skips symbolic links", func(t *testing.T) {
		fsys := fsx.MemFS{
			"sub/c.txt": {Data: []byte("gamma\n"), Mode: 0o644},
			"sub/link":  {Data: []byte("c.txt"), Mode: fs.ModeSymlink | 0o777},
		}

		res, fsys := run(t, New, fsys, nil, nil, "-r", "sub")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		if _, ok := fsys["sub/link"]; !ok {
			t.Error("sub/link: want the link to be left alone")
		}
	})

	t.Run("-rc recurses into directories", func(t *testing.T) {
		fsys := fsx.MemFS{"sub/a.gz": gzFile(t, "alpha\n")}
		res, _ := run(t, Gunzip, fsys, nil, nil, "-rc", "sub")
		if res.exit != 0 || res.stdout != "alpha\n" {
			t.Errorf("exit = %d, stdout = %q, stderr = %q", res.exit, res.stdout, res.stderr)
		}
	})

	t.Run("-rcf recurses into directories", func(t *testing.T) {
		fsys := fsx.MemFS{"sub/a.gz": gzFile(t, "alpha\n")}
		res, _ := run(t, Gunzip, fsys, nil, nil, "-rcf", "sub")
		if res.exit != 0 || res.stdout != "alpha\n" {
			t.Errorf("exit = %d, stdout = %q, stderr = %q", res.exit, res.stdout, res.stderr)
		}
	})

	t.Run("-rt tests files in directories", func(t *testing.T) {
		fsys := fsx.MemFS{"sub/a.gz": gzFile(t, "alpha\n")}
		res, _ := run(t, Gunzip, fsys, nil, nil, "-rt", "sub")
		if res.exit != 0 || res.stdout != "" {
			t.Errorf("exit = %d, stdout = %q, stderr = %q", res.exit, res.stdout, res.stderr)
		}
	})

	t.Run("-r restores a whole directory", func(t *testing.T) {
		fsys := fsx.MemFS{"sub/c.txt.gz": gzFile(t, "gamma\n")}

		res, fsys := run(t, Gunzip, fsys, nil, nil, "-r", "sub")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		wantData(t, fsys, map[string]string{"sub/c.txt": "gamma\n"}, "sub/c.txt.gz")
	})
}

func TestGzipInfo(t *testing.T) {
	t.Run("-h prints the usage", func(t *testing.T) {
		res, _ := run(t, New, testFS(), nil, nil, "-h")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
		if !strings.HasPrefix(res.stdout, "Usage: gzip ") {
			t.Errorf("stdout = %q, want it to start with the usage", res.stdout)
		}
	})

	t.Run("gunzip names itself in the usage", func(t *testing.T) {
		res, _ := run(t, Gunzip, testFS(), nil, nil, "--help")

		if !strings.HasPrefix(res.stdout, "Usage: gunzip ") {
			t.Errorf("stdout = %q, want it to start with the usage", res.stdout)
		}
	})

	t.Run("-V prints the version", func(t *testing.T) {
		res, _ := run(t, Zcat, testFS(), nil, nil, "-V")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
		if !strings.HasPrefix(res.stdout, "zcat (charm.land/snail/coreutils)") {
			t.Errorf("stdout = %q, want it to name the program", res.stdout)
		}
	})

	t.Run("informational options stop before the operands", func(t *testing.T) {
		res, fsys := run(t, New, testFS(), nil, nil, "--version", "a.txt")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0", res.exit)
		}
		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"})
	})
}

func TestHelpVersion(t *testing.T) {
	for _, test := range []struct {
		name string
		new  coreutils.NewFunc
	}{
		{"gzip", New},
		{"gunzip", Gunzip},
		{"zcat", Zcat},
		{"gzcat", Gzcat},
	} {
		t.Run(test.name+" --help", func(t *testing.T) {
			res, _ := run(t, test.new, testFS(), nil, nil, "--help")

			if res.exit != 0 {
				t.Fatalf("exit = %d, want 0 (%s)", res.exit, res.stderr)
			}
			if got := res.stdout; got != Usage(test.name) {
				t.Errorf("stdout = %q, want %q", got, Usage(test.name))
			}
		})

		t.Run(test.name+" --version", func(t *testing.T) {
			res, _ := run(t, test.new, testFS(), nil, nil, "--version")

			if res.exit != 0 {
				t.Fatalf("exit = %d, want 0 (%s)", res.exit, res.stderr)
			}
			want := test.name + " (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
			if got := res.stdout; got != want {
				t.Errorf("stdout = %q, want %q", got, want)
			}
		})

		t.Run(test.name+" -V", func(t *testing.T) {
			res, _ := run(t, test.new, testFS(), nil, nil, "-V")

			if res.exit != 0 {
				t.Fatalf("exit = %d, want 0 (%s)", res.exit, res.stderr)
			}
			want := test.name + " (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
			if got := res.stdout; got != want {
				t.Errorf("stdout = %q, want %q", got, want)
			}
		})
	}

	t.Run("gzip -h", func(t *testing.T) {
		res, _ := run(t, New, testFS(), nil, nil, "-h")

		if res.exit != 0 {
			t.Fatalf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		if got := res.stdout; got != Usage("gzip") {
			t.Errorf("stdout = %q, want %q", got, Usage("gzip"))
		}
	})
}

func TestGzipEnv(t *testing.T) {
	t.Run("GZIP holds options", func(t *testing.T) {
		fsys := fsx.MemFS{"a.txt.gz": gzFile(t, "alpha\n")}

		res, _ := run(t, Gunzip, fsys, nil, map[string]string{"GZIP": "-c"}, "a.txt.gz")

		if res.exit != 0 {
			t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
		}
		if res.stdout != "alpha\n" {
			t.Errorf("stdout = %q, want alpha\\n", res.stdout)
		}
	})

	t.Run("options on the command line come last", func(t *testing.T) {
		_, fsys := run(t, New, testFS(), nil, map[string]string{"GZIP": "-k"}, "a.txt")

		wantData(t, fsys, map[string]string{"a.txt": "alpha\n"})
	})

	t.Run("an invalid option in GZIP fails", func(t *testing.T) {
		res, _ := run(t, New, testFS(), nil, map[string]string{"GZIP": "-Z"}, "a.txt")
		if res.exit != 1 {
			t.Errorf("exit = %d, want 1", res.exit)
		}
	})

	t.Run("an invalid long option reports a diagnostic", func(t *testing.T) {
		res, _ := run(t, New, testFS(), nil, nil, "-q", "--bogus")
		if res.exit != 1 || res.stderr == "" {
			t.Errorf("exit = %d, stderr = %q, want error and diagnostic", res.exit, res.stderr)
		}
	})
}

func TestGzipContext(t *testing.T) {
	var stdout bytes.Buffer
	ctx, cancel := context.WithCancel(t.Context())
	rt := &coreutils.Runtime{
		Stdin:   &testutil.CancelReader{Cancel: cancel},
		Stdout:  &stdout,
		Stderr:  io.Discard,
		FS:      testFS(),
		Environ: func() []string { return nil },
	}

	if err := New(rt).Run(ctx, "-"); !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestGzipWindowsPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("operands with backslashes are only normalized on Windows")
	}

	fsys := fsx.MemFS{"sub/c.txt": {Data: []byte("gamma\n"), Mode: 0o644}}

	res, fsys := run(t, New, fsys, nil, nil, `sub\c.txt`)

	if res.exit != 0 {
		t.Errorf("exit = %d, want 0 (%s)", res.exit, res.stderr)
	}
	if _, ok := fsys["sub/c.txt.gz"]; !ok {
		t.Errorf("files = %v, want sub/c.txt.gz", names(fsys))
	}
}

// readHeader reads the header of the gzip file at name.
func readHeader(t *testing.T, fsys fsx.MemFS, name string) gzip.Header {
	t.Helper()

	file, ok := fsys[name]
	if !ok {
		t.Fatalf("%s: missing file", name)
	}
	reader, err := gzip.NewReader(bytes.NewReader(file.Data))
	if err != nil {
		t.Fatalf("%s: gzip.NewReader() error = %v", name, err)
	}
	return reader.Header
}

// gunzipAll returns the contents of a gzip string.
func gunzipAll(t *testing.T, data string) string {
	t.Helper()

	reader, err := gzip.NewReader(strings.NewReader(data))
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	defer reader.Close()

	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(out)
}
