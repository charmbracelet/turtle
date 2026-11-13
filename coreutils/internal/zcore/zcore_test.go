package zcore

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

func TestSniff(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want Algorithm
	}{
		{"gzip", []byte{0x1f, 0x8b}, Gzip},
		{"compress", []byte{0x1f, 0x9d}, LZW},
		{"plain", []byte("plain"), none},
		{"empty", nil, none},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sniff(bufio.NewReader(bytes.NewReader(test.data))); got != test.want {
				t.Errorf("sniff() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSourceDecode(t *testing.T) {
	t.Run("restores a gzip member", func(t *testing.T) {
		var data bytes.Buffer
		writer := gzip.NewWriter(&data)
		if _, err := io.WriteString(writer, "alpha\n"); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		var output strings.Builder
		n, err := openSource(bytes.NewReader(data.Bytes())).decode(t.Context(), &output, false)
		if err != nil {
			t.Fatalf("decode() error = %v", err)
		}
		if output.String() != "alpha\n" {
			t.Errorf("output = %q, want alpha\\n", output.String())
		}
		if n != int64(len("alpha\n")) {
			t.Errorf("bytes = %d, want %d", n, len("alpha\n"))
		}
	})

	t.Run("refuses plain data without passthrough", func(t *testing.T) {
		if _, err := openSource(strings.NewReader("plain")).decode(t.Context(), io.Discard, false); err != errNotCompressed {
			t.Errorf("decode() error = %v, want %v", err, errNotCompressed)
		}
	})

	t.Run("passes plain data through when allowed", func(t *testing.T) {
		var output strings.Builder
		if _, err := openSource(strings.NewReader("plain")).decode(t.Context(), &output, true); err != nil {
			t.Fatalf("decode() error = %v", err)
		}
		if output.String() != "plain" {
			t.Errorf("output = %q, want plain", output.String())
		}
	})

	t.Run("recognizes .Z but doesn't decode it yet", func(t *testing.T) {
		if _, err := openSource(bytes.NewReader([]byte{0x1f, 0x9d})).decode(t.Context(), io.Discard, false); err != errUnsupported {
			t.Errorf("decode() error = %v, want %v", err, errUnsupported)
		}
	})

	t.Run("reads a cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := openSource(strings.NewReader("plain")).decode(ctx, io.Discard, true); err != context.Canceled {
			t.Errorf("decode() error = %v, want context.Canceled", err)
		}
	})
}

func TestSuffix(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{"default gzip", Options{Algorithm: Gzip}, ".gz"},
		{"default LZW", Options{Algorithm: LZW}, ".Z"},
		{"override", Options{Algorithm: Gzip, Suffix: ".zip"}, ".zip"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.opts.suffix(); got != test.want {
				t.Errorf("suffix() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestStripSuffix(t *testing.T) {
	tests := []struct {
		name  string
		opts  Options
		input string
		want  string
		ok    bool
	}{
		{"gzip", Options{Algorithm: Gzip}, "file.txt.gz", "file.txt", true},
		{"short gzip", Options{Algorithm: Gzip}, "file.txt.z", "file.txt", true},
		{"compress", Options{Algorithm: Gzip}, "file.txt.Z", "file.txt", true},
		{"tar gzip", Options{Algorithm: Gzip}, "archive.tgz", "archive.tar", true},
		{"tar deflate", Options{Algorithm: Gzip}, "archive.tz", "archive.tar", true},
		{"custom suffix", Options{Algorithm: Gzip, Suffix: ".zip"}, "file.zip", "file", true},
		{"plain", Options{Algorithm: Gzip}, "file.txt", "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := stripSuffix(test.input, test.opts)
			if got != test.want || ok != test.ok {
				t.Errorf("stripSuffix() = %q, %v, want %q, %v", got, ok, test.want, test.ok)
			}
		})
	}
}

func TestValidStoredName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"ordinary name", "report.txt", true},
		{"traversal", "../evil", false},
		{"dot", ".", false},
		{"dot dot", "..", false},
		{"slash", "dir/file", false},
		{"backslash", `dir\file`, false},
		{"newline", "bad\nname", false},
		{"NUL", "bad\x00name", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validStoredName(test.input); got != test.want {
				t.Errorf("validStoredName(%q) = %v, want %v", test.input, got, test.want)
			}
		})
	}
}

