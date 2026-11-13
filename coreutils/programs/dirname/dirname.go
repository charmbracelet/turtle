// Package dirname implements the POSIX dirname utility, with the GNU
// extensions of accepting several name operands and of -z.
//
// POSIX leaves the removal of redundant slashes and "." components
// unspecified; this implementation removes redundant slashes, like the GNU one
// does, but never removes ".." components.
package dirname

import (
	"context"
	"io"
	"path/filepath"
	"strings"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/stdopts"
)

// Prog is the dirname core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the directory portion of each name operand to standard output,
// one result per line, or separated by NUL bytes with -z.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "dirname", usage, optSet, args); done {
		return err
	}
	zero, names, err := parseFlags(args)
	if err != nil {
		return stdopts.UsageError(p.Runtime, "dirname", err)
	}
	if len(names) == 0 {
		stdopts.Errorf(p.Runtime, "dirname", "missing operand")
		stdopts.TryHelp(p.Runtime, "dirname")
		return &coreutils.ExitError{Code: 2}
	}

	sep := "\n"
	if zero {
		sep = "\x00"
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Windows treats backslashes as path separators, so normalize them
		// like the other implementations do before parsing.
		result := dirname(filepath.ToSlash(name)) + sep
		if _, err := io.WriteString(p.Stdout, result); err != nil {
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	}
	return nil
}

// longOptions maps the long options of dirname to the short ones they stand
// for.
var longOptions = map[string]rune{
	"zero": 'z',
}

// optSet describes the options of dirname.
var optSet = getopt.Set{Long: longOptions}

// usage describes the options of dirname.
const usage = `Usage: dirname [OPTION] NAME...
Output each NAME with its last non-slash component and trailing slashes
removed; if NAME contains no /, output '.' (meaning the current directory).

  -z, --zero
         end each output line with NUL, not newline
` + stdopts.Tail

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (zero bool, names []string, err error) {
	names, err = optSet.Parse(args, func(opt rune, _ string) error {
		if opt != 'z' {
			return getopt.InvalidOption(opt)
		}
		zero = true
		return nil
	})
	return zero, names, err
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
