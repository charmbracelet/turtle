// Command fold is a thin wrapper around the fold core utility.
package main

import (
	"charm.land/snail/coreutils/fold"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(fold.New)
}
