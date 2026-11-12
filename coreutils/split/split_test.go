package split

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/testutil"
	"charm.land/snail/fsx"
)

// testCase is a testutil.Case plus the exact list of the files the run must
// leave in the file system of the case, when the file names alone matter.
type testCase struct {
	Name      string
	Stdin     io.Reader
	FS        fsx.FS
	Args      []string
	Want      string
	WantErr   string
	WantExit  int
	WantFiles map[string]string

	// wantFiles is the exact list of the files that must be left behind,
	// overriding the names of WantFiles.
	wantFiles []string
}

func lines(from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&b, "l%d\n", i)
	}
	return b.String()
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	if test.FS == nil {
		test.FS = fsx.MemFS{}
	}
	want := test.wantFiles
	if want == nil {
		want = make([]string, 0, len(test.WantFiles))
		for name := range test.WantFiles {
			want = append(want, name)
		}
		sort.Strings(want)
	}
	check := func(t *testing.T, rt *coreutils.Runtime) {
		mem, ok := rt.FS.(fsx.MemFS)
		if !ok {
			return
		}
		var got []string
		for name := range mem {
			got = append(got, name)
		}
		sort.Strings(got)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("Run() files = %v, want %v", got, want)
		}
	}
	testutil.Once(t, New, testutil.Case{
		Name:      test.Name,
		Stdin:     test.Stdin,
		FS:        test.FS,
		Args:      test.Args,
		Want:      test.Want,
		WantErr:   test.WantErr,
		WantExit:  test.WantExit,
		WantFiles: test.WantFiles,
		Check:     check,
	})
}

