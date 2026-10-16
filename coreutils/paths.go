package coreutils

import "strings"

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
