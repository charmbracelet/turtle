// Package gunzip implements the gunzip command.
package gunzip

import (
	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/gzip"
)

// New returns a program which restores compressed files.
func New(rt *coreutils.Runtime) coreutils.Program {
	return gzip.Gunzip(rt)
}

var _ coreutils.NewFunc = New