func TestRunLines(t *testing.T) {
	tests := []testCase{
		{
			Name:      "default is 1000 lines per piece",
			FS:        fsx.MemFS{"in.txt": {Data: []byte(lines(1, 2500))}},
			Args:      []string{"in.txt"},
			wantFiles: []string{"in.txt", "xaa", "xab", "xac"},
			WantFiles: map[string]string{
				"xaa": lines(1, 1000),
				"xab": lines(1001, 2000),
				"xac": lines(2001, 2500),
			},
		},
		{
			Name:  "pieces of two lines",
			Stdin: strings.NewReader("a\nb\nc\nd\ne\n"),
			Args:  []string{"-l", "2"},
			WantFiles: map[string]string{
				"xaa": "a\nb\n",
				"xab": "c\nd\n",
				"xac": "e\n",
			},
		},
		{
			Name:  "an input ending at a boundary leaves no empty piece",
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			Args:  []string{"-l2"},
			WantFiles: map[string]string{
				"xaa": "a\nb\n",
				"xab": "c\nd\n",
			},
		},
		{
			Name:  "the last line may miss its newline",
			Stdin: strings.NewReader("a\nb\nc"),
			Args:  []string{"-l", "2"},
			WantFiles: map[string]string{
				"xaa": "a\nb\n",
				"xab": "c",
			},
		},
		{
			Name:  "custom prefix",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-l", "2", "-", "part_"},
			WantFiles: map[string]string{
				"part_aa": "a\nb\n",
				"part_ab": "c\n",
			},
		},
		{
			Name:  "longer suffixes",
			Stdin: strings.NewReader("a\nb\nc\n"),
			Args:  []string{"-l", "1", "-a3"},
			WantFiles: map[string]string{
				"xaaa": "a\n",
				"xaab": "b\n",
				"xaac": "c\n",
			},
		},
		{
			Name:      "empty input creates no files",
			FS:        fsx.MemFS{"empty.txt": {Data: []byte("")}},
			Args:      []string{"empty.txt"},
			wantFiles: []string{"empty.txt"},
		},
		{
			Name:      "empty input creates no files even with zero suffix length",
			FS:        fsx.MemFS{"empty.txt": {Data: []byte("")}},
			Args:      []string{"-a", "0", "empty.txt"},
			wantFiles: []string{"empty.txt"},
		},
		{
			Name:      "zero suffix length selects the default",
			Stdin:     strings.NewReader("a\nb\n"),
			Args:      []string{"-a", "0", "-l", "9"},
			WantFiles: map[string]string{"xaa": "a\nb\n"},
		},
		{
			Name:      "pieces in a subdirectory prefix",
			FS:        fsx.MemFS{"in.txt": {Data: []byte("a\nb\nc\n")}, "out/keep.txt": {Data: []byte("k")}},
			Args:      []string{"-l", "1", "in.txt", "out/p_"},
			wantFiles: []string{"in.txt", "out/keep.txt", "out/p_aa", "out/p_ab", "out/p_ac"},
			WantFiles: map[string]string{
				"out/p_aa": "a\n",
				"out/p_ab": "b\n",
				"out/p_ac": "c\n",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunBytes(t *testing.T) {
	tests := []testCase{
		{
			Name:  "pieces of two bytes",
			Stdin: strings.NewReader("abcde"),
			Args:  []string{"-b", "2"},
			WantFiles: map[string]string{
				"xaa": "ab",
				"xab": "cd",
				"xac": "e",
			},
		},
		{
			Name:  "a trailing k multiplies by 1024",
			Stdin: strings.NewReader(strings.Repeat("z", 3000)),
			Args:  []string{"-b", "1k"},
			WantFiles: map[string]string{
				"xaa": strings.Repeat("z", 1024),
				"xab": strings.Repeat("z", 1024),
				"xac": strings.Repeat("z", 952),
			},
		},
		{
			Name:  "a trailing m multiplies by 1048576",
			Stdin: strings.NewReader(strings.Repeat("z", 1048576+10)),
			Args:  []string{"-b1m"},
			WantFiles: map[string]string{
				"xaa": strings.Repeat("z", 1048576),
				"xab": strings.Repeat("z", 10),
			},
		},
		{
			Name:  "byte pieces may split lines",
			Stdin: strings.NewReader("ab\ncd\nef"),
			Args:  []string{"-b", "4"},
			WantFiles: map[string]string{
				"xaa": "ab\nc",
				"xab": "d\nef",
			},
		},
		{
			Name:      "an input ending at a boundary leaves no empty piece",
			Stdin:     strings.NewReader("abcd"),
			Args:      []string{"-b", "4"},
			WantFiles: map[string]string{"xaa": "abcd"},
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSuffixExhaustion(t *testing.T) {
	files := fsx.MemFS{"in.txt": {Data: []byte(lines(1, 30))}}
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
		Stderr: stderr,
		FS:     files,
	}
	err := New(rt).Run(t.Context(), "-a", "1", "-l", "1", "in.txt")
	exitErr, ok := errors.AsType[*coreutils.ExitError](err)
	if !ok || exitErr.Code != 1 {
		t.Fatalf("Run() error = %v, want exit status 1", err)
	}
	if got, want := stderr.String(), "split: output file suffixes exhausted\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
	var names []string
	for name := range files {
		if strings.HasPrefix(name, "x") {
			names = append(names, name)
		}
	}
	if len(names) != 26 {
		t.Errorf("Run() created %d pieces, want 26", len(names))
	}
	if got := string(files["xz"].Data); got != "l26\n" {
		t.Errorf("Run() xz = %q, want %q", got, "l26\n")
	}
}

func TestRunInvalidUsage(t *testing.T) {
	long := strings.Repeat("n", 254)
	tests := []testCase{
		{
			Name:     "unknown option",
			Args:     []string{"-q"},
			WantErr:  "split: invalid option -- 'q'\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "byte and line modes conflict",
			Args:     []string{"-b", "1", "-l", "1"},
			WantErr:  "split: cannot split in more than one way\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "zero lines is invalid",
			Args:     []string{"-l", "0"},
			WantErr:  "split: invalid number of lines: '0'\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a non numeric line count is invalid",
			Args:     []string{"-l", "1x"},
			WantErr:  "split: invalid number of lines: '1x'\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "zero bytes is invalid",
			Args:     []string{"-b", "0"},
			WantErr:  "split: invalid number of bytes: '0'\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an unknown multiplier is invalid",
			Args:     []string{"-b", "1j"},
			WantErr:  "split: invalid number of bytes: '1j'\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a non numeric suffix length is invalid",
			Args:     []string{"-a", "x"},
			WantErr:  "split: invalid suffix length: 'x'\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a negative suffix length is invalid",
			Args:     []string{"-a", "-1"},
			WantErr:  "split: invalid suffix length: '-1'\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "extra operand",
			Args:     []string{"a", "b", "c"},
			WantErr:  "split: extra operand 'c'\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a prefix that would exceed NAME_MAX fails before any file",
			Args:     []string{"-l", "1", "-", long},
			Stdin:    strings.NewReader("a\n"),
			WantErr:  "split: " + long + "aa: file name too long\n",
			WantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrors(t *testing.T) {
	tests := []testCase{
		{
			Name:     "missing input file",
			FS:       fsx.MemFS{},
			Args:     []string{"missing.txt"},
			WantErr:  "split: missing.txt: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("boom")},
			Args:     []string{"-l", "1"},
			WantErr:  "split: -: boom\n",
			WantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}

	t.Run("write error", func(t *testing.T) {
		runCase(t, testCase{
			FS:       testutil.FailWriteFS{FS: fsx.MemFS{}, Err: errors.New("disk full")},
			Stdin:    strings.NewReader("a\n"),
			Args:     []string{"-l", "1"},
			WantErr:  "split: xaa: disk full\n",
			WantExit: 1,
		})
	})
}

func TestRunCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	prog := New(&coreutils.Runtime{
		Stdin:  &testutil.CancelReader{Cancel: cancel},
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     fsx.MemFS{},
	})
	if err := prog.Run(ctx, "-l", "1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRunNumericSuffixes(t *testing.T) {
	tests := []testCase{
		{
			Name:  "digits replace letters",
			Stdin: strings.NewReader(lines(1, 3)),
			Args:  []string{"-d", "-l", "1"},
			WantFiles: map[string]string{
				"x00": lines(1, 1),
				"x01": lines(2, 2),
				"x02": lines(3, 3),
			},
		},
		{
			Name:  "the long form works",
			Stdin: strings.NewReader(lines(1, 2)),
			Args:  []string{"--numeric-suffixes", "-l", "1"},
			WantFiles: map[string]string{
				"x00": lines(1, 1),
				"x01": lines(2, 2),
			},
		},
		{
			Name:  "a starting number counts from it",
			Stdin: strings.NewReader(lines(1, 2)),
			Args:  []string{"-d", "-l", "1"},
			WantFiles: map[string]string{
				"x00": lines(1, 1),
				"x01": lines(2, 2),
			},
		},
		{
			Name:  "an attached starting number counts from it",
			Stdin: strings.NewReader(lines(1, 2)),
			Args:  []string{"--numeric-suffixes=4", "-l", "1"},
			WantFiles: map[string]string{
				"x04": lines(1, 1),
				"x05": lines(2, 2),
			},
		},
		{
			Name:  "hexadecimal suffixes",
			Stdin: strings.NewReader(lines(1, 2)),
			Args:  []string{"-x", "-l", "1"},
			WantFiles: map[string]string{
				"x00": lines(1, 1),
				"x01": lines(2, 2),
			},
		},
		{
			Name:  "an additional suffix is appended",
			Stdin: strings.NewReader(lines(1, 2)),
			Args:  []string{"-d", "--additional-suffix=.part", "-l", "1"},
			WantFiles: map[string]string{
				"x00.part": lines(1, 1),
				"x01.part": lines(2, 2),
			},
		},
		{
			Name:     "numeric and hexadecimal conflict",
			Args:     []string{"-d", "-x", "-l", "1"},
			WantErr:  "split: cannot combine -d and -x\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "an additional suffix with a slash is invalid",
			Args:     []string{"--additional-suffix=a/b", "-l", "1"},
			WantErr:  "split: additional suffix contains slash: 'a/b'\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunLineBytes(t *testing.T) {
	tests := []testCase{
		{
			Name:  "as many whole lines as fit",
			Stdin: strings.NewReader("ab\ncd\nef\n"),
			Args:  []string{"-C", "6"},
			WantFiles: map[string]string{
				"xaa": "ab\ncd\n",
				"xab": "ef\n",
			},
		},
		{
			Name:  "a line longer than the size is broken",
			Stdin: strings.NewReader("abcdef\ngh\n"),
			Args:  []string{"-C", "3"},
			WantFiles: map[string]string{
				"xaa": "abc",
				"xab": "def",
				"xac": "\n",
				"xad": "gh\n",
			},
		},
		{
			Name:  "the long form works",
			Stdin: strings.NewReader("ab\ncd\n"),
			Args:  []string{"--line-bytes=4"},
			WantFiles: map[string]string{
				"xaa": "ab\n",
				"xab": "cd\n",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunNumber(t *testing.T) {
	tests := []testCase{
		{
			Name:  "byte chunks are equal, early ones longer",
			Stdin: strings.NewReader("abcdefgh"),
			Args:  []string{"-n", "3"},
			WantFiles: map[string]string{
				"xaa": "abc",
				"xab": "def",
				"xac": "gh",
			},
		},
		{
			Name:  "byte chunks with a remainder",
			Stdin: strings.NewReader("abcde"),
			Args:  []string{"-n", "2"},
			WantFiles: map[string]string{
				"xaa": "abc",
				"xab": "de",
			},
		},
		{
			Name:  "the kth byte chunk goes to the standard output",
			Stdin: strings.NewReader("abcdefgh"),
			Args:  []string{"-n", "2/3"},
			Want:  "def",
		},
		{
			Name:  "line chunks hold the lines which start in them",
			Stdin: strings.NewReader("ab\ncd\nef\ngh\n"),
			Args:  []string{"-n", "l/2"},
			WantFiles: map[string]string{
				"xaa": "ab\ncd\n",
				"xab": "ef\ngh\n",
			},
		},
		{
			Name:  "the kth line chunk goes to the standard output",
			Stdin: strings.NewReader("ab\ncd\nef\ngh\n"),
			Args:  []string{"-n", "l/2/3"},
			Want:  "ef\n",
		},
		{
			Name:  "a region with no line starting in it is empty",
			Stdin: strings.NewReader("abcdefgh\n"),
			Args:  []string{"-n", "l/3"},
			WantFiles: map[string]string{
				"xaa": "abcdefgh\n",
				"xab": "",
				"xac": "",
			},
		},
		{
			Name:  "-e elides the empty pieces",
			Stdin: strings.NewReader("abcdefgh\n"),
			Args:  []string{"-e", "-n", "l/3"},
			WantFiles: map[string]string{
				"xaa": "abcdefgh\n",
			},
		},
		{
			Name:  "round robin distributes the lines",
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			Args:  []string{"-n", "r/2"},
			WantFiles: map[string]string{
				"xaa": "a\nc\n",
				"xab": "b\nd\n",
			},
		},
		{
			Name:  "the kth round robin chunk goes to the standard output",
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			Args:  []string{"-n", "r/2/4"},
			Want:  "b\n",
		},
		{
			Name:  "all chunks are created when the input is short",
			Stdin: strings.NewReader("ab"),
			Args:  []string{"-n", "5"},
			WantFiles: map[string]string{
				"xaa": "a",
				"xab": "b",
				"xac": "",
				"xad": "",
				"xae": "",
			},
		},
		{
			Name:  "with -e only the non-empty chunks are created",
			Stdin: strings.NewReader("ab"),
			Args:  []string{"-e", "-n", "5"},
			WantFiles: map[string]string{
				"xaa": "a",
				"xab": "b",
			},
		},
		{
			Name:     "an invalid chunk count",
			Args:     []string{"-n", "x"},
			WantErr:  "split: invalid number of chunks: 'x'\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
		{
			Name:     "a chunk past the count",
			Args:     []string{"-n", "3/2"},
			WantErr:  "split: invalid chunk number: '3/2'\nTry 'split --help' for more information.\n",
			WantExit: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "split", usage)
}
