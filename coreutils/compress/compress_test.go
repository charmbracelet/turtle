package compress

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/gzip"
	"charm.land/snail/fsx"
)

// testFS holds the fixtures used by the tests.
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt": {Data: []byte(strings.Repeat("alpha ", 30)), Mode: 0o644},
		"b.txt": {Data: []byte("beta\n"), Mode: 0o644},
	}
}

// run executes the program built by new against the given arguments.
func run(t *testing.T, new coreutils.NewFunc, fsys fsx.MemFS, args ...string) (int, string, string, fsx.MemFS) {
	t.Helper()

	var stdout, stderr strings.Builder
	rt := &coreutils.Runtime{
		Stdin:     strings.NewReader(""),
		Stdout:    &stdout,
		Stderr:    &stderr,
		FS:        fsys,
		LookupEnv: func(string) (string, bool) { return "", false },
	}
	err := new(rt).Run(t.Context(), args...)
	if err == nil {
		return 0, stdout.String(), stderr.String(), fsys
	}
	var exitErr *coreutils.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
	}
	return exitErr.Code, stdout.String(), stderr.String(), fsys
}

func TestCompressGzipMode(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"-g", []string{"-g", "a.txt"}},
		{"-m gzip", []string{"-m", "gzip", "a.txt"}},
		{"-m deflate", []string{"-m", "deflate", "a.txt"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, _, stderr, fsys := run(t, New, testFS(), test.args...)

			if code != 0 {
				t.Fatalf("exit = %d, want 0 (%s)", code, stderr)
			}
			if _, ok := fsys["a.txt.gz"]; !ok {
				t.Errorf("files = %v, want a.txt.gz", fsys)
			}
			if _, ok := fsys["a.txt"]; ok {
				t.Error("a.txt: want the input removed")
			}
		})
	}

	t.Run("-c writes gzip data and leaves the input", func(t *testing.T) {
		code, stdout, stderr, fsys := run(t, New, testFS(), "-c", "-g", "a.txt")

		if code != 0 {
			t.Fatalf("exit = %d, want 0 (%s)", code, stderr)
		}
		if stdout == "" {
			t.Error("stdout: want gzip data")
		}
		if _, ok := fsys["a.txt"]; !ok {
			t.Error("a.txt: want the input kept")
		}
	})
}

func TestCompressLZWDeferred(t *testing.T) {
	tests := []struct {
		name string
		new  coreutils.NewFunc
		args []string
	}{
		{"default compress", New, []string{"a.txt"}},
		{"explicit lzw", New, []string{"-m", "lzw", "a.txt"}},
		{"uncompress .Z", Uncompress, []string{"a.txt.Z"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fsys := testFS()
			if test.name == "uncompress .Z" {
				fsys = fsx.MemFS{"a.txt.Z": {Data: []byte{0x1f, 0x9d, 0x48, 0x00}, Mode: 0o644}}
			}
			code, _, stderr, _ := run(t, test.new, fsys, test.args...)

			want := 3
			if test.name == "uncompress .Z" {
				want = 1
			}
			if code != want {
				t.Errorf("exit = %d, want %d (%s)", code, want, stderr)
			}
			if !strings.Contains(stderr, "compression algorithm not supported") && test.name != "uncompress .Z" {
				t.Errorf("stderr = %q, want unsupported algorithm", stderr)
			}
			if test.name == "uncompress .Z" && !strings.Contains(stderr, "unsupported compression format") {
				t.Errorf("stderr = %q, want unsupported format", stderr)
			}
		})
	}

	t.Run("-b validates the LZW code width", func(t *testing.T) {
		for _, value := range []string{"8", "17", "not-a-number"} {
			t.Run(value, func(t *testing.T) {
				code, _, stderr, _ := run(t, New, testFS(), "-b", value, "a.txt")
				if code != 1 {
					t.Errorf("exit = %d, want 1", code)
				}
				if !strings.Contains(stderr, "number of bits") {
					t.Errorf("stderr = %q, want invalid bit count", stderr)
				}
			})
		}
	})
}

func TestUncompressGzip(t *testing.T) {
	t.Run("restores a gzip file with either suffix", func(t *testing.T) {
		for _, name := range []string{"a.txt.gz", "a.txt.Z"} {
			t.Run(name, func(t *testing.T) {
				fsys := fsx.MemFS{name: {Data: gzipData(t, strings.Repeat("alpha ", 30)), Mode: 0o644}}

				code, _, stderr, fsys := run(t, Uncompress, fsys, name)

				if code != 0 {
					t.Errorf("exit = %d, want 0 (%s)", code, stderr)
				}
				if file := fsys["a.txt"]; file == nil {
					t.Fatal("a.txt: missing file")
				} else if got, want := string(file.Data), strings.Repeat("alpha ", 30); got != want {
					t.Errorf("a.txt = %q, want original", got)
				}
				if _, ok := fsys[name]; ok {
					t.Errorf("%s: want input removed", name)
				}
			})
		}
	})

	t.Run("-c restores to stdout and keeps the input", func(t *testing.T) {
		fsys := fsx.MemFS{"a.txt.gz": {Data: []byte("plain"), Mode: 0o644}}

		code, stdout, stderr, fsys := run(t, Uncompress, fsys, "-c", "a.txt.gz")

		if code != 1 {
			t.Errorf("exit = %d, want 1 (%s)", code, stderr)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want none", stdout)
		}
		if _, ok := fsys["a.txt.gz"]; !ok {
			t.Error("a.txt.gz: want input preserved")
		}
	})
}

