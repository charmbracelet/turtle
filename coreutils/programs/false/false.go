// Package false implements the POSIX false utility.
package false

import (
	"context"

	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/internal/getopt"
	"charm.land/turtle/coreutils/internal/stdopts"
)

// Prog is the false core utility.
type Prog struct {
	*coreutils.Runtime
}

// New returns a new Prog.
func New(rt *coreutils.Runtime) coreutils.Program {
	return &Prog{Runtime: rt}
}

// usage describes the options of false.
const usage = `Usage: false [ignored command line arguments]
Exit with a status code indicating failure.

` + stdopts.Tail

// Run returns a non-zero exit status, after handling the common options.
// As the GNU manual puts it, false always exits unsuccessfully, even when
// invoked with --help or --version.
func (p *Prog) Run(_ context.Context, args ...string) error {
	if done, err := stdopts.Check(p.Runtime, "false", usage, getopt.Set{}, args); done {
		if err != nil {
			return err
		}
		return &coreutils.ExitError{Code: 1}
	}
	return &coreutils.ExitError{Code: 1}
}

var (
	_ coreutils.Program = (*Prog)(nil)
	_ coreutils.NewFunc = New
)
