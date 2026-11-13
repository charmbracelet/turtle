// Command gzcat is a thin wrapper around the gzcat core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/gzcat"
)

func main() {
	cmdutil.Main(gzcat.New)
}
