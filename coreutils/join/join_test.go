package join

import (
	"bytes"
	"context"
	"errors"
	"io"
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

// cancelWriter cancels its context on the first write, simulating a context
// that is done while output is being written. It still records what is
// written after that.
type cancelWriter struct {
	cancel context.CancelFunc
	buf    bytes.Buffer
}

func (w *cancelWriter) Write(b []byte) (int, error) {
	w.cancel()
	return w.buf.Write(b)
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
	noFS     bool
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

// files holds the fixtures used by the tests:
//
//	f1.txt, f2.txt: joined on the first field
//	keys1.txt:      "1 a\n2 b\n", joined on the second field
//	keys2.txt:      "a x\nb y\n", joined on the first field
//	blanks1.txt:    "  a   1\nb 2\n", with blank separators
//	nonl1.txt:      "a 1" without a trailing newline
//	fa.txt, fb.txt: repeated keys, for the combinations
//	phone.txt:      the phone example of the specification
//	fax.txt:        the fax example of the specification
//	case1.txt, case2.txt: join fields differing in case, for -i
//	caserep1.txt, caserep2.txt: repeated join fields differing in case
var files = fsx.MemFS{
	"f1.txt":       {Data: []byte("a 1\nb 2\nc 3\n")},
	"f2.txt":       {Data: []byte("a x\nb y\nd z\n")},
	"keys1.txt":    {Data: []byte("1 a\n2 b\n")},
	"keys2.txt":    {Data: []byte("a x\nb y\n")},
	"blanks.txt":   {Data: []byte("  a   1\nb 2\n")},
	"nonl1.txt":    {Data: []byte("a 1")},
	"nonl2.txt":    {Data: []byte("a x")},
	"fa.txt":       {Data: []byte("a x\na y\na z\n")},
	"fb.txt":       {Data: []byte("a p\n")},
	"fa2.txt":      {Data: []byte("a b c\na d e\n")},
	"fb2.txt":      {Data: []byte("a w x\na y z\na o p\n")},
	"phone.txt":    {Data: []byte("Name\tPhone Number\nDon\t+1 123-456-7890\nHal\t+1 234-567-8901\nYasushi\t+2 345-678-9012\n")},
	"fax.txt":      {Data: []byte("Name\tFax Number\nDon\t+1 123-456-7899\nKeith\t+1 456-789-0122\nYasushi\t+2 345-678-9011\n")},
	"case1.txt":    {Data: []byte("A 1\nb 2\nc 3\n")},
	"case2.txt":    {Data: []byte("a x\nB y\nd z\n")},
	"caserep1.txt": {Data: []byte("a 1\nA 2\n")},
	"caserep2.txt": {Data: []byte("A p\nB q\n")},
}

func TestRunJoin(t *testing.T) {
	tests := []testCase{
		{
			name: "lines with equal join fields",
			args: []string{"f1.txt", "f2.txt"},
			want: "a 1 x\nb 2 y\n",
		},
		{
			name: "blank separators count as one",
			args: []string{"blanks.txt", "f2.txt"},
			want: "a 1 x\nb 2 y\n",
		},
		{
			name: "a separator given by -t",
			args: []string{"-t", ":", "colons1.txt", "colons2.txt"},
			want: "a:1:x\n",
		},
		{
			name: "a last line without a newline",
			args: []string{"nonl1.txt", "nonl2.txt"},
			want: "a 1 x\n",
		},
		{
			name: "join fields beside the first",
			args: []string{"-1", "2", "-2", "1", "keys1.txt", "keys2.txt"},
			want: "a 1 x\nb 2 y\n",
		},
		{
			name:  "the standard input as a file",
			stdin: strings.NewReader("a x\n"),
			args:  []string{"f1.txt", "-"},
			want:  "a 1 x\n",
		},
		{
			name: "combinations of repeated keys",
			args: []string{"fa.txt", "fb.txt"},
			want: "a x p\na y p\na z p\n",
		},
		{
			name: "combinations of repeated keys in both files",
			args: []string{"fa2.txt", "fb2.txt"},
			want: "a b c w x\na b c y z\na b c o p\na d e w x\na d e y z\na d e o p\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.fsys = withFiles(files)
			runCase(t, test)
		})
	}
}

func TestRunUnpaired(t *testing.T) {
	tests := []testCase{
		{
			name: "unpaired lines of file 1 with -a",
			args: []string{"-a", "1", "f1.txt", "f2.txt"},
			want: "a 1 x\nb 2 y\nc 3\n",
		},
		{
			name: "unpaired lines of file 2 with -a",
			args: []string{"-a", "2", "f1.txt", "f2.txt"},
			want: "a 1 x\nb 2 y\nd z\n",
		},
		{
			name: "all unpaired lines with -a 1 and -a 2",
			args: []string{"-a", "1", "-a", "2", "f1.txt", "f2.txt"},
			want: "a 1 x\nb 2 y\nc 3\nd z\n",
		},
		{
			name: "only unpaired lines with -v",
			args: []string{"-v", "1", "f1.txt", "f2.txt"},
			want: "c 3\n",
		},
		{
			name: "only unpaired lines of file 2 with -v",
			args: []string{"-v", "2", "f1.txt", "f2.txt"},
			want: "d z\n",
		},
		{
			name: "all unpaired lines with -v 1 and -v 2",
			args: []string{"-v", "1", "-v", "2", "f1.txt", "f2.txt"},
			want: "c 3\nd z\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.fsys = files
			runCase(t, test)
		})
	}
}

