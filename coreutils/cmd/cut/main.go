// Command cut is a thin wrapper around the cut core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/cut"
)

func main() {
	cmdutil.Main(cut.New)
}
