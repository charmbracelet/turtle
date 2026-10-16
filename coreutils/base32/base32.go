// Package base32 implements the GNU base32 utility.
//
// It encodes the input with the base32 alphabet of RFC 4648, or decodes it
// with -d, writing the result to the standard output. The standard input is
// used when no operand is given, and also when the operand is "-".
package base32

import (
	"context"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/basex"
)

// Prog is the base32 core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the base32 encoding of the input to the standard output, or its
// decoded bytes when -d is given. The standard input is used when no operands
// are given, and also when an operand is "-".
//
// Reading is interrupted when ctx is done, and the context error is returned.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	return basex.Base32.Run(ctx, p.Runtime, args)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
