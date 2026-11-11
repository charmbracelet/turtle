package coreutils

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
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

func TestFsPath(t *testing.T) {
	tests := []struct {
		wd, name, want string
	}{
		{"", "a/b", "a/b"},
		{"", "/a/b", "/a/b"},
		{"work", "a/b", "work/a/b"},
		{"work", "a//b", "work/a/b"},
		{"work", "a/./b", "work/a/b"},
		{"work", "a/../b", "work/b"},
		{"work", "/a/b", "/a/b"},
		{"work", "", ""},
	}
	for _, test := range tests {
		if runtime.GOOS == "windows" && strings.HasPrefix(test.name, "/") {
			// A leading slash doesn't root a name on Windows, where the
			// runtime joins it onto the working directory like fsx does.
			test.want = filepath.ToSlash(filepath.Join(test.wd, test.name))
		}
		r := &Runtime{WorkingDir: test.wd}
		if got := r.FsPath(test.name); got != test.want {
			t.Errorf("FsPath(%q) with wd %q = %q, want %q", test.name, test.wd, got, test.want)
		}
	}
}

func TestFsBase(t *testing.T) {
	tests := []struct {
		name, want string
	}{
		{"", "."},
		{"a", "a"},
		{"a/b", "b"},
		{"a/b/", "b"},
		{"a//b", "b"},
		{"/a/b", "b"},
	}
	for _, test := range tests {
		if got := FsBase(test.name); got != test.want {
			t.Errorf("FsBase(%q) = %q, want %q", test.name, got, test.want)
		}
	}
	if runtime.GOOS == "windows" {
		if got := FsBase(`a\b`); got != "b" {
			t.Errorf("FsBase(%q) = %q, want %q", `a\b`, got, "b")
		}
	}
}

func TestFsAbs(t *testing.T) {
	tests := []struct {
		wd, name, want string
	}{
		{"", "a/b", "a/b"},
		{"", "/a/b", "/a/b"},
		{"work", "a/b", "work/a/b"},
		{"work", "a//b", "work/a//b"},
		{"work", "a/../b", "work/a/../b"},
		{"work", "/a/b", "/a/b"},
		{".", "a/b", "./a/b"},
	}
	for _, test := range tests {
		r := &Runtime{WorkingDir: test.wd}
		if got := r.FsAbs(test.name); got != test.want {
			t.Errorf("FsAbs(%q) with wd %q = %q, want %q", test.name, test.wd, got, test.want)
		}
	}
	if runtime.GOOS == "windows" {
		// A drive-absolute operand is absolute before the separators
		// are normalized, and keeps its volume.
		r := &Runtime{WorkingDir: "work"}
		if got := r.FsAbs(`C:\dir\file`); got != "C:/dir/file" {
			t.Errorf("FsAbs(%q) = %q, want %q", `C:\dir\file`, got, "C:/dir/file")
		}
	}
}

func TestFsName(t *testing.T) {
	tests := []struct {
		wd, name, want string
	}{
		{"", "", ""},
		{"", "a/b", "a/b"},
		{"", "a/../b", "a/../b"},
		{"work", "", "work"},
		{"work", "a/b", "work/a/b"},
		{"work", "a//b", "work/a/b"},
		{"work", "a/b/", "work/a/b"},
		{"work", "a/../b", "work/b"},
		{".", "a/b", "a/b"},
	}
	for _, test := range tests {
		r := &Runtime{WorkingDir: test.wd}
		if got := r.FsName(test.name); got != test.want {
			t.Errorf("FsName(%q) with wd %q = %q, want %q", test.name, test.wd, got, test.want)
		}
	}

	t.Run("rooted names stay as they are", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("a leading slash alone isn't absolute to filepath.IsAbs")
		}
		tests := []struct{ name, want string }{
			{"/a/b", "/a/b"},
			{"/a/b/", "/a/b/"},
			{"/a/../b", "/a/../b"},
			{"//a", "//a"},
		}
		for _, test := range tests {
			r := &Runtime{WorkingDir: "work"}
			if got := r.FsName(test.name); got != test.want {
				t.Errorf("FsName(%q) = %q, want %q", test.name, got, test.want)
			}
		}
	})
}
