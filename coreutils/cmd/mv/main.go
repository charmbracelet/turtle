// Command mv is a thin wrapper around the mv core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/mv"
)

func main() {
	cmdutil.Main(mv.New)
}
