// Package gzcat implements the gzcat command.
package gzcat

import (
	"charm.land/turtle/coreutils"
	"charm.land/turtle/coreutils/gzip"
)

// New returns a program which writes restored files to standard output.
func New(rt *coreutils.Runtime) coreutils.Program {
	return gzip.Gzcat(rt)
}

var _ coreutils.NewFunc = New
