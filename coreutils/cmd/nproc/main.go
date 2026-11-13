// Command nproc is a thin wrapper around the nproc core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/nproc"
)

func main() {
	cmdutil.Main(nproc.New)
}
