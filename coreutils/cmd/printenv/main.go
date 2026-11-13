// Command printenv is a thin wrapper around the printenv core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/printenv"
)

func main() {
	cmdutil.Main(printenv.New)
}
