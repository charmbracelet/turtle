// Command expand is a thin wrapper around the expand core utility.
package main

import (
	"charm.land/snail/coreutils/expand"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(expand.New)
}
