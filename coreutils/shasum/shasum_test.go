package shasum

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
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

type testCase struct {
	name     string
	stdin    io.Reader
	fsys     fsx.FS
	args     []string
	want     string
	wantErr  string
	wantExit int
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  test.stdin,
		Stdout: stdout,
		Stderr: stderr,
		FS:     test.fsys,
	}
	err := New(rt).Run(t.Context(), test.args...)

	code := 0
	if err != nil {
		var exitErr *coreutils.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
		}
		code = exitErr.Code
	}
	if code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if got := stdout.String(); got != test.want {
		t.Errorf("Run() stdout = %q, want %q", got, test.want)
	}
	if got := stderr.String(); got != test.wantErr {
		t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
	}
}

func TestRun(t *testing.T) {
	tests := []testCase{
		{
			name: "the digest of a file",
			fsys: testFS(),
			args: []string{"hello.txt"},
			want: helloSHA1 + "  hello.txt\n",
		},
		{
			name:  "the digest of the standard input",
			stdin: strings.NewReader("hello\n"),
			want:  helloSHA1 + "  -\n",
		},
		{
			name: "an algorithm attached to its option",
			fsys: testFS(),
			args: []string{"-a256", "hello.txt"},
			want: helloSHA256 + "  hello.txt\n",
		},
		{
			name: "a long algorithm option with an attached value",
			fsys: testFS(),
			args: []string{"--algorithm=256", "hello.txt"},
			want: helloSHA256 + "  hello.txt\n",
		},
		{
			name: "a long algorithm option with the value in the next argument",
			fsys: testFS(),
			args: []string{"--algorithm", "256", "hello.txt"},
			want: helloSHA256 + "  hello.txt\n",
		},
		{
			name: "binary mode",
			fsys: testFS(),
			args: []string{"-b", "hello.txt"},
			want: helloSHA1 + " *hello.txt\n",
		},
		{
			name: "a BSD style line",
			fsys: testFS(),
			args: []string{"--tag", "hello.txt"},
			want: "SHA1 (hello.txt) = " + helloSHA1 + "\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
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
			runCase(t, testCase{
				fsys: testFS(),
				args: []string{"-a", test.algorithm, "hello.txt"},
				want: test.digest + "  hello.txt\n",
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
			runCase(t, testCase{
				fsys: testFS(),
				args: []string{"-a", test.algorithm, "--tag", "hello.txt"},
				want: test.tag + " (hello.txt) = " + test.digest + "\n",
			})
		})
	}
}

func TestRunInvalid(t *testing.T) {
	tests := []testCase{
		{
			name:     "the short help option",
			args:     []string{"-h"},
			wantErr:  "shasum: invalid option -- 'h'\n",
			wantExit: 1,
		},
		{
			name:     "the short version option",
			args:     []string{"-v"},
			wantErr:  "shasum: invalid option -- 'v'\n",
			wantExit: 1,
		},
		{
			name:     "an unrecognized algorithm",
			fsys:     testFS(),
			args:     []string{"-a", "3", "hello.txt"},
			wantErr:  "shasum: unrecognized algorithm '3'\n",
			wantExit: 1,
		},
		{
			name:     "an option the GNU utilities take",
			fsys:     testFS(),
			args:     []string{"--zero", "hello.txt"},
			wantErr:  "shasum: unrecognized option '--zero'\n",
			wantExit: 1,
		},
		{
			name:     "an option of the Perl script which isn't supported",
			fsys:     testFS(),
			args:     []string{"-0", "hello.txt"},
			wantErr:  "shasum: invalid option -- '0'\n",
			wantExit: 1,
		},
		{
			name:     "an algorithm without its value",
			fsys:     testFS(),
			args:     []string{"-a"},
			wantErr:  "shasum: option requires an argument -- 'a'\n",
			wantExit: 1,
		},
		{
			name:     "both file modes",
			fsys:     testFS(),
			args:     []string{"-bt", "hello.txt"},
			wantErr:  "shasum: Ambiguous file mode\n",
			wantExit: 1,
		},
		{
			name:     "warn without verifying",
			fsys:     testFS(),
			args:     []string{"-w", "hello.txt"},
			wantErr:  "shasum: --warn option used only when verifying checksums\n",
			wantExit: 1,
		},
		{
			name:     "the tag option with the text mode",
			fsys:     testFS(),
			args:     []string{"--tag", "-t", "hello.txt"},
			wantErr:  "shasum: --tag does not support --text mode\n",
			wantExit: 1,
		},
		{
			name:     "a file which isn't there",
			fsys:     testFS(),
			args:     []string{"missing.txt"},
			wantErr:  "shasum: missing.txt: file does not exist\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
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
      --help     display this help and exit
      --version  output version information and exit
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
		want := "shasum (charm.land/snail/coreutils) " + coreutils.Version +
			"\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}

func TestRunCheck(t *testing.T) {
	tests := []testCase{
		{
			name: "verifying a checksum",
			fsys: testFS(),
			args: []string{"-c", "sums.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name: "verifying with another algorithm",
			fsys: testFS(),
			args: []string{"-a", "256", "-c", "sha256.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name: "verifying a BSD style line",
			fsys: testFS(),
			args: []string{"-a", "256", "-c", "bsd256.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name:     "a checksum which doesn't match",
			fsys:     testFS(),
			args:     []string{"-c", "bad.txt"},
			want:     "hello.txt: FAILED\n",
			wantErr:  "shasum: WARNING: 1 computed checksum did NOT match\n",
			wantExit: 1,
		},
		{
			name:     "the algorithm the checksum lines don't hold",
			fsys:     testFS(),
			args:     []string{"-c", "sha256.txt"},
			wantErr:  "shasum: sha256.txt: no properly formatted checksum lines found\n",
			wantExit: 1,
		},
		{
			name: "the quiet option hides the OK lines",
			fsys: testFS(),
			args: []string{"-q", "-c", "sums.txt"},
		},
		{
			name:     "the status option holds everything back",
			fsys:     testFS(),
			args:     []string{"-s", "-c", "bad.txt"},
			wantExit: 1,
		},
		{
			name:     "a checksum file which isn't there",
			fsys:     testFS(),
			args:     []string{"-c", "missing.txt"},
			wantErr:  "shasum: missing.txt: file does not exist\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}
