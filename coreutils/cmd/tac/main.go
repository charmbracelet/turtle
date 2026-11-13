// Command tac is a thin wrapper around the tac core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/tac"
)

func main() {
	cmdutil.Main(tac.New)
}
