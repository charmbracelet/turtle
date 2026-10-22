// Command cmp is a thin wrapper around the cmp core utility.
package main

import (
	"charm.land/snail/coreutils/cmp"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(cmp.New)
}
