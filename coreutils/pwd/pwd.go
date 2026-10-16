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
	physical, err := parseFlags(args)
	if err != nil {
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

// parseFlags parses the -L and -P options. The last one to appear wins, and -L
// is assumed when neither is given. Operands are not accepted.
func parseFlags(args []string) (physical bool, err error) {
	options := true
	for _, arg := range args {
		if !options || arg == "-" || !strings.HasPrefix(arg, "-") {
			return false, fmt.Errorf("unexpected operand: %q", arg)
		}
		if arg == "--" {
			options = false
			continue
		}
		for i := 1; i < len(arg); i++ {
			switch arg[i] {
			case 'L':
				physical = false
			case 'P':
				physical = true
			default:
				return false, fmt.Errorf("invalid option: %q", arg)
			}
		}
	}
	return physical, nil
}

// dir returns the directory path to print. With -L it prefers the PWD
// environment variable when it holds a valid logical path; otherwise it falls
// back to the working directory.
func (p *Prog) dir(physical bool) (string, error) {
	if !physical && p.LookupEnv != nil {
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
