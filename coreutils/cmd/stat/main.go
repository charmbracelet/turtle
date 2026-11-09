// Command stat is a thin wrapper around the stat core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/stat"
)

func main() {
	cmdutil.Main(stat.New)
}
