package shasum

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// The digests of "hello\n", as the algorithms compute them.
const (
	helloSHA1   = "f572d396fae9206628714fb2ce00f72e94f2258f"
	helloSHA224 = "2d6d67d91d0badcdd06cbbba1fe11538a68a37ec9c2e26457ceff12b"
	helloSHA256 = "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"
	helloSHA384 = "1d0f284efe3edea4b9ca3bd514fa134b17eae361ccc7a1eefeff801b9bd6604e" +
		"01f21f6bf249ef030599f0c218f2ba8c"
	helloSHA512 = "e7c22b994c59d9cf2b48e549b1e24666636045930d3da7c1acb299d1c3b7f931" +
		"f94aae41edda2c2b207a36e10f8bcb8d45223e54878f5b316e7ce3b6bc019629"
	helloSHA512224 = "4d32058e76908e46640feeaf6e3f47ca6c124e971745748aa0ce8f7a"
	helloSHA512256 = "7f3f0c0d5219f51459578305ed2bbc198588758da85d08024c79c1195d1cd611"
)

// testFS holds the files used by the tests:
//
//	hello.txt:   "hello\n"
//	sums.txt:    the SHA-1 checksum of hello.txt
//	sha256.txt:  the SHA-256 checksum of hello.txt
//	bsd256.txt:  the BSD-style SHA-256 checksum of hello.txt
//	bad.txt:     a checksum file holding a digest which doesn't match
func testFS() fsx.MemFS {
	bad := strings.Repeat("0", len(helloSHA1))
	return fsx.MemFS{
		"hello.txt":  {Data: []byte("hello\n")},
		"sums.txt":   {Data: []byte(helloSHA1 + "  hello.txt\n")},
		"sha256.txt": {Data: []byte(helloSHA256 + "  hello.txt\n")},
		"bsd256.txt": {Data: []byte("SHA256 (hello.txt) = " + helloSHA256 + "\n")},
		"bad.txt":    {Data: []byte(bad + "  hello.txt\n")},
	}
}

func TestRun(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the digest of a file",
			FS:   testFS(),
			Args: []string{"hello.txt"},
			Want: helloSHA1 + "  hello.txt\n",
		},
		{
			Name:  "the digest of the standard input",
			Stdin: strings.NewReader("hello\n"),
			Want:  helloSHA1 + "  -\n",
		},
		{
			Name: "an algorithm attached to its option",
			FS:   testFS(),
			Args: []string{"-a256", "hello.txt"},
			Want: helloSHA256 + "  hello.txt\n",
		},
		{
			Name: "a long algorithm option with an attached value",
			FS:   testFS(),
			Args: []string{"--algorithm=256", "hello.txt"},
			Want: helloSHA256 + "  hello.txt\n",
		},
		{
			Name: "a long algorithm option with the value in the next argument",
			FS:   testFS(),
			Args: []string{"--algorithm", "256", "hello.txt"},
			Want: helloSHA256 + "  hello.txt\n",
		},
		{
			Name: "binary mode",
			FS:   testFS(),
			Args: []string{"-b", "hello.txt"},
			Want: helloSHA1 + " *hello.txt\n",
		},
		{
			Name: "a BSD style line",
			FS:   testFS(),
			Args: []string{"--tag", "hello.txt"},
			Want: "SHA1 (hello.txt) = " + helloSHA1 + "\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunAlgorithms(t *testing.T) {
	for _, test := range []struct {
		algorithm string
		digest    string
	}{
		{"1", helloSHA1},
		{"224", helloSHA224},
		{"256", helloSHA256},
		{"384", helloSHA384},
		{"512", helloSHA512},
		{"512224", helloSHA512224},
		{"512256", helloSHA512256},
	} {
		t.Run("the "+test.algorithm+" algorithm", func(t *testing.T) {
			testutil.Once(t, New, testutil.Case{
				FS:   testFS(),
				Args: []string{"-a", test.algorithm, "hello.txt"},
				Want: test.digest + "  hello.txt\n",
			})
		})
	}

	for _, test := range []struct {
		algorithm string
		tag       string
		digest    string
	}{
		{"1", "SHA1", helloSHA1},
		{"224", "SHA224", helloSHA224},
		{"256", "SHA256", helloSHA256},
		{"384", "SHA384", helloSHA384},
		{"512", "SHA512", helloSHA512},
		{"512224", "SHA512/224", helloSHA512224},
		{"512256", "SHA512/256", helloSHA512256},
	} {
		t.Run("the "+test.algorithm+" algorithm with --tag", func(t *testing.T) {
			testutil.Once(t, New, testutil.Case{
				FS:   testFS(),
				Args: []string{"-a", test.algorithm, "--tag", "hello.txt"},
				Want: test.tag + " (hello.txt) = " + test.digest + "\n",
			})
		})
	}
}

