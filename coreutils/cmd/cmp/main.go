// Command cmp is a thin wrapper around the cmp core utility.
package main

import (
	"charm.land/turtle/coreutils/cmp"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(cmp.New)
}
