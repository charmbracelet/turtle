// Command split is a thin wrapper around the split core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/split"
)

func main() {
	cmdutil.Main(split.New)
}
