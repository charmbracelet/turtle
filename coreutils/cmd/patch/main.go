// Command patch is a thin wrapper around the patch core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/patch"
)

func main() {
	cmdutil.Main(patch.New)
}
