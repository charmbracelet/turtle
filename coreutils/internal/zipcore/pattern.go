package zipcore

import (
	"regexp"
	"strings"

	"charm.land/turtle/coreutils/internal/glob"
)

// Pattern matches the member names of an archive the way the operands of the
// ZIP utilities do: a literal name matches exactly, a wildcard pattern
// matches the whole name with '*' and '?' also matching the directory
// separator, and the trailing slash of a directory entry is optional for the
// wildcards.
type Pattern struct {
	name   string
	regexp *regexp.Regexp
}

// CompilePattern compiles one member pattern.
func CompilePattern(pattern string) *Pattern {
	if !strings.ContainsAny(pattern, "*?[\\") {
		return &Pattern{name: pattern}
	}
	re, err := glob.Compile(pattern, false)
	if err != nil {
		return &Pattern{name: pattern}
	}
	return &Pattern{regexp: re}
}

// Matches reports whether name matches the pattern.
func (p *Pattern) Matches(name string) bool {
	if p.regexp == nil {
		return name == p.name
	}
	if p.regexp.MatchString(name) {
		return true
	}
	return strings.HasSuffix(name, "/") && p.regexp.MatchString(strings.TrimSuffix(name, "/"))
}
