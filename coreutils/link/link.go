// Package link implements the POSIX link utility.
//
// The utility takes no options, so every argument is taken as an operand,
// even one starting with a dash. It calls the link function of the file
// system, which fails when the new entry exists or the source is a
// directory, like link(2).
package link

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/patherr"
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
// options, every argument, including one starting with a dash, is an
// operand.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	switch len(args) {
	case 0:
		p.errorf("missing file operand")
		return &coreutils.ExitError{Code: 2}
	case 1:
		p.errorf("missing destination file operand after '%s'", args[0])
		return &coreutils.ExitError{Code: 2}
	case 2:
	default:
		p.errorf("extra operand '%s'", args[2])
		return &coreutils.ExitError{Code: 2}
	}
	if p.FS == nil {
		p.errorf("no file system available")
		return &coreutils.ExitError{Code: 1}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.link(args[0], args[1]); err != nil {
		p.errorf("%v", err)
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
	if err := p.FS.Link(p.resolve(source), p.resolve(target)); err != nil {
		return fmt.Errorf("cannot create link '%s' to '%s': %v", target, source, unwrapLinkError(err))
	}
	return nil
}

// resolve converts an operand into the slash-separated path used with the
// file system, relative to the working directory. The path isn't resolved
// lexically: components like "." and ".." and a trailing slash are kept,
// so only the file system resolves them, following symbolic links on the
// way.
func (p *Prog) resolve(operand string) string {
	operand = filepath.ToSlash(operand)
	if p.WorkingDir == "" || filepath.IsAbs(operand) {
		return operand
	}
	wd := filepath.ToSlash(p.WorkingDir)
	if strings.HasSuffix(wd, "/") {
		return wd + operand
	}
	return wd + "/" + operand
}

// unwrapLinkError unwraps the errors of a link creation, which *os.LinkError
// wraps with both names instead.
func unwrapLinkError(err error) error {
	if linkErr, ok := errors.AsType[*os.LinkError](err); ok {
		return linkErr.Err
	}
	return patherr.Unwrap(err)
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "link: "+format+"\n", args...)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