func TestCompressOptions(t *testing.T) {
	t.Run("POSIX -c, -d, -f and -v are accepted", func(t *testing.T) {
		_, _, _, fsys := run(t, New, testFS(), "-c", "-g", "-f", "-v", "a.txt")
		if _, ok := fsys["a.txt"]; !ok {
			t.Error("a.txt: -c should leave it in place")
		}
	})

	t.Run("-d is equivalent to uncompress", func(t *testing.T) {
		fsys := fsx.MemFS{"a.txt.gz": {Data: gzipData(t, strings.Repeat("alpha ", 30)), Mode: 0o644}}
		code, _, stderr, fsys := run(t, New, fsys, "-d", "a.txt.gz")

		if code != 0 {
			t.Errorf("exit = %d, want 0 (%s)", code, stderr)
		}
		if got := string(fsys["a.txt"].Data); got != strings.Repeat("alpha ", 30) {
			t.Errorf("a.txt = %q, want original", got)
		}
	})

	t.Run("-v reports compression", func(t *testing.T) {
		code, _, stderr, _ := run(t, New, testFS(), "-g", "-v", "a.txt")

		if code != 0 {
			t.Errorf("exit = %d, want 0 (%s)", code, stderr)
		}
		if !strings.Contains(stderr, "a.txt") || !strings.Contains(stderr, "%") {
			t.Errorf("stderr = %q, want a report", stderr)
		}
	})

	t.Run("the last algorithm option wins", func(t *testing.T) {
		code, _, _, fsys := run(t, New, testFS(), "-m", "lzw", "-g", "a.txt")

		if code != 0 {
			t.Errorf("exit = %d, want 0", code)
		}
		if _, ok := fsys["a.txt.gz"]; !ok {
			t.Error("a.txt.gz: want gzip output")
		}
	})

	t.Run("a later -m lzw wins", func(t *testing.T) {
		code, _, _, _ := run(t, New, testFS(), "-g", "-m", "lzw", "a.txt")
		if code != 3 {
			t.Errorf("exit = %d, want 3", code)
		}
	})

	t.Run("-b selects a DEFLATE compression level", func(t *testing.T) {
		code, _, stderr, fsys := run(t, New, testFS(), "-g", "-b", "9", "a.txt")
		if code != 0 {
			t.Errorf("exit = %d, want 0 (%s)", code, stderr)
		}
		if _, ok := fsys["a.txt.gz"]; !ok {
			t.Error("a.txt.gz: want gzip output")
		}
	})

	t.Run("an unsupported algorithm is rejected", func(t *testing.T) {
		code, _, stderr, _ := run(t, New, testFS(), "-m", "bzip2", "a.txt")
		if code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
		if !strings.Contains(stderr, "unknown compression algorithm") {
			t.Errorf("stderr = %q, want unknown algorithm", stderr)
		}
	})
}

func TestCompressStdin(t *testing.T) {
	var stdout, stderr strings.Builder
	rt := &coreutils.Runtime{
		Stdin:     strings.NewReader(strings.Repeat("alpha ", 30)),
		Stdout:    &stdout,
		Stderr:    &stderr,
		FS:        testFS(),
		LookupEnv: func(string) (string, bool) { return "", false },
	}
	err := New(rt).Run(t.Context(), "-g")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdout.Len() == 0 {
		t.Error("stdout: want compressed data")
	}

	rt.Stdin = strings.NewReader("plain")
	stdout.Reset()
	if err := Uncompress(rt).Run(t.Context(), "-c"); err != nil {
		var exitErr *coreutils.ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 1 {
			t.Errorf("Run() error = %v, want exit status 1", err)
		}
	} else if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want none", stdout.String())
	}
}

func TestCompressContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	rt := &coreutils.Runtime{
		Stdin:     strings.NewReader("data"),
		Stdout:    io.Discard,
		Stderr:    io.Discard,
		FS:        testFS(),
		LookupEnv: func(string) (string, bool) { return "", false },
	}
	if err := New(rt).Run(ctx, "-g", "-"); !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func gzipData(t *testing.T, data string) []byte {
	t.Helper()
	var stdout strings.Builder
	rt := &coreutils.Runtime{
		Stdin:     strings.NewReader(data),
		Stdout:    &stdout,
		Stderr:    io.Discard,
		FS:        fsx.MemFS{},
		LookupEnv: func(string) (string, bool) { return "", false },
	}
	if err := gzip.New(rt).Run(t.Context(), "-c"); err != nil {
		t.Fatalf("gzip.Run() error = %v", err)
	}
	return []byte(stdout.String())
}
