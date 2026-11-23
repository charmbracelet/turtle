package join

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

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
var files = fsx.MemoryFS{
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
	tests := []testutil.Case{
		{
			Name: "lines with equal join fields",
			Args: []string{"f1.txt", "f2.txt"},
			Want: "a 1 x\nb 2 y\n",
		},
		{
			Name: "blank separators count as one",
			Args: []string{"blanks.txt", "f2.txt"},
			Want: "a 1 x\nb 2 y\n",
		},
		{
			Name: "a separator given by -t",
			Args: []string{"-t", ":", "colons1.txt", "colons2.txt"},
			Want: "a:1:x\n",
		},
		{
			Name: "a last line without a newline",
			Args: []string{"nonl1.txt", "nonl2.txt"},
			Want: "a 1 x\n",
		},
		{
			Name: "join fields beside the first",
			Args: []string{"-1", "2", "-2", "1", "keys1.txt", "keys2.txt"},
			Want: "a 1 x\nb 2 y\n",
		},
		{
			Name:  "the standard input as a file",
			Stdin: strings.NewReader("a x\n"),
			Args:  []string{"f1.txt", "-"},
			Want:  "a 1 x\n",
		},
		{
			Name: "combinations of repeated keys",
			Args: []string{"fa.txt", "fb.txt"},
			Want: "a x p\na y p\na z p\n",
		},
		{
			Name: "combinations of repeated keys in both files",
			Args: []string{"fa2.txt", "fb2.txt"},
			Want: "a b c w x\na b c y z\na b c o p\na d e w x\na d e y z\na d e o p\n",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			test.FS = withFiles(files)
			testutil.Once(t, New, test)
		})
	}
}

func TestRunUnpaired(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "unpaired lines of file 1 with -a",
			Args: []string{"-a", "1", "f1.txt", "f2.txt"},
			Want: "a 1 x\nb 2 y\nc 3\n",
		},
		{
			Name: "unpaired lines of file 2 with -a",
			Args: []string{"-a", "2", "f1.txt", "f2.txt"},
			Want: "a 1 x\nb 2 y\nd z\n",
		},
		{
			Name: "all unpaired lines with -a 1 and -a 2",
			Args: []string{"-a", "1", "-a", "2", "f1.txt", "f2.txt"},
			Want: "a 1 x\nb 2 y\nc 3\nd z\n",
		},
		{
			Name: "only unpaired lines with -v",
			Args: []string{"-v", "1", "f1.txt", "f2.txt"},
			Want: "c 3\n",
		},
		{
			Name: "only unpaired lines of file 2 with -v",
			Args: []string{"-v", "2", "f1.txt", "f2.txt"},
			Want: "d z\n",
		},
		{
			Name: "all unpaired lines with -v 1 and -v 2",
			Args: []string{"-v", "1", "-v", "2", "f1.txt", "f2.txt"},
			Want: "c 3\nd z\n",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			test.FS = files
			testutil.Once(t, New, test)
		})
	}
}

func TestRunIgnoreCase(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "join fields differing in case match with -i",
			Args: []string{"-i", "case1.txt", "case2.txt"},
			Want: "A 1 x\nb 2 y\n",
		},
		{
			Name: "join fields differing in case match with --ignore-case",
			Args: []string{"--ignore-case", "case1.txt", "case2.txt"},
			Want: "A 1 x\nb 2 y\n",
		},
		{
			Name: "join fields differing in case do not match without -i",
			Args: []string{"case1.txt", "case2.txt"},
			Want: "",
		},
		{
			Name: "the join field is printed as it is in the input",
			Args: []string{"-i", "-o", "0,1.2,2.2", "case1.txt", "case2.txt"},
			Want: "A 1 x\nb 2 y\n",
		},
		{
			Name: "unpaired lines are reported with -i",
			Args: []string{"-i", "-a", "1", "-a", "2", "case1.txt", "case2.txt"},
			Want: "A 1 x\nb 2 y\nc 3\nd z\n",
		},
		{
			Name: "keys differing in case form one group",
			Args: []string{"-i", "caserep1.txt", "caserep2.txt"},
			Want: "a 1 p\nA 2 p\n",
		},
		{
			Name: "keys differing in case form two groups without -i",
			Args: []string{"caserep1.txt", "caserep2.txt"},
			Want: "",
		},
		{
			Name: "case-insensitive join fields beside the first",
			Args: []string{"-i", "-1", "2", "-2", "1", "icasekeys1.txt", "icasekeys2.txt"},
			Want: "A 1 x\nB 2 y\n",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			test.FS = withFiles(files)
			testutil.Once(t, New, test)
		})
	}
}

