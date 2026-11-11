package coreutils

import (
	"path"
	"path/filepath"
	"strings"
)

// FsPath resolves name relative to the working directory, cleaned and with
// forward slashes, as the file system expects it. An empty name stays empty.
func (r *Runtime) FsPath(name string) string {
	if name == "" {
		return ""
	}
	return path.Clean(filepath.ToSlash(r.ResolvePath(name)))
}

// FsName resolves name relative to the working directory with forward
// slashes, keeping rooted names as they are.
func (r *Runtime) FsName(name string) string {
	return filepath.ToSlash(r.ResolvePath(name))
}

// FsBase returns the last element of name, treating backslashes as separators.
func FsBase(name string) string {
	return path.Base(filepath.ToSlash(name))
}

// FsAbs makes name absolute relative to the working directory, with forward
// slashes but without cleaning it.
func (r *Runtime) FsAbs(name string) string {
	if filepath.IsAbs(name) {
		// A Windows drive-absolute name stays absolute; its slash form
		// doesn't look absolute to path.IsAbs.
		return filepath.ToSlash(name)
	}
	name = filepath.ToSlash(name)
	if path.IsAbs(name) || r.WorkingDir == "" {
		return name
	}
	wd := filepath.ToSlash(r.WorkingDir)
	if strings.HasSuffix(wd, "/") {
		return wd + name
	}
	return wd + "/" + name
}

// SplitPath splits path into its components, dropping redundant separators
// and "." components. A leading "/" is kept as the first component, and an
// empty path returns no components.
func SplitPath(p string) []string {
	if p == "" {
		return nil
	}
	var comps []string
	if p[0] == '/' {
		comps = append(comps, "/")
	}
	for comp := range strings.SplitSeq(p, "/") {
		if comp != "" && comp != "." {
			comps = append(comps, comp)
		}
	}
	return comps
}

// JoinPath joins components, as returned by SplitPath, back into a path.
func JoinPath(comps []string) string {
	if len(comps) == 0 {
		return ""
	}
	p := comps[0]
	for _, comp := range comps[1:] {
		if p != "/" {
			p += "/"
		}
		p += comp
	}
	return p
}
