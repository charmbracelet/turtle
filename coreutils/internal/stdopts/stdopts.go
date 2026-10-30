// Package stdopts implements the --help and --version options common to the
// core utilities, as described by the "Common options" section of the GNU
// Coreutils manual.
package stdopts

import (
	"errors"
	"fmt"
	"io"
	"maps"

	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/internal/getopt"
)

// The runes which --help and --version map to. They're kept well above the
// range where a utility holds its long-only options.
const (
	help rune = 0x10000 + iota
	version
)

// errDone stops the parsing once the usage or version was written.
var errDone = errors.New("standard option handled")

// Check reports whether args ask for --help or --version, parsing them with
// set so the option-arguments of the utility are skipped, and an unknown
// long option or a missing option argument stops the scan. When one of the
// two is found, before any operand, its text is written to the standard
// output of rt, and Check reports done as true, with a nil error for the
// successful exit it asks for, or the write failure wrapped in an
// [*coreutils.ExitError] with status 1.
//
// Everything else is ignored here, including the options the utility may
// reject later: it parses the arguments again and reports them as usual.
func Check(rt *coreutils.Runtime, prog, usage string, set getopt.Set, args []string) (done bool, err error) {
	long := make(map[string]rune, len(set.Long)+2)
	maps.Copy(long, set.Long)
	long["help"], long["version"] = help, version
	set.Long = long

	var out string
	_, _ = set.Parse(args, func(opt rune, _ string) error {
		switch opt {
		case help:
			out = usage
		case version:
			out = fmt.Sprintf("%s (charm.land/snail/coreutils) %s\nhttps://charm.land/snail\n",
				prog, coreutils.Version)
		default:
			return nil
		}
		return errDone
	})
	if out == "" {
		return false, nil
	}
	if _, err := io.WriteString(rt.Stdout, out); err != nil {
		return true, &coreutils.ExitError{Code: 1, Err: err}
	}
	return true, nil
}

// TryHelp writes to the standard error the hint which follows the usage
// errors of a utility, as the GNU utilities do: invalid options, missing
// option arguments and missing or extra operands.
func TryHelp(rt *coreutils.Runtime, prog string) {
	fmt.Fprintf(rt.Stderr, "Try '%s --help' for more information.\n", prog)
}
