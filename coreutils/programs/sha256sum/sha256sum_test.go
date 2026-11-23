package sha256sum

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/digest"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// helloDigest is the SHA256 digest of "hello\n".
const helloDigest = "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"

// testFS holds the files used by the tests:
//
//	hello.txt: "hello\n"
//	sums.txt:  a checksum file holding the digest of hello.txt
//	bad.txt:   a checksum file holding a digest which doesn't match
func testFS() fsx.MemoryFS {
	bad := strings.Repeat("0", len(helloDigest))
	return fsx.MemoryFS{
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
			Want: "SHA256 (hello.txt) = " + helloDigest + "\n",
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
			WantErr:  "sha256sum: WARNING: 1 computed checksum did NOT match\n",
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
		if got, want := stdout.String(), digest.Usage("sha256sum"); got != want {
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
		want := "sha256sum (charm.land/turtle/coreutils) " + coreutils.Version +
			"\nhttps://charm.land/turtle\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
