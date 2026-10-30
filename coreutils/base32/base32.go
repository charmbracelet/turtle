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
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
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
	if done, err := stdopts.Check(p.Runtime, "base32", usage, optSet, args); done {
		return err
	}
	return basex.Base32.Run(ctx, p.Runtime, args)
}

// longOptions maps the long options of base32 to the short ones they stand
// for, matching the ones basex parses.
var longOptions = map[string]rune{
	"decode":         'd',
	"ignore-garbage": 'i',
	"wrap":           'w',
}

// optSet describes the options of base32.
var optSet = getopt.Set{Valued: "w", Long: longOptions}

// usage describes the options of base32.
const usage = `Usage: base32 [OPTION]... [FILE]
Base32 encode or decode FILE, or standard input, to standard output.

With no FILE, or when FILE is -, read standard input.

  -d, --decode
         decode data
  -i, --ignore-garbage
         when decoding, ignore non-alphabet characters
  -w, --wrap=COLS
         wrap encoded lines after COLS characters (default 76). Use 0 to disable
         line wrapping
      --help
         display this help and exit
      --version
         output version information and exit
`

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
