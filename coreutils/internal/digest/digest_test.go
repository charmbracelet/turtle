package digest

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
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

type testCase struct {
	name string

	// algorithm is the digest the run uses, defaulting to SHA256.
	algorithm Algorithm

	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	fsys    fsx.FS
	dir     string
	args    []string
	want    string
	wantErr string

	// wantExit is the exit status wanted, none when the run succeeds.
	wantExit int

	// skipOn names a platform where the test can't run.
	skipOn string
}

// runCase runs the utility of the algorithm of the test and compares what it
// wrote with what the test expects. A test which gives its own writer is
// spared the comparison of what that writer holds.
func runCase(t *testing.T, test testCase) {
	t.Helper()

	if test.skipOn != "" && runtime.GOOS == test.skipOn {
		t.Skipf("not supported on %s", test.skipOn)
	}
	algorithm := test.algorithm
	if algorithm.Name == "" {
		algorithm = testSHA256
	}

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      test.stdin,
		Stdout:     orWriter(test.stdout, stdout),
		Stderr:     orWriter(test.stderr, stderr),
		FS:         test.fsys,
		WorkingDir: test.dir,
	}

	err := algorithm.Run(t.Context(), rt, test.args)

	var exitErr *coreutils.ExitError
	code := 0
	if err != nil {
		if !errors.As(err, &exitErr) {
			t.Fatalf("Run() error = %v, want *coreutils.ExitError", err)
		}
		code = exitErr.Code
	}
	if code != test.wantExit {
		t.Errorf("Run() exit code = %d, want %d (error = %v)", code, test.wantExit, err)
	}
	if test.stdout == nil {
		if got := stdout.String(); got != test.want {
			t.Errorf("Run() stdout = %q, want %q", got, test.want)
		}
	}
	if test.stderr == nil {
		if got := stderr.String(); got != test.wantErr {
			t.Errorf("Run() stderr = %q, want %q", got, test.wantErr)
		}
	}
}

// orWriter returns w, falling back to the buffer of the test when it isn't
// given.
func orWriter(w, fallback io.Writer) io.Writer {
	if w == nil {
		return fallback
	}
	return w
}

// cancelReader cancels its context on the first read, simulating a context
// that is done while reading the input.
type cancelReader struct {
	cancel context.CancelFunc
	reads  int
}

func (r *cancelReader) Read(p []byte) (int, error) {
	if r.reads > 0 {
		return 0, io.EOF
	}
	r.reads++
	r.cancel()
	return copy(p, "hello"), nil
}

// errorReader always fails to be read.
type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

// errorWriter always fails to be written to.
type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

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
			runCase(t, testCase{
				algorithm: test.algorithm,
				stdin:     strings.NewReader("hello\n"),
				want:      test.digest + "  -\n",
			})
		})
	}
}

