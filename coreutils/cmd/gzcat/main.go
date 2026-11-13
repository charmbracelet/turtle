// Command gzcat is a thin wrapper around the gzcat core utility.
package main

import (
	"charm.land/turtle/coreutils/gzcat"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(gzcat.New)
}
