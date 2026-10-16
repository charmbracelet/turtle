// Command rm is a thin wrapper around the rm core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/rm"
)

func main() {
	cmdutil.Main(rm.New)
}
