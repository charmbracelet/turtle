// Package dirname implements the POSIX dirname utility.
//
// POSIX leaves the removal of redundant slashes and "." components
// unspecified; this implementation removes redundant slashes, like the GNU one
// does, but never removes ".." components.
package dirname

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
)

// Prog is the dirname core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the directory portion of the string operand to standard output.
func (p *Prog) Run(_ context.Context, args ...string) error {
	path, err := parseArgs(args)
	if err != nil {
		return &coreutils.ExitError{Code: 2, Err: err}
	}
	// Windows treats backslashes as path separators, so normalize them like
	// the other implementations do before parsing.
	result := dirname(filepath.ToSlash(path))
	if _, err := io.WriteString(p.Stdout, result+"\n"); err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// parseArgs reads the single string operand. dirname has no options, but a
// leading "--" is still accepted as the end of options marker and skipped.
func parseArgs(args []string) (string, error) {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	switch len(args) {
	case 0:
		return "", errors.New("missing operand")
	case 1:
		return args[0], nil
	default:
		return "", fmt.Errorf("extra operand: %q", args[1])
	}
}

// dirname returns the directory portion of path, i.e. everything before its
// last component, with trailing slashes not counted as part of the pathname.
// Only forward slashes are treated as separators, so callers must normalize
// Windows-style paths first.
func dirname(path string) string {
	prefix := 0
	if strings.HasPrefix(path, "/") {
		prefix = 1
	}

	// Strip the redundant slashes right before the last component, but
	// never below the leading slash, which is part of the result.
	start := lastComponent(path)
	for prefix < start && path[start-1] == '/' {
		start--
	}
	if start == 0 {
		return "."
	}
	return path[:start]
}

// lastComponent returns the index at which the last component of path starts,
// ignoring trailing slashes.
func lastComponent(path string) int {
	start := 0
	for start < len(path) && path[start] == '/' {
		start++
	}
	sawSlash := false
	for i := start; i < len(path); i++ {
		if path[i] == '/' {
			sawSlash = true
		} else if sawSlash {
			start, sawSlash = i, false
		}
	}
	return start
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
