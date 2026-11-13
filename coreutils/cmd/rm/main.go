// Command rm is a thin wrapper around the rm core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/rm"
)

func main() {
	cmdutil.Main(rm.New)
}
