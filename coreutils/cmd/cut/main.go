// Command cut is a thin wrapper around the cut core utility.
package main

import (
	"charm.land/snail/coreutils/cut"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(cut.New)
}
