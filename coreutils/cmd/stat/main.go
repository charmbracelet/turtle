// Command stat is a thin wrapper around the stat core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/stat"
)

func main() {
	cmdutil.Main(stat.New)
}
