// Package gzcat implements the gzcat command.
package gzcat

import (
	"charm.land/snail/coreutils"
	"charm.land/snail/coreutils/gzip"
)

// New returns a program which writes restored files to standard output.
func New(rt *coreutils.Runtime) coreutils.Program {
	return gzip.Gzcat(rt)
}

var _ coreutils.NewFunc = New
