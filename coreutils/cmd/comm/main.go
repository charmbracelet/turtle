// Command comm is a thin wrapper around the comm core utility.
package main

import (
	"charm.land/snail/coreutils/comm"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(comm.New)
}
