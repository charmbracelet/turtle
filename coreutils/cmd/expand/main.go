// Command expand is a thin wrapper around the expand core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/expand"
)

func main() {
	cmdutil.Main(expand.New)
}
