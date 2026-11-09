// Package backup implements the backup scheme which the GNU utilities that
// overwrite or remove files (cp, install, ln and mv) share: before a file is
// replaced, it can be renamed aside, either under a simple name holding a
// suffix or under a numbered one.
package backup

import (
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
)

// Method is the kind of backups made for the files which would otherwise be
// overwritten or removed.
type Method int

const (
	// None never makes backups.
	None Method = iota

	// Numbered always makes numbered backups, as in file.~1~.
	Numbered

	// Existing makes numbered backups of files which already have them, and
	// simple backups of the others.
	Existing

	// Simple always makes simple backups, as in file~.
	Simple
)

// names maps the GNU names of the methods to them. Every name is a full
// match or the unique prefix of one.
var names = map[string]Method{
	"none":     None,
	"off":      None,
	"numbered": Numbered,
	"t":        Numbered,
	"existing": Existing,
	"nil":      Existing,
	"simple":   Simple,
	"never":    Simple,
}

// ParseMethod parses a backup method, given as one of its GNU names or the
// unique abbreviation of one.
func ParseMethod(s string) (Method, error) {
	if m, ok := names[s]; ok {
		return m, nil
	}
	for name, m := range names {
		if strings.HasPrefix(name, s) && !dupPrefix(s, name) {
			return m, nil
		}
	}
	return None, fmt.Errorf("invalid backup method '%s'", s)
}

// dupPrefix reports whether another method name than name starts with s.
func dupPrefix(s, name string) bool {
	for other := range names {
		if other != name && strings.HasPrefix(other, s) {
			return true
		}
	}
	return false
}

// Options describes the backups a utility makes.
type Options struct {
	// Method is the kind of backups made. None makes none.
	Method Method

	// Suffix is appended to the backup name of a simple backup.
	Suffix string
}

// DefaultSuffix is the suffix of simple backups when no other was given.
const DefaultSuffix = "~"

// Name returns the backup name for the file named dest, as its directory
// entries, read from fsys, require: simple backups take the suffix, and
// numbered backups take the version after the highest numbered one already
// there. The name is returned for a backup of any kind of file, existing or
// not, so callers only use it when they would destroy dest. An unreadable
// directory of dest is an error, since the versions it holds can't be
// counted and the chosen name could collide with a backup already there.
func Name(fsys fs.FS, dest string, opts Options) (string, error) {
	switch opts.Method {
	case None:
		return dest, nil
	case Simple:
		return dest + opts.Suffix, nil
	}
	var highest int
	prefix, suffixOf := path.Base(dest)+".~", "~"
	entries, err := fs.ReadDir(fsys, path.Dir(dest))
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		version, found := strings.CutPrefix(entry.Name(), prefix)
		version, ok := strings.CutSuffix(version, suffixOf)
		if !found || !ok {
			continue
		}
		if n, err := strconv.Atoi(version); err == nil && n > highest {
			highest = n
		}
	}
	if opts.Method == Existing && highest == 0 {
		return dest + opts.Suffix, nil
	}
	return dest + ".~" + strconv.Itoa(highest+1) + "~", nil
}
