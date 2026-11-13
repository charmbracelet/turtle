package digest

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"runtime"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// The algorithms of the utilities, built by the tests as the utilities
// themselves build them.
var (
	testMD5    = Algorithm{Name: "md5sum", Tag: "MD5", New: md5.New}
	testSHA1   = Algorithm{Name: "sha1sum", Tag: "SHA1", New: sha1.New}
	testSHA224 = Algorithm{Name: "sha224sum", Tag: "SHA224", New: sha256.New224}
	testSHA256 = Algorithm{Name: "sha256sum", Tag: "SHA256", New: sha256.New}
	testSHA384 = Algorithm{Name: "sha384sum", Tag: "SHA384", New: sha512.New384}
	testSHA512 = Algorithm{Name: "sha512sum", Tag: "SHA512", New: sha512.New}
)

// The digests of the fixtures of testFS, as the reference implementations of
// the algorithms compute them.
const (
	emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	helloMD5    = "b1946ac92492d2347c6235b4d2611184"
	helloSHA1   = "f572d396fae9206628714fb2ce00f72e94f2258f"
	helloSHA224 = "2d6d67d91d0badcdd06cbbba1fe11538a68a37ec9c2e26457ceff12b"
	helloSHA256 = "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"
	helloSHA384 = "1d0f284efe3edea4b9ca3bd514fa134b17eae361ccc7a1eefeff801b9bd6604e" +
		"01f21f6bf249ef030599f0c218f2ba8c"
	helloSHA512 = "e7c22b994c59d9cf2b48e549b1e24666636045930d3da7c1acb299d1c3b7f931" +
		"f94aae41edda2c2b207a36e10f8bcb8d45223e54878f5b316e7ce3b6bc019629"
	fooSHA256   = "d78931fcf2660108eec0d6674ecb4e02401b5256a6b5ee82527766ef6d198c67"
	innerSHA256 = "940a68104d3b690442453f4be394b0a14721a174127d84c1c2f834b7ad05d684"
)

// testFS holds the files used by the tests:
//
//	hello.txt:       "hello\n"
//	empty.txt:       ""
//	foo.txt:         "foo\nbar\n"
//	sub/inner.txt:   "inner\n"
//	-c:              "hello\n"
//	 lead.txt:       "hello\n"
//	we\nird.txt:     "hello\n"
//	cr\rname.txt:    "hello\n"
//	back\slash.txt:  "hello\n"
//	a)b.txt:         "hello\n"
func testFS() fsx.MemFS {
	hello := []byte("hello\n")
	return fsx.MemFS{
		"hello.txt":      {Data: hello},
		"empty.txt":      {Data: []byte("")},
		"foo.txt":        {Data: []byte("foo\nbar\n")},
		"sub/inner.txt":  {Data: []byte("inner\n")},
		"-c":             {Data: hello},
		" lead.txt":      {Data: hello},
		"we\nird.txt":    {Data: hello},
		"cr\rname.txt":   {Data: hello},
		`back\slash.txt`: {Data: hello},
		"a)b.txt":        {Data: hello},
	}
}

// newAlgorithm adapts an Algorithm to the NewFunc the shared test runner
// takes.
func newAlgorithm(a Algorithm) coreutils.NewFunc {
	return func(rt *coreutils.Runtime) coreutils.Program {
		return algorithmProg{a, rt}
	}
}

// algorithmProg runs one digest algorithm as a coreutils.Program.
type algorithmProg struct {
	Algorithm
	rt *coreutils.Runtime
}

func (p algorithmProg) Run(ctx context.Context, args ...string) error {
	return p.Algorithm.Run(ctx, p.rt, args)
}

// newSHA256 runs the algorithm the tests default to.
var newSHA256 = newAlgorithm(testSHA256)

func TestRunDigests(t *testing.T) {
	for _, test := range []struct {
		algorithm Algorithm
		digest    string
	}{
		{testMD5, helloMD5},
		{testSHA1, helloSHA1},
		{testSHA224, helloSHA224},
		{testSHA256, helloSHA256},
		{testSHA384, helloSHA384},
		{testSHA512, helloSHA512},
	} {
		t.Run(test.algorithm.Name, func(t *testing.T) {
			testutil.Once(t, newAlgorithm(test.algorithm), testutil.Case{
				Stdin: strings.NewReader("hello\n"),
				Want:  test.digest + "  -\n",
			})
		})
	}
}

