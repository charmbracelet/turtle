package shuf

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/testutil"
	"charm.land/turtle/fsx"
)

// randFile builds a deterministic source of n bytes with a 64-bit LCG, so
// the 8-byte windows the generator reads are all distinct.
func randFile(n int, seed uint64) []byte {
	b := make([]byte, n)
	for i := range b {
		seed = seed*6364136223846793005 + 1442695040888963407
		b[i] = byte(seed >> 56)
	}
	return b
}

// testFS holds the fixtures used by the tests:
//
//	names.txt:     "zoe\neve\namy\n"
//	cards.txt:     "a\nb\nc\nd\n"
//	nonl.txt:      "b\na" (no trailing newline)
//	empty.txt:     ""
//	out.txt:       "old\n" (truncated by output tests)
//	rand1:         1024 random bytes (a random source)
//	rand2:         1024 other random bytes (another random source)
//	randshort:     the first 16 bytes of rand1 (too short for three lines)
//	randempty:     "" (an empty random source)
//	"-n":          "2\n1\n" (a file named like an option)
//	sub/inner.txt: "b\na\n"
func testFS() fsx.MemFS {
	return fsx.MemFS{
		"names.txt":     {Data: []byte("zoe\neve\namy\n")},
		"cards.txt":     {Data: []byte("a\nb\nc\nd\n")},
		"nonl.txt":      {Data: []byte("b\na")},
		"empty.txt":     {Data: []byte("")},
		"out.txt":       {Data: []byte("old\n")},
		"rand1":         {Data: randFile(1024, 1)},
		"rand2":         {Data: randFile(1024, 2)},
		"randshort":     {Data: randFile(16, 1)},
		"randempty":     {Data: nil},
		"-n":            {Data: []byte("2\n1\n")},
		"sub/inner.txt": {Data: []byte("b\na\n")},
	}
}

// runShuf runs shuf with the given fixtures and returns its standard output,
// for the tests which don't demand an exact output.
func runShuf(t *testing.T, stdin io.Reader, fsys fsx.MemFS, dir string, args ...string) string {
	t.Helper()

	stdout := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdin:      stdin,
		Stdout:     stdout,
		Stderr:     io.Discard,
		FS:         fsys,
		WorkingDir: dir,
	}
	if err := New(rt).Run(t.Context(), args...); err != nil {
		t.Fatalf("Run(%v) error = %v, want nil", args, err)
	}
	return stdout.String()
}

// items splits an output on its delimiter, dropping the final one.
func items(s string, delim byte) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, string(delim)), string(delim))
}

// checkPermutation verifies that got holds exactly the items of want, in any
// order.
func checkPermutation(t *testing.T, got, want string, delim byte) {
	t.Helper()

	gotItems, wantItems := items(got, delim), items(want, delim)
	slices.Sort(gotItems)
	slices.Sort(wantItems)
	if !slices.Equal(gotItems, wantItems) {
		t.Errorf("output items = %q, want a permutation of %q", gotItems, wantItems)
	}
}

// checkSample verifies that got holds count distinct items of want.
func checkSample(t *testing.T, got, want string, count int, delim byte) {
	t.Helper()

	gotItems, wantItems := items(got, delim), items(want, delim)
	if len(gotItems) != count {
		t.Errorf("output holds %d items, want %d: %q", len(gotItems), count, gotItems)
		return
	}
	seen := make(map[string]bool, count)
	for _, item := range gotItems {
		if !slices.Contains(wantItems, item) {
			t.Errorf("output item %q isn't in the input", item)
			return
		}
		if seen[item] {
			t.Errorf("output item %q repeats", item)
			return
		}
		seen[item] = true
	}
}

// checkRepeats verifies that got holds count items of want, allowing
// repetitions.
func checkRepeats(t *testing.T, got, want string, count int, delim byte) {
	t.Helper()

	gotItems, wantItems := items(got, delim), items(want, delim)
	if len(gotItems) != count {
		t.Errorf("output holds %d items, want %d: %q", len(gotItems), count, gotItems)
		return
	}
	for _, item := range gotItems {
		if !slices.Contains(wantItems, item) {
			t.Errorf("output item %q isn't in the input", item)
			return
		}
	}
}

