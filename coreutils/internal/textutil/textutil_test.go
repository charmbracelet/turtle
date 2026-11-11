package textutil

import (
	"context"
	"errors"
	"strings"
	"testing"
)

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

func TestDelim(t *testing.T) {
	if got := Delim(false); got != '\n' {
		t.Errorf("Delim(false) = %q, want newline", got)
	}
	if got := Delim(true); got != 0 {
		t.Errorf("Delim(true) = %q, want NUL", got)
	}
}

func TestEach(t *testing.T) {
	tests := []struct {
		name  string
		input string
		sep   byte
		want  []string
	}{
		{name: "newlines", input: "ab\ncd\nef", sep: '\n', want: []string{"ab\n", "cd\n", "ef"}},
		{name: "all terminated", input: "ab\ncd\n", sep: '\n', want: []string{"ab\n", "cd\n"}},
		{name: "empty", input: "", sep: '\n', want: nil},
		{name: "nuls", input: "ab\x00cd\x00\x00", sep: 0, want: []string{"ab\x00", "cd\x00", "\x00"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got []string
			err := Each(t.Context(), strings.NewReader(test.input), test.sep, func(rec []byte) error {
				got = append(got, string(rec))
				return nil
			})
			if err != nil {
				t.Fatalf("Each: %v", err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("Each called fn with %q, want %q", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Errorf("record %d = %q, want %q", i, got[i], test.want[i])
				}
			}
		})
	}

	t.Run("fn error", func(t *testing.T) {
		boom := errors.New("boom")
		count := 0
		err := Each(t.Context(), strings.NewReader("a\nb\nc\n"), '\n', func(rec []byte) error {
			count++
			if count == 2 {
				return boom
			}
			return nil
		})
		if !errors.Is(err, boom) {
			t.Errorf("Each returned %v, want %v", err, boom)
		}
		if count != 2 {
			t.Errorf("fn called %d times, want 2", count)
		}
	})

	t.Run("context done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := Each(ctx, strings.NewReader("a\nb\n"), '\n', func(rec []byte) error {
			return nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Each returned %v, want %v", err, context.Canceled)
		}
	})
}