func TestRatio(t *testing.T) {
	tests := []struct {
		compressed int64
		original   int64
		want       string
	}{
		{50, 100, "50.00%"},
		{150, 100, "-50.00%"},
		{1, 0, "0.00%"},
	}

	for _, test := range tests {
		if got := ratio(test.compressed, test.original); got != test.want {
			t.Errorf("ratio(%d, %d) = %q, want %q", test.compressed, test.original, got, test.want)
		}
	}
}

func TestCheckName(t *testing.T) {
	if err := checkName("dir/" + strings.Repeat("x", nameMax)); err != nil {
		t.Errorf("checkName() error = %v, want nil", err)
	}
	if err := checkName("dir/" + strings.Repeat("x", nameMax+1)); err == nil {
		t.Error("checkName() error = nil, want error")
	}
}

// removeErrFS fails the removal of one name, so that the cleanup of the file it
// produced can still be observed.
type removeErrFS struct {
	fsx.MemFS
	blocked string
}

// Remove implements fsx.RemoveFS.
func (f removeErrFS) Remove(name string) error {
	if path.Base(name) == f.blocked {
		return errors.New("permission denied")
	}
	return f.MemFS.Remove(name)
}

// gzData builds the contents of a gzip file holding data.
func gzData(t *testing.T, data string) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := io.WriteString(writer, data); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}

func TestRemoveInputFailure(t *testing.T) {
	tests := []struct {
		name     string
		opts     Options
		fsys     fsx.MemFS
		operand  string
		blocked  string
		out      string
		wantExit int
		wantOut  bool
	}{
		{
			name:     "compression discards its output",
			opts:     Options{Prog: "gzip", Algorithm: Gzip, Level: -1},
			fsys:     fsx.MemFS{"a.txt": {Data: []byte(strings.Repeat("alpha ", 30)), Mode: 0o644}},
			operand:  "a.txt",
			blocked:  "a.txt",
			out:      "a.txt.gz",
			wantExit: 1,
			wantOut:  false,
		},
		{
			name:     "forced compression keeps its output",
			opts:     Options{Prog: "gzip", Algorithm: Gzip, Level: -1, Force: true},
			fsys:     fsx.MemFS{"a.txt": {Data: []byte(strings.Repeat("alpha ", 30)), Mode: 0o644}},
			operand:  "a.txt",
			blocked:  "a.txt",
			out:      "a.txt.gz",
			wantExit: 2,
			wantOut:  true,
		},
		{
			name:     "decompression discards its output",
			opts:     Options{Prog: "gunzip", Algorithm: Gzip, Level: -1, Decompress: true},
			fsys:     fsx.MemFS{"a.txt.gz": {Data: gzData(t, "alpha\n"), Mode: 0o644}},
			operand:  "a.txt.gz",
			blocked:  "a.txt.gz",
			out:      "a.txt",
			wantExit: 1,
			wantOut:  false,
		},
		{
			name:     "forced decompression keeps its output",
			opts:     Options{Prog: "gunzip", Algorithm: Gzip, Level: -1, Decompress: true, Force: true},
			fsys:     fsx.MemFS{"a.txt.gz": {Data: gzData(t, "alpha\n"), Mode: 0o644}},
			operand:  "a.txt.gz",
			blocked:  "a.txt.gz",
			out:      "a.txt",
			wantExit: 2,
			wantOut:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rt := &coreutils.Runtime{
				Stdin:   strings.NewReader(""),
				Stdout:  io.Discard,
				Stderr:  io.Discard,
				FS:      removeErrFS{MemFS: test.fsys, blocked: test.blocked},
				Environ: func() []string { return nil },
			}

			err := Run(t.Context(), rt, test.opts, []string{test.operand})
			var exitErr *coreutils.ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != test.wantExit {
				t.Fatalf("Run() error = %v, want exit %d", err, test.wantExit)
			}
			if _, ok := test.fsys[test.out]; ok != test.wantOut {
				t.Errorf("%s exists = %v, want %v", test.out, ok, test.wantOut)
			}
			if _, ok := test.fsys[test.operand]; !ok {
				t.Errorf("%s: want the input kept", test.operand)
			}
		})
	}
}

