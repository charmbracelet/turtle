package csplit

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// testCase is the fields of a testutil.Case plus the exact set of files the
// file system must hold after the run: csplit creates and removes files, so
// the runner's Present and Absent subsets can't check it.
type testCase struct {
	Name     string
	Stdin    io.Reader
	FS       fsx.FS
	Args     []string
	Want     map[string]string
	WantOut  string
	WantErr  string
	WantExit int

	WantFiles []string
}

// runCase runs a single case through testutil, checking the set of files on
// the file system afterwards.
func runCase(t *testing.T, test testCase) {
	t.Helper()

	fsys := test.FS
	if fsys == nil {
		fsys = fsx.MemFS{}
	}
	before := map[string]bool{}
	if mem, ok := fsys.(fsx.MemFS); ok {
		for name := range mem {
			before[name] = true
		}
	}

	wantFiles, want := test.WantFiles, test.Want
	c := testutil.Case{
		Name:      test.Name,
		Stdin:     test.Stdin,
		FS:        fsys,
		Args:      test.Args,
		Want:      test.WantOut,
		WantErr:   test.WantErr,
		WantExit:  test.WantExit,
		WantFiles: want,
	}
	c.Check = func(t *testing.T, rt *coreutils.Runtime) {
		mem, ok := rt.FS.(fsx.MemFS)
		if !ok {
			return
		}
		var got []string
		for name := range mem {
			got = append(got, name)
		}
		sort.Strings(got)
		wantList := append([]string{}, wantFiles...)
		if wantFiles == nil {
			for name := range want {
				wantList = append(wantList, name)
			}
			for name := range before {
				if _, ok := want[name]; !ok {
					wantList = append(wantList, name)
				}
			}
		}
		sort.Strings(wantList)
		if fmt.Sprint(got) != fmt.Sprint(wantList) {
			t.Errorf("Run() files = %v, want %v", got, wantList)
		}
	}
	testutil.Once(t, New, c)
}