// onceCase runs a single case like testutil.Once, with the test fixtures
// when the case sets no file system.
func onceCase(t *testing.T, test testutil.Case) {
	t.Helper()

	if test.FS == nil {
		test.FS = testFS()
	}
	testutil.Once(t, New, test)
}

// runCases runs the cases as sub-tests, each one like onceCase.
func runCases(t *testing.T, tests []testutil.Case) {
	t.Helper()

	for i := range tests {
		if tests[i].FS == nil {
			tests[i].FS = testFS()
		}
	}
	testutil.Run(t, New, tests)
}

func TestRunStdin(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		want  string
	}{
		{"four lines", "a\nb\nc\nd\n", "a\nb\nc\nd\n"},
		{"an empty input", "", ""},
		{"an empty line", "b\n\na\n", "b\n\na\n"},
		{"duplicates are kept", "b\na\na\n", "b\na\na\n"},
		{"a missing final newline", "b\na", "b\na\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := runShuf(t, strings.NewReader(test.stdin), nil, "")
			checkPermutation(t, got, test.want, '\n')
		})
	}
}

func TestRunFiles(t *testing.T) {
	t.Run("a file operand", func(t *testing.T) {
		got := runShuf(t, nil, testFS(), "", "names.txt")
		checkPermutation(t, got, "zoe\neve\namy\n", '\n')
	})

	t.Run("a dash operand reads the standard input", func(t *testing.T) {
		got := runShuf(t, strings.NewReader("b\na\n"), testFS(), "", "-")
		checkPermutation(t, got, "b\na\n", '\n')
	})

	t.Run("pathnames are resolved from the working directory", func(t *testing.T) {
		got := runShuf(t, nil, testFS(), "sub", "inner.txt")
		checkPermutation(t, got, "b\na\n", '\n')
	})

	t.Run("a file named like an option after a double dash", func(t *testing.T) {
		got := runShuf(t, nil, testFS(), "", "--", "-n")
		checkPermutation(t, got, "2\n1\n", '\n')
	})

	t.Run("a zero-length file", func(t *testing.T) {
		got := runShuf(t, nil, testFS(), "", "empty.txt")
		checkPermutation(t, got, "", '\n')
	})

	t.Run("a missing file", func(t *testing.T) {
		test := testutil.Case{
			FS:       testFS(),
			Args:     []string{"missing.txt"},
			WantErr:  "shuf: missing.txt: file does not exist\n",
			WantExit: 1,
		}
		onceCase(t, test)
	})

	t.Run("two file operands", func(t *testing.T) {
		test := testutil.Case{
			FS:       testFS(),
			Args:     []string{"names.txt", "cards.txt"},
			WantErr:  "shuf: extra operand 'cards.txt'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		}
		onceCase(t, test)
	})
}

// TestRunDeterministic covers the outputs which a --random-source file pins
// down, since they don't depend on the entropy of the system.
func TestRunDeterministic(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "four lines from the standard input",
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			FS:    testFS(),
			Args:  []string{"--random-source=rand1"},
			Want:  "b\nd\nc\na\n",
		},
		{
			Name:  "the same source gives the same output",
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			FS:    testFS(),
			Args:  []string{"--random-source=rand1"},
			Want:  "b\nd\nc\na\n",
		},
		{
			Name:  "a different source gives a different permutation",
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			FS:    testFS(),
			Args:  []string{"--random-source=rand2"},
			Want:  "d\nb\nc\na\n",
		},
		{
			Name:  "three lines",
			Stdin: strings.NewReader("x\ny\nz\n"),
			FS:    testFS(),
			Args:  []string{"--random-source=rand1"},
			Want:  "y\nz\nx\n",
		},
		{
			Name: "a file operand",
			FS:   testFS(),
			Args: []string{"--random-source=rand1", "names.txt"},
			Want: "eve\namy\nzoe\n",
		},
		{
			Name: "a file without a final newline",
			FS:   testFS(),
			Args: []string{"--random-source=rand1", "nonl.txt"},
			Want: "a\nb\n",
		},
		{
			Name: "a relative file from the working directory",
			FS:   testFS(),
			Dir:  "sub",
			Args: []string{"--random-source=../rand1", "inner.txt"},
			Want: "a\nb\n",
		},
		{
			Name: "a zero count doesn't read a missing file",
			FS:   testFS(),
			Args: []string{"-n", "0", "missing.txt"},
			Want: "",
		},
	}

	runCases(t, tests)
}

