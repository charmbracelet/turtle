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
