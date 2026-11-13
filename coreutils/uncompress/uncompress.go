// Package uncompress implements the uncompress command.
package uncompress

import (
	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/compress"
)

// New returns a program which restores compressed files.
func New(rt *coreutils.Runtime) coreutils.Program {
	return compress.Uncompress(rt)
}

var _ coreutils.NewFunc = New
