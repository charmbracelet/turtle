// Package md5sum implements the GNU md5sum utility.
//
// It writes the MD5 digest of each operand to the standard output, or
// verifies the checksum lines read from them when -c is given. The standard
// input is used when no operand is given, and also when an operand is "-".
package md5sum

import (
	"context"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/digest"
)

// Prog is the md5sum core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the MD5 digest of each operand, or verifies the checksums given
// by -c. Long operations are interrupted when ctx is done, returning the
// context error as it is.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	return digest.MD5.Run(ctx, p.Runtime, args)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
