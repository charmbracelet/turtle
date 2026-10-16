package coreutils

import (
	"slices"
	"testing"
)

func TestSplitPath(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"", nil},
		{".", nil},
		{"a", []string{"a"}},
		{"a/b/c", []string{"a", "b", "c"}},
		{"a/b/", []string{"a", "b"}},
		{"a//b", []string{"a", "b"}},
		{"./a/./b", []string{"a", "b"}},
		{"/a/b", []string{"/", "a", "b"}},
		{"/", []string{"/"}},
		{"a/../b", []string{"a", "..", "b"}},
	}
	for _, test := range tests {
		if got := SplitPath(test.path); !slices.Equal(got, test.want) {
			t.Errorf("SplitPath(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}

func TestJoinPath(t *testing.T) {
	tests := []struct {
		comps []string
		want  string
	}{
		{nil, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b", "c"}, "a/b/c"},
		{[]string{"/"}, "/"},
		{[]string{"/", "a", "b"}, "/a/b"},
	}
	for _, test := range tests {
		if got := JoinPath(test.comps); got != test.want {
			t.Errorf("JoinPath(%v) = %q, want %q", test.comps, got, test.want)
		}
	}
}

func TestSplitJoinRoundTrip(t *testing.T) {
	for _, path := range []string{"a", "a/b", "/a/b"} {
		if got := JoinPath(SplitPath(path)); got != path {
			t.Errorf("JoinPath(SplitPath(%q)) = %q, want %q", path, got, path)
		}
	}
}
