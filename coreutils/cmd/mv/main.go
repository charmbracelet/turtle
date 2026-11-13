// Command mv is a thin wrapper around the mv core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/mv"
)

func main() {
	cmdutil.Main(mv.New)
}
