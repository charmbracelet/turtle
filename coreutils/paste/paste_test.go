package paste

import (
	"io/fs"
	"strings"
	"testing"

	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

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

func TestRunDefaults(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a single file is copied line by line",
			FS:   testFS(),
			Args: []string{"a.txt"},
			Want: "a1\na2\na3\n",
		},
		{
			Name: "two files of the same length",
			FS:   testFS(),
			Args: []string{"b.txt", "b.txt"},
			Want: "b1\tb1\nb2\tb2\n",
		},
		{
			Name: "an exhausted file contributes empty lines",
			FS:   testFS(),
			Args: []string{"a.txt", "b.txt"},
			Want: "a1\tb1\na2\tb2\na3\t\n",
		},
		{
			Name: "an exhausted first file contributes empty lines",
			FS:   testFS(),
			Args: []string{"b.txt", "a.txt"},
			Want: "b1\ta1\nb2\ta2\n\ta3\n",
		},
		{
			Name: "three files",
			FS:   testFS(),
			Args: []string{"a.txt", "b.txt", "c.txt"},
			Want: "a1\tb1\tc1\na2\tb2\tc2\na3\t\tc3\n\t\tc4\n",
		},
		{
			Name: "an empty file contributes empty lines",
			FS:   testFS(),
			Args: []string{"a.txt", "empty.txt"},
			Want: "a1\t\na2\t\na3\t\n",
		},
		{
			Name: "all files empty produces no output",
			FS:   testFS(),
			Args: []string{"empty.txt", "empty.txt"},
			Want: "",
		},
		{
			Name: "a missing final newline is added",
			FS:   testFS(),
			Args: []string{"nonl.txt"},
			Want: "x1\nx2\n",
		},
		{
			Name: "pathnames are resolved from the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"inner.txt"},
			Want: "inner\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunDelimiters(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a single delimiter replaces the tab",
			FS:   testFS(),
			Args: []string{"-d", ":", "a.txt", "b.txt"},
			Want: "a1:b1\na2:b2\na3:\n",
		},
		{
			Name: "an attached delimiter",
			FS:   testFS(),
			Args: []string{"-d:", "a.txt", "b.txt"},
			Want: "a1:b1\na2:b2\na3:\n",
		},
		{
			Name: "delimiters are used circularly within a line",
			FS:   testFS(),
			Args: []string{"-d", ",;", "a.txt", "b.txt", "c.txt"},
			Want: "a1,b1;c1\na2,b2;c2\na3,;c3\n,;c4\n",
		},
		{
			Name: "the delimiter is reset for every line",
			FS:   testFS(),
			Args: []string{"-d", ":", "a.txt", "empty.txt", "a.txt"},
			Want: "a1::a1\na2::a2\na3::a3\n",
		},
		{
			Name: "the newline escape",
			FS:   testFS(),
			Args: []string{"-d", `\n`, "a.txt", "b.txt"},
			Want: "a1\nb1\na2\nb2\na3\n\n",
		},
		{
			Name: "the tab escape",
			FS:   testFS(),
			Args: []string{"-d", `\t`, "a.txt", "b.txt"},
			Want: "a1\tb1\na2\tb2\na3\t\n",
		},
		{
			Name: "the backslash escape",
			FS:   testFS(),
			Args: []string{"-d", `\\`, "a.txt", "b.txt"},
			Want: `a1\b1` + "\n" + `a2\b2` + "\n" + `a3\` + "\n",
		},
		{
			Name: "the backspace escape",
			FS:   testFS(),
			Args: []string{"-d", `\b`, "a.txt", "b.txt"},
			Want: "a1\bb1\na2\bb2\na3\b\n",
		},
		{
			Name: "the form feed escape",
			FS:   testFS(),
			Args: []string{"-d", `\f`, "a.txt", "b.txt"},
			Want: "a1\fb1\na2\fb2\na3\f\n",
		},
		{
			Name: "the carriage return escape",
			FS:   testFS(),
			Args: []string{"-d", `\r`, "a.txt", "b.txt"},
			Want: "a1\rb1\na2\rb2\na3\r\n",
		},
		{
			Name: "the vertical tab escape",
			FS:   testFS(),
			Args: []string{"-d", `\v`, "a.txt", "b.txt"},
			Want: "a1\vb1\na2\vb2\na3\v\n",
		},
		{
			Name: "the empty string escape",
			FS:   testFS(),
			Args: []string{"-d", `\0`, "a.txt", "b.txt"},
			Want: "a1b1\na2b2\na3\n",
		},
		{
			Name: "an unknown escape stands for the character itself",
			FS:   testFS(),
			Args: []string{"-d", `\x`, "a.txt", "b.txt"},
			Want: "a1xb1\na2xb2\na3x\n",
		},
		{
			Name: "an empty list means no separator",
			FS:   testFS(),
			Args: []string{"-d", "", "a.txt", "b.txt"},
			Want: "a1b1\na2b2\na3\n",
		},
		{
			Name: "the last delimiter list wins",
			FS:   testFS(),
			Args: []string{"-d", ":", "-d", ";", "a.txt", "b.txt"},
			Want: "a1;b1\na2;b2\na3;\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunSerial(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "all the lines of a file in one output line",
			FS:   testFS(),
			Args: []string{"-s", "a.txt"},
			Want: "a1\ta2\ta3\n",
		},
		{
			Name: "one output line per file",
			FS:   testFS(),
			Args: []string{"-s", "a.txt", "b.txt"},
			Want: "a1\ta2\ta3\nb1\tb2\n",
		},
		{
			Name: "a custom delimiter",
			FS:   testFS(),
			Args: []string{"-s", "-d", ",", "a.txt"},
			Want: "a1,a2,a3\n",
		},
		{
			Name: "delimiters are used circularly",
			FS:   testFS(),
			Args: []string{"-s", "-d", ",;", "c.txt"},
			Want: "c1,c2;c3,c4\n",
		},
		{
			Name: "the delimiter is reset for every file",
			FS:   testFS(),
			Args: []string{"-s", "-d", ",;", "a.txt", "c.txt"},
			Want: "a1,a2;a3\nc1,c2;c3,c4\n",
		},
		{
			Name: "an empty file produces an empty line",
			FS:   testFS(),
			Args: []string{"-s", "a.txt", "empty.txt", "b.txt"},
			Want: "a1\ta2\ta3\n\nb1\tb2\n",
		},
		{
			Name: "a missing final newline is added",
			FS:   testFS(),
			Args: []string{"-s", "nonl.txt"},
			Want: "x1\tx2\n",
		},
		{
			Name: "the newline escape joins lines with newlines",
			FS:   testFS(),
			Args: []string{"-s", "-d", `\n`, "a.txt"},
			Want: "a1\na2\na3\n",
		},
		{
			Name: "the empty string escape concatenates the lines",
			FS:   testFS(),
			Args: []string{"-s", "-d", `\0`, "a.txt"},
			Want: "a1a2a3\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunZeroTerminated(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "a single file is copied item by item",
			FS:   fsx.MemFS{"n.txt": {Data: []byte("n1\x00n2\x00")}},
			Args: []string{"-z", "n.txt"},
			Want: "n1\x00n2\x00",
		},
		{
			Name: "two files of the same length",
			FS: fsx.MemFS{
				"x.txt": {Data: []byte("x1\x00x2\x00")},
				"y.txt": {Data: []byte("y1\x00y2\x00")},
			},
			Args: []string{"-z", "x.txt", "y.txt"},
			Want: "x1\ty1\x00x2\ty2\x00",
		},
		{
			Name: "an exhausted file contributes empty items",
			FS: fsx.MemFS{
				"x.txt": {Data: []byte("x1\x00x2\x00")},
				"y.txt": {Data: []byte("y1\x00")},
			},
			Args: []string{"-z", "x.txt", "y.txt"},
			Want: "x1\ty1\x00x2\t\x00",
		},
		{
			Name: "a missing final NUL is added",
			FS:   fsx.MemFS{"n.txt": {Data: []byte("n1\x00n2")}},
			Args: []string{"-z", "n.txt"},
			Want: "n1\x00n2\x00",
		},
		{
			Name: "newlines are kept inside items",
			FS:   fsx.MemFS{"n.txt": {Data: []byte("n1\nn2\x00n3\x00")}},
			Args: []string{"-z", "n.txt"},
			Want: "n1\nn2\x00n3\x00",
		},
		{
			Name: "a custom delimiter",
			FS: fsx.MemFS{
				"x.txt": {Data: []byte("x1\x00x2\x00")},
				"y.txt": {Data: []byte("y1\x00")},
			},
			Args: []string{"-z", "-d", ",", "x.txt", "y.txt"},
			Want: "x1,y1\x00x2,\x00",
		},
		{
			Name: "serial mode",
			FS:   fsx.MemFS{"n.txt": {Data: []byte("n1\x00n2\x00n3\x00")}},
			Args: []string{"-s", "-z", "n.txt"},
			Want: "n1\tn2\tn3\x00",
		},
		{
			Name: "serial mode with a missing final NUL",
			FS:   fsx.MemFS{"n.txt": {Data: []byte("n1\x00n2")}},
			Args: []string{"-s", "-z", "n.txt"},
			Want: "n1\tn2\x00",
		},
		{
			Name:  "the standard input",
			Stdin: strings.NewReader("s1\x00s2\x00"),
			Args:  []string{"-z", "-"},
			Want:  "s1\x00s2\x00",
		},
		{
			Name:  "the standard input is shared circularly by dashes",
			Stdin: strings.NewReader("1\x002\x003\x004\x00"),
			Args:  []string{"-z", "-", "-"},
			Want:  "1\t2\x003\t4\x00",
		},
		{
			Name: "an empty file produces no output",
			FS:   fsx.MemFS{"n.txt": {Data: []byte("")}},
			Args: []string{"-z", "n.txt"},
			Want: "",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunStandardInput(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "dash reads the standard input",
			Stdin: strings.NewReader("s1\ns2\n"),
			Args:  []string{"-"},
			Want:  "s1\ns2\n",
		},
		{
			Name:  "the standard input is shared circularly by dashes",
			Stdin: strings.NewReader("1\n2\n3\n4\n"),
			Args:  []string{"-", "-"},
			Want:  "1\t2\n3\t4\n",
		},
		{
			Name:  "dash combined with a file",
			Stdin: strings.NewReader("s1\ns2\ns3\n"),
			FS:    testFS(),
			Args:  []string{"a.txt", "-"},
			Want:  "a1\ts1\na2\ts2\na3\ts3\n",
		},
		{
			Name:  "dash between files",
			Stdin: strings.NewReader("s1\ns2\n"),
			FS:    testFS(),
			Args:  []string{"b.txt", "-", "c.txt"},
			Want:  "b1\ts1\tc1\nb2\ts2\tc2\n\t\tc3\n\t\tc4\n",
		},
		{
			Name:  "serial dash reads the whole standard input in one line",
			Stdin: strings.NewReader("s1\ns2\n"),
			Args:  []string{"-s", "-"},
			Want:  "s1\ts2\n",
		},
		{
			Name:  "serial dashes share the standard input",
			Stdin: strings.NewReader("s1\ns2\n"),
			Args:  []string{"-s", "-", "-"},
			Want:  "s1\ts2\n\n",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "missing file writes no output",
			FS:       testFS(),
			Args:     []string{"missing.txt"},
			WantErr:  "paste: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a missing file suppresses the output of the other files",
			FS:       testFS(),
			Args:     []string{"a.txt", "missing.txt"},
			WantErr:  "paste: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "serial mode keeps processing the other files",
			FS:       testFS(),
			Args:     []string{"-s", "missing.txt", "a.txt"},
			Want:     "a1\ta2\ta3\n",
			WantErr:  "paste: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "directory operand",
			FS:       fsx.MemFS{"dir": {Mode: fs.ModeDir}},
			Args:     []string{"dir"},
			WantErr:  "paste: dir: invalid argument\n",
			WantExit: 1,
		},
		{
			Name:     "zero-length file name",
			FS:       testFS(),
			Args:     []string{""},
			WantErr:  "paste: invalid zero-length file name\n",
			WantExit: 1,
		},
		{
			Name:     "file operand without a file system",
			Args:     []string{"a.txt"},
			WantErr:  "paste: a.txt: no file system available\n",
			WantExit: 1,
		},
		{
			Name:     "dash without a standard input",
			Args:     []string{"-"},
			WantErr:  "paste: standard input: no standard input available\n",
			WantExit: 1,
		},
		{
			Name:     "a delimiter list ending with a backslash",
			FS:       testFS(),
			Args:     []string{"-d", `\`, "a.txt"},
			WantErr:  "paste: delimiter list ends with an unescaped backslash: '\\'\nTry 'paste --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a delimiter list ending with a backslash after a character",
			FS:       testFS(),
			Args:     []string{"-d", `a\`, "a.txt"},
			WantErr:  "paste: delimiter list ends with an unescaped backslash: 'a\\'\nTry 'paste --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "missing operand",
			WantErr:  "paste: missing operand\nTry 'paste --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option",
			Args:     []string{"-x"},
			WantErr:  "paste: invalid option -- 'x'\nTry 'paste --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "invalid option in a group",
			Args:     []string{"-sx"},
			WantErr:  "paste: invalid option -- 'x'\nTry 'paste --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "unknown long option",
			Args:     []string{"--delimiter"},
			WantErr:  "paste: unrecognized option '--delimiter'\nTry 'paste --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "missing option argument",
			Args:     []string{"-d"},
			WantErr:  "paste: option requires an argument -- 'd'\nTry 'paste --help' for more information.\n",
			WantExit: 2,
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunLongOptions(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "delimiters",
			FS:   testFS(),
			Args: []string{"--delimiters", ":", "a.txt", "b.txt"},
			Want: "a1:b1\na2:b2\na3:\n",
		},
		{
			Name: "serial",
			FS:   testFS(),
			Args: []string{"--serial", "a.txt"},
			Want: "a1\ta2\ta3\n",
		},
		{
			Name: "zero-terminated",
			FS:   fsx.MemFS{"n.txt": {Data: []byte("n1\x00n2\x00")}},
			Args: []string{"--zero-terminated", "n.txt"},
			Want: "n1\x00n2\x00",
		},
	}

	testutil.Run(t, New, tests)
}

func TestRunContext(t *testing.T) {
	t.Run("cancelled before running", func(t *testing.T) {
		testutil.Cancelled(t, New, nil, testFS(), "a.txt")
	})

	t.Run("cancelled while reading the standard input", func(t *testing.T) {
		testutil.Cancelled(t, New, &testutil.CancelReader{}, nil, "-")
	})
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "paste", usage)
}
