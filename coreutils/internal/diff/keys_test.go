package diff

import (
	"strings"
	"testing"
)

func TestKeyOpts(t *testing.T) {
	tests := []struct {
		name  string
		opts  KeyOpts
		equal bool // whether the two lines compare equal under the key
		a, b  string
	}{
		{
			name:  "no options compare lines as they are",
			equal: true,
			a:     "a b\n", b: "a b\n",
		},
		{
			name:  "case is ignored with -i",
			opts:  KeyOpts{IgnoreCase: true},
			equal: true,
			a:     "a B c\n", b: "A b C\n",
		},
		{
			name:  "case is still significant without -i",
			equal: false,
			a:     "a B\n", b: "A b\n",
		},
		{
			name:  "all space is ignored with -w",
			opts:  KeyOpts{IgnoreAllSpace: true},
			equal: true,
			a:     "a b c\n", b: "abc\n",
		},
		{
			name:  "a missing blank still differs with -b",
			opts:  KeyOpts{IgnoreSpaceChange: true},
			equal: false,
			a:     "a b\n", b: "ab\n",
		},
		{
			name:  "the amount of blanks is ignored with -b",
			opts:  KeyOpts{IgnoreSpaceChange: true},
			equal: true,
			a:     "a b\t c\n", b: "a  b c\n",
		},
		{
			name:  "trailing blanks are ignored with -b",
			opts:  KeyOpts{IgnoreSpaceChange: true},
			equal: true,
			a:     "a \n", b: "a\n",
		},
		{
			name:  "tabs are expanded with -E",
			opts:  KeyOpts{IgnoreTabExpansion: true},
			equal: true,
			a:     "a\tb\n", b: "a       b\n",
		},
		{
			name:  "amount of spaces still differs with -E alone",
			opts:  KeyOpts{IgnoreTabExpansion: true},
			equal: false,
			a:     "a b\n", b: "a  b\n",
		},
		{
			name:  "tabs are expanded with a custom tab size",
			opts:  KeyOpts{IgnoreTabExpansion: true, TabSize: 4},
			equal: true,
			a:     "a\tb\n", b: "a   b\n",
		},
		{
			name:  "trailing space is ignored with -Z",
			opts:  KeyOpts{IgnoreTrailingSpace: true},
			equal: true,
			a:     "a \t\n", b: "a\n",
		},
		{
			name:  "amount of blanks still differs with -Z alone",
			opts:  KeyOpts{IgnoreTrailingSpace: true},
			equal: false,
			a:     "a b\n", b: "a  b\n",
		},
		{
			name:  "tab expansion and trailing space combine",
			opts:  KeyOpts{IgnoreTabExpansion: true, IgnoreTrailingSpace: true},
			equal: true,
			a:     "a\t \n", b: "a \t\n",
		},
		{
			name:  "case and space options combine",
			opts:  KeyOpts{IgnoreCase: true, IgnoreSpaceChange: true},
			equal: true,
			a:     "A\tb \n", b: "a  B\n",
		},
		{
			name:  "a missing final newline differs with -i",
			opts:  KeyOpts{IgnoreCase: true},
			equal: false,
			a:     "a\nB\n", b: "a\nB",
		},
		{
			name:  "a missing final newline on both sides compares equal",
			opts:  KeyOpts{IgnoreCase: true},
			equal: true,
			a:     "a\nB", b: "a\nb",
		},
		{
			name:  "a missing final newline differs with -E",
			opts:  KeyOpts{IgnoreTabExpansion: true},
			equal: false,
			a:     "a\n", b: "a",
		},
		{
			name:  "a missing final newline is a trailing blank with -Z",
			opts:  KeyOpts{IgnoreTrailingSpace: true},
			equal: true,
			a:     "a\n", b: "a",
		},
		{
			name:  "a missing final newline is a trailing blank with -b",
			opts:  KeyOpts{IgnoreSpaceChange: true},
			equal: true,
			a:     "a \n", b: "a",
		},
		{
			name:  "a missing final newline is a trailing blank with -w",
			opts:  KeyOpts{IgnoreAllSpace: true},
			equal: true,
			a:     "a \n", b: "a",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key := test.opts.Key()
			var gotA, gotB string
			if key != nil {
				gotA, gotB = key(test.a), key(test.b)
			} else {
				gotA, gotB = test.a, test.b
			}
			if equal := gotA == gotB; equal != test.equal {
				t.Errorf("Key() = %q and %q, equal = %v, want %v", gotA, gotB, equal, test.equal)
			}
		})
	}
}

func TestKeyOptsBlank(t *testing.T) {
	tests := []struct {
		name string
		opts KeyOpts
		line string
		want bool
	}{
		{name: "an empty line", line: "\n", want: true},
		{name: "a line of blanks is not blank by itself", line: " \t\n"},
		{name: "a line of blanks is blank with -w", opts: KeyOpts{IgnoreAllSpace: true}, line: " \t\n", want: true},
		{name: "a line of blanks is blank with -b", opts: KeyOpts{IgnoreSpaceChange: true}, line: " \t\n", want: true},
		{name: "a line of blanks is blank with -Z", opts: KeyOpts{IgnoreTrailingSpace: true}, line: " \t\n", want: true},
		{
			name: "a line of blanks is not blank with -E alone",
			opts: KeyOpts{IgnoreTabExpansion: true},
			line: "\t\n",
		},
		{
			name: "a missing final newline doesn't make a line blank",
			opts: KeyOpts{IgnoreCase: true},
			line: "b",
		},
		{
			name: "case doesn't make a line blank",
			opts: KeyOpts{IgnoreCase: true},
			line: "B\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.opts.Blank(test.line); got != test.want {
				t.Errorf("Blank() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestKeyOptsIdentity(t *testing.T) {
	if key := (KeyOpts{}).Key(); key != nil {
		t.Error("Key() is not nil for no options")
	}
	if key := (KeyOpts{IgnoreCase: true}).Key(); key == nil {
		t.Error("Key() = nil for -i")
	}
}

func TestExpandTabs(t *testing.T) {
	tests := []struct {
		in      string
		tabSize int
		want    string
	}{
		{in: "a\n", tabSize: 8, want: "a\n"},
		{in: "\t\n", tabSize: 8, want: "        \n"},
		{in: "a\tb\n", tabSize: 4, want: "a   b\n"},
		{in: "\r\ta\n", tabSize: 8, want: "\r        a\n"},
		{in: "a\b\tb\n", tabSize: 8, want: "a\b        b\n"},
	}
	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			if got := expandTabs(test.in, test.tabSize); got != test.want {
				t.Errorf("expandTabs() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWriteLinesOpts(t *testing.T) {
	lo := LineOpts{InitialTab: true, ExpandTabs: true, TabSize: 4}
	var out strings.Builder
	if err := writeLines(&out, "<", []string{"a\tb\n"}, lo); err != nil {
		t.Fatalf("writeLines() error = %v", err)
	}
	if want := "<\ta   b\n"; out.String() != want {
		t.Errorf("writeLines() = %q, want %q", out.String(), want)
	}
}

func TestWritePlainIncompleteLine(t *testing.T) {
	var out strings.Builder
	if err := writePlain(&out, "x", LineOpts{}); err != nil {
		t.Fatalf("writePlain() error = %v", err)
	}
	if out.String() != "x" {
		t.Errorf("writePlain() = %q, want %q", out.String(), "x")
	}
}