func TestRunOperands(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "no operands read the standard input",
			Stdin: strings.NewReader("hello\n"),
			Want:  helloSHA256 + "  -\n",
		},
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("hello\n"),
			Args:  []string{"-"},
			Want:  helloSHA256 + "  -\n",
		},
		{
			Name:  "double dash with no operands reads the standard input",
			Stdin: strings.NewReader("hello\n"),
			Args:  []string{"--"},
			Want:  helloSHA256 + "  -\n",
		},
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"hello.txt"},
			Want: helloSHA256 + "  hello.txt\n",
		},
		{
			Name: "an empty file",
			FS:   testFS(),
			Args: []string{"empty.txt"},
			Want: emptySHA256 + "  empty.txt\n",
		},
		{
			Name: "several operands are all written, in order",
			FS:   testFS(),
			Args: []string{"foo.txt", "hello.txt", "empty.txt"},
			Want: fooSHA256 + "  foo.txt\n" +
				helloSHA256 + "  hello.txt\n" +
				emptySHA256 + "  empty.txt\n",
		},
		{
			Name: "a file is resolved against the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"inner.txt"},
			Want: innerSHA256 + "  inner.txt\n",
		},
		{
			Name: "a file named like an option after a double dash",
			FS:   testFS(),
			Args: []string{"--", "-c"},
			Want: helloSHA256 + "  -c\n",
		},
		{
			Name:  "the standard input is written as dash",
			Stdin: strings.NewReader("hello\n"),
			FS:    testFS(),
			Args:  []string{"-", "hello.txt"},
			Want:  helloSHA256 + "  -\n" + helloSHA256 + "  hello.txt\n",
		},
	}

	testutil.Run(t, newSHA256, tests)
}

func TestRunModes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "text mode is the default",
			FS:   testFS(),
			Args: []string{"hello.txt"},
			Want: helloSHA256 + "  hello.txt\n",
		},
		{
			Name: "text mode writes one space and the mode character",
			FS:   testFS(),
			Args: []string{"-t", "hello.txt"},
			Want: helloSHA256 + "  hello.txt\n",
		},
		{
			Name: "the long text option",
			FS:   testFS(),
			Args: []string{"--text", "hello.txt"},
			Want: helloSHA256 + "  hello.txt\n",
		},
		{
			Name: "binary mode writes a star",
			FS:   testFS(),
			Args: []string{"-b", "hello.txt"},
			Want: helloSHA256 + " *hello.txt\n",
		},
		{
			Name: "the long binary option",
			FS:   testFS(),
			Args: []string{"--binary", "hello.txt"},
			Want: helloSHA256 + " *hello.txt\n",
		},
		{
			Name: "the last mode option wins",
			FS:   testFS(),
			Args: []string{"-bt", "hello.txt"},
			Want: helloSHA256 + "  hello.txt\n",
		},
		{
			Name: "the other order of the mode options",
			FS:   testFS(),
			Args: []string{"-tb", "hello.txt"},
			Want: helloSHA256 + " *hello.txt\n",
		},
		{
			Name: "the tag option writes BSD style",
			FS:   testFS(),
			Args: []string{"--tag", "hello.txt"},
			Want: "SHA256 (hello.txt) = " + helloSHA256 + "\n",
		},
		{
			Name:  "the tag option writes the operand of the standard input",
			Stdin: strings.NewReader("hello\n"),
			Args:  []string{"--tag"},
			Want:  "SHA256 (-) = " + helloSHA256 + "\n",
		},
		{
			Name: "the tag option hides the input mode",
			FS:   testFS(),
			Args: []string{"--tag", "-b", "hello.txt"},
			Want: "SHA256 (hello.txt) = " + helloSHA256 + "\n",
		},
		{
			Name: "zero ends the line with a NUL",
			FS:   testFS(),
			Args: []string{"-z", "hello.txt"},
			Want: helloSHA256 + "  hello.txt\x00",
		},
		{
			Name: "the long zero option",
			FS:   testFS(),
			Args: []string{"--zero", "hello.txt", "foo.txt"},
			Want: helloSHA256 + "  hello.txt\x00" + fooSHA256 + "  foo.txt\x00",
		},
	}

	testutil.Run(t, newSHA256, tests)

	t.Run("the tag option of another algorithm", func(t *testing.T) {
		testutil.Once(t, newAlgorithm(testMD5), testutil.Case{
			FS:   testFS(),
			Args: []string{"--tag", "hello.txt"},
			Want: "MD5 (hello.txt) = " + helloMD5 + "\n",
		})
	})
}