func TestRunOperands(t *testing.T) {
	tests := []testCase{
		{
			name:  "no operands read the standard input",
			stdin: strings.NewReader("hello\n"),
			want:  helloSHA256 + "  -\n",
		},
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("hello\n"),
			args:  []string{"-"},
			want:  helloSHA256 + "  -\n",
		},
		{
			name:  "double dash with no operands reads the standard input",
			stdin: strings.NewReader("hello\n"),
			args:  []string{"--"},
			want:  helloSHA256 + "  -\n",
		},
		{
			name: "a file operand",
			fsys: testFS(),
			args: []string{"hello.txt"},
			want: helloSHA256 + "  hello.txt\n",
		},
		{
			name: "an empty file",
			fsys: testFS(),
			args: []string{"empty.txt"},
			want: emptySHA256 + "  empty.txt\n",
		},
		{
			name: "several operands are all written, in order",
			fsys: testFS(),
			args: []string{"foo.txt", "hello.txt", "empty.txt"},
			want: fooSHA256 + "  foo.txt\n" +
				helloSHA256 + "  hello.txt\n" +
				emptySHA256 + "  empty.txt\n",
		},
		{
			name: "a file is resolved against the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt"},
			want: innerSHA256 + "  inner.txt\n",
		},
		{
			name: "a file named like an option after a double dash",
			fsys: testFS(),
			args: []string{"--", "-c"},
			want: helloSHA256 + "  -c\n",
		},
		{
			name:  "the standard input is written as dash",
			stdin: strings.NewReader("hello\n"),
			fsys:  testFS(),
			args:  []string{"-", "hello.txt"},
			want:  helloSHA256 + "  -\n" + helloSHA256 + "  hello.txt\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunModes(t *testing.T) {
	tests := []testCase{
		{
			name: "text mode is the default",
			fsys: testFS(),
			args: []string{"hello.txt"},
			want: helloSHA256 + "  hello.txt\n",
		},
		{
			name: "text mode writes one space and the mode character",
			fsys: testFS(),
			args: []string{"-t", "hello.txt"},
			want: helloSHA256 + "  hello.txt\n",
		},
		{
			name: "the long text option",
			fsys: testFS(),
			args: []string{"--text", "hello.txt"},
			want: helloSHA256 + "  hello.txt\n",
		},
		{
			name: "binary mode writes a star",
			fsys: testFS(),
			args: []string{"-b", "hello.txt"},
			want: helloSHA256 + " *hello.txt\n",
		},
		{
			name: "the long binary option",
			fsys: testFS(),
			args: []string{"--binary", "hello.txt"},
			want: helloSHA256 + " *hello.txt\n",
		},
		{
			name: "the last mode option wins",
			fsys: testFS(),
			args: []string{"-bt", "hello.txt"},
			want: helloSHA256 + "  hello.txt\n",
		},
		{
			name: "the other order of the mode options",
			fsys: testFS(),
			args: []string{"-tb", "hello.txt"},
			want: helloSHA256 + " *hello.txt\n",
		},
		{
			name: "the tag option writes BSD style",
			fsys: testFS(),
			args: []string{"--tag", "hello.txt"},
			want: "SHA256 (hello.txt) = " + helloSHA256 + "\n",
		},
		{
			name:      "the tag option of another algorithm",
			algorithm: testMD5,
			fsys:      testFS(),
			args:      []string{"--tag", "hello.txt"},
			want:      "MD5 (hello.txt) = " + helloMD5 + "\n",
		},
		{
			name:  "the tag option writes the operand of the standard input",
			stdin: strings.NewReader("hello\n"),
			args:  []string{"--tag"},
			want:  "SHA256 (-) = " + helloSHA256 + "\n",
		},
		{
			name: "the tag option hides the input mode",
			fsys: testFS(),
			args: []string{"--tag", "-b", "hello.txt"},
			want: "SHA256 (hello.txt) = " + helloSHA256 + "\n",
		},
		{
			name: "zero ends the line with a NUL",
			fsys: testFS(),
			args: []string{"-z", "hello.txt"},
			want: helloSHA256 + "  hello.txt\x00",
		},
		{
			name: "the long zero option",
			fsys: testFS(),
			args: []string{"--zero", "hello.txt", "foo.txt"},
			want: helloSHA256 + "  hello.txt\x00" + fooSHA256 + "  foo.txt\x00",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunEscapedNames(t *testing.T) {
	tests := []testCase{
		{
			name: "a file name holding a newline is escaped",
			fsys: testFS(),
			args: []string{"we\nird.txt"},
			want: `\` + helloSHA256 + `  we\nird.txt` + "\n",
		},
		{
			name: "the tag line is escaped as a whole",
			fsys: testFS(),
			args: []string{"--tag", "we\nird.txt"},
			want: `\SHA256 (we\nird.txt) = ` + helloSHA256 + "\n",
		},
		{
			name: "zero disables the escaping",
			fsys: testFS(),
			args: []string{"-z", "we\nird.txt"},
			want: helloSHA256 + "  we\nird.txt\x00",
		},
		{
			name: "a file name holding a carriage return is escaped",
			fsys: testFS(),
			args: []string{"cr\rname.txt"},
			want: `\` + helloSHA256 + `  cr\rname.txt` + "\n",
		},
		{
			name:   "a file name holding a backslash is escaped",
			fsys:   testFS(),
			args:   []string{`back\slash.txt`},
			want:   `\` + helloSHA256 + `  back\\slash.txt` + "\n",
			skipOn: "windows",
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
			name:     "a file which isn't there",
			fsys:     testFS(),
			args:     []string{"missing.txt"},
			wantErr:  "sha256sum: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "the other operands are still written",
			fsys:     testFS(),
			args:     []string{"missing.txt", "hello.txt"},
			want:     helloSHA256 + "  hello.txt\n",
			wantErr:  "sha256sum: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "no standard input to read",
			wantErr:  "sha256sum: -: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "no file system to read from",
			args:     []string{"hello.txt"},
			wantErr:  "sha256sum: hello.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "the standard input which can't be read",
			stdin:    errorReader{},
			wantErr:  "sha256sum: -: boom\n",
			wantExit: 1,
		},
		{
			name:     "an option which isn't taken",
			fsys:     testFS(),
			args:     []string{"-q", "hello.txt"},
			wantErr:  "sha256sum: invalid option -- 'q'\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a long option which has no short equivalent",
			fsys:     testFS(),
			args:     []string{"-T", "hello.txt"},
			wantErr:  "sha256sum: invalid option -- 'T'\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a long option of another utility",
			fsys:     testFS(),
			args:     []string{"--raw", "hello.txt"},
			wantErr:  "sha256sum: unrecognized option '--raw'\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a long option given a value",
			fsys:     testFS(),
			args:     []string{"--tag=1", "hello.txt"},
			wantErr:  "sha256sum: option '--tag=1' doesn't allow an argument\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "a failing standard output",
			fsys:     testFS(),
			stdout:   errorWriter{},
			args:     []string{"hello.txt"},
			wantErr:  "sha256sum: write error: boom\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOptionConflicts(t *testing.T) {
	tests := []testCase{
		{
			name:     "zero while verifying",
			args:     []string{"-z", "-c", "sums.txt"},
			wantErr:  "sha256sum: the --zero option is not supported when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "tag while verifying",
			args:     []string{"--tag", "-c", "sums.txt"},
			wantErr:  "sha256sum: the --tag option is meaningless when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "binary while verifying",
			args:     []string{"-b", "-c", "sums.txt"},
			wantErr:  "sha256sum: the --binary and --text options are meaningless when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "text while verifying",
			args:     []string{"-c", "-t", "sums.txt"},
			wantErr:  "sha256sum: the --binary and --text options are meaningless when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "ignore-missing without verifying",
			args:     []string{"--ignore-missing", "hello.txt"},
			wantErr:  "sha256sum: the --ignore-missing option is meaningful only when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "status without verifying",
			args:     []string{"--status", "hello.txt"},
			wantErr:  "sha256sum: the --status option is meaningful only when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "warn without verifying",
			args:     []string{"-w", "hello.txt"},
			wantErr:  "sha256sum: the --warn option is meaningful only when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "quiet without verifying",
			args:     []string{"--quiet", "hello.txt"},
			wantErr:  "sha256sum: the --quiet option is meaningful only when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "strict without verifying",
			args:     []string{"--strict", "hello.txt"},
			wantErr:  "sha256sum: the --strict option is meaningful only when verifying checksums\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "tagging in text mode",
			args:     []string{"--tag", "-t", "hello.txt"},
			wantErr:  "sha256sum: --tag does not support --text mode\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
		{
			name:     "text mode before the tag option",
			args:     []string{"-t", "--tag", "hello.txt"},
			wantErr:  "sha256sum: --tag does not support --text mode\nTry 'sha256sum --help' for more information.\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunContext(t *testing.T) {
	t.Run("a context which is already done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		runCancelled(t, ctx, strings.NewReader("hello\n"), nil)
	})

	t.Run("a context which is done while reading", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runCancelled(t, ctx, &cancelReader{cancel: cancel}, nil)
	})

	t.Run("a context which is done while verifying", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runCancelled(t, ctx, &cancelReader{cancel: cancel}, []string{"-c"})
	})
}

// runCancelled runs the utility with a context that is either already done or
// becomes done while reading, and checks that the context error is returned
// without any diagnostic.
func runCancelled(t *testing.T, ctx context.Context, stdin io.Reader, args []string) {
	t.Helper()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		FS:     testFS(),
	}
	err := testSHA256.Run(ctx, rt, args)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if stdout.Len() > 0 {
		t.Errorf("Run() stdout = %q, want nothing", stdout.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("Run() stderr = %q, want nothing", stderr.String())
	}
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
				want := test.name + " (charm.land/snail/coreutils) " + coreutils.Version +
					"\nhttps://charm.land/snail\n"
				if got := stdout.String(); got != want {
					t.Errorf("Run() stdout = %q, want %q", got, want)
				}
			})
		})
	}
}
