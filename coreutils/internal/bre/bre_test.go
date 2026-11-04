package bre

import (
	"testing"
)

func TestCompile(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    string
		wantErr string
	}{
		{name: "literal", pattern: "abc", want: "abc"},
		{name: "dot", pattern: "a.c", want: "a.c"},
		{name: "star quantifier", pattern: "ab*c", want: "ab*c"},
		{name: "star at start is ordinary", pattern: "*abc", want: `\*abc`},
		{name: "star after anchor is ordinary", pattern: "^*abc", want: `^\*abc`},
		{name: "star after star is ordinary", pattern: "a**", want: `a*\*`},
		{name: "anchors", pattern: "^abc$", want: "^abc$"},
		{name: "caret mid pattern is ordinary", pattern: "a^b", want: `a\^b`},
		{name: "dollar mid pattern is ordinary", pattern: "a$b", want: `a\$b`},
		{name: "group", pattern: `\(ab\)*c`, want: "(ab)*c"},
		{name: "anchor after group start", pattern: `\(^a\)*$`, want: "(^a)*$"},
		{name: "plain parentheses are ordinary", pattern: "a(b)c", want: `a\(b\)c`},
		{name: "interval", pattern: `a\{2,3\}`, want: "a{2,3}"},
		{name: "open interval", pattern: `a\{2,\}`, want: "a{2,}"},
		{name: "exact interval", pattern: `a\{2\}`, want: "a{2}"},
		{name: "interval without atom is ordinary", pattern: `\{2\}`, want: `\{2\}`},
		{name: "invalid interval is ordinary", pattern: `a\{x\}`, want: `a\{x\}`},
		{name: "unterminated interval is ordinary", pattern: `a\{2`, want: `a\{2`},
		{name: "plain braces are ordinary", pattern: "a{2}b", want: `a\{2\}b`},
		{name: "plus question bar are ordinary", pattern: "a+b?c|d", want: `a\+b\?c\|d`},
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
		{name: "unmatched group close", pattern: `a\)`, wantErr: `unmatched \)`},
		{name: "unmatched group open", pattern: `\(a`, wantErr: `unmatched \(`},
		{name: "backreference", pattern: `\(a\)\1`, wantErr: "backreferences are not supported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := translate(test.pattern)
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("translate(%q) error = %v, want %q", test.pattern, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("translate(%q) error = %v", test.pattern, err)
			}
			if got != test.want {
				t.Errorf("translate(%q) = %q, want %q", test.pattern, got, test.want)
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
		{`a\{2\}`, "aa", true},
		{`a\{2\}`, "a", false},
		{`\(ab\)\{2\}`, "abab", true},
		{`x*`, "anything", true},
		{`[0-9]\{3\}`, "a123b", true},
		{"[[:digit:]]", "x0y", true},
		{"[[:digit:]]", "xyz", false},
		{"[[:digit:]x]", "x", true},
		{"[]]", "]", true},
		{"[]]", "a", false},
		{"a b", "a b", true},
		{"a$b", "a$b", true},
		{"a$b", "ab", false},
	}
	for _, test := range tests {
		re, err := Compile(test.pattern)
		if err != nil {
			t.Fatalf("Compile(%q) error = %v", test.pattern, err)
		}
		if got := re.MatchString(test.text); got != test.want {
			t.Errorf("Compile(%q).MatchString(%q) = %v, want %v", test.pattern, test.text, got, test.want)
		}
	}
}
