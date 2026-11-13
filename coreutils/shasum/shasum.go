// Package shasum implements the shasum utility of the Perl distribution.
//
// It writes the SHA digest of each operand to the standard output, or
// verifies the checksum lines read from them when -c is given, hashing with
// the algorithm the -a option selects: SHA-1 unless it says otherwise. The
// standard input is used when no operand is given, and also when an operand
// is "-".
package shasum

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/digest"
)

// Prog is the shasum core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// family is the utility: the algorithms of the sha*sum family, with the tags
// the Perl script writes on BSD-style lines.
var family = digest.Family{
	Name:    "shasum",
	Default: "1",
	Algorithms: map[string]digest.Algorithm{
		"1":      {Tag: "SHA1", New: sha1.New},
		"224":    {Tag: "SHA224", New: sha256.New224},
		"256":    {Tag: "SHA256", New: sha256.New},
		"384":    {Tag: "SHA384", New: sha512.New384},
		"512":    {Tag: "SHA512", New: sha512.New},
		"512224": {Tag: "SHA512/224", New: sha512.New512_224},
		"512256": {Tag: "SHA512/256", New: sha512.New512_256},
	},
}

// Run writes the SHA digest of each operand, or verifies the checksums given
// by -c, hashing with the algorithm -a selects. Long operations are
// interrupted when ctx is done, returning the context error as it is.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	return family.Run(ctx, p.Runtime, args)
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
