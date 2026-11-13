// Command cmp is a thin wrapper around the cmp core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/cmp"
)

func main() {
	cmdutil.Main(cmp.New)
}
