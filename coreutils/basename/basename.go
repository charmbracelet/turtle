// Package basename implements the POSIX and GNU basename utilities.
//
// A null string operand produces an empty output, which POSIX leaves
// unspecified; this matches the GNU implementation.
package basename

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
	"charm.land/snail/coreutils/internal/stdopts"
)

// Prog is the basename core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// Run writes the non-directory portion of each name operand to standard
// output, removing the suffix operand from the traditional form, or the
// option-argument of -s, from each result. With -a every operand is a name,
// and with -z each result ends in a NUL byte instead of a newline.
func (p *Prog) Run(ctx context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "basename", usage, optSet, args); done {
		return err
	}
	opts, names, suffix, err := parseArgs(args)
	if err != nil {
		p.errorf("%v", err)
		stdopts.TryHelp(p.Runtime, "basename")
		return &coreutils.ExitError{Code: 2, Err: err}
	}
	sep := "\n"
	if opts.zero {
		sep = "\x00"
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Windows treats backslashes as path separators, so normalize them
		// like the other implementations do before parsing.
		result := basename(filepath.ToSlash(name), suffix)
		if _, err := io.WriteString(p.Stdout, result+sep); err != nil {
			return &coreutils.ExitError{Code: 1, Err: err}
		}
	}
	return nil
}

// parseArgs parses the options and operands. Option parsing stops at the
// first operand or at "--". Without -a or -s the traditional form of a name
// and an optional suffix operand is kept, and with them every operand is a
// name, with the suffix coming from -s.
func parseArgs(args []string) (options, []string, string, error) {
	var opts options
	operands, err := optSet.Parse(args, func(opt rune, value string) error {
		switch opt {
		case 'a':
			opts.multiple = true
		case 's':
			opts.multiple = true
			opts.suffix = value
		case 'z':
			opts.zero = true
		default:
			return getopt.InvalidOption(opt)
		}
		return nil
	})
	if err != nil {
		return options{}, nil, "", err
	}
	if len(operands) == 0 {
		return options{}, nil, "", errors.New("missing operand")
	}
	if !opts.multiple {
		if len(operands) > 2 {
			return options{}, nil, "", fmt.Errorf("extra operand: %q", operands[2])
		}
		if len(operands) == 2 {
			return opts, operands[:1], operands[1], nil
		}
	}
	return opts, operands, opts.suffix, nil
}

// errorf writes a diagnostic message to standard error.
func (p *Prog) errorf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "basename: "+format+"\n", args...)
}

// basename returns the last component of path, with suffix removed when it's
// not identical to and is a suffix of the result. Only forward slashes are
// treated as separators, so callers must normalize Windows-style paths first.
func basename(path, suffix string) string {
	if path == "" {
		return ""
	}
	if strings.Trim(path, "/") == "" {
		return "/"
	}
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	if suffix != "" && suffix != path && strings.HasSuffix(path, suffix) {
		path = path[:len(path)-len(suffix)]
	}
	return path
}

// options holds the -a, -s and -z flags.
type options struct {
	multiple bool
	suffix   string
	zero     bool
}

// longOptions maps the long options of basename to the short ones they stand
// for.
var longOptions = map[string]rune{
	"multiple": 'a',
	"suffix":   's',
	"zero":     'z',
}

// optSet describes the options of basename.
var optSet = getopt.Set{Valued: "s", Long: longOptions}

// usage describes the options of basename.
const usage = `Usage: basename NAME [SUFFIX]
  or:  basename OPTION... NAME...
Print NAME with any leading directory components removed.
If specified, also remove a trailing SUFFIX.

  -a, --multiple       support multiple arguments and treat each one as a NAME
  -s, --suffix=SUFFIX  remove a trailing SUFFIX; implies -a
  -z, --zero           end each output line with NUL, not newline
      --help     display this help and exit
      --version  output version information and exit
`

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