func TestRunIgnoreCase(t *testing.T) {
	tests := []testCase{
		{
			name: "join fields differing in case match with -i",
			args: []string{"-i", "case1.txt", "case2.txt"},
			want: "A 1 x\nb 2 y\n",
		},
		{
			name: "join fields differing in case match with --ignore-case",
			args: []string{"--ignore-case", "case1.txt", "case2.txt"},
			want: "A 1 x\nb 2 y\n",
		},
		{
			name: "join fields differing in case do not match without -i",
			args: []string{"case1.txt", "case2.txt"},
			want: "",
		},
		{
			name: "the join field is printed as it is in the input",
			args: []string{"-i", "-o", "0,1.2,2.2", "case1.txt", "case2.txt"},
			want: "A 1 x\nb 2 y\n",
		},
		{
			name: "unpaired lines are reported with -i",
			args: []string{"-i", "-a", "1", "-a", "2", "case1.txt", "case2.txt"},
			want: "A 1 x\nb 2 y\nc 3\nd z\n",
		},
		{
			name: "keys differing in case form one group",
			args: []string{"-i", "caserep1.txt", "caserep2.txt"},
			want: "a 1 p\nA 2 p\n",
		},
		{
			name: "keys differing in case form two groups without -i",
			args: []string{"caserep1.txt", "caserep2.txt"},
			want: "",
		},
		{
			name: "case-insensitive join fields beside the first",
			args: []string{"-i", "-1", "2", "-2", "1", "icasekeys1.txt", "icasekeys2.txt"},
			want: "A 1 x\nB 2 y\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.fsys = withFiles(files)
			runCase(t, test)
		})
	}
}

