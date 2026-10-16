// Package false implements the POSIX false utility.
package false

import (
	"context"

	"charm.land/snail/coreutils"
)

// Prog is the false core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run always returns a non-zero exit status.
func (p *Prog) Run(_ context.Context, _ ...string) error {
	return &coreutils.ExitError{Code: 1}
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
