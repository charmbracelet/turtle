// Package link implements the POSIX link utility.
//
// The utility takes no options of its own, so every argument is taken as an
// operand, even one starting with a dash, with the common --help and
// --version as the only exception. It calls the link function of the file
// system, which fails when the new entry exists or the source is a
// directory, like link(2).
package link

import (
	"context"
	"errors"
	"fmt"
	"os"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/patherr"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the link core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run creates file2 as a hard link to file1. As the utility takes no
// options of its own, every argument, including one starting with a dash,
// is an operand, with the common --help and --version as the only exception.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "link", usage, getopt.Set{}, args); done {
		return err
	}
	switch len(args) {
	case 0:
		stdopts.Errorf(p.Runtime, "link", "missing file operand")
		stdopts.TryHelp(p.Runtime, "link")
		return &coreutils.ExitError{Code: 2}
	case 1:
		stdopts.Errorf(p.Runtime, "link", "missing destination file operand after '%s'", args[0])
		stdopts.TryHelp(p.Runtime, "link")
		return &coreutils.ExitError{Code: 2}
	case 2:
	default:
		stdopts.Errorf(p.Runtime, "link", "extra operand '%s'", args[2])
		stdopts.TryHelp(p.Runtime, "link")
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		stdopts.Errorf(p.Runtime, "link", "no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.link(args[0], args[1]); err != nil {
		stdopts.Errorf(p.Runtime, "link", "%v", err)
		return &coreutils.ExitError{Code: 1}
	}
	return nil
}

// link creates the target operand as a hard link to the source operand. An
// error is returned ready to be reported.
func (p *Prog) link(source, target string) error {
	if source == "" || target == "" {
		return errors.New("invalid zero-length file name")
	}
	if err := p.FS.Link(p.FsAbs(source), p.FsAbs(target)); err != nil {
		return fmt.Errorf("cannot create link '%s' to '%s': %v", target, source, unwrapLinkError(err))
	}
	return nil
}

// unwrapLinkError unwraps the errors of a link creation, which *os.LinkError
// wraps with both names instead.
func unwrapLinkError(err error) error {
	if linkErr, ok := errors.AsType[*os.LinkError](err); ok {
		return linkErr.Err
	}
	return patherr.Unwrap(err)
}

// usage describes the options of link.
const usage = `Usage: link FILE1 FILE2
Call the link function to create a link named FILE2 to an existing FILE1.

` + stdopts.Tail

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
