// Command fold is a thin wrapper around the fold core utility.
package main

import (
	"charm.land/turtle/coreutils/fold"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(fold.New)
}
