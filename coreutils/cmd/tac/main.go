// Command tac is a thin wrapper around the tac core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/tac"
)

func main() {
	cmdutil.Main(tac.New)
}
