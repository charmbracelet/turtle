// Package unlink implements the POSIX unlink utility.
//
// The utility takes no options of its own, so every argument is taken as an
// operand, even one starting with a dash, with the common --help and
// --version as the only exception. The file is removed with the file
// system's Remove operation, which removes a final symbolic link itself,
// like unlink(2), and directories are refused.
package unlink

import (
	"context"
	"errors"
	"fmt"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the unlink core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run unlinks the file named by the single operand. As the utility takes no
// options of its own, every argument, including one starting with a dash,
// is an operand, with the common --help and --version as the only exception.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "unlink", usage, getopt.Set{}, args); done {
		return err
	}
	switch len(args) {
	case 0:
		stdopts.Errorf(p.Runtime, "unlink", "missing operand")
		stdopts.TryHelp(p.Runtime, "unlink")
		return &coreutils.ExitError{Code: 2}
	case 1:
	default:
		stdopts.Errorf(p.Runtime, "unlink", "extra operand '%s'", args[1])
		stdopts.TryHelp(p.Runtime, "unlink")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "unlink", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.unlink(args[0]); err != nil {
		stdopts.Errorf(p.Runtime, "unlink", "%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// unlink removes the operand, which must not name a directory. A final
// symbolic link is removed itself, not the file it points to. Errors are
// returned ready to be reported.
func (p *Prog) unlink(operand string) error {
	if operand == "" {
		return errors.New("invalid zero-length file name")
	}
	name := p.FsAbs(operand)
	fi, err := p.FS.Lstat(name)
	if err != nil {
		return fmt.Errorf("%s: %v", operand, patherr.Unwrap(err))
	}
	if fi.IsDir() {
		return fmt.Errorf("%s: %v", operand, errIsDir)
	}
	if err := p.FS.Remove(name); err != nil {
		return fmt.Errorf("%s: %v", operand, patherr.Unwrap(err))
	}
	return nil
}

// errIsDir is returned when the operand names a directory.
var errIsDir = errors.New("is a directory")

// usage describes the options of unlink.
const usage = `Usage: unlink FILE
Call the unlink function to remove the specified FILE.

` + stdopts.Tail

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
