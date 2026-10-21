// Command uudecode is a thin wrapper around the uudecode core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/uudecode"
)

func main() {
	cmdutil.Main(uudecode.New)
}
