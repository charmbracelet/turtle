// Command expand is a thin wrapper around the expand core utility.
package main

import (
	"charm.land/turtle/coreutils/expand"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(expand.New)
}
