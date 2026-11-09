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

	"charm.land/snail/coreutils"
	"charm.land/snail/fsx"
)

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
	name      string
	stdin     io.Reader
	fsys      fsx.MemFS
	dir       string
	args      []string
	want      string
	wantErr   string
	wantExit  int
	wantFiles map[string]string
}

func runCase(t *testing.T, test testCase) {
	t.Helper()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	fsys := test.fsys
	if fsys == nil {
		fsys = testFS()
	}
	rt := &coreutils.Runtime{
		Stdin:      test.stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		FS:         fsys,
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
	for name, want := range test.wantFiles {
		f := fsys[name]
		if f == nil {
			t.Errorf("Run() didn't create file %q", name)
			continue
		}
		if got := string(f.Data); got != want {
			t.Errorf("Run() file %q = %q, want %q", name, got, want)
		}
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
		test := testCase{
			fsys:     testFS(),
			args:     []string{"missing.txt"},
			wantErr:  "shuf: missing.txt: file does not exist\n",
			wantExit: 1,
		}
		runCase(t, test)
	})

	t.Run("two file operands", func(t *testing.T) {
		test := testCase{
			fsys:     testFS(),
			args:     []string{"names.txt", "cards.txt"},
			wantErr:  "shuf: extra operand 'cards.txt'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		}
		runCase(t, test)
	})
}

// TestRunDeterministic covers the outputs which a --random-source file pins
// down, since they don't depend on the entropy of the system.
func TestRunDeterministic(t *testing.T) {
	tests := []testCase{
		{
			name:  "four lines from the standard input",
			stdin: strings.NewReader("a\nb\nc\nd\n"),
			fsys:  testFS(),
			args:  []string{"--random-source=rand1"},
			want:  "b\nd\nc\na\n",
		},
		{
			name:  "the same source gives the same output",
			stdin: strings.NewReader("a\nb\nc\nd\n"),
			fsys:  testFS(),
			args:  []string{"--random-source=rand1"},
			want:  "b\nd\nc\na\n",
		},
		{
			name:  "a different source gives a different permutation",
			stdin: strings.NewReader("a\nb\nc\nd\n"),
			fsys:  testFS(),
			args:  []string{"--random-source=rand2"},
			want:  "d\nb\nc\na\n",
		},
		{
			name:  "three lines",
			stdin: strings.NewReader("x\ny\nz\n"),
			fsys:  testFS(),
			args:  []string{"--random-source=rand1"},
			want:  "y\nz\nx\n",
		},
		{
			name: "a file operand",
			fsys: testFS(),
			args: []string{"--random-source=rand1", "names.txt"},
			want: "eve\namy\nzoe\n",
		},
		{
			name: "a file without a final newline",
			fsys: testFS(),
			args: []string{"--random-source=rand1", "nonl.txt"},
			want: "a\nb\n",
		},
		{
			name: "a relative file from the working directory",
			fsys: testFS(),
			dir:  "sub",
			args: []string{"--random-source=../rand1", "inner.txt"},
			want: "a\nb\n",
		},
		{
			name: "a zero count doesn't read a missing file",
			fsys: testFS(),
			args: []string{"-n", "0", "missing.txt"},
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
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

	tests := []testCase{
		{
			name: "a deterministic permutation",
			args: []string{"-e", "--random-source=rand1", "clubs", "hearts", "diamonds", "spades"},
			want: "hearts\nspades\ndiamonds\nclubs\n",
		},
		{
			name: "two operands keep both",
			args: []string{"-e", "--random-source=rand1", "one", "two"},
			want: "two\none\n",
		},
		{
			name: "duplicate operands are kept",
			args: []string{"-e", "--random-source=rand1", "a", "a", "b"},
			want: "a\nb\na\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
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

	tests := []testCase{
		{
			name: "a range of four",
			args: []string{"-i", "1-4", "--random-source=rand1"},
			want: "1\n4\n3\n2\n",
		},
		{
			name: "the long form",
			args: []string{"--input-range=1-4", "--random-source=rand1"},
			want: "1\n4\n3\n2\n",
		},
		{
			name: "an attached value",
			args: []string{"-i1-5", "--random-source=rand1"},
			want: "5\n4\n3\n1\n2\n",
		},
		{
			name: "a range of one",
			args: []string{"-i", "5-5", "--random-source=rand1"},
			want: "5\n",
		},
		{
			name: "a range at zero",
			args: []string{"-i", "0-0", "--random-source=rand1"},
			want: "0\n",
		},
		{
			name: "a head count of the range",
			args: []string{"-i", "1-4", "-n", "2", "--random-source=rand1"},
			want: "1\n4\n",
		},
		{
			name: "a big range with a head count",
			args: []string{"-i", "0-999999999", "-n", "3", "--random-source=rand1"},
			want: "581272172\n163459798\n882872940\n",
		},
		{
			name: "a huge range with a head count",
			args: []string{"-i", "0-99999999999999999", "-n", "3", "--random-source=rand1"},
			want: "39829582737932908\n89598964340241110\n92574438307318115\n",
		},
		{
			name:     "a range with an operand",
			args:     []string{"-i", "1-2", "file"},
			wantErr:  "shuf: extra operand 'file'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a missing value",
			args:     []string{"-i"},
			wantErr:  "shuf: option requires an argument -- 'i'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an empty value",
			args:     []string{"-i", ""},
			wantErr:  "shuf: invalid input range ''\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a value without a dash",
			args:     []string{"-i", "5"},
			wantErr:  "shuf: invalid input range '5'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a reversed range",
			args:     []string{"-i", "5-3"},
			wantErr:  "shuf: invalid input range '5-3'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "two dashes",
			args:     []string{"-i", "5-3-7"},
			wantErr:  "shuf: invalid input range '5-3-7'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a missing high value",
			args:     []string{"-i", "5-"},
			wantErr:  "shuf: invalid input range '5-'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "letters",
			args:     []string{"-i", "a-b"},
			wantErr:  "shuf: invalid input range 'a-b'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an overflowing low value",
			args:     []string{"-i", "18446744073709551616-20"},
			wantErr:  "shuf: invalid input range '18446744073709551616-20'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a range that wraps around",
			args:     []string{"-i", "0-18446744073709551615"},
			wantErr:  "shuf: invalid input range '0-18446744073709551615'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a range too large to hold its indices",
			args:     []string{"-i", "0-9223372036854775807"},
			wantErr:  "shuf: too many input lines\n",
			wantExit: 1,
		},
		{
			name:     "a range representable but unallocatable",
			args:     []string{"-i", "0-1152921504606846975"},
			wantErr:  "shuf: too many input lines\n",
			wantExit: 1,
		},
		{
			name:     "a range too large to permute in memory",
			args:     []string{"-i", "0-9999999"},
			wantErr:  "shuf: too many input lines\n",
			wantExit: 1,
		},
		{
			name:     "a sample too large to hold in memory",
			args:     []string{"-i", "0-99999999", "-n", "2000000"},
			wantErr:  "shuf: too many input lines\n",
			wantExit: 1,
		},
		{
			name:     "a range near the top of the unsigned numbers",
			args:     []string{"-i", "5-18446744073709551615"},
			wantErr:  "shuf: too many input lines\n",
			wantExit: 1,
		},
		{
			name:     "two ranges",
			args:     []string{"-i", "1-5", "-i", "2-4"},
			wantErr:  "shuf: multiple -i options specified\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "echo with a range",
			args:     []string{"-e", "-i", "1-2"},
			wantErr:  "shuf: cannot combine -e and -i options\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
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

	tests := []testCase{
		{
			name:     "a non-numeric count",
			args:     []string{"-n", "abc"},
			wantErr:  "shuf: invalid line count: 'abc'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a negative count",
			args:     []string{"-n", "-1"},
			wantErr:  "shuf: invalid line count: '-1'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an empty count",
			args:     []string{"-n", ""},
			wantErr:  "shuf: invalid line count: ''\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a missing count",
			args:     []string{"-n"},
			wantErr:  "shuf: option requires an argument -- 'n'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
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
		test := testCase{
			args: []string{"-r", "-n", "5", "-e", "--random-source=rand1", "a", "b"},
			want: "a\na\na\nb\na\n",
		}
		runCase(t, test)
	})

	t.Run("an empty input", func(t *testing.T) {
		test := testCase{
			stdin:    strings.NewReader(""),
			args:     []string{"-r", "-n", "3"},
			wantErr:  "shuf: no lines to repeat\n",
			wantExit: 1,
		}
		runCase(t, test)
	})

	t.Run("echo without operands", func(t *testing.T) {
		test := testCase{
			args:     []string{"-r", "-n", "3", "-e"},
			wantErr:  "shuf: no lines to repeat\n",
			wantExit: 1,
		}
		runCase(t, test)
	})

	t.Run("a zero count of an empty input writes nothing", func(t *testing.T) {
		test := testCase{
			stdin: strings.NewReader(""),
			args:  []string{"-r", "-n", "0"},
			want:  "",
		}
		runCase(t, test)
	})
}

func TestRunOutput(t *testing.T) {
	t.Run("the output file is written", func(t *testing.T) {
		test := testCase{
			stdin: strings.NewReader("a\nb\nc\nd\n"),
			fsys:  testFS(),
			args:  []string{"-o", "out.txt", "--random-source=rand1"},
			wantFiles: map[string]string{
				"out.txt": "b\nd\nc\na\n",
			},
		}
		runCase(t, test)
	})

	t.Run("an existing output file is truncated", func(t *testing.T) {
		test := testCase{
			stdin: strings.NewReader("a\nb\n"),
			fsys:  testFS(),
			args:  []string{"-o", "out.txt", "--random-source=rand1"},
			wantFiles: map[string]string{
				"out.txt": "b\na\n",
			},
		}
		runCase(t, test)
	})

	t.Run("a file is shuffled in place", func(t *testing.T) {
		test := testCase{
			fsys: testFS(),
			args: []string{"--random-source=rand1", "-o", "cards.txt", "cards.txt"},
			wantFiles: map[string]string{
				"cards.txt": "b\nd\nc\na\n",
			},
		}
		runCase(t, test)
	})

	t.Run("repeating the same output file is allowed", func(t *testing.T) {
		test := testCase{
			stdin: strings.NewReader("a\nb\n"),
			fsys:  testFS(),
			args:  []string{"-o", "out.txt", "-o", "out.txt", "--random-source=rand1"},
			wantFiles: map[string]string{
				"out.txt": "b\na\n",
			},
		}
		runCase(t, test)
	})

	tests := []testCase{
		{
			name:     "two different output files",
			stdin:    strings.NewReader("a\nb\n"),
			fsys:     testFS(),
			args:     []string{"-o", "out.txt", "-o", "other.txt"},
			wantErr:  "shuf: multiple output files specified\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "a missing output file argument",
			args:     []string{"-o"},
			wantErr:  "shuf: option requires an argument -- 'o'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an output file without a parent directory",
			stdin:    strings.NewReader("a\nb\n"),
			fsys:     testFS(),
			args:     []string{"-o", "no/such/out.txt"},
			wantErr:  "shuf: no/such/out.txt: file does not exist\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunRandomSource(t *testing.T) {
	tests := []testCase{
		{
			name:  "a source with enough bytes",
			stdin: strings.NewReader("a\nb\nc\nd\n"),
			fsys:  testFS(),
			args:  []string{"--random-source=rand1"},
			want:  "b\nd\nc\na\n",
		},
		{
			name:  "repeating the same random source is allowed",
			stdin: strings.NewReader("a\nb\nc\nd\n"),
			fsys:  testFS(),
			args:  []string{"--random-source=rand1", "--random-source=rand1"},
			want:  "b\nd\nc\na\n",
		},
		{
			name:     "an empty random source fails",
			stdin:    strings.NewReader("a\nb\nc\nd\n"),
			fsys:     testFS(),
			args:     []string{"--random-source=randempty"},
			wantErr:  "shuf: randempty: end of file\n",
			wantExit: 1,
		},
		{
			name:  "a single line needs no random bytes",
			stdin: strings.NewReader("only\n"),
			fsys:  testFS(),
			args:  []string{"--random-source=randempty"},
			want:  "only\n",
		},
		{
			name: "a single number range needs no random bytes",
			fsys: testFS(),
			args: []string{"-i", "5-5", "--random-source=randempty"},
			want: "5\n",
		},
		{
			name:     "a short random source fails",
			stdin:    strings.NewReader("a\nb\nc\nd\n"),
			fsys:     testFS(),
			args:     []string{"--random-source=randshort"},
			wantErr:  "shuf: randshort: end of file\n",
			wantExit: 1,
		},
		{
			name:  "a short random source is enough for two lines",
			stdin: strings.NewReader("a\nb\n"),
			fsys:  testFS(),
			args:  []string{"--random-source=randshort"},
			want:  "b\na\n",
		},
		{
			name:     "a missing random source",
			stdin:    strings.NewReader("a\nb\n"),
			fsys:     testFS(),
			args:     []string{"--random-source=missing"},
			wantErr:  "shuf: missing: file does not exist\n",
			wantExit: 1,
		},
		{
			name:     "two different random sources",
			stdin:    strings.NewReader("a\nb\n"),
			fsys:     testFS(),
			args:     []string{"--random-source=rand1", "--random-source=rand2"},
			wantErr:  "shuf: multiple random sources specified\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunZeroTerminated(t *testing.T) {
	t.Run("the items of the input are permuted", func(t *testing.T) {
		got := runShuf(t, strings.NewReader("x\x00z\x00y\x00"), nil, "", "-z")
		checkPermutation(t, got, "x\x00z\x00y\x00", 0)
	})

	tests := []testCase{
		{
			name:  "the output is zero terminated",
			stdin: strings.NewReader("a\x00b\x00c\x00"),
			args:  []string{"-z", "--random-source=rand1"},
			want:  "b\x00c\x00a\x00",
		},
		{
			name:  "a missing final zero byte",
			stdin: strings.NewReader("a\x00b\x00c"),
			args:  []string{"-z", "--random-source=rand1"},
			want:  "b\x00c\x00a\x00",
		},
		{
			name: "echo with zero termination",
			args: []string{"-z", "-e", "--random-source=rand1", "a", "b", "c"},
			want: "b\x00c\x00a\x00",
		},
		{
			name: "a range with zero termination",
			args: []string{"-z", "-i", "1-3", "--random-source=rand1"},
			want: "1\x002\x003\x00",
		},
		{
			name: "the long form",
			args: []string{"--zero-terminated", "-e", "--random-source=rand1", "a", "b", "c"},
			want: "b\x00c\x00a\x00",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunOptionForms(t *testing.T) {
	t.Run("grouped options", func(t *testing.T) {
		test := testCase{
			args: []string{"-rn2", "-e", "--random-source=rand1", "a", "b"},
			want: "a\na\n",
		}
		runCase(t, test)
	})

	t.Run("options stop at the first operand", func(t *testing.T) {
		test := testCase{
			fsys:     testFS(),
			args:     []string{"names.txt", "-z"},
			wantErr:  "shuf: extra operand '-z'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		}
		runCase(t, test)
	})

	t.Run("repeat with a count of the input", func(t *testing.T) {
		test := testCase{
			stdin: strings.NewReader("a\nb\nc\nd\n"),
			args:  []string{"-r", "-n", "2", "--random-source=rand1"},
			want:  "a\nc\n",
		}
		runCase(t, test)
	})
}

func TestRunErrors(t *testing.T) {
	tests := []testCase{
		{
			name:     "an invalid short option",
			args:     []string{"-x"},
			wantErr:  "shuf: invalid option -- 'x'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
		{
			name:     "an unrecognized long option",
			args:     []string{"--foo"},
			wantErr:  "shuf: unrecognized option '--foo'\nTry 'shuf --help' for more information.\n",
			wantExit: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runCase(t, test)
		})
	}
}

func TestRunInterrupted(t *testing.T) {
	t.Run("a done context stops reading the input", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		stdout := new(bytes.Buffer)
		rt := &coreutils.Runtime{
			Stdin:  &cancelReader{cancel: cancel},
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
		Stdout: errorWriter{err: errors.New("boom")},
		Stderr: stderr,
	}
	err := New(rt).Run(t.Context(), "-e", "a", "b", "c")
	if code := exitCode(t, err); code != 1 {
		t.Errorf("Run() exit code = %d, want 1", code)
	}
	if got, want := stderr.String(), "shuf: write error: boom\n"; got != want {
		t.Errorf("Run() stderr = %q, want %q", got, want)
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
		want := "shuf (charm.land/snail/coreutils) " + coreutils.Version + "\nhttps://charm.land/snail\n"
		if got := stdout.String(); got != want {
			t.Errorf("Run() stdout = %q, want %q", got, want)
		}
	})
}