func TestStdoutWriteFailure(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		stdin   io.Reader
		fsys    fsx.MemFS
		operand string
	}{
		{
			name:  "compressing the standard input",
			opts:  Options{Prog: "gzip", Algorithm: Gzip, Level: -1},
			stdin: strings.NewReader("alpha\n"),
		},
		{
			name:  "restoring the standard input",
			opts:  Options{Prog: "gunzip", Algorithm: Gzip, Level: -1, Decompress: true, Stdout: true},
			stdin: bytes.NewReader(gzData(t, "alpha\n")),
		},
		{
			name:    "compressing a file",
			opts:    Options{Prog: "gzip", Algorithm: Gzip, Level: -1, Stdout: true},
			stdin:   strings.NewReader(""),
			fsys:    fsx.MemFS{"a.txt": {Data: []byte("alpha\n"), Mode: 0o644}},
			operand: "a.txt",
		},
		{
			name:    "restoring a file",
			opts:    Options{Prog: "zcat", Algorithm: Gzip, Level: -1, Decompress: true, Stdout: true},
			stdin:   strings.NewReader(""),
			fsys:    fsx.MemFS{"a.txt.gz": {Data: gzData(t, "alpha\n"), Mode: 0o644}},
			operand: "a.txt.gz",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.fsys == nil {
				test.fsys = fsx.MemFS{}
			}
			rt := &coreutils.Runtime{
				Stdin:   test.stdin,
				Stdout:  testutil.ErrorWriter{Err: errors.New("no space left on device")},
				Stderr:  io.Discard,
				FS:      test.fsys,
				Environ: func() []string { return nil },
			}

			var operands []string
			if test.operand != "" {
				operands = []string{test.operand}
			}
			err := Run(t.Context(), rt, test.opts, operands)
			var exitErr *coreutils.ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 1 {
				t.Fatalf("Run() error = %v, want exit 1", err)
			}
		})
	}
}

func TestMissingRuntimeValues(t *testing.T) {
	t.Run("no standard input", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdout:  io.Discard,
			Stderr:  io.Discard,
			FS:      fsx.MemFS{},
			Environ: func() []string { return nil },
		}
		err := Run(t.Context(), rt, Options{Prog: "gzip", Algorithm: Gzip, Level: -1}, nil)
		var exitErr *coreutils.ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 1 {
			t.Fatalf("Run() error = %v, want exit 1", err)
		}
	})

	t.Run("no file system", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:   strings.NewReader(""),
			Stdout:  io.Discard,
			Stderr:  io.Discard,
			Environ: func() []string { return nil },
		}
		err := Run(t.Context(), rt, Options{Prog: "gzip", Algorithm: Gzip, Level: -1}, []string{"a.txt"})
		var exitErr *coreutils.ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 1 {
			t.Fatalf("Run() error = %v, want exit 1", err)
		}
	})
}

func TestStoreNameHeaderFailure(t *testing.T) {
	fsys := fsx.MemFS{"a.gz": {Data: []byte{0x1f, 0x8b}, Mode: 0o644}}
	rt := &coreutils.Runtime{
		Stdin:   strings.NewReader(""),
		Stdout:  io.Discard,
		Stderr:  io.Discard,
		FS:      fsys,
		Environ: func() []string { return nil },
	}

	opts := Options{Prog: "gunzip", Algorithm: Gzip, Level: -1, Decompress: true, StoreName: true}
	err := Run(t.Context(), rt, opts, []string{"a.gz"})
	var exitErr *coreutils.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("Run() error = %v, want exit 1", err)
	}
	if _, ok := fsys["a"]; ok {
		t.Error("a: want no output file")
	}
}