func TestRunInvalid(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "the short help option",
			Args:     []string{"-h"},
			WantErr:  "shasum: invalid option -- 'h'\nTry 'shasum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "the short version option",
			Args:     []string{"-v"},
			WantErr:  "shasum: invalid option -- 'v'\nTry 'shasum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an unrecognized algorithm",
			FS:       testFS(),
			Args:     []string{"-a", "3", "hello.txt"},
			WantErr:  "shasum: unrecognized algorithm '3'\nTry 'shasum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an option the GNU utilities take",
			FS:       testFS(),
			Args:     []string{"--zero", "hello.txt"},
			WantErr:  "shasum: unrecognized option '--zero'\nTry 'shasum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an option of the Perl script which isn't supported",
			FS:       testFS(),
			Args:     []string{"-0", "hello.txt"},
			WantErr:  "shasum: invalid option -- '0'\nTry 'shasum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an algorithm without its value",
			FS:       testFS(),
			Args:     []string{"-a"},
			WantErr:  "shasum: option requires an argument -- 'a'\nTry 'shasum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "both file modes",
			FS:       testFS(),
			Args:     []string{"-bt", "hello.txt"},
			WantErr:  "shasum: Ambiguous file mode\nTry 'shasum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "warn without verifying",
			FS:       testFS(),
			Args:     []string{"-w", "hello.txt"},
			WantErr:  "shasum: --warn option used only when verifying checksums\nTry 'shasum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "the tag option with the text mode",
			FS:       testFS(),
			Args:     []string{"--tag", "-t", "hello.txt"},
			WantErr:  "shasum: --tag does not support --text mode\nTry 'shasum --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a file which isn't there",
			FS:       testFS(),
			Args:     []string{"missing.txt"},
			WantErr:  "shasum: missing.txt: file does not exist\n",
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
		want := `Usage: shasum [-bcqstw] [-a algorithm] [file ...]

  -a, --algorithm=algorithm
         1 (default), 224, 256, 384, 512, 512224, 512256
  -b, --binary
         read in binary mode
  -c, --check
         read SHA sums from the files and check them
      --ignore-missing
         don't fail or report status for missing files
  -q, --quiet
         don't print OK for each successfully verified file
  -s, --status
         don't output anything, status code shows success
      --strict
         exit non-zero for improperly formatted checksum lines
      --tag
         create a BSD-style checksum
  -t, --text
         read in text mode (default)
  -w, --warn
         warn about improperly formatted checksum lines
      --help
         display this help and exit
      --version
         output version information and exit
`
		if got := stdout.String(); got != want {
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
		want := "shasum (charm.land/turtle/coreutils) " + coreutils.Version +
			"\nhttps://charm.land/turtle\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}

func TestRunCheck(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "verifying a checksum",
			FS:   testFS(),
			Args: []string{"-c", "sums.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name: "verifying with another algorithm",
			FS:   testFS(),
			Args: []string{"-a", "256", "-c", "sha256.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name: "verifying a BSD style line",
			FS:   testFS(),
			Args: []string{"-a", "256", "-c", "bsd256.txt"},
			Want: "hello.txt: OK\n",
		},
		{
			Name:     "a checksum which doesn't match",
			FS:       testFS(),
			Args:     []string{"-c", "bad.txt"},
			Want:     "hello.txt: FAILED\n",
			WantErr:  "shasum: WARNING: 1 computed checksum did NOT match\n",
			WantExit: 1,
		},
		{
			Name:     "the algorithm the checksum lines don't hold",
			FS:       testFS(),
			Args:     []string{"-c", "sha256.txt"},
			WantErr:  "shasum: sha256.txt: no properly formatted checksum lines found\n",
			WantExit: 1,
		},
		{
			Name: "the quiet option hides the OK lines",
			FS:   testFS(),
			Args: []string{"-q", "-c", "sums.txt"},
		},
		{
			Name:     "the status option holds everything back",
			FS:       testFS(),
			Args:     []string{"-s", "-c", "bad.txt"},
			WantExit: 1,
		},
		{
			Name:     "a checksum file which isn't there",
			FS:       testFS(),
			Args:     []string{"-c", "missing.txt"},
			WantErr:  "shasum: missing.txt: file does not exist\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}
