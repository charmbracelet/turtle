// Command fold is a thin wrapper around the fold core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/fold"
)

func main() {
	cmdutil.Main(fold.New)
}
