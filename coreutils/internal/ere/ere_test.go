package ere

import (
	"testing"
)

func TestTranslate(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    string
		wantErr string
	}{
		{name: "literal", pattern: "abc", want: "abc"},
		{name: "dot", pattern: "a.c", want: "a.c"},
		{name: "star quantifier", pattern: "ab*c", want: "ab*c"},
		{name: "plus question quantifiers", pattern: "ab+c?d", want: "ab+c?d"},
		{name: "alternation", pattern: "abc|def", want: "abc|def"},
		{name: "group", pattern: "(ab)*c", want: "(ab)*c"},
		{name: "anchors", pattern: "^abc$", want: "^abc$"},
		{name: "anchors mid pattern", pattern: "a^b$c", want: "a^b$c"},
		{name: "interval", pattern: "a{2,3}", want: "a{2,3}"},
		{name: "open interval", pattern: "a{2,}", want: "a{2,}"},
		{name: "exact interval", pattern: "a{2}", want: "a{2}"},
		{name: "interval without atom is ordinary", pattern: "a|{2}", want: `a|\{2}`},
		{name: "unterminated interval", pattern: "a{2", wantErr: `unmatched \{`},
		{name: "invalid interval", pattern: "a{x}", wantErr: `unmatched \{`},
		{name: "empty interval", pattern: "a{}", wantErr: `unmatched \{`},
		{name: "brace without atom is ordinary", pattern: "{2}", want: `\{2}`},
		{name: "escaped brace is ordinary", pattern: `a\{b`, want: `a\{b`},
		{name: "escaped group is ordinary", pattern: `a\(b\)c`, want: `a\(b\)c`},
		{name: "escaped plus is ordinary", pattern: `a\+b`, want: `a\+b`},
		{name: "escaped dot is ordinary", pattern: `a\.c`, want: `a\.c`},
		{name: "escaped slash loses the backslash", pattern: `a\/b`, want: "a/b"},
		{name: "escaped ordinary letter keeps the backslash", pattern: `a\nb`, want: `a\nb`},
		{name: "trailing backslash is ordinary", pattern: `a\`, want: `a\\`},
		{name: "brackets", pattern: "[a-z]", want: "[a-z]"},
		{name: "brackets with caret", pattern: "[^a]", want: "[^a]"},
		{name: "bracket with literal close", pattern: "[]a]", want: `[\]a]`},
		{name: "bracket with a literal close alone", pattern: "[]]", want: `[\]]`},
		{name: "bracket with class", pattern: "[[:alpha:]]", want: "[[:alpha:]]"},
		{name: "bracket with digit class", pattern: "[[:digit:]]", want: "[[:digit:]]"},
		{name: "bracket with class and members", pattern: "[[:digit:]x]", want: "[[:digit:]x]"},
		{name: "bracket with collating symbol", pattern: "[[.a.]]", want: "[a]"},
		{name: "bracket with equivalence class", pattern: "[[=a=]]", want: "[a]"},
		{name: "multi-character collating symbol", pattern: "[[.ab.]]", wantErr: "invalid collating element [.ab.]"},
		{name: "backslash in brackets is ordinary", pattern: `[a\]b]`, want: `[a\\]b]`},
		{name: "unmatched open bracket", pattern: "[a", wantErr: "unmatched ["},
		{name: "backreference", pattern: `(a)\1`, wantErr: "backreferences are not supported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Translate(test.pattern)
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("Translate(%q) error = %v, want %q", test.pattern, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Translate(%q) error = %v", test.pattern, err)
			}
			if got != test.want {
				t.Errorf("Translate(%q) = %q, want %q", test.pattern, got, test.want)
			}
			if _, err := Compile(test.pattern); err != nil {
				t.Errorf("Compile(%q) error = %v", test.pattern, err)
			}
		})
	}
}

func TestCompileMatches(t *testing.T) {
	tests := []struct {
		pattern string
		text    string
		want    bool
	}{
		{`^foo`, "foobar", true},
		{`^foo`, "barfoo", false},
		{`foo$`, "barfoo", true},
		{`foo$`, "foobar", false},
		{`a{2}`, "aa", true},
		{`a{2}`, "a", false},
		{`(ab){2}`, "abab", true},
		{`x*`, "anything", true},
		{`[0-9]{3}`, "a123b", true},
		{`[0-9]{3}`, "a12b", false},
		{`foo|bar`, "xbarx", true},
		{`foo|bar`, "xbazx", false},
		{`colou?r`, "color", true},
		{`colou?r`, "colour", true},
		{`a\.b`, "a.b", true},
		{`a\.b`, "axb", false},
		{`a\/b`, "a/b", true},
		{`(x)*y`, "xxxy", true},
	}
	for _, test := range tests {
		re, err := Compile(test.pattern)
		if err != nil {
			t.Fatalf("Compile(%q) error = %v", test.pattern, err)
		}
		if got := re.MatchString(test.text); got != test.want {
			t.Errorf("Compile(%q).Match(%q) = %v, want %v", test.pattern, test.text, got, test.want)
		}
	}
}

func TestCompileInvalid(t *testing.T) {
	for _, pattern := range []string{"(a", "a)", "[", "a{2,1}", "*a", "+a", "?a"} {
		if _, err := Compile(pattern); err == nil {
			t.Errorf("Compile(%q) succeeded, want an error", pattern)
		}
	}
}
