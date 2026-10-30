// Package gzip implements the GNU gzip utility.
//
// It compresses files in the format of RFC 1952, naming the results with the
// ".gz" suffix, and restores them with -d. The gunzip, zcat and gzcat packages
// are this same utility with different defaults, each built by its own New
// function.
package gzip

import (
	"context"
	"fmt"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
	"charm.land/snail/coreutils/internal/zcore"
)

// Prog is the gzip core utility.
type Prog struct {
	*coreutils.Runtime
	base zcore.Options
}

// New returns a new gzip Prog, which compresses files by default.
func New(rt *coreutils.Runtime) coreutils.Program {
	return newProg(rt, "gzip", nil)
}

// Gunzip returns a new Prog which restores files, as gzip does when invoked as
// gunzip.
func Gunzip(rt *coreutils.Runtime) coreutils.Program {
	return newProg(rt, "gunzip", func(opts *zcore.Options) {
		opts.Decompress = true
	})
}

// Zcat returns a new Prog which writes restored files to standard output, as
// gzip does when invoked as zcat. This front end also restores .Z input only
// when the LZW decoder is implemented.
func Zcat(rt *coreutils.Runtime) coreutils.Program {
	return newProg(rt, "zcat", func(opts *zcore.Options) {
		opts.Decompress = true
		opts.Stdout = true
	})
}

// Gzcat returns a new Prog, an alias for zcat on some systems.
func Gzcat(rt *coreutils.Runtime) coreutils.Program {
	return newProg(rt, "gzcat", func(opts *zcore.Options) {
		opts.Decompress = true
		opts.Stdout = true
	})
}

// newProg builds a Prog with the given name and defaults applied on top of the
// options shared by all of them.
func newProg(rt *coreutils.Runtime, name string, defaults func(*zcore.Options)) *Prog {
	opts := zcore.Options{Prog: name, Level: -1, SkipSuffixed: true}
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
// and the context error when ctx is done.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, p.base.Prog, Usage(p.base.Prog), optSet, args); done {
		return err
	}
	set, err := parseFlags(append(envOptions(p.Getenv("GZIP")), args...), p.base)
	if err != nil {
		fmt.Fprintf(p.Stderr, "%s: %v\n", p.base.Prog, err)
		stdopts.TryHelp(p.Runtime, p.base.Prog)
		return &coreutils.ExitError{Code: 1, Err: err}
	}
	switch {
	case set.help:
		fmt.Fprint(p.Stdout, Usage(p.base.Prog))
		return nil
	case set.version:
		fmt.Fprintf(p.Stdout, "%s (charm.land/snail/coreutils) %s\nhttps://charm.land/snail\n",
			p.base.Prog, coreutils.Version)
		return nil
	}
	return zcore.Run(ctx, p.Runtime, set.Options, set.operands)
}

// settings are the parsed options of the utility, plus the requests which make
// it stop before touching any file.
type settings struct {
	zcore.Options
	operands []string
	help     bool
	version  bool
}

// longOptions maps the long options of gzip to the short ones they stand for.
var longOptions = map[string]rune{
	"best":       '9',
	"decompress": 'd',
	"fast":       '1',
	"force":      'f',
	"help":       'h',
	"keep":       'k',
	"name":       'N',
	"no-name":    'n',
	"quiet":      'q',
	"recursive":  'r',
	"silent":     'q',
	"stdout":     'c',
	"suffix":     'S',
	"test":       't',
	"to-stdout":  'c',
	"uncompress": 'd',
	"verbose":    'v',
	"version":    'V',
}

// optSet describes the options of the utility.
var optSet = getopt.Set{Valued: "S", Long: longOptions}

// parseFlags parses args on top of the base options of the program. Option
// parsing stops at the first operand or at "--".
func parseFlags(args []string, base zcore.Options) (settings, error) {
	set := settings{Options: base}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'c', 'd', 'f', 'k', 'N', 'n', 'q', 'r', 't', 'v':
			set.apply(opt)
		case 'h':
			set.help = true
		case 'V':
			set.version = true
		case 'S':
			set.Suffix = value
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			set.Level = int(opt - '0')
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return settings{}, err
	}
	set.operands = operands
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
	case 'k':
		s.Keep = true
	case 'N':
		s.StoreName = true
	case 'n':
		s.NoName = true
		s.StoreName = false
	case 'q':
		s.Quiet = true
	case 'r':
		s.Recursive = true
	case 't':
		s.Test = true
	case 'v':
		s.Verbose = true
	}
}

// envOptions splits the GZIP environment variable into options which come
// before the ones given on the command line.
func envOptions(value string) []string {
	return strings.Fields(value)
}

// Usage describes the options of the utility, as the given program.
func Usage(prog string) string {
	return `Usage: ` + prog + ` [-cdfhknNqrStv] [-0..-9] [-S suffix] [file ...]
Compress or expand files (Lempel-Ziv coding LZ77).

  -1..-9
         compression level: -1 (fast) to -9 (best), -0 for none
  -c, --stdout
         write on standard output, keeping original files unchanged
  -d, --decompress
         decompress instead of compress
  -f, --force
         force overwrite of output files and (with -c) pass non-compressed data
         through unchanged
  -h, --help
         display this help and exit
  -k, --keep
         keep (do not delete or truncate) the input files
  -N, --name
         use the name stored in a compressed file for the output
  -n, --no-name
         do not store the original name and modification time
  -q, --quiet, --silent
         suppress all warnings
  -r, --recursive
         walk down directories recursively
  -S, --suffix=suffix
         override the suffix of compressed files
  -t, --test
         test the integrity of compressed files
  -v, --verbose
         be verbose
  -V, --version
         output version information and exit
`
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
	_ coreutils.NewFunc = Gunzip
	_ coreutils.NewFunc = Zcat
	_ coreutils.NewFunc = Gzcat
)
