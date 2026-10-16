// Package chmod implements the POSIX chmod utility.
package chmod

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
)

// Prog is the chmod core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run changes the file mode bits of the given files, as specified by the mode
// operand, which is either an octal number or a symbolic mode expression. With
// -R, directories have their whole hierarchy changed.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	recursive, operands, err := parseFlags(args)
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 2}
	}
	switch {
	case len(operands) == 0:
		p.errorf("missing operand")
		return &coreutils.ExitError{Code: 2}
	case len(operands) == 1:
		p.errorf("missing file operand")
		return &coreutils.ExitError{Code: 2}
	case p.FS == nil:
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	spec, err := ParseMode(operands[0])
	if err != nil {
		p.errorf("%v", err)
		return &coreutils.ExitError{Code: 1}
	}

	ok := true
	for _, file := range operands[1:] {
		if err := ctx.Err(); err != nil {
			return err
		}
		failed, err := p.chmod(ctx, file, spec, recursive)
		if err != nil {
			return err
		}
		ok = ok && !failed
	}
	if !ok {
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// chmod changes the mode of a single operand, reporting a diagnostic for every
// file it fails to change. It reports whether any file failed, and only
// returns an error when the run must be aborted.
func (p *Prog) chmod(ctx context.Context, name string, spec ModeSpec, recursive bool) (bool, error) {
	resolved := filepath.ToSlash(p.ResolvePath(name))
	umask := p.Umask.Perm()
	failed := false

	report := func(walked string, err error) {
		p.errorf("%s: %v", display(name, resolved, walked), unwrapPathError(err))
		failed = true
	}

	if !recursive {
		fi, err := fs.Stat(p.FS, resolved)
		if err == nil {
			err = p.apply(resolved, spec, umask, fi)
		}
		if err != nil {
			report(resolved, err)
		}
		return failed, nil
	}

	err := fs.WalkDir(p.FS, resolved, func(walked string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			report(walked, err)
			if d == nil || d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		fi, err := d.Info()
		if err == nil {
			err = p.apply(walked, spec, umask, fi)
		}
		if err != nil {
			report(walked, err)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return failed, nil
}

// apply changes the mode of a single file.
func (p *Prog) apply(name string, spec ModeSpec, umask fs.FileMode, fi fs.FileInfo) error {
	return p.FS.Chmod(name, spec(fi.Mode(), fi.IsDir(), umask))
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "chmod: "+format+"\n", args...)
}

// display returns the path to show in diagnostics: the operand as given, with
// any sub-path walked below it appended.
func display(operand, resolved, walked string) string {
	if rel, ok := strings.CutPrefix(walked, resolved+"/"); ok {
		return path.Join(operand, rel)
	}
	return operand
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
func parseFlags(args []string) (recursive bool, operands []string, err error) {
	parsing := true
	for _, arg := range args {
		switch {
		case !parsing || arg == "-" || !strings.HasPrefix(arg, "-"):
			parsing = false
			operands = append(operands, arg)
		case arg == "--":
			parsing = false
		case strings.HasPrefix(arg, "--"):
			return false, nil, fmt.Errorf("unrecognized option '%s'", arg)
		default:
			for j := 1; j < len(arg); j++ {
				if arg[j] != 'R' {
					return false, nil, fmt.Errorf("invalid option -- '%c'", arg[j])
				}
				recursive = true
			}
		}
	}
	return recursive, operands, nil
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
