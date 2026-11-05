// Command realpath is a thin wrapper around the realpath core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/realpath"
)

func main() {
	cmdutil.Main(realpath.New)
}
