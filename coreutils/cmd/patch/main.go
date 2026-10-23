// Command patch is a thin wrapper around the patch core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/patch"
)

func main() {
	cmdutil.Main(patch.New)
}
