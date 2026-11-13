// Package zcat implements the zcat command.
package zcat

import (
	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/gzip"
)

// New returns a program which writes restored files to standard output.
func New(rt *coreutils.Runtime) coreutils.Program {
	return gzip.Zcat(rt)
}

var _ coreutils.NewFunc = New
