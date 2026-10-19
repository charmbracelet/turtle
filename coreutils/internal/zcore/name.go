package zcore

import (
	"fmt"
	"path"
	"strings"
)

// nameMax is how long a file name component may be, which POSIX asks the
// utilities to check before adding a suffix to a name.
const nameMax = 255

// knownSuffixes are the suffixes of compressed files, besides the one of the
// algorithm in use. The ones of the tar archives are replaced with ".tar".
var knownSuffixes = []string{".gz", ".z", ".Z", ".tz", ".tgz"}

// stripSuffix returns the name of the file a compressed file at name restores
// to, reporting whether name has a suffix of a known format.
func stripSuffix(name string, opts Options) (string, bool) {
	if suffix := opts.suffix(); strings.HasSuffix(name, suffix) && name != suffix {
		return strings.TrimSuffix(name, suffix), true
	}
	for _, suffix := range knownSuffixes {
		if name == suffix || !strings.HasSuffix(name, suffix) {
			continue
		}
		stripped := strings.TrimSuffix(name, suffix)
		if suffix == ".tz" || suffix == ".tgz" {
			stripped += ".tar"
		}
		return stripped, true
	}
	return "", false
}

// validStoredName reports whether name is an ordinary basename, as only such a
// name from a gzip header may be used for the restored file.
func validStoredName(name string) bool {
	return name != "" && name != "." && name != ".." &&
		!strings.ContainsAny(name, "/\\\x00\n\r") && path.Base(name) == name
}

// checkName reports whether a file can be created at name, as far as the
// length of its last component goes.
func checkName(name string) error {
	if base := path.Base(name); base == "" || len(base) > nameMax {
		return fmt.Errorf("cannot create file: invalid name '%s'", name)
	}
	return nil
}

// ratio formats the percentage of space saved by compression, as the -v option
// reports it.
func ratio(compressed, original int64) string {
	if original <= 0 {
		return "0.00%"
	}
	return fmt.Sprintf("%.2f%%", 100*float64(original-compressed)/float64(original))
}
