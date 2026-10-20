package sha384sum

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

// helloDigest is the SHA384 digest of "hello\n".
const helloDigest = "1d0f284efe3edea4b9ca3bd514fa134b17eae361ccc7a1eefeff801b9bd6604e01f21f6bf249ef030599f0c218f2ba8c"

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
			want: helloDigest + "  hello.txt\n",
		},
		{
			name:  "the digest of the standard input",
			stdin: strings.NewReader("hello\n"),
			want:  helloDigest + "  -\n",
		},
		{
			name: "binary mode",
			fsys: testFS(),
			args: []string{"-b", "hello.txt"},
			want: helloDigest + " *hello.txt\n",
		},
		{
			name: "a BSD style line",
			fsys: testFS(),
			args: []string{"--tag", "hello.txt"},
			want: "SHA384 (hello.txt) = " + helloDigest + "\n",
		},
		{
			name: "verifying a checksum",
			fsys: testFS(),
			args: []string{"-c", "sums.txt"},
			want: "hello.txt: OK\n",
		},
		{
			name:     "a checksum which doesn't match",
			fsys:     testFS(),
			args:     []string{"-c", "bad.txt"},
			want:     "hello.txt: FAILED\n",
			wantErr:  "sha384sum: WARNING: 1 computed checksum did NOT match\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}
