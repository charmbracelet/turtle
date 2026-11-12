package cksum

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

// files holds the fixtures used by the tests, with the checksums the
// algorithm of the specification gives for them.
var files = fsx.MemFS{
	"abc.txt":   {Data: []byte("abc")},
	"empty.txt": {Data: []byte("")},
	"hello.txt": {Data: []byte("hello")},
	"sub/1.txt": {Data: []byte("1\n")},
}

func TestRun(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "the standard input without a name",
			Stdin: strings.NewReader("abc"),
			Want:  "1219131554 3\n",
		},
		{
			Name:  "an empty input",
			Stdin: strings.NewReader(""),
			Want:  "4294967295 0\n",
		},
		{
			Name:  "a binary input",
			Stdin: strings.NewReader("\x00\xff\x80abc"),
			Want:  "1630418514 6\n",
		},
		{
			Name:  "an input across chunks",
			Stdin: strings.NewReader(strings.Repeat("a", 40000)),
			Want:  "330876385 40000\n",
		},
		{
			Name:  "an explicit standard input keeps its name",
			Stdin: strings.NewReader("abc"),
			Args:  []string{"-"},
			Want:  "1219131554 3 -\n",
		},
		{
			Name: "a file",
			FS:   files,
			Args: []string{"abc.txt"},
			Want: "1219131554 3 abc.txt\n",
		},
		{
			Name: "several files",
			FS:   files,
			Args: []string{"abc.txt", "hello.txt"},
			Want: "1219131554 3 abc.txt\n3287646509 5 hello.txt\n",
		},
		{
			Name: "a file in a subdirectory",
			FS:   files,
			Args: []string{"sub/1.txt"},
			Want: "4219530715 2 sub/1.txt\n",
		},
		{
			Name: "a file named like an option",
			FS:   files,
			Args: []string{"--", "abc.txt"},
			Want: "1219131554 3 abc.txt\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLegacyAlgorithms(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "crc is the algorithm of the default",
			FS:   files,
			Args: []string{"-a", "crc", "abc.txt"},
			Want: "1219131554 3 abc.txt\n",
		},
		{
			Name: "sysv pads its checksum and blocks",
			FS:   files,
			Args: []string{"-a", "sysv", "abc.txt"},
			Want: "00294     1 abc.txt\n",
		},
		{
			Name:  "sysv without a name",
			Stdin: strings.NewReader("abc"),
			Args:  []string{"-asysv"},
			Want:  "00294     1\n",
		},
		{
			Name:  "sysv rounds the blocks up",
			Stdin: strings.NewReader(strings.Repeat("a", 513)),
			Args:  []string{"-a", "sysv"},
			Want:  "49761     2\n",
		},
		{
			Name: "bsd pads its checksum and blocks",
			FS:   files,
			Args: []string{"-a", "bsd", "abc.txt"},
			Want: "16556     1 abc.txt\n",
		},
		{
			Name:  "bsd rounds the blocks up",
			Stdin: strings.NewReader(strings.Repeat("a", 1025)),
			Args:  []string{"-a", "bsd"},
			Want:  "00033     2\n",
		},
		{
			Name: "crc32b prints the byte count",
			FS:   files,
			Args: []string{"--algorithm=crc32b", "abc.txt"},
			Want: "891568578 3 abc.txt\n",
		},
		{
			Name:  "crc32b of an empty input",
			Stdin: strings.NewReader(""),
			Args:  []string{"-a", "crc32b"},
			Want:  "0 0\n",
		},
	}

	testutil.Run(t, New, tests)
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
				testutil.Once(t, New, testutil.Case{
					FS:   files,
					Args: []string{"-a", test.algorithm, "abc.txt"},
					Want: test.tag + " (abc.txt) = " + test.digest + "\n",
				})
			})
			t.Run("untagged", func(t *testing.T) {
				testutil.Once(t, New, testutil.Case{
					FS:   files,
					Args: []string{"-a", test.algorithm, "--untagged", "abc.txt"},
					Want: test.digest + "  abc.txt\n",
				})
			})
		})
	}
}

