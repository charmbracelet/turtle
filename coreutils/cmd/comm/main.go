// Command comm is a thin wrapper around the comm core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/comm"
)

func main() {
	cmdutil.Main(comm.New)
}
