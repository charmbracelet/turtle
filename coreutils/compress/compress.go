// Package compress implements the POSIX compress utility.
//
// It shares its behavior with the gzip utility, exposing the options and the
// defaults of the specification. The LZW algorithm, which the specification
// uses by default, is not implemented yet: compressing needs -g or -m, and
// files of that format are rejected when restored.
package compress

import (
	"context"
	"fmt"
	"strconv"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/zcore"
)

// Prog is the compress core utility.
type Prog struct {
	*coreutils.Runtime
	base zcore.Options
}

// New returns a new Prog, which compresses files by default.
func New(rt *coreutils.Runtime) coreutils.Program {
	return newProg(rt, "compress", nil)
}

// Uncompress returns a new Prog which restores gzip files. Restoring the POSIX
// default LZW format is not implemented yet.
func Uncompress(rt *coreutils.Runtime) coreutils.Program {
	return newProg(rt, "uncompress", func(opts *zcore.Options) {
		opts.Decompress = true
	})
}

// newProg builds a Prog with the given name and defaults applied on top of the
// options shared by both of them.
func newProg(rt *coreutils.Runtime, name string, defaults func(*zcore.Options)) *Prog {
	opts := zcore.Options{
		Prog:         name,
		Algorithm:    zcore.LZW,
		RefuseGrowth: true,
		Level:        -1,
	}
	if defaults != nil {
		defaults(&opts)
	}
	return &Prog{Runtime: rt, base: opts}
}

// Run compresses or restores the given files, following the options given in
// args. The standard input is read when no operand is given, and also when the
// operand is "-".
//
// It returns a [*coreutils.ExitError] when an operand failed or was skipped,
// and the context error when ctx is done. Files which would grow are left
// alone, as the specification asks, unless -f is given.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, p.base.Prog, Usage(p.base.Prog), optSet, args); done {
		return err
	}
	set, err := parseFlags(args, p.base)
	if err != nil {
		fmt.Fprintf(p.Stderr, "%s: %v\n", p.base.Prog, err)
		stdopts.TryHelp(p.Runtime, p.base.Prog)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	return zcore.Run(ctx, p.Runtime, set.Options, set.operands)
}

// settings are the parsed options of the utility, plus the operands to process.
type settings struct {
	zcore.Options
	operands []string
	bits     int
	bitsSet  bool
}

// optSet describes the options of the utility.
var optSet = getopt.Set{Valued: "bm"}

// parseFlags parses args on top of the base options of the program. The -b and
// -m options take an option-argument, and the last -g or -m given wins. Option
// parsing stops at the first operand or at "--".
func parseFlags(args []string, base zcore.Options) (settings, error) {
	set := settings{Options: base}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'c', 'd', 'f', 'g', 'v':
			set.apply(opt)
		case 'b', 'm':
			return set.assign(opt, value)
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return settings{}, err
	}
	set.operands = operands
	if set.bitsSet {
		if err := set.checkBits(); err != nil {
			return settings{}, err
		}
	}
	return set, nil
}

// apply sets the option given as a short flag.
func (s *settings) apply(opt rune) {
	switch opt {
	case 'c':
		s.Stdout = true
	case 'd':
		s.Decompress = true
	case 'f':
		s.Force = true
	case 'g':
		s.Algorithm = zcore.Gzip
	case 'v':
		s.Verbose = true
	}
}

// assign sets the option-argument of -b or -m.
func (s *settings) assign(opt rune, value string) error {
	if opt == 'm' {
		switch value {
		case "lzw":
			s.Algorithm = zcore.LZW
		case "deflate", "gzip":
			s.Algorithm = zcore.Gzip
		default:
			return fmt.Errorf("unknown compression algorithm: %s", value)
		}
		return nil
	}
	bits, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("illegal number of bits: %s", value)
	}
	s.bits, s.bitsSet = bits, true
	return nil
}

// checkBits makes the -b value fit the algorithm in use, which asks for a code
// size between 9 and 16 bits, or for a compression level between 1 and 9.
func (s *settings) checkBits() error {
	if s.Algorithm == zcore.LZW {
		if s.bits < 9 || s.bits > 16 {
			return fmt.Errorf("illegal number of bits: %d", s.bits)
		}
		return nil
	}
	if s.bits < 1 || s.bits > 9 {
		return fmt.Errorf("invalid compression level: %d", s.bits)
	}
	s.Level = s.bits
	return nil
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
	_ coreutils.NewFunc = Uncompress
)

// Usage describes the options of the utility, as the given program.
func Usage(prog string) string {
	synopsis := "[-cfv] [-b bits] [-g | -m algorithm] [file ...]"
	if prog == "uncompress" {
		synopsis = "[-cfv] [file ...]"
	}
	return `Usage: ` + prog + ` ` + synopsis + `
Compress or expand files.

  -b bits
         set the code size, 9 to 16 bits for LZW, 1 to 9 for gzip
  -c
         write the result on the standard output, keeping the input files
         unchanged
  -d
         restore the files, like uncompress
  -f
         force compression, even when the files would grow
  -g
         compress with the gzip format instead of LZW
  -m method
         compression method: lzw (default) or gzip
  -v
         write the names and the sizes of the files processed
      --help
         display this help and exit
      --version
         output version information and exit
`
}