func TestRunOutputList(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "the join field and fields of both files",
			Args: []string{"-o", "0,1.2,2.2", "f1.txt", "f2.txt"},
			Want: "a 1 x\nb 2 y\n",
		},
		{
			Name: "fields of the files only",
			Args: []string{"-o", "1.2,2.2", "f1.txt", "f2.txt"},
			Want: "1 x\n2 y\n",
		},
		{
			Name: "a blank separated list",
			Args: []string{"-o", "1.2 2.2", "f1.txt", "f2.txt"},
			Want: "1 x\n2 y\n",
		},
		{
			Name: "fields in any order",
			Args: []string{"-o", "2.2,0,1.2", "f1.txt", "f2.txt"},
			Want: "x a 1\ny b 2\n",
		},
		{
			Name: "an empty leading field keeps its separator",
			Args: []string{"-o", "1.3,1.1,1.2", "f1.txt", "f2.txt"},
			Want: " a 1\n b 2\n",
		},
		{
			Name: "empty fields keep their separators",
			Args: []string{"-o", "1.3,1.3", "f1.txt", "f2.txt"},
			Want: " \n \n",
		},
		{
			Name: "fields past the line are empty",
			Args: []string{"-o", "0,1.3,2.3", "f1.txt", "f2.txt"},
			Want: "a  \nb  \n",
		},
		{
			Name: "empty fields are replaced with -e",
			Args: []string{"-e", "?", "-o", "0,1.3,2.3", "f1.txt", "f2.txt"},
			Want: "a ? ?\nb ? ?\n",
		},
		{
			Name: "empty fields of unpaired lines are replaced",
			Args: []string{"-a", "1", "-e", "(unknown)", "-o", "0,1.2,2.2", "f1.txt", "f2.txt"},
			Want: "a 1 x\nb 2 y\nc 3 (unknown)\n",
		},
		{
			Name: "the join field is replaced too when empty",
			Args: []string{"-e", "X", "-o", "0,1.2,2.2", "empty1.txt", "empty2.txt"},
			Want: "X X X\n",
		},
		{
			Name: "a separator given by -t",
			Args: []string{"-t", ":", "-o", "0,1.2,2.2", "colons1.txt", "colons2.txt"},
			Want: "a:1:x\n",
		},
		{
			Name: "a multibyte separator given by -t",
			Args: []string{"-t", "é", "accents1.txt", "accents2.txt"},
			Want: "aé1éx\n",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			test.FS = withFiles(files)
			testutil.Once(t, New, test)
		})
	}
}

