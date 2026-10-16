// Package rmdir implements the POSIX rmdir utility.
package rmdir

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
)

// Prog is the rmdir core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run removes the given directories, in the order given. With -p, the
// directories above each operand are removed too, as long as they become
// empty.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	parents, dirs, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	if len(dirs) == 0 {
		p.errorf("missing operand")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}

	ok := true
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		failed, err := p.rmdir(dir, parents)
		if err != nil {
			p.errorf("%s: %v", failed, unwrapPathError(err))
			ok = false
		}
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// rmdir removes a single directory and, with parents, the directories above
// it, stopping at the first error. It returns the path of the directory that
// failed to be removed, if any.
func (p *Prog) rmdir(dir string, parents bool) (string, error) {
	comps := coreutils.SplitPath(dir)
	if len(comps) == 0 {
		comps = []string{"."}
	}
	for i := len(comps); i > 0; i-- {
		name := coreutils.JoinPath(comps[:i])
		display := name
		if i == len(comps) {
			display = dir
		}
		if err := p.remove(filepath.ToSlash(p.ResolvePath(name))); err != nil {
			return display, err
		}
		if !parents {
			break
		}
	}
	return "", nil
}

// remove removes a single directory, which must be empty.
func (p *Prog) remove(name string) error {
	fi, err := fs.Stat(p.FS, name)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return &fs.PathError{Op: "rmdir", Path: name, Err: errNotDir}
	}
	return p.FS.Remove(name)
}

// errNotDir is returned when an operand names something other than a
// directory.
var errNotDir = errors.New("not a directory")

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "rmdir: "+format+"\n", args...)
}

// unwrapPathError unwraps *fs.PathError so diagnostics don't repeat the file
// name.
func unwrapPathError(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}
	return err
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--".
func parseFlags(args []string) (parents bool, dirs []string, err error) {
	parsing := true
	for _, arg := range args {
		switch {
		case !parsing || arg == "-" || !strings.HasPrefix(arg, "-"):
			parsing = false
			dirs = append(dirs, arg)
		case arg == "--":
			parsing = false
		case strings.HasPrefix(arg, "--"):
			return false, nil, fmt.Errorf("unrecognized option '%s'", arg)
		default:
			for j := 1; j < len(arg); j++ {
				if arg[j] != 'p' {
					return false, nil, fmt.Errorf("invalid option -- '%c'", arg[j])
				}
				parents = true
			}
		}
	}
	return parents, dirs, nil
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
