// Command printenv is a thin wrapper around the printenv core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/printenv"
)

func main() {
	cmdutil.Main(printenv.New)
}
