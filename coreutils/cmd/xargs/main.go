// Command xargs is a thin wrapper around the xargs core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/xargs"
)

func main() {
	cmdutil.Main(xargs.New)
}
