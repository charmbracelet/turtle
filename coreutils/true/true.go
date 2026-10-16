// Package true implements the POSIX true utility.
package true

import (
	"context"

	"charm.land/snail/coreutils"
)

// Prog is the true core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run always returns a zero exit status.
func (p *Prog) Run(_ context.Context, _ ...string) error {
	return nil
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
