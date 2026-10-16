// Package basename implements the POSIX basename utility.
//
// A null string operand produces an empty output, which POSIX leaves
// unspecified; this matches the GNU implementation.
package basename

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
)

// Prog is the basename core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the non-directory portion of the string operand to standard
// output, removing the suffix operand if it's a suffix of the result.
func (p *Prog) Run(_ context.Context, args ...string) error {
	path, suffix, err := parseArgs(args)
	if err != nil {
		return &coreutils.ExitError{Code: 2, Err: err}
	}
	// Windows treats backslashes as path separators, so normalize them like
	// the other implementations do before parsing.
	result := basename(filepath.ToSlash(path), suffix)
	if _, err := io.WriteString(p.Stdout, result+"\n"); err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// parseArgs splits the argument list into the string and suffix operands.
// basename has no options, but a leading "--" is still accepted as the end of
// options marker and skipped.
func parseArgs(args []string) (string, string, error) {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	switch len(args) {
	case 0:
		return "", "", errors.New("missing operand")
	case 1:
		return args[0], "", nil
	case 2:
		return args[0], args[1], nil
	default:
		return "", "", fmt.Errorf("extra operand: %q", args[2])
	}
}

// basename returns the last component of path, with suffix removed when it's
// not identical to and is a suffix of the result. Only forward slashes are
// treated as separators, so callers must normalize Windows-style paths first.
func basename(path, suffix string) string {
	if path == "" {
		return ""
	}
	if strings.Trim(path, "/") == "" {
		return "/"
	}
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	if suffix != "" && suffix != path && strings.HasSuffix(path, suffix) {
		path = path[:len(path)-len(suffix)]
	}
	return path
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