func TestRunLineNumbers(t *testing.T) {
	tests := []testCase{
		{
			Name:      "a line number splits off the rest",
			FS:        fsx.MemFS{"f": {Data: []byte("a\nb\nc\nd\n")}},
			Args:      []string{"f", "3"},
			WantOut:   "4\n4\n",
			WantFiles: []string{"f", "xx00", "xx01"},
			Want: map[string]string{
				"xx00": "a\nb\n",
				"xx01": "c\nd\n",
			},
		},
		{
			Name:      "line one creates an empty first piece",
			FS:        fsx.MemFS{"f": {Data: []byte("a\nb\n")}},
			Args:      []string{"f", "1"},
			WantOut:   "0\n4\n",
			WantFiles: []string{"f", "xx00", "xx01"},
			Want: map[string]string{
				"xx00": "",
				"xx01": "a\nb\n",
			},
		},
		{
			Name:      "the last line number leaves the last line for the rest",
			FS:        fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			Args:      []string{"f", "3"},
			WantOut:   "4\n2\n",
			WantFiles: []string{"f", "xx00", "xx01"},
			Want: map[string]string{
				"xx00": "a\nb\n",
				"xx01": "c\n",
			},
		},
		{
			Name:    "silent mode suppresses the sizes",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			Args:    []string{"-s", "f", "2"},
			WantOut: "",
			Want: map[string]string{
				"f":    "a\nb\nc\n",
				"xx00": "a\n",
				"xx01": "b\nc\n",
			},
		},
		{
			Name:    "custom prefix and digit count",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			Args:    []string{"-f", "part", "-n", "3", "f", "2"},
			WantOut: "2\n4\n",
			Want: map[string]string{
				"f":       "a\nb\nc\n",
				"part000": "a\n",
				"part001": "b\nc\n",
			},
		},
		{
			Name:      "numbers wider than the digit count still work",
			FS:        fsx.MemFS{"f": {Data: []byte(lines(1, 24))}},
			Args:      []string{"-s", "-n", "1", "f", "2", "{11}"},
			WantOut:   "",
			WantFiles: append([]string{"f"}, names("xx", 0, 12)...),
		},
		{
			Name:      "input may come from the standard input",
			Stdin:     strings.NewReader("a\nb\nc\n"),
			Args:      []string{"-", "2"},
			WantOut:   "2\n4\n",
			WantFiles: []string{"xx00", "xx01"},
			Want: map[string]string{
				"xx00": "a\n",
				"xx01": "b\nc\n",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRegexps(t *testing.T) {
	tests := []testCase{
		{
			Name:    "a regexp ends the piece before the matching line",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\nEND\nd\n")}},
			Args:    []string{"f", "/END/"},
			WantOut: "4\n6\n",
			Want: map[string]string{
				"xx00": "a\nb\n",
				"xx01": "END\nd\n",
			},
		},
		{
			Name:    "the first expression may match the first line",
			FS:      fsx.MemFS{"f": {Data: []byte("END\nb\nEND\nd\n")}},
			Args:    []string{"f", "/END/", "/END/"},
			WantOut: "0\n6\n6\n",
			Want: map[string]string{
				"xx00": "",
				"xx01": "END\nb\n",
				"xx02": "END\nd\n",
			},
		},
		{
			Name:    "a positive offset includes lines past the match",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nEND\nc\nd\n")}},
			Args:    []string{"f", "/END/+1"},
			WantOut: "6\n4\n",
			Want: map[string]string{
				"xx00": "a\nEND\n",
				"xx01": "c\nd\n",
			},
		},
		{
			Name:    "a negative offset ends the piece before the match",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\nEND\nd\ne\n")}},
			Args:    []string{"f", "/END/-1"},
			WantOut: "2\n10\n",
			Want: map[string]string{
				"xx00": "a\n",
				"xx01": "b\nEND\nd\ne\n",
			},
		},
		{
			Name:    "a percent expression skips lines without a file",
			FS:      fsx.MemFS{"f": {Data: []byte("skip\nthis\nEND\nkeep\n")}},
			Args:    []string{"f", "%END%"},
			WantOut: "9\n",
			Want: map[string]string{
				"xx00": "END\nkeep\n",
			},
		},
		{
			Name:    "the search starts after the current line",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\nc\nEND\ne\nf\nEND\nh\n")}},
			Args:    []string{"f", "5", "/END/"},
			WantOut: "10\n4\n6\n",
			Want: map[string]string{
				"xx00": "a\nb\nc\nEND\n",
				"xx01": "e\nf\n",
				"xx02": "END\nh\n",
			},
		},
		{
			Name:    "repeats reapply the search",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nE\nb\nE\nc\n")}},
			Args:    []string{"f", "/^E$/", "{1}"},
			WantOut: "2\n4\n4\n",
			Want: map[string]string{
				"xx00": "a\n",
				"xx01": "E\nb\n",
				"xx02": "E\nc\n",
			},
		},
		{
			Name:    "basic regular expression features",
			FS:      fsx.MemFS{"f": {Data: []byte("int x;\n\nint main()\n{\n}\n")}},
			Args:    []string{"f", `/^$/`, `/^{/`},
			WantOut: "7\n12\n4\n",
			Want: map[string]string{
				"xx00": "int x;\n",
				"xx01": "\nint main()\n",
				"xx02": "{\n}\n",
			},
		},
		{
			Name:    "an escaped delimiter is part of the expression",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb/c\nd\n")}},
			Args:    []string{"f", `/b\/c/`},
			WantOut: "2\n6\n",
			Want: map[string]string{
				"xx00": "a\n",
				"xx01": "b/c\nd\n",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRepeats(t *testing.T) {
	tests := []testCase{
		{
			Name:    "a line number repeat splits evenly",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\nc\nd\ne\nf\ng\n")}},
			Args:    []string{"f", "2", "{2}"},
			WantOut: "2\n4\n4\n4\n",
			Want: map[string]string{
				"xx00": "a\n",
				"xx01": "b\nc\n",
				"xx02": "d\ne\n",
				"xx03": "f\ng\n",
			},
		},
		{
			Name:     "a repeat of zero is invalid",
			FS:       fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			Args:     []string{"f", "2", "{0}"},
			WantErr:  "csplit: '{0}': integer required between '{' and '}'\n",
			WantExit: 1,
		},
		{
			Name:    "a repeated pattern with a negative offset moves on",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\nx\nc\nx\nd\n")}},
			Args:    []string{"f", "/x/-1", "{*}"},
			WantOut: "2\n4\n6\n",
			Want: map[string]string{
				"xx00": "a\n",
				"xx01": "b\nx\n",
				"xx02": "c\nx\nd\n",
			},
		},
		{
			Name:    "an unbounded repeat splits until the input ends",
			FS:      fsx.MemFS{"f": {Data: []byte("01\n02\n03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n13\n14\n")}},
			Args:    []string{"f", "/[05]$/", "{*}"},
			WantOut: "12\n15\n15\n",
			Want: map[string]string{
				"xx00": "01\n02\n03\n04\n",
				"xx01": "05\n06\n07\n08\n09\n",
				"xx02": "10\n11\n12\n13\n14\n",
			},
		},
		{
			Name:    "an unbounded repeat of a line number splits evenly",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\nc\nd\n")}},
			Args:    []string{"f", "2", "{*}"},
			WantOut: "2\n4\n2\n",
			Want: map[string]string{
				"xx00": "a\n",
				"xx01": "b\nc\n",
				"xx02": "d\n",
			},
		},
		{
			Name:    "an unbounded repeat with no match writes the rest",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\n")}},
			Args:    []string{"f", "/x/", "{*}"},
			WantOut: "4\n",
			Want: map[string]string{
				"xx00": "a\nb\n",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSuffixFormat(t *testing.T) {
	tests := []testCase{
		{
			Name:    "a hexadecimal format",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\n")}},
			Args:    []string{"-b", "%x", "f", "2"},
			WantOut: "2\n2\n",
			Want: map[string]string{
				"xx0": "a\n",
				"xx1": "b\n",
			},
		},
		{
			Name:    "a format with a prefix and width",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\n")}},
			Args:    []string{"--suffix-format=part-%03d.", "f", "2"},
			WantOut: "2\n2\n",
			Want: map[string]string{
				"xxpart-000.": "a\n",
				"xxpart-001.": "b\n",
			},
		},
		{
			Name:     "a percent literal alone is not a conversion",
			FS:       fsx.MemFS{"f": {Data: []byte("a\nb\n")}},
			Args:     []string{"-b", "%%d", "f", "2"},
			WantErr:  "csplit: '%%d': % conversion missing\nTry 'csplit --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "two conversions are rejected",
			FS:       fsx.MemFS{"f": {Data: []byte("a\nb\n")}},
			Args:     []string{"-b", "%d%d", "f", "2"},
			WantErr:  "csplit: '%d%d': too many % conversions\nTry 'csplit --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid verb is rejected",
			FS:       fsx.MemFS{"f": {Data: []byte("a\nb\n")}},
			Args:     []string{"-b", "%s", "f", "2"},
			WantErr:  "csplit: '%s': invalid conversion specifier %s\nTry 'csplit --help' for more information.\n",
			WantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunSuppressMatched(t *testing.T) {
	tests := []testCase{
		{
			Name:    "the matched line is left out",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\nc\nd\n")}},
			Args:    []string{"--suppress-matched", "f", "/c/"},
			WantOut: "4\n2\n",
			Want: map[string]string{
				"xx00": "a\nb\n",
				"xx01": "d\n",
			},
		},
		{
			Name:    "a positive offset still leaves the line out",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\nc\nd\ne\n")}},
			Args:    []string{"--suppress-matched", "f", "/b/+2"},
			WantOut: "4\n4\n",
			Want: map[string]string{
				"xx00": "a\nc\n",
				"xx01": "d\ne\n",
			},
		},
		{
			Name:    "the pieces are renumbered around the skipped lines",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\nc\nd\ne\n")}},
			Args:    []string{"--suppress-matched", "f", "/b/", "/e/"},
			WantOut: "2\n4\n0\n",
			Want: map[string]string{
				"xx00": "a\n",
				"xx01": "c\nd\n",
				"xx02": "",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunElideEmptyFiles(t *testing.T) {
	tests := []testCase{
		{
			Name:    "empty pieces are removed",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\n")}},
			Args:    []string{"-z", "f", "1"},
			WantOut: "4\n",
			Want: map[string]string{
				"xx01": "a\nb\n",
			},
		},
		{
			Name:    "the long form works",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			Args:    []string{"--elide-empty-files", "f", "1", "2", "3"},
			WantOut: "2\n2\n2\n",
			Want: map[string]string{
				"xx01": "a\n",
				"xx02": "b\n",
				"xx03": "c\n",
			},
		},
		{
			Name:    "with a suffix format",
			FS:      fsx.MemFS{"f": {Data: []byte("a\nb\n")}},
			Args:    []string{"-z", "-b", "%d", "f", "1"},
			WantOut: "4\n",
			Want: map[string]string{
				"xx1": "a\nb\n",
			},
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
			Name:     "missing operand",
			Args:     nil,
			WantErr:  "csplit: missing operand\nTry 'csplit --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "missing pattern",
			Args:     []string{"f"},
			FS:       fsx.MemFS{"f": {Data: []byte("a\n")}},
			WantErr:  "csplit: missing operand after 'f'\nTry 'csplit --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "unknown option",
			Args:     []string{"-y", "f", "1"},
			WantErr:  "csplit: invalid option -- 'y'\nTry 'csplit --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "invalid digit count",
			Args:     []string{"-n", "x", "f", "1"},
			WantErr:  "csplit: invalid number: 'x'\nTry 'csplit --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "missing input file",
			FS:       fsx.MemFS{},
			Args:     []string{"missing", "1"},
			WantErr:  "csplit: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "a line number past the end of the file",
			FS:       fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			Args:     []string{"f", "2", "10"},
			WantOut:  "2\n4\n",
			WantErr:  "csplit: '10': line number out of range\n",
			WantExit: 1,
		},
		{
			Name:     "a line number before the current line",
			FS:       fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			Args:     []string{"f", "3", "1"},
			WantOut:  "4\n",
			WantErr:  "csplit: '1': line number out of range\n",
			WantExit: 1,
		},
		{
			Name:     "a regexp without a match",
			FS:       fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			Args:     []string{"f", "/NOPE/"},
			WantOut:  "6\n",
			WantErr:  "csplit: '/NOPE/': match not found\n",
			WantExit: 1,
		},
		{
			Name:     "a regexp without a match on a repetition",
			FS:       fsx.MemFS{"f": {Data: []byte("a\nE\nb\nc\n")}},
			Args:     []string{"f", "/^E$/", "{2}"},
			WantOut:  "2\n6\n",
			WantErr:  "csplit: '/^E$/': match not found on repetition 1\n",
			WantExit: 1,
		},
		{
			Name:     "an offset past the end of the file",
			FS:       fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}},
			Args:     []string{"f", "/c/+5"},
			WantOut:  "6\n",
			WantErr:  "csplit: '/c/+5': line number out of range\n",
			WantExit: 1,
		},
		{
			Name:     "an offset before the current line",
			FS:       fsx.MemFS{"f": {Data: []byte("a\nb\nc\nd\n")}},
			Args:     []string{"f", "3", "/d/-3"},
			WantOut:  "4\n",
			WantErr:  "csplit: '/d/-3': line number out of range\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid regular expression",
			FS:       fsx.MemFS{"f": {Data: []byte("a\n")}},
			Args:     []string{"f", "/[/"},
			WantErr:  "csplit: '/[/': unmatched [\n",
			WantExit: 1,
		},
		{
			Name:     "a missing closing delimiter",
			FS:       fsx.MemFS{"f": {Data: []byte("a\n")}},
			Args:     []string{"f", "/abc"},
			WantErr:  "csplit: /abc: closing delimiter '/' missing\n",
			WantExit: 1,
		},
		{
			Name:     "a missing repeat count brace",
			FS:       fsx.MemFS{"f": {Data: []byte("a\n")}},
			Args:     []string{"f", "1", "{2"},
			WantErr:  "csplit: '{2': '}' is required in repeat count\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid repeat count",
			FS:       fsx.MemFS{"f": {Data: []byte("a\n")}},
			Args:     []string{"f", "1", "{x}"},
			WantErr:  "csplit: '{x}': integer required between '{' and '}'\n",
			WantExit: 1,
		},
		{
			Name:     "a stray repeat count",
			FS:       fsx.MemFS{"f": {Data: []byte("a\n")}},
			Args:     []string{"f", "{2}"},
			WantErr:  "csplit: '{2}': invalid pattern\n",
			WantExit: 1,
		},
		{
			Name:     "an invalid pattern",
			FS:       fsx.MemFS{"f": {Data: []byte("a\n")}},
			Args:     []string{"f", "abc"},
			WantErr:  "csplit: 'abc': invalid pattern\n",
			WantExit: 1,
		},
		{
			Name:     "a zero line number",
			FS:       fsx.MemFS{"f": {Data: []byte("a\n")}},
			Args:     []string{"f", "0"},
			WantErr:  "csplit: 0: line number must be greater than zero\n",
			WantExit: 1,
		},
		{
			Name:     "read error",
			Stdin:    testutil.ErrorReader{Err: errors.New("boom")},
			Args:     []string{"-", "1"},
			WantErr:  "csplit: -: boom\n",
			WantExit: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunErrorKeepsFiles(t *testing.T) {
	t.Run("created files are removed on error by default", func(t *testing.T) {
		files := fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}}
		runCase(t, testCase{
			FS:        files,
			Args:      []string{"f", "2", "10"},
			WantOut:   "2\n4\n",
			WantErr:   "csplit: '10': line number out of range\n",
			WantExit:  1,
			WantFiles: []string{"f"},
		})
	})

	t.Run("-k keeps the files created", func(t *testing.T) {
		files := fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}}
		runCase(t, testCase{
			FS:        files,
			Args:      []string{"-k", "f", "2", "10"},
			WantOut:   "2\n4\n",
			WantErr:   "csplit: '10': line number out of range\n",
			WantExit:  1,
			WantFiles: []string{"f", "xx00", "xx01"},
			Want: map[string]string{
				"xx00": "a\n",
				"xx01": "b\nc\n",
			},
		})
	})

	t.Run("-k keeps the partial file of a failed match", func(t *testing.T) {
		files := fsx.MemFS{"f": {Data: []byte("a\nb\nc\n")}}
		runCase(t, testCase{
			FS:        files,
			Args:      []string{"-k", "f", "/NOPE/"},
			WantOut:   "6\n",
			WantErr:   "csplit: '/NOPE/': match not found\n",
			WantExit:  1,
			WantFiles: []string{"f", "xx00"},
			Want: map[string]string{
				"xx00": "a\nb\nc\n",
			},
		})
	})

	t.Run("write error", func(t *testing.T) {
		runCase(t, testCase{
			FS:       testutil.FailWriteFS{FS: fsx.MemFS{}, Err: errors.New("disk full")},
			Stdin:    strings.NewReader("a\nb\n"),
			Args:     []string{"-", "1"},
			WantOut:  "0\n",
			WantErr:  "csplit: xx01: disk full\n",
			WantExit: 1,
		})
	})
}

func TestRunCancellation(t *testing.T) {
	testutil.Cancelled(t, New, &testutil.CancelReader{}, fsx.MemFS{}, "-", "1")
}

func lines(from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&b, "l%d\n", i)
	}
	return b.String()
}

func names(prefix string, from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf("%s%d", prefix, i))
	}
	return out
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "csplit", usage)
}
