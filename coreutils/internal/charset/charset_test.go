package charset

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Elem
	}{
		{
			name:  "ordinary characters",
			input: "abc",
			want: []Elem{
				{Kind: Char, Char: 'a'},
				{Kind: Char, Char: 'b'},
				{Kind: Char, Char: 'c'},
			},
		},
		{
			name:  "a range",
			input: "a-c",
			want: []Elem{
				{Kind: Char, Char: 'a'},
				{Kind: Char, Char: 'b'},
				{Kind: Char, Char: 'c'},
			},
		},
		{
			name:  "a trailing dash is ordinary",
			input: "a-",
			want: []Elem{
				{Kind: Char, Char: 'a'},
				{Kind: Char, Char: '-'},
			},
		},
		{
			name:  "a leading dash is ordinary",
			input: "-a",
			want: []Elem{
				{Kind: Char, Char: '-'},
				{Kind: Char, Char: 'a'},
			},
		},
		{
			name:  "a dash after a range is ordinary",
			input: "a-c-e",
			want: []Elem{
				{Kind: Char, Char: 'a'},
				{Kind: Char, Char: 'b'},
				{Kind: Char, Char: 'c'},
				{Kind: Char, Char: '-'},
				{Kind: Char, Char: 'e'},
			},
		},
		{
			name:  "an octal range",
			input: `\101-\103`,
			want: []Elem{
				{Kind: Char, Char: 'A', Raw: true},
				{Kind: Char, Char: 'B', Raw: true},
				{Kind: Char, Char: 'C', Raw: true},
			},
		},
		{
			name:  "escape sequences",
			input: `\t\n\\`,
			want: []Elem{
				{Kind: Char, Char: '\t'},
				{Kind: Char, Char: '\n'},
				{Kind: Char, Char: '\\'},
			},
		},
		{
			name:  "an octal sequence with three digits",
			input: `\1011`,
			want: []Elem{
				{Kind: Char, Char: 'A', Raw: true},
				{Kind: Char, Char: '1'},
			},
		},
		{
			name:  "a trailing backslash",
			input: `\`,
			want:  []Elem{{Kind: Char, Char: '\\'}},
		},
		{
			name:  "a character class",
			input: "[:alpha:]",
			want:  []Elem{{Kind: Class, Class: "alpha"}},
		},
		{
			name:  "a character class in brackets",
			input: "[[:alpha:]]",
			want: []Elem{
				{Kind: Char, Char: '['},
				{Kind: Class, Class: "alpha"},
				{Kind: Char, Char: ']'},
			},
		},
		{
			name:  "an equivalence class",
			input: "[=e=]",
			want:  []Elem{{Kind: Equiv, Char: 'e'}},
		},
		{
			name:  "an equivalence class in brackets",
			input: "[[=e=]]",
			want: []Elem{
				{Kind: Char, Char: '['},
				{Kind: Equiv, Char: 'e'},
				{Kind: Char, Char: ']'},
			},
		},
		{
			name:  "a repetition with a count",
			input: "[a*3]",
			want:  []Elem{{Kind: Repeat, Char: 'a', Repeat: 3}},
		},
		{
			name:  "a repetition without a count",
			input: "[a*]",
			want:  []Elem{{Kind: Repeat, Char: 'a', Repeat: -1}},
		},
		{
			name:  "a repetition with a zero count fills",
			input: "[a*0]",
			want:  []Elem{{Kind: Repeat, Char: 'a', Repeat: -1}},
		},
		{
			name:  "a repetition with an octal count",
			input: "[a*010]",
			want:  []Elem{{Kind: Repeat, Char: 'a', Repeat: 8}},
		},
		{
			name:  "a repetition of an escape",
			input: `[\n*]`,
			want:  []Elem{{Kind: Repeat, Char: '\n', Repeat: -1}},
		},
		{
			name:  "an unclosed repetition is ordinary",
			input: "[a*",
			want: []Elem{
				{Kind: Char, Char: '['},
				{Kind: Char, Char: 'a'},
				{Kind: Char, Char: '*'},
			},
		},
		{
			name:  "an unclosed class is ordinary",
			input: "[:alpha",
			want: []Elem{
				{Kind: Char, Char: '['},
				{Kind: Char, Char: ':'},
				{Kind: Char, Char: 'a'},
				{Kind: Char, Char: 'l'},
				{Kind: Char, Char: 'p'},
				{Kind: Char, Char: 'h'},
				{Kind: Char, Char: 'a'},
			},
		},
		{
			name:  "a multibyte character",
			input: "é",
			want:  []Elem{{Kind: Char, Char: 'é'}},
		},
		{
			name:  "an empty string",
			input: "",
			want:  nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Parse(test.input)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", test.input, err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("Parse(%q) = %v, want %v", test.input, got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Errorf("Parse(%q)[%d] = %+v, want %+v", test.input, i, got[i], test.want[i])
				}
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "an unknown character class",
			input:   "[:foo:]",
			wantErr: `invalid character class "foo"`,
		},
		{
			name:    "a reversed range",
			input:   "z-a",
			wantErr: `range-endpoints of "z-a" are in reverse collating sequence order`,
		},
		{
			name:    "a huge repetition count",
			input:   "[a*99999999]",
			wantErr: `repetition count "99999999" is too large`,
		},
		{
			name:    "an invalid octal repetition count",
			input:   "[a*08]",
			wantErr: `invalid repetition count "08"`,
		},
		{
			name:    "a repetition count which wraps around",
			input:   "[a*18446744073709551616]",
			wantErr: `repetition count "18446744073709551616" is too large`,
		},
		{
			name:    "a repetition count with more digits than an int",
			input:   "[a*99999999999999999999]",
			wantErr: `repetition count "99999999999999999999" is too large`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(test.input)
			if err == nil {
				t.Fatalf("Parse(%q) error = nil, want %q", test.input, test.wantErr)
			}
			if err.Error() != test.wantErr {
				t.Errorf("Parse(%q) error = %q, want %q", test.input, err, test.wantErr)
			}
		})
	}
}

func TestExpand(t *testing.T) {
	t.Run("characters in order", func(t *testing.T) {
		elems, err := Parse(`a-cx`)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Expand(elems, -1)
		if err != nil {
			t.Fatal(err)
		}
		want := "abcx"
		if chars(got) != want {
			t.Errorf("Expand() = %q, want %q", chars(got), want)
		}
	})

	t.Run("a fill repetition reaches the given length", func(t *testing.T) {
		elems, err := Parse(`x[\n*]`)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Expand(elems, 4)
		if err != nil {
			t.Fatal(err)
		}
		if chars(got) != "x\n\n\n" {
			t.Errorf("Expand() = %q, want %q", chars(got), "x\n\n\n")
		}
	})

	t.Run("a fill repetition without a length", func(t *testing.T) {
		elems, err := Parse(`[a*]`)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Expand(elems, -1); err == nil {
			t.Error("Expand() error = nil, want an error")
		}
	})

	t.Run("a class expands to its characters", func(t *testing.T) {
		elems, err := Parse(`[:lower:]`)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Expand(elems, -1)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(chars(got), "abcdefghijklmnopqrstuvwxyz") {
			t.Errorf("Expand() = %q..., want it to start with the ASCII lowercase", chars(got))
		}
	})

	t.Run("an equivalence class holds its character", func(t *testing.T) {
		elems, err := Parse(`[=e=]`)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Expand(elems, -1)
		if err != nil {
			t.Fatal(err)
		}
		if chars(got) != "e" {
			t.Errorf("Expand() = %q, want %q", chars(got), "e")
		}
	})
}

func TestCasePairs(t *testing.T) {
	t.Run("from lower to upper", func(t *testing.T) {
		from, to := CasePairs(true)
		if got := pairOf(from, to, 'a'); got != 'A' {
			t.Errorf("pair of 'a' = %q, want 'A'", got)
		}
		if got := pairOf(from, to, 'é'); got != 'É' {
			t.Errorf("pair of 'é' = %q, want 'É'", got)
		}
		if pairOf(from, to, 'A') != 0 {
			t.Error("pair of 'A' exists, want none")
		}
	})

	t.Run("from upper to lower", func(t *testing.T) {
		from, to := CasePairs(false)
		if got := pairOf(from, to, 'A'); got != 'a' {
			t.Errorf("pair of 'A' = %q, want 'a'", got)
		}
		if got := pairOf(from, to, 'É'); got != 'é' {
			t.Errorf("pair of 'É' = %q, want 'é'", got)
		}
	})
}

func pairOf(from, to []rune, r rune) rune {
	for i, f := range from {
		if f == r {
			return to[i]
		}
	}
	return 0
}

func chars(members []Member) string {
	var b strings.Builder
	for _, m := range members {
		b.WriteRune(m.Char)
	}
	return b.String()
}