func TestRunOutputList(t *testing.T) {
	tests := []testCase{
		{
			name: "the join field and fields of both files",
			args: []string{"-o", "0,1.2,2.2", "f1.txt", "f2.txt"},
			want: "a 1 x\nb 2 y\n",
		},
		{
			name: "fields of the files only",
			args: []string{"-o", "1.2,2.2", "f1.txt", "f2.txt"},
			want: "1 x\n2 y\n",
		},
		{
			name: "a blank separated list",
			args: []string{"-o", "1.2 2.2", "f1.txt", "f2.txt"},
			want: "1 x\n2 y\n",
		},
		{
			name: "fields in any order",
			args: []string{"-o", "2.2,0,1.2", "f1.txt", "f2.txt"},
			want: "x a 1\ny b 2\n",
		},
		{
			name: "an empty leading field keeps its separator",
			args: []string{"-o", "1.3,1.1,1.2", "f1.txt", "f2.txt"},
			want: " a 1\n b 2\n",
		},
		{
			name: "empty fields keep their separators",
			args: []string{"-o", "1.3,1.3", "f1.txt", "f2.txt"},
			want: " \n \n",
		},
		{
			name: "fields past the line are empty",
			args: []string{"-o", "0,1.3,2.3", "f1.txt", "f2.txt"},
			want: "a  \nb  \n",
		},
		{
			name: "empty fields are replaced with -e",
			args: []string{"-e", "?", "-o", "0,1.3,2.3", "f1.txt", "f2.txt"},
			want: "a ? ?\nb ? ?\n",
		},
		{
			name: "empty fields of unpaired lines are replaced",
			args: []string{"-a", "1", "-e", "(unknown)", "-o", "0,1.2,2.2", "f1.txt", "f2.txt"},
			want: "a 1 x\nb 2 y\nc 3 (unknown)\n",
		},
		{
			name: "the join field is replaced too when empty",
			args: []string{"-e", "X", "-o", "0,1.2,2.2", "empty1.txt", "empty2.txt"},
			want: "X X X\n",
		},
		{
			name: "a separator given by -t",
			args: []string{"-t", ":", "-o", "0,1.2,2.2", "colons1.txt", "colons2.txt"},
			want: "a:1:x\n",
		},
		{
			name: "a multibyte separator given by -t",
			args: []string{"-t", "é", "accents1.txt", "accents2.txt"},
			want: "aé1éx\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.fsys = withFiles(files)
			runCase(t, test)
		})
	}
}

func TestRunSpecExample(t *testing.T) {
	t.Run("the phone and fax example", func(t *testing.T) {
		test := testCase{
			args: []string{
				"-t", "\t", "-a", "1", "-a", "2", "-e", "(unknown)",
				"-o", "0,1.2,2.2", "phone.txt", "fax.txt",
			},
			want: "Name\tPhone Number\tFax Number\n" +
				"Don\t+1 123-456-7890\t+1 123-456-7899\n" +
				"Hal\t+1 234-567-8901\t(unknown)\n" +
				"Keith\t(unknown)\t+1 456-789-0122\n" +
				"Yasushi\t+2 345-678-9012\t+2 345-678-9011\n",
		}
		test.fsys = files
		runCase(t, test)
	})
}

func TestRunErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "an unrecognized long option",
			args:     []string{"--wrong", "f1.txt", "f2.txt"},
			wantErr:  "join: unrecognized option '--wrong'\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid option",
			args:     []string{"-z", "f1.txt", "f2.txt"},
			wantErr:  "join: invalid option -- 'z'\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "no operands",
			args:     []string{},
			wantErr:  "join: missing operand\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a missing file2",
			args:     []string{"f1.txt"},
			wantErr:  "join: missing operand after \"f1.txt\"\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an extra operand",
			args:     []string{"f1.txt", "f2.txt", "f2.txt"},
			wantErr:  "join: extra operand \"f2.txt\"\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "both files are the standard input",
			args:     []string{"-", "-"},
			wantErr:  "join: both files are the standard input\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid file number",
			args:     []string{"-a", "3", "f1.txt", "f2.txt"},
			wantErr:  "join: invalid file number \"3\"\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid field number",
			args:     []string{"-1", "0", "f1.txt", "f2.txt"},
			wantErr:  "join: invalid field number \"0\"\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a signed field number",
			args:     []string{"-1", "+1", "f1.txt", "f2.txt"},
			wantErr:  "join: invalid field number \"+1\"\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid file number in a list",
			args:     []string{"-o", "3.1", "f1.txt", "f2.txt"},
			wantErr:  "join: invalid file number in \"3.1\"\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a signed file number in a list",
			args:     []string{"-o", "+1.2", "f1.txt", "f2.txt"},
			wantErr:  "join: invalid file number in \"+1.2\"\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a signed field number in a list",
			args:     []string{"-o", "1.+2", "f1.txt", "f2.txt"},
			wantErr:  "join: invalid field number in \"1.+2\"\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid field number in a list",
			args:     []string{"-o", "1.0", "f1.txt", "f2.txt"},
			wantErr:  "join: invalid field number in \"1.0\"\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an invalid field specifier",
			args:     []string{"-o", "x", "f1.txt", "f2.txt"},
			wantErr:  "join: invalid field specifier \"x\"\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an empty list",
			args:     []string{"-o", ",", "f1.txt", "f2.txt"},
			wantErr:  "join: empty field list\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an empty element in a list",
			args:     []string{"-o", "0,,1.2", "f1.txt", "f2.txt"},
			wantErr:  "join: empty field specifier\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a trailing comma in a list",
			args:     []string{"-o", "0,", "f1.txt", "f2.txt"},
			wantErr:  "join: empty field specifier\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "whitespace other than blanks in a list",
			args:     []string{"-o", "0,1.2\n", "f1.txt", "f2.txt"},
			wantErr:  "join: invalid field number in \"1.2\\n\"\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an empty separator",
			args:     []string{"-t", "", "f1.txt", "f2.txt"},
			wantErr:  "join: empty separator\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a separator which is not a single character",
			args:     []string{"-t", "::", "f1.txt", "f2.txt"},
			wantErr:  "join: separator \"::\" is not a single character\nTry 'join --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a missing file",
			args:     []string{"missing.txt", "f2.txt"},
			wantErr:  "join: missing.txt: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "no file system",
			noFS:     true,
			args:     []string{"f1.txt", "f2.txt"},
			wantErr:  "join: f1.txt: no file system available\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !test.noFS {
				test.fsys = files
			}
			runCase(t, test)
		})
	}
}

