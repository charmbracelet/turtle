package textutil

import "testing"

func TestIsBlank(t *testing.T) {
	for _, c := range []byte("abc1.\xff") {
		if IsBlank(c) {
			t.Errorf("IsBlank(%q) = true, want false", c)
		}
	}
	for _, c := range []byte(" \t") {
		if !IsBlank(c) {
			t.Errorf("IsBlank(%q) = false, want true", c)
		}
	}
}

func TestSkipFields(t *testing.T) {
	tests := []struct {
		name string
		line string
		n    int
		want string
	}{
		{name: "no fields skipped", line: "a b c", n: 0, want: "a b c"},
		{name: "one field", line: "a b c", n: 1, want: " b c"},
		{name: "a field with leading blanks", line: "  a b", n: 1, want: " b"},
		{name: "blank runs belong to the next field", line: "a  b", n: 1, want: "  b"},
		{name: "more fields than the line has", line: "a b", n: 3, want: ""},
		{name: "an empty line", line: "", n: 1, want: ""},
		{name: "only blanks", line: "   ", n: 1, want: ""},
		{name: "tabs are blanks", line: "a\tb", n: 1, want: "\tb"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SkipFields([]byte(test.line), test.n); string(got) != test.want {
				t.Errorf("SkipFields(%q, %d) = %q, want %q", test.line, test.n, got, test.want)
			}
		})
	}
}

func TestSkipChars(t *testing.T) {
	tests := []struct {
		name string
		line string
		n    int
		want string
	}{
		{name: "no characters skipped", line: "abc", n: 0, want: "abc"},
		{name: "one character", line: "abc", n: 1, want: "bc"},
		{name: "a multibyte character", line: "ábç", n: 1, want: "bç"},
		{name: "two multibyte characters", line: "日本語x", n: 2, want: "語x"},
		{name: "an invalid byte is one character", line: "a\xffb", n: 2, want: "b"},
		{name: "more characters than the line has", line: "ab", n: 3, want: ""},
		{name: "an empty line", line: "", n: 1, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SkipChars([]byte(test.line), test.n); string(got) != test.want {
				t.Errorf("SkipChars(%q, %d) = %q, want %q", test.line, test.n, got, test.want)
			}
		})
	}
}

func TestFields(t *testing.T) {
	tests := []struct {
		name string
		line string
		sep  string
		want []string
	}{
		{name: "blank separated fields", line: "a b c", want: []string{"a", "b", "c"}},
		{name: "leading blanks are ignored", line: "  a b", want: []string{"a", "b"}},
		{name: "a blank run is one separator", line: "a  \t b", want: []string{"a", "b"}},
		{name: "an empty line", line: "", want: nil},
		{name: "only blanks", line: "  ", want: nil},
		{name: "fields without a separator", line: "a", want: []string{"a"}},
		{name: "a separator splits every time", line: "a:b:c", sep: ":", want: []string{"a", "b", "c"}},
		{name: "leading separators count", line: ":a", sep: ":", want: []string{"", "a"}},
		{name: "trailing separators count", line: "a:", sep: ":", want: []string{"a", ""}},
		{name: "empty fields between separators", line: "a::b", sep: ":", want: []string{"a", "", "b"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sep []byte
			if test.sep != "" {
				sep = []byte(test.sep)
			}
			got := Fields([]byte(test.line), sep)
			if len(got) != len(test.want) {
				t.Fatalf("Fields(%q, %q) = %v, want %v", test.line, test.sep, got, test.want)
			}
			for i := range got {
				if string(got[i]) != test.want[i] {
					t.Errorf("Fields(%q, %q)[%d] = %q, want %q", test.line, test.sep, i, got[i], test.want[i])
				}
			}
		})
	}
}