func TestRunEcho(t *testing.T) {
	t.Run("the operands are the input lines", func(t *testing.T) {
		got := runShuf(t, nil, nil, "", "-e", "clubs", "hearts", "diamonds", "spades")
		checkPermutation(t, got, "clubs\nhearts\ndiamonds\nspades\n", '\n')
	})

	t.Run("no operands write nothing", func(t *testing.T) {
		got := runShuf(t, strings.NewReader("a\n"), nil, "", "-e")
		if got != "" {
			t.Errorf("Run() stdout = %q, want %q", got, "")
		}
	})

	tests := []testutil.Case{
		{
			Name: "a deterministic permutation",
			Args: []string{"-e", "--random-source=rand1", "clubs", "hearts", "diamonds", "spades"},
			Want: "hearts\nspades\ndiamonds\nclubs\n",
		},
		{
			Name: "two operands keep both",
			Args: []string{"-e", "--random-source=rand1", "one", "two"},
			Want: "two\none\n",
		},
		{
			Name: "duplicate operands are kept",
			Args: []string{"-e", "--random-source=rand1", "a", "a", "b"},
			Want: "a\nb\na\n",
		},
	}

	runCases(t, tests)
}

func TestRunRange(t *testing.T) {
	t.Run("the values are within the range", func(t *testing.T) {
		got := runShuf(t, nil, nil, "", "-i", "10-12")
		checkPermutation(t, got, "10\n11\n12\n", '\n')
	})

	t.Run("a head count samples the range", func(t *testing.T) {
		got := runShuf(t, nil, nil, "", "-i", "0-99", "-n", "5")
		gotItems := items(got, '\n')
		if len(gotItems) != 5 {
			t.Errorf("output holds %d items, want 5: %q", len(gotItems), gotItems)
		}
		seen := make(map[string]bool, len(gotItems))
		for _, item := range gotItems {
			n, err := strconv.Atoi(item)
			if err != nil || n < 0 || n > 99 {
				t.Errorf("output item %q is out of the range", item)
				return
			}
			if seen[item] {
				t.Errorf("output item %q repeats", item)
				return
			}
			seen[item] = true
		}
	})

	tests := []testutil.Case{
		{
			Name: "a range of four",
			Args: []string{"-i", "1-4", "--random-source=rand1"},
			Want: "1\n4\n3\n2\n",
		},
		{
			Name: "the long form",
			Args: []string{"--input-range=1-4", "--random-source=rand1"},
			Want: "1\n4\n3\n2\n",
		},
		{
			Name: "an attached value",
			Args: []string{"-i1-5", "--random-source=rand1"},
			Want: "5\n4\n3\n1\n2\n",
		},
		{
			Name: "a range of one",
			Args: []string{"-i", "5-5", "--random-source=rand1"},
			Want: "5\n",
		},
		{
			Name: "a range at zero",
			Args: []string{"-i", "0-0", "--random-source=rand1"},
			Want: "0\n",
		},
		{
			Name: "a head count of the range",
			Args: []string{"-i", "1-4", "-n", "2", "--random-source=rand1"},
			Want: "1\n4\n",
		},
		{
			Name: "a big range with a head count",
			Args: []string{"-i", "0-999999999", "-n", "3", "--random-source=rand1"},
			Want: "581272172\n163459798\n882872940\n",
		},
		{
			Name: "a huge range with a head count",
			Args: []string{"-i", "0-99999999999999999", "-n", "3", "--random-source=rand1"},
			Want: "39829582737932908\n89598964340241110\n92574438307318115\n",
		},
		{
			Name:     "a range with an operand",
			Args:     []string{"-i", "1-2", "file"},
			WantErr:  "shuf: extra operand 'file'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing value",
			Args:     []string{"-i"},
			WantErr:  "shuf: option requires an argument -- 'i'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an empty value",
			Args:     []string{"-i", ""},
			WantErr:  "shuf: invalid input range ''\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a value without a dash",
			Args:     []string{"-i", "5"},
			WantErr:  "shuf: invalid input range '5'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a reversed range",
			Args:     []string{"-i", "5-3"},
			WantErr:  "shuf: invalid input range '5-3'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "two dashes",
			Args:     []string{"-i", "5-3-7"},
			WantErr:  "shuf: invalid input range '5-3-7'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing high value",
			Args:     []string{"-i", "5-"},
			WantErr:  "shuf: invalid input range '5-'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "letters",
			Args:     []string{"-i", "a-b"},
			WantErr:  "shuf: invalid input range 'a-b'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an overflowing low value",
			Args:     []string{"-i", "18446744073709551616-20"},
			WantErr:  "shuf: invalid input range '18446744073709551616-20'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a range that wraps around",
			Args:     []string{"-i", "0-18446744073709551615"},
			WantErr:  "shuf: invalid input range '0-18446744073709551615'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a range too large to hold its indices",
			Args:     []string{"-i", "0-9223372036854775807"},
			WantErr:  "shuf: too many input lines\n",
			WantExit: 1,
		},
		{
			Name:     "a range representable but unallocatable",
			Args:     []string{"-i", "0-1152921504606846975"},
			WantErr:  "shuf: too many input lines\n",
			WantExit: 1,
		},
		{
			Name:     "a range too large to permute in memory",
			Args:     []string{"-i", "0-9999999"},
			WantErr:  "shuf: too many input lines\n",
			WantExit: 1,
		},
		{
			Name:     "a sample too large to hold in memory",
			Args:     []string{"-i", "0-99999999", "-n", "2000000"},
			WantErr:  "shuf: too many input lines\n",
			WantExit: 1,
		},
		{
			Name:     "a range near the top of the unsigned numbers",
			Args:     []string{"-i", "5-18446744073709551615"},
			WantErr:  "shuf: too many input lines\n",
			WantExit: 1,
		},
		{
			Name:     "two ranges",
			Args:     []string{"-i", "1-5", "-i", "2-4"},
			WantErr:  "shuf: multiple -i options specified\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "echo with a range",
			Args:     []string{"-e", "-i", "1-2"},
			WantErr:  "shuf: cannot combine -e and -i options\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
	}

	runCases(t, tests)
}

func TestRunHeadCount(t *testing.T) {
	t.Run("a sample of the input", func(t *testing.T) {
		got := runShuf(t, strings.NewReader("a\nb\nc\nd\n"), nil, "", "-n", "2")
		checkSample(t, got, "a\nb\nc\nd\n", 2, '\n')
	})

	t.Run("a count larger than the input writes everything", func(t *testing.T) {
		got := runShuf(t, strings.NewReader("a\nb\nc\n"), nil, "", "-n", "10")
		checkPermutation(t, got, "a\nb\nc\n", '\n')
	})

	t.Run("a zero count writes nothing", func(t *testing.T) {
		got := runShuf(t, strings.NewReader("a\nb\n"), nil, "", "-n", "0")
		if got != "" {
			t.Errorf("Run() stdout = %q, want %q", got, "")
		}
	})

	t.Run("a single line", func(t *testing.T) {
		got := runShuf(t, strings.NewReader("a\nb\nc\n"), nil, "", "-n", "1")
		checkSample(t, got, "a\nb\nc\n", 1, '\n')
	})

	t.Run("several counts keep the smallest", func(t *testing.T) {
		got := runShuf(t, strings.NewReader("a\nb\nc\nd\n"), nil, "", "-n", "3", "-n", "1")
		checkSample(t, got, "a\nb\nc\nd\n", 1, '\n')
	})

	t.Run("an overflowing count means no limit", func(t *testing.T) {
		got := runShuf(t, strings.NewReader("a\nb\nc\nd\n"), nil, "", "-n", "99999999999999999999")
		checkPermutation(t, got, "a\nb\nc\nd\n", '\n')
	})

	t.Run("the count applies to a range", func(t *testing.T) {
		got := runShuf(t, nil, nil, "", "-i", "0-9", "-n", "4")
		checkSample(t, got, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9\n", 4, '\n')
	})

	tests := []testutil.Case{
		{
			Name:     "a non-numeric count",
			Args:     []string{"-n", "abc"},
			WantErr:  "shuf: invalid line count: 'abc'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a negative count",
			Args:     []string{"-n", "-1"},
			WantErr:  "shuf: invalid line count: '-1'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an empty count",
			Args:     []string{"-n", ""},
			WantErr:  "shuf: invalid line count: ''\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing count",
			Args:     []string{"-n"},
			WantErr:  "shuf: option requires an argument -- 'n'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
	}

	runCases(t, tests)
}

func TestRunRepeat(t *testing.T) {
	t.Run("the head count lines repeat", func(t *testing.T) {
		got := runShuf(t, strings.NewReader("a\nb\nc\n"), nil, "", "-r", "-n", "5")
		checkRepeats(t, got, "a\nb\nc\n", 5, '\n')
	})

	t.Run("a count larger than the input is allowed", func(t *testing.T) {
		got := runShuf(t, strings.NewReader("a\nb\n"), nil, "", "-r", "-n", "10")
		checkRepeats(t, got, "a\nb\n", 10, '\n')
	})

	t.Run("the range is sampled with replacement", func(t *testing.T) {
		got := runShuf(t, nil, nil, "", "-r", "-n", "8", "-i", "0-9")
		checkRepeats(t, got, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9\n", 8, '\n')
	})

	t.Run("a deterministic selection", func(t *testing.T) {
		test := testutil.Case{
			Args: []string{"-r", "-n", "5", "-e", "--random-source=rand1", "a", "b"},
			Want: "a\na\na\nb\na\n",
		}
		onceCase(t, test)
	})

	t.Run("an empty input", func(t *testing.T) {
		test := testutil.Case{
			Stdin:    strings.NewReader(""),
			Args:     []string{"-r", "-n", "3"},
			WantErr:  "shuf: no lines to repeat\n",
			WantExit: 1,
		}
		onceCase(t, test)
	})

	t.Run("echo without operands", func(t *testing.T) {
		test := testutil.Case{
			Args:     []string{"-r", "-n", "3", "-e"},
			WantErr:  "shuf: no lines to repeat\n",
			WantExit: 1,
		}
		onceCase(t, test)
	})

	t.Run("a zero count of an empty input writes nothing", func(t *testing.T) {
		test := testutil.Case{
			Stdin: strings.NewReader(""),
			Args:  []string{"-r", "-n", "0"},
			Want:  "",
		}
		onceCase(t, test)
	})
}

func TestRunOutput(t *testing.T) {
	t.Run("the output file is written", func(t *testing.T) {
		test := testutil.Case{
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			FS:    testFS(),
			Args:  []string{"-o", "out.txt", "--random-source=rand1"},
			WantFiles: map[string]string{
				"out.txt": "b\nd\nc\na\n",
			},
		}
		onceCase(t, test)
	})

	t.Run("an existing output file is truncated", func(t *testing.T) {
		test := testutil.Case{
			Stdin: strings.NewReader("a\nb\n"),
			FS:    testFS(),
			Args:  []string{"-o", "out.txt", "--random-source=rand1"},
			WantFiles: map[string]string{
				"out.txt": "b\na\n",
			},
		}
		onceCase(t, test)
	})

	t.Run("a file is shuffled in place", func(t *testing.T) {
		test := testutil.Case{
			FS:   testFS(),
			Args: []string{"--random-source=rand1", "-o", "cards.txt", "cards.txt"},
			WantFiles: map[string]string{
				"cards.txt": "b\nd\nc\na\n",
			},
		}
		onceCase(t, test)
	})

	t.Run("repeating the same output file is allowed", func(t *testing.T) {
		test := testutil.Case{
			Stdin: strings.NewReader("a\nb\n"),
			FS:    testFS(),
			Args:  []string{"-o", "out.txt", "-o", "out.txt", "--random-source=rand1"},
			WantFiles: map[string]string{
				"out.txt": "b\na\n",
			},
		}
		onceCase(t, test)
	})

	tests := []testutil.Case{
		{
			Name:     "two different output files",
			Stdin:    strings.NewReader("a\nb\n"),
			FS:       testFS(),
			Args:     []string{"-o", "out.txt", "-o", "other.txt"},
			WantErr:  "shuf: multiple output files specified\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "a missing output file argument",
			Args:     []string{"-o"},
			WantErr:  "shuf: option requires an argument -- 'o'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an output file without a parent directory",
			Stdin:    strings.NewReader("a\nb\n"),
			FS:       testFS(),
			Args:     []string{"-o", "no/such/out.txt"},
			WantErr:  "shuf: no/such/out.txt: file does not exist\n",
			WantExit: 1,
		},
	}

	runCases(t, tests)
}

func TestRunRandomSource(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:  "a source with enough bytes",
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			FS:    testFS(),
			Args:  []string{"--random-source=rand1"},
			Want:  "b\nd\nc\na\n",
		},
		{
			Name:  "repeating the same random source is allowed",
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			FS:    testFS(),
			Args:  []string{"--random-source=rand1", "--random-source=rand1"},
			Want:  "b\nd\nc\na\n",
		},
		{
			Name:     "an empty random source fails",
			Stdin:    strings.NewReader("a\nb\nc\nd\n"),
			FS:       testFS(),
			Args:     []string{"--random-source=randempty"},
			WantErr:  "shuf: randempty: end of file\n",
			WantExit: 1,
		},
		{
			Name:  "a single line needs no random bytes",
			Stdin: strings.NewReader("only\n"),
			FS:    testFS(),
			Args:  []string{"--random-source=randempty"},
			Want:  "only\n",
		},
		{
			Name: "a single number range needs no random bytes",
			FS:   testFS(),
			Args: []string{"-i", "5-5", "--random-source=randempty"},
			Want: "5\n",
		},
		{
			Name:     "a short random source fails",
			Stdin:    strings.NewReader("a\nb\nc\nd\n"),
			FS:       testFS(),
			Args:     []string{"--random-source=randshort"},
			WantErr:  "shuf: randshort: end of file\n",
			WantExit: 1,
		},
		{
			Name:  "a short random source is enough for two lines",
			Stdin: strings.NewReader("a\nb\n"),
			FS:    testFS(),
			Args:  []string{"--random-source=randshort"},
			Want:  "b\na\n",
		},
		{
			Name:     "a missing random source",
			Stdin:    strings.NewReader("a\nb\n"),
			FS:       testFS(),
			Args:     []string{"--random-source=missing"},
			WantErr:  "shuf: missing: file does not exist\n",
			WantExit: 1,
		},
		{
			Name:     "two different random sources",
			Stdin:    strings.NewReader("a\nb\n"),
			FS:       testFS(),
			Args:     []string{"--random-source=rand1", "--random-source=rand2"},
			WantErr:  "shuf: multiple random sources specified\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
	}

	runCases(t, tests)
}

func TestRunZeroTerminated(t *testing.T) {
	t.Run("the items of the input are permuted", func(t *testing.T) {
		got := runShuf(t, strings.NewReader("x\x00z\x00y\x00"), nil, "", "-z")
		checkPermutation(t, got, "x\x00z\x00y\x00", 0)
	})

	tests := []testutil.Case{
		{
			Name:  "the output is zero terminated",
			Stdin: strings.NewReader("a\x00b\x00c\x00"),
			Args:  []string{"-z", "--random-source=rand1"},
			Want:  "b\x00c\x00a\x00",
		},
		{
			Name:  "a missing final zero byte",
			Stdin: strings.NewReader("a\x00b\x00c"),
			Args:  []string{"-z", "--random-source=rand1"},
			Want:  "b\x00c\x00a\x00",
		},
		{
			Name: "echo with zero termination",
			Args: []string{"-z", "-e", "--random-source=rand1", "a", "b", "c"},
			Want: "b\x00c\x00a\x00",
		},
		{
			Name: "a range with zero termination",
			Args: []string{"-z", "-i", "1-3", "--random-source=rand1"},
			Want: "1\x002\x003\x00",
		},
		{
			Name: "the long form",
			Args: []string{"--zero-terminated", "-e", "--random-source=rand1", "a", "b", "c"},
			Want: "b\x00c\x00a\x00",
		},
	}

	runCases(t, tests)
}

func TestRunOptionForms(t *testing.T) {
	t.Run("grouped options", func(t *testing.T) {
		test := testutil.Case{
			Args: []string{"-rn2", "-e", "--random-source=rand1", "a", "b"},
			Want: "a\na\n",
		}
		onceCase(t, test)
	})

	t.Run("options stop at the first operand", func(t *testing.T) {
		test := testutil.Case{
			FS:       testFS(),
			Args:     []string{"names.txt", "-z"},
			WantErr:  "shuf: extra operand '-z'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		}
		onceCase(t, test)
	})

	t.Run("repeat with a count of the input", func(t *testing.T) {
		test := testutil.Case{
			Stdin: strings.NewReader("a\nb\nc\nd\n"),
			Args:  []string{"-r", "-n", "2", "--random-source=rand1"},
			Want:  "a\nc\n",
		}
		onceCase(t, test)
	})
}

func TestRunErrors(t *testing.T) {
	tests := []testutil.Case{
		{
			Name:     "an invalid short option",
			Args:     []string{"-x"},
			WantErr:  "shuf: invalid option -- 'x'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
		{
			Name:     "an unrecognized long option",
			Args:     []string{"--foo"},
			WantErr:  "shuf: unrecognized option '--foo'\nTry 'shuf --help' for more information.\n",
			WantExit: 1,
		},
	}

	runCases(t, tests)
}

func TestRunInterrupted(t *testing.T) {
	t.Run("a done context stops reading the input", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  &testutil.CancelReader{Cancel: cancel},
			Stdout: stdout,
			Stderr: io.Discard,
		}
		err := New(rt).Run(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	})

	t.Run("a done context stops shuffling the operands", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdout: stdout,
			Stderr: io.Discard,
		}
		err := New(rt).Run(ctx, "-e", "a", "b", "c")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("Run() stdout = %q, want %q", got, "")
		}
	})

	t.Run("a done context stops a range permutation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdout: stdout,
			Stderr: io.Discard,
		}
		err := New(rt).Run(ctx, "-i", "1-100000")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("Run() stdout = %q, want %q", got, "")
		}
	})

	t.Run("a done context stops an endless repeat", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdout: stdout,
			Stderr: io.Discard,
		}
		err := New(rt).Run(ctx, "-r", "-e", "a", "b")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("Run() stdout = %q, want %q", got, "")
		}
	})
}

func TestRunWriteError(t *testing.T) {
	stderr := new(bytes.Buffer)
	rt := &coreutils.Runtime{
		Stdout: testutil.ErrorWriter{Err: errors.New("boom")},
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context(), "-e", "a", "b", "c")
	if code := testutil.ExitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1", code)
	}
	if got, want := stderr.String(), "shuf: write error: boom\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
	}
}

func TestHelpVersion(t *testing.T) {
	testutil.HelpVersion(t, New, "shuf", usage)
}
