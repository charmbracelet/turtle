// Command xargs is a thin wrapper around the xargs core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/xargs"
)

func main() {
	cmdutil.Main(xargs.New)
}
