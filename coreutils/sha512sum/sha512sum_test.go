package sha512sum

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/digest"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

// helloDigest is the SHA512 digest of "hello\n".
const helloDigest = "e7c22b994c59d9cf2b48e549b1e24666636045930d3da7c1acb299d1c3b7f931f94aae41edda2c2b207a36e10f8bcb8d45223e54878f5b316e7ce3b6bc019629"

// testFS holds the files used by the tests:
//
//	hello.txt: "hello\n"
//	sums.txt:  a checksum file holding the digest of hello.txt
//	bad.txt:   a checksum file holding a digest which doesn't match
func testFS() fsx.MemFS {
	bad := strings.Repeat("0", len(helloDigest))
	return fsx.MemFS{
		"hello.txt": {Data: []byte("hello\n")},
		"sums.txt":  {Data: []byte(helloDigest + "  hello.txt\n")},
		"bad.txt":   {Data: []byte(bad + "  hello.txt\n")},
	}
}

func TestRun(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the digest of a file",
			FS:   testFS(),
			Args: []string{"hello.txt"},
			Want: helloDigest + "  hello.txt\n",
		},
		{
			Name:  "the digest of the standard input",
			Stdin: strings.NewReader("hello\n"),
			Want:  helloDigest + "  -\n",
		},
		{
			Name: "binary mode",
			FS:   testFS(),
			Args: []string{"-b", "hello.txt"},
			Want: helloDigest + " *hello.txt\n",
		},
		{
			Name: "a BSD style line",
			FS:   testFS(),
			Args: []string{"--tag", "hello.txt"},
			Want: "SHA512 (hello.txt) = " + helloDigest + "\n",
		},
		{
			Name: "verifying a checksum",
			FS:   testFS(),
			Args: []string{"-c", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name:     "a checksum which doesn't match",
			FS:       testFS(),
			Args:     []string{"-c", "bad.txt"},
			Want:     "hello.txt: FAILED\n",
			WantErr:  "sha512sum: WARNING: 1 computed checksum did NOT match\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestHelpVersion(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
		if err := New(rt).Run(t.Context(), "--help"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
		if got, want := stdout.String(), digest.Usage("sha512sum"); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})

	t.Run("version", func(t *testing.T) {
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
		if err := New(rt).Run(t.Context(), "--version"); err != nil {
			t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("Run() stderr = %q, want empty", got)
		}
		want := "sha512sum (charm.land/snail/coreutils) " + coreutils.Version +
			"\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