func TestRunIOErrors(t *testing.T) {
	t.Run("a write error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader("a x\n"),
			Stdout: errorWriter{err: errors.New("boom")},
			Stderr: new(bytes.Buffer),
			FS:     files,
		}
		err := New(rt).Run(t.Context(), "f1.txt", "-")
		if code := exitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
	})

	t.Run("a read error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  errorReader{err: errors.New("boom")},
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
			FS:     files,
		}
		err := New(rt).Run(t.Context(), "f1.txt", "-")
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
			FS:     files,
		}
		err := New(rt).Run(ctx, "f1.txt", "-")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})

	t.Run("a done context while writing the combinations of equal keys", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		rt := &coreutils.Runtime{
			Stdout: &cancelWriter{cancel: cancel},
			Stderr: new(bytes.Buffer),
			FS:     files,
		}
		err := New(rt).Run(ctx, "fa2.txt", "fb2.txt")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})

	t.Run("a done context while writing the unpaired lines left", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		stdout := &cancelWriter{cancel: cancel}
		rt := &coreutils.Runtime{
			Stdout: stdout,
			Stderr: new(bytes.Buffer),
			FS: fsx.MemFS{
				"left.txt":  {Data: []byte("a 1\nb 2\nc 3\nd 4\n")},
				"empty.txt": {Data: nil},
			},
		}
		err := New(rt).Run(ctx, "-a", "1", "left.txt", "empty.txt")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if got := stdout.buf.String(); got != "a 1\n" {
			t.Errorf("Run() wrote %q after the context was done, want %q", got, "a 1\n")
		}
	})
}

// withFiles returns a copy of the fixture files plus the ones the arguments
// of the test need, with their contents set for a single line join.
func withFiles(base fsx.MemFS, args ...string) fsx.MemFS {
	fsys := fsx.MemFS{}
	for name, f := range base {
		fsys[name] = &fsx.MemFile{Data: append([]byte(nil), f.Data...)}
	}
	fsys["colons1.txt"] = &fsx.MemFile{Data: []byte("a:1\n")}
	fsys["colons2.txt"] = &fsx.MemFile{Data: []byte("a:x\n")}
	fsys["empty1.txt"] = &fsx.MemFile{Data: []byte("\nfoo a\n")}
	fsys["empty2.txt"] = &fsx.MemFile{Data: []byte("\nbar b\n")}
	fsys["accents1.txt"] = &fsx.MemFile{Data: []byte("aé1\n")}
	fsys["accents2.txt"] = &fsx.MemFile{Data: []byte("aéx\n")}
	fsys["icasekeys1.txt"] = &fsx.MemFile{Data: []byte("1 A\n2 B\n")}
	fsys["icasekeys2.txt"] = &fsx.MemFile{Data: []byte("a x\nb y\n")}
	return fsys
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
		want := "join (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
