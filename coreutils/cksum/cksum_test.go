package cksum

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
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
	args     []string
	want     string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  test.stdin,
		Stdout: stdout,
		Stderr: stderr,
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
}

// files holds the fixtures used by the tests, with the checksums the
// algorithm of the specification gives for them.
var files = fsx.MemFS{
	"abc.txt":   {Data: []byte("abc")},
	"empty.txt": {Data: []byte("")},
	"hello.txt": {Data: []byte("hello")},
	"sub/1.txt": {Data: []byte("1\n")},
}

func TestRun(t *testing.T) {
	tests := []testCase{
		{
			name:  "the standard input without a name",
			stdin: strings.NewReader("abc"),
			want:  "1219131554 3\n",
		},
		{
			name:  "an empty input",
			stdin: strings.NewReader(""),
			want:  "4294967295 0\n",
		},
		{
			name:  "a binary input",
			stdin: strings.NewReader("\x00\xff\x80abc"),
			want:  "1630418514 6\n",
		},
		{
			name:  "an input across chunks",
			stdin: strings.NewReader(strings.Repeat("a", 40000)),
			want:  "330876385 40000\n",
		},
		{
			name:  "an explicit standard input keeps its name",
			stdin: strings.NewReader("abc"),
			args:  []string{"-"},
			want:  "1219131554 3 -\n",
		},
		{
			name: "a file",
			fsys: files,
			args: []string{"abc.txt"},
			want: "1219131554 3 abc.txt\n",
		},
		{
			name: "several files",
			fsys: files,
			args: []string{"abc.txt", "hello.txt"},
			want: "1219131554 3 abc.txt\n3287646509 5 hello.txt\n",
		},
		{
			name: "a file in a subdirectory",
			fsys: files,
			args: []string{"sub/1.txt"},
			want: "4219530715 2 sub/1.txt\n",
		},
		{
			name: "a file named like an option",
			fsys: files,
			args: []string{"--", "abc.txt"},
			want: "1219131554 3 abc.txt\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLegacyAlgorithms(t *testing.T) {
	tests := []testCase{
		{
			name: "crc is the algorithm of the default",
			fsys: files,
			args: []string{"-a", "crc", "abc.txt"},
			want: "1219131554 3 abc.txt\n",
		},
		{
			name: "sysv pads its checksum and blocks",
			fsys: files,
			args: []string{"-a", "sysv", "abc.txt"},
			want: "00294     1 abc.txt\n",
		},
		{
			name:  "sysv without a name",
			stdin: strings.NewReader("abc"),
			args:  []string{"-asysv"},
			want:  "00294     1\n",
		},
		{
			name:  "sysv rounds the blocks up",
			stdin: strings.NewReader(strings.Repeat("a", 513)),
			args:  []string{"-a", "sysv"},
			want:  "49761     2\n",
		},
		{
			name: "bsd pads its checksum and blocks",
			fsys: files,
			args: []string{"-a", "bsd", "abc.txt"},
			want: "16556     1 abc.txt\n",
		},
		{
			name:  "bsd rounds the blocks up",
			stdin: strings.NewReader(strings.Repeat("a", 1025)),
			args:  []string{"-a", "bsd"},
			want:  "00033     2\n",
		},
		{
			name: "crc32b prints the byte count",
			fsys: files,
			args: []string{"--algorithm=crc32b", "abc.txt"},
			want: "891568578 3 abc.txt\n",
		},
		{
			name:  "crc32b of an empty input",
			stdin: strings.NewReader(""),
			args:  []string{"-a", "crc32b"},
			want:  "0 0\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

// digests holds the digest of "abc" with each algorithm, along with the tag
// its tagged output shows.
var digests = []struct {
	algorithm string
	tag       string
	digest    string
}{
	{"md5", "MD5", "900150983cd24fb0d6963f7d28e17f72"},
	{"sha1", "SHA1", "a9993e364706816aba3e25717850c26c9cd0d89d"},
	{"sha224", "SHA224", "23097d223405d8228642a477bda255b32aadbce4bda0b3f7e36c9da7"},
	{"sha256", "SHA256", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
	{"sha384", "SHA384", "cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7"},
	{"sha512", "SHA512", "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"},
}

func TestRunDigestAlgorithms(t *testing.T) {
	for _, test := range digests {
		t.Run(test.algorithm, func(t *testing.T) {
			t.Run("tagged", func(t *testing.T) {
				runCase(t, testCase{
					fsys: files,
					args: []string{"-a", test.algorithm, "abc.txt"},
					want: test.tag + " (abc.txt) = " + test.digest + "\n",
				})
			})
			t.Run("untagged", func(t *testing.T) {
				runCase(t, testCase{
					fsys: files,
					args: []string{"-a", test.algorithm, "--untagged", "abc.txt"},
					want: test.digest + "  abc.txt\n",
				})
			})
		})
	}
}

func TestRunVariableAlgorithms(t *testing.T) {
	tests := []testCase{
		{
			name: "sha2 of 224 bits",
			fsys: files,
			args: []string{"-a", "sha2", "-l", "224", "abc.txt"},
			want: "SHA224 (abc.txt) = 23097d223405d8228642a477bda255b32aadbce4bda0b3f7e36c9da7\n",
		},
		{
			name: "sha2 of 512 bits",
			fsys: files,
			args: []string{"-a", "sha2", "--length=512", "abc.txt"},
			want: "SHA512 (abc.txt) = ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f\n",
		},
		{
			name: "sha3 of 224 bits",
			fsys: files,
			args: []string{"-a", "sha3", "-l", "224", "abc.txt"},
			want: "SHA3-224 (abc.txt) = e642824c3f8cf24ad09234ee7d3c766fc9a3a5168d0c94ad73b46fdf\n",
		},
		{
			name: "sha3 of 256 bits",
			fsys: files,
			args: []string{"-a", "sha3", "-l", "256", "abc.txt"},
			want: "SHA3-256 (abc.txt) = 3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532\n",
		},
		{
			name: "sha3 of 384 bits",
			fsys: files,
			args: []string{"-a", "sha3", "-l", "384", "abc.txt"},
			want: "SHA3-384 (abc.txt) = ec01498288516fc926459f58e2c6ad8df9b473cb0fc08c2596da7cf0e49be4b298d88cea927ac7f539f1edf228376d25\n",
		},
		{
			name: "sha3 of 512 bits",
			fsys: files,
			args: []string{"-a", "sha3", "-l", "512", "abc.txt"},
			want: "SHA3-512 (abc.txt) = b751850b1a57168a5693cd924b6b096e08f621827444f70d884f5d0240d2712e10e116e9192af3c91a7ec57647e3934057340b4cf408d5a56592f8274eec53f0\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDigestOptions(t *testing.T) {
	tests := []testCase{
		{
			name: "an explicit --tag",
			fsys: files,
			args: []string{"-a", "md5", "--tag", "abc.txt"},
			want: "MD5 (abc.txt) = 900150983cd24fb0d6963f7d28e17f72\n",
		},
		{
			name: "base64 in the tagged output",
			fsys: files,
			args: []string{"-a", "sha256", "--base64", "abc.txt"},
			want: "SHA256 (abc.txt) = ungWv48Bz+pBQUDeXa4iI7ADYaOWF3qctBD/YfIAFa0=\n",
		},
		{
			name: "base64 in the untagged output",
			fsys: files,
			args: []string{"-a", "md5", "--base64", "--untagged", "abc.txt"},
			want: "kAFQmDzST7DWlj99KOF/cg==  abc.txt\n",
		},
		{
			name: "a zero byte ends the lines with --zero",
			fsys: files,
			args: []string{"-a", "md5", "--zero", "abc.txt", "hello.txt"},
			want: "MD5 (abc.txt) = 900150983cd24fb0d6963f7d28e17f72\x00" +
				"MD5 (hello.txt) = 5d41402abc4b2a76b9719d911017c592\x00",
		},
		{
			name:  "the standard input keeps its dash",
			stdin: strings.NewReader("abc"),
			args:  []string{"-a", "md5"},
			want:  "MD5 (-) = 900150983cd24fb0d6963f7d28e17f72\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}

	t.Run("a name holding a line break is escaped", func(t *testing.T) {
		fsys := fsx.MemFS{"a\nb.txt": {Data: []byte("abc")}}
		runCase(t, testCase{
			fsys: fsys,
			args: []string{"-a", "md5", "a\nb.txt"},
			want: `\MD5 (a\nb.txt) = 900150983cd24fb0d6963f7d28e17f72` + "\n",
		})
		runCase(t, testCase{
			fsys: fsys,
			args: []string{"-a", "md5", "--zero", "a\nb.txt"},
			want: "MD5 (a\nb.txt) = 900150983cd24fb0d6963f7d28e17f72\x00",
		})
	})
}

func TestRunOptionErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "an unknown algorithm",
			fsys:     files,
			args:     []string{"-a", "bogus", "abc.txt"},
			wantErr:  "cksum: unrecognized algorithm 'bogus'\nTry 'cksum --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a length without an algorithm",
			fsys:     files,
			args:     []string{"-l", "256", "abc.txt"},
			wantErr:  "cksum: the --length option is only supported with the sha2, sha3 and blake2b algorithms\nTry 'cksum --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a length with a fixed digest",
			fsys:     files,
			args:     []string{"-a", "md5", "-l", "256", "abc.txt"},
			wantErr:  "cksum: the --length option is only supported with the sha2, sha3 and blake2b algorithms\nTry 'cksum --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a length which sha2 requires",
			fsys:     files,
			args:     []string{"-a", "sha2", "abc.txt"},
			wantErr:  "cksum: the --length option is required with the sha2 algorithm\nTry 'cksum --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a length which sha3 requires",
			fsys:     files,
			args:     []string{"-a", "sha3", "abc.txt"},
			wantErr:  "cksum: the --length option is required with the sha3 algorithm\nTry 'cksum --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a length the algorithm has no digest of",
			fsys:     files,
			args:     []string{"-a", "sha2", "-l", "300", "abc.txt"},
			wantErr:  "cksum: the --length option of the sha2 algorithm must be 224, 256, 384 or 512\nTry 'cksum --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "base64 of a legacy algorithm",
			fsys:     files,
			args:     []string{"--base64", "abc.txt"},
			wantErr:  "cksum: the --base64 option is not supported with the crc algorithm\nTry 'cksum --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "tag and untagged together",
			fsys:     files,
			args:     []string{"-a", "md5", "--tag", "--untagged", "abc.txt"},
			wantErr:  "cksum: the --tag and --untagged options are mutually exclusive\nTry 'cksum --help' for more information.\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "an invalid option",
			fsys:     files,
			args:     []string{"-x", "abc.txt"},
			wantErr:  "cksum: invalid option -- 'x'\nTry 'cksum --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a missing file",
			fsys:     files,
			args:     []string{"missing.txt"},
			wantErr:  "cksum: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "the files after a missing one are still written",
			fsys:     files,
			args:     []string{"missing.txt", "abc.txt"},
			want:     "1219131554 3 abc.txt\n",
			wantErr:  "cksum: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "no file system",
			args:     []string{"abc.txt"},
			wantErr:  "cksum: abc.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "a read error",
			stdin:    errorReader{err: errors.New("boom")},
			args:     []string{"-"},
			wantErr:  "cksum: -: boom\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunIOErrors(t *testing.T) {
	t.Run("a write error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader("abc"),
			Stdout: errorWriter{err: errors.New("boom")},
			Stderr: new(bytes.Buffer),
		}
		err := New(rt).Run(t.Context())
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
	})

	t.Run("a done context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		rt := &coreutils.Runtime{
			Stdin:  &cancelReader{cancel: cancel},
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
		}
		err := New(rt).Run(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})
}

// failFS opens files whose reads always fail, recording when they are
// closed.
type failFS struct {
	fsx.MemFS
	closed bool
}

type failFile struct {
	fs *failFS
}

func (f *failFile) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}

func (f *failFile) Stat() (fs.FileInfo, error) {
	return nil, errors.New("no file information")
}

func (f *failFile) Close() error {
	f.fs.closed = true
	return nil
}

func (f *failFS) Open(string) (fs.File, error) {
	return &failFile{fs: f}, nil
}

func TestChecksumClosesOnError(t *testing.T) {
	t.Run("a read error", func(t *testing.T) {
		fsys := &failFS{}
		rt := &coreutils.Runtime{
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
			FS:     fsys,
		}
		p := &Prog{Runtime: rt}
		_, err := p.checksum(t.Context(), algorithms["crc"], options{}, "file", true)
		if err == nil {
			t.Fatal("checksum() error = nil, want an error")
		}
		if !fsys.closed {
			t.Error("checksum() didn't close the file")
		}
	})

	t.Run("a done context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		fsys := &failFS{}
		rt := &coreutils.Runtime{
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
			FS:     fsys,
		}
		p := &Prog{Runtime: rt}
		if _, err := p.checksum(ctx, algorithms["crc"], options{}, "file", true); !errors.Is(err, context.Canceled) {
			t.Errorf("checksum() error = %v, want context.Canceled", err)
		}
		if !fsys.closed {
			t.Error("checksum() didn't close the file")
		}
	})
}

func TestCrc(t *testing.T) {
	tests := []struct {
		name string
		data string
		want uint32
	}{
		{name: "an empty input", data: "", want: 4294967295},
		{name: "abc", data: "abc", want: 1219131554},
		{name: "hello", data: "hello", want: 3287646509},
		{name: "the quick brown fox", data: "The quick brown fox jumps over the lazy dog", want: 2074844392},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var c crc
			c.write([]byte(test.data))
			c.writeSize(int64(len(test.data)))
			if got := ^c.value; got != test.want {
				t.Errorf("checksum of %q = %d, want %d", test.data, got, test.want)
			}
		})
	}
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got, want := stdout.String(), usage; got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: new(bytes.Buffer)}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		want := "cksum (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
