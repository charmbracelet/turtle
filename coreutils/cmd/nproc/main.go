// Command nproc is a thin wrapper around the nproc core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/nproc"
)

func main() {
	cmdutil.Main(nproc.New)
}