func TestRunVariableAlgorithms(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "sha2 of 224 bits",
			FS:   files,
			Args: []string{"-a", "sha2", "-l", "224", "abc.txt"},
			Want: "SHA224 (abc.txt) = 23097d223405d8228642a477bda255b32aadbce4bda0b3f7e36c9da7\n",
		},
		{
			Name: "sha2 of 512 bits",
			FS:   files,
			Args: []string{"-a", "sha2", "--length=512", "abc.txt"},
			Want: "SHA512 (abc.txt) = ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f\n",
		},
		{
			Name: "sha3 of 224 bits",
			FS:   files,
			Args: []string{"-a", "sha3", "-l", "224", "abc.txt"},
			Want: "SHA3-224 (abc.txt) = e642824c3f8cf24ad09234ee7d3c766fc9a3a5168d0c94ad73b46fdf\n",
		},
		{
			Name: "sha3 of 256 bits",
			FS:   files,
			Args: []string{"-a", "sha3", "-l", "256", "abc.txt"},
			Want: "SHA3-256 (abc.txt) = 3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532\n",
		},
		{
			Name: "sha3 of 384 bits",
			FS:   files,
			Args: []string{"-a", "sha3", "-l", "384", "abc.txt"},
			Want: "SHA3-384 (abc.txt) = ec01498288516fc926459f58e2c6ad8df9b473cb0fc08c2596da7cf0e49be4b298d88cea927ac7f539f1edf228376d25\n",
		},
		{
			Name: "sha3 of 512 bits",
			FS:   files,
			Args: []string{"-a", "sha3", "-l", "512", "abc.txt"},
			Want: "SHA3-512 (abc.txt) = b751850b1a57168a5693cd924b6b096e08f621827444f70d884f5d0240d2712e10e116e9192af3c91a7ec57647e3934057340b4cf408d5a56592f8274eec53f0\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunDigestOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "an explicit --tag",
			FS:   files,
			Args: []string{"-a", "md5", "--tag", "abc.txt"},
			Want: "MD5 (abc.txt) = 900150983cd24fb0d6963f7d28e17f72\n",
		},
		{
			Name: "base64 in the tagged output",
			FS:   files,
			Args: []string{"-a", "sha256", "--base64", "abc.txt"},
			Want: "SHA256 (abc.txt) = ungWv48Bz+pBQUDeXa4iI7ADYaOWF3qctBD/YfIAFa0=\n",
		},
		{
			Name: "base64 in the untagged output",
			FS:   files,
			Args: []string{"-a", "md5", "--base64", "--untagged", "abc.txt"},
			Want: "kAFQmDzST7DWlj99KOF/cg==  abc.txt\n",
		},
		{
			Name: "a zero byte ends the lines with --zero",
			FS:   files,
			Args: []string{"-a", "md5", "--zero", "abc.txt", "hello.txt"},
			Want: "MD5 (abc.txt) = 900150983cd24fb0d6963f7d28e17f72\x00" +
				"MD5 (hello.txt) = 5d41402abc4b2a76b9719d911017c592\x00",
		},
		{
			Name:  "the standard input keeps its dash",
			Stdin: strings.NewReader("abc"),
			Args:  []string{"-a", "md5"},
			Want:  "MD5 (-) = 900150983cd24fb0d6963f7d28e17f72\n",
		},
	}

	testutil.Run(t, New, tests)

	t.Run("a name holding a line break is escaped", func(t *testing.T) {
		fsys := fsx.MemFS{"a\nb.txt": {Data: []byte("abc")}}
		testutil.Once(t, New, testutil.Case{
			FS:   fsys,
			Args: []string{"-a", "md5", "a\nb.txt"},
			Want: `\MD5 (a\nb.txt) = 900150983cd24fb0d6963f7d28e17f72` + "\n",
		})
		testutil.Once(t, New, testutil.Case{
			FS:   fsys,
			Args: []string{"-a", "md5", "--zero", "a\nb.txt"},
			Want: "MD5 (a\nb.txt) = 900150983cd24fb0d6963f7d28e17f72\x00",
		})
	})
}

func TestRunOptionErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "an unknown algorithm",
			FS:       files,
			Args:     []string{"-a", "bogus", "abc.txt"},
			WantErr:  "cksum: unrecognized algorithm 'bogus'\nTry 'cksum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a length without an algorithm",
			FS:       files,
			Args:     []string{"-l", "256", "abc.txt"},
			WantErr:  "cksum: the --length option is only supported with the sha2, sha3 and blake2b algorithms\nTry 'cksum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a length with a fixed digest",
			FS:       files,
			Args:     []string{"-a", "md5", "-l", "256", "abc.txt"},
			WantErr:  "cksum: the --length option is only supported with the sha2, sha3 and blake2b algorithms\nTry 'cksum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a length which sha2 requires",
			FS:       files,
			Args:     []string{"-a", "sha2", "abc.txt"},
			WantErr:  "cksum: the --length option is required with the sha2 algorithm\nTry 'cksum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a length which sha3 requires",
			FS:       files,
			Args:     []string{"-a", "sha3", "abc.txt"},
			WantErr:  "cksum: the --length option is required with the sha3 algorithm\nTry 'cksum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a length the algorithm has no digest of",
			FS:       files,
			Args:     []string{"-a", "sha2", "-l", "300", "abc.txt"},
			WantErr:  "cksum: the --length option of the sha2 algorithm must be 224, 256, 384 or 512\nTry 'cksum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "base64 of a legacy algorithm",
			FS:       files,
			Args:     []string{"--base64", "abc.txt"},
			WantErr:  "cksum: the --base64 option is not supported with the crc algorithm\nTry 'cksum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "tag and untagged together",
			FS:       files,
			Args:     []string{"-a", "md5", "--tag", "--untagged", "abc.txt"},
			WantErr:  "cksum: the --tag and --untagged options are mutually exclusive\nTry 'cksum --help' for more information.\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "an invalid option",
			FS:       files,
			Args:     []string{"-x", "abc.txt"},
			WantErr:  "cksum: invalid option -- 'x'\nTry 'cksum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file",
			FS:       files,
			Args:     []string{"missing.txt"},
			WantErr:  "cksum: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "the files after a missing one are still written",
			FS:       files,
			Args:     []string{"missing.txt", "abc.txt"},
			Want:     "1219131554 3 abc.txt\n",
			WantErr:  "cksum: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "no file system",
			Args:     []string{"abc.txt"},
			WantErr:  "cksum: abc.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "a read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("boom")},
			Args:     []string{"-"},
			WantErr:  "cksum: -: boom\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunIOErrors(t *testing.T) {
	t.Run("a write error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader("abc"),
			Stdout: testutil.ErrorWriter{Err: errors.New("boom")},
			Stderr: new(bytes.Buffer),
		}
		err := New(rt).Run(t.Context())
		if code := testutil.ExitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
	})

	t.Run("a done context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		rt := &coreutils.Runtime{
			Stdin:  &testutil.CancelReader{Cancel: cancel},
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
	testutil.HelpVersion(t, New, "cksum", usage)
}
