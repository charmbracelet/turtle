// Command cut is a thin wrapper around the cut core utility.
package main

import (
	"charm.land/turtle/coreutils/cut"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(cut.New)
}
