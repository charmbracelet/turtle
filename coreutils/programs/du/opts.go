package du

import (
	"fmt"
	"strconv"

	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/sizefmt"
	"charm.land/turtle/coreutils/internal/stdopts"
)

// optMaxDepth is a rune which no short option can reach, so --max-depth has
// a character of its own to hold its argument.
const optMaxDepth rune = 0x100 + iota

// longOptions maps the long options of du to the characters they stand for.
var longOptions = map[string]rune{
	"all":              'a',
	"bytes":            'b',
	"dereference":      'L',
	"dereference-args": 'H',
	"human-readable":   'h',
	"max-depth":        optMaxDepth,
	"one-file-system":  'x',
	"separate-dirs":    'S',
	"summarize":        's',
	"total":            'c',
}

// optSet describes the options of du.
var optSet = getopt.Set{Valued: "d" + string(optMaxDepth), Long: longOptions}

// usage describes the options of du.
const usage = `Usage: du [OPTION]... [FILE]...
Summarize device usage of the set of FILEs, recursively for directories.

  -a, --all
         write counts for all files, not just directories
  -b, --bytes
         print apparent sizes in bytes rather than device usage
  -c, --total
         produce a grand total
  -d, --max-depth=N
         print the total for a directory (or file, with --all) only if it is
         N or fewer levels below the command line argument;
         --max-depth=0 is the same as --summarize
  -H
         count the size of the file or file hierarchy referenced by a
         command line symbolic link
  -h, --human-readable
         print sizes in human readable format (e.g., 1K 234M 2G)
  -k
         write file sizes in units of 1024 bytes rather than the default
         512-byte units
  -L, --dereference
         count the size of the file or file hierarchy referenced by all
         symbolic links
  -S, --separate-dirs
         for directories do not include size of subdirectories
  -s, --summarize
         display only a total for each argument
  -x, --one-file-system
         skip directories on different file systems
` + stdopts.Tail

// options holds what a run of du selects.
type options struct {
	all         bool // -a: write every file in the hierarchy
	summarize   bool // -s: write the operands alone
	oneFS       bool // -x: stay on one file system
	separate    bool // -S: directories exclude their subdirectories
	total       bool // -c: write a grand total
	apparent    bool // -b: count apparent sizes
	follow      byte // 'P' or zero by default, 'H' for operands, 'L' always
	maxDepth    int  // -d, negative when unset
	maxDepthSet bool
	spec        sizefmt.Spec // how the sizes are written

	// unit is the bytes each written size counts. Sizes are counted in
	// whole units per file, the way the GNU utility sums them, so it
	// matches the fixed unit of spec.
	unit int64
}

// parseFlags parses the options and operands. Option parsing stops at the
// first operand or at "--". The last of -H and -L, and the last of the size
// options, determine the behavior.
func parseFlags(args []string) (options, []string, error) {
	opts := options{maxDepth: -1, spec: sizefmt.Fixed(512), unit: 512}
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
			opts.all = true
		case 'b':
			opts.apparent = true
			opts.spec = sizefmt.ByteSpec
			opts.unit = 1
		case 'c':
			opts.total = true
		case 'd', optMaxDepth:
			depth, err := strconv.ParseInt(value, 0, 64)
			if err != nil || depth < 0 {
				return fmt.Errorf("invalid maximum depth '%s'", value)
			}
			opts.maxDepth, opts.maxDepthSet = int(depth), true
		case 'h':
			opts.spec = sizefmt.HumanSpec(false)
			opts.unit = 1
		case 'H':
			opts.follow = 'H'
		case 'k':
			opts.spec = sizefmt.Fixed(1024)
			opts.unit = 1024
		case 'L':
			opts.follow = 'L'
		case 'S':
			opts.separate = true
		case 's':
			opts.summarize = true
		case 'x':
			opts.oneFS = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	return opts, operands, err
}
