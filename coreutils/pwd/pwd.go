// Package pwd implements the POSIX pwd utility.
package pwd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the pwd core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the absolute pathname of the current working directory to
// standard output.
func (p *Prog) Run(_ context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "pwd", usage, optSet, args); done {
		return err
	}
	physical, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "pwd")
		return &coreutils.ExitError{Code: 2, Err: err}
	}
	dir, err := p.dir(physical)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(p.Stdout, filepath.ToSlash(dir)+"\n"); err != nil {
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return nil
}

// longOptions maps the long options of pwd to the short ones they stand for.
var longOptions = map[string]rune{
	"logical":  'L',
	"physical": 'P',
}

// optSet describes the options of pwd.
var optSet = getopt.Set{Long: longOptions}

// usage describes the options of pwd.
const usage = `Usage: pwd [OPTION]...
Print the full filename of the current working directory.

  -L, --logical   use PWD from environment, even if it contains symlinks
  -P, --physical  avoid all symlinks
      --help     display this help and exit
      --version  output version information and exit
`

// errorf writes a diagnostic message to the standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "pwd: "+format+"\n", args...)
}

// parseFlags parses the -L and -P options. The last one to appear wins, and -L
// is assumed when neither is given. Operands are not accepted.
func parseFlags(args []string) (physical bool, err error) {
	operands, err := optSet.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case 'L':
			physical = false
		case 'P':
			physical = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if len(operands) > 0 {
		return false, fmt.Errorf("unexpected operand: %q", operands[0])
	}
	return physical, nil
}

// dir returns the directory path to print. With -L it prefers the PWD
// environment variable when it holds a valid logical path; otherwise it falls
// back to the working directory.
func (p *Prog) dir(physical bool) (string, error) {
	if !physical {
		if pwd := p.Getenv("PWD"); isValidLogicalPath(pwd) {
			return pwd, nil
		}
	}
	dir := filepath.Clean(p.WorkingDir)
	if !filepath.IsAbs(dir) {
		return "", &coreutils.ExitError{Code: 1, Err: errors.New("cannot determine current directory")}
	}
	return dir, nil
}

// isValidLogicalPath reports whether path is an absolute pathname that does not
// contain any dot or dot-dot components.
func isValidLogicalPath(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	for _, part := range strings.FieldsFunc(path, isSeparator) {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

func isSeparator(r rune) bool {
	return r == '/' || r == '\\'
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
