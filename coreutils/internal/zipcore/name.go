package zipcore

import (
	"errors"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// ErrDotDot reports a member name with a ".." component, which an extraction
// refuses: it could write outside the directory it extracts into.
var ErrDotDot = errors.New(`skipped "../" path component(s)`)

// MemberName returns the name an archive entry gets for the operand name:
// forward slashes, no redundant components, and no leading slash, so an
// absolute operand doesn't store one.
func MemberName(name string) string {
	name = path.Clean(filepath.ToSlash(name))
	if name == "." || name == "/" {
		return ""
	}
	return strings.TrimPrefix(name, "/")
}

// SafeName turns the name of an archive entry into a path which can't escape
// the directory the entry is extracted into: backslashes are separators, the
// leading slashes are dropped, a Windows drive prefix is dropped too, since
// it would name a drive absolute there, and any ".." component refuses the
// name. The empty name, for the root of an archive, is reported by the ""
// return.
func SafeName(name string) (string, error) {
	// Backslashes are separators on Windows: they're normalized everywhere,
	// before the component checks, or a member named "..\x" would escape.
	name = strings.TrimLeft(strings.ReplaceAll(name, "\\", "/"), "/")
	if slices.Contains(strings.Split(name, "/"), "..") {
		return "", ErrDotDot
	}
	// The cleaning comes before the drive check, so a member named
	// "./C:/x" can't reach the drive it names on Windows.
	name = path.Clean(name)
	if volume := driveVolume(name); volume != "" {
		name = strings.TrimLeft(strings.TrimPrefix(name, volume), "/")
	}
	if name == "." {
		return "", nil
	}
	return name, nil
}

// driveVolume returns the drive volume prefix of name, like "C:" or "D:".
// Dropping it makes a member of a Windows-authored archive a relative path
// everywhere, since a UNC name already loses the slashes in front of it.
func driveVolume(name string) string {
	if len(name) >= 2 && name[1] == ':' &&
		(name[0] >= 'a' && name[0] <= 'z' || name[0] >= 'A' && name[0] <= 'Z') {
		return name[:2]
	}
	return ""
}
