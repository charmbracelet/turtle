// Command comm is a thin wrapper around the comm core utility.
package main

import (
	"charm.land/turtle/coreutils/comm"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(comm.New)
}
