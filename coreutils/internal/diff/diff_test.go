package diff

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

func TestLines(t *testing.T) {
	blank := BlankKey
	tests := []struct {
		name string
		a, b string
		key  func(string) string
		want []Change
	}{
		{
			name: "empty files",
			a:    "", b: "",
		},
		{
			name: "identical files",
			a:    "a\nb\nc\n", b: "a\nb\nc\n",
		},
		{
			name: "no newline at all",
			a:    "ab", b: "ab",
		},
		{
			name: "insertion into empty",
			a:    "", b: "x\n",
			want: []Change{{A0: 0, A1: 0, B0: 0, B1: 1}},
		},
		{
			name: "insertion of a line without newline",
			a:    "", b: "x",
			want: []Change{{A0: 0, A1: 0, B0: 0, B1: 1}},
		},
		{
			name: "deletion to empty",
			a:    "x\n", b: "",
			want: []Change{{A0: 0, A1: 1, B0: 0, B1: 0}},
		},
		{
			name: "insertion in the middle",
			a:    "a\nb\nc\n", b: "a\nb\nx\nc\n",
			want: []Change{{A0: 2, A1: 2, B0: 2, B1: 3}},
		},
		{
			name: "deletion in the middle",
			a:    "a\nb\nx\nc\n", b: "a\nb\nc\n",
			want: []Change{{A0: 2, A1: 3, B0: 2, B1: 2}},
		},
		{
			name: "changed first line",
			a:    "a\nb\n", b: "c\nb\n",
			want: []Change{{A0: 0, A1: 1, B0: 0, B1: 1}},
		},
		{
			name: "changed last line",
			a:    "a\nb\n", b: "a\nc\n",
			want: []Change{{A0: 1, A1: 2, B0: 1, B1: 2}},
		},
		{
			name: "changed line without newline",
			a:    "a\nb", b: "a\nc",
			want: []Change{{A0: 1, A1: 2, B0: 1, B1: 2}},
		},
		{
			name: "missing newline at end of file",
			a:    "a\nb\n", b: "a\nb",
			want: []Change{{A0: 1, A1: 2, B0: 1, B1: 2}},
		},
		{
			name: "added newline at end of file",
			a:    "a\nb", b: "a\nb\n",
			want: []Change{{A0: 1, A1: 2, B0: 1, B1: 2}},
		},
		{
			name: "deleted head and added tail",
			a:    "a\nb\nc\n", b: "b\nc\nd\n",
			want: []Change{
				{A0: 0, A1: 1, B0: 0, B1: 0},
				{A0: 3, A1: 3, B0: 2, B1: 3},
			},
		},
		{
			name: "repeated lines",
			a:    "a\na\na\n", b: "a\na\n",
			want: []Change{{A0: 2, A1: 3, B0: 2, B1: 2}},
		},
		{
			name: "two separated changes",
			a:    "a\nb\nc\nd\ne\n", b: "a\nx\nc\nd\ny\n",
			want: []Change{
				{A0: 1, A1: 2, B0: 1, B1: 2},
				{A0: 4, A1: 5, B0: 4, B1: 5},
			},
		},
		{
			name: "swap of two lines",
			a:    "a\nb\n", b: "b\na\n",
			want: []Change{
				{A0: 0, A1: 1, B0: 0, B1: 0},
				{A0: 2, A1: 2, B0: 1, B1: 2},
			},
		},
		{
			name: "blank amount ignored with key",
			a:    "x y\nz\t\tw\n", b: "x  y\nz w\n",
			key: blank,
		},
		{
			name: "trailing blanks ignored with key",
			a:    "x \n", b: "x\n",
			key: blank,
		},
		{
			name: "missing blanks still differ with key",
			a:    "x y\n", b: "xy\n",
			key:  blank,
			want: []Change{{A0: 0, A1: 1, B0: 0, B1: 1}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, b := SplitLines([]byte(test.a)), SplitLines([]byte(test.b))
			got := Lines(a, b, test.key)
			if !slices.Equal(got, test.want) {
				t.Errorf("Lines() = %+v, want %+v", got, test.want)
			}
			// A key that ignores parts of the lines makes the equal runs come
			// from a, so only plain comparisons rebuild b.
			if test.key == nil {
				if out := Apply(a, b, got); !slices.Equal(out, b) {
					t.Errorf("Apply() = %q, want %q", out, b)
				}
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{in: ""},
		{in: "a", want: []string{"a"}},
		{in: "a\n", want: []string{"a\n"}},
		{in: "\n", want: []string{"\n"}},
		{in: "\n\n", want: []string{"\n", "\n"}},
		{in: "a\nb", want: []string{"a\n", "b"}},
		{in: "a\nb\n", want: []string{"a\n", "b\n"}},
		{in: "a\n\nb\n", want: []string{"a\n", "\n", "b\n"}},
	}
	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			if got := SplitLines([]byte(test.in)); !slices.Equal(got, test.want) {
				t.Errorf("SplitLines() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBlankKey(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{in: "", want: ""},
		{in: "\n", want: ""},
		{in: " \t \n", want: ""},
		{in: "a", want: "a"},
		{in: "a\n", want: "a"},
		{in: "a \n", want: "a"},
		{in: "a\t\n", want: "a"},
		{in: " a", want: " a"},
		{in: "  a", want: " a"},
		{in: "a  b\n", want: "a b"},
		{in: "a\t\tb", want: "a b"},
		{in: "a b c\n", want: "a b c"},
		{in: " a  b \t\n", want: " a b"},
	}
	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			if got := BlankKey(test.in); got != test.want {
				t.Errorf("BlankKey() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestLinesMinimal checks random inputs against a brute-force oracle: the
// script must rebuild b from a and be as short as the longest common
// subsequence allows.
func TestLinesMinimal(t *testing.T) {
	rnd := rand.New(rand.NewPCG(1, 2))
	for _, key := range []struct {
		name string
		fn   func(string) string
	}{
		{name: "plain"},
		{name: "blank key", fn: BlankKey},
	} {
		t.Run(key.name, func(t *testing.T) {
			for range 3000 {
				a := randomLines(rnd, key.fn != nil)
				b := randomLines(rnd, key.fn != nil)
				changes := Lines(a, b, key.fn)

				if key.fn == nil {
					if out := Apply(a, b, changes); !slices.Equal(out, b) {
						t.Fatalf("Apply(%q, %q, %+v) = %q", a, b, changes, out)
					}
				}
				xa, xb := pack(a, b, key.fn)
				if got, want := edits(changes), len(a)+len(b)-2*lcsLen(xa, xb); got != want {
					t.Fatalf("edits of %q and %q = %d, want %d (changes %+v)", a, b, got, want, changes)
				}
			}
		})
	}
}

// TestLinesTooDifferent checks the fallback when the search gives up: the
// whole comparison turns into one change, which is still a valid script.
func TestLinesTooDifferent(t *testing.T) {
	a := numberedLines(1, 5000)
	b := numberedLines(5001, 5000)
	want := []Change{{A0: 0, A1: 5000, B0: 0, B1: 5000}}

	got := Lines(a, b, nil)
	if !slices.Equal(got, want) {
		t.Errorf("Lines() = %+v, want %+v", got, want)
	}
	if out := Apply(a, b, got); !slices.Equal(out, b) {
		t.Errorf("Apply() rebuilt %d lines, want %d", len(out), len(b))
	}
}

// TestLinesLarge checks a large input with scattered changes stays valid.
func TestLinesLarge(t *testing.T) {
	a := numberedLines(1, 20000)
	b := slices.Clone(a)
	for i := range 500 {
		b[4*i+3] = "changed\n"
	}

	changes := Lines(a, b, nil)
	if out := Apply(a, b, changes); !slices.Equal(out, b) {
		t.Errorf("Apply() rebuilt %d of %d lines correctly", countEqual(out, b), len(b))
	}
}

// TestLinesFallbackMinimal checks the fallback still finds a minimal script
// when nothing matches, since replacing everything is then optimal.
func TestLinesFallbackMinimal(t *testing.T) {
	a := numberedLines(1, 5000)
	b := numberedLines(5001, 5000)

	if got, want := edits(Lines(a, b, nil)), 10000; got != want {
		t.Errorf("edits() = %d, want %d", got, want)
	}
}

func randomLines(rnd *rand.Rand, blanks bool) []string {
	lines := make([]string, rnd.IntN(15))
	for i := range lines {
		if blanks {
			lines[i] = randomBlankLine(rnd)
		} else {
			lines[i] = randomPlainLine(rnd)
		}
	}
	return lines
}

func randomPlainLine(rnd *rand.Rand) string {
	return string(rune('a' + rnd.IntN(4)))
}

func randomBlankLine(rnd *rand.Rand) string {
	var b strings.Builder
	if rnd.IntN(2) == 0 {
		b.WriteString(strings.Repeat(" \t", 1+rnd.IntN(2)))
	}
	for range 1 + rnd.IntN(3) {
		b.WriteByte(byte('x' + rnd.IntN(2)))
		if rnd.IntN(2) == 0 {
			b.WriteString(strings.Repeat(" \t", 1+rnd.IntN(3)))
		}
	}
	if rnd.IntN(2) == 0 {
		b.WriteString(strings.Repeat(" \t", 1+rnd.IntN(2)))
	}
	return b.String()
}

// numberedLines returns n lines numbered from start.
func numberedLines(start, n int) []string {
	lines := make([]string, n)
	for i := range n {
		lines[i] = itoa(start+i) + "\n"
	}
	return lines
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func edits(changes []Change) int {
	n := 0
	for _, c := range changes {
		n += c.A1 - c.A0 + c.B1 - c.B0
	}
	return n
}

func lcsLen(a, b []int) int {
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := range len(a) {
		for j := range len(b) {
			if a[i] == b[j] {
				curr[j+1] = prev[j] + 1
			} else {
				curr[j+1] = max(prev[j+1], curr[j])
			}
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func countEqual(a, b []string) int {
	n := 0
	for i := range min(len(a), len(b)) {
		if a[i] == b[i] {
			n++
		}
	}
	return n
}