func TestRunEscapedNames(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a file name holding a newline is escaped",
			FS:   testFS(),
			Args: []string{"we\nird.txt"},
			Want: `\` + helloSHA256 + `  we\nird.txt` + "\n",
		},
		{
			Name: "the tag line is escaped as a whole",
			FS:   testFS(),
			Args: []string{"--tag", "we\nird.txt"},
			Want: `\SHA256 (we\nird.txt) = ` + helloSHA256 + "\n",
		},
		{
			Name: "zero disables the escaping",
			FS:   testFS(),
			Args: []string{"-z", "we\nird.txt"},
			Want: helloSHA256 + "  we\nird.txt\x00",
		},
		{
			Name: "a file name holding a carriage return is escaped",
			FS:   testFS(),
			Args: []string{"cr\rname.txt"},
			Want: `\` + helloSHA256 + `  cr\rname.txt` + "\n",
		},
	}

	testutil.Run(t, newSHA256, tests)

	t.Run("a file name holding a backslash is escaped", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skipf("not supported on %s", "windows")
		}
		testutil.Once(t, newSHA256, testutil.Case{
			FS:   testFS(),
			Args: []string{`back\slash.txt`},
			Want: `\` + helloSHA256 + `  back\\slash.txt` + "\n",
		})
	})
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "a file which isn't there",
			FS:       testFS(),
			Args:     []string{"missing.txt"},
			WantErr:  "sha256sum: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "the other operands are still written",
			FS:       testFS(),
			Args:     []string{"missing.txt", "hello.txt"},
			Want:     helloSHA256 + "  hello.txt\n",
			WantErr:  "sha256sum: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "no standard input to read",
			WantErr:  "sha256sum: -: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "no file system to read from",
			Args:     []string{"hello.txt"},
			WantErr:  "sha256sum: hello.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "the standard input which can't be read",
			Stdin:    testutil.ErrorReader{},
			WantErr:  "sha256sum: -: boom\n",
			WantExit: 1,
		},
		{
			Name:     "an option which isn't taken",
			FS:       testFS(),
			Args:     []string{"-q", "hello.txt"},
			WantErr:  "sha256sum: invalid option -- 'q'\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a long option which has no short equivalent",
			FS:       testFS(),
			Args:     []string{"-T", "hello.txt"},
			WantErr:  "sha256sum: invalid option -- 'T'\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a long option of another utility",
			FS:       testFS(),
			Args:     []string{"--raw", "hello.txt"},
			WantErr:  "sha256sum: unrecognized option '--raw'\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a long option given a value",
			FS:       testFS(),
			Args:     []string{"--tag=1", "hello.txt"},
			WantErr:  "sha256sum: option '--tag=1' doesn't allow an argument\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, newSHA256, tests)

	t.Run("a failing standard output", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdout: testutil.ErrorWriter{},
			Stderr: new(bytes.Buffer),
			FS:     testFS(),
		}
		err := newSHA256(rt).Run(t.Context(), "hello.txt")
		if code := testutil.ExitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1 (error = %v)", code, err)
		}
		if got, want := rt.Stderr.(*bytes.Buffer).String(), "sha256sum: write error: boom\n"; got != want {
			t.Errorf("Run() stderr = %q, want %q", got, want)
		}
	})
}

func TestRunOptionConflicts(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "zero while verifying",
			Args:     []string{"-z", "-c", "sums.txt"},
			WantErr:  "sha256sum: the --zero option is not supported when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "tag while verifying",
			Args:     []string{"--tag", "-c", "sums.txt"},
			WantErr:  "sha256sum: the --tag option is meaningless when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "binary while verifying",
			Args:     []string{"-b", "-c", "sums.txt"},
			WantErr:  "sha256sum: the --binary and --text options are meaningless when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "text while verifying",
			Args:     []string{"-c", "-t", "sums.txt"},
			WantErr:  "sha256sum: the --binary and --text options are meaningless when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "ignore-missing without verifying",
			Args:     []string{"--ignore-missing", "hello.txt"},
			WantErr:  "sha256sum: the --ignore-missing option is meaningful only when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "status without verifying",
			Args:     []string{"--status", "hello.txt"},
			WantErr:  "sha256sum: the --status option is meaningful only when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "warn without verifying",
			Args:     []string{"-w", "hello.txt"},
			WantErr:  "sha256sum: the --warn option is meaningful only when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "quiet without verifying",
			Args:     []string{"--quiet", "hello.txt"},
			WantErr:  "sha256sum: the --quiet option is meaningful only when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "strict without verifying",
			Args:     []string{"--strict", "hello.txt"},
			WantErr:  "sha256sum: the --strict option is meaningful only when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "tagging in text mode",
			Args:     []string{"--tag", "-t", "hello.txt"},
			WantErr:  "sha256sum: --tag does not support --text mode\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "text mode before the tag option",
			Args:     []string{"-t", "--tag", "hello.txt"},
			WantErr:  "sha256sum: --tag does not support --text mode\nTry 'sha256sum --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, newSHA256, tests)
}

func TestRunContext(t *testing.T) {
	t.Run("a context which is already done", func(t *testing.T) {
		testutil.Cancelled(t, newSHA256, strings.NewReader("hello\n"), testFS())
	})

	t.Run("a context which is done while reading", func(t *testing.T) {
		testutil.Cancelled(t, newSHA256, &testutil.CancelReader{Data: []byte("hello")}, testFS())
	})

	t.Run("a context which is done while verifying", func(t *testing.T) {
		testutil.Cancelled(t, newSHA256, &testutil.CancelReader{Data: []byte("hello")}, testFS(), "-c")
	})
}

// testFamily is the family utility the shasum tests would build.
var testFamily = Family{
	Name:    "shasum",
	Default: "1",
	Algorithms: map[string]Algorithm{
		"1": {New: sha1.New},
	},
}

func TestHelpVersion(t *testing.T) {
	tests := []struct {
		name  string
		usage string
		run   func(ctx context.Context, rt *coreutils.Runtime, args []string) error
	}{
		{"shasum", usage("shasum"), testFamily.Run},
		{"md5sum", Usage("md5sum"), testMD5.Run},
		{"sha1sum", Usage("sha1sum"), testSHA1.Run},
		{"sha224sum", Usage("sha224sum"), testSHA224.Run},
		{"sha256sum", Usage("sha256sum"), testSHA256.Run},
		{"sha384sum", Usage("sha384sum"), testSHA384.Run},
		{"sha512sum", Usage("sha512sum"), testSHA512.Run},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("help", func(t *testing.T) {
				stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
				rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
				if err := test.run(t.Context(), rt, []string{"--help"}); err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got := stderr.String(); got != "" {
					t.Errorf("Run() stderr = %q, want empty", got)
				}
				if got := stdout.String(); got != test.usage {
					t.Errorf("Run() stdout = %q, want %q", got, test.usage)
				}
			})

			t.Run("version", func(t *testing.T) {
				stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
				rt := &coreutils.Runtime{Stdout: stdout, Stderr: stderr}
				if err := test.run(t.Context(), rt, []string{"--version"}); err != nil {
					t.Fatalf("Run() error = %v, want nil (exit status 0)", err)
				}
				if got := stderr.String(); got != "" {
					t.Errorf("Run() stderr = %q, want empty", got)
				}
				want := test.name + " (charm.land/turtle/coreutils) " + coreutils.Version +
					"\nhttps://charm.land/turtle\n"
				if got := stdout.String(); got != want {
					t.Errorf("Run() stdout = %q, want %q", got, want)
				}
			})
		})
	}
}
