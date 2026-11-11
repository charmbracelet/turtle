// Package true implements the POSIX true utility.
package true

import (
	"context"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the true core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// usage describes the options of true.
const usage = `Usage: true [ignored command line arguments]
  or:  true OPTION
Exit with a status code indicating success.

` + stdopts.Tail

// Run returns a zero exit status, after handling the common options.
func (p *Prog) Run(_ context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "true", usage, getopt.Set{}, args); done {
		return err
	}
	return nil
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