func TestRunSpecExample(t *testing.T) {
	t.Run("the phone and fax example", func(t *testing.T) {
		test := testutil.Case{
			Args: []string{
				"-t", "\t", "-a", "1", "-a", "2", "-e", "(unknown)",
				"-o", "0,1.2,2.2", "phone.txt", "fax.txt",
			},
			Want: "Name\tPhone Number\tFax Number\n" +
				"Don\t+1 123-456-7890\t+1 123-456-7899\n" +
				"Hal\t+1 234-567-8901\t(unknown)\n" +
				"Keith\t(unknown)\t+1 456-789-0122\n" +
				"Yasushi\t+2 345-678-9012\t+2 345-678-9011\n",
		}
		test.FS = files
		testutil.Once(t, New, test)
	})
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "an unrecognized long option",
			Args:     []string{"--wrong", "f1.txt", "f2.txt"},
			WantErr:  "join: unrecognized option '--wrong'\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid option",
			Args:     []string{"-z", "f1.txt", "f2.txt"},
			WantErr:  "join: invalid option -- 'z'\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "no operands",
			Args:     []string{},
			WantErr:  "join: missing operand\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file2",
			Args:     []string{"f1.txt"},
			WantErr:  "join: missing operand after \"f1.txt\"\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an extra operand",
			Args:     []string{"f1.txt", "f2.txt", "f2.txt"},
			WantErr:  "join: extra operand \"f2.txt\"\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "both files are the standard input",
			Args:     []string{"-", "-"},
			WantErr:  "join: both files are the standard input\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid file number",
			Args:     []string{"-a", "3", "f1.txt", "f2.txt"},
			WantErr:  "join: invalid file number \"3\"\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid field number",
			Args:     []string{"-1", "0", "f1.txt", "f2.txt"},
			WantErr:  "join: invalid field number \"0\"\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a signed field number",
			Args:     []string{"-1", "+1", "f1.txt", "f2.txt"},
			WantErr:  "join: invalid field number \"+1\"\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid file number in a list",
			Args:     []string{"-o", "3.1", "f1.txt", "f2.txt"},
			WantErr:  "join: invalid file number in \"3.1\"\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a signed file number in a list",
			Args:     []string{"-o", "+1.2", "f1.txt", "f2.txt"},
			WantErr:  "join: invalid file number in \"+1.2\"\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a signed field number in a list",
			Args:     []string{"-o", "1.+2", "f1.txt", "f2.txt"},
			WantErr:  "join: invalid field number in \"1.+2\"\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid field number in a list",
			Args:     []string{"-o", "1.0", "f1.txt", "f2.txt"},
			WantErr:  "join: invalid field number in \"1.0\"\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid field specifier",
			Args:     []string{"-o", "x", "f1.txt", "f2.txt"},
			WantErr:  "join: invalid field specifier \"x\"\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an empty list",
			Args:     []string{"-o", ",", "f1.txt", "f2.txt"},
			WantErr:  "join: empty field list\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an empty element in a list",
			Args:     []string{"-o", "0,,1.2", "f1.txt", "f2.txt"},
			WantErr:  "join: empty field specifier\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a trailing comma in a list",
			Args:     []string{"-o", "0,", "f1.txt", "f2.txt"},
			WantErr:  "join: empty field specifier\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "whitespace other than blanks in a list",
			Args:     []string{"-o", "0,1.2\n", "f1.txt", "f2.txt"},
			WantErr:  "join: invalid field number in \"1.2\\n\"\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an empty separator",
			Args:     []string{"-t", "", "f1.txt", "f2.txt"},
			WantErr:  "join: empty separator\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a separator which is not a single character",
			FS:       files,
			Args:     []string{"-t", "::", "f1.txt", "f2.txt"},
			WantErr:  "join: separator \"::\" is not a single character\nTry 'join --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file",
			FS:       files,
			Args:     []string{"missing.txt", "f2.txt"},
			WantErr:  "join: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "no file system",
			Args:     []string{"f1.txt", "f2.txt"},
			WantErr:  "join: f1.txt: no file system available\n",
			WantExit: 1,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunIOErrors(t *testing.T) {
	t.Run("a write error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  strings.NewReader("a x\n"),
			Stdout: testutil.ErrorWriter{Err: errors.New("boom")},
			Stderr: new(bytes.Buffer),
			FS:     files,
		}
		err := New(rt).Run(t.Context(), "f1.txt", "-")
		if code := testutil.ExitCode(t, err); code != 1 {
			t.Errorf("Run() exit code = %d, want 1", code)
		}
	})

	t.Run("a read error", func(t *testing.T) {
		rt := &coreutils.Runtime{
			Stdin:  testutil.ErrorReader{Err: errors.New("boom")},
			Stdout: new(bytes.Buffer),
			Stderr: new(bytes.Buffer),
			FS:     files,
		}
		err := New(rt).Run(t.Context(), "f1.txt", "-")
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
			FS: fsx.MemoryFS{
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
func withFiles(base fsx.MemoryFS, args ...string) fsx.MemoryFS {
	fsys := fsx.MemoryFS{}
	for name, f := range base {
		fsys[name] = &fsx.MemoryFile{Data: append([]byte(nil), f.Data...)}
	}
	fsys["colons1.txt"] = &fsx.MemoryFile{Data: []byte("a:1\n")}
	fsys["colons2.txt"] = &fsx.MemoryFile{Data: []byte("a:x\n")}
	fsys["empty1.txt"] = &fsx.MemoryFile{Data: []byte("\nfoo a\n")}
	fsys["empty2.txt"] = &fsx.MemoryFile{Data: []byte("\nbar b\n")}
	fsys["accents1.txt"] = &fsx.MemoryFile{Data: []byte("aé1\n")}
	fsys["accents2.txt"] = &fsx.MemoryFile{Data: []byte("aéx\n")}
	fsys["icasekeys1.txt"] = &fsx.MemoryFile{Data: []byte("1 A\n2 B\n")}
	fsys["icasekeys2.txt"] = &fsx.MemoryFile{Data: []byte("a x\nb y\n")}
	return fsys
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "join", usage)
}
