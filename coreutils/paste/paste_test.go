package paste

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

// cancelReader cancels its context on the first read, simulating a context
// that is done while input is being read.
type cancelReader struct {
	cancel context.CancelFunc
}

func (r *cancelReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
}

// testFS holds the fixtures used by the tests:
//
//	a.txt:         "a1\na2\na3\n"
//	b.txt:         "b1\nb2\n"
//	c.txt:         "c1\nc2\nc3\nc4\n"
//	empty.txt:     ""
//	nonl.txt:      "x1\nx2" (no trailing newline)
//	sub/inner.txt: "inner\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"a.txt":         {Data: []byte("a1\na2\na3\n")},
		"b.txt":         {Data: []byte("b1\nb2\n")},
		"c.txt":         {Data: []byte("c1\nc2\nc3\nc4\n")},
		"empty.txt":     {Data: []byte("")},
		"nonl.txt":      {Data: []byte("x1\nx2")},
		"sub/inner.txt": {Data: []byte("inner\n")},
	}
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
	fsys     fsx.FS
	dir      string
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
		Stdin:      test.stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         test.fsys,
		WorkingDir: test.dir,
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

func TestRunDefaults(t *testing.T) {
	tests := []testCase{
		{
			name: "a single file is copied line by line",
			fsys: testFS(),
			args: []string{"a.txt"},
			want: "a1\na2\na3\n",
		},
		{
			name: "two files of the same length",
			fsys: testFS(),
			args: []string{"b.txt", "b.txt"},
			want: "b1\tb1\nb2\tb2\n",
		},
		{
			name: "an exhausted file contributes empty lines",
			fsys: testFS(),
			args: []string{"a.txt", "b.txt"},
			want: "a1\tb1\na2\tb2\na3\t\n",
		},
		{
			name: "an exhausted first file contributes empty lines",
			fsys: testFS(),
			args: []string{"b.txt", "a.txt"},
			want: "b1\ta1\nb2\ta2\n\ta3\n",
		},
		{
			name: "three files",
			fsys: testFS(),
			args: []string{"a.txt", "b.txt", "c.txt"},
			want: "a1\tb1\tc1\na2\tb2\tc2\na3\t\tc3\n\t\tc4\n",
		},
		{
			name: "an empty file contributes empty lines",
			fsys: testFS(),
			args: []string{"a.txt", "empty.txt"},
			want: "a1\t\na2\t\na3\t\n",
		},
		{
			name: "all files empty produces no output",
			fsys: testFS(),
			args: []string{"empty.txt", "empty.txt"},
			want: "",
		},
		{
			name: "a missing final newline is added",
			fsys: testFS(),
			args: []string{"nonl.txt"},
			want: "x1\nx2\n",
		},
		{
			name: "pathnames are resolved from the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"inner.txt"},
			want: "inner\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunDelimiters(t *testing.T) {
	tests := []testCase{
		{
			name: "a single delimiter replaces the tab",
			fsys: testFS(),
			args: []string{"-d", ":", "a.txt", "b.txt"},
			want: "a1:b1\na2:b2\na3:\n",
		},
		{
			name: "an attached delimiter",
			fsys: testFS(),
			args: []string{"-d:", "a.txt", "b.txt"},
			want: "a1:b1\na2:b2\na3:\n",
		},
		{
			name: "delimiters are used circularly within a line",
			fsys: testFS(),
			args: []string{"-d", ",;", "a.txt", "b.txt", "c.txt"},
			want: "a1,b1;c1\na2,b2;c2\na3,;c3\n,;c4\n",
		},
		{
			name: "the delimiter is reset for every line",
			fsys: testFS(),
			args: []string{"-d", ":", "a.txt", "empty.txt", "a.txt"},
			want: "a1::a1\na2::a2\na3::a3\n",
		},
		{
			name: "the newline escape",
			fsys: testFS(),
			args: []string{"-d", `\n`, "a.txt", "b.txt"},
			want: "a1\nb1\na2\nb2\na3\n\n",
		},
		{
			name: "the tab escape",
			fsys: testFS(),
			args: []string{"-d", `\t`, "a.txt", "b.txt"},
			want: "a1\tb1\na2\tb2\na3\t\n",
		},
		{
			name: "the backslash escape",
			fsys: testFS(),
			args: []string{"-d", `\\`, "a.txt", "b.txt"},
			want: `a1\b1` + "\n" + `a2\b2` + "\n" + `a3\` + "\n",
		},
		{
			name: "the backspace escape",
			fsys: testFS(),
			args: []string{"-d", `\b`, "a.txt", "b.txt"},
			want: "a1\bb1\na2\bb2\na3\b\n",
		},
		{
			name: "the form feed escape",
			fsys: testFS(),
			args: []string{"-d", `\f`, "a.txt", "b.txt"},
			want: "a1\fb1\na2\fb2\na3\f\n",
		},
		{
			name: "the carriage return escape",
			fsys: testFS(),
			args: []string{"-d", `\r`, "a.txt", "b.txt"},
			want: "a1\rb1\na2\rb2\na3\r\n",
		},
		{
			name: "the vertical tab escape",
			fsys: testFS(),
			args: []string{"-d", `\v`, "a.txt", "b.txt"},
			want: "a1\vb1\na2\vb2\na3\v\n",
		},
		{
			name: "the empty string escape",
			fsys: testFS(),
			args: []string{"-d", `\0`, "a.txt", "b.txt"},
			want: "a1b1\na2b2\na3\n",
		},
		{
			name: "an unknown escape stands for the character itself",
			fsys: testFS(),
			args: []string{"-d", `\x`, "a.txt", "b.txt"},
			want: "a1xb1\na2xb2\na3x\n",
		},
		{
			name: "an empty list means no separator",
			fsys: testFS(),
			args: []string{"-d", "", "a.txt", "b.txt"},
			want: "a1b1\na2b2\na3\n",
		},
		{
			name: "the last delimiter list wins",
			fsys: testFS(),
			args: []string{"-d", ":", "-d", ";", "a.txt", "b.txt"},
			want: "a1;b1\na2;b2\na3;\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSerial(t *testing.T) {
	tests := []testCase{
		{
			name: "all the lines of a file in one output line",
			fsys: testFS(),
			args: []string{"-s", "a.txt"},
			want: "a1\ta2\ta3\n",
		},
		{
			name: "one output line per file",
			fsys: testFS(),
			args: []string{"-s", "a.txt", "b.txt"},
			want: "a1\ta2\ta3\nb1\tb2\n",
		},
		{
			name: "a custom delimiter",
			fsys: testFS(),
			args: []string{"-s", "-d", ",", "a.txt"},
			want: "a1,a2,a3\n",
		},
		{
			name: "delimiters are used circularly",
			fsys: testFS(),
			args: []string{"-s", "-d", ",;", "c.txt"},
			want: "c1,c2;c3,c4\n",
		},
		{
			name: "the delimiter is reset for every file",
			fsys: testFS(),
			args: []string{"-s", "-d", ",;", "a.txt", "c.txt"},
			want: "a1,a2;a3\nc1,c2;c3,c4\n",
		},
		{
			name: "an empty file produces an empty line",
			fsys: testFS(),
			args: []string{"-s", "a.txt", "empty.txt", "b.txt"},
			want: "a1\ta2\ta3\n\nb1\tb2\n",
		},
		{
			name: "a missing final newline is added",
			fsys: testFS(),
			args: []string{"-s", "nonl.txt"},
			want: "x1\tx2\n",
		},
		{
			name: "the newline escape joins lines with newlines",
			fsys: testFS(),
			args: []string{"-s", "-d", `\n`, "a.txt"},
			want: "a1\na2\na3\n",
		},
		{
			name: "the empty string escape concatenates the lines",
			fsys: testFS(),
			args: []string{"-s", "-d", `\0`, "a.txt"},
			want: "a1a2a3\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunZeroTerminated(t *testing.T) {
	tests := []testCase{
		{
			name: "a single file is copied item by item",
			fsys: fsx.MemFS{"n.txt": {Data: []byte("n1\x00n2\x00")}},
			args: []string{"-z", "n.txt"},
			want: "n1\x00n2\x00",
		},
		{
			name: "two files of the same length",
			fsys: fsx.MemFS{
				"x.txt": {Data: []byte("x1\x00x2\x00")},
				"y.txt": {Data: []byte("y1\x00y2\x00")},
			},
			args: []string{"-z", "x.txt", "y.txt"},
			want: "x1\ty1\x00x2\ty2\x00",
		},
		{
			name: "an exhausted file contributes empty items",
			fsys: fsx.MemFS{
				"x.txt": {Data: []byte("x1\x00x2\x00")},
				"y.txt": {Data: []byte("y1\x00")},
			},
			args: []string{"-z", "x.txt", "y.txt"},
			want: "x1\ty1\x00x2\t\x00",
		},
		{
			name: "a missing final NUL is added",
			fsys: fsx.MemFS{"n.txt": {Data: []byte("n1\x00n2")}},
			args: []string{"-z", "n.txt"},
			want: "n1\x00n2\x00",
		},
		{
			name: "newlines are kept inside items",
			fsys: fsx.MemFS{"n.txt": {Data: []byte("n1\nn2\x00n3\x00")}},
			args: []string{"-z", "n.txt"},
			want: "n1\nn2\x00n3\x00",
		},
		{
			name: "a custom delimiter",
			fsys: fsx.MemFS{
				"x.txt": {Data: []byte("x1\x00x2\x00")},
				"y.txt": {Data: []byte("y1\x00")},
			},
			args: []string{"-z", "-d", ",", "x.txt", "y.txt"},
			want: "x1,y1\x00x2,\x00",
		},
		{
			name: "serial mode",
			fsys: fsx.MemFS{"n.txt": {Data: []byte("n1\x00n2\x00n3\x00")}},
			args: []string{"-s", "-z", "n.txt"},
			want: "n1\tn2\tn3\x00",
		},
		{
			name: "serial mode with a missing final NUL",
			fsys: fsx.MemFS{"n.txt": {Data: []byte("n1\x00n2")}},
			args: []string{"-s", "-z", "n.txt"},
			want: "n1\tn2\x00",
		},
		{
			name:  "the standard input",
			stdin: strings.NewReader("s1\x00s2\x00"),
			args:  []string{"-z", "-"},
			want:  "s1\x00s2\x00",
		},
		{
			name:  "the standard input is shared circularly by dashes",
			stdin: strings.NewReader("1\x002\x003\x004\x00"),
			args:  []string{"-z", "-", "-"},
			want:  "1\t2\x003\t4\x00",
		},
		{
			name: "an empty file produces no output",
			fsys: fsx.MemFS{"n.txt": {Data: []byte("")}},
			args: []string{"-z", "n.txt"},
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunStandardInput(t *testing.T) {
	tests := []testCase{
		{
			name:  "dash reads the standard input",
			stdin: strings.NewReader("s1\ns2\n"),
			args:  []string{"-"},
			want:  "s1\ns2\n",
		},
		{
			name:  "the standard input is shared circularly by dashes",
			stdin: strings.NewReader("1\n2\n3\n4\n"),
			args:  []string{"-", "-"},
			want:  "1\t2\n3\t4\n",
		},
		{
			name:  "dash combined with a file",
			stdin: strings.NewReader("s1\ns2\ns3\n"),
			fsys:  testFS(),
			args:  []string{"a.txt", "-"},
			want:  "a1\ts1\na2\ts2\na3\ts3\n",
		},
		{
			name:  "dash between files",
			stdin: strings.NewReader("s1\ns2\n"),
			fsys:  testFS(),
			args:  []string{"b.txt", "-", "c.txt"},
			want:  "b1\ts1\tc1\nb2\ts2\tc2\n\t\tc3\n\t\tc4\n",
		},
		{
			name:  "serial dash reads the whole standard input in one line",
			stdin: strings.NewReader("s1\ns2\n"),
			args:  []string{"-s", "-"},
			want:  "s1\ts2\n",
		},
		{
			name:  "serial dashes share the standard input",
			stdin: strings.NewReader("s1\ns2\n"),
			args:  []string{"-s", "-", "-"},
			want:  "s1\ts2\n\n",
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
			name:     "missing file writes no output",
			fsys:     testFS(),
			args:     []string{"missing.txt"},
			wantErr:  "paste: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "a missing file suppresses the output of the other files",
			fsys:     testFS(),
			args:     []string{"a.txt", "missing.txt"},
			wantErr:  "paste: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "serial mode keeps processing the other files",
			fsys:     testFS(),
			args:     []string{"-s", "missing.txt", "a.txt"},
			want:     "a1\ta2\ta3\n",
			wantErr:  "paste: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "directory operand",
			fsys:     fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			args:     []string{"dir"},
			wantErr:  "paste: dir: invalid argument\n",
			wantExit: 1,
		},
		{
			name:     "zero-length file name",
			fsys:     testFS(),
			args:     []string{""},
			wantErr:  "paste: invalid zero-length file name\n",
			wantExit: 1,
		},
		{
			name:     "file operand without a file system",
			args:     []string{"a.txt"},
			wantErr:  "paste: a.txt: no file system available\n",
			wantExit: 1,
		},
		{
			name:     "dash without a standard input",
			args:     []string{"-"},
			wantErr:  "paste: standard input: no standard input available\n",
			wantExit: 1,
		},
		{
			name:     "a delimiter list ending with a backslash",
			fsys:     testFS(),
			args:     []string{"-d", `\`, "a.txt"},
			wantErr:  "paste: delimiter list ends with an unescaped backslash: '\\'\n",
			wantExit: 1,
		},
		{
			name:     "a delimiter list ending with a backslash after a character",
			fsys:     testFS(),
			args:     []string{"-d", `a\`, "a.txt"},
			wantErr:  "paste: delimiter list ends with an unescaped backslash: 'a\\'\n",
			wantExit: 1,
		},
		{
			name:     "missing operand",
			wantErr:  "paste: missing operand\n",
			wantExit: 2,
		},
		{
			name:     "invalid option",
			args:     []string{"-x"},
			wantErr:  "paste: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "invalid option in a group",
			args:     []string{"-sx"},
			wantErr:  "paste: invalid option -- 'x'\n",
			wantExit: 2,
		},
		{
			name:     "unknown long option",
			args:     []string{"--delimiter"},
			wantErr:  "paste: unrecognized option '--delimiter'\n",
			wantExit: 2,
		},
		{
			name:     "missing option argument",
			args:     []string{"-d"},
			wantErr:  "paste: option requires an argument -- 'd'\n",
			wantExit: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLongOptions(t *testing.T) {
	tests := []testCase{
		{
			name: "delimiters",
			fsys: testFS(),
			args: []string{"--delimiters", ":", "a.txt", "b.txt"},
			want: "a1:b1\na2:b2\na3:\n",
		},
		{
			name: "serial",
			fsys: testFS(),
			args: []string{"--serial", "a.txt"},
			want: "a1\ta2\ta3\n",
		},
		{
			name: "zero-terminated",
			fsys: fsx.MemFS{"n.txt": {Data: []byte("n1\x00n2\x00")}},
			args: []string{"--zero-terminated", "n.txt"},
			want: "n1\x00n2\x00",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		runCancelled(t, ctx, nil, testFS())
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runCancelled(t, ctx, &cancelReader{cancel: cancel}, nil)
	})
}

// runCancelled runs the utility with a context that is either already done or
// becomes done while reading, and checks that the context error is returned
// without any output.
func runCancelled(t *testing.T, ctx context.Context, stdin io.Reader, fsys fsx.FS) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		FS:     fsys,
	}
	args := []string{"-"}
	if fsys != nil {
		args = []string{"a.txt"}
	}
	err := New(rt).Run(ctx, args...)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("Run() stderr = %q, want empty", got)
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
		want := "paste (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
